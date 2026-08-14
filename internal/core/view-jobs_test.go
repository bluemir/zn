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
)

// newJobsView 는 도는 작업과 끝난 작업을 직접 넣은 목록 화면이다.
func newJobsView(t *testing.T, running, finished []job) viewJobs {
	t.Helper()

	e := newTestEditor("a\nb\n", 80, 10).editor
	e.jobs = running
	e.finished = finished

	return viewJobs{editor: e}
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
	assert.Contains(t, rows[0], "▸ 파일 인덱싱", "고른 자리는 첫 줄이다")
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
	palette.input = ">jobs"
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
	assert.True(t, strings.HasPrefix(rows[0], " ▸ 파일 인덱싱"), "행: %q", rows[0])
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

	assert.Contains(t, ansi.Strip(hint), "j/k 이동  x 취소  q 닫기")
	assert.Contains(t, hint, "38;5;244", "흐린 글씨라 목록 내용과 갈린다")
	assert.Empty(t, strings.TrimSpace(barOf(t, m)[1]), "알림이 없으면 아래 줄은 비어 있다")

	// 알림이 뜨면 안내를 밀어내지 않고 그 아래에 나란히 보인다.
	next := send(m, "x")
	assert.Contains(t, barOf(t, next)[1], "이미 끝난 작업입니다")
	assert.Contains(t, ansi.Strip(strings.Split(next.View().Content, "\n")[len(rows)-statusBarHeight-jobsHintHeight]), "q 닫기")
}
