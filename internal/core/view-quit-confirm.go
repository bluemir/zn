package core

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"
)

// ConfirmDiscard 는 저장하지 않은 변경을 잃게 될 때 한 번 더 묻는 화면이다.
// confirm 은 Yes 를 눌렀을 때 갈 곳이다. 종료일 수도, tab 닫기일 수도, 다시 읽기일 수도 있다.
func ConfirmDiscard(parent tea.Model, question string, confirm func() (tea.Model, tea.Cmd)) tea.Model {
	return viewConfirmDiscard{parent: parent, question: question, confirm: confirm}
}

type viewConfirmDiscard struct {
	parent   tea.Model
	question string
	confirm  func() (tea.Model, tea.Cmd)
	cursor   int
}

func (m viewConfirmDiscard) Init() tea.Cmd {
	return nil
}

func (m viewConfirmDiscard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		// 편집 화면과 같은 방식이다 — 음절 하나가 키 여럿으로 풀리므로 차례로 먹인다.
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			confirm, ok := model.(viewConfirmDiscard)
			if !ok {
				// 앞의 키에서 창을 벗어났다. 남은 키는 버린다.
				return model, nil
			}

			next, cmd := confirm.press(key)
			if cmd != nil {
				return next, cmd
			}

			model = next
		}

		return model, nil
	case jobProgressMsg:
		// 이 창은 editor 를 들고 있지 않아 진행을 반영할 곳이 없다. 그렇다고 흘려보내면
		// 다음 조각을 받을 Cmd 를 아무도 발행하지 않아 작업이 영영 멈춘다. 고리만 잇는다 —
		// 놓친 진행은 부모로 돌아간 뒤 다음 조각이 통째로 채운다(job.go).
		return m, waitJob(msg.name, msg.ch)
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다.
func (m viewConfirmDiscard) press(key string) (tea.Model, tea.Cmd) {
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
		switch m.cursor {
		case 0:
			return m.confirm()
		case 1:
			return m.parent, nil
		default:
			return ExitWithError(errors.Errorf("Invalid state"))
		}
	default:
		return m, nil
	}
}
func (m viewConfirmDiscard) View() tea.View {
	style := lipgloss.NewStyle().Padding(2)
	view := tea.NewView(
		style.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				// 좁은 화면에서 잘리지 않게 두 줄로 나눈다.
				// 이 화면은 터미널 너비를 몰라서 statusBar 처럼 잘라내지 못한다.
				"저장하지 않은 변경이 있습니다.",
				m.question,
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
	//
	// MouseMode 도 같이 따라간다. 이 창은 mouse 를 받지 않지만(선택지 둘이라 키로 충분하다)
	// 여기서 꺼지면 확인창이 뜰 때마다 터미널에 mouse 를 끄고 켜는 escape 가 오간다.
	parent := m.parent.View()
	view.AltScreen = parent.AltScreen
	view.MouseMode = parent.MouseMode

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

// quitAll 은 `Ctrl+C` 와 `:qa` 가 쓰는 경로다. 편집기를 통째로 끝낸다.
//
// 어느 tab 이든 저장하지 않은 변경이 있으면 확인창을 띄운다. 보고 있지 않은 tab 의 변경도
// 같이 잃기 때문에 활성 buffer 만 봐서는 안 된다. 잃을 것이 없으면 묻지 않고 나간다.
// parent 는 확인창에서 취소했을 때 돌아갈 화면이다.
func quitAll(parent tea.Model, e editor) (tea.Model, tea.Cmd) {
	if e.anyDirty() {
		return ConfirmDiscard(parent, "정말 종료 하시겠습니까?", Exit), nil
	}

	return Exit()
}

// closeTab 은 `:q` 가 쓰는 경로다. 지금 보고 있는 tab 만 닫는다.
// 마지막 tab 이면 닫을 것이 없으므로 종료가 된다.
//
// 활성 tab 에 저장하지 않은 변경이 있으면 확인창을 띄운다. 다른 tab 의 변경은 남으므로 묻지 않는다.
func closeTab(parent tea.Model, e editor) (tea.Model, tea.Cmd) {
	if len(e.buffers) < 2 {
		return quitAll(parent, e)
	}

	if e.buffer().dirty {
		return ConfirmDiscard(parent, "이 tab 을 닫으시겠습니까?", func() (tea.Model, tea.Cmd) {
			return forceCloseTab(e)
		}), nil
	}

	return forceCloseTab(e)
}

// forceCloseTab 은 묻지 않고 활성 tab 을 닫는다. `:q!` 와 확인창의 Yes 가 쓴다.
func forceCloseTab(e editor) (tea.Model, tea.Cmd) {
	if !e.closeTab() {
		return Exit()
	}

	return normalMode(e)
}
