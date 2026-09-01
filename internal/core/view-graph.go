package core

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// graphChunk 는 한 번에 읽는 커밋 수다.
//
// 저장소가 수만 커밋인 것이 드물지 않아서 다 읽고 여는 길은 없다. 이만큼이면 화면 예순 번쯤
// 훑을 분량이고, 끝이 가까워지면 다음 조각을 읽는다.
const graphChunk = 1000

// graphPrefetch 는 끝에서 이만큼 안으로 들어오면 다음 조각을 읽는다는 값이다.
//
// 한 화면이 서른 커밋 남짓이라 화면 서너 개 앞에서 읽기 시작한다. `G` 로 끝까지 뛰어도
// 잠깐이면 이어진다.
const graphPrefetch = 100

// graphRowsPerCommit 은 커밋 하나가 쓰는 화면 행 수다. `git graph` 와 같이 두 줄이다.
const graphRowsPerCommit = 2

// graphJobName 은 커밋을 읽는 작업의 이름이자 신원이다. 같은 이름은 한 번에 하나만 돈다(job.go).
const graphJobName = "커밋 읽기"

// graphState 는 커밋 기록 화면이 담은 것이다.
//
// **훑는 자리를 job 에게 넘겼다 돌려받는다.** 시작하는 쪽이 `walk` 를 떼어 goroutine 에 실어
// 보내고(그 사이 여기는 nil 이다) 끝나면 apply 가 되돌려 놓는다. 그래서 훑기를 두 곳에서
// 동시에 만지는 자리가 없고 잠금이 필요 없다 — job.go 가 apply 를 Update 안에서 부르는
// 약속을 그대로 쓴 것이다(ADR-0115).
type graphState struct {
	rows    []graphRow
	walk    *graphWalk // 읽는 중이면 nil 이다. job 이 들고 갔다
	reading bool
	done    bool // 더 읽을 커밋이 없다
}

// graphMode 는 `:graph` 와 팔레트의 「커밋 기록」이 여는 화면이다. mode 는 `GRAPH` 다.
//
// `:jobs`·`:messages` 처럼 화면을 통째로 쓴다. 팔레트처럼 얹지 않는 것은 커밋 하나가 두 줄이라
// 상자에 담으면 대여섯 개밖에 못 보이기 때문이고, 하단 drawer 로 내리지 않은 것은 이 화면이
// 지금 보고 있는 파일에 딸린 것이 아니라서다 — 편집 화면이 같이 보일 값이 없다.
//
// **볼 파일이 없어도 연다.** 담는 것이 저장소에서 오지 지금 buffer 에서 오지 않는다.
// 여러 파일 검색이 같은 까닭으로 `refuseNoBuffer` 를 지나지 않는다(ADR-0078 §6).
//
// **저장소를 여는 것은 여기서 한다.** ref 를 읽는 것뿐이라 값이 없고, 저장소가 아니라는 답을
// 친 자리에서 바로 돌려줄 수 있다. 비싼 것은 커밋을 훑는 쪽이고 그것만 작업으로 나간다.
func graphMode(e *editor) (tea.Model, tea.Cmd) {
	e.clearNotice()

	walk, err := newGraphWalk(".")
	if err != nil {
		return normalModeError(e, err)
	}

	// 열 때마다 처음부터다. 앞서 본 것이 남아 있으면 그 사이의 커밋이 빠진 기록이 된다.
	e.graph = graphState{walk: walk}

	m := viewGraph{editor: e}

	return m, e.startGraphRead()
}

// runGraph 는 팔레트의 「커밋 기록」이다. `:graph` 와 같은 길이다.
func runGraph(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return graphMode(e)
}

// startGraphRead 는 다음 조각을 백그라운드에서 읽는다. 읽을 것이 없으면 아무 일도 하지 않는다.
func (e *editor) startGraphRead() tea.Cmd {
	if e.graph.reading || e.graph.done || e.graph.walk == nil {
		return nil
	}

	// 훑는 자리를 job 에게 넘긴다. 돌아올 때까지 여기는 비어 있다.
	walk := e.graph.walk
	e.graph.walk = nil
	e.graph.reading = true

	// 요약에 적을 「지금까지 읽은 수」다. 작업은 editor 를 읽을 수 없어서 여기서 떠 간다.
	before := len(e.graph.rows)

	return e.startJob(graphJobName, nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress, 1)

		go func() {
			defer close(ch)

			rows, more, err := walk.next(ctx, graphChunk)

			// **끊겼어도 읽은 것과 훑는 자리를 돌려준다.** 버리면 이 화면이 다시는 읽지
			// 못한다 — `reading` 이 선 채로 굳는다. 반쪽이어도 훑기는 이어지는 것이라
			// 다음에 부르면 끊긴 자리부터 간다.
			ch <- jobProgress{
				done:    before + len(rows),
				summary: formatCount(before+len(rows)) + " 개",
				err:     err,
				apply: func(e *editor) {
					e.graph.walk = walk
					e.graph.reading = false
					e.graph.rows = append(e.graph.rows, rows...)
					e.graph.done = !more
				},
			}
		}()

		return ch
	})
}

// prefetch 는 목록의 끝이 가까우면 다음 조각을 읽는다.
//
// **자리를 보고 읽는다.** 조각이 도착할 때마다 다음 것을 걸면 여는 순간부터 저장소를 통째로
// 읽게 되어, 조금씩 읽기로 한 뜻이 사라진다(ADR-0115).
func (m viewGraph) prefetch() tea.Cmd {
	if m.selected < len(m.graph.rows)-graphPrefetch {
		return nil
	}

	return m.startGraphRead()
}

// viewGraph 는 커밋 기록 목록이다.
//
// 담긴 것에서 하나를 고르는 판이라 미리보기가 없다. `enter` 가 상세 화면을 열고 `q` 로
// 그 화면에서 돌아오면 고른 자리가 그대로다(view-commit.go).
type viewGraph struct {
	*editor

	selected int // graph.rows 안의 자리
	top      int // 화면 첫 커밋
}

func (m viewGraph) Init() tea.Cmd { return nil }

func (m viewGraph) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		m.scrollTo()

		return m, m.prefetch()
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 조각이 오면 목록이 그대로 자란다. 고른 자리는 건드리지 않는다 —
		// 훑는 도중에 커서가 튀면 보던 자리를 잃는다.
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

// press 는 키 하나를 먹는다. 한글은 두벌식 자리의 영문 키로 되돌린다 —
// 이 화면의 키가 전부 글자 키라 되돌리지 않으면 아무것도 먹지 않는다(ADR-0008).
func (m viewGraph) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewGraph)
		if !ok {
			// 이미 이 화면을 벗어났다. 남은 자모는 버린다.
			return model, nil
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, cmd
		}

		model = next
	}

	// 이 화면에 그대로 있으면 끝이 가까운지 보고 다음 조각을 읽는다. **키마다가 아니라 여기
	// 한 번이다** — 자모 하나가 키 여럿으로 풀리므로(`ㅗ` 가 `h` 다) 안쪽에서 부르면 같은
	// 작업을 여러 번 시작하려 든다.
	if here, ok := model.(viewGraph); ok {
		return model, here.prefetch()
	}

	return model, nil
}

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
//
// 기록을 고치는 키는 없다. 읽고 고르는 화면이다.
func (m viewGraph) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		// **담아 둔 것을 여기서 놓는다.** 어차피 다시 열 때 처음부터 읽으므로 들고 있을 값이
		// 없고, 깊이 훑어 내려간 뒤 닫으면 수만 커밋이 그대로 남는다.
		m.graph = graphState{}

		return normalMode(m.editor)
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓰되 커밋 수로 센다(ADR-0076).
		m.move(pageRows(pageFull, m.graphRows()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.graphRows()))

		return m, nil
	case "g", "home":
		m.move(-len(m.graph.rows))

		return m, nil
	case "G", "end":
		// **지금 담긴 것의 끝이다.** 다 읽지 않았으면 거기서 다음 조각이 이어 붙는다 —
		// 저장소의 첫 커밋이 어디인지는 다 읽어야 알 수 있고, 그것을 기다리게 하지 않는다.
		m.move(len(m.graph.rows))

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 `enter` 다. 고른 커밋의 상세 화면을 연다.
//
// 이 화면을 그대로 넘겨준다. 돌아올 때 고른 자리와 스크롤이 남는 것이 그래서다(view-commit.go).
func (m viewGraph) open() (tea.Model, tea.Cmd) {
	if len(m.graph.rows) == 0 {
		return m, nil
	}

	return commitMode(m, m.editor, m.graph.rows[m.selected].commit)
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다.
func (m *viewGraph) move(delta int) {
	if len(m.graph.rows) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(m.graph.rows)-1)
	m.scrollTo()
}

// graphRows 는 목록에 보일 수 있는 커밋 수다. 커밋 하나가 두 행이라 절반이다.
//
// **반쪽 커밋은 그리지 않는다.** 화면 행이 홀수면 마지막 한 행이 남는데, 거기에 커밋의 첫
// 줄만 그리면 그 커밋이 목록에 있는지 없는지가 갈리지 않는다.
func (m viewGraph) graphRows() int {
	return max(m.listHeight()/graphRowsPerCommit, 1)
}

// scrollTo 는 고른 커밋이 보이도록 첫 커밋을 최소한으로 움직인다.
func (m *viewGraph) scrollTo() {
	rows, height := len(m.graph.rows), m.graphRows()
	if rows == 0 {
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

func (m viewGraph) View() tea.View {
	height := m.graphRows()
	shown := m.graph.rows[m.top:min(m.top+height, len(m.graph.rows))]

	body := make([]string, 0, height*graphRowsPerCommit)
	for i, row := range shown {
		body = append(body, m.renderCommit(row, m.top+i == m.selected)...)
	}

	if len(m.graph.rows) == 0 && height > 0 {
		body = append(body, " "+m.emptyLabel())
	}

	for len(body) < m.listHeight() {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(" j/k 이동  enter 상세  q 닫기"))
	screen = append(screen, m.bareStatusBar("GRAPH", m.notice)...)

	view := newView(screen, m.renderWindowTitle())

	// 커서는 고른 커밋의 첫 행 왼쪽 끝이다. 제목줄이 한 행을 쓰므로 그 아래에서 센다.
	if len(m.graph.rows) > 0 {
		view.Cursor = tea.NewCursor(0, (m.selected-m.top)*graphRowsPerCommit+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// emptyLabel 은 담긴 것이 없을 때 목록 자리에 적는 한 줄이다.
func (m viewGraph) emptyLabel() string {
	if m.graph.reading {
		return "커밋을 읽는 중입니다"
	}

	return "커밋이 없습니다"
}

// renderTitle 은 맨 윗줄이다. tabline 과 같이 반전이라 「다른 화면으로 왔다」로 읽힌다.
//
// **branch 는 적지 않는다.** statusBar 오른쪽이 이미 들고 있어서, 여기 또 적으면 어느 쪽을
// 보아야 하는지가 갈린다(ADR-0079).
func (m viewGraph) renderTitle() string {
	label := "커밋  " + formatCount(len(m.graph.rows)) + " 개"
	if !m.graph.done {
		label += " · 더 읽는 중"
	}

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// renderCommit 은 커밋 하나의 두 행이다. `git graph` 의 두 줄 형식 그대로다.
//
//	▸ ● ca5e330 - Tue, 1 Sep 2026 11:26:05 +0900 (3 시간 전) (HEAD → master)
//	  │           feature: 명령줄의 `%` 를 지금 보고 있는 파일로 편다 - BlueMir
//
// 아랫줄의 들여쓰기는 짧은 해시와 ` - ` 를 합친 만큼이라 제목이 날짜 아래에 선다.
//
// 고른 커밋은 **두 행 다** 반전하고 첫 행에 `▸` 가 붙는다. 한 항목이 두 행이라 한 행만
// 칠하면 어디까지가 한 커밋인지가 갈리지 않는다.
//
// **고른 줄에는 색을 얹지 않는다.** 색을 켜고 끄는 escape 가 안에 있으면 그 자리에서 반전이
// 끊겨 줄이 얼룩덜룩해진다. 고른 줄이 무엇인지는 반전 하나로 이미 다 말한다.
func (m viewGraph) renderCommit(row graphRow, selected bool) []string {
	marker, blank := "  ", "  "
	if selected {
		marker = "▸ "
	}

	// paint 는 고르지 않은 줄에만 색을 입힌다.
	paint := func(text string, style lipgloss.Style) string {
		if selected || text == "" {
			return text
		}

		return style.Render(text)
	}

	commit, lanes := row.commit, graphLanes(row)

	head := paint(commit.short, styleCommitHash) +
		" - " + paint(commit.when.Format(graphDateFormat), styleCommitDate) +
		" (" + paint(relativeTime(commit.when, time.Now()), styleCommitAge) + ")" +
		paint(renderRefs(commit.refs), styleCommitRefs)

	subject := strings.Repeat(" ", len(commit.short)+3) + commit.subject
	if commit.author != "" {
		subject += " - " + paint(commit.author, styleDetail)
	}

	// 그래프 칸은 열마다 사이 칸을 이미 들고 있어서(graphCellsPerLane) 따로 띄우지 않는다.
	lines := []string{
		marker + renderGraphMark(m.boxChars, row, lanes) + head,
		blank + renderGraphLink(m.boxChars, row, lanes) + subject,
	}

	for i, line := range lines {
		line = truncateToWidth(line, m.width)
		if selected {
			line = reverse.Render(padTo(line, m.width))
		}

		lines[i] = line
	}

	return lines
}

// graphDateFormat 은 날짜 형식이다. `git log` 의 `%aD`(RFC 2822) 와 같다.
const graphDateFormat = "Mon, 2 Jan 2006 15:04:05 -0700"

// renderRefs 는 커밋에 붙은 이름들이다. 없으면 빈 문자열이라 앞의 괄호도 나오지 않는다.
func renderRefs(refs []string) string {
	if len(refs) == 0 {
		return ""
	}

	return " (" + strings.Join(refs, ", ") + ")"
}
