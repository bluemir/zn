package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmDiscard 는 저장하지 않은 변경을 잃게 될 때 한 번 더 묻는 화면이다.
// confirm 은 Yes 를 눌렀을 때 갈 곳이다. 종료일 수도, tab 닫기일 수도, 다시 읽기일 수도 있다.
func ConfirmDiscard(parent tea.Model, e *editor, question string, confirm func() (tea.Model, tea.Cmd)) tea.Model {
	return viewConfirmDiscard{editor: e, parent: parent, question: question, confirm: confirm}
}

// viewConfirmDiscard 는 이 창만의 상태(고른 자리, 물음, Yes 로 갈 곳) 를 든다.
//
// editor 는 이 창이 직접 편집하지 않지만 다른 mode 와 같이 백그라운드 작업의 진행을 받기 위해 든다.
// 창이 떠 있는 동안 온 진행이 버려지면 부모로 돌아갔을 때 표시가 뒤로 돌아간다.
type viewConfirmDiscard struct {
	*editor

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
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go). 파일 검사 tick 은
		// 여기서 보지 않고 주기만 이어 간다 — 보는 것은 normal·트리다(ADR-0038).
		//
		// 이 창은 statusBar 를 그리지 않지만 그래도 받아야 한다. 흘려보내면 다음 조각을 받을
		// Cmd 를 아무도 발행하지 않아 작업이 영영 멈춘다.
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
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
		// 고른 자리는 left/right 로만 움직여서 0(Yes) 아니면 1(No) 이다.
		if m.cursor == 0 {
			return m.confirm()
		}

		return m.parent, nil
	default:
		return m, nil
	}
}

// boxWidth 는 모달 박스의 너비다. 내용에 맞추되 터미널 너비를 넘지 않는다.
func (m viewConfirmDiscard) boxWidth() int {
	maxContent := screenWidthOf("저장하지 않은 변경이 있습니다.")
	for _, qLine := range strings.Split(m.question, "\n") {
		if w := screenWidthOf(qLine); w > maxContent {
			maxContent = w
		}
	}
	if btnW := screenWidthOf(cursor(true, "Yes") + "    " + cursor(false, "No")); btnW > maxContent {
		maxContent = btnW
	}

	wanted := maxContent + 8
	if m.width <= 0 {
		return max(wanted, 40)
	}

	return min(max(wanted, 40), max(m.width-2, 10))
}

// renderBox 는 모달 박스 전체 문자열을 만든다.
func (m viewConfirmDiscard) renderBox(width int) string {
	inner := width - 4 // 좌우 테두리 2 + 좌우 1칸 공백
	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{
		chars.topLeft + line + chars.topRight,
		chars.vertical + strings.Repeat(" ", width-2) + chars.vertical,
		chars.vertical + " " + padTo(truncateToWidth("저장하지 않은 변경이 있습니다.", inner), inner) + " " + chars.vertical,
		chars.vertical + strings.Repeat(" ", width-2) + chars.vertical,
	}

	for _, qLine := range strings.Split(m.question, "\n") {
		rows = append(rows, chars.vertical+" "+padTo(truncateToWidth(qLine, inner), inner)+" "+chars.vertical)
	}

	buttons := cursor(m.cursor == 0, "Yes") + "    " + cursor(m.cursor == 1, "No")

	rows = append(rows,
		chars.vertical+strings.Repeat(" ", width-2)+chars.vertical,
		chars.vertical+" "+padTo(buttons, inner)+" "+chars.vertical,
		chars.vertical+strings.Repeat(" ", width-2)+chars.vertical,
		chars.bottomLeft+line+chars.bottomRight,
	)

	return strings.Join(rows, "\n")
}

func (m viewConfirmDiscard) View() tea.View {
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

	// **부모의 view 를 그대로 쓰고 내용만 갈아끼운다.** 부모 화면 위에 뜨는 것이므로 터미널
	// 상태는 부모를 따라가야 한다 — AltScreen 이 꺼지면 확인창을 띄울 때마다 셸 화면이
	// 번쩍이고, MouseMode 가 꺼지면 mouse 를 끄고 켜는 escape 가 오간다.
	//
	// **베끼지 않고 그대로 쓰는 것이 요점이다.** 전에는 새 view 를 만들고 그 둘만 베꼈는데,
	// `newView` 가 켜 두는 것은 넷이라 ReportFocus 와 키 확장이 이 창에서 꺼졌다. 베끼는
	// 자리를 두면 목록이 늘 때 반드시 뒤처진다(ADR-0110).
	view := m.parent.View()

	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(box).X(left).Y(top).Z(1),
	).Render()

	return view
}

func Exit() (tea.Model, tea.Cmd) {
	return finalExit{}, tea.Quit
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
func quitAll(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	if e.anyDirty() {
		return ConfirmDiscard(parent, e, "정말 종료 하시겠습니까?", Exit), nil
	}

	return Exit()
}

// closeTab 은 `:q` 가 쓰는 경로다. 지금 보고 있는 tab 만 닫는다.
// 마지막 tab 을 닫으면 빈 화면이 남는다.
//
// **닫을 tab 이 없으면 종료다.** 빈 화면을 보면서 `:q` 를 친 것은 닫으라는 것이 아니라
// 나가겠다는 뜻이다 — tab 이 없다는 것이 화면에 이미 드러나 있다. 그래서 `:q` 를 두 번
// 치면 편집기가 끝나고, vim 에서 오는 손버릇이 그대로 산다(ADR-0064).
//
// 활성 tab 에 저장하지 않은 변경이 있으면 확인창을 띄운다. 다른 tab 의 변경은 남으므로 묻지 않는다.
func closeTab(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	if !e.hasTab() {
		return quitAll(parent, e)
	}

	if e.activeBuffer().dirty {
		return ConfirmDiscard(parent, e, "이 tab 을 닫으시겠습니까?", func() (tea.Model, tea.Cmd) {
			return forceCloseTab(e)
		}), nil
	}

	return forceCloseTab(e)
}

// forceCloseTab 은 묻지 않고 활성 tab 을 닫는다. `:q!` 와 확인창의 Yes 가 쓴다.
//
// 닫을 tab 이 없으면 종료다. `:q` 와 같은 자리다(closeTab, ADR-0064).
func forceCloseTab(e *editor) (tea.Model, tea.Cmd) {
	if !e.hasTab() {
		return Exit()
	}

	e.closeTab()

	// 닫은 파일이 아니라 그 자리에 드러난 파일이 이제 보는 파일이다. 트리가 아직 그 자리를
	// 읽지 않았으면 읽는 작업이 시작된다(ADR-0032).
	//
	// 마지막 tab 이었으면 드러날 파일이 없어서 **트리를 건드리지 않는다.** 접거나 뿌리로
	// 되돌리지도 않는다 — tab 을 닫은 사람은 대개 그 옆의 것을 열려는 참이다(ADR-0064).
	var reveal tea.Cmd
	if e.hasTab() {
		reveal = e.revealInSidebar(e.activeBuffer().path)
	}

	model, cmd := normalMode(e)

	return model, tea.Batch(cmd, reveal)
}

// closeTabAt 은 tabline 우클릭이 쓰는 경로다. index 자리의 tab 하나를 닫는다(ADR-0060).
//
// **tab 이 하나뿐이어도 닫는다.** 예전에는 거부했는데, 그 근거는 「한 번 잘못 누른 것으로
// 편집기가 통째로 닫히면 안 된다」 하나였다. 이제 마지막 tab 을 닫으면 빈 화면이 남고
// 편집기는 끝나지 않으므로 그 근거가 사라졌다 — 슬쩍 눌러 잃는 것은 그 tab 의 커서와
// 스크롤이고, 그것은 남의 tab 을 우클릭할 때 이미 잃는 것과 같다(ADR-0064).
//
// 저장하지 않은 변경이 있으면 확인창을 띄운다. 보고 있지 않은 tab 이어도 묻는다 — 오히려
// 그쪽이 무엇을 잃는지 화면에 드러나지 않는다('다른 tab 모두 닫기' 와 같다, ADR-0016).
// 그래서 물음에 파일 이름을 적는다. 「이 tab」 이라고만 하면 어느 것인지 알 수 없다.
func closeTabAt(parent tea.Model, e *editor, index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(e.buffers) {
		return parent, nil
	}

	if e.buffers[index].dirty {
		question := fmt.Sprintf("%s tab 을 닫으시겠습니까?", e.tabName(index))

		return ConfirmDiscard(parent, e, question, func() (tea.Model, tea.Cmd) {
			return forceCloseTabAt(parent, e, index)
		}), nil
	}

	return forceCloseTabAt(parent, e, index)
}

// forceCloseTabAt 은 묻지 않고 index 자리의 tab 을 닫는다. 확인창의 Yes 도 쓴다.
//
// 보고 있지 않은 tab 을 닫았으면 그대로 parent 로 돌아간다. 보는 파일도 커서도 그대로여서
// mode 를 옮길 이유가 없다 — insert 로 치던 중에 옆 tab 을 닫았으면 계속 치면 된다.
//
// 보고 있던 tab 을 닫았으면 normal 로 간다. 고치던 buffer 가 사라졌으니 insert·visual 에
// 남을 수 없다. 남으면 그 mode 가 이제 다른 파일을 고친다. `:q` 와 같은 자리다.
func forceCloseTabAt(parent tea.Model, e *editor, index int) (tea.Model, tea.Cmd) {
	viewing := index == e.active

	if !e.closeTabAt(index) {
		return parent, nil
	}

	if !viewing {
		return parent, nil
	}

	// 닫은 파일이 아니라 그 자리에 드러난 파일이 이제 보는 파일이다. 트리가 아직 그 자리를
	// 읽지 않았으면 읽는 작업이 시작된다(ADR-0032). 마지막 tab 이었으면 드러날 파일이
	// 없어서 트리를 그대로 둔다(ADR-0064).
	var reveal tea.Cmd
	if e.hasTab() {
		reveal = e.revealInSidebar(e.activeBuffer().path)
	}

	model, cmd := normalMode(e)

	return model, tea.Batch(cmd, reveal)
}
