package core

import (
	"fmt"
	"strings"
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
	return viewJobs{editor: e, expanded: map[string]bool{}}, nil
}

type viewJobs struct {
	*editor

	selected int // rows 안의 자리
	top      int // 화면 첫 행

	// expanded 는 펼쳐 둔 이름들이다. 없는 이름은 접힌 것이라 열자마자는 전부 접혀 있다.
	//
	// editor 가 아니라 여기 산다 — 화면을 나가면 잊는 것이 맞다. 다시 열었을 때 접힘이
	// 남아 있으면, 그 사이에 끝나고 사라진 작업 때문에 펼쳐 둔 자리가 빈 채로 선다(ADR-0002).
	expanded map[string]bool

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
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 진행이 오면 목록이 그대로 자란다. 고른 자리는 유지하고 범위만 맞춘다 —
		// 작업 하나가 끝날 때마다 커서가 튀면 취소하려던 것을 놓친다.
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}
		m.scrollTo()

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 차례로 실행한다.
// normal·트리와 같은 나눔이고, 도중에 목록을 벗어나면 남은 동작은 버린다(ADR-0008).
func (m viewJobs) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

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
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
		m.move(pageRows(pageFull, m.listHeight()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.listHeight()))

		return m, nil
	case "home":
		// 이 판에는 `g`·`G` 가 아직 없어서 처음·끝으로 가는 키가 이 둘뿐이다.
		m.selected = 0
		m.scrollTo()

		return m, nil
	case "end":
		m.selected = len(m.jobRows()) - 1
		m.scrollTo()

		return m, nil
	case "enter":
		m.toggleSelected()

		return m, nil
	case "x":
		m.cancelSelected()

		return m, nil
	default:
		return m, nil
	}
}

// cancelSelected 는 고른 작업에게 그만하라고 말한다.
//
// 이름 줄에서 누르면 그 이름으로 도는 것을 **전부** 끊는다. 접혀 있으면 무엇이 도는지 화면에
// 없지만, 접힌 이름 줄을 골라 누른 손이 가리키는 것은 그 이름의 일 전부다 — 하나만 골라
// 끊으면 어느 것이 끊겼는지 알 길이 없다.
//
// 끝난 작업을 고르고 눌렀으면 알리고 만다. 아무 일도 안 나면 키가 먹었는지 알 수 없다.
//
// 끊는 것은 ctx 를 취소하는 일이라 그 자리에서 끝난다 — 시작되는 작업이 없어서 Cmd 가 없다.
func (m *viewJobs) cancelSelected() {
	rows := m.jobRows()
	if m.selected >= len(rows) {
		return
	}

	selected := rows[m.selected]
	if selected.kind != jobRowParent {
		if selected.job.cancel == nil {
			m.notify("이미 끝난 작업입니다")

			return
		}

		m.cancelJob(selected.job.name, selected.job.args)

		return
	}

	stopped := 0
	for _, member := range selected.group.members {
		if member.cancel == nil {
			continue
		}

		m.cancelJob(member.name, member.args)
		stopped++
	}

	if stopped == 0 {
		m.notify("이미 끝난 작업입니다")
	}
}

// toggleSelected 는 이름 줄을 접거나 편다. 이름 줄이 아니면 아무 일도 하지 않는다.
//
// 접어도 고른 자리는 그대로다 — 자식은 이름 줄 뒤에 오므로 앞의 행 수가 바뀌지 않는다.
func (m *viewJobs) toggleSelected() {
	rows := m.jobRows()
	if m.selected >= len(rows) || rows[m.selected].kind != jobRowParent {
		return
	}

	name := rows[m.selected].group.name
	m.expanded[name] = !m.expanded[name]
	m.scrollTo()
}

// jobGroup 은 이름이 같은 작업들이다. 목록은 이것을 이름 줄 하나와 자식 여럿으로 편다.
//
// 도는 것이 앞, 끝난 것이 뒤다 — 목록 전체가 지키던 그 순서가 이름 안으로 들어왔다.
// 끝난 것은 이름당 하나뿐이라(job.go 의 finishJob) 뒤에 붙는 것은 늘 하나 아니면 없다.
type jobGroup struct {
	name    string
	members []job
}

// rollup 은 이름 줄이 찍을 알맹이다. 자식의 done·total 을 더하고 가장 이른 시작을 든다.
//
// **하나라도 전체를 모르면 total 은 0 이다.** 아는 것만 더한 백분율은 거짓말이 된다 —
// renderJobBar 가 전체를 모를 때 막대를 아예 안 그리는 것과 같은 기준이다(job.go).
//
// 하나라도 돌고 있으면 끝난 것이 아니라서 finished 를 비운다. 다 끝났으면 마지막에 끝난 것의
// 상태와 요약을 그대로 든다 — 끝난 것은 이름당 하나라 고를 것이 없다.
func (g jobGroup) rollup() job {
	rolled := job{name: g.name}

	known, running := true, false
	for i, member := range g.members {
		rolled.done += member.done
		rolled.total += member.total
		if member.total <= 0 {
			known = false
		}

		if i == 0 || member.started.Before(rolled.started) {
			rolled.started = member.started
		}

		if member.finished.IsZero() {
			running = true

			continue
		}

		if member.finished.After(rolled.finished) {
			rolled.finished = member.finished
			rolled.summary, rolled.err = member.summary, member.err
		}
	}

	if !known {
		rolled.total = 0
	}
	if running {
		rolled.finished, rolled.summary, rolled.err = time.Time{}, "", nil
	}

	return rolled
}

// jobGroup 은 그 이름으로 도는 것과 끝난 것을 모은 것이다. 도는 것이 앞이다.
func (e editor) jobGroup(name string) jobGroup {
	group := jobGroup{name: name}

	for _, running := range e.jobs {
		if running.name == name {
			group.members = append(group.members, running)
		}
	}
	for _, done := range e.finished {
		if done.name == name {
			group.members = append(group.members, done)
		}
	}

	return group
}

// jobRowKind 는 목록 한 행의 갈래다.
type jobRowKind int

const (
	// jobRowPlain 은 인자 없는 작업 한 줄이다. 접을 것이 없다.
	jobRowPlain jobRowKind = iota
	// jobRowParent 는 이름 줄이다. `+`/`−` 로 접고 편다.
	jobRowParent
	// jobRowChild 는 펼친 이름 줄 아래의 작업 하나다. 이름 자리에 인자가 온다.
	jobRowChild
)

// jobRow 는 목록의 한 행이다. 이름 줄이면 group 이, 그 밖이면 job 이 알맹이다.
type jobRow struct {
	kind  jobRowKind
	group jobGroup
	job   job
}

// jobRows 는 목록에 그릴 행들이다. 도는 것이 위, 끝난 것이 아래다.
//
// **인자가 있는 작업만 이름 줄 아래로 모인다.** 인자가 없으면 모을 것이 없어서 도는 것 위·
// 끝난 것 아래의 차례가 그대로 남는다 — 같은 이름이 둘 서는 일은 인자가 없어도 있다.
// 다시 도는 git 갱신 아래에 지난 결과가 그대로 남아 있는 자리가 그것이다(ADR-0030).
//
// 모으는 자를 개수가 아니라 인자로 삼는다. 개수로 가르면 디렉터리 하나를 읽는 동안 평평하던
// 줄이 둘째가 시작되며 접혀서, 같은 작업이 화면에서 자리를 옮긴다(ADR-0075).
func (m viewJobs) jobRows() []jobRow {
	rows := make([]jobRow, 0, len(m.jobs)+len(m.finished))
	nested := map[string]bool{}

	put := func(member job) {
		if len(member.args) == 0 {
			rows = append(rows, jobRow{kind: jobRowPlain, job: member})

			return
		}

		if nested[member.name] {
			return
		}
		nested[member.name] = true

		group := m.jobGroup(member.name)
		rows = append(rows, jobRow{kind: jobRowParent, group: group})

		if !m.expanded[member.name] {
			return
		}

		for _, child := range group.members {
			rows = append(rows, jobRow{kind: jobRowChild, job: child})
		}
	}

	for _, running := range m.jobs {
		put(running)
	}
	for _, done := range m.finished {
		put(done)
	}

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

// 이름 칸의 폭이다. 이름이 짧아도 진행 막대가 세로로 줄이 맞아야 읽힌다.
//
// jobNameWidth 는 `디렉터리 읽기 (3)` 처럼 개수까지 붙은 이름 줄을 담는다. 자식은 그 안에서
// jobChildIndent 만큼 더 들어가고 인자 칸이 그만큼 좁아진다 — 상태 칸이 어느 갈래에서나
// 같은 열에서 시작한다.
const (
	jobNameWidth   = 18
	jobChildIndent = 2
)

func (m viewJobs) View() tea.View {
	rows := m.jobRows()
	height := m.listHeight()

	now := time.Now()

	body := make([]string, 0, height)
	for i := m.top; i < len(rows) && len(body) < height; i++ {
		body = append(body, m.renderJobRow(rows[i], i == m.selected, now))
	}

	// 도는 것도 끝난 것도 없을 때 빈 화면만 두면 목록을 못 연 것처럼 보인다.
	if len(rows) == 0 && height > 0 {
		body = append(body, " 도는 작업이 없습니다")
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(" j/k 이동  enter 펼치기  x 취소  q 닫기"))
	screen = append(screen, m.renderBareStatusBar()...)

	view := newView(screen)

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

// renderBareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다.
//
// statusBar 는 sidebar 가 열려 있으면 mode 를 그 아래 칸에 넣고 나머지를 32 칸 들여쓴다(ADR-0005).
// 이 화면은 트리를 덮으므로 그대로 두면 목록은 왼쪽 끝에서 시작하는데 statusBar 만 밀려서
// 화면이 반쪽만 바뀐 것처럼 보인다. editor 를 값으로 복사해서 트리만 지운 뒤 그린다.
func (m viewJobs) renderBareStatusBar() []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	// 접두 키를 기다리는 동안 먹은 키는 normal·트리와 같이 아래 줄 오른쪽 끝에 붙는다.
	// 기다리는 상태가 아직 없어서 지금은 늘 빈 문자열이고 아래 줄이 그대로 나간다.
	// 폭은 트리를 지운 bare 기준이다 — 이 화면은 트리를 덮는다.
	return bare.renderStatusBar("JOBS", bare.renderWithShowcmd(m.notice, m.keyState().showcmd()))
}

// jobsTitleHeight 는 제목줄이 차지하는 줄 수다. tabline 과 같은 자리를 쓴다.
const jobsTitleHeight = tablineHeight

// renderTitle 은 맨 윗줄이다. 이 화면이 무엇인지와 몇 개가 도는지를 적는다.
//
// tabline 과 같이 반전이다 — 화면 맨 위를 가로지르는 띠라는 것이 편집 화면과 같은 문법이라야
// 화면이 바뀐 것이 아니라 다른 화면으로 온 것으로 읽힌다.
func (m viewJobs) renderTitle() string {
	label := fmt.Sprintf("작업  도는 중 %d · 끝난 것 %d", len(m.jobs), len(m.finished))

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// renderJobRow 는 목록 한 줄이다.
//
//	▸ − 디렉터리 읽기 (3)  ⣿⣿⣄⣀⣀⣀⣀⣀⣀⣀   30%   0:03
//	      internal/core    ⣿⣿⣄⣀⣀⣀⣀⣀⣀⣀   30%   0:03
//	      docs/adr         끝남  1,200 개         0:00
//	      cmd              ⣿⣿⣿⣿⣿⣿⣄⣀⣀⣀   62%   0:01
//	    파일 인덱싱        ⣿⣿⣿⣄⣀⣀⣀⣀⣀⣀   42%   0:01
//
// 도는 것은 막대와 백분율, 끝난 것은 상태와 요약이 같은 칸에 온다. 전체를 모르는 동안에는
// statusBar 와 같이 개수만 찍는다(job.go). 이름 줄은 자식을 더한 것이다(jobGroup.rollup).
//
// 접힘 표시는 `+`/`−` 다. 트리의 `▸`/`▾` 를 쓰면 고른 줄 표시(`▸ `) 와 글자가 겹쳐서 한 행에
// 같은 화살표가 둘 선다 — 이 화면은 트리와 달리 고른 줄을 반전이 아니라 표시로 가른다(ADR-0075).
func (m viewJobs) renderJobRow(row jobRow, selected bool, now time.Time) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	fold, name, shown := "  ", "", row.job
	switch row.kind {
	case jobRowParent:
		fold = "+ "
		if m.expanded[row.group.name] {
			fold = "− "
		}

		shown = row.group.rollup()
		name = padTo(truncateToWidth(fmt.Sprintf("%s (%d)", row.group.name, len(row.group.members)), jobNameWidth), jobNameWidth)
	case jobRowChild:
		// 인자가 이름 자리에 온다. **왼쪽부터 접는다** — 경로에서 어느 디렉터리인지를 말해
		// 주는 것은 뒤쪽이라, 오른쪽부터 자르면 어느 줄에나 같은 앞머리만 남는다
		// (view-locations.go 와 같은 까닭이다).
		body := jobNameWidth - jobChildIndent
		name = strings.Repeat(" ", jobChildIndent) + padTo(trimLeftToWidth(strings.Join(row.job.args, " "), body), body)
	default:
		name = padTo(truncateToWidth(row.job.name, jobNameWidth), jobNameWidth)
	}

	state := shown.label()
	if shown.finished.IsZero() {
		if shown.total > 0 {
			state = fmt.Sprintf("%s  %3d%%", renderBar(shown.done, shown.total), 100*shown.done/shown.total)
		} else {
			state = formatCount(shown.done)
		}
	}

	line := " " + marker + fold + name + " " + state + "   " + formatElapsed(shown.elapsed(now))

	// 화면 전체를 쓰므로 편집 영역이 아니라 화면 너비로 자른다.
	return truncateToWidth(line, m.width)
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
