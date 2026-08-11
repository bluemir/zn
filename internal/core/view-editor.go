package core

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type viewEditor struct {
	buffers []Buffer
}

func (m viewEditor) Init() tea.Cmd {
	return nil
}
func (m viewEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return QuitConfirm(m), nil
		case "q":

			return m, nil
		case "up":
			//m.cursor--
			//if m.cursor < 0 {
			//		m.cursor = 0
			//	}
			return m, nil
		case "down":
			//	m.cursor++
			//	if m.cursor > 2 {
			//		m.cursor = 2
			//	}
			return m, nil
		default:
			return m, nil
		}
	default:
		return m, nil
	}
}
func (m viewEditor) View() tea.View {
	view := tea.NewView(lipgloss.JoinVertical(
		lipgloss.Left,
		"hello world",
	))

	view.MouseMode = tea.MouseModeAllMotion
	view.AltScreen = true
	return view
}
