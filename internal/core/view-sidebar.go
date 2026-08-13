package core

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// viewSidebar 는 포커스가 좌측 파일 트리에 있는 상태다.
//
// 키 해석이 통째로 다르다 — 위아래가 커서 이동이 아니라 트리 이동이다.
// 그래서 editor 에 focus 필드를 두고 mode 마다 분기하는 대신 型 을 따로 뒀다(ADR-0002).
// 트리 자체(펼친 상태, 고른 항목)는 mode 를 넘어 살아야 하므로 editor.sidebar 에 있다.
func sidebarMode(e editor) (tea.Model, tea.Cmd) {
	e.sidebar.scrollTo(e.sidebarHeight())

	return viewSidebar{editor: e}, nil
}

type viewSidebar struct {
	editor

	// pending 은 ctrl+w 처럼 뒤에 키가 하나 더 붙는 접두 키다.
	pending string
}

func (m viewSidebar) Init() tea.Cmd { return nil }

func (m viewSidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져서 sidebar 가 자동으로 숨으면 포커스가 안 보이는 곳에 남는다.
		// 다른 mode 의 크기 처리와 달리 여기만 한 줄로 끝나지 않는다.
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}
		m.sidebar.scrollTo(m.sidebarHeight())

		return m, nil
	case tea.ResumeMsg:
		// 내려가 있는 동안의 commit·checkout 을 statusBar 에 반영한다. git 표시는 mode 와
		// 무관하게 보이고 알림을 띄울 자리가 필요하지 않아서 편집 화면과 같이 읽는다.
		// 파일이 밖에서 바뀌었는지는 여기서도 보지 않는다 (ADR-0009, ADR-0023).
		m.git = readGitStatus()

		return m, nil
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		// 편집 화면과 같은 방식이다 — 음절 하나가 키 여럿으로 풀리므로 차례로 먹인다.
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			tree, ok := model.(viewSidebar)
			if !ok {
				// 앞의 키에서 포커스가 옮겨갔다. 남은 키는 버린다.
				return model, nil
			}

			next, cmd := tree.press(key)
			if cmd != nil {
				return next, cmd
			}

			model = next
		}

		return model, nil
	case tea.MouseClickMsg:
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			return m.click(mouse)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다.
func (m viewSidebar) press(key string) (tea.Model, tea.Cmd) {
	// 접두 키를 기다리고 있었으면 이 키가 그 뒤에 붙는 키다.
	// esc 와 ctrl+c 보다 먼저 봐야 `ctrl+w esc` 가 sidebar 를 나가버리지 않는다.
	if m.pending != "" {
		pending := m.pending
		m.pending = ""

		if pending == "ctrl+w" && (key == "ctrl+w" || key == "w") {
			return normalMode(m.editor)
		}

		return m, nil
	}

	switch key {
	case "ctrl+c":
		// 다른 mode 와 같은 경로다. 확인창에서 취소하면 여기로 돌아온다.
		return quitAll(m, m.editor)
	case "ctrl+z":
		// 편집 화면과 같이 셸로 내려간다. 포커스는 트리에 그대로 두고 올라온다.
		// 파일이 밖에서 바뀌었는지는 보지 않는다 — 지금 보고 있는 것은 파일 내용이 아니다(ADR-0023).
		return m, tea.Suspend
	case "ctrl+w":
		m.pending = "ctrl+w"

		return m, nil
	case "ctrl+p":
		// 트리를 뒤지다 이름으로 건너뛰는 길이다. 팔레트가 끝나면 편집 영역으로 나온다.
		return paletteMode(m.editor)
	case "esc":
		return normalMode(m.editor)
	case "up", "k":
		m.sidebar.selected--
	case "down", "j":
		m.sidebar.selected++
	case "enter":
		return m.enter()
	default:
		return m, nil
	}

	m.sidebar.scrollTo(m.sidebarHeight())

	return m, nil
}

// enter 는 고른 항목을 연다. 디렉터리면 펼치거나 접고, 파일이면 tab 으로 연다.
func (m viewSidebar) enter() (tea.Model, tea.Cmd) {
	node := m.sidebar.selectedNode()
	if node == nil {
		return m, nil
	}

	if node.isDir && !node.symlink {
		node.toggle()
		m.sidebar.scrollTo(m.sidebarHeight())

		return m, nil
	}

	// 열기 전에 지금 무엇인지 다시 본다. 트리는 펼칠 때 읽은 것이라 그 사이에 지워졌을 수 있다.
	//
	// Stat 은 symlink 를 따라가므로 링크가 가리키는 것이 무엇인지로 판단한다.
	// 일반 파일이 아니면 열지 않는다 — 디렉터리를 가리키는 링크는 ReadFile 이 EISDIR 을 내고,
	// FIFO 나 소켓은 ReadFile 이 영영 돌아오지 않아서 편집기가 통째로 멈춘다.
	info, err := os.Stat(node.path)
	if err != nil {
		return normalModeMessage(m.editor, errors.Cause(err).Error())
	}
	if !info.Mode().IsRegular() {
		return normalModeMessage(m.editor, "일반 파일이 아닙니다: "+node.name)
	}

	if err := m.openTab(node.path); err != nil {
		return normalModeMessage(m.editor, errors.Cause(err).Error())
	}
	m.buffer().scrollTo(m.contentWidth(), m.textHeight())

	// 연 파일을 보러 왔으므로 포커스도 편집 영역으로 간다. 돌아올 때는 ctrl+w ctrl+w 다.
	return normalMode(m.editor)
}

func (m viewSidebar) View() tea.View {
	// 고른 항목을 아래 줄에 보여준다. 편집 중인 파일의 커서 위치는 지금 볼 것이 아니다.
	// 접두 키를 기다리는 중이면 오른쪽 끝에 그것도 같이 보여준다.
	view := m.render(tea.CursorBlock, "TREE", m.withShowcmd(m.sidebar.selectedLabel(), m.pending))

	// 커서는 편집 내용이 아니라 고른 트리 항목 위에 있어야 한다.
	// 명령줄 mode 가 하는 것과 같은 방식이다. 이 커서가 곧 포커스 표시다.
	//
	// sidebar 는 화면 맨 윗줄부터 시작하므로 트리 행 번호가 곧 화면 행이다.
	if row, ok := m.sidebar.selectedRow(m.sidebarHeight()); ok {
		view.Cursor = tea.NewCursor(0, row)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}
