package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewSidebar 는 포커스가 좌측 파일 트리에 있는 상태다.
//
// 키 해석이 통째로 다르다 — 위아래가 커서 이동이 아니라 트리 이동이다.
// 그래서 editor 에 focus 필드를 두고 mode 마다 분기하는 대신 型 을 따로 뒀다(ADR-0002).
// 트리 자체(펼친 상태, 고른 항목)는 mode 를 넘어 살아야 하므로 editor.sidebar 에 있다.
func sidebarMode(e editor) (tea.Model, tea.Cmd) {
	e.sidebar.scrollTo(e.textHeight())

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
		m.sidebar.scrollTo(m.textHeight())

		return m, nil
	case tea.KeyPressMsg:
		// 접두 키를 기다리고 있었으면 이 키가 그 뒤에 붙는 키다.
		// esc 와 ctrl+c 보다 먼저 봐야 `ctrl+w esc` 가 sidebar 를 나가버리지 않는다.
		if m.pending != "" {
			pending := m.pending
			m.pending = ""

			if pending == "ctrl+w" && (msg.String() == "ctrl+w" || msg.String() == "w") {
				return normalMode(m.editor)
			}

			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			// 다른 mode 와 같은 경로다. 확인창에서 취소하면 여기로 돌아온다.
			return quitAll(m, m.editor)
		case "ctrl+w":
			m.pending = "ctrl+w"

			return m, nil
		case "esc":
			return normalMode(m.editor)
		case "up":
			m.sidebar.selected--
		case "down":
			m.sidebar.selected++
		case "enter":
			return m.enter()
		default:
			return m, nil
		}

		m.sidebar.scrollTo(m.textHeight())

		return m, nil
	default:
		return m, nil
	}
}

// enter 는 고른 항목을 연다. 디렉터리면 펼치거나 접고, 파일이면 tab 으로 연다.
func (m viewSidebar) enter() (tea.Model, tea.Cmd) {
	node := m.sidebar.selectedNode()
	if node == nil {
		return m, nil
	}

	if node.isDir && !node.symlink {
		node.toggle()
		m.sidebar.scrollTo(m.textHeight())

		return m, nil
	}

	return m, nil
}

func (m viewSidebar) View() tea.View {
	// 고른 항목을 아래 줄에 보여준다. 편집 중인 파일의 커서 위치는 지금 볼 것이 아니다.
	view := m.render(tea.CursorBlock, "TREE", m.sidebar.selectedLabel())

	// 커서는 편집 내용이 아니라 고른 트리 항목 위에 있어야 한다.
	// 명령줄 mode 가 하는 것과 같은 방식이다. 이 커서가 곧 포커스 표시다.
	if row, ok := m.sidebar.selectedRow(m.textHeight()); ok {
		view.Cursor = tea.NewCursor(0, tablineHeight+row)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}
