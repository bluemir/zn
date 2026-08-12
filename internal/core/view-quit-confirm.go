package core

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"
)

// quit 은 `:q` 와 `Ctrl+C` 가 공유하는 종료 경로다. 둘의 동작이 같아야 한다.
//
// 저장하지 않은 변경이 있을 때만 확인창을 띄운다. 잃을 것이 없으면 묻지 않고 나간다.
// parent 는 확인창에서 취소했을 때 돌아갈 화면이다.
func quit(parent tea.Model, buf *Buffer) (tea.Model, tea.Cmd) {
	if buf.dirty {
		return QuitConfirm(parent), nil
	}

	return Exit()
}

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
	view := tea.NewView(
		style.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				// 좁은 화면에서 잘리지 않게 두 줄로 나눈다.
				// 이 화면은 터미널 너비를 몰라서 statusBar 처럼 잘라내지 못한다.
				"저장하지 않은 변경이 있습니다.",
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

	// 부모 화면 위에 뜨는 것이므로 터미널 상태는 부모를 따라간다.
	// 여기서 AltScreen 이 꺼지면 종료 확인창을 띄울 때마다 셸 화면이 번쩍이고 편집 내용이 사라진다.
	view.AltScreen = m.parent.View().AltScreen

	return view
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
