package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/textarea"
)

// `.` 은 마지막 고치기를 지금 커서 자리에서 다시 한다. insert 로 들어가지 않는 동작이다.
func TestRepeatNormalChanges(t *testing.T) {
	tests := []struct {
		name string
		text string
		keys []string
		want []string
	}{
		{name: "dw", text: "one two three", keys: []string{"d", "w", "."}, want: []string{"three"}},
		{name: "x", text: "abcd", keys: []string{"x", "."}, want: []string{"cd"}},
		{name: "r", text: "abcd", keys: []string{"r", "z", "l", "."}, want: []string{"zzcd"}},
		{name: "~", text: "abcd", keys: []string{"~", "."}, want: []string{"ABcd"}},
		{name: "J", text: "a\nb\nc", keys: []string{"J", "."}, want: []string{"a b c"}},
		{name: ">>", text: "a\nb", keys: []string{">", ">", "j", "."}, want: []string{"\ta", "\tb"}},
		{name: "p", text: "ab", keys: []string{"y", "l", "p", "."}, want: []string{"aaab"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := send(newTestEditor(test.text, 80, 10), test.keys...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

// 숫자를 대면 원래 숫자를 갈아 낀다. 그 숫자가 다음 `.` 에도 남는다. vim 과 같다.
func TestRepeatCountReplacesOriginal(t *testing.T) {
	m := send(newTestEditor("abcdefghij", 80, 10), "2", "x")
	require.Equal(t, []string{"cdefghij"}, linesOf(bufferOf(t, m)))

	m = send(m, "3", ".")
	assert.Equal(t, []string{"fghij"}, linesOf(bufferOf(t, m)), "`3.` 은 `3x` 다")

	m = send(m, ".")
	assert.Equal(t, []string{"ij"}, linesOf(bufferOf(t, m)), "숫자 없는 `.` 은 방금 댄 3 을 쓴다")
}

// `c` 뒤에 친 글까지 되풀이한다. 한 번의 `u` 로 통째로 돌아간다.
func TestRepeatChangeWithTypedText(t *testing.T) {
	m := send(newTestEditor("one two three", 80, 10), "c", "w", "X", "Y", "esc", "w", ".")

	require.Equal(t, []string{"XY XY three"}, linesOf(bufferOf(t, m)))
	_, ok := m.(viewEditorNormal)
	assert.True(t, ok, "되풀이한 뒤에는 normal 이다")

	m = send(m, "u")
	assert.Equal(t, []string{"XY two three"}, linesOf(bufferOf(t, m)), "지운 것과 친 것이 한 번에 돌아온다")
}

// insert 에서 지운 것과 줄바꿈도 친 차례대로 되풀이한다.
func TestRepeatInsertReplaysEditingKeys(t *testing.T) {
	m := send(newTestEditor("a\nb", 80, 10), "a", "x", "y", "backspace", "enter", "z", "esc", "j", "j", ".")

	assert.Equal(t, []string{"ax", "z", "bx", "z"}, linesOf(bufferOf(t, m)))
}

// 되풀이한 뒤의 커서는 insert 에서 `esc` 로 나온 자리다. 다음 `.` 이 거기서 잇는다.
func TestRepeatLeavesCursorLikeEsc(t *testing.T) {
	m := send(newTestEditor("", 80, 10), "a", "a", "b", "esc", ".", ".")

	assert.Equal(t, []string{"ababab"}, linesOf(bufferOf(t, m)))
}

// `o` 로 연 줄과 친 글을 되풀이한다.
func TestRepeatOpenLine(t *testing.T) {
	m := send(newTestEditor("a\nb", 80, 10), "o", "x", "esc", "j", ".")

	assert.Equal(t, []string{"a", "x", "b", "x"}, linesOf(bufferOf(t, m)))
}

// 커서를 옮기면 담는 것이 거기서 멈춘다. 옮긴 뒤에 친 것은 되풀이하지 않는다.
func TestRepeatStopsAtCursorMove(t *testing.T) {
	m := send(newTestEditor("\nz", 80, 10), "i", "a", "b", "left", "X", "esc", "j", ".")

	assert.Equal(t, []string{"aXb", "abz"}, linesOf(bufferOf(t, m)))
}

// 붙여넣은 것도 담는다.
func TestRepeatInsertWithPaste(t *testing.T) {
	m := send(newTestEditor("\nz", 80, 10), "i")
	m, _ = m.Update(tea.PasteMsg{Content: "p1\np2"})
	m = send(m, "esc", "j", "0", ".")

	assert.Equal(t, []string{"p1", "p2", "p1", "p2z"}, linesOf(bufferOf(t, m)))
}

// `y` 는 고치지 않으므로 마지막 고치기를 덮지 않는다.
func TestRepeatIgnoresYank(t *testing.T) {
	m := send(newTestEditor("abcd", 80, 10), "x", "y", "l", ".")

	assert.Equal(t, []string{"cd"}, linesOf(bufferOf(t, m)))
}

// 이동은 마지막 고치기를 덮지 않는다.
func TestRepeatAfterMoves(t *testing.T) {
	m := send(newTestEditor("abc\nabc", 80, 10), "x", "j", "0", ".")

	assert.Equal(t, []string{"bc", "bc"}, linesOf(bufferOf(t, m)))
}

// 고친 것이 없으면 알린다. 아무 일도 안 일어나면 키가 안 먹은 것으로 읽힌다.
func TestRepeatWithoutChangeNotifies(t *testing.T) {
	m := send(newTestEditor("abc", 80, 10), ".")

	assert.Equal(t, []string{"abc"}, linesOf(bufferOf(t, m)))
	assert.Contains(t, barOf(t, m)[1], "되풀이할 것이 없습니다")
}

// 자동완성으로 넣은 것은 다시 묻지 않고 그 결과를 커서 기준 같은 폭으로 갈아끼운다.
func TestReplayCompletionStep(t *testing.T) {
	buf := textarea.NewBuffer("", []byte("x fo y\n"))
	buf.Cursor = scheme.Cursor{Line: 0, Col: 4} // `fo` 뒤

	replayInsertStep(&buf, insertStep{key: stepCompletion, text: "foobar", before: 2, after: 0})

	assert.Equal(t, []string{"x foobar y"}, linesOf(buf))
	assert.Equal(t, 8, buf.Cursor.Col, "커서는 넣은 것 뒤다")
}
