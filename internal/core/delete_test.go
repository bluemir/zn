package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pressFrom 은 커서를 놓고 키를 친 뒤의 buffer 다.
// 이동과 지우기가 한 경로에서 만나는지 봐야 해서 Buffer 가 아니라 mode 를 거쳐 친다.
func pressFrom(t *testing.T, data string, line, col int, keys ...string) Buffer {
	t.Helper()

	m := newTestEditor(data, 80, 20)
	m.buffers[0].cursorLine = line
	m.buffers[0].cursorCol = col

	return bufferOf(t, send(m, keys...))
}

// 아래 기대값은 vim 9.1 에서 같은 키를 쳐서 확인한 것이다(ADR-0013).
func TestDeleteMotion(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want []string
	}{
		// `dw` 는 줄 끝에서 멈춘다. 다음 줄이 끌려 올라오지 않는다.
		{name: "dw 줄의 유일한 단어", data: "foo\nbar", keys: []string{"d", "w"}, want: []string{"", "bar"}},
		{name: "dw 줄의 마지막 단어", data: "foo bar\nbaz", col: 4, keys: []string{"d", "w"}, want: []string{"foo ", "baz"}},
		{name: "dw 줄 끝 공백까지", data: "foo   \nbar", keys: []string{"d", "w"}, want: []string{"", "bar"}},
		{name: "dw 파일 마지막 글자", data: "foo bar", col: 6, keys: []string{"d", "w"}, want: []string{"foo ba"}},
		{name: "dw 줄 안의 다음 단어", data: "foo bar\nbaz", keys: []string{"d", "w"}, want: []string{"bar", "baz"}},
		{name: "dw 들여쓰기는 남는다", data: "  foo\nbar", col: 2, keys: []string{"d", "w"}, want: []string{"  ", "bar"}},

		// 빈 줄에서는 그 줄이 지울 것이다.
		{name: "dw 빈 줄", data: "a\n\nb", line: 1, keys: []string{"d", "w"}, want: []string{"a", "b"}},
		{name: "dw 빈 줄 다음이 들여쓰기", data: "a\n\n  bar", line: 1, keys: []string{"d", "w"}, want: []string{"a", "  bar"}},
		{name: "dw 마지막 빈 줄은 갈 곳이 없다", data: "a\n\n", line: 1, keys: []string{"d", "w"}, want: []string{"a", ""}},

		// 숫자가 붙으면 중간 걸음은 줄을 넘는다. 마지막 걸음만 줄 끝에서 멈춘다.
		{name: "2dw 는 줄을 넘는다", data: "foo\nbar baz", keys: []string{"2", "d", "w"}, want: []string{"baz"}},
		{name: "2dw 줄 안", data: "foo bar baz", keys: []string{"2", "d", "w"}, want: []string{"baz"}},
		{name: "3d2w 는 여섯 단어", data: "a b c d e f g h", keys: []string{"3", "d", "2", "w"}, want: []string{"g h"}},

		{name: "dW 는 공백으로만 끊는다", data: "a.b c", keys: []string{"d", "W"}, want: []string{"c"}},

		// `e` `E` 는 커서가 선 글자까지 지운다.
		{name: "de", data: "foo bar\nbaz", col: 4, keys: []string{"d", "e"}, want: []string{"foo ", "baz"}},
		{name: "de 줄 안", data: "foo bar", keys: []string{"d", "e"}, want: []string{" bar"}},
		{name: "dE 는 공백으로만 끊는다", data: "a.b c", keys: []string{"d", "E"}, want: []string{" c"}},

		// 뒤로 가는 motion 은 커서가 범위의 끝이다.
		{name: "db", data: "foo bar", col: 4, keys: []string{"d", "b"}, want: []string{"bar"}},
		{name: "db 줄 앞에서 앞 줄과 합친다", data: "foo\nbar", line: 1, keys: []string{"d", "b"}, want: []string{"bar"}},
		{name: "db 파일 첫 글자에서는 아무 일도 없다", data: "foo", keys: []string{"d", "b"}, want: []string{"foo"}},
		{name: "d0", data: "foo bar", col: 4, keys: []string{"d", "0"}, want: []string{"bar"}},
		{name: "d^ 는 들여쓰기를 남긴다", data: "  foo bar", col: 8, keys: []string{"d", "^"}, want: []string{"  r"}},

		{name: "d$", data: "foo bar\nbaz", col: 1, keys: []string{"d", "$"}, want: []string{"f", "baz"}},
		{name: "2d$ 는 다음 줄 끝까지", data: "foo\nbar\nbaz", col: 1, keys: []string{"2", "d", "$"}, want: []string{"f", "baz"}},

		{name: "dl", data: "ab", col: 1, keys: []string{"d", "l"}, want: []string{"a"}},
		{name: "3dl 은 줄 끝에서 멈춘다", data: "abcd", keys: []string{"3", "d", "l"}, want: []string{"d"}},
		{name: "dh", data: "abcd", col: 2, keys: []string{"d", "h"}, want: []string{"acd"}},
		{name: "d← 는 dh 와 같다", data: "abcd", col: 2, keys: []string{"d", "left"}, want: []string{"acd"}},
		{name: "d→ 는 dl 과 같다", data: "abcd", col: 2, keys: []string{"d", "right"}, want: []string{"abd"}},

		// 줄 단위 motion 이다. 커서 칸과 상관없이 줄 전체가 사라진다.
		{name: "dd", data: "a\nb\nc", line: 1, keys: []string{"d", "d"}, want: []string{"a", "c"}},
		{name: "dd 커서 칸은 상관없다", data: "  aaa\nbbbb", col: 3, keys: []string{"d", "d"}, want: []string{"bbbb"}},
		{name: "3dd", data: "a\nb\nc\nd", keys: []string{"3", "d", "d"}, want: []string{"d"}},
		{name: "d3d 도 세 줄", data: "a\nb\nc\nd", keys: []string{"d", "3", "d"}, want: []string{"d"}},
		{name: "5dd 는 있는 줄까지만", data: "a\nb", keys: []string{"5", "d", "d"}, want: []string{""}},
		{name: "dd 한 줄뿐이면 빈 줄이 남는다", data: "only", keys: []string{"d", "d"}, want: []string{""}},

		{name: "dj 는 두 줄", data: "a\nb\nc", keys: []string{"d", "j"}, want: []string{"c"}},
		{name: "dj 마지막 줄에서는 아무 일도 없다", data: "a\nb", line: 1, keys: []string{"d", "j"}, want: []string{"a", "b"}},
		{name: "2dj 는 줄이 모자라면 파일 끝까지", data: "a\nb", keys: []string{"2", "d", "j"}, want: []string{""}},
		{name: "dk 는 두 줄", data: "a\nb\nc", line: 2, keys: []string{"d", "k"}, want: []string{"a"}},
		{name: "dk 첫 줄에서는 아무 일도 없다", data: "a\nb", keys: []string{"d", "k"}, want: []string{"a", "b"}},

		{name: "dG 는 파일 끝까지", data: "a\nb\nc", line: 1, keys: []string{"d", "G"}, want: []string{"a"}},
		{name: "d2G 는 그 줄까지", data: "a\nb\nc\nd", keys: []string{"d", "2", "G"}, want: []string{"c", "d"}},
		{name: "dgg 는 파일 처음까지", data: "a\nb\nc", line: 2, keys: []string{"d", "g", "g"}, want: []string{""}},
		{name: "10dgg 는 있는 줄까지만", data: "a\nb\nc", keys: []string{"1", "0", "d", "g", "g"}, want: []string{""}},

		// `x` 는 `dl` 과 같다.
		{name: "x", data: "ab", col: 1, keys: []string{"x"}, want: []string{"a"}},
		{name: "3x 는 줄 끝에서 멈춘다", data: "abcd", col: 2, keys: []string{"3", "x"}, want: []string{"ab"}},
		{name: "x 빈 줄에서는 아무 일도 없다", data: "\nb", keys: []string{"x"}, want: []string{"", "b"}},
		{name: "x 한글도 한 글자", data: "한글", keys: []string{"x"}, want: []string{"글"}},

		// 모르는 motion 은 아무 일도 하지 않는다. 잘못 누른 operator 를 무르는 길이기도 하다.
		{name: "짝 없는 조합", data: "foo", keys: []string{"d", "i"}, want: []string{"foo"}},
		{name: "esc 로 무른다", data: "foo", keys: []string{"d", "esc"}, want: []string{"foo"}},
		{name: "검색 키는 motion 이 아니다", data: "foo bar", keys: []string{"d", "n"}, want: []string{"foo bar"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, linesOf(buf))
		})
	}
}

// 지운 뒤 커서 자리다. vim 은 글자 단위면 지운 자리에, 줄 단위면 메운 줄의 첫 비공백에 둔다.
func TestDeleteCursor(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want [2]int
	}{
		{name: "지운 자리에 선다", data: "abcd", col: 2, keys: []string{"d", "h"}, want: [2]int{0, 1}},
		{name: "줄 끝을 지우면 마지막 글자로 당겨진다", data: "foo bar", col: 1, keys: []string{"d", "$"}, want: [2]int{0, 0}},
		{name: "뒤로 지우면 범위의 앞에 선다", data: "foo bar", col: 4, keys: []string{"d", "b"}, want: [2]int{0, 0}},
		{name: "줄을 지우면 메운 줄의 첫 비공백", data: "a\n  bb\nc", keys: []string{"d", "d"}, want: [2]int{0, 2}},
		{name: "마지막 줄을 지우면 앞 줄로", data: "a\nb", line: 1, keys: []string{"d", "d"}, want: [2]int{0, 0}},
		{name: "한 줄뿐이면 빈 줄에 선다", data: "only", col: 2, keys: []string{"d", "d"}, want: [2]int{0, 0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, [2]int{buf.cursorLine, buf.cursorCol})
		})
	}
}

// 지우기는 한 번이 한 되돌리기 구간이다. 앞의 타이핑에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
func TestDeleteUndoIsOwnStep(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	after := send(m, "i", "X", "esc", "d", "w", "d", "d")
	require.Equal(t, []string{"baz"}, linesOf(bufferOf(t, after)))

	after = send(after, "u")
	assert.Equal(t, []string{"bar", "baz"}, linesOf(bufferOf(t, after)), "dd 만 돌아온다")

	after = send(after, "u")
	assert.Equal(t, []string{"Xfoo bar", "baz"}, linesOf(bufferOf(t, after)), "dw 만 돌아온다")

	after = send(after, "u")
	assert.Equal(t, []string{"foo bar", "baz"}, linesOf(bufferOf(t, after)), "타이핑이 돌아온다")
}

func TestDeleteMarksDirty(t *testing.T) {
	buf := pressFrom(t, "foo", 0, 0, "x")
	assert.True(t, buf.dirty)

	// 아무것도 지우지 않았으면 파일은 그대로다.
	buf = pressFrom(t, "foo", 0, 0, "d", "i")
	assert.False(t, buf.dirty, "모르는 motion 은 파일을 건드리지 않는다")

	buf = pressFrom(t, "", 0, 0, "x")
	assert.False(t, buf.dirty, "빈 줄에서 x 는 지울 것이 없다")
}

// 지운 내용은 무명 register 에 남는다. 아직 읽는 곳은 없다.
func TestDeleteFillsRegister(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	after, ok := send(m, "d", "w").(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, [][]byte{[]byte("foo ")}, after.registers.unnamed.lines)
	assert.False(t, after.registers.unnamed.linewise)

	after, ok = send(after, "d", "d").(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, [][]byte{[]byte("bar")}, after.registers.unnamed.lines)
	assert.True(t, after.registers.unnamed.linewise, "줄 단위로 지웠다")
}

// 여러 줄에 걸친 글자 단위 지우기는 줄이 합쳐진다.
func TestDeleteJoinsLines(t *testing.T) {
	buf := pressFrom(t, "foo\nbar\nbaz", 2, 1, "d", "g", "g")
	require.Equal(t, []string{""}, linesOf(buf))

	buf = pressFrom(t, "abc\ndef", 1, 2, "d", "b")
	assert.Equal(t, []string{"abc", "f"}, linesOf(buf), "단어 처음까지 지운다")

	// 줄 시작에서 뒤로 가면 앞 줄의 단어 처음까지라, 줄바꿈까지 지워져 두 줄이 합쳐진다.
	buf = pressFrom(t, "abc\ndef", 1, 0, "d", "b")
	assert.Equal(t, []string{"def"}, linesOf(buf))
}

// 되돌리기가 지운 줄을 그대로 되살리는지 본다. replaceLines 가 제자리에서 늘리면 여기가 깨진다.
func TestDeleteLinesUndo(t *testing.T) {
	m := newTestEditor("a\nb\nc\nd", 80, 20)

	after := send(m, "j", "2", "d", "d")
	require.Equal(t, []string{"a", "d"}, linesOf(bufferOf(t, after)))

	after = send(after, "u")
	assert.Equal(t, []string{"a", "b", "c", "d"}, linesOf(bufferOf(t, after)))

	after = send(after, "ctrl+r")
	assert.Equal(t, []string{"a", "d"}, linesOf(bufferOf(t, after)))
}
