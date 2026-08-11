package core

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"
)

func QuitConfirm(parent tea.Model) tea.Model {
	return viewQuitConfirm{parent: parent}
}

type viewQuitConfirm struct {
	parent tea.Model
	cursor int
}

func (m viewQuitConfirm) Init() tea.Cmd {
	return nil
}

func (m viewQuitConfirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
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
			switch m.cursor {
			case 0:
				return Exit()
			case 1:
				return m.parent, nil
			default:
				return ExitWithError(errors.Errorf("Invalid state"))
			}
		default:
			return m, nil
		}
	default:
		return m, nil
	}
}
func (m viewQuitConfirm) View() tea.View {
	style := lipgloss.NewStyle().Padding(2)
	return tea.NewView(
		style.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				"정말 종료 하시겠습니까?",
				"",
				lipgloss.JoinHorizontal(
					lipgloss.Top,
					cursor(m.cursor == 0, "Yes"),
					cursor(m.cursor == 1, "No"),
				),
			),
		),
	)
}

func Exit() (tea.Model, tea.Cmd) {
	return finalExit{}, tea.Quit
}
func ExitWithError(err error) (tea.Model, tea.Cmd) {
	return finalExit{err: err}, tea.Quit
}

func cursor(cond bool, str string) string {
	if cond {
		return "> " + str
	} else {
		return "  " + str
	}
}
