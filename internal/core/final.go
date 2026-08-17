package core

import tea "charm.land/bubbletea/v2"

type final struct{}

func (m final) Init() tea.Cmd                           { return tea.Quit }
func (m final) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, tea.Quit }
func (m final) View() tea.View                          { return tea.NewView("") }

type finalExit struct {
	final
}
