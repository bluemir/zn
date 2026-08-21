package core

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// 이 파일이 internal/core 의 첫 os/exec 다. git 을 라이브러리로 옮긴 ADR-0042 와 어긋나지
// 않는다 — 그쪽은 "우리가 아는 일을 프로세스로 하지 말자" 였고, 이쪽은 사용자가 시킨 남의
// 프로세스라 우리가 대신할 수 있는 것이 없다 (ADR-0045).

// shellPath 는 명령을 넘길 셸이다. vim 의 `shell` 옵션 기본값과 같은 태도다 —
// 사용자가 쓰는 셸에 넘겨야 그 셸의 문법으로 친 것이 그대로 돈다.
func shellPath() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	return "sh"
}

// shellRun 은 tea.Exec 가 터미널을 넘겨주고 돌리는 것이다.
//
// tea.ExecProcess(exec.Cmd 를 그대로 넘기는 것) 를 쓰지 않는 이유는 명령이 끝난 뒤 **출력을
// 읽을 틈**을 줘야 하기 때문이다. 돌아가는 순간 대체 화면이 다시 켜져서 출력을 덮는다 —
// 그 사이에 멈춰 서는 자리가 필요하고, 그것은 Run 안에서만 만들 수 있다 (ADR-0045).
type shellRun struct {
	line string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (s *shellRun) SetStdin(r io.Reader)  { s.stdin = r }
func (s *shellRun) SetStdout(w io.Writer) { s.stdout = w }
func (s *shellRun) SetStderr(w io.Writer) { s.stderr = w }

// Run 은 셸에 한 줄을 넘기고, 끝나면 Enter 를 기다린다.
//
// 줄바꿈을 `\r\n` 으로 내는 것은 터미널이 raw mode 에서 막 돌아온 자리라서다. 되돌리기가
// 끝나 있으면 `\r` 이 한 번 더 붙는 것뿐이고, 아직이면 그것이 있어야 줄이 밀리지 않는다.
func (s *shellRun) Run() error {
	// 무엇이 돌았는지 남긴다. 출력만 있으면 어느 명령의 것인지 알 수 없다.
	fmt.Fprintf(s.stdout, "\r\n:!%s\r\n", s.line)

	cmd := exec.Command(shellPath(), "-c", s.line)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.stdin, s.stdout, s.stderr

	err := cmd.Run()

	// 출력을 읽을 틈이다. 실패해도 먼저 묻는다 — 무엇 때문에 실패했는지가 화면에 있다.
	// vim 의 「Press ENTER or type command to continue」와 같은 자리다.
	fmt.Fprint(s.stdout, "\r\n계속하려면 Enter 를 누르세요")
	_, _ = bufio.NewReader(s.stdin).ReadString('\n')

	return err
}

// shellDoneMsg 는 셸에 넘겼던 터미널이 돌아왔다는 것이다.
type shellDoneMsg struct {
	err error
}

// runShell 은 친 것을 셸에 넘긴다. `:!` 와 팔레트의 `!` 가 같은 자리로 온다.
//
// 편집 화면이 물러나고 터미널을 그대로 넘기므로 `git commit`·`less` 처럼 화면을 쓰는 명령도
// 그대로 돈다. 돌아갈 곳은 언제나 normal 이다 — 팔레트가 어디서 열렸는지 기억하지 않는 것과
// 같은 태도다(ADR-0011).
//
// 빈 줄은 여기서 보지 않는다. 아무것도 치지 않았을 때 할 일이 부르는 쪽마다 다르다 —
// `:!` 는 알리고, 팔레트는 아직 치는 중이라 가만히 있는다.
func runShell(e *editor, line string) (tea.Model, tea.Cmd) {
	model, next := normalMode(e)

	run := tea.Exec(&shellRun{line: line}, func(err error) tea.Msg {
		return shellDoneMsg{err: err}
	})

	return model, tea.Batch(next, run)
}

// finishShell 은 셸에서 돌아온 뒤를 정리한다.
//
// 실패만 알린다 — 성공은 방금 화면에서 보고 Enter 로 넘어온 것이라 다시 말할 것이 없다.
// 실패도 이미 봤지만, 종료 코드는 출력에 섞여 안 보일 수 있어서 한 줄로 남긴다.
//
// 신호로 끊긴 것은 실패에 넣지 않는다. `ctrl+c` 로 그만하라고 한 사람이 결과를 이미 알고
// 있어서, `:jobs` 의 취소가 조용한 것과 같은 규칙이다(ADR-0027). 신호로 끝난 프로세스는
// 종료 코드가 없어서 -1 이다.
//
// 바깥을 만지고 온 자리이므로 보고 있는 파일을 다시 검사한다. `ctrl+z` 복귀·터미널 포커스
// 복귀와 같은 자리다(ADR-0023, ADR-0031). 잃을 것이 없으면 그대로 가져온다(ADR-0038).
// git 은 여기서 읽지 않는다 — 주기 갱신이 따라온다(ADR-0030).
func (e *editor) finishShell(err error) tea.Cmd {
	var exit *exec.ExitError
	signaled := errors.As(err, &exit) && exit.ExitCode() == -1

	if err != nil && !signaled {
		e.message = "셸 명령이 실패했습니다: " + err.Error()
	}

	return e.startOutsideCheck()
}
