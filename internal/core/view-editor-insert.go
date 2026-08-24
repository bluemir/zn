package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorInsert 는 insert mode 다. 커서가 글자 사이에 있어서 줄 끝 다음 칸까지 갈 수 있다.
func insertMode(e *editor) (tea.Model, tea.Cmd) {
	// visual 에서 `c` 로 들어오는 길이 있다. 고른 범위를 놓는 문이 여기와 normalMode 둘뿐이다.
	e.activeBuffer().selection = selection{}

	return viewEditorInsert{editor: e}, nil
}

type viewEditorInsert struct {
	*editor
}

func (m viewEditorInsert) Init() tea.Cmd { return nil }

func (m viewEditorInsert) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.FocusMsg:
		// 치던 중에 다른 창을 만지고 돌아오는 것이 흔하다. 여기서 알려야 남의 변경 위에
		// 계속 치다가 저장할 때에야 막히는 일이 준다 (ADR-0015, ADR-0031).
		//
		// 치기 시작했으면 `dirty` 라서 마커만 붙는다. 아직 아무것도 치지 않았으면 잃을 것이
		// 없으므로 가져온다 — 검사가 작업으로 내려가며 normal 과 같은 길이 되었다(ADR-0044).
		return m, m.startOutsideCheck()
	case tea.PasteMsg:
		// 붙여넣기는 여러 줄일 수 있다. insert 가 줄바꿈을 알아서 가른다.
		buf := m.activeBuffer()
		buf.insert([]byte(msg.Content), m.contentWidth())
		m.scrollToCursor()

		return m, m.scheduleLspTick()
	case tea.KeyPressMsg:
		// 알림은 다음 키를 누르면 사라진다. normal 과 같다.
		m.clearNotice()

		buf := m.activeBuffer()

		// 목록이 떠 있는 동안에만 몇 키가 목록의 것이 된다. 떠 있지 않으면 이 자리는 없는 것과
		// 같다 — insert 의 키는 늘 글자였고, 그 규칙이 목록이 없을 때 그대로 남는다(ADR-0066).
		if m.completionOpen() {
			if next, cmd, taken := m.completionKey(msg.String()); taken {
				return next, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c":
			// :qa 와 같은 경로다. 어느 tab 이든 저장하지 않은 변경이 있으면 확인창이 뜬다.
			return quitAll(m, m.editor)
		case "ctrl+p":
			// 팔레트를 insert 에서도 연다. 글을 쓰다 기호가 필요한 순간이 잦아서
			// esc 를 한 번 거치게 하지 않는다(ADR-0011 을 여기서 고쳤다, ADR-0056).
			//
			// 팔레트는 어디서 열렸는지 기억하지 않으므로 커서를 여기서 normal 자리로
			// 맞춰 놓고 넘긴다 — `esc` 가 하는 것과 같다. 특수문자 drawer 가 `a` 처럼
			// 한 칸 오른쪽으로 되돌리므로 치던 자리로 돌아온다.
			//
			// **줄 맨 앞(0 칸) 에서만 한 칸 오른쪽에서 시작한다.** 왼쪽으로 갈 자리가 없어
			// moveLeft 가 아무 일도 안 하는데 drawer 는 그대로 한 칸 나아가기 때문이다.
			// `i<Esc>a` 가 같은 자리에서 한 칸 움직이는 것과 같고, 되돌리려면 어디서
			// 열렸는지 기억해야 해서 두었다(ADR-0056).
			buf.endEdit()
			buf.moveLeft(1, m.contentWidth())
			m.scrollToCursor()

			return paletteMode(m.editor)
		case "esc":
			// insert mode 의 커서는 글자 사이에 있다. normal 로 돌아오면 왼쪽 글자 위에 선다.
			// vim 과 같은 동작이라 a<Esc> 는 제자리로 돌아오고 i<Esc> 는 한 글자 왼쪽이 된다.
			// 줄 끝 다음 칸에서 돌아오는 경우도 이 한 번의 이동으로 같이 처리된다.
			buf.endEdit()
			buf.moveLeft(1, m.contentWidth())
			m.scrollToCursor()

			return normalMode(m.editor)
		case "enter":
			// 낱말로 블록을 닫는 언어(shell 의 `fi`) 는 줄을 떠나는 이 순간에 낱말이 끝난다.
			buf.reindentClosing([]byte{'\n'}, m.contentWidth())
			buf.insertNewLine(m.contentWidth())
		case "backspace":
			// 커서 앞이 들여쓰기뿐이면 한 칸이 아니라 앞 단위 경계까지 지운다(indent.go).
			if !buf.deleteIndentBackward(m.contentWidth()) {
				buf.deleteBackward(m.contentWidth())
			}
		case "ctrl+space":
			// 손으로 부르는 자리다. 글자를 넣지 않고 묻기만 한다 — VS Code 와 같은 키다.
			// 이 키는 터미널에서 NUL 로 와서 `msg.Text` 가 비어 있고, 아래 default 가
			// 그것을 걸러 내므로 여기서 따로 받는다.
			return m, m.startCompletion()
		case "tab":
			buf.insertIndent(m.contentWidth())
		case "shift+tab":
			buf.outdentLine(m.contentWidth())
		case "up", "down", "left", "right":
			// 커서를 옮기면 undo 구간이 끊긴다. vim 과 같다.
			buf.endEdit()

			switch msg.String() {
			case "up":
				buf.moveUp(1, m.contentWidth())
			case "down":
				buf.moveDown(1, m.contentWidth())
			case "left":
				buf.moveLeft(1, m.contentWidth())
			case "right":
				buf.moveRight(1, m.contentWidth())
			}
		default:
			// Text 는 출력 가능한 문자에만 채워진다. Enter·Tab 같은 특수 키와
			// modifier 조합에서는 비어 있어서 따로 걸러낼 필요가 없다.
			if msg.Text == "" {
				return m, nil
			}

			// 줄 앞이 닫는 표시가 되면 그 줄이 한 단계 당겨진다. 넣기 전에 자리를 잡아야
			// 커서가 옮겨진 자리에서 글자를 받는다(indent.go).
			buf.reindentClosing([]byte(msg.Text), m.contentWidth())
			buf.insert([]byte(msg.Text), m.contentWidth())
		}

		m.scrollToCursor()

		// 고친 것을 언어 서버에도 알려야 한다. 여기서 보내지 않고 예약만 한다 — 250ms
		// 조용해지면 그때 한 번 간다(ADR-0051).
		return m, tea.Batch(m.scheduleLspTick(), m.completionAfter(msg))
	case tea.MouseClickMsg:
		switch mouse := msg.Mouse(); mouse.Button {
		case tea.MouseLeft:
			return m.click(mouse)
		case tea.MouseRight:
			// tabline 의 tab 을 닫는다. 보고 있던 tab 이었으면 normal 로 나간다(ADR-0060).
			return rightClick(m, m.editor, mouse)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case completionMsg:
		// 답은 insert 만 받는다. 다른 mode 로 옮겨 간 뒤에 온 답은 아무도 받지 않고 버려지는데,
		// 그때 기다리는 표시를 내리는 것은 closeCompletion 이 한다(completion.go).
		return m, m.finishCompletion(msg)
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go). 파일 검사 tick 은
		// 여기서 보지 않고 주기만 이어 간다 — 보는 것은 normal·트리다(ADR-0038).
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

// completionKey 는 목록이 떠 있는 동안 목록이 가로채는 키다.
//
// 가로챈 키는 taken 이 참이고 거기서 끝난다. 커서를 옆으로 옮기거나 mode 를 떠나는 키는
// 목록만 닫고 **거짓을 준다** — 닫는 것과 원래 하던 일이 한 키에 같이 일어나야 한다.
//
// `esc` 는 목록만 닫는다. insert 에서 나가려면 한 번 더 눌러야 한다 — VS Code 도 vim 도 같다.
// `enter`·`tab` 이 넣기인 것도 VS Code 를 따른 것이다.
func (m viewEditorInsert) completionKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "up":
		m.moveCompletion(-1)

		return m, nil, true
	case "down":
		m.moveCompletion(1)

		return m, nil, true
	case "enter", "tab":
		m.applyCompletion()

		return m, nil, true
	case "esc":
		m.closeCompletion()

		return m, nil, true
	case "left", "right", "ctrl+c", "ctrl+p":
		m.closeCompletion()
	}

	return m, nil, false
}

// completionAfter 는 키 하나를 처리하고 나서 목록을 어떻게 할지다.
//
// **부를 만한 글자가 아니면 닫는다.** 괄호나 빈칸을 치면 그 자리의 후보는 이미 뜻이 없다.
// 지우기는 좁힌 것을 되돌리는 일이라, 목록이 떠 있었으면 다시 묻는다.
func (m viewEditorInsert) completionAfter(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case completionTriggers(msg.Text):
		return m.startCompletion()
	case msg.String() == "backspace" && m.completionOpen():
		return m.startCompletion()
	}

	m.closeCompletion()

	return nil
}

func (m viewEditorInsert) View() tea.View {
	// 커서가 글자 사이에 있으므로 막대다.
	//
	// showcmd 자리에 빈 문자열을 넘긴다. insert 에는 키 파서가 없어서 기다리는 접두 키라는 것이
	// 없다 — 그래서 이 mode 에서는 그 칸이 늘 tip 자리다(tip.go).
	view := m.editorView(tea.CursorBar, "INSERT", m.renderWithTip(m.noticeOr(m.renderPosition()), ""))

	return m.overlayCompletion(view)
}
