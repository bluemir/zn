package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/lsp"

	"github.com/bluemir/zn/internal/textarea"
)

// serverInstallConfirmMode 는 언어 서버가 없을 때 설치 여부를 묻는 모달 화면이다.
// `\gd`·`\gr` 또는 팔레트의 「정의로 가기」·「사용처로 가기」에서 들어온다.
//
// **어느 서버를 묻는지 받는다.** 서버가 여럿이라 화면이 이름과 설치 명령을 그 줄에서
// 읽어야 한다 — 문구를 여기 적어 두면 서버를 더할 때 이 화면도 같이 손봐야 한다(ADR-0107).
func serverInstallConfirmMode(parent tea.Model, e *editor, server *lsp.Server) (tea.Model, tea.Cmd) {
	return viewServerInstallConfirm{editor: e, parent: parent, server: server}, nil
}

// viewServerInstallConfirm 은 언어 서버 설치 확인창의 상태를 든다.
type viewServerInstallConfirm struct {
	*editor

	parent tea.Model
	server *lsp.Server
	cursor int // 0: Yes, 1: No
}

func (m viewServerInstallConfirm) Init() tea.Cmd {
	return nil
}

func (m viewServerInstallConfirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			confirm, ok := model.(viewServerInstallConfirm)
			if !ok {
				return model, nil
			}

			next, cmd := confirm.press(key)
			if cmd != nil {
				return next, cmd
			}

			model = next
		}

		return model, nil
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

// press 는 키 하나를 처리한다.
func (m viewServerInstallConfirm) press(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return exit()
	case "left", "y":
		m.cursor = 0
		return m, nil
	case "right", "n":
		m.cursor = 1
		return m, nil
	case "esc":
		return m.parent, nil
	case "enter":
		if m.cursor == 0 {
			return m.parent, m.installServer(m.server)
		}

		return m.parent, nil
	default:
		return m, nil
	}
}

// lines 는 창에 적는 두 줄이다. 폭을 재는 자리와 그리는 자리가 같은 글을 보아야 한다.
func (m viewServerInstallConfirm) lines() (string, string) {
	return m.server.Name + " 가 설치되어 있지 않습니다.",
		"지금 설치하시겠습니까? (" + m.server.InstallHint() + ")"
}

func (m viewServerInstallConfirm) boxWidth() int {
	title, question := m.lines()

	maxContent := max(textarea.WidthOf(title), textarea.WidthOf(question))
	wanted := maxContent + 8
	if m.width <= 0 {
		return max(wanted, 40)
	}

	return min(max(wanted, 40), max(m.width-2, 10))
}

func (m viewServerInstallConfirm) renderBox(width int) string {
	inner := width - 4
	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	title, question := m.lines()
	buttons := pickMark(m.cursor == 0, "Yes") + "    " + pickMark(m.cursor == 1, "No")

	rows := []string{
		chars.topLeft + line + chars.topRight,
		chars.vertical + strings.Repeat(" ", width-2) + chars.vertical,
		chars.vertical + " " + padTo(truncateToWidth(title, inner), inner) + " " + chars.vertical,
		chars.vertical + strings.Repeat(" ", width-2) + chars.vertical,
		chars.vertical + " " + padTo(truncateToWidth(question, inner), inner) + " " + chars.vertical,
		chars.vertical + strings.Repeat(" ", width-2) + chars.vertical,
		chars.vertical + " " + padTo(buttons, inner) + " " + chars.vertical,
		chars.vertical + strings.Repeat(" ", width-2) + chars.vertical,
		chars.bottomLeft + line + chars.bottomRight,
	}

	return strings.Join(rows, "\n")
}

func (m viewServerInstallConfirm) View() tea.View {
	width := m.boxWidth()
	box := m.renderBox(width)
	boxRows := strings.Split(box, "\n")
	boxHeight := len(boxRows)

	left := (m.width - width) / 2
	if left < 0 {
		left = 0
	}
	top := (m.height - boxHeight) / 2
	if top < 0 {
		top = 0
	}

	// 부모의 view 를 그대로 쓰고 내용만 갈아끼운다. 종료 확인창과 같은 자리다(ADR-0110).
	view := m.parent.View()

	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(box).X(left).Y(top).Z(1),
	).Render()

	return view
}
