package core

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// viewMessages 는 `:messages` 로 여는 알림 기록이다(ADR-0053).
//
// 화면을 통째로 쓰는 세 번째 목록이다. `:jobs`(ADR-0027) 와 `GOTO`(ADR-0051) 와 같은 문법을
// 따른다 — 제목 띠, 본문, 키 안내 한 줄, 트리를 지운 statusBar.
//
// **`:jobs` 와 다른 것이 둘 있다.**
//
// 하나는 순서다. 작업 목록은 새것이 위인데(job.go 의 finishJob) 여기는 오래된 것이 위다.
// 알림 기록은 뒤에만 붙으므로 이미 있는 줄의 자리가 밀리지 않고, 그래야 목록을 보는 동안
// 새 알림이 도착해도 고른 줄이 다른 알림으로 바뀌지 않는다. 작업 목록이 앞에 끼워 넣으면서도
// 「고른 자리는 유지한다」고 한 것은 앞에 끼는 일이 드물어서 통하는 근사치인데, 알림 기록은
// 앞에 끼는 것이 유일한 변화라 그 근사치가 맞지 않는다.
//
// 둘은 하는 일이다. 작업 목록은 도는 것을 보고 취소하는 화면이고 여기는 지나간 것을 읽는
// 화면이라 지우거나 고치는 키가 없다.
//
// 이름이 갈리는 자리가 하나 있다 — 명령과 화면은 vim 을 따라 `messages` 이고, 알갱이 하나는
// `notice` 다. 알림을 세우는 자리가 `notify` 라서 그쪽 이름을 알갱이에 맞췄다(notice.go).
func messagesMode(e *editor) (tea.Model, tea.Cmd) {
	m := viewMessages{editor: e}

	// 아래 줄에 떠 있던 알림은 이미 이 목록의 마지막 줄이다. 비워야 같은 문구가 두 번
	// 보이지 않는다. 기록은 건드리지 않는다(notice.go 의 clearNotice).
	m.clearNotice()

	// 맨 아래에서 시작한다. 찾는 것은 대개 방금 지나간 것이다.
	m.selected = len(m.rows()) - 1
	m.scrollTo()

	return m, nil
}

type viewMessages struct {
	*editor

	selected int // rows() 안의 자리
	top      int // 화면 첫 행

	// failedOnly 는 실패만 걸러 보는 중인지다. `f` 가 켜고 끈다.
	failedOnly bool
}

// rows 는 목록에 보일 알림이다. 거르는 중이면 실패한 것만이다.
//
// **고른 자리와 첫 행은 이것 안의 자리다.** 걸러 놓고 `m.notices` 로 세면 목록에 없는
// 알림을 가리키게 된다.
//
// 거르지 않을 때는 담긴 것을 그대로 준다. 알림 기록은 뒤에만 붙으므로(ADR-0053) 새 slice 를
// 만들 이유가 없다.
func (m viewMessages) rows() []notice {
	if !m.failedOnly {
		return m.notices
	}

	rows := make([]notice, 0, len(m.notices))
	for _, entry := range m.notices {
		if entry.failed {
			rows = append(rows, entry)
		}
	}

	return rows
}

func (m viewMessages) Init() tea.Cmd { return nil }

func (m viewMessages) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// **이 자리가 이 화면의 값이다.** 여기로 오는 것들이 알림을 내는 자리이고
		// (작업 실패·바깥 파일 변경·서버 기동 실패), 그것들은 사람이 키를 누르지 않았는데
		// 도착해서 지금까지 다음 키에 사라졌다. 목록을 열어 둔 채로 그것이 쌓이는 것을 본다.
		//
		// **맨 아래였는지는 여기서 잰다.** handleJob 이 알림을 늘릴 수 있으므로 그 뒤에
		// 재면 늦다 — 늘어난 개수로 되짚으면 「끝에서 둘째 줄」과 갈리지 않아서, 알림이
		// 늘지 않는 msg 가 올 때마다 고른 자리가 한 칸씩 아래로 끌려간다(ADR-0053).
		atBottom := m.selected == len(m.rows())-1

		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}
		m.follow(atBottom)

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 실행한다.
//
// 키 상태 기계를 두지 않은 것은 `GOTO` 와 같은 이유다 — 받는 키가 전부 한 개짜리다
// (view-locations.go). 한글은 되돌린다. 입력줄이 없는 화면이라 `ㅓ` 를 `j` 로 읽어야 한다(ADR-0008).
func (m viewMessages) press(key string) (tea.Model, tea.Cmd) {
	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewMessages)
		if !ok {
			return model, nil
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, cmd
		}

		model = next
	}

	return model, nil
}

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
//
// 알림을 지우거나 고치는 키는 없다. 기록은 전부 남기기로 한 것이라(ADR-0053) 지우는 키가
// 그 결정과 어긋난다.
func (m viewMessages) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return normalMode(m.editor)
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
		m.move(pageRows(pageFull, m.listHeight()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.listHeight()))

		return m, nil
	case "g", "home":
		m.selected = 0
		m.scrollTo()

		return m, nil
	case "G", "end":
		m.selected = len(m.rows()) - 1
		m.scrollTo()

		return m, nil
	case "f":
		// 실패만 걸러 본다. 목록을 여는 까닭이 대개 「뭔가 잘못됐나」라서, 제목줄의 개수만으로
		// 모자랄 때 이것으로 좁힌다(ADR-0053).
		//
		// **켜고 끌 때마다 맨 아래로 간다.** 거른 목록에서 지금 고른 알림이 어디로 갔는지를
		// 되짚어 옮기는 것보다, 목록을 다시 여는 것과 같이 두는 편이 헷갈리지 않는다
		// (messagesMode). 찾는 것은 대개 방금 지나간 실패다.
		m.failedOnly = !m.failedOnly
		m.selected = len(m.rows()) - 1
		m.scrollTo()

		return m, nil
	default:
		return m, nil
	}
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다. 다른 목록들과 같은 규칙이다.
func (m *viewMessages) move(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(rows)-1)
	m.scrollTo()
}

// follow 는 새 알림이 왔을 때 화면을 맞춘다. atBottom 은 **늘기 전에** 맨 아래였는지다.
//
// **맨 아래를 보고 있었으면 따라 내려가고, 아니면 그대로 둔다.** 뒤에만 붙는 목록이라
// 그대로 두는 것은 공짜고, 맨 아래는 「지금 오는 것을 보는 중」이라는 뜻이라 따라가야 한다.
// 로그를 보는 프로그램들이 하는 것과 같다.
//
// 맨 아래였는지를 인자로 받는 것이 요점이다. 늘어난 뒤의 개수로 되짚으려 하면 그 짐작이
// 끝에서 둘째 줄까지 삼켜서, 알림이 늘지 않는 msg 마다 고른 자리가 아래로 끌려간다.
func (m *viewMessages) follow(atBottom bool) {
	if atBottom {
		m.selected = len(m.rows()) - 1
	}

	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. `:jobs` 의 것과 같은 규칙이다.
func (m *viewMessages) scrollTo() {
	rows, height := len(m.rows()), m.listHeight()
	if rows == 0 || height < 1 {
		m.selected, m.top = 0, 0

		return
	}

	m.selected = min(max(m.selected, 0), rows-1)
	m.top = min(max(m.top, 0), max(rows-height, 0))

	if m.selected < m.top {
		m.top = m.selected
	}
	if m.selected >= m.top+height {
		m.top = m.selected - height + 1
	}
}

func (m viewMessages) View() tea.View {
	height := m.listHeight()

	rows := m.rows()

	body := make([]string, 0, height)
	for i := m.top; i < len(rows) && len(body) < height; i++ {
		body = append(body, m.renderRow(rows[i], i == m.selected))
	}

	if len(rows) == 0 && height > 0 {
		body = append(body, " "+m.emptyReason())
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(m.renderHint()))
	screen = append(screen, m.renderBareStatusBar()...)

	view := newView(screen)

	if len(rows) > 0 {
		view.Cursor = tea.NewCursor(0, m.selected-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// renderTitle 은 맨 윗줄이다. 몇 개이고 그중 실패가 몇인지 적는다 —
// 목록을 여는 까닭이 대개 「뭔가 잘못됐나」라서 그 수가 제목에 있어야 한다.
//
// **거르는 중에도 담긴 것 전부를 센다.** 거른 뒤의 수는 목록 길이가 이미 말하고, 제목이
// 같이 좁아지면 무엇에서 걸러낸 것인지 알 수 없다.
func (m viewMessages) renderTitle() string {
	failed := 0
	for _, entry := range m.notices {
		if entry.failed {
			failed++
		}
	}

	label := fmt.Sprintf("알림  %d 개", len(m.notices))
	if failed > 0 {
		label += fmt.Sprintf(" · 실패 %d", failed)
	}
	if m.failedOnly {
		label += " · 실패만"
	}

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// emptyReason 은 목록이 빈 까닭이다. 담긴 것이 없는 것과 걸러서 없는 것이 다르다 —
// 거르는 중에 「지나간 알림이 없습니다」가 뜨면 기록이 사라진 것처럼 읽힌다.
func (m viewMessages) emptyReason() string {
	if m.failedOnly {
		return "실패한 알림이 없습니다"
	}

	return "지나간 알림이 없습니다"
}

// renderHint 는 키 안내 한 줄이다. `f` 는 지금 상태의 **반대**를 적는다 — 누르면 무엇이
// 되는지가 안내이고, 지금 무엇인지는 제목줄이 말한다.
func (m viewMessages) renderHint() string {
	filter := "f 실패만"
	if m.failedOnly {
		filter = "f 전부"
	}

	return " j/k 이동  g/G 처음·끝  " + filter + "  q 닫기"
}

// noticeMarkWidth 는 시각과 갈래 표시가 쓰는 앞머리 폭이다. `▸ ` 두 칸까지 더한 값이다.
const noticeMarkWidth = 2 + 8 + 1 + 1 + 1 // 표시 + 15:04:05 + 공백 + !/공백 + 공백

// renderRow 는 알림 한 줄이다.
//
//	▸ 14:03:11   저장함: docs/tasks.md
//	  14:03:05 ! git 상태 실패: exit status 128
//
// **갈래를 색으로만 나타내지 않는다.** 오류에 `!` 를 찍는다 — 화면을 글자로 떠서 보는 길이
// 있고(.claude/skills/run-zn), 색으로만 갈리면 그 길에서 갈래가 사라진다.
func (m viewMessages) renderRow(entry notice, selected bool) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	kind := " "
	if entry.failed {
		kind = "!"
	}

	head := " " + marker + styleDetail.Render(formatClock(entry.at)) + " " + kind + " "
	text := trimTextRight(entry.text, m.width-noticeMarkWidth-1)

	if entry.failed {
		text = styleNoticeFailed.Render(text)
	}

	return head + text
}

// renderBareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다. `:jobs` 와 같은 이유다
// (view-jobs.go) — 이 화면은 트리를 덮는다.
//
// 아래 줄은 비어 있다. 방금 뜬 알림은 이 목록의 마지막 줄에 이미 있다.
func (m viewMessages) renderBareStatusBar() []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	return bare.renderStatusBar("MESSAGES", bare.notice)
}

// formatClock 은 알림이 난 시각이다.
//
// 늘 여덟 칸이라 목록의 세로 줄이 맞는다. 초까지 찍는 것은 알림이 초 단위로 몰려 오기
// 때문이다 — 저장하면 곧바로 git 갱신이 따라온다. 분까지만 찍으면 같은 시각이 줄줄이 늘어
// 시각 칸이 뜻을 잃는다.
//
// 날짜는 찍지 않는다. 한 세션이 하루를 넘는 일이 드물고, 넘어도 순서는 목록 순서가 말해 준다.
//
// 이 저장소에서 절대 시각을 화면에 찍는 첫 자리다. 지금까지 있던 것은 경과를 재는
// `formatElapsed`(view-jobs.go) 뿐이었다(ADR-0053).
func formatClock(at time.Time) string {
	return at.Format("15:04:05")
}

// trimTextRight 는 너무 긴 알림을 오른쪽부터 줄인다.
//
// 경로를 왼쪽부터 접는 `trimPathLeft`(view-locations.go) 와 반대다. 경로는 뒤쪽의 파일
// 이름과 줄 번호가 고르는 데 쓰이지만, 알림은 앞머리가 무엇이 일어났는지다.
func trimTextRight(text string, width int) string {
	if width < 1 {
		return ""
	}
	if screenWidthOf(text) <= width {
		return text
	}

	return truncateToWidth(text, width-1) + "…"
}

// runMessages 는 팔레트의 「알림 목록」이다. `:messages` 와 같은 길이다.
func runMessages(e *editor) (tea.Model, tea.Cmd) {
	return messagesMode(e)
}
