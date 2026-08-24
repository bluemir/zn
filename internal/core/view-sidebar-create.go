package core

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// createPrompt 는 이름을 받는 동안 statusBar 아래 줄 앞에 서는 말이다.
// 명령줄의 `:` 와 같은 자리라 무엇을 치고 있는지가 그 줄만 보고 갈린다.
const createPrompt = "새 파일: "

// createFileMode 는 새 파일 이름을 받는 화면이다. 트리에서 `mc` 로 들어온다.
//
// **키가 동작이 아니라 글자다.** 그래서 sidebarState 에 상태를 하나 더하는 대신 mode 를
// 따로 뒀다(ADR-0054). 트리의 키는 두벌식 자리의 영문 키로 되돌려 받는데(ADR-0008) 이름은
// 되돌리면 안 된다 — `한글.txt` 가 `gksrmf.txt` 가 된다. 여기는 명령줄과 같이 `msg.Text` 를
// 그대로 붙인다.
//
// dir 은 만들 자리다. 고른 항목이 디렉터리면 그 안, 파일이면 그 파일이 있는 디렉터리다.
func createFileMode(e *editor, dir string) (tea.Model, tea.Cmd) {
	return viewSidebarCreate{editor: e, dir: dir}, nil
}

type viewSidebarCreate struct {
	*editor

	dir   string // 만들 자리의 절대 경로
	input string // 치고 있는 이름
}

func (m viewSidebarCreate) Init() tea.Cmd { return nil }

func (m viewSidebarCreate) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져 트리가 숨으면 만들 자리가 화면에서 사라진다. 치던 이름을 버리고
		// 편집 영역으로 내보낸다 — 트리에 포커스를 두는 mode 는 모두 같은 규칙이다.
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
			return m.create()
		case "backspace":
			// 명령줄과 같다. 다 지우면 만들던 것을 그만두고 트리로 돌아간다.
			if m.input == "" {
				return sidebarMode(m.editor)
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
		// 이름을 치는 동안에도 화면은 둘러볼 수 있다. 클릭은 받지 않는다 —
		// 치던 이름이 클릭 한 번에 조용히 사라지면 안 된다. 명령줄과 같은 규칙이다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg, renameMsg:
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

// create 는 친 이름으로 파일을 만든다.
//
//	main.go        dir 안에 파일을 만들고 tab 으로 연다
//	docs/spec.md   없는 `docs/` 를 같이 만든다
//	docs/          디렉터리만 만들고 트리에 머문다
//
// 이미 있는 이름은 만들지 않는다. 덮어쓰면 그 파일의 내용이 조용히 사라지고, 트리는 이미
// 그 이름을 보여주고 있어서 무엇이 없어졌는지도 화면에 남지 않는다.
func (m viewSidebarCreate) create() (tea.Model, tea.Cmd) {
	// 앞뒤 빈 칸은 눌러 둔 자국이다. 가운데 빈 칸은 파일 이름에 쓸 수 있으므로 건드리지 않는다.
	name := strings.TrimSpace(m.input)
	if name == "" {
		return sidebarModeMessage(m.editor, "이름이 없습니다")
	}

	// `docs/` 처럼 `/` 로 끝나면 디렉터리를 만드는 것이다.
	// filepath.Join 이 그 자국을 지우므로 먼저 본다.
	isDir := strings.HasSuffix(name, "/")

	path := filepath.Join(m.dir, name)
	if !m.sidebar.underRoot(path) {
		return sidebarModeMessage(m.editor, "뿌리 밖에는 만들 수 없습니다: "+name)
	}

	// Lstat 이다. symlink 가 이미 그 이름을 쓰고 있으면 그것도 있는 것이다 —
	// Stat 은 끊어진 링크를 없는 것으로 보고, 그 자리에 파일을 만들면 링크가 가리키던 곳에 쓴다.
	if _, err := os.Lstat(path); err == nil {
		return sidebarModeMessage(m.editor, "이미 있습니다: "+m.sidebar.relLabel(path))
	}

	if isDir {
		if err := os.MkdirAll(path, 0755); err != nil {
			return sidebarModeError(m.editor, errors.Mark(err, errCreateFile))
		}

		// 디렉터리는 열 것이 없으므로 트리에 머문다. 만든 자리가 다음에 무언가를 만들 자리다.
		model, cmd := sidebarModeMessage(m.editor, "만들었습니다: "+m.sidebar.relLabel(path)+"/")

		return model, tea.Batch(cmd, m.refreshDir(m.dir), m.startGitRefresh())
	}

	// 중간 디렉터리는 같이 만든다. `docs/spec.md` 를 치면 `docs/` 가 없어도 된다.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return sidebarModeError(m.editor, errors.Mark(err, errCreateFile))
	}

	// O_EXCL 이라 그 사이에 같은 이름이 생겼으면 여기서 걸린다. 위의 Lstat 은 알아보고
	// 알려주기 위한 것이고, 덮어쓰지 않는다는 것을 지키는 것은 이 flag 다.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return sidebarModeError(m.editor, errors.Mark(err, errCreateFile))
	}
	file.Close()

	// 트리를 먼저 맞춘다. openTab 이 그 파일 자리로 트리를 데려가는데(ADR-0019) 다시 읽기
	// 전이면 그 자리에 아직 방금 만든 이름이 없어서 가던 길이 그대로 끝난다.
	//
	// 다시 읽는 자리는 이름을 받은 dir 이다. `a/b/c.go` 를 쳐도 dir 안에 새로 생긴 것은 `a/`
	// 하나이고 그 아래는 reveal 이 한 층씩 펼치며 읽는다(ADR-0032).
	refresh := m.refreshDir(m.dir)

	open, err := m.openTab(path)
	if err != nil {
		return sidebarModeError(m.editor, err)
	}
	m.scrollToCursor()

	// 만들자마자 쓰는 것이므로 포커스도 편집 영역으로 간다. `enter` 로 파일을 여는 것과 같다.
	// 만든 파일은 git 이 모르는 파일이라 저장소 상태가 달라진다(ADR-0030).
	model, cmd := normalModeMessage(m.editor, "만들었습니다: "+m.sidebar.relLabel(path))

	return model, tea.Batch(cmd, refresh, open, m.startGitRefresh())
}

func (m viewSidebarCreate) View() tea.View {
	// 아래 줄은 치고 있는 이름이다. 만들 자리를 앞에 붙여야 어디에 생기는지가 보인다 —
	// 트리 커서는 이 mode 에서 이름 끝으로 옮겨가 있어서 고른 자리를 가리키지 못한다.
	line := createPrompt + m.createDirLabel() + m.input
	view := m.editorView(tea.CursorBlock, "TREE", line)

	// 커서는 치고 있는 이름 끝이다. 아래 줄은 편집 영역 아래에서 시작하므로 sidebar 만큼
	// 오른쪽으로 옮긴다. 명령줄과 같다.
	view.Cursor = tea.NewCursor(screenWidthOf(line)+m.sidebarLeft(), m.height-1)

	return view
}

// createDirLabel 은 만들 자리를 이름 앞에 보일 형태로 준다. 뿌리 기준 상대 경로에 `/` 를 붙인다.
func (m viewSidebarCreate) createDirLabel() string {
	label := m.sidebar.relLabel(m.dir)

	// 뿌리는 relLabel 이 이미 `zn/` 처럼 준다. 그 밖은 `docs` 라 자국을 붙인다.
	if strings.HasSuffix(label, "/") {
		return label
	}

	return label + "/"
}
