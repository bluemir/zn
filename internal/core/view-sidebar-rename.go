package core

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// renamePrompt 는 이름을 바꾸는 동안 statusBar 아래 줄 앞에 서는 말이다.
// 만들 때의 `새 파일: ` 과 짝이라 무엇을 치고 있는지가 그 줄만 보고 갈린다.
const renamePrompt = "새 이름: "

// renameFileMode 는 이름을 고치는 화면이다. 트리에서 `mm` 으로 들어온다.
//
// 만들기(`mc`) 와 달리 **칸이 비어 있지 않다.** 지금 경로가 뿌리 기준으로 채워져 있고 커서는
// 그 끝이다. 한 글자만 고치거나 확장자만 두고 앞을 고치는 것이 이 동작의 거의 전부이고,
// 앞부분을 고치면 자리를 옮기는 것이 된다 — `mv` 하나로 이름과 자리를 함께 다루는 셈이다(ADR-0054).
//
// 그래서 치는 것이 이름이 아니라 **경로**다. 만들기가 고른 디렉터리를 기준으로 삼는 것과 다르게
// 여기는 뿌리를 기준으로 읽는다. 화면에 그 경로가 그대로 보이므로 어느 기준인지 눈에 남는다.
//
// 이름을 받는 동안 키가 동작이 아니라 글자인 것은 만들기와 같다(ADR-0008, ADR-0054).
func renameFileMode(e *editor, node *treeNode) (tea.Model, tea.Cmd) {
	return viewSidebarRename{
		editor: e,
		from:   node.path,
		isDir:  node.isDir && !node.isSymlink,
		input:  e.sidebar.relLabel(node.path),
	}, nil
}

type viewSidebarRename struct {
	*editor

	from  string // 지금 경로. 절대 경로다
	isDir bool   // 디렉터리인가. symlink 는 링크만 옮기므로 파일 쪽이다
	input string // 치고 있는 경로. 뿌리 기준 상대 경로다
}

func (m viewSidebarRename) Init() tea.Cmd { return nil }

func (m viewSidebarRename) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져 트리가 숨으면 고치던 것을 버리고 편집 영역으로 내보낸다.
		// 트리에 포커스를 두는 mode 는 모두 같은 규칙이다.
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return sidebarMode(m.editor)
		case "enter":
			return m.rename()
		case "backspace":
			// 다 지워도 이 화면에 남는다. 만들기와 다른 자리다 — 거기서는 빈 칸이 시작점이라
			// 다 지운 것이 「아무것도 치지 않았다」 지만, 여기서는 채워져 있던 것을 지운 것이라
			// 그만두려는 뜻으로 읽을 수 없다. 그만두는 것은 `esc` 다.
			if m.input == "" {
				return m, nil
			}
			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]

			return m, nil
		default:
			// 이름이 될 수 없는 키(방향키, ctrl 조합) 는 Text 가 비어 있다.
			if msg.Text == "" {
				return m, nil
			}
			m.input += msg.Text

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 고치는 동안에도 화면은 둘러볼 수 있다. 클릭은 받지 않는다 —
		// 치던 것이 클릭 한 번에 조용히 사라지면 안 된다. 명령줄과 같은 규칙이다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
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

// rename 은 친 경로로 옮긴다. 이름만 고쳤으면 제자리 이름 바꾸기이고, 앞을 고쳤으면 자리 옮기기다.
//
// 없는 중간 디렉터리는 같이 만든다. 만들기(`mc`) 와 같은 규칙이라 `docs/spec.md` 를 치면
// `docs/` 가 없어도 된다.
func (m viewSidebarRename) rename() (tea.Model, tea.Cmd) {
	// 앞뒤 빈 칸은 눌러 둔 자국이다. 가운데 빈 칸은 파일 이름에 쓸 수 있으므로 건드리지 않는다.
	name := strings.TrimSpace(m.input)
	if name == "" {
		return sidebarModeMessage(m.editor, "이름이 없습니다")
	}

	to := filepath.Join(m.sidebar.root, name)
	if !m.sidebar.underRoot(to) {
		return sidebarModeMessage(m.editor, "뿌리 밖으로는 옮길 수 없습니다: "+name)
	}
	if to == m.from {
		return sidebarModeMessage(m.editor, "이름이 그대로입니다")
	}

	// 디렉터리를 자기 안으로 옮길 수 없다. os.Rename 이 내는 `invalid argument` 로는
	// 무엇이 잘못됐는지 읽히지 않는다.
	if m.isDir && strings.HasPrefix(to, m.from+string(filepath.Separator)) {
		return sidebarModeMessage(m.editor, "디렉터리를 자기 안으로 옮길 수 없습니다")
	}

	// 이미 있는 이름 위로 옮기지 않는다. os.Rename 은 묻지 않고 덮어써서, 그대로 두면
	// 이름 바꾸기 한 번이 다른 파일을 없앤다.
	//
	// 대소문자만 바꾸는 것은 예외다. 대소문자를 가리지 않는 파일 시스템(APFS·NTFS) 에서는
	// 새 이름이 자기 자신으로 잡히므로, 같은 파일이면 지나간다.
	if target, err := os.Lstat(to); err == nil {
		source, err := os.Lstat(m.from)
		if err != nil || !os.SameFile(target, source) {
			return sidebarModeMessage(m.editor, "이미 있습니다: "+m.sidebar.relLabel(to))
		}
	}

	if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return sidebarModeError(m.editor, errors.Mark(err, errRenameFile))
	}
	if err := os.Rename(m.from, to); err != nil {
		return sidebarModeError(m.editor, errors.Mark(err, errRenameFile))
	}

	// 그 파일을 열어둔 tab 은 새 이름을 따라간다(ADR-0054).
	m.renameBuffers(m.from, to, m.isDir)

	// 옛 자리와 새 자리를 다시 읽는다. 같은 디렉터리 안에서 이름만 바꿨으면 한 번이다.
	// 새 자리가 아직 트리에 없으면 refreshDir 이 아무 일도 하지 않고, 아래 reveal 이
	// 뿌리부터 한 층씩 펼치며 내려간다(ADR-0032).
	refresh := m.refreshDir(filepath.Dir(m.from))

	var moved tea.Cmd
	if filepath.Dir(to) != filepath.Dir(m.from) {
		moved = m.refreshDir(filepath.Dir(to))
	}

	// 고른 자리는 옮겨간 그 파일이다. 이름을 바꾼 뒤 어디로 갔는지 눈으로 따라갈 수 있어야 한다.
	reveal := m.revealInSidebar(to)

	message := "이름을 바꿨습니다: " + m.label(m.from) + " → " + m.label(to)

	// git 이 아는 파일이 없어지고 새 이름이 생긴 것이라 저장소 상태가 달라진다(ADR-0030).
	model, cmd := sidebarModeMessage(m.editor, message)

	return model, tea.Batch(cmd, refresh, moved, reveal, m.startGitRefresh())
}

// label 은 알림에 쓰는 이름이다. 디렉터리는 `/` 를 붙여 파일과 갈린다.
func (m viewSidebarRename) label(path string) string {
	if m.isDir {
		return m.sidebar.relLabel(path) + "/"
	}

	return m.sidebar.relLabel(path)
}

func (m viewSidebarRename) View() tea.View {
	line := renamePrompt + m.input
	view := m.editorView(tea.CursorBlock, "TREE", line)

	// 커서는 치고 있는 경로 끝이다. 아래 줄은 편집 영역 아래에서 시작하므로 sidebar 만큼
	// 오른쪽으로 옮긴다. 명령줄과 같다.
	view.Cursor = tea.NewCursor(screenWidthOf(line)+m.sidebarLeft(), m.height-1)

	return view
}
