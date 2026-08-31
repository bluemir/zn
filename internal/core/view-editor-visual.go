package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorVisual 은 visual mode 다. 커서는 normal 과 같이 글자 위에 있고,
// 반대쪽 끝(anchor) 은 Buffer 가 든다(selection.go).
//
// 여는 것은 startSelection 이고 이것은 화면만 만든다. 갈래를 바꾸는 `V` 가 이미 열린 범위를
// 그대로 두고 다시 들어와야 해서 둘이 나뉘어 있다.
func visualMode(e *editor) (tea.Model, tea.Cmd) {
	return viewEditorVisual{editor: e}, nil
}

type viewEditorVisual struct {
	*editor

	// state 는 키 나열을 동작 하나로 만드는 상태다. 숫자 접두와 `g` 가 여기 산다.
	// mode 안에서만 사는 상태라 editor 가 아니라 여기에 둔다(ADR-0002).
	state visualState
}

// keyState 는 지금 키 상태다. zero value(nil) 는 아무것도 먹지 않은 처음이다.
func (m viewEditorVisual) keyState() visualState {
	if m.state == nil {
		return visualStart{}
	}

	return m.state
}

func (m viewEditorVisual) Init() tea.Cmd { return nil }

func (m viewEditorVisual) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		// 한글 되돌림은 파서가 한다. 여기는 키를 그대로 넘긴다(ADR-0008).
		return m.press(msg.String())
	case tea.MouseClickMsg:
		switch mouse := msg.Mouse(); mouse.Button {
		case tea.MouseLeft:
			return m.click(mouse)
		case tea.MouseRight:
			// tabline 의 tab 을 닫는다. 보고 있던 tab 이었으면 normal 로 나간다(ADR-0060).
			return rightClick(m, m.editor, mouse)
		}

		return m, nil
	case tea.MouseMotionMsg:
		// 버튼을 누른 채 움직이는 중이다. 고른 범위가 커서를 따라 자란다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			m.dragTo(mouse.X, mouse.Y)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		// 바깥 변경 알림(ResumeMsg·FocusMsg) 은 받지 않는다. normal·insert·트리만 본다(ADR-0031).
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 차례로 실행한다.
// viewEditorNormal.press 와 같은 고리다 — 도중에 mode 가 바뀌면 남은 동작은 버린다(ADR-0008).
func (m viewEditorVisual) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	actions, state := m.keyState().press(key)
	m.state = state

	for _, act := range actions {
		next, cmd := act.run(m.editor)

		if next == nil && cmd == nil {
			continue
		}

		if next == nil {
			return m, cmd
		}

		return next, cmd
	}

	return m, nil
}

// modeName 은 statusBar 에 찍히는 이름이다. 갈래가 둘이라 줄 단위만 뒤에 붙인다.
func (m viewEditorVisual) modeName() string {
	if m.activeBuffer().selection.linewise {
		return "VISUAL LINE"
	}

	return "VISUAL"
}

func (m viewEditorVisual) View() tea.View {
	// 커서가 글자 위에 있으므로 normal 과 같이 블록이다.
	return m.editorView(tea.CursorBlock, m.modeName(),
		m.renderWithTip(m.noticeOr(m.renderPosition()), m.keyState().showcmd()))
}
