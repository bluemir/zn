package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// viewEditorContextMenu 는 편집 영역을 우클릭했을 때 커서 자리에 뜨는 컨텍스트 메뉴다(ADR-0159).
//
// **부모의 view 를 그대로 쓰고 내용만 갈아끼운다**(ADR-0110). 새 view 를 만들면 터미널
// 설정 넷을 손으로 베끼는 자리가 하나 더 생긴다.
//
// **고르는 것은 마우스뿐이고 esc 로 닫는다.** 트리 메뉴와 같은 손이다(view-sidebar-menu.go, ADR-0146).
type viewEditorContextMenu struct {
	*editor

	parent   tea.Model // 닫으면 돌아갈 화면. 우클릭 직전의 mode 그대로다
	left     int       // 상자의 화면 가로 좌표
	top      int       // 상자의 화면 세로 좌표
	hover    int       // 포인터가 얹힌 항목 자리 (-1 이면 없음)
	target   string    // 상자 첫 줄에 서는 대상 이름 (단어/심볼명, "선택 영역", 또는 "편집")
	hasWord  bool      // 커서 아래에 단어가 있는가
	hasLSP   bool      // 현재 파일에 언어 서버가 있는가
	isVisual bool      // visual mode (선택 영역) 인가
}

// contextMenuItem 은 메뉴 한 줄이다.
type contextMenuItem struct {
	label    string
	disabled bool
	run      func() (tea.Model, tea.Cmd)
}

// editorContextMenuMode 는 편집 영역을 우클릭했을 때 커서를 옮기고 컨텍스트 메뉴를 연다(ADR-0159).
func editorContextMenuMode(parent tea.Model, e *editor, x, y int) (tea.Model, tea.Cmd) {
	if !e.hasTab() {
		return parent, nil
	}

	buf := e.activeBuffer()
	if buf == nil {
		return parent, nil
	}

	// visual mode 에서 고른 범위가 있는 중이 아니면 커서를 누른 자리로 옮긴다.
	// 이미 고른 범위가 있으면 고른 영역에 대한 메뉴이므로 커서를 움직여 범위를 깨지 않는다.
	if !buf.Selection.Active {
		if _, isInsert := parent.(viewEditorInsert); isInsert {
			buf.EndEdit()
		}
		e.clickText(x, y)
		if _, isNormal := parent.(viewEditorNormal); isNormal {
			buf.ClampToNormal()
		}
	}

	word, _, hasWord := buf.WordUnderCursor()
	_, _, hasLSP := serverPath(buf.Path)
	isVisual := buf.Selection.Active

	target := "편집"
	if isVisual {
		target = "선택 영역"
	} else if hasWord {
		target = word
	}

	menu := viewEditorContextMenu{
		editor:   e,
		parent:   parent,
		hover:    -1,
		target:   target,
		hasWord:  hasWord,
		hasLSP:   hasLSP,
		isVisual: isVisual,
	}

	menu.left, menu.top = popupPos(x, y, menu.boxWidth(), len(menu.items())+menuFrame, e.width, e.height)

	return menu, nil
}

func (m viewEditorContextMenu) Init() tea.Cmd { return nil }

func (m viewEditorContextMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 폭이 바뀌면 본문 행도 상자 자리도 어긋난다. 메뉴는 누른 자리에 대한 것이라
		// 그 자리가 움직이면 남겨 둘 것이 없다.
		return m.parent, nil
	case tea.KeyPressMsg:
		// **`esc` 만 받는다.** 고르는 것은 마우스이고 키는 닫는 길 하나다.
		// `ctrl+c` 가 종료가 아니라 취소인 것은 지우기 확인창과 같다(ADR-0054).
		switch msg.String() {
		case "esc", "ctrl+c":
			return m.parent, nil
		default:
			return m, nil
		}
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m.parent, nil
		}

		// **바깥 클릭은 닫기만 한다.**
		at := m.itemAt(mouse.X, mouse.Y)
		if at < 0 {
			return m.parent, nil
		}

		item := m.items()[at]
		if item.disabled {
			return m, nil
		}

		return item.run()
	case tea.MouseMotionMsg:
		at := m.itemAt(msg.X, msg.Y)
		if at >= 0 && m.items()[at].disabled {
			at = -1
		}
		m.hover = at

		return m, nil
	case tea.MouseWheelMsg:
		// 굴리지 않는다.
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

// items 는 메뉴에 낼 항목이다.
func (m viewEditorContextMenu) items() []contextMenuItem {
	canLSP := m.hasWord && m.hasLSP

	if m.isVisual {
		return []contextMenuItem{
			{
				label: "선택 영역 복사",
				run:   m.copySelection,
			},
			{
				label:    "정의로 가기",
				disabled: !canLSP,
				run:      m.openDefinition,
			},
			{
				label:    "사용처로 가기",
				disabled: !canLSP,
				run:      m.openReferences,
			},
			{
				label:    "단어 검색",
				disabled: !m.hasWord,
				run:      m.searchWordUnderCursor,
			},
		}
	}

	return []contextMenuItem{
		{
			label:    "정의로 가기",
			disabled: !canLSP,
			run:      m.openDefinition,
		},
		{
			label:    "사용처로 가기",
			disabled: !canLSP,
			run:      m.openReferences,
		},
		{
			label:    "이름 바꾸기",
			disabled: !canLSP || m.editor.refuseReadOnly(),
			run:      m.openRename,
		},
		{
			label:    "단어 검색",
			disabled: !m.hasWord,
			run:      m.searchWordUnderCursor,
		},
		{
			label:    "붙여넣기",
			disabled: m.editor.refuseReadOnly(),
			run:      m.openPaste,
		},
	}
}

func (m viewEditorContextMenu) copySelection() (tea.Model, tea.Cmd) {
	return actionVisualYank{reg: clipboardRegister}.run(m.editor)
}

func (m viewEditorContextMenu) openDefinition() (tea.Model, tea.Cmd) {
	model, cmd := gotoDefinition(m.parent, m.editor)
	if model != nil {
		return model, cmd
	}

	return m.parent, cmd
}

func (m viewEditorContextMenu) openReferences() (tea.Model, tea.Cmd) {
	model, cmd := gotoReferences(m.parent, m.editor)
	if model != nil {
		return model, cmd
	}

	return m.parent, cmd
}

func (m viewEditorContextMenu) openRename() (tea.Model, tea.Cmd) {
	return renameInputMode(m.editor)
}

func (m viewEditorContextMenu) searchWordUnderCursor() (tea.Model, tea.Cmd) {
	m.editor.searchWord(textarea.SearchForward, 1)

	return m.parent, nil
}

func (m viewEditorContextMenu) openPaste() (tea.Model, tea.Cmd) {
	if m.editor.refuseReadOnly() {
		return m.parent, nil
	}

	if m.editor.registers.byName("").Filled() {
		actionPasteAfter{}.run(m.editor)

		return m.parent, nil
	}

	return m.parent, m.editor.askClipboard(false, 1)
}

func (m viewEditorContextMenu) itemAt(x, y int) int {
	if x < m.left || x >= m.left+m.boxWidth() {
		return -1
	}

	at := y - m.top - menuHeader
	if at < 0 || at >= len(m.items()) {
		return -1
	}

	return at
}

func (m viewEditorContextMenu) title() string {
	if m.target != "" {
		return m.target
	}

	return "편집"
}

func (m viewEditorContextMenu) boxWidth() int {
	inner := textarea.WidthOf(m.title())
	for _, item := range m.items() {
		if w := textarea.WidthOf(item.label); w > inner {
			inner = w
		}
	}

	wanted := inner + 4
	if m.width < wanted {
		return max(m.width, 4)
	}

	return wanted
}

func (m viewEditorContextMenu) renderBox() string {
	width := m.boxWidth()
	inner := width - 4 // 좌우 테두리 2 + 좌우 1칸 공백
	chars := m.boxChars
	edge := strings.Repeat(chars.horizontal, width-2)

	row := func(text string, hover, disabled bool) string {
		body := " " + padTo(text, inner) + " "
		if disabled {
			body = styleMenuDisabled.Render(body)
		} else if hover {
			body = styleMenuHover.Render(body)
		}

		return chars.vertical + body + chars.vertical
	}

	rows := []string{
		chars.topLeft + edge + chars.topRight,
		row(trimLeftToWidth(m.title(), inner), false, false),
		chars.leftTee + edge + chars.rightTee,
	}

	for at, item := range m.items() {
		rows = append(rows, row(truncateToWidth(item.label, inner), at == m.hover && !item.disabled, item.disabled))
	}

	return strings.Join(append(rows, chars.bottomLeft+edge+chars.bottomRight), "\n")
}

func (m viewEditorContextMenu) View() tea.View {
	view := m.parent.View()
	view.MouseMode = tea.MouseModeAllMotion

	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderBox()).X(m.left).Y(m.top).Z(1),
	).Render()

	return view
}
