package core

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// progressOf 는 진행 msg 하나를 만든다. 채널은 닫혀 있지 않으므로 고리를 잇는 Cmd 를 부르면 막힌다 —
// 고리가 이어졌는지는 Cmd 가 nil 이 아닌 것으로 본다.
func progressOf(name string, done, total int) jobProgressMsg {
	return jobProgressMsg{
		name:        name,
		ch:          make(chan jobProgress),
		jobProgress: jobProgress{done: done, total: total},
	}
}

// 진행이 오면 statusBar 위 줄에 이름과 막대와 백분율이 붙는다.
func TestJobShowsProgressInStatusBar(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 5)

	m, _ = m.Update(progressOf("파일 인덱싱", 42, 100))

	assert.Contains(t, barOf(t, m)[0], "파일 인덱싱 ⣿⣿⣿⣿⣄⣀⣀⣀⣀⣀ 42%")
}

// 전체를 모르면 막대 대신 지금까지 한 개수를 찍는다. 반쯤 찬 막대는 거짓말이 된다.
func TestJobShowsCountWhenTotalUnknown(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 5)

	m, _ = m.Update(progressOf("파일 인덱싱", 12431, 0))

	top := barOf(t, m)[0]
	assert.Contains(t, top, "파일 인덱싱 12,431")
	assert.NotContains(t, top, "⣿")
}

// 막대는 예순 단계다. 점자 한 칸이 여섯 단계이고 열 칸이다.
func TestJobBarSteps(t *testing.T) {
	for _, tt := range []struct {
		done, total int
		want        string
	}{
		{done: 0, total: 60, want: "⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀"},
		{done: 1, total: 60, want: "⣄⣀⣀⣀⣀⣀⣀⣀⣀⣀"},
		{done: 3, total: 60, want: "⣇⣀⣀⣀⣀⣀⣀⣀⣀⣀"},
		{done: 6, total: 60, want: "⣿⣀⣀⣀⣀⣀⣀⣀⣀⣀"},
		{done: 42, total: 100, want: "⣿⣿⣿⣿⣄⣀⣀⣀⣀⣀"},
		{done: 60, total: 60, want: "⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿"},
		// 끝을 넘겨 세도 막대는 꽉 찬 데서 멈춘다.
		{done: 90, total: 60, want: "⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿"},
	} {
		bar := renderBar(tt.done, tt.total)

		assert.Equal(t, tt.want, bar, "%d/%d", tt.done, tt.total)
		assert.Equal(t, jobBarCells, len([]rune(bar)), "폭은 언제나 열 칸이다")
	}
}

// 작업이 여럿이면 맨 앞의 것만 찍고 나머지는 개수로 알린다.
func TestJobShowsRestAsCount(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 5)

	m, _ = m.Update(progressOf("파일 인덱싱", 1, 2))
	m, _ = m.Update(progressOf("여러 파일 검색", 1, 2))

	top := barOf(t, m)[0]
	assert.Contains(t, top, "파일 인덱싱", "먼저 시작한 것이 자리를 지킨다")
	assert.NotContains(t, top, "여러 파일 검색")
	assert.Contains(t, top, "(+1)")
}

// 칸이 모자라면 진행 표시부터 줄인다. 막대를 떼고, 그래도 모자라면 통째로 뺀다.
func TestJobProgressGivesWayWhenNarrow(t *testing.T) {
	// editor 는 하나뿐이라 폭마다 새로 연다. 같은 것을 고쳐 쓰면 앞의 검사가 뒤에 섞인다.
	atWidth := func(width int) string {
		e := newTestEditor("abc\n", width, 5)
		e.git = gitStatus{branch: "master", commit: "a1b2c3d"}

		m, _ := tea.Model(e).Update(progressOf("파일 인덱싱", 42, 100))

		return barOf(t, m)[0]
	}

	assert.Contains(t, atWidth(70), "파일 인덱싱 ⣿⣿⣿⣿⣄⣀⣀⣀⣀⣀ 42%", "넓으면 막대까지 그린다")

	narrow := atWidth(52)
	assert.Contains(t, narrow, "파일 인덱싱 42%")
	assert.NotContains(t, narrow, "⣿", "칸이 모자라면 막대를 뗀다")
	assert.Contains(t, narrow, "master(a1b2c3d)", "git 은 늘 같은 자리다")

	narrower := atWidth(40)
	assert.NotContains(t, narrower, "파일 인덱싱", "더 좁으면 진행 표시를 통째로 뺀다")
	assert.Contains(t, narrower, "master(a1b2c3d)")
	assert.Contains(t, narrower, "test.txt", "경로도 그대로다")
}

// 어느 mode 에 있든 진행을 받고 다음 조각을 받을 Cmd 를 돌려준다.
// mode 를 새로 만들면서 빠뜨리는 것을 여기서 잡는다.
func TestJobProgressReachesEveryMode(t *testing.T) {
	for _, tt := range []struct {
		name string
		open func(t *testing.T) tea.Model
	}{
		{name: "normal", open: func(t *testing.T) tea.Model { return newTestEditor("abc\n", 80, 5) }},
		{name: "insert", open: func(t *testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 5), "i") }},
		{name: "command", open: func(t *testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 5), ":") }},
		{name: "search", open: func(t *testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 5), "/") }},
		{name: "sidebar", open: func(t *testing.T) tea.Model {
			return send(newTreeEditor(t, 80, 5), "ctrl+w", "ctrl+w")
		}},
		{name: "palette", open: func(t *testing.T) tea.Model { return newPaletteView(t, 80, 20, "a.txt") }},
		{name: "확인창", open: func(t *testing.T) tea.Model {
			e := newTestEditor("abc\n", 80, 5)
			e.activeBuffer().insert([]byte("X"), e.contentWidth())

			return ConfirmDiscard(e, e.editor, "정말 종료 하시겠습니까?", Exit)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.open(t)
			require.NotNil(t, m)

			next, cmd := m.Update(progressOf("파일 인덱싱", 1, 2))

			assert.IsType(t, m, next, "진행 msg 로 mode 가 바뀌지 않는다")
			assert.NotNil(t, cmd, "다음 조각을 받을 Cmd 로 고리를 잇는다")
		})
	}
}

// 확인창이 떠 있는 동안 온 진행도 그대로 남는다. 창은 statusBar 를 그리지 않지만
// editor 는 하나뿐이라, No 로 부모에 돌아가면 그동안의 진행이 보인다 (ADR-0026).
func TestJobSurvivesConfirmDialog(t *testing.T) {
	parent := newTestEditor("abc\n", 80, 5)
	parent.activeBuffer().insert([]byte("X"), parent.contentWidth())

	var confirm tea.Model = ConfirmDiscard(parent, parent.editor, "이 tab 을 닫으시겠습니까?", Exit)

	confirm, cmd := confirm.Update(progressOf("파일 인덱싱", 42, 100))
	require.NotNil(t, cmd, "다음 조각을 받을 고리를 잇는다")

	back := send(confirm, "n", "enter")

	require.IsType(t, viewEditorNormal{}, back)
	assert.Contains(t, barOf(t, back)[0], "파일 인덱싱 ⣿⣿⣿⣿⣄⣀⣀⣀⣀⣀ 42%")
}

// 결과를 어디에 어떻게 넣을지는 작업이 정한다. 실행기는 그것이 무엇인지 알지 못한다.
func TestJobAppliesResultItself(t *testing.T) {
	e := editor{}

	msg := progressOf("파일 인덱싱", 1, 2)
	msg.apply = func(e *editor) { e.files = []string{"a.txt", "b.txt"} }

	next, cmd := e.handleJob(msg)

	assert.Nil(t, next, "진행 조각은 mode 를 바꾸지 않는다")
	require.NotNil(t, cmd, "다음 조각을 받을 고리를 잇는다")
	assert.Equal(t, []string{"a.txt", "b.txt"}, e.files)
}

// 알릴 결과가 아직 없는 조각은 진행만 갱신하고 지나간다.
func TestJobProgressWithoutResult(t *testing.T) {
	e := editor{}

	e.handleJob(progressOf("파일 인덱싱", 7, 0))

	require.Len(t, e.jobs, 1)
	assert.Equal(t, 7, e.jobs[0].done)
	assert.Nil(t, e.files)
}

// 같은 이름은 하나만 돈다. 두 번째 요청은 시작하지 않고 돌고 있는 것에 붙는다.
func TestStartJobKeepsOneOfEachName(t *testing.T) {
	e := editor{}

	started := 0
	start := func(context.Context) <-chan jobProgress {
		started++
		ch := make(chan jobProgress)
		close(ch)

		return ch
	}

	require.NotNil(t, e.startJob("파일 인덱싱", start))
	assert.Nil(t, e.startJob("파일 인덱싱", start), "이미 돌고 있으면 Cmd 도 주지 않는다")

	assert.Equal(t, 1, started, "채널조차 만들지 않는다")
	assert.Len(t, e.jobs, 1)
}

// 목록에 없는 이름의 진행이 와도 끼워 넣는다. 시작한 자리를 지나온 msg 여도 자리를 잡는다.
func TestJobProgressAddsUnknownName(t *testing.T) {
	e := editor{}

	e.handleJob(progressOf("여러 파일 검색", 1, 2))

	require.Len(t, e.jobs, 1)
	assert.Equal(t, "여러 파일 검색", e.jobs[0].name)
}

// 채널이 닫히면 목록에서 빠지고 statusBar 에서 사라진다.
func TestJobDoneRemovesFromStatusBar(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 5)

	m, _ = m.Update(progressOf("파일 인덱싱", 1, 2))
	require.Contains(t, barOf(t, m)[0], "파일 인덱싱")

	m, cmd := m.Update(jobDoneMsg{name: "파일 인덱싱"})

	assert.Nil(t, cmd, "받을 것이 더 없다")
	assert.NotContains(t, barOf(t, m)[0], "파일 인덱싱")
}

// 세 자리마다 쉼표를 넣는다.
func TestFormatCount(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"},
		{n: 7, want: "7"},
		{n: 999, want: "999"},
		{n: 1000, want: "1,000"},
		{n: 12431, want: "12,431"},
		{n: 1234567, want: "1,234,567"},
	} {
		assert.Equal(t, tt.want, formatCount(tt.n))
	}
}

// waitJob 은 채널에서 조각을 받아 msg 로 바꾸고, 닫힌 채널은 끝났다는 msg 가 된다.
func TestWaitJobTurnsChannelIntoMessages(t *testing.T) {
	ch := make(chan jobProgress, 1)
	ch <- jobProgress{done: 3, total: 10}

	msg := waitJob("파일 인덱싱", ch)()

	require.IsType(t, jobProgressMsg{}, msg)
	progress := msg.(jobProgressMsg)
	assert.Equal(t, "파일 인덱싱", progress.name)
	assert.Equal(t, 3, progress.done)
	assert.NotNil(t, progress.ch, "다음 조각을 받을 곳을 같이 들고 온다")

	close(ch)

	assert.Equal(t, jobDoneMsg{name: "파일 인덱싱"}, waitJob("파일 인덱싱", ch)())
}

// 진행 표시가 붙어도 statusBar 는 화면 너비를 넘지 않는다. 넘치면 터미널이 줄바꿈해 화면이 밀린다.
func TestJobProgressKeepsStatusBarWidth(t *testing.T) {
	e := newTestEditor("abc\n", 50, 5)
	e.git = gitStatus{branch: "master", commit: "a1b2c3d", dirty: true}

	var m tea.Model = e
	m, _ = m.Update(progressOf("파일 인덱싱", 42, 100))

	for _, row := range barOf(t, m) {
		assert.LessOrEqual(t, screenColAt([]byte(row), len(row)), 50, strings.TrimSpace(row))
	}
}
