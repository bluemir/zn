package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	// sidebarInputInner 는 이름 상자 안쪽 폭이다. 이름 바꾸기 창과 같은 값이라 두 상자가
	// 같은 크기로 뜬다. 고정값인 것도 같은 까닭이다 — 글자를 칠 때마다 상자가 늘면 눈이
	// 그때마다 다시 읽는다.
	sidebarInputInner = 44

	// sidebarInputRows 는 상자 높이다. 테두리 둘과 치는 줄 하나다.
	sidebarInputRows = 3
)

// viewSidebarInput 은 메뉴에서 이름을 받는 상자다(ADR-0146).
//
// **메뉴가 섰던 자리에서 받는다.** `mc`·`mm` 은 statusBar 아래 줄에서 받는데, 마우스로
// 시작한 일이 화면 반대편 끝에서 이어지면 눈이 그리로 내려갔다 와야 한다 — 이름 바꾸기 창이
// 커서 옆에서 받는 것과 같은 까닭이다(ADR-0066, ADR-0067).
//
// 새 파일과 새 이름이 같은 타입을 쓴다. 다른 것은 「무엇을 적어 두고 시작하는가」와 「`enter`
// 에 무엇을 하는가」 둘뿐이라, 그것을 flag 로 되묻는 대신 들어올 때 정해서 들고 온다 —
// 종료 확인창이 `confirm` 을 들고 오는 것과 같은 손이다(view-quit-confirm.go).
type viewSidebarInput struct {
	*editor

	parent tea.Model // 그만두면 돌아갈 화면. 메뉴가 아니라 메뉴를 열기 전 화면이다
	prompt string    // 치는 줄 앞에 서는 말. `새 파일` 또는 `새 이름`
	input  inputLine
	commit func(name string) (tea.Model, tea.Cmd)

	left, top int // 상자의 화면 좌표
}

// sidebarInputMode 는 메뉴가 섰던 자리에 이름 상자를 연다.
//
// 메뉴를 통째로 받는 것은 자리와 돌아갈 화면과 editor 가 다 거기 있어서다. 다섯을 따로
// 넘기면 부르는 두 곳이 같은 다섯 줄을 쓴다.
func sidebarInputMode(menu viewSidebarMenu, prompt, text string, commit func(name string) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	input := viewSidebarInput{
		editor: menu.editor,
		parent: menu.parent,
		prompt: prompt,
		input:  newInputLine(text),
		commit: commit,
	}
	input.place(menu.left, menu.top)

	return input, nil
}

// place 는 상자를 화면 안에 가둔다. 한 칸이라도 나가면 캔버스가 그만큼 커져 화면이 밀린다(ADR-0011).
//
// statusBar 두 줄은 덮지 않는다. 저장 문구와 알림이 거기 뜬다.
func (m *viewSidebarInput) place(left, top int) {
	m.left = max(0, min(left, m.width-m.inner()-2))
	m.top = max(0, min(top, m.height-statusBarHeight-sidebarInputRows))
}

// inner 는 상자 안쪽 폭이다. 화면이 좁으면 그만큼 줄인다.
func (m viewSidebarInput) inner() int {
	return min(sidebarInputInner, max(m.width-2, 1))
}

func (m viewSidebarInput) Init() tea.Cmd { return nil }

func (m viewSidebarInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져 트리가 숨으면 만들 자리가 화면에서 사라진다. 트리에 기대는 mode 는
		// 모두 같은 규칙이다(view-sidebar-create.go).
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}
		m.place(m.left, m.top)

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			// 메뉴가 아니라 메뉴를 열기 전 화면으로 돌아간다. 한 단계씩 되짚어 나가면
			// 그만두려고 `esc` 를 두 번 눌러야 한다.
			return m.parent, nil
		case "enter":
			return m.commit(strings.TrimSpace(m.input.text))
		case "left", "right", "home", "end":
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()

			return m, nil
		case "backspace":
			// 다 지워도 이 상자에 남는다. 그만두는 것은 `esc` 다 — 새 이름은 채워진 채로
			// 시작하므로 「다 지웠다」를 그만두려는 뜻으로 읽을 수 없고, 두 상자가 같은
			// 자리에서 다르게 굴면 어느 쪽인지 기억해야 한다(view-sidebar-rename.go).
			m.input.deleteBackward()

			return m, nil
		default:
			// 이름이 될 수 없는 키(방향키, ctrl 조합) 는 Text 가 비어 있다.
			//
			// 한글 되돌리기를 지나지 않는다. 여기서 받는 것은 동작이 아니라 글자다 —
			// 되돌리면 `한글.txt` 가 `gksrmf.txt` 가 된다(ADR-0008).
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)

			return m, nil
		}
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		// 클릭도 휠도 받지 않는다. 치던 이름이 클릭 한 번에 조용히 사라지면 안 되고,
		// 굴려도 상자에 가려 아래를 볼 수 없다. 명령줄과 같은 규칙이다.
		return m, nil
	}
}

// line 은 상자 안의 한 줄과 커서가 설 칸이다.
//
// **커서 자리를 재는 데도 쓰므로 한 자리에서 만든다** — 그리는 글과 재는 글이 갈리면 커서가
// 친 글자 뒤에 서지 않는다. 넘치면 왼쪽부터 접는 것과 커서 한 칸을 남기는 것도 이름 바꾸기
// 창과 같다(view-rename-input.go).
func (m viewSidebarInput) line() (text string, cursorCol int) {
	return m.input.visible(" "+m.prompt+": ", m.inner()-1)
}

func (m viewSidebarInput) renderBox() string {
	chars := m.boxChars
	inner := m.inner()
	edge := strings.Repeat(chars.horizontal, inner)
	text, _ := m.line()

	return strings.Join([]string{
		chars.topLeft + edge + chars.topRight,
		chars.vertical + padTo(text, inner) + chars.vertical,
		chars.bottomLeft + edge + chars.bottomRight,
	}, "\n")
}

func (m viewSidebarInput) View() tea.View {
	// 부모의 view 를 그대로 쓰고 내용만 갈아끼운다(ADR-0110).
	view := m.parent.View()

	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderBox()).X(m.left).Y(m.top).Z(1),
	).Render()

	// 커서는 치는 자리에 선다. 치고 있는 것이 어디로 들어가는지가 커서로 보여야 한다.
	_, cursor := m.line()

	view.Cursor = tea.NewCursor(min(m.left+1+cursor, m.width-1), m.top+1)
	view.Cursor.Shape = tea.CursorBar

	return view
}
