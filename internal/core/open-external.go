package core

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// 트리 메뉴의 「브라우저로 열기」가 오는 자리다(ADR-0162).
//
// **셸을 거치지 않는다.** `:!& open %` 와 하는 일은 같지만 경로를 셸 줄에 넣으면 공백·따옴표를
// 막아야 하고, `%` 는 일부러 감싸지 않는다(ADR-0114). 경로를 인자 하나로 바로 넘기면 막을 것이
// 없다.

// openJobName 은 OS 에 파일을 넘기는 작업의 이름이다. 인자는 그 파일의 경로다.
const openJobName = "브라우저로 열기"

// openerFor 는 그 OS 에서 파일을 기본 앱으로 넘기는 명령이다.
//
// macOS 는 `open`, 나머지 unix 는 `xdg-open` 이다. windows 는 보지 않는다 — background 셸이
// 이미 unix 전용이다(ADR-0138).
func openerFor(goos string) string {
	if goos == "darwin" {
		return "open"
	}

	return "xdg-open"
}

// startOpenExternal 은 파일을 OS 의 기본 앱에 넘기는 작업을 연다.
//
// **ctx 로 죽이지 않는다.** 편집기를 끝내면 작업 ctx 가 끊기는데, `xdg-open` 은 브라우저가
// 처음 뜰 때 그 브라우저가 닫힐 때까지 돌아오지 않기도 한다. ctx 에 매면 zn 을 닫는 순간
// 브라우저가 같이 죽는다. 띄운 것은 사람이 쓰려고 연 창이라 zn 의 수명과 무관하다.
//
// 세션을 가르는 것(`Setsid`) 은 background 셸과 같은 까닭이다. `ctrl+z` 가 그룹 전체를 멈출 때
// 브라우저까지 멈추지 않게 한다(shell-background.go 의 newBackgroundShell).
func (e *editor) startOpenExternal(path string) tea.Cmd {
	return e.startJob(openJobName, []string{path}, func(context.Context) <-chan jobProgress {
		var output bytes.Buffer

		cmd := exec.Command(openerFor(runtime.GOOS), path)
		cmd.Stdout = &output
		cmd.Stderr = &output
		cmd.WaitDelay = shellWaitDelay
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		result := make(chan jobProgress, 1)

		go func() {
			defer close(result)

			result <- openResult(cmd.Run(), output.String())
		}()

		return result
	})
}

// openResult 는 끝난 opener 의 결과 조각이다.
//
// **WaitDelay 로 끝난 것은 성공이다.** 띄운 브라우저가 출력 pipe 를 물려받아 붙들고 있는
// 것뿐이다 — background 셸이 같은 자리에서 같은 판정을 한다(shell-background.go 의 shellResult).
//
// 실패하면 출력의 마지막 줄이 이유다. `open` 은 「No application knows how to open …」 처럼
// 왜 못 열었는지를 stderr 에 적고, 종료 코드만으로는 그것을 알 수 없다.
func openResult(err error, output string) jobProgress {
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		return jobProgress{}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")

	return jobProgress{err: shellFailure(err, strings.TrimSpace(lines[len(lines)-1]))}
}
