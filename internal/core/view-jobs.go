package core

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// viewJobs 는 `:jobs` 로 여는 작업 목록이다.
//
// 도는 작업이 위, 최근에 끝난 것이 아래다. 도는 것은 막대와 백분율을, 끝난 것은 상태와 요약을 보여준다.
// statusBar 는 맨 앞 하나와 개수만 찍으므로(job.go) 둘째 작업이 무엇인지, 조용히 실패한 것이
// 있는지는 여기서만 알 수 있다.
//
// 팔레트처럼 박스로 얹지 않는다. 목록이 실시간으로 자라고 줄어들어서, 아래 화면이 비치면 무엇이
// 움직이는 것인지 읽기 어렵다.
//
// tabline 과 sidebar 도 그리지 않는다. 편집기 틀을 쓰면 "파일을 보고 있는 화면" 으로 읽혀서
// 여기서 치는 키가 편집기 키인 줄 알게 된다. 맨 위 제목줄이 그 자리를 대신하고, statusBar 만
// 남겨 mode 와 git·알림이 늘 같은 자리에 있게 한다.
func jobsMode(e *editor) (tea.Model, tea.Cmd) {
	return viewJobs{editor: e}, nil
}

type viewJobs struct {
	*editor

	selected int // rows 안의 자리
	top      int // 화면 첫 행

	// state 는 키 나열을 동작 하나로 만드는 상태다.
	// mode 안에서만 사는 상태라 editor 가 아니라 여기에 둔다(ADR-0002).
	state jobsState
}

// keyState 는 지금 키 상태다. zero value(nil) 는 아무것도 먹지 않은 처음이다.
func (m viewJobs) keyState() jobsState {
	if m.state == nil {
		return jobsStart{}
	}

	return m.state
}

func (m viewJobs) Init() tea.Cmd { return nil }

func (m viewJobs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		// 한글 되돌림은 파서가 한다. 여기는 키를 그대로 넘긴다(ADR-0008).
		// `x`(취소)·`q`(닫기)·`j`·`k` 가 전부 글자 키라 되돌리지 않으면 아무것도 먹지 않는다.
		return m.press(msg.String())
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg:
		// 진행이 오면 목록이 그대로 자란다. 고른 자리는 유지하고 범위만 맞춘다 —
		// 작업 하나가 끝날 때마다 커서가 튀면 취소하려던 것을 놓친다.
		cmd := m.handleJob(msg)
		m.scrollTo()

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 차례로 실행한다.
// normal·트리와 같은 나눔이고, 도중에 목록을 벗어나면 남은 동작은 버린다(ADR-0008).
func (m viewJobs) press(key string) (tea.Model, tea.Cmd) {
	m.message = ""

	// 한글은 파서가 받아서 푼다. 여기는 키를 그대로 넘기고 나온 동작을 실행하기만 한다.
	names, state := m.keyState().press(key)
	m.state = state

	var model tea.Model = m
	for _, name := range names {
		jobs, ok := model.(viewJobs)
		if !ok {
			return model, nil
		}

		next, cmd := jobs.run(name)
		if cmd != nil {
			return next, cmd
		}

		model = next
	}

	return model, nil
}

// run 은 완성된 동작 하나를 실행한다. 모르는 이름이면 아무 일도 하지 않는다.
func (m viewJobs) run(name string) (tea.Model, tea.Cmd) {
	switch name {
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
	case "x":
		return m, m.cancelSelected()
	default:
		return m, nil
	}
}

// cancelSelected 는 고른 작업에게 그만하라고 말한다.
//
// 끝난 작업을 고르고 눌렀으면 알리고 만다. 아무 일도 안 나면 키가 먹었는지 알 수 없다.
func (m *viewJobs) cancelSelected() tea.Cmd {
	rows := m.jobRows()
	if m.selected >= len(rows) {
		return nil
	}

	selected := rows[m.selected]
	if selected.cancel == nil {
		m.message = "이미 끝난 작업입니다"

		return nil
	}

	m.cancelJob(selected.name)

	return nil
}

// jobRows 는 목록에 그릴 작업들이다. 도는 것이 위, 끝난 것이 아래다.
func (e editor) jobRows() []job {
	rows := make([]job, 0, len(e.jobs)+len(e.finished))
	rows = append(rows, e.jobs...)
	rows = append(rows, e.finished...)

	return rows
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다. 팔레트·트리와 같은 규칙이다.
func (m *viewJobs) move(delta int) {
	rows := m.jobRows()
	if len(rows) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(rows)-1)
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. sidebar 의 것과 같은 규칙이다.
func (m *viewJobs) scrollTo() {
	rows, height := len(m.jobRows()), m.listHeight()
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

// jobNameWidth 는 이름 칸의 폭이다. 이름이 짧아도 진행 막대가 세로로 줄이 맞아야 읽힌다.
const jobNameWidth = 16

func (m viewJobs) View() tea.View {
	rows := m.jobRows()
	height := m.listHeight()

	now := time.Now()

	body := make([]string, 0, height)
	for i := m.top; i < len(rows) && len(body) < height; i++ {
		body = append(body, m.jobRow(rows[i], i == m.selected, now))
	}

	// 도는 것도 끝난 것도 없을 때 빈 화면만 두면 목록을 못 연 것처럼 보인다.
	if len(rows) == 0 && height > 0 {
		body = append(body, " 도는 작업이 없습니다")
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.title()}, body...)
	screen = append(screen, styleDetail.Render(" j/k 이동  x 취소  q 닫기"))
	screen = append(screen, m.bareStatusBar()...)

	view := m.screenView(screen)

	// 커서는 고른 행 왼쪽 끝에 둔다. 제목줄이 한 줄을 쓰므로 목록은 그 아래에서 시작한다.
	if len(rows) > 0 {
		view.Cursor = tea.NewCursor(0, m.selected-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// listHeight 는 목록에 쓸 수 있는 행 수다. 제목줄과 키 안내가 편집 영역에서 한 줄씩 가져간다.
//
// 안내는 statusBar 안이 아니라 그 바로 위 줄이다. statusBar 아래 줄은 알림 자리로 비워 둔다 —
// 다른 mode 와 같아서, 취소가 실패하거나 작업이 실패했을 때 안내를 밀어내지 않고 나란히 보인다.
func (e editor) listHeight() int {
	return max(0, e.textHeight()-jobsHintHeight)
}

// jobsHintHeight 는 키 안내가 차지하는 줄 수다.
const jobsHintHeight = 1

// bareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다.
//
// statusBar 는 sidebar 가 열려 있으면 mode 를 그 아래 칸에 넣고 나머지를 32 칸 들여쓴다(ADR-0005).
// 이 화면은 트리를 덮으므로 그대로 두면 목록은 왼쪽 끝에서 시작하는데 statusBar 만 밀려서
// 화면이 반쪽만 바뀐 것처럼 보인다. editor 를 값으로 복사해서 트리만 지운 뒤 그린다.
func (m viewJobs) bareStatusBar() []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	// 접두 키를 기다리는 동안 먹은 키는 normal·트리와 같이 아래 줄 오른쪽 끝에 붙는다.
	// 기다리는 상태가 아직 없어서 지금은 늘 빈 문자열이고 아래 줄이 그대로 나간다.
	// 폭은 트리를 지운 bare 기준이다 — 이 화면은 트리를 덮는다.
	return bare.statusBar("JOBS", bare.withShowcmd(m.message, m.keyState().showcmd()))
}

// jobsTitleHeight 는 제목줄이 차지하는 줄 수다. tabline 과 같은 자리를 쓴다.
const jobsTitleHeight = tablineHeight

// title 은 맨 윗줄이다. 이 화면이 무엇인지와 몇 개가 도는지를 적는다.
//
// tabline 과 같이 반전이다 — 화면 맨 위를 가로지르는 띠라는 것이 편집 화면과 같은 문법이라야
// 화면이 바뀐 것이 아니라 다른 화면으로 온 것으로 읽힌다.
func (m viewJobs) title() string {
	label := fmt.Sprintf("작업  도는 중 %d · 끝난 것 %d", len(m.jobs), len(m.finished))

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// jobRow 는 작업 한 줄이다.
//
//	▸ 파일 인덱싱      ⣿⣿⣿⣿⣄⣀⣀⣀⣀⣀  42%   0:03
//	  파일 인덱싱      끝남  151,933 개         0:12
//
// 도는 것은 막대와 백분율, 끝난 것은 상태와 요약이 같은 칸에 온다. 전체를 모르는 동안에는
// statusBar 와 같이 개수만 찍는다(job.go).
func (m viewJobs) jobRow(running job, selected bool, now time.Time) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	state := running.label()
	if running.finished.IsZero() {
		if running.total > 0 {
			state = fmt.Sprintf("%s  %3d%%", drawBar(running.done, running.total), 100*running.done/running.total)
		} else {
			state = formatCount(running.done)
		}
	}

	row := " " + marker + padTo(running.name, jobNameWidth) + " " + state + "   " + formatElapsed(running.elapsed(now))

	// 화면 전체를 쓰므로 편집 영역이 아니라 화면 너비로 자른다.
	return truncateToWidth(row, m.width)
}

// formatElapsed 는 걸린 시간이다. 분과 초만 본다 — 그보다 오래 도는 작업은 아직 없고,
// 있더라도 초 단위로 세는 것이 무의미해지는 자리다.
func formatElapsed(d time.Duration) string {
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// runJobs 는 팔레트의 「작업 목록」이다. `:jobs` 와 같은 길이다.
func runJobs(e *editor) (tea.Model, tea.Cmd) {
	return jobsMode(e)
}
