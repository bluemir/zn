package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// viewGrepInput 은 검색어를 받는 박스다. 팔레트의 「프로젝트 검색」과 인자 없는 `:grep` 이
// 둘 다 여기로 온다.
//
// **팔레트가 서던 자리에 같은 폭으로 뜬다.** 박스를 보고 있다가 고른 것이니 박스로 받는 것이
// 맥락이 이어진다 — 아래 줄로 떨어지면 눈이 화면 끝까지 내려갔다 와야 하고, 방금 무엇을
// 골랐는지가 화면에서 사라진다(ADR-0011, ADR-0078 §7).
//
// 처음에는 `/` 검색창처럼 아래 줄에서 받았고, 팔레트에서 올 때만 박스로 갈랐다. **한 일에
// 입력창이 둘이 되니 갈라 둘 값이 없었다** — 어느 길로 왔는지는 친 사람이 알고, 무엇을 치는지는
// 같다. 아래 줄 쪽을 걷어서 하나로 모았다(ADR-0078 §7).
//
// 하는 일(검색을 시작하고 판을 연다) 은 `runGrep` 하나다. `:grep <패턴>` 도 그것으로 간다.
func grepInputMode(e *editor) (tea.Model, tea.Cmd) {
	if !e.grepInputFits() {
		return normalModeMessage(e, "화면이 좁아 검색창을 열 수 없습니다")
	}

	return viewGrepInput{editor: e}, nil
}

// grepInputFits 는 박스를 그릴 자리가 있는지다.
//
// **팔레트의 문턱을 그대로 쓰지 않는다.** `paletteFits` 는 목록 열 줄이 들어가는지를 재는데
// 이 박스에는 목록이 없다 — 그것으로 재면 낮은 화면에서 그릴 수 있는데도 거절한다.
// 폭은 팔레트와 같은 자를 쓴다(같은 자리에 같은 폭으로 떠야 한다).
func (e editor) grepInputFits() bool {
	return e.paletteWidth() >= paletteMinWidth && e.textAndDrawerHeight() >= paletteTop+grepInputRows
}

// grepInputRows 는 박스가 쓰는 행 수다. 테두리 둘과 입력줄 하나다.
const grepInputRows = 3

type viewGrepInput struct {
	*editor

	input inputLine
}

func (m viewGrepInput) Init() tea.Cmd { return nil }

func (m viewGrepInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return normalMode(m.editor)
		case "enter":
			return m.run()
		case "left", "right", "home", "end":
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()

			return m, nil
		case "backspace":
			// 팔레트·명령줄과 같이 다 지우면 나간다.
			if m.input.empty() {
				return normalMode(m.editor)
			}
			m.input.deleteBackward()

			return m, nil
		default:
			// Text 는 출력 가능한 글자에만 찬다. modifier 조합은 비어 있고 특수 키는
			// 위에서 잡힌다.
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 치는 동안에도 화면은 둘러볼 수 있다. 팔레트와 같다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// run 은 친 패턴으로 검색을 시작하고 결과 판으로 넘어간다.
func (m viewGrepInput) run() (tea.Model, tea.Cmd) {
	if m.input.empty() {
		return normalModeMessage(m.editor, "검색할 패턴이 없습니다")
	}

	return runGrep(m.editor, m.input.text)
}

// grepInputTitle 은 박스 위 테두리에 얹는 말이다. 무엇을 치는 자리인지가 이것으로 보인다 —
// 팔레트에서 고른 이름과 같은 말이라 이어서 읽힌다.
const grepInputTitle = " 프로젝트 검색 "

func (m viewGrepInput) View() tea.View {
	view := m.editorView(tea.CursorBar, "GREP",
		m.renderWithShowcmd(m.noticeOr("enter 로 찾고 esc 로 그만둡니다"), ""))

	// 팔레트와 같은 자리·같은 폭이다. 고른 박스가 있던 곳에 그대로 뜬다.
	left := m.paletteLeft()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderBox()).X(left).Y(paletteTop).Z(1),
	).Render()

	// 커서는 편집 내용이 아니라 박스 안 치는 자리에 있어야 한다. 팔레트와 같은 셈이다.
	_, cursor := m.inputText()
	view.Cursor = tea.NewCursor(left+2+cursor, paletteTop+1)
	view.Cursor.Shape = tea.CursorBar

	return view
}

// renderBox 는 박스다. 테두리 둘과 입력줄 하나뿐이라 팔레트보다 두 줄 낮다.
func (m viewGrepInput) renderBox() string {
	width := m.paletteWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	side := chars.vertical

	text, _ := m.inputText()

	return strings.Join([]string{
		m.renderTitleLine(width),
		side + " " + padTo(text, inner) + " " + side,
		chars.bottomLeft + strings.Repeat(chars.horizontal, width-2) + chars.bottomRight,
	}, "\n")
}

// renderTitleLine 은 위 테두리에 제목을 얹은 줄이다.
//
// 제목이 폭에 안 들어가면 테두리만 그린다 — 반쯤 잘린 제목은 제목으로 읽히지 않는다.
// 빈 화면의 로고를 자르지 않고 버리는 것과 같은 태도다(ADR-0061).
func (m viewGrepInput) renderTitleLine(width int) string {
	chars := m.boxChars

	room := width - 2
	if widthOf(grepInputTitle) > room {
		return chars.topLeft + strings.Repeat(chars.horizontal, room) + chars.topRight
	}

	rest := room - widthOf(grepInputTitle)

	return chars.topLeft + grepInputTitle + strings.Repeat(chars.horizontal, rest) + chars.topRight
}

// inputText 는 입력줄에 그릴 글과 그 안에서 커서가 설 칸이다.
//
// **넘치면 커서가 보이도록 왼쪽부터 접는다.** 접는 셈은 visible 이 든다 — 이름 바꾸기 창이
// 같은 자리에서 같은 답을 쓴다(input-line.go, view-rename-input.go).
//
// 비어 있으면 빈 줄이다. 무엇을 치면 되는지는 위 테두리의 제목과 아래 줄의 안내가 이미
// 말한다 — 박스가 한 줄뿐이라 안내를 그 안에 넣으면 치는 자리와 겹친다.
//
// **커서 자리를 재는 데도 쓰므로 한 자리에서 만든다** — 그리는 글과 재는 글이 갈리면 커서가
// 글자 뒤에 서지 않는다.
func (m viewGrepInput) inputText() (text string, cursorCol int) {
	// 커서 한 칸을 남긴다. 안 남기면 마지막 글자를 친 순간 커서가 테두리 위에 선다.
	room := m.paletteWidth() - 4 - 1

	return m.input.visible("", max(room, 1))
}

// runGrepInput 은 팔레트의 「프로젝트 검색」이다.
func runGrepInput(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return grepInputMode(e)
}
