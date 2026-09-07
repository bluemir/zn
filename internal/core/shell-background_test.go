package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectBackgroundShell 은 셸 한 줄을 끝까지 돌려 조각을 다 받는다.
//
// 셸을 `/bin/sh` 로 못 박는다. `$SHELL` 이 사람마다 달라서 문법이 갈리면 시험이 그 셸을
// 검사하게 된다(shell_test.go 와 같은 손).
func collectBackgroundShell(t *testing.T, ctx context.Context, line string) []jobProgress {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")

	output := &shellOutput{}
	cmd := newBackgroundShell(ctx, line, output)

	require.NoError(t, cmd.Start())

	got := []jobProgress{}
	for progress := range runBackgroundShell(ctx, cmd, output) {
		got = append(got, progress)
	}

	return got
}

// lastBackgroundShell 은 마지막 조각이다. summary·err 은 거기만 찬다(job.go).
func lastBackgroundShell(t *testing.T, line string) jobProgress {
	t.Helper()

	got := collectBackgroundShell(t, t.Context(), line)
	require.NotEmpty(t, got, "마지막 조각이 와야 한다")

	return got[len(got)-1]
}

// 줄을 세고 끝나며 요약을 남긴다.
func TestBackgroundShellCountsLines(t *testing.T) {
	last := lastBackgroundShell(t, "echo a; echo b; echo c")

	assert.Equal(t, 3, last.done)
	assert.Equal(t, "3 줄", last.summary)
	assert.NoError(t, last.err)
}

// stdout 과 stderr 이 한 줄기로 온다. 같은 fd 를 주었기 때문이다.
func TestBackgroundShellMergesStderr(t *testing.T) {
	last := lastBackgroundShell(t, "echo one; echo two >&2")

	assert.Equal(t, 2, last.done)
}

// 마지막 `\n` 이 없어도 한 줄로 센다. 흘리면 마지막 말이 사라진다.
func TestBackgroundShellCountsLineWithoutNewline(t *testing.T) {
	last := lastBackgroundShell(t, "printf 'no newline'")

	assert.Equal(t, 1, last.done)
	assert.Equal(t, "1 줄", last.summary)
}

// 실패한 이유는 마지막 비어 있지 않은 줄이다. 출력을 보는 화면이 없어서 그 한 줄이 전부다.
func TestBackgroundShellFailsWithLastLine(t *testing.T) {
	last := lastBackgroundShell(t, "echo boom >&2; echo; exit 2")

	require.Error(t, last.err)
	assert.Equal(t, "boom", last.err.Error(), "끝의 빈 줄을 이유로 들지 않는다")
}

// 출력이 없이 실패하면 exec 이 준 것을 그대로 쓴다.
func TestBackgroundShellFailsWithoutOutput(t *testing.T) {
	last := lastBackgroundShell(t, "exit 3")

	require.Error(t, last.err)
	assert.Contains(t, last.err.Error(), "exit status 3")
}

// 성공하면 apply 가 알린다. finishJob 은 실패만 알리므로(job.go) 여기가 그 자리다.
func TestBackgroundShellNotifiesOnSuccess(t *testing.T) {
	last := lastBackgroundShell(t, "echo hi")

	require.NotNil(t, last.apply, "성공은 apply 가 알린다")
	require.Nil(t, last.err)

	e := &editor{}
	last.apply(e)

	assert.Contains(t, e.notice, "셸 명령이 끝났습니다")
	assert.Contains(t, e.notice, "echo hi")
	assert.Contains(t, e.notice, "1 줄")
}

// 실패한 것은 apply 를 두지 않는다. 알리는 자리가 둘이면 같은 일을 두 번 말한다.
func TestBackgroundShellDoesNotNotifyOnFailure(t *testing.T) {
	last := lastBackgroundShell(t, "exit 1")

	require.Error(t, last.err)
	assert.Nil(t, last.apply)
}

// 꼬리를 잃지 않는다. 몇 줄 뱉고 조용해지는 것이 `make dev-run` 의 모양이다.
//
// 줄 수로 조각을 모으면 그 몇 줄이 상한에 닿지 못해 화면에 영영 오르지 않는다.
// 시간으로 모으므로 채널이 닫히기 전에 그 줄 수가 담긴 조각이 온다.
func TestBackgroundShellSendsProgressBeforeFinishing(t *testing.T) {
	got := collectBackgroundShell(t, t.Context(), "echo a; echo b; sleep 1")

	require.Greater(t, len(got), 1, "끝나기 전에 조각이 와야 한다")
	assert.Equal(t, 2, got[0].done)
	assert.Empty(t, got[0].summary, "진행 조각은 요약을 채우지 않는다")
}

// 긴 줄은 기억만 자르고 계속 읽는다. 읽기를 멈추면 pipe 가 차서 자식이 write 에서 막힌다.
//
// `tr` 은 줄바꿈 없이 내므로 뒤의 `echo` 가 그 줄을 닫는다. 그래서 두 줄이다 —
// 긴 줄 하나와 그 뒤의 `done` 이고, 둘째 줄이 세어진 것이 계속 읽었다는 증거다.
func TestBackgroundShellTrimsLongLineButKeepsReading(t *testing.T) {
	line := "head -c " + strconv.Itoa(shellMaxLineLen*3) + " /dev/zero | tr '\\0' 'x'; echo; echo done"

	last := lastBackgroundShell(t, line)

	require.NoError(t, last.err)
	assert.Equal(t, 2, last.done, "긴 줄 뒤의 줄도 세야 한다")
	assert.Contains(t, last.summary, "긴 줄을 잘랐습니다", "조용히 자르지 않고 적는다")
}

// 취소하면 마지막 조각을 보내지 않는다.
//
// 보내면 updateJob 이 cancelJob 이 미리 적어둔 context.Canceled 를 덧칠해서, 그만하라고 한
// 사람에게 `signal: terminated` 가 실패로 뜬다.
func TestBackgroundShellSendsNothingWhenCancelled(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	ctx, cancel := context.WithCancel(t.Context())

	output := &shellOutput{}
	cmd := newBackgroundShell(ctx, "sleep 30", output)
	require.NoError(t, cmd.Start())

	ch := runBackgroundShell(ctx, cmd, output)
	cancel()

	for progress := range ch {
		assert.NoError(t, progress.err, "취소한 뒤에는 err 을 싣지 않는다")
		assert.Empty(t, progress.summary, "취소한 뒤에는 요약도 없다")
	}
}

// 손자가 pipe 를 붙들고 있어도 끝난다. WaitDelay 를 둔 까닭이 이것이다.
//
// 직계 자식(`$SHELL -c`) 은 곧 끝나는데 손자가 우리 pipe 를 물려받아 안 놓으면 EOF 가 오지
// 않는다. 그대로 두면 채널이 닫히지 않아 `:jobs` 의 그 줄이 영원히 「도는 중」으로 남는다.
func TestBackgroundShellFinishesWhenGrandchildHoldsPipe(t *testing.T) {
	last := lastBackgroundShell(t, "sleep 30 & echo started")

	assert.NoError(t, last.err, "붙든 자식이 남은 것은 실패가 아니다")
	assert.Contains(t, last.summary, "출력을 붙든 자식이 남았습니다")
	require.NotNil(t, last.apply, "성공이라 알린다")
}

// waitForGone 은 그 프로세스가 사라질 때까지 짧게 기다린다.
//
// 신호를 보낸 것과 프로세스가 없어지는 것 사이에 틈이 있어서 곧바로 물으면 아직 살아 있다.
// `Kill(pid, 0)` 은 살아 있으면 nil, 없으면 ESRCH 다.
//
// **거두지 않은 우리 직계 자식에게는 쓸 수 없다.** 죽어도 zombie 로 남아 `Kill(pid, 0)` 이
// 계속 nil 을 준다. 손자를 보거나, `Wait` 를 거두는 쪽을 띄워 두고 봐야 한다.
func waitForGone(t *testing.T, pid int) bool {
	t.Helper()

	for range 200 {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return true
		}

		time.Sleep(20 * time.Millisecond)
	}

	return false
}

// startGrandchild 는 손자를 하나 띄우고 그 pid 를 준다.
//
// 직계 자식이 아니라 그 자식이라야 시험이 뜻이 있다. process group 없이 ctx 만 끊으면
// exec 은 직계 자식만 죽이고 이 손자는 살아남는다.
func startGrandchild(t *testing.T, ctx context.Context, output *shellOutput) (*exec.Cmd, int) {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")

	pidPath := filepath.Join(t.TempDir(), "pid")
	line := "sleep 300 & echo $! > " + pidPath + "; sleep 300"

	cmd := newBackgroundShell(ctx, line, output)
	require.NoError(t, cmd.Start())

	for range 200 {
		text, err := os.ReadFile(pidPath)
		if err == nil && strings.TrimSpace(string(text)) != "" {
			pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
			require.NoError(t, err)

			// 시험이 실패해도 흘리지 않는다.
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

			return cmd, pid
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("손자가 뜨지 않았다")

	return nil, 0
}

// 취소하면 손자까지 죽는다. process group 을 따로 만든 까닭이 이것이다.
//
// `make dev-run` 은 make 가 자식을 낳으므로 직계 자식만 죽이면 서버가 산다.
func TestBackgroundShellKillsGrandchildOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	output := &shellOutput{}
	_, pid := startGrandchild(t, ctx, output)

	cancel()

	assert.True(t, waitForGone(t, pid), "손자가 살아남았다")
}

// 나가는 길이 도는 셸을 죽인다.
//
// ctx 를 끊는 것만으로는 모자라다 — 신호를 보내는 것은 exec 의 감시 goroutine 이고 main 이
// 먼저 반환하면 그것이 돌지 않는다. 그래서 여기서 직접 그룹에 보낸다.
func TestStopBackgroundShellsKillsGroup(t *testing.T) {
	output := &shellOutput{}
	cmd, pid := startGrandchild(t, t.Context(), output)

	e := &editor{}
	e.rememberBackgroundShell("sleep 300", cmd.Process.Pid)

	e.stopBackgroundShells()

	assert.True(t, waitForGone(t, pid), "손자가 살아남았다")
	assert.Empty(t, e.backgroundShells, "지우고 나간다")
}

// `X` 는 그룹에 SIGKILL 을 보낸다. ctx 와 무관한 길이라 이미 `x` 를 누른 것에도 듣는다.
//
// **SIGTERM 을 무시하는 프로세스로 시험하지 않는다.** 그것을 짜려면 셸의 trap 과 foreground
// 자식이 신호로 죽었을 때의 동작에 기대야 하는데, `/bin/sh` 가 무엇인지에 따라 갈려서
// 시험이 그 셸을 검사하게 된다. 여기서 못 박을 것은 「그룹에 SIGKILL 이 닿는가」이고
// 무시하는 놈이 실제로 끝나는지는 앱을 띄워 손으로 본다.
func TestKillBackgroundShellKillsGroup(t *testing.T) {
	output := &shellOutput{}
	cmd, pid := startGrandchild(t, t.Context(), output)

	e := &editor{}
	e.rememberBackgroundShell("sleep 300", cmd.Process.Pid)

	assert.True(t, e.killBackgroundShell("sleep 300"))
	assert.True(t, waitForGone(t, pid), "손자가 살아남았다")
}

// 적어 두지 않은 것은 죽일 것이 없다. 부르는 쪽이 그때 무엇을 할지 정한다.
func TestKillBackgroundShellWithoutGroup(t *testing.T) {
	e := &editor{}

	assert.False(t, e.killBackgroundShell("없는 것"))
}

// pid 를 확인하지 않으면 우리 자신을 죽인다.
//
// `kill(-0, …)` 은 우리 프로세스 그룹 전체이고 `kill(-1, …)` 은 보낼 수 있는 모든
// 프로세스다. 방어가 아니라 필수다.
func TestKillShellGroupRefusesDangerousPids(t *testing.T) {
	for _, pid := range []int{0, 1, -1} {
		err := killShellGroup(&os.Process{Pid: pid}, syscall.SIGKILL)

		assert.ErrorIs(t, err, os.ErrProcessDone, "pid %d 에는 보내지 않는다", pid)
	}

	assert.ErrorIs(t, killShellGroup(nil, syscall.SIGKILL), os.ErrProcessDone)
}

// 화면에 낼 수 없는 글자를 다듬는다. 이 줄이 statusBar 알림과 `:jobs` 의 상태 칸으로 간다.
func TestShellSummaryLine(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "그대로 낼 수 있는 줄", text: "make: *** Error 1", want: "make: *** Error 1"},
		{name: "앞뒤 공백을 뗀다", text: "  boom  ", want: "boom"},
		{name: "공백뿐이면 빈 글", text: "   \t ", want: ""},
		{name: "색을 켜는 escape 를 뗀다", text: "\x1b[31mFAIL\x1b[0m", want: "FAIL"},
		{name: "제어문자를 버린다", text: "a\x00b\x07c", want: "abc"},
		{name: "덮어쓰는 진행 막대의 `\\r` 도 버린다", text: "50%\r100%", want: "50%100%"},
		{name: "한글은 그대로", text: "빌드 실패", want: "빌드 실패"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, shellSummaryLine(test.text))
		})
	}
}

// 실패 이유는 마지막 줄이 있으면 그것이고, 없으면 exec 이 준 것이다.
func TestShellFailure(t *testing.T) {
	base := exec.ErrNotFound

	assert.Equal(t, "boom", shellFailure(base, "boom").Error())
	assert.ErrorIs(t, shellFailure(base, ""), base, "한 줄도 없으면 그대로 쓴다")
}

// `:!&` 가 작업을 열고 편집 화면으로 돌아온다. 터미널을 넘기지 않으므로 화면이 그대로다.
func TestCommandStartsBackgroundShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m = send(m, ":", "!", "&", "t", "r", "u", "e")
	m, cmd := m.Update(key("enter"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.NotNil(t, cmd)

	e := m.(viewEditorNormal).editor
	require.Len(t, e.jobs, 1, "작업이 하나 선다")
	assert.Equal(t, shellJobName, e.jobs[0].name)
	assert.Equal(t, []string{"true"}, e.jobs[0].args, "셸 줄이 인자다")
	assert.NotEmpty(t, e.backgroundShells, "죽일 그룹을 적어 둔다")

	e.stopBackgroundShells()
}

// `:!&` 만 치면 알리고 만다. 작업을 열지 않는다.
func TestCommandBackgroundShellWithoutCommand(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m = send(m, ":", "!", "&")
	m, _ = m.Update(key("enter"))

	require.IsType(t, viewEditorNormal{}, m)

	e := m.(viewEditorNormal).editor
	assert.Equal(t, "셸 명령이 없습니다", e.notice)
	assert.Empty(t, e.jobs)
}

// 공백만 친 것도 셸에 넘기지 않는다. 아무것도 안 하는 작업이 목록에 남는다.
func TestCommandBackgroundShellWithBlankCommand(t *testing.T) {
	e := newTestEditor("abc\n", 80, 10).editor

	assert.Nil(t, e.startBackgroundShell("   "))
	assert.Equal(t, "셸 명령이 없습니다", e.notice)
	assert.Empty(t, e.jobs)
}

// 앞뒤 공백을 뗀 것이 신원이다. 안 떼면 `:!& make` 와 `:!&make` 가 서로 다른 작업이 된다.
func TestCommandBackgroundShellTrimsIdentity(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	e := newTestEditor("abc\n", 80, 10).editor
	require.NotNil(t, e.startBackgroundShell(" sleep 30 "))

	require.Len(t, e.jobs, 1)
	assert.Equal(t, []string{"sleep 30"}, e.jobs[0].args)

	// 공백만 다른 것은 같은 작업이라 새로 열지 않는다.
	assert.Nil(t, e.startBackgroundShell("sleep 30"))
	assert.Contains(t, e.notice, "이미 돌고 있습니다")
	assert.Len(t, e.jobs, 1)

	e.stopBackgroundShells()
}

// 같은 줄이 이미 돌고 있으면 알리고 만다. 사람이 방금 친 것이라 조용하면 먹지 않은 것으로 읽힌다.
func TestCommandBackgroundShellAlreadyRunning(t *testing.T) {
	e := newTestEditor("abc\n", 80, 10).editor
	e.jobs = append(e.jobs, job{name: shellJobName, args: []string{"sleep 1"}, started: time.Now()})

	assert.Nil(t, e.startBackgroundShell("sleep 1"))
	assert.Contains(t, e.notice, "이미 돌고 있습니다")
	assert.Len(t, e.jobs, 1, "새로 열지 않는다")
}

// 줄 범위는 받지 않는다. 줄을 명령에 흘려 넣는 filter 기능이 이 편집기에 없다.
func TestCommandBackgroundShellRefusesRange(t *testing.T) {
	var m tea.Model = newTestEditor("a\nb\nc\n", 80, 10)

	m = send(m, ":", "1", ",", "2", "!", "&", "s", "o", "r", "t")
	m, _ = m.Update(key("enter"))

	e := m.(viewEditorNormal).editor
	assert.Contains(t, e.notice, "줄 범위를 받지 않습니다")
	assert.Empty(t, e.jobs)
}

// 이름 없는 tab 에서 `%` 를 치면 펴지 못한다. `:!` 와 같은 자리다.
func TestCommandBackgroundShellRefusesPercentWithoutName(t *testing.T) {
	e := newTestEditor("abc\n", 80, 10).editor
	e.activeBuffer().Path = ""

	assert.Nil(t, e.startBackgroundShell("cat %"))
	assert.NotEmpty(t, e.notice)
	assert.Empty(t, e.jobs)
}

// 끝나면 죽일 그룹을 지운다. 남겨 두면 그 pid 가 돌아 쓰일 때 남을 죽인다.
func TestBackgroundShellForgetsGroupWhenDone(t *testing.T) {
	e := newTestEditor("abc\n", 80, 10).editor
	e.jobs = append(e.jobs, job{name: shellJobName, args: []string{"echo hi"}, started: time.Now()})
	e.rememberBackgroundShell("echo hi", 424242)

	_, _ = e.handleJob(jobDoneMsg{name: shellJobName, args: []string{"echo hi"}})

	assert.Empty(t, e.backgroundShells)
}

// 팔레트의 `!&` 도 같은 자리로 온다. `!` 만 있고 `!&` 가 없으면 두 길이 갈린다.
func TestPaletteShellBackgroundPrefix(t *testing.T) {
	t.Run("`!&` 를 `!` 보다 먼저 본다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go")

		m = send(m, "!", "&")

		kind, rest := m.(viewPalette).kind()
		assert.Equal(t, paletteKindShellBackground, kind)
		assert.Empty(t, rest)

		rows := strings.Join(boxRowsOf(t, m.(viewPalette)), "\n")
		assert.Contains(t, rows, "!& 뒤에 백그라운드로 돌릴 셸 명령을 칩니다")
	})

	t.Run("치는 도중에 갈래가 바뀐다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go")

		m = send(m, "!", "&", "m", "a", "k", "e")

		rows := strings.Join(boxRowsOf(t, m.(viewPalette)), "\n")
		assert.Contains(t, rows, "Enter 로 백그라운드에서 실행합니다")

		// `&` 를 지우면 동기 셸로 돌아온다.
		m = send(m, "backspace", "backspace", "backspace", "backspace", "backspace")

		kind, _ := m.(viewPalette).kind()
		assert.Equal(t, paletteKindShell, kind)
	})

	t.Run("고를 목록이 없다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go", "b.go")

		m = send(m, "!", "&", "l", "s")

		rows := strings.Join(boxRowsOf(t, m.(viewPalette)), "\n")
		assert.NotContains(t, rows, "a.go", "파일 목록은 나오지 않는다")
		assert.Empty(t, m.(viewPalette).renderCounter())
	})
}
