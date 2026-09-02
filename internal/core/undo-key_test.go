package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// **키 하나가 되돌리기 구간 하나인가**를 보는 시험이다. `u` 한 번에 그 키가 한 일만
// 돌아와야 한다(ADR-0033).

// 바꿔 넣기 한 번이 되돌리기 한 구간이다.
func TestReplaceCharUndoIsOwnStep(t *testing.T) {
	m := newTestEditor("abcdef", 80, 20)

	after := send(m, "i", "X", "esc", "3", "r", "z")
	require.Equal(t, []string{"zzzcdef"}, linesOf(bufferOf(t, after)))

	after = send(after, "u")
	assert.Equal(t, []string{"Xabcdef"}, linesOf(bufferOf(t, after)), "바꾼 세 글자가 한 번에 돌아온다")

	after = send(after, "u")
	assert.Equal(t, []string{"abcdef"}, linesOf(bufferOf(t, after)), "타이핑이 돌아온다")

	after = send(after, "ctrl+r", "ctrl+r")
	assert.Equal(t, []string{"zzzcdef"}, linesOf(bufferOf(t, after)))
}

// `r` 을 누르고 기다리는 동안 showcmd 에 보인다.
func TestReplaceShowcmd(t *testing.T) {
	m := newTestEditor("abc", 80, 20)

	after, ok := send(m, "3", "r").(viewEditorNormal)
	require.True(t, ok)

	assert.Equal(t, "3r", after.keyState().showcmd())
}

// 읽기 전용 파일은 잇지 않는다. 고치는 동작이 모두 지나는 문이다.
func TestJoinRefusesReadOnly(t *testing.T) {
	m := newTestEditor("foo\nbar\n", 80, 10)
	m.buffers[0].readOnly = true

	next := send(tea.Model(m), "J")

	assert.Equal(t, []string{"foo", "bar"}, linesOf(bufferOf(t, next)))
	assert.Contains(t, barOf(t, next)[1], "읽기 전용")
}

// 이은 줄이 화면보다 길어져도 커서는 화면 안에 남는다. 이은 자리가 wrap 된 줄의 아래 행이다.
func TestJoinKeepsCursorVisible(t *testing.T) {
	long := strings.Repeat("a", 200)
	m := newTestEditor(long+"\n"+long, 40, 3)

	next := send(tea.Model(m), "J")
	view := next.(viewEditorNormal)

	buf := bufferOf(t, next)
	require.Equal(t, 200, buf.cursor.Col, "이은 자리는 원래 첫 줄의 끝 다음이다")

	_, y, ok := buf.cursorScreenPos(view.textHeight())
	require.True(t, ok, "커서가 화면 안에 있다")
	assert.Less(t, y, view.textHeight())
}
