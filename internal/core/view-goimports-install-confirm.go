package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// goimportsInstallConfirmMode 는 goimports 가 없을 때 설치 여부를 묻는 모달 화면이다.
// Go 파일을 저장할 때 들어온다(save-hook.go, ADR-0065).
//
// **gopls 확인창(view-gopls-install-confirm.go) 과 같은 모양이고 따로 산다.** 하나로 묶어
// 물음과 설치를 값으로 받게 하면 창 하나에 부르는 쪽 둘이 매달리는데, 그 둘은 뜨는 까닭도
// 거절의 뜻도 다르다 — 이쪽은 저장에 딸려 와서 거절을 적어 두고, 저쪽은 사용자가 친 키에서만
// 온다. 같아 보이는 것을 묶는 값보다 각자 고쳐지는 값이 크다고 보았다.
func goimportsInstallConfirmMode(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	return viewGoimportsInstallConfirm{editor: e, parent: parent}, nil
}

// viewGoimportsInstallConfirm 은 goimports 설치 확인창의 상태를 든다.
type viewGoimportsInstallConfirm struct {
	*editor

	parent tea.Model
	cursor int // 0: Yes, 1: No
}

func (m viewGoimportsInstallConfirm) Init() tea.Cmd {
	return nil
}

func (m viewGoimportsInstallConfirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			confirm, ok := model.(viewGoimportsInstallConfirm)
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
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, watchMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg:
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
func (m viewGoimportsInstallConfirm) press(key string) (tea.Model, tea.Cmd) {
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
		m.goimportsDeclined = true

		return m.parent, nil
	case "enter":
		if m.cursor == 0 {
			return m.parent, m.installGoimports()
		}

		// **거절을 적어 둔다.** 이 물음은 사용자가 부른 것이 아니라 저장에 딸려 오므로,
		// 적어 두지 않으면 저장할 때마다 창이 뜬다 — 그러면 그것이 곧 방해다(ADR-0065).
		m.goimportsDeclined = true

		return m.parent, nil
	default:
		return m, nil
	}
}

func (m viewGoimportsInstallConfirm) boxWidth() int {
	title := "goimports 가 설치되어 있지 않습니다."
	question := "지금 설치하시겠습니까? (go install golang.org/x/tools/cmd/goimports@latest)"

	maxContent := max(screenWidthOf(title), screenWidthOf(question))
	wanted := maxContent + 8
	if m.width <= 0 {
		return max(wanted, 40)
	}

	return min(max(wanted, 40), max(m.width-2, 10))
}

func (m viewGoimportsInstallConfirm) renderBox(width int) string {
	inner := width - 4
	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	title := "goimports 가 설치되어 있지 않습니다."
	question := "지금 설치하시겠습니까? (go install golang.org/x/tools/cmd/goimports@latest)"
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

func (m viewGoimportsInstallConfirm) View() tea.View {
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
