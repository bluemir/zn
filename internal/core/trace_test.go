package core

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureTrace 는 로그를 Debug 로 켜고 traceEvent 가 적은 것을 돌려준다.
// 끝나면 원래 설정으로 돌린다 — 다른 시험이 로그를 켠 채로 돌면 안 된다.
func captureTrace(t *testing.T, model tea.Model, msg tea.Msg) (string, tea.Msg) {
	t.Helper()

	out, level := logrus.StandardLogger().Out, logrus.GetLevel()
	t.Cleanup(func() {
		logrus.SetOutput(out)
		logrus.SetLevel(level)
	})

	buf := &bytes.Buffer{}
	logrus.SetOutput(buf)
	logrus.SetLevel(logrus.DebugLevel)

	got := traceEvent(model, msg)

	return buf.String(), got
}

// **filter 는 받은 msg 를 그대로 돌려줘야 한다.**
//
// nil 을 돌려주면 bubbletea 가 그 msg 를 버린다. 로그를 켜고 끄는 것이 편집기의 동작을
// 바꾸면 안 된다 — 이것이 깨지면 `-vv` 로 띄운 편집기가 키를 하나도 안 먹는다.
func TestTraceEventPassesMessageThrough(t *testing.T) {
	model := newTestEditor("가나\n", 40, 5)

	msgs := []tea.Msg{
		tea.KeyPressMsg{Code: 'a', Text: "a"},
		tea.KeyPressMsg{Code: 'ㅁ', Text: "ㅁ"},
		tea.KeyReleaseMsg{Code: 'a'},
		tea.PasteMsg{Content: "붙임"},
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.FocusMsg{},
		gitTickMsg{},
		fileTickMsg{},
		// 안 적는 갈래도 그대로 지나가야 한다.
		tea.MouseClickMsg{},
		shellDoneMsg{},
	}

	for _, msg := range msgs {
		_, got := captureTrace(t, model, msg)
		assert.Equal(t, msg, got, "%T 를 그대로 돌려줘야 한다", msg)
	}
}

// 로그가 꺼져 있으면 아무것도 쓰지 않는다. 키마다 도는 자리다.
func TestTraceEventSilentWhenDisabled(t *testing.T) {
	out, level := logrus.StandardLogger().Out, logrus.GetLevel()
	t.Cleanup(func() {
		logrus.SetOutput(out)
		logrus.SetLevel(level)
	})

	buf := &bytes.Buffer{}
	logrus.SetOutput(buf)
	logrus.SetLevel(logrus.WarnLevel) // 기본값이다

	msg := tea.KeyPressMsg{Code: 'ㅁ', Text: "ㅁ"}
	got := traceEvent(newTestEditor("", 40, 5), msg)

	assert.Empty(t, buf.String(), "꺼져 있으면 한 글자도 안 쓴다")
	assert.Equal(t, msg, got)
}

// 한글이 어떤 모양으로 왔는지가 로그 한 줄에 다 담겨야 한다.
//
// 눈으로는 같은 글자가 다른 코드포인트일 수 있어서(호환 자모 `ㅁ` U+3141 과 음절 자모
// `ᄆ` U+1106) 글자만 적으면 가릴 수 없다.
func TestTraceKeyRecordsCodePoints(t *testing.T) {
	tests := []struct {
		name string
		key  tea.Key
		want []string
	}{
		{
			name: "한글 자모",
			key:  tea.Key{Code: 'ㅁ', Text: "ㅁ"},
			want: []string{`text="ㅁ"[U+3141]`, `code='ㅁ'(U+3141)`, "base=-", "mod=-"},
		},
		{
			name: "조합된 음절",
			key:  tea.Key{Code: '둘', Text: "둘"},
			want: []string{`text="둘"[U+B458]`, `code='둘'(U+B458)`},
		},
		{
			// ADR-0014 가 Ghostty 에서 잰 값이다. 한글 상태에서도 `ctrl+p` 를 알아보는 근거다.
			name: "한글 상태의 ctrl+p 는 BaseCode 로 온다",
			key:  tea.Key{Code: 'ㅔ', BaseCode: 'p', Mod: tea.ModCtrl},
			want: []string{`code='ㅔ'(U+3154)`, `base='p'(U+0070)`, "mod=ctrl", "text=-"},
		},
		{
			name: "여러 modifier",
			key:  tea.Key{Code: 'a', Mod: tea.ModCtrl | tea.ModShift},
			want: []string{"mod=shift+ctrl"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := traceKey(test.key)
			for _, want := range test.want {
				assert.Contains(t, got, want)
			}
		})
	}
}

// 키를 안 눌렀는데 화면을 다시 그리게 하는 것들이 남아야 한다.
// 이것이 없으면 「손을 안 댔는데 글자가 사라졌다」를 로그에서 확인할 길이 없다.
func TestTraceLineRecordsRepaintsWithoutKeys(t *testing.T) {
	model := newTestEditor("", 40, 5)

	tests := []struct {
		msg  tea.Msg
		want string
	}{
		{gitTickMsg{}, "tick git"},
		{fileTickMsg{}, "tick file"},
		{jobProgressMsg{}, "job progress"},
		{jobDoneMsg{}, "job done"},
		{tea.WindowSizeMsg{Width: 80, Height: 24}, "80x24"},
		{tea.FocusMsg{}, "focus"},
		{tea.BlurMsg{}, "blur"},
	}

	for _, test := range tests {
		assert.Contains(t, traceLine(model, test.msg), test.want)
	}

	// 나머지는 적지 않는다. 다 적으면 봐야 할 것이 잡음에 묻힌다.
	assert.Empty(t, traceLine(model, tea.MouseClickMsg{}), "마우스는 적지 않는다")
}

// mode 이름이 로그에 남아야 한다 — 조합이 어느 mode 로 도착했는지가 요점이다.
func TestTraceLineNamesMode(t *testing.T) {
	var normal tea.Model = newTestEditor("가나\n", 40, 5)

	line := traceLine(normal, tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Contains(t, line, "mode=NORMAL")

	insert, _ := normal.(viewEditorNormal).press("i")
	line = traceLine(insert, tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Contains(t, line, "mode=INSERT")
}

// 실제로 로그에 나가는지. Debug 로 켜야만 나간다.
func TestTraceEventWritesLine(t *testing.T) {
	logged, _ := captureTrace(t, newTestEditor("", 40, 5),
		tea.KeyPressMsg{Code: 'ㅁ', Text: "ㅁ"})

	require.NotEmpty(t, logged)
	assert.Contains(t, logged, "key press")
	assert.Contains(t, logged, "U+3141")
	assert.True(t, strings.Contains(logged, "mode=NORMAL"))
}

// 키 줄에 커서 자리와 그 줄이 같이 남아야 한다.
//
// 「빈칸에서 지웠는데 옆 글자가 없어졌다」 같은 것은 무엇이 지워졌는지를 앞뒤 줄로 견주어야
// 가릴 수 있어서, 키 이름만으로는 모자란다.
func TestTraceLineRecordsBufferState(t *testing.T) {
	var m tea.Model = newTestEditor("\t- — 오른쪽\n", 60, 5)
	buf := m.(viewEditorNormal).activeBuffer()

	// `—` 다음 빈칸에 커서를 둔다
	buf.cursorLine, buf.cursorCol = 0, 6

	line := traceLine(m, tea.KeyPressMsg{Code: 'x', Text: "x"})

	assert.Contains(t, line, "cur=1:6", "줄:칸 이 남는다")
	assert.Contains(t, line, traceCursor+" 오른쪽", "커서 표시가 빈칸 위에 온다")
	assert.Contains(t, line, "screen=7", "화면 칸도 남는다 — tab 은 네 칸이다")
}

// 커서가 줄 끝 다음 칸이어도(insert) 죽지 않는다.
func TestTraceBufferAtLineEnd(t *testing.T) {
	var m tea.Model = newTestEditor("가\n", 60, 5)
	buf := m.(viewEditorNormal).activeBuffer()
	buf.cursorLine, buf.cursorCol = 0, 3

	assert.NotPanics(t, func() {
		assert.Contains(t, traceLine(m, tea.KeyPressMsg{Code: 'x', Text: "x"}), "가"+traceCursor)
	})
}

// tick 줄에도 buffer 상태가 붙어야 한다.
//
// 키 줄은 「누르기 직전」이라 마지막 키의 결과가 안 남는다. 그것을 보려고 키를 하나 더
// 누르게 하면 그 키가 또 무언가를 바꾼다. tick 은 저절로 오므로 가만히 있어도 결과가 남는다.
func TestTraceTickCarriesBufferState(t *testing.T) {
	var m tea.Model = newTestEditor("\t- — 오른쪽\n", 60, 5)
	buf := m.(viewEditorNormal).activeBuffer()
	buf.cursorLine, buf.cursorCol = 0, 6

	for _, msg := range []tea.Msg{gitTickMsg{}, fileTickMsg{}} {
		line := traceLine(m, msg)
		assert.Contains(t, line, "cur=1:6", "%T 에 커서가 없다", msg)
		assert.Contains(t, line, traceCursor+" 오른쪽", "%T 에 줄이 없다", msg)
	}
}
