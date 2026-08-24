package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// goplsInstallConfirmMode 는 gopls 가 없을 때 설치 여부를 묻는 모달 화면이다.
// `\gd` 또는 팔레트의 「정의로 가기」에서 들어온다.
func goplsInstallConfirmMode(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	return viewGoplsInstallConfirm{editor: e, parent: parent}, nil
}

// viewGoplsInstallConfirm 은 gopls 설치 확인창의 상태를 든다.
type viewGoplsInstallConfirm struct {
	*editor

	parent tea.Model
	cursor int // 0: Yes, 1: No
}

func (m viewGoplsInstallConfirm) Init() tea.Cmd {
	return nil
}

func (m viewGoplsInstallConfirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			confirm, ok := model.(viewGoplsInstallConfirm)
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
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg:
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
func (m viewGoplsInstallConfirm) press(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return Exit()
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
			return m.parent, m.installGopls()
		}

		return m.parent, nil
	default:
		return m, nil
	}
}

func (m viewGoplsInstallConfirm) boxWidth() int {
	title := "gopls 가 설치되어 있지 않습니다."
	question := "지금 설치하시겠습니까? (go install golang.org/x/tools/gopls@latest)"

	maxContent := max(screenWidthOf(title), screenWidthOf(question))
	wanted := maxContent + 8
	if m.width <= 0 {
		return max(wanted, 40)
	}

	return min(max(wanted, 40), max(m.width-2, 10))
}

func (m viewGoplsInstallConfirm) renderBox(width int) string {
	inner := width - 4
	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	title := "gopls 가 설치되어 있지 않습니다."
	question := "지금 설치하시겠습니까? (go install golang.org/x/tools/gopls@latest)"
	buttons := cursor(m.cursor == 0, "Yes") + "    " + cursor(m.cursor == 1, "No")

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

func (m viewGoplsInstallConfirm) View() tea.View {
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

	parent := m.parent.View()
	view := tea.NewView(
		lipgloss.NewCompositor(
			lipgloss.NewLayer(parent.Content).Z(0),
			lipgloss.NewLayer(box).X(left).Y(top).Z(1),
		).Render(),
	)

	view.AltScreen = parent.AltScreen
	view.MouseMode = parent.MouseMode

	return view
}
