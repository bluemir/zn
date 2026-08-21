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

		return m, nil
	case tea.KeyPressMsg:
		// 알림은 다음 키를 누르면 사라진다. normal 과 같다.
		m.message = ""

		buf := m.activeBuffer()

		switch msg.String() {
		case "ctrl+c":
			// :qa 와 같은 경로다. 어느 tab 이든 저장하지 않은 변경이 있으면 확인창이 뜬다.
			return quitAll(m, m.editor)
		case "esc":
			// insert mode 의 커서는 글자 사이에 있다. normal 로 돌아오면 왼쪽 글자 위에 선다.
			// vim 과 같은 동작이라 a<Esc> 는 제자리로 돌아오고 i<Esc> 는 한 글자 왼쪽이 된다.
			// 줄 끝 다음 칸에서 돌아오는 경우도 이 한 번의 이동으로 같이 처리된다.
			buf.endEdit()
			buf.moveLeft(1, m.contentWidth())
			m.scrollToCursor()

			return normalMode(m.editor)
		case "enter":
			buf.insert([]byte("\n"), m.contentWidth())
		case "backspace":
			buf.deleteBackward(m.contentWidth())
		case "tab":
			buf.insert([]byte("\t"), m.contentWidth())
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
			buf.insert([]byte(msg.Text), m.contentWidth())
		}

		m.scrollToCursor()

		return m, nil
	case tea.MouseClickMsg:
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			return m.click(mouse)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go). 파일 검사 tick 은
		// 여기서 보지 않고 주기만 이어 간다 — 보는 것은 normal·트리다(ADR-0038).
		return m, m.handleJob(msg)
	default:
		return m, nil
	}
}

func (m viewEditorInsert) View() tea.View {
	// 커서가 글자 사이에 있으므로 막대다.
	return m.editorView(tea.CursorBar, "INSERT", m.messageOr(m.renderPosition()))
}
