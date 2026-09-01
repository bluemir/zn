package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 아래 기대값은 vim 9.1 에서 같은 키를 쳐서 확인한 것이다(ADR-0018).
func TestReplaceChar(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want []string
	}{
		{name: "r", data: "abcdef", keys: []string{"r", "x"}, want: []string{"xbcdef"}},
		{name: "3r 은 세 글자", data: "abcdef", keys: []string{"3", "r", "x"}, want: []string{"xxxdef"}},
		{name: "줄이 짧으면 아무 일도 없다", data: "ab", keys: []string{"3", "r", "x"}, want: []string{"ab"}},
		{name: "줄 끝 글자", data: "abc", col: 2, keys: []string{"r", "x"}, want: []string{"abx"}},
		{name: "빈 줄에서는 아무 일도 없다", data: "\nabc", keys: []string{"r", "x"}, want: []string{"", "abc"}},

		// 글자로 세므로 한글 한 글자가 통째로 바뀌고, 한글로도 바꿔 넣는다.
		{name: "한글을 바꾼다", data: "한글abc", keys: []string{"r", "Z"}, want: []string{"Z글abc"}},
		{name: "한글로 바꾼다", data: "abcdef", col: 1, keys: []string{"r", "한"}, want: []string{"a한cdef"}},
		{name: "3r 한글", data: "abcdef", keys: []string{"3", "r", "한"}, want: []string{"한한한def"}},

		// 빈 칸과 tab 은 이름으로 온다.
		{name: "빈 칸으로 바꾼다", data: "abcdef", col: 1, keys: []string{"r", "space"}, want: []string{"a cdef"}},
		{name: "tab 으로 바꾼다", data: "abcdef", col: 1, keys: []string{"r", "tab"}, want: []string{"a\tcdef"}},

		// 글자가 아닌 키는 무른다. 잘못 누른 `r` 을 되돌리는 길이다.
		{name: "esc 로 무른다", data: "abcdef", keys: []string{"r", "esc"}, want: []string{"abcdef"}},
		{name: "방향키로 무른다", data: "abcdef", keys: []string{"r", "up"}, want: []string{"abcdef"}},
		{name: "ctrl 조합으로 무른다", data: "abcdef", keys: []string{"r", "ctrl+w"}, want: []string{"abcdef"}},

		// `r<Enter>` 는 그 자리에서 줄을 가른다.
		{name: "r enter", data: "abc\nxyz", col: 1, keys: []string{"r", "enter"}, want: []string{"a", "c", "xyz"}},
		{name: "r enter 줄 앞에서", data: "abc", keys: []string{"r", "enter"}, want: []string{"", "bc"}},
		{name: "3r enter 는 한 번만 가른다", data: "abcdef", col: 1,
			keys: []string{"3", "r", "enter"}, want: []string{"a", "ef"}},
		{name: "r enter 도 줄이 짧으면 아무 일도 없다", data: "ab",
			keys: []string{"3", "r", "enter"}, want: []string{"ab"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, linesOf(buf))
		})
	}
}

// 바꾼 뒤 커서 자리다. vim 은 마지막으로 바꾼 글자 위에 두고, 줄을 갈랐으면 새 줄 첫 칸이다.
func TestReplaceCharCursor(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want [2]int
	}{
		{name: "제자리", data: "abcdef", keys: []string{"r", "x"}, want: [2]int{0, 0}},
		{name: "3r 은 마지막으로 바꾼 글자", data: "abcdef", keys: []string{"3", "r", "x"}, want: [2]int{0, 2}},
		{name: "3r 한글은 byte 로 센다", data: "abcdef", keys: []string{"3", "r", "한"}, want: [2]int{0, 6}},
		{name: "바뀌지 않으면 제자리", data: "ab", col: 1, keys: []string{"3", "r", "x"}, want: [2]int{0, 1}},
		{name: "r enter 는 새 줄 첫 칸", data: "abc", col: 1, keys: []string{"r", "enter"}, want: [2]int{1, 0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, [2]int{buf.cursorLine, buf.cursorCol})
		})
	}
}

// 한글 입력 상태에서도 `r` 로 한글을 넣을 수 있어야 한다.
// `ㄱ` 이 `r` 이고(ADR-0008), 그 다음 키만 되돌리지 않는다(ADR-0018).
func TestReplaceCharFromHangulKeys(t *testing.T) {
	buf := pressFrom(t, "abcdef", 0, 0, "ㄱ", "x")
	assert.Equal(t, []string{"xbcdef"}, linesOf(buf), "ㄱ 은 r 이다")

	buf = pressFrom(t, "abcdef", 0, 0, "ㄱ", "한")
	assert.Equal(t, []string{"한bcdef"}, linesOf(buf), "다음 키는 자모로 풀지 않는다")

	// 되돌림을 건너뛰는 것은 그 한 키뿐이다. 뒤에 오는 키는 다시 동작이다.
	buf = pressFrom(t, "abcdef", 0, 0, "ㄱ", "한", "ㅌ")
	assert.Equal(t, []string{"bcdef"}, linesOf(buf), "ㅌ 은 x 라서 커서가 선 글자를 지운다")
}

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

func TestReplaceCharMarksDirty(t *testing.T) {
	buf := pressFrom(t, "abc", 0, 0, "r", "x")
	assert.True(t, buf.dirty)

	buf = pressFrom(t, "abc", 0, 0, "r", "esc")
	assert.False(t, buf.dirty, "무른 것은 파일을 건드리지 않는다")

	buf = pressFrom(t, "ab", 0, 0, "3", "r", "x")
	assert.False(t, buf.dirty, "줄이 짧아 바꾸지 못하면 파일은 그대로다")
}

// `r` 을 누르고 기다리는 동안 showcmd 에 보인다.
func TestReplaceShowcmd(t *testing.T) {
	m := newTestEditor("abc", 80, 20)

	after, ok := send(m, "3", "r").(viewEditorNormal)
	require.True(t, ok)

	assert.Equal(t, "3r", after.keyState().showcmd())
}
