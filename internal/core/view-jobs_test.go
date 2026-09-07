package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// newJobsView 는 도는 작업과 끝난 작업을 직접 넣은 목록 화면이다.
func newJobsView(t *testing.T, running, finished []job) viewJobs {
	t.Helper()

	e := newTestEditor("a\nb\n", 80, 10).editor
	e.jobs = running
	e.finished = finished

	return viewJobs{editor: e, expanded: map[string]bool{}}
}

// jobRowsOf 는 목록 행만 색을 빼고 돌려준다. 제목줄·키 안내·statusBar 는 뺀다.
func jobRowsOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	rows := strings.Split(m.View().Content, "\n")
	require.Greater(t, len(rows), jobsTitleHeight+jobsHintHeight+statusBarHeight)

	body := []string{}
	for _, row := range rows[jobsTitleHeight : len(rows)-statusBarHeight-jobsHintHeight] {
		plain := strings.TrimRight(ansi.Strip(row), " ")
		if plain == "" {
			continue
		}

		body = append(body, plain)
	}

	return body
}

// 도는 작업이 위, 끝난 작업이 아래다. 도는 것은 막대와 백분율, 끝난 것은 상태와 요약이다.
func TestJobsListShowsRunningThenFinished(t *testing.T) {
	started := time.Now().Add(-3 * time.Second)
	m := newJobsView(t,
		[]job{{name: "파일 인덱싱", done: 42, total: 100, started: started, cancel: func() {}}},
		[]job{
			{name: "여러 파일 검색", started: started, finished: time.Now(), summary: "12 개"},
			{name: "파일 인덱싱", started: started, finished: time.Now(), err: errors.New("git 이 없습니다")},
			{name: "여러 파일 치환", started: started, finished: time.Now(), err: context.Canceled},
		})

	rows := jobRowsOf(t, m)

	require.Len(t, rows, 4)
	assert.Contains(t, rows[0], "▸   파일 인덱싱", "고른 자리는 첫 줄이다")
	assert.Contains(t, rows[0], "⣿⣿⣿⣿⣄⣀⣀⣀⣀⣀")
	assert.Contains(t, rows[0], "42%")
	assert.Contains(t, rows[1], "끝남  12 개")
	assert.Contains(t, rows[2], "실패  git 이 없습니다")
	assert.Contains(t, rows[3], "취소됨")
}

// 전체를 모르는 동안에는 statusBar 와 같이 개수만 찍는다.
func TestJobsListShowsCountWhenTotalUnknown(t *testing.T) {
	m := newJobsView(t, []job{{name: "파일 인덱싱", done: 12431, started: time.Now(), cancel: func() {}}}, nil)

	rows := jobRowsOf(t, m)

	require.NotEmpty(t, rows)
	assert.Contains(t, rows[0], "12,431")
	assert.NotContains(t, rows[0], "⣿")
}

// 도는 것도 끝난 것도 없으면 빈 화면 대신 그렇다고 적는다.
func TestJobsListTellsWhenEmpty(t *testing.T) {
	m := newJobsView(t, nil, nil)

	rows := jobRowsOf(t, m)

	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "도는 작업이 없습니다")
}

// `x` 는 고른 작업에게 그만하라고 말한다. 목록에서 바로 빼지는 않는다 — 끝나는 시점은 작업이 안다.
func TestJobsListCancelsSelected(t *testing.T) {
	cancelled := false
	m := newJobsView(t,
		[]job{{name: "파일 인덱싱", done: 1, total: 2, started: time.Now(), cancel: func() { cancelled = true }}}, nil)

	next := send(m, "x")

	require.IsType(t, viewJobs{}, next)
	assert.True(t, cancelled)
	assert.Len(t, next.(viewJobs).jobs, 1, "채널이 닫힐 때까지 목록에 남는다")
}

// 끝난 작업에는 취소할 것이 없다. 아무 일도 안 나면 키가 먹었는지 알 수 없으므로 알린다.
func TestJobsListTellsWhenAlreadyFinished(t *testing.T) {
	m := newJobsView(t, nil, []job{{name: "파일 인덱싱", finished: time.Now(), summary: "3 개"}})

	next := send(m, "x")

	assert.Contains(t, barOf(t, next)[1], "이미 끝난 작업입니다")
}

// j/k 로 오르내리고 양끝에서 멈춘다. 화살표도 같다.
func TestJobsListMovesSelection(t *testing.T) {
	m := newJobsView(t, nil, []job{
		{name: "하나", finished: time.Now()},
		{name: "둘", finished: time.Now()},
	})

	var next tea.Model = send(m, "k")
	assert.Zero(t, next.(viewJobs).selected, "위 끝에서 멈춘다")

	next = send(m, "j", "j", "j")
	assert.Equal(t, 1, next.(viewJobs).selected, "아래 끝에서 멈춘다")

	next = send(m, "down", "up")
	assert.Zero(t, next.(viewJobs).selected, "화살표도 같다")
}

// 목록이 화면보다 길면 내려갈 때 화면이 따라온다.
func TestJobsListScrolls(t *testing.T) {
	finished := make([]job, 0, 30)
	for i := range 30 {
		finished = append(finished, job{name: "작업", done: i, finished: time.Now(), summary: "끝"})
	}

	m := newJobsView(t, nil, finished)
	height := m.listHeight()

	var next tea.Model = m
	for range height + 2 {
		next = send(next, "j")
	}

	jobs := next.(viewJobs)
	assert.Equal(t, height+2, jobs.selected)
	assert.Equal(t, jobs.selected-height+1, jobs.top, "고른 자리가 마지막 행에 오도록만 민다")
	assert.Len(t, jobRowsOf(t, next), height)
}

// 목록이 떠 있는 동안에도 진행이 들어오고, 고른 자리는 그대로다.
func TestJobsListTakesProgress(t *testing.T) {
	m := newJobsView(t, nil, []job{{name: "지난 작업", finished: time.Now(), summary: "끝"}})

	var next tea.Model = send(m, "j")
	require.Zero(t, next.(viewJobs).selected, "끝난 것 하나뿐이라 그대로다")

	next, cmd := next.Update(progressOf("파일 인덱싱", 1, 2))

	require.NotNil(t, cmd, "다음 조각을 받을 고리를 잇는다")
	require.IsType(t, viewJobs{}, next)
	assert.Len(t, next.(viewJobs).jobRows(), 2, "새 작업이 목록에 들어온다")
}

// `:jobs` 와 팔레트 「작업 목록」이 같은 화면을 연다. q·Esc 로 돌아온다.
func TestJobsOpensFromCommandAndPalette(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m = typeInto(m, ":jobs")
	m = send(m, "enter")
	require.IsType(t, viewJobs{}, m)
	assert.Contains(t, barOf(t, m)[0], "JOBS")

	m = send(m, "q")
	assert.IsType(t, viewEditorNormal{}, m)

	palette := newPaletteView(t, 80, 20, "a.txt")
	palette.input = newInputLine(">jobs")
	palette.filter()

	opened := send(palette, "enter")
	assert.IsType(t, viewJobs{}, opened)

	assert.IsType(t, viewEditorNormal{}, send(opened, "esc"))
}

// 한글 상태에서도 목록 키가 먹는다. `ㅂ` 가 `q` 다 (ADR-0008).
func TestJobsListTakesHangulKeys(t *testing.T) {
	m := newJobsView(t, nil, []job{{name: "작업", finished: time.Now()}})

	assert.IsType(t, viewEditorNormal{}, send(m, "ㅂ"))
}

// 편집기 틀을 쓰지 않는다. tabline 도 sidebar 도 그리지 않고 맨 위는 제목줄이다 —
// 편집 화면처럼 보이면 여기서 치는 키가 편집기 키인 줄 알게 된다.
func TestJobsListCoversEditorChrome(t *testing.T) {
	tree := newTreeEditor(t, 80, 10)
	tree.jobs = []job{{name: "파일 인덱싱", done: 1, total: 2, started: time.Now(), cancel: func() {}}}
	require.True(t, tree.sidebarVisible(), "편집 화면에서는 트리가 보인다")

	editing := ansi.Strip(tree.View().Content)
	require.Contains(t, editing, "docs", "편집 화면에는 트리가 있다")
	require.Contains(t, editing, "main.go", "편집 화면에는 tabline 이 있다")

	opened := send(typeInto(tree, ":jobs"), "enter")
	require.IsType(t, viewJobs{}, opened)

	jobs := ansi.Strip(opened.View().Content)

	rows := strings.Split(jobs, "\n")
	assert.Contains(t, rows[0], "작업  도는 중 1", "맨 위는 제목줄이다")
	assert.NotContains(t, jobs, "docs", "트리를 덮는다")
	assert.Contains(t, rows[len(rows)-statusBarHeight], "JOBS", "statusBar 는 남는다")
}

// 목록은 화면 전체 너비를 쓴다. sidebar 자리만큼 밀려 있으면 트리가 걷힌 것처럼 보이지 않는다.
func TestJobsListStartsAtLeftEdge(t *testing.T) {
	tree := newTreeEditor(t, 80, 10)
	tree.jobs = []job{{name: "파일 인덱싱", done: 1, total: 2, started: time.Now(), cancel: func() {}}}

	rows := jobRowsOf(t, jobsView(t, tree.editor))

	require.NotEmpty(t, rows)
	// 고른 줄 표시 뒤의 두 칸은 접힘 표시 자리다. 접을 것이 없는 줄도 그 칸을 비워 두어야
	// 이름 줄과 막대가 세로로 줄이 맞는다(view-jobs.go 의 renderJobRow).
	assert.True(t, strings.HasPrefix(rows[0], " ▸   파일 인덱싱"), "행: %q", rows[0])
}

// jobsView 는 그 editor 로 연 목록 화면이다.
func jobsView(t *testing.T, e *editor) viewJobs {
	t.Helper()

	m, _ := jobsMode(e)
	require.IsType(t, viewJobs{}, m)

	return m.(viewJobs)
}

// 키 안내는 statusBar 안이 아니라 그 바로 위 줄이다. 아래 줄은 알림 자리로 비워 둔다.
func TestJobsHintSitsAboveStatusBar(t *testing.T) {
	m := newJobsView(t, nil, []job{{name: "파일 인덱싱", finished: time.Now(), summary: "3 개"}})

	rows := strings.Split(m.View().Content, "\n")
	hint := rows[len(rows)-statusBarHeight-jobsHintHeight]

	assert.Contains(t, ansi.Strip(hint), "j/k 이동  enter 펼치기  x 취소  X 강제 종료  q 닫기")
	assert.Contains(t, hint, "38;5;244", "흐린 글씨라 목록 내용과 갈린다")
	assert.Empty(t, strings.TrimSpace(barOf(t, m)[1]), "알림이 없으면 아래 줄은 비어 있다")

	// 알림이 뜨면 안내를 밀어내지 않고 그 아래에 나란히 보인다.
	next := send(m, "x")
	assert.Contains(t, barOf(t, next)[1], "이미 끝난 작업입니다")
	assert.Contains(t, ansi.Strip(strings.Split(next.View().Content, "\n")[len(rows)-statusBarHeight-jobsHintHeight]), "q 닫기")
}

// 인자가 있는 작업은 이름 줄 하나로 모이고 열자마자는 접혀 있다. 이름 옆에 몇인지가 붙는다.
func TestJobsListNestsJobsWithArgs(t *testing.T) {
	started := time.Now().Add(-3 * time.Second)
	m := newJobsView(t, []job{
		{name: dirJobName, args: []string{"docs"}, done: 1, total: 4, started: started, cancel: func() {}},
		{name: dirJobName, args: []string{"internal/core"}, done: 2, total: 4, started: started, cancel: func() {}},
	}, nil)

	rows := jobRowsOf(t, m)

	require.Len(t, rows, 1, "이름 줄 하나로 모인다")
	assert.Contains(t, rows[0], "+ 디렉터리 읽기 (2)")
	assert.NotContains(t, rows[0], "docs", "접혀 있으면 인자는 보이지 않는다")
}

// enter 로 펼치면 인자가 한 줄씩 선다. 다시 치면 접힌다.
func TestJobsListExpandsOnEnter(t *testing.T) {
	m := newJobsView(t, []job{
		{name: dirJobName, args: []string{"docs"}, done: 1, total: 4, started: time.Now(), cancel: func() {}},
		{name: dirJobName, args: []string{"internal/core"}, done: 2, total: 4, started: time.Now(), cancel: func() {}},
	}, nil)

	opened := send(m, "enter")
	rows := jobRowsOf(t, opened)

	require.Len(t, rows, 3)
	assert.Contains(t, rows[0], "− 디렉터리 읽기 (2)")
	assert.Contains(t, rows[1], "docs")
	assert.Contains(t, rows[2], "internal/core")

	assert.Len(t, jobRowsOf(t, send(opened, "enter")), 1, "다시 치면 접힌다")
}

// 이름 줄의 막대는 자식을 더한 것이다. 하나라도 전체를 모르면 막대를 그리지 않는다.
func TestJobsListParentSumsChildren(t *testing.T) {
	summed := newJobsView(t, []job{
		{name: dirJobName, args: []string{"docs"}, done: 1, total: 4, started: time.Now(), cancel: func() {}},
		{name: dirJobName, args: []string{"cmd"}, done: 2, total: 4, started: time.Now(), cancel: func() {}},
	}, nil)

	assert.Contains(t, jobRowsOf(t, summed)[0], "37%", "3/8 이다")

	unknown := newJobsView(t, []job{
		{name: dirJobName, args: []string{"docs"}, done: 1, total: 4, started: time.Now(), cancel: func() {}},
		{name: dirJobName, args: []string{"cmd"}, done: 2, started: time.Now(), cancel: func() {}},
	}, nil)

	row := jobRowsOf(t, unknown)[0]
	assert.NotContains(t, row, "⣿", "아는 것만 더한 백분율은 거짓말이 된다")
	assert.Contains(t, row, "3")
}

// 다 끝났으면 이름 줄이 마지막에 끝난 것의 상태와 요약을 든다.
func TestJobsListParentShowsFinishedLabel(t *testing.T) {
	m := newJobsView(t, nil,
		[]job{{name: dirJobName, args: []string{"docs"}, started: time.Now(), finished: time.Now(), summary: "1,200 개"}})

	assert.Contains(t, jobRowsOf(t, m)[0], "끝남  1,200 개")
}

// 이름 줄에서 `x` 는 그 이름으로 도는 것을 전부 끊는다. 하나만 끊으면 어느 것인지 알 수 없다.
func TestJobsListCancelsWholeGroup(t *testing.T) {
	stopped := []string{}
	m := newJobsView(t, []job{
		{name: dirJobName, args: []string{"docs"}, started: time.Now(), cancel: func() { stopped = append(stopped, "docs") }},
		{name: dirJobName, args: []string{"cmd"}, started: time.Now(), cancel: func() { stopped = append(stopped, "cmd") }},
	}, nil)

	send(m, "x")

	assert.Equal(t, []string{"docs", "cmd"}, stopped)
}

// 펼친 자식에서 `x` 는 그것 하나만 끊는다.
func TestJobsListCancelsOneChild(t *testing.T) {
	stopped := []string{}
	m := newJobsView(t, []job{
		{name: dirJobName, args: []string{"docs"}, started: time.Now(), cancel: func() { stopped = append(stopped, "docs") }},
		{name: dirJobName, args: []string{"cmd"}, started: time.Now(), cancel: func() { stopped = append(stopped, "cmd") }},
	}, nil)

	send(send(m, "enter"), "j", "j", "x")

	assert.Equal(t, []string{"cmd"}, stopped)
}

// 인자가 없으면 모을 것이 없다. 같은 이름이 도는 중과 끝난 것으로 둘 서는 자리가 그대로 남는다.
func TestJobsListKeepsFlatRowsWithoutArgs(t *testing.T) {
	started := time.Now()
	m := newJobsView(t,
		[]job{{name: gitJobName, done: 1, total: 2, started: started, cancel: func() {}}},
		[]job{{name: gitJobName, started: started, finished: time.Now(), summary: "8 개"}})

	rows := jobRowsOf(t, m)

	require.Len(t, rows, 2, "접히지 않는다")
	assert.NotContains(t, rows[0], "+", "접힘 표시가 없다")
	assert.Contains(t, rows[1], "끝남  8 개")
}

// 이름이 길어도 상태 칸을 밀지 않는다. 경로가 이름에 붙어 있던 때가 이것으로 끝난다(ADR-0075).
func TestJobsListKeepsStateColumnAligned(t *testing.T) {
	m := newJobsView(t, []job{
		{name: dirJobName, args: []string{"internal/core/very/deep/path"}, done: 1, total: 2, started: time.Now(), cancel: func() {}},
		{name: "파일 인덱싱", done: 1, total: 2, started: time.Now(), cancel: func() {}},
	}, nil)

	rows := jobRowsOf(t, send(m, "enter"))

	require.Len(t, rows, 3)
	for _, row := range rows {
		// 칸은 byte 자리가 아니라 화면 폭으로 잰다 — 한글이 한 글자에 세 byte 다.
		assert.Equal(t, barColumnOf(rows[0]), barColumnOf(row), "행: %q", row)
	}
	assert.Contains(t, rows[1], "…", "긴 경로는 왼쪽부터 접는다")
	assert.Contains(t, rows[1], "deep/path", "뒤쪽이 남는다")
}

// barColumnOf 는 막대가 시작하는 화면 칸이다.
func barColumnOf(row string) int {
	return textarea.WidthOf(row[:strings.Index(row, "⣿")])
}
