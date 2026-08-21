package core

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellPath(t *testing.T) {
	t.Run("$SHELL 을 쓴다", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")

		assert.Equal(t, "/bin/zsh", shellPath())
	})

	t.Run("비어 있으면 sh 다", func(t *testing.T) {
		t.Setenv("SHELL", "")

		assert.Equal(t, "sh", shellPath())
	})
}

// runShell 은 무엇이 돌았는지 먼저 찍는다. 출력만 남으면 어느 명령의 것인지 알 수 없다.
// 그리고 끝나면 Enter 를 기다린다 — 그 틈이 없으면 대체 화면이 출력을 곧바로 덮는다.
func TestShellRunEchoesCommandAndWaitsForEnter(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	out := &bytes.Buffer{}
	run := &shellRun{line: "echo hi"}
	run.SetStdin(strings.NewReader("\n"))
	run.SetStdout(out)
	run.SetStderr(out)

	require.NoError(t, run.Run())

	text := out.String()
	assert.Less(t, strings.Index(text, ":!echo hi"), strings.Index(text, "hi\n"), "명령이 출력보다 먼저다")
	assert.Less(t, strings.Index(text, "hi\n"), strings.Index(text, "계속하려면 Enter"), "묻는 것은 맨 나중이다")
}

// 종료 코드는 그대로 돌려준다. 그래도 Enter 는 먼저 묻는다 — 왜 실패했는지가 화면에 있다.
func TestShellRunReturnsExitError(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	out := &bytes.Buffer{}
	run := &shellRun{line: "exit 3"}
	run.SetStdin(strings.NewReader("\n"))
	run.SetStdout(out)
	run.SetStderr(out)

	err := run.Run()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 3")
	assert.Contains(t, out.String(), "계속하려면 Enter")
}

// 셸이 stdin 을 그대로 받는다. 대화형 명령이 도는 것이 이 기능의 절반이다.
func TestShellRunPassesStdin(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")

	out := &bytes.Buffer{}
	run := &shellRun{line: "read line; echo [$line]"}
	run.SetStdin(strings.NewReader("보낸 줄\n\n"))
	run.SetStdout(out)
	run.SetStderr(out)

	require.NoError(t, run.Run())
	assert.Contains(t, out.String(), "[보낸 줄]")
}

func TestFinishShellReportsOnlyFailure(t *testing.T) {
	t.Run("성공은 조용하다", func(t *testing.T) {
		e := newTestEditor("a\n", 40, 5).editor

		e.finishShell(nil)

		assert.Empty(t, e.message)
	})

	t.Run("실패는 아래 줄에 남는다", func(t *testing.T) {
		e := newTestEditor("a\n", 40, 5).editor

		e.finishShell(assert.AnError)

		assert.Contains(t, e.message, "셸 명령이 실패했습니다")
	})

	// `ctrl+c` 로 끊은 것은 실패가 아니다. 그만하라고 한 사람이 결과를 이미 안다(ADR-0027).
	t.Run("신호로 끊긴 것은 알리지 않는다", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/sh")

		cmd := exec.Command("/bin/sh", "-c", "kill -INT $$")
		err := cmd.Run()
		require.Error(t, err)

		e := newTestEditor("a\n", 40, 5).editor

		e.finishShell(err)

		assert.Empty(t, e.message)
	})
}

// `:!` 는 normal 로 나가면서 터미널을 넘기는 Cmd 를 낸다.
// Cmd 를 태우지는 않는다 — tea.Exec 는 실제 터미널을 요구한다.
func TestCommandShellLeavesToNormalWithExec(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "!", "l", "s")

	m, cmd := m.Update(key("enter"))

	assert.IsType(t, viewEditorNormal{}, m)
	assert.NotNil(t, cmd, "터미널을 넘길 Cmd 가 나온다")
}

func TestCommandShellWithoutCommand(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "!", "enter")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "셸 명령이 없습니다")
}

// 셸에 넘길 것은 뜯지 않는다. 닫히지 않은 따옴표도 여기서는 오류가 아니다 — 셸이 읽는다.
func TestCommandShellDoesNotTokenizeTail(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "!", "e", "c", "h", "o", " ", `"`, "a")

	m, cmd := m.Update(key("enter"))

	assert.IsType(t, viewEditorNormal{}, m)
	assert.NotNil(t, cmd)
	assert.NotContains(t, barOf(t, m)[1], "따옴표")
}

func TestPaletteShellPrefix(t *testing.T) {
	t.Run("안내를 놓고 counter 를 비운다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go", "b.go")

		m = send(m, "!")

		rows := strings.Join(boxRowsOf(t, m.(viewPalette)), "\n")
		assert.Contains(t, rows, "! 뒤에 셸 명령을 칩니다")
		assert.Empty(t, m.(viewPalette).renderCounter())

		m = send(m, "l", "s")

		rows = strings.Join(boxRowsOf(t, m.(viewPalette)), "\n")
		assert.Contains(t, rows, "Enter 로 셸에서 실행합니다")
		assert.NotContains(t, rows, "a.go", "파일 목록은 나오지 않는다")
	})

	t.Run("`!` 를 지우면 파일 찾기로 돌아온다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go", "b.go")

		m = send(m, "!", "backspace")

		rows := strings.Join(boxRowsOf(t, m.(viewPalette)), "\n")
		assert.Contains(t, rows, "a.go")
	})

	t.Run("Enter 가 터미널을 넘긴다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go")

		m = send(m, "!", "l", "s")

		m, cmd := m.Update(key("enter"))

		assert.IsType(t, viewEditorNormal{}, m)
		assert.NotNil(t, cmd)
	})

	t.Run("친 것이 없으면 Enter 가 아무 일도 하지 않는다", func(t *testing.T) {
		var m tea.Model = newPaletteView(t, 80, 20, "a.go")

		m = send(m, "!")

		m, cmd := m.Update(key("enter"))

		assert.IsType(t, viewPalette{}, m)
		assert.Nil(t, cmd)
	})
}
