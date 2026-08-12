package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorNormal 은 normal mode 다. 커서가 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
func normalMode(e editor) (tea.Model, tea.Cmd) {
	return viewEditorNormal{editor: e}, nil
}

// normalModeMessage 는 명령 결과를 아래 줄에 띄운 채로 normal 로 돌아간다.
func normalModeMessage(e editor, message string) (tea.Model, tea.Cmd) {
	return viewEditorNormal{editor: e, message: message}, nil
}

type viewEditorNormal struct {
	editor

	// message 는 명령 결과나 오류다. 다음 키를 누르면 사라진다.
	message string
}

func (m viewEditorNormal) Init() tea.Cmd { return nil }

func (m viewEditorNormal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		buf := m.buffer()

		// 알림은 다음 키를 누르면 사라진다.
		m.message = ""

		switch msg.String() {
		case "ctrl+c":
			// :q 와 같은 경로다. 저장하지 않은 변경이 있을 때만 확인창이 뜬다.
			return quit(m, buf)
		case ":":
			return commandMode(m.editor)
		case "i":
			// 커서 앞에 넣는다. 커서는 그대로다.
			return insertMode(m.editor)
		case "a":
			// 커서 글자 뒤에 넣는다.
			// 줄 끝 다음 칸은 insert mode 에서만 갈 수 있어서 mode 를 먼저 바꾼다.
			next, cmd := insertMode(m.editor)
			buf.moveRight(m.width)
			buf.scrollTo(m.width, m.textHeight())

			return next, cmd
		case "u":
			buf.applyUndo(m.width)
			buf.clampToNormal(m.width)
		case "ctrl+r":
			buf.applyRedo(m.width)
			buf.clampToNormal(m.width)
		case "up":
			buf.moveUp(1, m.width)
			buf.clampToNormal(m.width)
		case "down":
			buf.moveDown(1, m.width)
			buf.clampToNormal(m.width)
		case "left":
			buf.moveLeft(m.width)
		case "right":
			buf.moveRight(m.width)
			buf.clampToNormal(m.width)
		default:
			return m, nil
		}

		buf.scrollTo(m.width, m.textHeight())

		return m, nil
	default:
		return m, nil
	}
}

func (m viewEditorNormal) View() tea.View {
	bottom := m.position()
	if m.message != "" {
		bottom = m.message
	}

	// 커서가 글자 위에 있으므로 블록이다.
	return m.render(tea.CursorBlock, "NORMAL", bottom)
}
