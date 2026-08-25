package core

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// finishOf 는 마지막 조각과 채널이 닫힌 것을 잇달아 먹인다. 작업이 끝나는 길 그대로다.
func finishOf(t *testing.T, m tea.Model, name string, last jobProgress) tea.Model {
	t.Helper()

	msg := jobProgressMsg{name: name, ch: make(chan jobProgress), jobProgress: last}

	next, _ := m.Update(msg)
	next, _ = next.Update(jobDoneMsg{name: name})

	return next
}

// 끝난 작업은 목록에서 사라지지 않고 끝난 것으로 옮겨간다. 요약이 같이 남는다.
func TestFinishedJobKeepsSummary(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = m.Update(progressOf("파일 인덱싱", 1, 2))
	m = finishOf(t, m, "파일 인덱싱", jobProgress{done: 2, total: 2, summary: "151,933 개"})

	e := m.(viewEditorNormal).editor
	assert.Empty(t, e.jobs, "도는 목록에서는 빠진다")
	require.Len(t, e.finished, 1)
	assert.Equal(t, "151,933 개", e.finished[0].summary)
	assert.False(t, e.finished[0].finished.IsZero(), "끝난 시각이 찍힌다")
	assert.Nil(t, e.finished[0].cancel, "끝난 작업에는 취소할 것이 없다")
	assert.NotContains(t, barOf(t, m)[0], "파일 인덱싱", "statusBar 는 도는 것만 본다")
}

// 실패는 목록을 열기 전에도 알아야 한다. statusBar 아래 줄이 알린다.
func TestFailedJobTellsInStatusBar(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = m.Update(progressOf("파일 인덱싱", 1, 2))
	m = finishOf(t, m, "파일 인덱싱", jobProgress{err: errors.New("git 이 없습니다")})

	assert.Contains(t, barOf(t, m)[1], "파일 인덱싱 실패: git 이 없습니다")
	assert.Equal(t, "실패  git 이 없습니다", m.(viewEditorNormal).finished[0].label())
}

// 취소는 알리지 않는다. 그만하라고 한 사람이 결과를 이미 안다.
func TestCancelledJobDoesNotTell(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = m.Update(progressOf("파일 인덱싱", 1, 2))
	m = finishOf(t, m, "파일 인덱싱", jobProgress{err: context.Canceled})

	assert.NotContains(t, barOf(t, m)[1], "실패")
	assert.Equal(t, "취소됨", m.(viewEditorNormal).finished[0].label())
}

// 실패 알림도 다음 키에 사라진다. 명령 결과와 같은 자리이고 같은 규칙이다.
func TestFailedJobMessageClearsOnNextKey(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = m.Update(progressOf("파일 인덱싱", 1, 2))
	m = finishOf(t, m, "파일 인덱싱", jobProgress{err: errors.New("git 이 없습니다")})
	require.Contains(t, barOf(t, m)[1], "실패")

	assert.NotContains(t, barOf(t, send(m, "j"))[1], "실패")
}

// 끝난 목록은 이름당 마지막 결과 하나다. 주기적으로 도는 갱신이 목록을 뒤덮지 않는다(ADR-0030).
func TestFinishedJobsKeepLastPerName(t *testing.T) {
	e := editor{}

	for i := range 25 {
		e.jobs = append(e.jobs, job{name: gitJobName, done: i, started: time.Now()})
		e.finishJob(gitJobName, nil)
	}

	require.Len(t, e.finished, 1)
	assert.Equal(t, 24, e.finished[0].done, "마지막 것만 남는다")
}

// 이름이 다르면 각각 남는다. 최근에 끝난 것이 맨 위다.
func TestFinishedJobsKeepEveryName(t *testing.T) {
	e := editor{}

	for _, name := range []string{"파일 인덱싱", gitJobName, "파일 인덱싱"} {
		e.jobs = append(e.jobs, job{name: name, started: time.Now()})
		e.finishJob(name, nil)
	}

	require.Len(t, e.finished, 2)
	assert.Equal(t, "파일 인덱싱", e.finished[0].name)
	assert.Equal(t, gitJobName, e.finished[1].name)
}

// cancelJob 은 그 작업의 ctx 만 끊는다. 다른 작업은 그대로 돈다.
func TestCancelJobStopsOnlyThatJob(t *testing.T) {
	e := editor{}

	indexed, searched := false, false
	e.jobs = []job{
		{name: "파일 인덱싱", cancel: func() { indexed = true }},
		{name: "여러 파일 검색", cancel: func() { searched = true }},
	}

	e.cancelJob("파일 인덱싱", nil)

	assert.True(t, indexed)
	assert.False(t, searched)
	assert.Len(t, e.jobs, 2, "채널이 닫힐 때까지 목록에 남는다")
}

// 작업은 editor 의 ctx 에서 갈라져 나온다. 편집기를 끝내면 도는 것이 전부 정리된다.
func TestStartJobDerivesFromEditorContext(t *testing.T) {
	root, stop := context.WithCancel(context.Background())
	e := editor{ctx: root}

	var given context.Context
	e.startJob("파일 인덱싱", nil, func(ctx context.Context) <-chan jobProgress {
		given = ctx
		ch := make(chan jobProgress)
		close(ch)

		return ch
	})

	require.NotNil(t, given)
	require.NoError(t, given.Err())

	stop()
	assert.Error(t, given.Err(), "루트가 끊기면 작업도 끊긴다")
}

// 취소하면 작업의 ctx 가 끊긴다. 그 뒤는 작업이 채널을 닫는 것으로 이어진다.
func TestCancelJobCutsTheJobContext(t *testing.T) {
	e := editor{}

	var given context.Context
	e.startJob("파일 인덱싱", nil, func(ctx context.Context) <-chan jobProgress {
		given = ctx

		return make(chan jobProgress)
	})
	require.NoError(t, given.Err())

	e.cancelJob("파일 인덱싱", nil)

	assert.ErrorIs(t, given.Err(), context.Canceled)
}

// 취소된 인덱싱은 채널을 닫고 물러난다. 아무도 받지 않는 채널에 goroutine 이 남으면 안 된다.
func TestIndexFilesStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ch := indexFiles(ctx, newPaletteFixture(t))

	// 취소된 뒤에도 조각이 하나쯤 올 수 있다. 채널이 닫히는 것만 본다.
	for range ch { //nolint:revive // 비우는 것이 목적이다
	}
}

// 조각을 보내다 끊긴 작업은 아무 말 없이 채널만 닫는다. 그래도 목록에는 「취소됨」으로 남아야 한다.
func TestCancelledJobStaysCancelledWhenJobSaysNothing(t *testing.T) {
	e := editor{}

	e.jobs = []job{{name: "파일 인덱싱", cancel: func() {}}}
	e.cancelJob("파일 인덱싱", nil)
	e.finishJob("파일 인덱싱", nil)

	require.Len(t, e.finished, 1)
	assert.Equal(t, "취소됨", e.finished[0].label())
	assert.Empty(t, e.notice, "취소는 알리지 않는다")
}

// 이름이 같아도 인자가 다르면 다른 작업이라 나란히 돈다. 디렉터리 읽기가 그렇다.
func TestJobsWithSameNameRunPerArgs(t *testing.T) {
	e := editor{}

	docs := e.startJob(dirJobName, []string{"docs"}, emptyJob)
	core := e.startJob(dirJobName, []string{"internal/core"}, emptyJob)
	again := e.startJob(dirJobName, []string{"docs"}, emptyJob)

	assert.NotNil(t, docs)
	assert.NotNil(t, core, "인자가 다르면 새로 시작한다")
	assert.Nil(t, again, "인자까지 같으면 돌고 있는 것에 붙는다")
	assert.Len(t, e.jobs, 2)
}

// emptyJob 은 곧바로 닫히는 채널이다. 시작되었는지만 보는 시험이 쓴다.
func emptyJob(context.Context) <-chan jobProgress {
	ch := make(chan jobProgress)
	close(ch)

	return ch
}

// 끝난 목록은 인자가 달라도 이름당 하나다. 펼친 디렉터리마다 한 줄씩 쌓이던 것이 이것으로 끝난다.
//
// 지워지는 것은 어느 경로가 언제 끝났는지인데, 실패는 알림 목록에 통째로 남는다(ADR-0075).
func TestFinishedJobsKeepLastPerNameAcrossArgs(t *testing.T) {
	e := editor{}

	for _, dir := range []string{"docs", "internal/core", "cmd"} {
		e.jobs = append(e.jobs, job{name: dirJobName, args: []string{dir}, started: time.Now()})
		e.finishJob(dirJobName, []string{dir})
	}

	require.Len(t, e.finished, 1)
	assert.Equal(t, []string{"cmd"}, e.finished[0].args, "마지막 것만 남는다")
}

// 실패 알림에는 인자가 같이 간다. 이름만 남기면 어느 디렉터리가 실패했는지 알 길이 없다.
func TestFailedJobNoticeCarriesArgs(t *testing.T) {
	e := editor{}
	e.jobs = append(e.jobs, job{name: dirJobName, args: []string{"internal/core"}, err: errors.New("permission denied")})
	e.finishJob(dirJobName, []string{"internal/core"})

	assert.Equal(t, "디렉터리 읽기 internal/core 실패: permission denied", e.notice)
}
