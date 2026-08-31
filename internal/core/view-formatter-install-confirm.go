package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// formatterInstallConfirmMode 는 저장 포매터가 없을 때 설치 여부를 묻는 모달 화면이다.
// 그 언어의 파일을 저장할 때 들어온다(save-hook.go, ADR-0065).
//
// **언어 서버 확인창(view-server-install-confirm.go) 과 같은 모양이고 따로 산다.** 하나로 묶어
// 물음과 설치를 값으로 받게 하면 창 하나에 부르는 쪽 둘이 매달리는데, 그 둘은 뜨는 까닭도
// 거절의 뜻도 다르다 — 이쪽은 저장에 딸려 와서 거절을 적어 두고, 저쪽은 사용자가 친 키에서만
// 온다. 같아 보이는 것을 묶는 값보다 각자 고쳐지는 값이 크다고 보았다.
//
// **포매터끼리는 묶었다.** goimports 와 ruff 는 뜨는 까닭도(저장) 거절의 뜻도 같아서, 위에서
// 가른 그 기준이 여기서는 같다고 말한다(ADR-0107).
func formatterInstallConfirmMode(parent tea.Model, e *editor, spec *formatter) (tea.Model, tea.Cmd) {
	return viewFormatterInstallConfirm{editor: e, parent: parent, spec: spec}, nil
}

// viewFormatterInstallConfirm 은 저장 포매터 설치 확인창의 상태를 든다.
type viewFormatterInstallConfirm struct {
	*editor

	parent tea.Model
	spec   *formatter
	cursor int // 0: Yes, 1: No
}

func (m viewFormatterInstallConfirm) Init() tea.Cmd {
	return nil
}

func (m viewFormatterInstallConfirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			confirm, ok := model.(viewFormatterInstallConfirm)
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
func (m viewFormatterInstallConfirm) press(key string) (tea.Model, tea.Cmd) {
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
		m.declineFormatter(m.spec)

		return m.parent, nil
	case "enter":
		if m.cursor == 0 {
			return m.parent, m.installFormatter(m.spec)
		}

		// **거절을 적어 둔다.** 이 물음은 사용자가 부른 것이 아니라 저장에 딸려 오므로,
		// 적어 두지 않으면 저장할 때마다 창이 뜬다 — 그러면 그것이 곧 방해다(ADR-0065).
		m.declineFormatter(m.spec)

		return m.parent, nil
	default:
		return m, nil
	}
}

// lines 는 창에 적는 두 줄이다. 폭을 재는 자리와 그리는 자리가 같은 글을 보아야 한다.
func (m viewFormatterInstallConfirm) lines() (string, string) {
	return m.spec.name + " 가 설치되어 있지 않습니다.",
		"지금 설치하시겠습니까? (" + m.spec.InstallHint() + ")"
}

func (m viewFormatterInstallConfirm) boxWidth() int {
	title, question := m.lines()

	maxContent := max(screenWidthOf(title), screenWidthOf(question))
	wanted := maxContent + 8
	if m.width <= 0 {
		return max(wanted, 40)
	}

	return min(max(wanted, 40), max(m.width-2, 10))
}

func (m viewFormatterInstallConfirm) renderBox(width int) string {
	inner := width - 4
	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	title, question := m.lines()
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

func (m viewFormatterInstallConfirm) View() tea.View {
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
