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

	// pending 은 `g` 처럼 뒤에 키가 하나 더 붙는 접두 키다. 다음 키를 받으면 비워진다.
	// mode 안에서만 사는 상태라 editor 가 아니라 여기에 둔다(ADR-0002).
	pending string
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

		// 접두 키를 기다리고 있었으면 이 키가 그 뒤에 붙는 키다.
		if m.pending != "" {
			pending := m.pending
			m.pending = ""

			return m.runPending(pending, msg.String())
		}

		switch msg.String() {
		case "ctrl+c":
			// :qa 와 같은 경로다. 어느 tab 이든 저장하지 않은 변경이 있으면 확인창이 뜬다.
			return quitAll(m, m.editor)
		case "g":
			// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 그대로다.
			m.pending = "g"

			return m, nil
		case ":":
			return commandMode(m.editor)
		case "i":
			// 커서 앞에 넣는다. 커서는 그대로다.
			return insertMode(m.editor)
		case "a":
			// 커서 글자 뒤에 넣는다.
			// 줄 끝 다음 칸은 insert mode 에서만 갈 수 있어서 mode 를 먼저 바꾼다.
			next, cmd := insertMode(m.editor)
			buf.moveRight(m.textWidth())
			buf.scrollTo(m.textWidth(), m.textHeight())

			return next, cmd
		case "u":
			buf.applyUndo(m.textWidth())
			buf.clampToNormal(m.textWidth())
		case "ctrl+r":
			buf.applyRedo(m.textWidth())
			buf.clampToNormal(m.textWidth())
		case "up":
			buf.moveUp(1, m.textWidth())
			buf.clampToNormal(m.textWidth())
		case "down":
			buf.moveDown(1, m.textWidth())
			buf.clampToNormal(m.textWidth())
		case "left":
			buf.moveLeft(m.textWidth())
		case "right":
			buf.moveRight(m.textWidth())
			buf.clampToNormal(m.textWidth())
		default:
			return m, nil
		}

		buf.scrollTo(m.textWidth(), m.textHeight())

		return m, nil
	default:
		return m, nil
	}
}

// runPending 은 접두 키 뒤에 붙은 키를 처리한다.
//
// 짝이 없는 조합은 vim 처럼 아무 일도 하지 않고 버린다.
// `esc` 와 `ctrl+c` 도 여기로 와서 버려진다. 잘못 누른 접두 키를 무르는 것이라
// vim 과 같고, `g` 뒤에 손이 미끄러져서 편집기가 꺼지는 일도 없다.
func (m viewEditorNormal) runPending(pending, key string) (tea.Model, tea.Cmd) {
	if pending != "g" {
		return m, nil
	}

	switch key {
	case "t":
		m.nextTab()
	case "T":
		m.prevTab()
	default:
		return m, nil
	}

	// 옮겨 간 tab 은 이 크기의 화면을 처음 볼 수도 있다.
	m.buffer().scrollTo(m.textWidth(), m.textHeight())

	return m, nil
}

func (m viewEditorNormal) View() tea.View {
	bottom := m.position()
	if m.message != "" {
		bottom = m.message
	}

	// 커서가 글자 위에 있으므로 블록이다.
	return m.render(tea.CursorBlock, "NORMAL", bottom)
}
