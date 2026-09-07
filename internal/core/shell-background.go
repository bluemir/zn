package core

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cockroachdb/errors"
)

// `:!&` 로 셸 명령을 background 작업으로 돌리는 자리다.
//
// **`shell.go` 와 정반대의 일을 한다.** 그쪽은 터미널을 셸에 넘기고 Enter 로 돌아오는
// 동기 실행이고(ADR-0045), 이쪽은 터미널을 주지 않고 편집기를 멈추지 않는다. 같이 쓰는 것은
// `shellPath()` 와 `expandShellLine()` 둘뿐이라 파일을 갈랐다.
//
// **출력을 보는 화면을 만들지 않는다.** 줄 수를 세고 마지막 비어 있지 않은 줄만 든다.
// ADR-0045 가 job 으로 돌리지 않은 까닭 가운데 가장 큰 것이 「출력을 담을 화면」이었고,
// 그것을 만들지 않기로 해서 이 기능이 가능해졌다 (ADR-0138).
//
// **이 파일은 unix 전용이다.** `SysProcAttr.Setsid` 와 `syscall.Kill` 은 windows 에 없다.
// build tag 로 가르지 않는 것이 결정이다 (ADR-0138).

// shellJobName 은 background 셸 명령 작업의 이름이다.
//
// 신원은 이름과 셸 줄을 합친 것이라 서로 다른 명령은 나란히 돌고, `:jobs` 에서는 이름
// 하나로 모여 접힌다(ADR-0075). 이름을 셸 줄로 두면 「이름은 코드에 있는 종류만큼만」이
// 깨진다 — 디렉터리 읽기가 경로를 이름에 붙이다 깨졌던 그 자리다(ADR-0030).
const shellJobName = "셸 명령"

// shellProgressInterval 은 진행 조각을 모아 보내는 간격이다.
//
// **줄마다 보내지 않는다.** 10 만 줄을 뱉는 명령에 Update 와 View 가 10 만 번 돌고, 채널이
// 버퍼가 없어서 편집기의 그리는 속도가 자식의 속도를 잡는다.
//
// **줄 수로 모으지도 않는다.** 「N 줄마다」는 조용한 쪽에서 깨진다 — `make dev-run` 은 석
// 줄 뱉고 몇 시간 조용한데, 그러면 그 석 줄이 상한에 닿지 못해 화면에 영영 오르지 않는다.
// 안 보고 있던 것을 위한 기능인데 정작 볼 것이 비어 있게 된다.
//
// 값은 `editIdleDelay` 와 같은 자리다. 그것이 이미 「손이 멈춘 것을 알아보고 화면을 다시
// 맞추는」 간격이라 화면을 다시 그리게 하는 리듬을 하나로 둔다.
const shellProgressInterval = editIdleDelay

// shellMaxLineLen 은 한 줄로 들고 있을 byte 수의 상한이다.
//
// **읽기를 멈추는 것이 아니라 넘는 부분만 버린다.** 멈추면 pipe 가 차서 자식이 write 에서
// 막힌다. 그래서 이것은 「어디까지 읽나」가 아니라 「얼마를 기억하나」의 상한이다.
//
// 상한이 필요한 까닭은 `\n` 없이 계속 오는 출력이 있어서다(`base64 -w0`, `\r` 로 덮어쓰는
// 진행 막대, minify 된 한 줄). 없으면 그 크기를 우리가 그대로 든다.
//
// 8KiB 인 것은 이 줄이 가는 곳이 statusBar 알림과 `:jobs` 의 상태 칸이라서다 — 화면 한 줄에
// 들어갈 것의 천 배쯤이면 잘려서 뜻을 잃을 걱정이 없다. 닿으면 조용히 자르지 않고 적는다
// (ADR-0077 §7).
const shellMaxLineLen = 8 << 10

// shellWaitDelay 는 자식이 끝난 뒤 pipe 가 닫히기를 기다리는 한계다.
//
// **이것이 없으면 `Wait` 가 영원히 안 돌아온다.** 손자가 pipe 를 물려받고 직계 자식만
// 끝나면(`:!& nohup foo &`, 데몬을 남기는 make) EOF 가 오지 않는다. Go 가 그 경우를
// 그대로 적어 두었다.
//
//	// If WaitDelay is zero (the default), I/O pipes will be read until EOF,
//	// which might not occur until orphaned subprocesses of the command have
//	// also closed their descriptors for the pipes.
//
// 돌아오지 않으면 작업 goroutine 이 채널을 닫지 못해 `:jobs` 의 그 줄이 영원히 「도는 중」
// 으로 남는다. 근거 없는 상수가 아니라 **막히는 자리가 확인된 상한**이다(ADR-0093).
//
// 값은 `saveHookTimeout` 과 같은 자리다. 둘 다 「여기까지 왔으면 그 판이 무언가 잘못된 것」
// 을 재는 시간이라 다른 숫자를 들일 까닭이 없다.
const shellWaitDelay = saveHookTimeout

// shellOutput 은 셸이 낸 것을 세는 자리다. 담아 두지 않고 줄 수와 마지막 줄만 든다.
//
// **exec 의 복사 goroutine 이 쓰고 작업 goroutine 이 읽으므로 잠금이 필요하다.** 우리가
// `io.Writer` 를 주는 까닭이 그 복사 goroutine 을 만드는 것이다 — `cmd.StdoutPipe()` 로
// 직접 읽으면 `*os.File` 이라 복사 goroutine 이 없고, 그러면 `WaitDelay` 가 pipe 를 닫아
// 주는 길이 잠긴다(shellWaitDelay).
type shellOutput struct {
	mu sync.Mutex

	lines int    // 지나간 줄 수
	last  string // 마지막 비어 있지 않은 줄. 실패 이유가 된다
	tail  []byte // 아직 `\n` 을 못 만난 조각
	long  bool   // 한 줄 상한에 닿은 적이 있다
}

// Write 는 `\n` 으로 잘라 줄을 센다. stdout 과 stderr 이 같은 fd 로 와서 순서가 섞이지 않는다.
func (o *shellOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// 넘겨받은 길이를 그대로 돌려준다. 우리가 얼마를 기억하든 셸이 낸 것은 다 받은 것이다 —
	// 적게 돌려주면 io.Copy 가 short write 로 보고 복사를 그만둔다.
	written := len(p)

	for {
		at := bytes.IndexByte(p, '\n')
		if at < 0 {
			o.appendTail(p)

			return written, nil
		}

		o.appendTail(p[:at])
		o.endLine()

		p = p[at+1:]
	}
}

// appendTail 은 모으는 중인 줄에 잇는다. 상한을 넘는 부분은 버리고 닿았다는 것만 적는다.
func (o *shellOutput) appendTail(p []byte) {
	if room := shellMaxLineLen - len(o.tail); room < len(p) {
		o.long = true

		if room <= 0 {
			return
		}

		p = p[:room]
	}

	o.tail = append(o.tail, p...)
}

// endLine 은 줄 하나가 끝났음을 적는다.
func (o *shellOutput) endLine() {
	o.lines++

	// **비어 있지 않은 줄만 든다.** 끝에 빈 줄을 붙이는 명령이 흔해서, 그것을 실패 이유로
	// 들면 아무 말도 없는 알림이 된다.
	if text := shellSummaryLine(string(o.tail)); text != "" {
		o.last = text
	}

	o.tail = o.tail[:0]
}

// counted 는 지금까지 센 것이다. 마지막 조각을 지을 때는 아직 `\n` 을 못 만난 꼬리도 한 줄로 센다.
func (o *shellOutput) counted(withTail bool) (lines int, last string, long bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if withTail && len(o.tail) > 0 {
		o.endLine()
	}

	return o.lines, o.last, o.long
}

// shellSummaryLine 은 화면에 낼 수 있게 다듬은 한 줄이다. 낼 것이 없으면 빈 글이다.
//
// **셸 출력을 그대로 화면에 얹을 수 없다.** 이 줄이 가는 곳이 statusBar 알림과 `:jobs` 의
// 상태 칸이라, 색을 켜는 escape 가 섞여 있으면 그 색이 우리 화면에 새고 제어문자가 있으면
// 커서가 움직인다. 편집기가 파일 안의 `\x1b` 를 `^[` 로 그리는 것과 같은 자리다(ADR-0118).
func shellSummaryLine(text string) string {
	text = ansi.Strip(text)

	var out strings.Builder
	for _, r := range text {
		// 제어문자는 버린다. 그리는 자리가 한 줄이라 `^[` 로 펴 봐야 읽을 것이 없다.
		if r < 0x20 || r == 0x7f {
			continue
		}

		out.WriteRune(r)
	}

	return strings.TrimSpace(out.String())
}

// runBackgroundShell 은 셸이 낸 것을 세면서 끝나기를 기다린다.
//
// 조각을 보내다 막히는 자리마다 빠져나온다. 취소한 뒤 아무도 받지 않는 채널에 goroutine 이
// 남지 않는다(ADR-0027).
//
// **취소된 길에서는 마지막 조각을 보내지 않는다.** `cancelJob` 이 `context.Canceled` 를 미리
// 적어 두는데(job.go), 그 뒤 우리가 err 을 실은 조각을 보내면 `updateJob` 이 그것을 덧칠해서
// 「취소는 알리지 않는다」가 깨진다 — 그만하라고 한 사람에게 `signal: terminated` 가 실패로
// 뜬다. 신호로 죽은 프로세스의 `Wait` 는 `context.Canceled` 가 아니라 `*ExitError` 를 주므로
// err 을 보고 가릴 수도 없다. 그래서 **우리가 끊었다는 사실만 본다.**
func runBackgroundShell(ctx context.Context, cmd *exec.Cmd, output *shellOutput) <-chan jobProgress {
	ch := make(chan jobProgress)

	go func() {
		defer close(ch)

		send := func(progress jobProgress) bool {
			select {
			case ch <- progress:
				return true
			case <-ctx.Done():
				return false
			}
		}

		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()

		ticker := time.NewTicker(shellProgressInterval)
		defer ticker.Stop()

		sent := 0
		for {
			select {
			case <-ticker.C:
				lines, _, _ := output.counted(false)
				if lines == sent {
					continue
				}
				if !send(jobProgress{done: lines}) {
					return
				}
				sent = lines
			case err := <-waited:
				lines, last, long := output.counted(true)
				send(shellResult(cmd, lines, last, long, err))

				return
			case <-ctx.Done():
				// 프로세스는 cmd.Cancel 이 그룹째 죽였다. Wait 은 그 goroutine 이 거두고,
				// 그것이 우리가 든 pipe 를 닫아 복사 goroutine 도 끝낸다.
				return
			}
		}
	}()

	return ch
}

// shellResult 는 끝난 것을 한 조각으로 만든다. summary·err 은 마지막 조각만 채운다(job.go).
func shellResult(cmd *exec.Cmd, lines int, last string, long bool, err error) jobProgress {
	summary := formatCount(lines) + " 줄"
	if long {
		// 조용히 자르지 않고 적는다. grep 이 `5,000+` 로 하는 것과 같은 규칙이다(ADR-0077 §7).
		summary += " (긴 줄을 잘랐습니다)"
	}

	// **WaitDelay 로 끝난 것은 성공이다.** 자식은 제대로 끝났고 pipe 를 붙든 손자가 남은
	// 것뿐인데, 그대로 두면 성공한 `make build` 가 「실패 exec: WaitDelay expired…」로 남는다.
	if errors.Is(err, exec.ErrWaitDelay) {
		return jobProgress{
			done:    lines,
			summary: summary + " (출력을 붙든 자식이 남았습니다)",
			apply:   shellNotifyDone(cmd, lines),
		}
	}

	if err != nil {
		return jobProgress{done: lines, summary: summary, err: shellFailure(err, last)}
	}

	return jobProgress{done: lines, summary: summary, apply: shellNotifyDone(cmd, lines)}
}

// shellFailure 는 실패한 이유다. 출력의 마지막 줄이 있으면 그것이 이유다.
//
// 종료 코드만으로는 무엇이 잘못됐는지 알 수 없고, 출력을 보는 화면이 없으므로 그 한 줄이
// 사용자가 얻는 전부다. 한 줄도 없으면 exec 이 준 것을 그대로 쓴다.
func shellFailure(err error, last string) error {
	if last == "" {
		return err
	}

	return errors.New(last)
}

// shellNotifyDone 은 끝났다고 알리는 apply 다.
//
// **성공도 알린다.** `finishJob` 은 실패만 알리는데(job.go) 이 작업은 안 보고 있던 것이
// 요점이라 끝난 것을 말해야 한다. 설치 작업이 같은 자리에서 성공을 알린다
// (language-server.go, save-hook.go).
func shellNotifyDone(cmd *exec.Cmd, lines int) func(*editor) {
	line := shellCommandLine(cmd)

	return func(e *editor) {
		e.notify("셸 명령이 끝났습니다: " + line + " (" + formatCount(lines) + " 줄)")
	}
}

// shellCommandLine 은 `$SHELL -c` 에 넘긴 줄이다. 알림에 무엇이 끝났는지를 적는 데 쓴다.
func shellCommandLine(cmd *exec.Cmd) string {
	if len(cmd.Args) < 3 {
		return ""
	}

	return cmd.Args[2]
}

// newBackgroundShell 은 돌릴 프로세스를 짓는다. 아직 띄우지는 않는다.
//
// **세션을 따로 가른다(`Setsid`).** 죽이기 때문이 아니라 **신호 때문이다.**
//
//   - `:!` 중에는 터미널이 cooked 로 돌아가 `ctrl+c` 가 진짜 SIGINT 이고 그것은 foreground
//     프로세스 그룹 전체에 간다. 안 가르면 `:!` 를 끊으려 누른 한 번이 돌던 빌드까지 죽인다
//   - `ctrl+z`(`tea.Suspend`) 는 `syscall.Kill(0, SIGTSTP)` 로 그룹 전체를 멈춘다. 안 가르면
//     빌드까지 정지하고, 정지한 자식은 `Wait` 가 안 돌아와 작업이 얼어붙는다
//   - 제어 터미널이 없으면 `/dev/tty` 열기가 실패한다. `sudo` 가 화면 한가운데에 「Password:」
//     를 찍거나 SIGTTIN 으로 조용히 정지하는 대신 「no tty present」로 곧 끝난다
//
// 세션 리더는 자기 pid 가 곧 pgid 라 그룹 번호를 따로 알아낼 것이 없다.
//
// **stdin 은 세우지 않는다.** nil 이면 자식이 `/dev/null` 을 읽어 즉시 EOF 다. 터미널을 주면
// 편집기와 키를 다투게 되므로, 대화형 명령은 이 길로 돌 수 없고 그 사실이 마지막 줄에 남는다.
func newBackgroundShell(ctx context.Context, line string, output *shellOutput) *exec.Cmd {
	cmd := exec.CommandContext(ctx, shellPath(), "-c", line)

	cmd.Stdout = output
	cmd.Stderr = cmd.Stdout // 같은 값이면 childStderr 가 fd 하나를 쓴다. 순서가 섞이지 않는다
	cmd.WaitDelay = shellWaitDelay

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// **`exec.CommandContext` 의 기본 Cancel 은 직계 자식만 죽인다.** `make dev-run` 은
	// make 가 자식을 낳으므로 그것만 죽이면 서버가 산다. 그룹째 죽이는 것이 이 기능의 알맹이다.
	//
	// **pid 를 여기서만 읽는다.** exec 은 `Start` 가 실패하면 Cancel 을 부르지 않고 감시
	// goroutine 은 `Start` 의 끝에서 뜨므로, 이 자리에서는 `Process` 가 이미 차 있다.
	// 밖에서 pid 를 들고 다니면 그것을 읽고 쓰는 자리가 갈려 경쟁이 생긴다.
	cmd.Cancel = func() error { return killShellGroup(cmd.Process, syscall.SIGTERM) }

	return cmd
}

// killShellGroup 은 그 프로세스가 이끄는 그룹 전체에 신호를 보낸다.
//
// **`pid` 를 반드시 확인한다.** `kill(-0, …)` 은 우리 프로세스 그룹 전체(편집기와 그 터미널의
// 모든 것) 이고 `kill(-1, …)` 은 보낼 수 있는 모든 프로세스다. 방어가 아니라 필수다.
//
// 그룹이 갈리지 않았으면(`Setsid` 가 어떤 사정으로 안 걸렸으면) 그룹 대신 그 프로세스만
// 죽인다. 남의 그룹에 신호를 보내는 것보다 덜 죽이는 것이 낫다.
//
// **스스로 그룹을 새로 만든 자식은 놓친다.** `kill(-pgid)` 는 그 그룹에만 가므로, 자식이
// `setpgid` 를 불러 나가면 그 아래가 남는다. 재서 확인했다 — `make dev-run` 의 `watcher` 가
// 자기 자식(`make test run`) 을 새 그룹에 넣어서, 우리가 그룹을 죽인 뒤 그것이 고아로
// 남았다. 세션으로 죽이려면 프로세스 전부를 훑어 세션 번호를 견줘야 하는데 그 목록을 얻는
// 길이 GOOS 마다 달라서 두지 않았다 (docs/tasks.md, ADR-0138).
//
// 거두어진 pid 를 쏠 좁은 창도 남는다. Go 에 그룹 kill 의 pidfd 상당물이 없어 막을 길이
// 없고, 아래 확인이 그 창을 좁힌다 (ADR-0138).
func killShellGroup(process *os.Process, sig syscall.Signal) error {
	if process == nil {
		return os.ErrProcessDone
	}

	pid := process.Pid
	if pid <= 1 {
		return os.ErrProcessDone
	}

	if pgid, err := syscall.Getpgid(pid); err != nil || pgid != pid {
		return process.Signal(sig)
	}

	return syscall.Kill(-pid, sig)
}

// runShellBackgroundMode 는 `:!&` 와 팔레트의 `!&` 가 오는 자리다. runShell 과 짝이다.
//
// 터미널을 넘기지 않으므로 편집 화면이 그대로 있다. 돌아갈 곳은 `:!` 와 같이 언제나 normal
// 이다 — 팔레트가 어디서 열렸는지 기억하지 않는 것과 같은 태도다(ADR-0011).
func runShellBackgroundMode(e *editor, line string) (tea.Model, tea.Cmd) {
	start := e.startBackgroundShell(line)

	model, next := normalMode(e)

	return model, tea.Batch(next, start)
}

// startBackgroundShell 은 작업을 연다. 프로세스를 띄우는 것도 여기다.
//
// **프로세스를 띄우기 전에 이미 도는지 먼저 묻는다.** `startJob` 은 같은 신원이 돌면 조용히
// nil 을 주는데(job.go) 띄우는 자리가 그 안이라, 묻지 않으면 갈 곳 없는 셸이 하나 뜬다.
// 언어 서버 설치가 같은 손으로 먼저 묻는다(language-server.go).
//
// 조용히 물러나지 않고 알리는 것은 **사람이 방금 친 것**이기 때문이다. 코드가 부른 작업
// (트리 읽기·git 갱신) 은 조용해도 되지만, `:` 를 치고 enter 를 눌렀는데 아무 일도 안 나면
// 먹지 않은 것으로 읽힌다.
func (e *editor) startBackgroundShell(line string) tea.Cmd {
	line, err := expandShellLine(line, e.currentFile())
	if err != nil {
		e.notifyError(err)

		return nil
	}

	// **앞뒤 공백을 뗀다. `:!` 는 떼지 않는다.**
	//
	// 셸이 하는 일은 그대로다(`sh -c " make"` 와 `sh -c "make"` 가 같다). 떼는 까닭은 이
	// 줄이 **작업의 신원**이라서다 — 안 떼면 `:!& make` 와 `:!&make` 가 서로 다른 작업이
	// 되어 `make dev-run` 이 둘 뜬다. `:!` 에는 신원이 없어서 그 자리가 없었다.
	//
	// 곁들여 알림도 깨끗해진다. `job.title()` 이 이름과 인자를 한 칸으로 이으므로 앞 공백을
	// 두면 「셸 명령  make 실패」처럼 두 칸이 된다.
	line = strings.TrimSpace(line)

	// 빈 줄은 셸에 넘기지 않는다. `:!` 는 넘겨도 화면에 아무 일이 없지만 이쪽은 아무것도
	// 하지 않는 작업 하나가 목록에 남는다.
	if line == "" {
		e.notify("셸 명령이 없습니다")

		return nil
	}

	args := []string{line}
	if e.jobRunning(shellJobName, args) {
		e.notify("이미 돌고 있습니다: " + line)

		return nil
	}

	return e.startJob(shellJobName, args, func(ctx context.Context) <-chan jobProgress {
		output := &shellOutput{}
		cmd := newBackgroundShell(ctx, line, output)

		if err := cmd.Start(); err != nil {
			failed := make(chan jobProgress, 1)
			failed <- jobProgress{err: err}
			close(failed)

			return failed
		}

		// **여기는 goroutine 이 아니라 Update 안이다** — `startJob` 이 이 자리에서 그대로
		// 부른다(job.go). 그래서 나가는 길이 읽을 그룹 번호를 곧바로 적을 수 있다. 조각으로
		// 미루면 그 사이에 편집기가 끝났을 때 무엇을 죽여야 하는지 모르게 된다.
		e.rememberBackgroundShell(line, cmd.Process.Pid)

		return runBackgroundShell(ctx, cmd, output)
	})
}

// rememberBackgroundShell 은 죽일 그룹을 적어 둔다. 키는 셸 줄, 곧 그 작업의 인자다.
//
// **작업 목록(`e.jobs`) 에 담지 않는다.** 실행기는 진행률만 알고 결과 모양은 작업이 정한다는
// 규칙이라(job.go), 프로세스를 아는 것도 그 일을 하는 쪽이다.
func (e *editor) rememberBackgroundShell(line string, pgid int) {
	if e.backgroundShells == nil {
		e.backgroundShells = map[string]int{}
	}

	e.backgroundShells[line] = pgid
}

// forgetBackgroundShell 은 끝난 그룹을 지운다.
//
// **남겨 두면 그 pid 가 돌아 쓰일 때 남을 죽인다.** 지우는 자리는 작업이 끝났음을 아는 곳
// 하나여야 한다 — 끝났든 실패했든 취소됐든 `jobDoneMsg` 가 그 자리를 지난다(job.go).
func (e *editor) forgetBackgroundShell(line string) {
	delete(e.backgroundShells, line)
}

// killBackgroundShell 은 그 셸을 SIGKILL 로 끝낸다. `:jobs` 의 `X` 다.
//
// **`x`(취소) 와 다른 길이다.** 그쪽은 ctx 를 끊어 `cmd.Cancel` 이 SIGTERM 을 보내게 하는데,
// ctx 는 한 번만 끊을 수 있어서 무시하는 프로세스에 다시 손쓸 자리가 없다. 그룹 번호를 들고
// 있으므로 여기서 곧바로 보낸다 — 이미 `x` 를 누른 것에도 듣는다.
//
// 죽일 프로세스가 없으면 false 다. 부르는 쪽이 그때 무엇을 할지 정한다.
func (e *editor) killBackgroundShell(line string) bool {
	pgid, ok := e.backgroundShells[line]
	if !ok {
		return false
	}

	_ = killShellGroup(&os.Process{Pid: pgid}, syscall.SIGKILL)

	return true
}

// stopBackgroundShells 는 나가는 길이 도는 셸을 죽이는 자리다.
//
// **ctx 를 끊는 것만으로는 모자라다.** 끊어도 신호를 보내는 것은 exec 의 감시 goroutine 이고,
// `main` 이 먼저 반환하면 그것이 돌지 않아 신호가 나가기도 전에 프로세스가 끝난다. 그러면
// `make dev-run` 이 편집기보다 오래 산다. 여기서 직접 그룹에 보내면 커널이 그 자리에서
// 배달하므로 우리가 먼저 나가도 죽는 것이 보장된다 (ADR-0138).
func (e *editor) stopBackgroundShells() {
	for _, pgid := range e.backgroundShells {
		// 이미 죽은 그룹이면 볼 것이 없다. 편집기의 마지막 줄이라 알릴 화면도 없다.
		_ = killShellGroup(&os.Process{Pid: pgid}, syscall.SIGKILL)
	}

	e.backgroundShells = nil
}
