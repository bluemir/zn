package core

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// deleteFileMode 는 지우기 전에 한 번 더 묻는 화면이다. 트리에서 `md` 로 들어온다.
//
// 지우기는 편집기 안에서 되돌릴 수 없는 유일한 동작이다. undo 가 닿지 않고 `:w` 로도 돌아오지
// 않으므로 키 두 번으로 끝나서는 안 된다(ADR-0054).
//
// 확인창(ConfirmDiscard) 을 띄우지 않고 statusBar 아래 줄에서 묻는다. 무엇을 지우는지 보여주는
// 것이 트리 그 자리이므로, 화면을 덮어 그 자리를 가리면 무엇을 고르고 있었는지가 사라진다.
//
// 노드 포인터가 아니라 경로를 든다. 묻는 사이에 디렉터리 읽기가 끝나면 그 포인터는 이미
// 어느 화면에도 없을 수 있다 — readDirJob 의 apply 가 경로로 다시 찾는 것과 같은 이유다.
func deleteFileMode(e *editor, node *treeNode) (tea.Model, tea.Cmd) {
	return viewSidebarDelete{editor: e, path: node.path, isDir: node.isDir && !node.isSymlink}, nil
}

type viewSidebarDelete struct {
	*editor

	path  string // 지울 것의 절대 경로
	isDir bool   // 디렉터리인가. symlink 는 링크만 지우므로 파일 쪽이다
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
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg:
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
// 되어서는 안 된다 — `n`·`esc` 를 외워야 무를 수 있으면 그 사이의 오타가 파일을 지운다.
// `ctrl+c` 도 여기서는 취소다. 편집기를 끄는 것이 아니라 묻던 것을 무르는 것이다.
func (m viewSidebarDelete) press(key string) (tea.Model, tea.Cmd) {
	if key != "y" {
		return sidebarModeMessage(m.editor, "지우지 않았습니다")
	}

	return m.remove()
}

// remove 는 정말 지운다.
//
// 디렉터리는 안의 것까지 통째로 지운다. 빈 것만 지우게 하면 안을 하나씩 비우는 동안 트리를
// 오르내려야 하고, 그 되풀이가 더 위험하다. 무엇을 잃는지는 묻는 문구가 파일과 나눠 말한다.
func (m viewSidebarDelete) remove() (tea.Model, tea.Cmd) {
	var err error
	if m.isDir {
		err = os.RemoveAll(m.path)
	} else {
		err = os.Remove(m.path)
	}
	if err != nil {
		return sidebarModeError(m.editor, errors.Mark(err, errRemoveFile))
	}

	label := m.sidebar.relLabel(m.path)
	if m.isDir {
		label += "/"
	}

	// 지운 자리를 다시 읽는다. 고른 자리는 번호로 들고 있어서 한 행이 빠지면 그 아래 항목이
	// 그 자리에 올라온다 — 편집 영역에서 `dd` 뒤에 커서가 다음 줄에 서는 것과 같다.
	refresh := m.refreshDir(filepath.Dir(m.path))

	// 지운 것이 git 이 아는 파일이었으면 저장소 상태가 달라진다(ADR-0030).
	model, cmd := sidebarModeMessage(m.editor, "지웠습니다: "+label)

	return model, tea.Batch(cmd, refresh, m.startGitRefresh())
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
// 디렉터리는 문구를 나눈다. 파일 하나를 지우는 것과 그 아래를 통째로 지우는 것은 잃는 것이
// 다르므로, 같은 문구로 물으면 `y` 를 같은 무게로 누른다.
func (m viewSidebarDelete) question() string {
	if m.isDir {
		return "안의 것까지 모두 지울까요? " + m.sidebar.relLabel(m.path) + "/ (y/n)"
	}

	return "지울까요? " + m.sidebar.relLabel(m.path) + " (y/n)"
}
