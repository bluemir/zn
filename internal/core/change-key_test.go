package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bluemir/zn/internal/textarea"
)

// `c`·`cc`·`cw` 를 키로 보는 시험이다.

// 아래 기대값은 vim 9.1 에서 같은 키를 쳐서 확인한 것이다.
func TestChange(t *testing.T) {
	tests := []struct {
		name string
		data string
		Line int
		Col  int
		keys []string
		want []string
	}{
		// `cw` 는 `ce` 다. 단어 뒤 공백이 남는 것이 `dw` 와 갈리는 자리다.
		{name: "cw 는 단어 뒤 공백을 남긴다", data: "foo    bar",
			keys: []string{"c", "w"}, want: []string{"    bar"}},
		{name: "같은 자리의 dw 는 공백까지 지운다", data: "foo    bar",
			keys: []string{"d", "w"}, want: []string{"bar"}},
		{name: "단어 마지막 글자의 cw 는 그 글자 하나다", data: "foo bar", Col: 2,
			keys: []string{"c", "w"}, want: []string{"fo bar"}},
		{name: "단어 가운데의 cw 는 끝까지다", data: "foobar baz", Col: 3,
			keys: []string{"c", "w"}, want: []string{"foo baz"}},
		{name: "공백 위의 cw 는 dw 와 같다", data: "foo    bar", Col: 3,
			keys: []string{"c", "w"}, want: []string{"foobar"}},
		{name: "c2w 는 둘째 단어의 끝까지다", data: "foo bar baz",
			keys: []string{"c", "2", "w"}, want: []string{" baz"}},
		{name: "cW 는 공백으로만 끊는다", data: "foo.bar baz",
			keys: []string{"c", "W"}, want: []string{" baz"}},
		{name: "cw 는 줄을 넘지 않는다", data: "foo\nbar", Col: 2,
			keys: []string{"c", "w"}, want: []string{"fo", "bar"}},
		{name: "빈 줄의 cw 는 아무것도 바꾸지 않는다", data: "\nbar",
			keys: []string{"c", "w"}, want: []string{"", "bar"}},
		{name: "cw 한글은 한 글자", data: "한글 abc", Col: 3,
			keys: []string{"c", "w"}, want: []string{"한 abc"}},

		// `cc` 는 줄을 없애지 않고 들여쓰기만 남긴다.
		{name: "cc 는 들여쓰기를 남긴다", data: "\tfoo",
			keys: []string{"c", "c"}, want: []string{"\t"}},
		{name: "cc 는 space 들여쓰기도 남긴다", data: "    foo",
			keys: []string{"c", "c"}, want: []string{"    "}},
		{name: "들여쓰기가 없으면 빈 줄", data: "foo",
			keys: []string{"c", "c"}, want: []string{""}},
		{name: "dd 와 달리 줄이 사라지지 않는다", data: "a\n  b\nc", Line: 1,
			keys: []string{"c", "c"}, want: []string{"a", "  ", "c"}},
		{name: "3cc 는 세 줄을 한 줄로 합친다", data: "  a\n  b\n  c\nd",
			keys: []string{"3", "c", "c"}, want: []string{"  ", "d"}},
		{name: "줄이 모자라면 있는 만큼만", data: "  a\n  b",
			keys: []string{"5", "c", "c"}, want: []string{"  "}},
		{name: "cj 도 줄 단위다", data: "  a\n  b\nc",
			keys: []string{"c", "j"}, want: []string{"  ", "c"}},
		{name: "ck 는 위로 줄 단위다", data: "  a\n\tb\nc", Line: 1,
			keys: []string{"c", "k"}, want: []string{"  ", "c"}},
		{name: "cG 는 파일 끝까지", data: "\ta\nb\nc",
			keys: []string{"c", "G"}, want: []string{"\t"}},
		{name: "cgg 는 파일 처음까지", data: "a\nb\n  c", Line: 2,
			keys: []string{"c", "g", "g"}, want: []string{""}},
		{name: "cj 가 마지막 줄이면 아무 일도 없다", data: "a\nb", Line: 1,
			keys: []string{"c", "j"}, want: []string{"a", "b"}},

		// 나머지 motion 은 `d` 와 같은 범위다.
		{name: "c$", data: "abcdef", Col: 2, keys: []string{"c", "$"}, want: []string{"ab"}},
		{name: "c0", data: "abcdef", Col: 3, keys: []string{"c", "0"}, want: []string{"def"}},
		{name: "cb", data: "foo bar", Col: 4, keys: []string{"c", "b"}, want: []string{"bar"}},
		{name: "ce", data: "foo bar", keys: []string{"c", "e"}, want: []string{" bar"}},
		{name: "cl 은 한 글자", data: "abc", keys: []string{"c", "l"}, want: []string{"bc"}},
		{name: "c3l", data: "abcdef", keys: []string{"c", "3", "l"}, want: []string{"def"}},
		{name: "3c2l 은 곱해서 여섯 글자", data: "abcdefg",
			keys: []string{"3", "c", "2", "l"}, want: []string{"g"}},

		// 모르는 motion 은 아무 일도 하지 않는다.
		{name: "cq 는 무른다", data: "abc", keys: []string{"c", "q"}, want: []string{"abc"}},
		{name: "c 뒤의 esc 도 무른다", data: "abc", keys: []string{"c", "esc"}, want: []string{"abc"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.Line, test.Col, test.keys...)

			assert.Equal(t, test.want, linesOf(buf))
		})
	}
}

// `c` 는 지우고 insert mode 로 들어간다. 이어 친 글자가 지운 자리에 들어가는지 본다.
func TestChangeEntersInsert(t *testing.T) {
	tests := []struct {
		name string
		data string
		Line int
		Col  int
		keys []string
		want []string
	}{
		{name: "cw 뒤에 이어 친다", data: "foo    bar",
			keys: []string{"c", "w", "X", "Y"}, want: []string{"XY    bar"}},
		{name: "cc 는 들여쓰기 뒤에서 시작한다", data: "\tfoo",
			keys: []string{"c", "c", "X"}, want: []string{"\tX"}},
		{name: "c$ 는 줄 끝 다음 칸에서 시작한다", data: "abcdef", Col: 2,
			keys: []string{"c", "$", "X"}, want: []string{"abX"}},
		{name: "빈 줄의 cw 도 insert 로 간다", data: "\nbar",
			keys: []string{"c", "w", "X"}, want: []string{"X", "bar"}},
		{name: "모르는 motion 이면 mode 도 그대로다", data: "abc",
			keys: []string{"c", "q", "x"}, want: []string{"bc"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.Line, test.Col, test.keys...)

			assert.Equal(t, test.want, linesOf(buf))
		})
	}
}

// 바꾼 자리의 커서다. insert mode 라 줄 끝 다음 칸에 설 수 있다.
func TestChangeCursor(t *testing.T) {
	tests := []struct {
		name              string
		data              string
		Line, Col         int
		keys              []string
		wantLine, wantCol int
	}{
		{name: "cw 는 지운 자리다", data: "foo bar", keys: []string{"c", "w"}, wantCol: 0},
		{name: "cc 는 들여쓰기 다음 칸", data: "\t\tfoo", keys: []string{"c", "c"}, wantCol: 2},
		{name: "c$ 는 줄 끝 다음 칸", data: "abcdef", Col: 2, keys: []string{"c", "$"}, wantCol: 2},
		{name: "cb 는 범위의 시작", data: "foo bar", Col: 4, keys: []string{"c", "b"}, wantCol: 0},
		{name: "ck 는 위 줄의 들여쓰기 뒤", data: "  a\n  b", Line: 1,
			keys: []string{"c", "k"}, wantLine: 0, wantCol: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.Line, test.Col, test.keys...)

			assert.Equal(t, test.wantLine, buf.Cursor.Line)
			assert.Equal(t, test.wantCol, buf.Cursor.Col)
		})
	}
}

// `c` 는 지우기와 이어 친 글자가 한 동작이라 `u` 한 번에 함께 돌아간다. vim 과 같다.
func TestChangeUndoIsOneStep(t *testing.T) {
	tests := []struct {
		name string
		data string
		keys []string
	}{
		{name: "cw 와 이어 친 글자", data: "foo    bar", keys: []string{"c", "w", "X", "Y"}},
		{name: "cc 와 이어 친 글자", data: "\tfoo", keys: []string{"c", "c", "X"}},
		{name: "3cc 와 이어 친 글자", data: "  a\n  b\n  c", keys: []string{"3", "c", "c", "X"}},
		{name: "c$ 와 이어 친 글자", data: "abcdef", keys: []string{"c", "$", "X"}},
		{name: "친 글자가 없어도", data: "foo bar", keys: []string{"c", "w"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := linesOf(pressFrom(t, test.data, 0, 0))

			keys := append(append([]string(nil), test.keys...), "esc", "u")
			buf := pressFrom(t, test.data, 0, 0, keys...)

			assert.Equal(t, before, linesOf(buf))
		})
	}
}

// 바꾼 내용은 register 에 남는다. `d` 와 같다.
func TestChangeFillsRegister(t *testing.T) {
	// `cw` 로 지운 `foo` 를 다음 줄에 붙인다.
	buf := pressFrom(t, "foo bar\nbaz", 0, 0, "c", "w", "esc", "j", "p")
	assert.Equal(t, []string{" bar", "bfooaz"}, linesOf(buf))

	// 줄 단위는 줄로 붙는다.
	buf = pressFrom(t, "  a\nb", 0, 0, "c", "c", "esc", "j", "p")
	assert.Equal(t, []string{"  ", "b", "  a"}, linesOf(buf))

	// 바꿀 것이 없었으면 register 를 건드리지 않는다.
	buf = pressFrom(t, "abc\n\nx", 0, 0, "y", "y", "j", "c", "w", "esc", "p")
	assert.Equal(t, []string{"abc", "", "abc", "x"}, linesOf(buf))
}

// `cw` 는 파서에서 `dw` `yw` 와 같은 자리를 쓴다. 다만 motion 이 갈린다 —
// `cw` 는 `ce` 라서 `c` 뒤의 `w` 만 motionChangeWord 다(ADR-0033).
func TestChangeKeyParser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: "cw 는 motion 이 갈린다", keys: []string{"c", "w"},
			want: actionChange{motion: motionChangeWord{kind: textarea.WordSmall}}},
		{name: "cW 도 같다", keys: []string{"c", "W"},
			want: actionChange{motion: motionChangeWord{kind: textarea.WordBig}}},
		{name: "dw 는 그대로다", keys: []string{"d", "w"},
			want: actionDelete{motion: motionWordForward{kind: textarea.WordSmall}}},
		{name: "cc", keys: []string{"c", "c"}, want: actionChange{motion: motionWholeLines{}}},
		{name: "3cc", keys: []string{"3", "c", "c"}, want: actionChange{motion: motionWholeLines{}, count: 3}},
		{name: "c3w", keys: []string{"c", "3", "w"},
			want: actionChange{motion: motionChangeWord{kind: textarea.WordSmall}, count: 3}},
		{name: "3c2w", keys: []string{"3", "c", "2", "w"},
			want: actionChange{motion: motionChangeWord{kind: textarea.WordSmall}, count: 6}},
		{name: "cgg", keys: []string{"c", "g", "g"}, want: actionChange{motion: motionToFirstLine{}}},
		{name: "c3c 는 3cc 와 같다", keys: []string{"c", "3", "c"},
			want: actionChange{motion: motionWholeLines{}, count: 3}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := pressAll(test.keys...)

			assert.Equal(t, test.want, got)
		})
	}
}

// showcmd 는 치는 동안 지금까지 먹은 키를 그대로 보여준다.
func TestChangeShowcmd(t *testing.T) {
	var state normalState = normalStart{}

	_, state = state.press("3")
	assert.Equal(t, "3", state.showcmd())

	_, state = state.press("c")
	assert.Equal(t, "3c", state.showcmd())

	_, state = state.press("2")
	assert.Equal(t, "3c2", state.showcmd())
}
