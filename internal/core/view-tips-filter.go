package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// tipsFilterPrompt 는 거르는 글을 받는 동안 statusBar 아래 줄 앞에 서는 말이다.
// 편집 영역의 `/` 와 같은 글자라 무엇을 치고 있는지가 그 줄만 보고 갈린다.
const tipsFilterPrompt = "/"

// viewTipsFilter 는 tip 목록을 거를 글을 받는 화면이다. `:tips` 에서 `/` 로 들어온다(ADR-0147).
//
// **키가 동작이 아니라 글자다.** 그래서 목록에 상태를 하나 더하는 대신 화면을 따로 뒀다
// (ADR-0002). 목록의 키는 두벌식 자리의 영문 키로 되돌려 받는데(ADR-0008) 거를 글은 되돌리면
// 안 된다 — 「트리」를 치면 `xmfl` 가 된다. 여기는 명령줄과 같이 `msg.Text` 를 그대로 붙인다.
//
// **치는 동안 목록이 좁아진다.** 편집 영역의 `/` 가 치는 대로 화면을 옮기는 것과 같은 자리다
// (ADR-0010). 몇 글자에서 멈출지를 보면서 정하려면 그 결과가 보여야 한다.
type viewTipsFilter struct {
	// tips 는 돌아갈 목록이다. 고른 자리와 첫 행을 그대로 들고 있다가 되돌려준다.
	tips viewTips

	input inputLine
}

// tipsFilterMode 는 거르는 글을 받는 화면으로 간다. 지금 거르고 있던 글이 채워진 채로 뜬다.
//
// 빈 칸에서 시작하지 않는 것은 이름 바꾸기와 같은 까닭이다(ADR-0054). 한 글자만 더 치거나
// 지우는 것이 대부분이고, 비우고 시작하면 거른 것을 고칠 때마다 다시 쳐야 한다.
func tipsFilterMode(tips viewTips) (tea.Model, tea.Cmd) {
	return viewTipsFilter{tips: tips, input: newInputLine(tips.filter)}, nil
}

func (m viewTipsFilter) Init() tea.Cmd { return nil }

func (m viewTipsFilter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.tips.resize(msg)

		// 좁아진 화면에 맞춰 첫 행을 당긴다. 좁힌 목록으로 재고 그 결과만 들고 온다 —
		// 치던 글은 아직 목록의 것이 아니다.
		tips := m.narrowed()
		m.tips.selected, m.tips.top = tips.selected, tips.top

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.tips.editor)
		case "esc":
			// 치던 것을 버리고 들어오기 전의 목록으로 돌아간다. 거르던 글도 그대로다.
			return m.tips, nil
		case "enter":
			return m.accept()
		case "left", "right", "home", "end":
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()

			return m, nil
		case "backspace":
			// 명령줄과 같다. 다 지우면 거르던 것을 그만두고 전체 목록으로 돌아간다.
			if m.input.empty() {
				return m.accept()
			}
			m.input.deleteBackward()

			return m, nil
		default:
			// 글자가 될 수 없는 키(방향키, ctrl 조합) 는 Text 가 비어 있다.
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 치는 동안에도 좁아지는 목록을 훑을 수 있다. 클릭은 받지 않는다 —
		// 치던 것이 클릭 한 번에 조용히 사라지면 안 된다. 명령줄과 같은 규칙이다.
		tips := m.narrowed()
		switch msg.Button {
		case tea.MouseWheelUp:
			tips.move(-wheelRows)
		case tea.MouseWheelDown:
			tips.move(wheelRows)
		}
		m.tips.selected, m.tips.top = tips.selected, tips.top

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		next, cmd := m.tips.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// accept 는 친 글로 거른 목록에 선다.
func (m viewTipsFilter) accept() (tea.Model, tea.Cmd) {
	return m.narrowed(), nil
}

// narrowed 는 지금 치고 있는 글로 거른 목록이다.
//
// **그리는 쪽과 넘겨주는 쪽이 같은 것을 본다.** 치는 동안 보이는 목록과 `enter` 뒤에 서는
// 목록이 갈리면, 눈으로 고른 줄이 다른 문장으로 바뀐다.
//
// 고른 자리는 좁아진 목록 안으로 당겨진다. 걸러서 세 줄만 남았는데 서른째 줄을 고르고 있을
// 수 없다 — scrollTo 가 그 일을 한다.
func (m viewTipsFilter) narrowed() viewTips {
	tips := m.tips
	tips.filter = m.input.text
	tips.scrollTo()

	return tips
}

func (m viewTipsFilter) View() tea.View {
	tips := m.narrowed()

	// 커서는 치고 있는 자리다. 아래 줄은 화면 끝에서 끝까지라(트리를 덮는 판이다)
	// 편집 영역의 명령줄과 달리 왼쪽으로 밀 것이 없다.
	cursor := tea.NewCursor(textarea.WidthOf(tipsFilterPrompt)+m.input.screenCursor(), tips.height-1)
	cursor.Shape = tea.CursorBar

	return tips.viewWith(tipsFilterPrompt+m.input.text, cursor)
}
