package core

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// removeTreeEntry 는 정말 지운다. 곧바로 지우는 파일과 확인을 거친 디렉터리가 같이 쓴다.
//
// 지울 수 있는 자리인지는 이미 걸러져 있다(viewSidebar.deleteFile). 여기서는 지우고,
// 그 자리를 다시 읽고, 무엇이 사라졌는지 남기는 것만 한다.
//
// 지운 뒤에도 트리에 머문다. 파일을 만들고 지우는 것은 몇 번을 이어서 하는 일이라, 결과마다
// 다른 화면으로 내보내면 그 되풀이가 끊긴다(ADR-0054).
func removeTreeEntry(e *editor, path string, isDir bool) (tea.Model, tea.Cmd) {
	var err error
	if isDir {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		return sidebarModeError(e, errors.Mark(err, errRemoveFile))
	}

	label := e.sidebar.relLabel(path)
	if isDir {
		label += "/"
	}

	// 지운 자리를 다시 읽는다. 고른 자리는 번호로 들고 있어서 한 행이 빠지면 그 아래 항목이
	// 그 자리에 올라온다 — 편집 영역에서 `dd` 뒤에 커서가 다음 줄에 서는 것과 같다.
	refresh := e.refreshDir(filepath.Dir(path))

	// 파일은 묻지 않고 지우므로 이 알림이 무엇이 사라졌는지 말하는 유일한 자리다(ADR-0057).
	// 지나간 것은 `:messages` 에 남는다(ADR-0053).
	model, cmd := sidebarModeMessage(e, "지웠습니다: "+label)

	// 지운 것이 git 이 아는 파일이었으면 저장소 상태가 달라진다(ADR-0030).
	return model, tea.Batch(cmd, refresh, e.startGitRefresh())
}

// deleteDirMode 는 디렉터리를 지우기 전에 한 번 더 묻는 화면이다. 트리에서 `md` 로 들어온다.
//
// **파일은 여기 오지 않는다.** 묻지 않고 곧바로 지운다(ADR-0057). 디렉터리만 묻는 것은
// 안의 것까지 통째로 사라지고 그 안에 무엇이 있었는지 화면에 드러나지 않기 때문이다 —
// 접혀 있으면 트리는 이름 한 줄만 보여준다.
//
// 확인창(ConfirmDiscard) 을 띄우지 않고 statusBar 아래 줄에서 묻는다. 무엇을 지우는지 보여주는
// 것이 트리 그 자리이므로, 화면을 덮어 그 자리를 가리면 무엇을 고르고 있었는지가 사라진다.
//
// 노드 포인터가 아니라 경로를 든다. 묻는 사이에 디렉터리 읽기가 끝나면 그 포인터는 이미
// 어느 화면에도 없을 수 있다 — readDirJob 의 apply 가 경로로 다시 찾는 것과 같은 이유다.
func deleteDirMode(e *editor, node *treeNode) (tea.Model, tea.Cmd) {
	return viewSidebarDelete{editor: e, path: node.path}, nil
}

type viewSidebarDelete struct {
	*editor

	path string // 지울 디렉터리의 절대 경로
}

func (m viewSidebarDelete) Init() tea.Cmd { return nil }

func (m viewSidebarDelete) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져 트리가 숨으면 무엇을 지우는지 보여줄 자리가 없어진다.
		// 묻던 것을 그만두고 편집 영역으로 내보낸다.
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}
		m.sidebar.scrollTo(m.sidebarHeight())

		return m, nil
	case tea.KeyPressMsg:
		// 한글 입력 상태의 키는 두벌식 자리의 영문 키로 되돌린다. `ㅛ` 가 `y` 다(ADR-0008).
		// 음절 하나가 키 여럿으로 풀릴 수 있지만 여기서 뜻이 있는 것은 첫 키뿐이다 —
		// 나머지는 무엇이든 취소이고, 취소는 되풀이해도 취소다.
		return m.press(expandHangul(msg.String())[0])
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, watchMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go).
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다.
//
// **`y` 만 지우고 나머지는 전부 취소다.** 되돌릴 수 없는 일 앞에서 잘못 누른 키가 실행이
// 되어서는 안 된다 — `n`·`esc` 를 외워야 무를 수 있으면 그 사이의 오타가 디렉터리를 지운다.
// `ctrl+c` 도 여기서는 취소다. 편집기를 끄는 것이 아니라 묻던 것을 무르는 것이다.
func (m viewSidebarDelete) press(key string) (tea.Model, tea.Cmd) {
	if key != "y" {
		return sidebarModeMessage(m.editor, "지우지 않았습니다")
	}

	return m.remove()
}

// remove 는 정말 지운다.
//
// 안의 것까지 통째로 지운다. 빈 것만 지우게 하면 안을 하나씩 비우는 동안 트리를 오르내려야
// 하고, 그 되풀이가 더 위험하다.
func (m viewSidebarDelete) remove() (tea.Model, tea.Cmd) {
	return removeTreeEntry(m.editor, m.path, true)
}

func (m viewSidebarDelete) View() tea.View {
	view := m.editorView(tea.CursorBlock, "TREE", m.question())

	// 커서는 지울 항목 위에 그대로 둔다. 아래 줄이 이름을 말하고 커서가 그 자리를 가리켜서,
	// 무엇을 지우는지 두 곳에서 맞춰 볼 수 있다. 트리 화면과 같은 자리다.
	if row, ok := m.sidebar.selectedRow(m.sidebarHeight()); ok {
		view.Cursor = tea.NewCursor(0, row)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// question 은 아래 줄에 서는 물음이다.
//
// 「안의 것까지」를 앞에 둔다. 이름만 보고 `y` 를 누르는 것을 막는 것이 이 물음이 남은
// 까닭이라, 무엇을 잃는지가 이름보다 먼저 와야 한다.
func (m viewSidebarDelete) question() string {
	return "안의 것까지 모두 지울까요? " + m.sidebar.relLabel(m.path) + "/ (y/n)"
}
