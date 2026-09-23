package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/textarea"
)

// helpSearchPrompt 는 찾는 글을 받는 동안 statusBar 아래 줄 앞에 서는 말이다.
// 편집 영역의 `/` 와 같은 글자다.
const helpSearchPrompt = "/"

// viewHelpSearch 는 도움말에서 찾을 글을 받는 화면이다. `:help` 에서 `/` 로 들어온다(ADR-0149).
//
// **키가 동작이 아니라 글자다.** 그래서 판에 상태를 하나 더하는 대신 화면을 따로 뒀다
// (ADR-0002). 판의 키는 두벌식 자리의 영문 키로 되돌려 받는데(ADR-0008) 찾을 글은 되돌리면
// 안 된다 — 「트리」를 치면 `xmfl` 가 된다.
//
// **`:tips` 의 `/` 와 하는 일이 다르다.** 저쪽은 목록을 좁히는 거르기이고 이쪽은 자리로
// 뛰는 찾기다. 같은 키가 갈리는 것은 든 것이 갈려서다 — 목록은 줄이 항목이라 골라낼 수
// 있고, 도움말은 이어진 글이라 골라내면 앞뒤가 끊긴다.
//
// **치는 대로 움직인다.** 편집 영역의 `/` 와 같다(ADR-0010). 몇 글자에서 멈출지를 보면서
// 정하려면 그 결과가 보여야 한다.
type viewHelpSearch struct {
	// help 는 돌아갈 판이다.
	help viewHelp

	input inputLine

	// origin 은 들어올 때의 자리다. `esc` 가 여기로 되돌린다.
	//
	// 치는 대로 커서를 옮기므로, 되돌릴 자리를 들고 있지 않으면 찾다 그만둔 손이 엉뚱한
	// 데에 남는다. 편집 영역의 `/` 와 같은 규칙이다.
	origin scheme.Cursor
}

// helpSearchMode 는 찾는 글을 받는 화면으로 간다. 앞서 찾던 글이 채워진 채로 뜬다.
func helpSearchMode(help viewHelp) (tea.Model, tea.Cmd) {
	return viewHelpSearch{
		help:   help,
		input:  newInputLine(help.query),
		origin: help.buf.Cursor,
	}, nil
}

func (m viewHelpSearch) Init() tea.Cmd { return nil }

func (m viewHelpSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.resize(msg)
		m.help.layout()

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.help.editor)
		case "esc":
			// 치던 것을 버리고 들어오기 전 자리로 돌아간다. 앞서 찾던 글과 그 색은 그대로다.
			m.help.buf.Cursor = m.origin
			m.help.buf.ScrollTo(m.help.listHeight())

			return m.help, nil
		case "enter":
			return m.accept()
		case "left", "right", "home", "end":
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()

			return m.preview()
		case "backspace":
			// 명령줄과 같다. 다 지우면 찾기를 그만두고 들어오기 전 자리로 돌아간다.
			if m.input.empty() {
				m.help.buf.Cursor = m.origin
				m.help.buf.ScrollTo(m.help.listHeight())

				return m.help, nil
			}
			m.input.deleteBackward()

			return m.preview()
		default:
			// 글자가 될 수 없는 키(방향키, ctrl 조합) 는 Text 가 비어 있다.
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)

			return m.preview()
		}
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		next, cmd := m.help.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// preview 는 지금 치고 있는 글로 찾아 본 화면이다. 치는 동안 이것이 보인다.
//
// **찾지 못해도 알리지 않는다.** 치는 도중에는 아직 반쯤 친 글이라, 한 글자마다 「찾지
// 못했습니다」가 뜨면 아래 줄이 깜빡인다. 편집 영역의 `/` 와 같다.
func (m viewHelpSearch) preview() (tea.Model, tea.Cmd) {
	pattern, err := parseSearchPattern(m.input.text)
	if err != nil || m.input.text == "" {
		// 잘못된 패턴은 아직 다 치지 않은 것일 수 있다(`[` 하나). 색만 걷고 자리는 둔다.
		m.help.pattern, m.help.query = nil, ""

		return m, nil
	}

	m.help.pattern, m.help.query = pattern, m.input.text

	// **들어온 자리에서 다시 찾는다.** 지금 커서에서 찾으면 한 글자 칠 때마다 앞으로만
	// 밀려서, 지웠다 다시 치면 엉뚱하게 멀리 가 있다.
	if found, ok := m.help.buf.Find(pattern, textarea.SearchForward, m.origin); ok {
		m.help.buf.Cursor = found.Cursor
		m.help.buf.UpdateDesiredCol()
		m.help.buf.ScrollTo(m.help.listHeight())
	}

	return m, nil
}

// accept 는 친 글로 찾은 자리에 선다.
//
// 찾지 못했으면 들어온 자리로 돌아가고 알린다. 여기서는 알리는 것이 맞다 — 손이 다 치고
// `enter` 를 누른 것이라, 아무 일도 없으면 키가 먹지 않은 것처럼 보인다.
func (m viewHelpSearch) accept() (tea.Model, tea.Cmd) {
	if m.input.text == "" {
		m.help.buf.Cursor = m.origin

		return m.help, nil
	}

	pattern, err := parseSearchPattern(m.input.text)
	if err != nil {
		m.help.buf.Cursor = m.origin
		m.help.notifyError(err)

		return m.help, nil
	}

	m.help.pattern, m.help.query = pattern, m.input.text

	found, ok := m.help.buf.Find(pattern, textarea.SearchForward, m.origin)
	if !ok {
		m.help.buf.Cursor = m.origin
		m.help.buf.ScrollTo(m.help.listHeight())
		m.help.notify(m.input.text + " 를 찾지 못했습니다")

		return m.help, nil
	}

	m.help.buf.Cursor = found.Cursor
	m.help.buf.UpdateDesiredCol()
	m.help.buf.ScrollTo(m.help.listHeight())

	return m.help, nil
}

func (m viewHelpSearch) View() tea.View {
	// 커서는 치고 있는 자리다. 아래 줄은 화면 끝에서 끝까지라(트리를 덮는 판이다)
	// 편집 영역의 명령줄과 달리 왼쪽으로 밀 것이 없다.
	cursor := tea.NewCursor(textarea.WidthOf(helpSearchPrompt)+m.input.screenCursor(), m.help.height-1)
	cursor.Shape = tea.CursorBar

	return m.help.viewWith(helpSearchPrompt+m.input.text, cursor)
}
