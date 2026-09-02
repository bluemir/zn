package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// visual 에서 고른 범위를 `:` 로 실어 보내는 자리다(ADR-0089).

// `:` 를 누르면 명령줄이 열리고 `'<,'>` 가 미리 적힌다.
func TestVisualColonPrefillsRange(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\nd\n", 40, 10), "v", "j", ":")

	require.IsType(t, viewEditorCommand{}, m)
	assert.Equal(t, "'<,'>", m.(viewEditorCommand).input.text)

	// 아래 줄에 그대로 보인다. 보이는 글자라 지울 수도 있다.
	assert.Contains(t, barOf(t, m)[1], ":'<,'>")
}

// 명령줄을 치는 동안 고른 범위가 그대로 칠해져 있다. 무엇에 걸리는지가 눈에 있어야 한다.
func TestVisualColonKeepsSelection(t *testing.T) {
	m := send(newTestEditor("aaa\nbbb\nccc\n", 40, 10), "V", "j", ":")

	buf := bufferOf(t, m)
	require.True(t, buf.selection.active, "고른 것이 살아 있다")

	area, ok := buf.selectionRange()
	require.True(t, ok)
	assert.Equal(t, 0, area.start.line)
	assert.Equal(t, 1, area.end.line)
}

// `backspace` 로 걷어내면 그냥 커서 줄 하나짜리 명령이다.
func TestVisualColonRangeCanBeErased(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\n", 40, 10), "v", "j", ":")

	// `'<,'>` 는 다섯 글자다.
	m = send(m, "backspace", "backspace", "backspace", "backspace", "backspace")

	require.IsType(t, viewEditorCommand{}, m)
	assert.Equal(t, "", m.(viewEditorCommand).input.text)
}

// `esc` 로 나가면 고른 것이 지워진다. normal 로 가는 문이 그것을 한다(ADR-0037).
func TestVisualColonEscapeClearsSelection(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\n", 40, 10), "v", "j", ":", "esc")

	require.IsType(t, viewEditorNormal{}, m)
	assert.False(t, bufferOf(t, m).selection.active)
}

// `V` 로 고른 것은 그 줄들 전체다.
func TestSelectionRangeLinewiseSubstitute(t *testing.T) {
	m := send(newTestEditor("foo\nfoo\nfoo\n", 40, 10), "V", "j", ":")
	m = sendText(m, "s/foo/X/")
	m = send(m, "enter")

	assert.Equal(t, []string{"X", "X", "foo"}, linesOf(bufferOf(t, m)))
	assert.Contains(t, barOf(t, m)[1], "2 곳")
}

// **`v` 로 고른 것은 고른 글자만 바뀐다.** vim 과 갈리는 자리다(ADR-0089 §4).
func TestSelectionRangeCharwiseSubstitute(t *testing.T) {
	// `foo bar foo` 에서 `bar foo`(4..10 칸) 를 고른다. 앞의 `foo` 는 고른 밖이다.
	m := send(newTestEditor("foo bar foo\n", 40, 10), "l", "l", "l", "l", "v")
	for range 6 {
		m = send(m, "l")
	}
	m = send(m, ":")
	m = sendText(m, "s/foo/X/g")
	m = send(m, "enter")

	assert.Equal(t, []string{"foo bar X"}, linesOf(bufferOf(t, m)), "고른 밖의 foo 는 그대로다")
}

// 고른 구간에 걸친 자리는 바꾸지 않는다. 반만 고른 낱말을 바꾸면 고른 밖이 딸려 나간다.
func TestSelectionRangeSkipsStraddlingMatch(t *testing.T) {
	// `abcd` 에서 `bc` 만 고르고 `abcd` 를 찾는다. 걸친 것이라 바꾸지 않는다.
	m := send(newTestEditor("abcd\n", 40, 10), "v", "l")
	m = send(m, ":")
	m = sendText(m, "s/abcd/X/")
	m = send(m, "enter")

	assert.Equal(t, []string{"abcd"}, linesOf(bufferOf(t, m)))
	assert.Contains(t, barOf(t, m)[1], "찾을 수 없음")
}

// 앵커는 줄의 끝에 맞는다. 고른 구간의 끝이 아니다(ADR-0089 §4).
//
// 줄 가운데를 골랐으면 `^` 가 맞는 자리(줄 앞) 가 구간 밖이라 아무 일도 없다.
func TestSelectionRangeAnchorsBindToLine(t *testing.T) {
	m := send(newTestEditor("abcdef\n", 40, 10), "v", "l", "l")

	// 줄 앞에서 골라서 `abc`(0..2 칸) 다. `^` 가 맞는 자리 0 이 구간 안에 있다.
	m = send(m, ":")
	m = sendText(m, "s/^/> /")
	m = send(m, "enter")

	assert.Equal(t, []string{"> abcdef"}, linesOf(bufferOf(t, m)), "줄 앞이 구간 안이라 붙는다")

	// 이번에는 줄 가운데만 고른다. `^` 가 맞는 자리는 구간 밖이다.
	m = send(newTestEditor("abcdef\n", 40, 10), "l", "l", "v", "l")
	m = send(m, ":")
	m = sendText(m, "s/^/> /")
	m = send(m, "enter")

	assert.Equal(t, []string{"abcdef"}, linesOf(bufferOf(t, m)), "구간 밖이라 아무 일도 없다")
}

// `g` 가 없는 `:s` 는 **구간 안의** 첫 자리를 바꾼다. 먼저 하나로 줄이면 구간 밖의 첫
// 자리가 남아서 아무 일도 하지 않게 된다(matchesIn).
func TestSelectionRangeFirstMatchInsideArea(t *testing.T) {
	m := send(newTestEditor("foo foo foo\n", 40, 10), "l", "l", "l", "l", "v")
	for range 6 {
		m = send(m, "l")
	}
	m = send(m, ":")
	m = sendText(m, "s/foo/X/")
	m = send(m, "enter")

	assert.Equal(t, []string{"foo X foo"}, linesOf(bufferOf(t, m)), "구간 안의 첫 자리다")
}

// `:'<,'>d` 도 고른 모양 그대로다. 글자로 골랐으면 그 글자만 지운다.
func TestSelectionRangeDelete(t *testing.T) {
	m := send(newTestEditor("abcdef\n", 40, 10), "v", "l", "l")
	m = send(m, ":")
	m = sendText(m, "d")
	m = send(m, "enter")

	assert.Equal(t, []string{"def"}, linesOf(bufferOf(t, m)))
	assert.Contains(t, barOf(t, m)[1], "3 글자 지웠습니다")
}

// `V` 로 고른 것은 줄로 지우고 줄로 센다.
func TestSelectionRangeDeleteLinewise(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\n", 40, 10), "V", "j")
	m = send(m, ":")
	m = sendText(m, "d")
	m = send(m, "enter")

	assert.Equal(t, []string{"c"}, linesOf(bufferOf(t, m)))
	assert.Contains(t, barOf(t, m)[1], "2 줄 지웠습니다")
}

// `:'<,'>y` 는 고른 글자를 담는다.
func TestSelectionRangeYank(t *testing.T) {
	m := send(newTestEditor("abcdef\n", 40, 10), "v", "l", "l")
	m = send(m, ":")
	m = sendText(m, "y")
	m = send(m, "enter")

	assert.Equal(t, []string{"abcdef"}, linesOf(bufferOf(t, m)), "파일은 그대로다")
	assert.Contains(t, barOf(t, m)[1], "3 글자 복사되었습니다")
}

// normal 에서 친 `'<,'>` 는 거절한다. mark 를 남기지 않았다(ADR-0089 §3).
func TestSelectionRangeRefusedInNormal(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\n", 40, 10), ":")
	m = sendText(m, "'<,'>d")
	m = send(m, "enter")

	assert.Equal(t, []string{"a", "b", "c"}, linesOf(bufferOf(t, m)), "파일은 그대로다")
	assert.Contains(t, barOf(t, m)[1], "고른 범위가 없습니다")
}

// `'` 뒤에 `<` `>` 가 아닌 것이 오면 mark 다. 아직 없다고 말해 준다.
func TestSelectionRangeRejectsMark(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\n", 40, 10), ":")
	m = sendText(m, "'ad")
	m = send(m, "enter")

	assert.Contains(t, barOf(t, m)[1], "mark 는 아직 쓸 수 없습니다")
}

// `:>` 는 명령이 아니다. `<` `>` 가 범위를 열지 못하므로 이름으로 읽힌다(isRangeChar).
func TestSelectionRangeDoesNotSwallowAngleCommands(t *testing.T) {
	m := send(newTestEditor("a\nb\n", 40, 10), ":")
	m = sendText(m, ">")
	m = send(m, "enter")

	assert.Contains(t, barOf(t, m)[1], "알 수 없는 명령")
}

// 한쪽만 `'<` 이거나 자리를 옮기면 줄 단위다. 사람이 줄 번호를 섞어 넣은 것이다.
func TestSelectionRangeMixedIsLinewise(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "한쪽만 고른 범위", input: "'<,2d", want: []string{"ccc"}},
		{name: "자리를 옮긴 것", input: "'<,'>+1d", want: []string{"ccc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 첫 줄 가운데를 글자로 고른다. 줄 단위로 읽히면 그 줄이 통째로 지워진다.
			m := send(newTestEditor("aaa\nbbb\nccc\n", 40, 10), "v", "l")
			m = send(m, ":")

			// 미리 적힌 `'<,'>` 를 걷어내고 다시 친다.
			for range 5 {
				m = send(m, "backspace")
			}
			m = sendText(m, tt.input)
			m = send(m, "enter")

			assert.Equal(t, tt.want, linesOf(bufferOf(t, m)))
		})
	}
}

// 물어보며 바꾸는 길(`:s///c`) 도 고른 구간을 지킨다. 두 길이 같은 함수를 쓴다(ADR-0089 §5).
func TestSelectionRangeConfirmSubstitute(t *testing.T) {
	m := send(newTestEditor("foo bar foo\n", 40, 10), "l", "l", "l", "l", "v")
	for range 6 {
		m = send(m, "l")
	}
	m = send(m, ":")
	m = sendText(m, "s/foo/X/gc")
	m = send(m, "enter")

	require.IsType(t, viewEditorSubstitute{}, m, "물어보는 판이 열린다")

	// 물어보는 자리가 고른 안의 것 하나뿐이다. `y` 로 바꾸면 끝난다.
	m = send(m, "y")

	assert.Equal(t, []string{"foo bar X"}, linesOf(bufferOf(t, m)))
}

// sendText 는 명령줄에 글자를 한 자씩 넣는다. `send` 는 키 이름을 받아서 `'` 나 `/` 같은
// 글자를 그대로 실을 수 없다.
func sendText(m tea.Model, text string) tea.Model {
	for _, ch := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	return m
}

// 아래 줄에 친 그대로 보이는지 본다. ansi 를 벗겨 견준다.
func TestSelectionRangeShowsInBar(t *testing.T) {
	m := send(newTestEditor("a\nb\nc\n", 40, 10), "v", "j", ":")
	m = sendText(m, "s/a/b/")

	bar := ansi.Strip(strings.Join(barOf(t, m), "\n"))

	assert.Contains(t, bar, ":'<,'>s/a/b/")
}
