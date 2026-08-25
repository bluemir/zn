package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// renameBoxInner 는 이름 바꾸기 창 안쪽 폭이다.
//
// 고정값이다 — 글자를 칠 때마다 창이 늘면 눈이 그때마다 다시 읽는다. 식별자 둘이 들어가는
// 폭이라 웬만한 이름은 잘리지 않고, 넘치면 앞쪽(옛 이름) 부터 잘린다.
const renameBoxInner = 44

// renameInputMode 는 커서 자리에서 새 이름을 받는 창이다. `\rn`·팔레트·인자 없는 `:rename`
// 이 여기로 온다(ADR-0067).
//
// **커서 옆에서 받는다.** 무엇을 바꾸는지는 그 자리에 있는데 아래 줄에서 받으면 눈이 화면
// 끝까지 내려갔다 와야 한다 — 자동완성이 목록을 커서 옆에 띄우는 것과 같은 까닭이다(ADR-0066).
//
// **커서를 그 이름의 첫 글자로 옮긴다.** 서버에 묻는 자리가 커서라, 옮겨 두면 화면에 보이는
// 것과 묻는 자리가 같아진다. vim 의 `*` 가 그 줄 오른쪽의 첫 낱말을 잡아 주는 것처럼
// wordUnderCursor 도 커서가 낱말 위가 아니면 오른쪽에서 찾는다(search.go).
func renameInputMode(e *editor) (tea.Model, tea.Cmd) {
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	buf := e.activeBuffer()

	word, col, ok := buf.wordUnderCursor()
	if !ok {
		e.notify("바꿀 이름이 없습니다")

		return normalMode(e)
	}

	buf.moveTo(buf.cursorLine, col, e.contentWidth())
	e.scrollToCursor()

	return viewRenameInput{editor: e, old: word, input: word}, nil
}

// viewRenameInput 은 이름 바꾸기 창의 상태를 든다.
//
// input 이 옛 이름으로 채워져 있다. 한 글자만 고치거나 앞에 무언가를 붙이는 것이 이 동작의
// 거의 전부라, 빈 칸에서 시작하면 매번 다시 쳐야 한다 — 트리의 이름 고치기와 같은 자리다
// (view-sidebar-rename.go, ADR-0054).
type viewRenameInput struct {
	*editor

	old   string // 바꿀 이름. 창에 같이 적어 무엇을 바꾸는지 보인다
	input string
}

func (m viewRenameInput) Init() tea.Cmd { return nil }

func (m viewRenameInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return m.rename()
		case "backspace":
			// 다 지워도 이 창에 남는다. 채워져 있던 것을 지운 것이라 그만두려는 뜻으로 읽을
			// 수 없다 — 그만두는 것은 `esc` 다(view-sidebar-rename.go 와 같은 규칙이다).
			if m.input == "" {
				return m, nil
			}

			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]

			return m, nil
		default:
			// 이름이 될 수 없는 키(방향키, ctrl 조합) 는 Text 가 비어 있다.
			//
			// 한글 되돌리기(expandHangul) 를 지나지 않는다. 여기서 받는 것은 동작이 아니라
			// 글자다 — 이름에 한글을 쓰지는 않겠지만, 그 판정은 서버가 한다(ADR-0008).
			if msg.Text == "" {
				return m, nil
			}

			m.input += msg.Text

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 치는 동안에도 화면은 둘러볼 수 있다. 클릭은 받지 않는다 — 치던 것이 클릭 한 번에
		// 조용히 사라지면 안 된다. 명령줄과 같은 규칙이다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg:
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// rename 은 친 이름으로 바꾼다. 묻는 것은 startRename 이 하고 이 화면은 여기서 물러난다.
//
// 옛 이름 그대로면 아무 일도 하지 않는다. 서버는 그것도 「모든 자리를 같은 글자로 바꾸는」
// 편집으로 답하는데, 파일을 열어 다시 쓰기만 하고 달라지는 것이 없다.
func (m viewRenameInput) rename() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.input)

	switch {
	case name == "":
		m.notify("이름이 비어 있습니다")

		return normalMode(m.editor)
	case name == m.old:
		return normalMode(m.editor)
	}

	model, next := normalMode(m.editor)

	return model, tea.Batch(next, m.startRename(name))
}

func (m viewRenameInput) View() tea.View {
	view := m.editorView(tea.CursorBar, "RENAME",
		m.renderWithShowcmd(m.noticeOr("enter 로 바꾸고 esc 로 그만둡니다"), ""))

	buf := m.activeBuffer()

	x, y, ok := buf.cursorScreenPos(m.contentWidth(), m.textHeight())
	if !ok {
		return view
	}

	body := m.renderRenameBox()
	rows := strings.Split(body, "\n")

	left, top := popupPos(x+m.contentLeft(), y+tablineHeight, renameBoxInner+2, len(rows), m.width, m.height)

	next := tea.NewView(lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(body).X(left).Y(top).Z(1),
	).Render())

	// **커서는 치는 자리에 선다.** 창 안의 글자 뒤이지 본문이 아니다 — 치고 있는 것이
	// 어디로 들어가는지가 커서로 보여야 한다.
	//
	// 그리는 글과 같은 함수에서 칸을 받는다. 둘이 갈리면 커서가 글자 뒤에 서지 않는다.
	_, cursor := m.renameLine()

	next.Cursor = tea.NewCursor(min(left+1+cursor, m.width-1), top+1)
	next.Cursor.Shape = tea.CursorBar
	next.AltScreen = view.AltScreen
	next.MouseMode = view.MouseMode

	return next
}

// renderRenameBox 는 창이다. `옛 이름 → 치고 있는 이름` 한 줄이다.
func (m viewRenameInput) renderRenameBox() string {
	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, renameBoxInner)
	text, _ := m.renameLine()

	return strings.Join([]string{
		chars.topLeft + line + chars.topRight,
		chars.vertical + padTo(text, renameBoxInner) + chars.vertical,
		chars.bottomLeft + line + chars.bottomRight,
	}, "\n")
}

// renameLine 은 창 안의 한 줄과 커서가 설 칸이다.
//
// **커서 자리를 재는 데도 쓰므로 한 자리에서 만든다** — 그리는 글과 재는 글이 갈리면 커서가
// 글자 뒤에 서지 않는다.
//
// **넘치면 왼쪽부터 접는다.** 치고 있는 것은 뒤쪽이라 오른쪽부터 자르면 방금 친 글자가
// 보이지 않는다 — 긴 이름을 치면 실제로 그렇게 되었고, 창 밖으로 나간 커서가 화면 끝에서
// 다음 줄로 감겨 엉뚱한 자리에 섰다. `GOTO` 목록이 경로를 접는 것과 같은 규칙이다
// (render-status-bar.go 의 trimLeftToWidth).
//
// **커서 한 칸을 남긴다.** 안 남기면 마지막 글자를 친 순간 커서가 테두리 위에 선다.
//
// 색을 넣지 않는다. 옛 이름과 새 이름은 `→` 로 이미 갈리고, 색을 넣으면 이 줄이 폭을 재는
// 자리(padTo) 를 지날 때 escape 가 칸으로 세어진다.
func (m viewRenameInput) renameLine() (text string, cursorCol int) {
	text = trimLeftToWidth(" "+m.old+" → "+m.input, renameBoxInner-1)

	return text, screenWidthOf(text)
}
