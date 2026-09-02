package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 아래 기대값은 vim 9.1 에서 같은 키를 쳐서 확인한 것이다(ADR-0017).
func TestPaste(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want []string
	}{
		// 글자 단위는 `p` 가 커서 뒤, `P` 가 커서 자리다.
		{name: "xp 는 글자를 뒤바꾼다", data: "abc", keys: []string{"x", "p"}, want: []string{"bac"}},
		{name: "xp 한글도 한 글자", data: "한글", keys: []string{"x", "p"}, want: []string{"글한"}},
		{name: "dw 뒤 p", data: "foo bar\nbaz", keys: []string{"d", "w", "p"}, want: []string{"bfoo ar", "baz"}},
		{name: "dw 뒤 P 는 제자리로 되돌린다", data: "foo bar\nbaz", keys: []string{"d", "w", "P"}, want: []string{"foo bar", "baz"}},
		{name: "줄 끝에서 p", data: "ab", col: 1, keys: []string{"y", "l", "p"}, want: []string{"abb"}},
		{name: "빈 줄에 p", data: "ab\n\ncd", keys: []string{"y", "l", "j", "p"}, want: []string{"ab", "a", "cd"}},

		// 줄 단위는 `p` 가 아래, `P` 가 위다. 커서 칸과 상관없다.
		{name: "dd 뒤 p", data: "a\n  bb\nc", line: 1, keys: []string{"d", "d", "p"}, want: []string{"a", "c", "  bb"}},
		{name: "dd 뒤 P", data: "a\n  bb\nc", line: 1, keys: []string{"d", "d", "P"}, want: []string{"a", "  bb", "c"}},
		{name: "마지막 줄 아래에 p", data: "a\nb", line: 1, keys: []string{"y", "y", "p"}, want: []string{"a", "b", "b"}},
		{name: "첫 줄 위에 P", data: "a\nb", keys: []string{"y", "y", "P"}, want: []string{"a", "a", "b"}},
		{name: "파일 끝에서 p", data: "a\nb\nc", keys: []string{"y", "y", "G", "p"}, want: []string{"a", "b", "c", "a"}},
		{name: "빈 buffer 에 p", data: "a", keys: []string{"y", "y", "d", "G", "p"}, want: []string{"", "a"}},

		// 숫자는 되풀이다.
		{name: "3p", data: "abc", keys: []string{"y", "l", "3", "p"}, want: []string{"aaaabc"}},
		{name: "3P", data: "abc", keys: []string{"y", "l", "3", "P"}, want: []string{"aaaabc"}},
		{name: "yy3p", data: "a\nb", keys: []string{"y", "y", "3", "p"}, want: []string{"a", "a", "a", "a", "b"}},

		// 여러 줄에 걸친 글자 단위다. 붙이면 줄이 갈린다.
		{name: "여러 줄 p", data: "foo\nbar\nbaz", col: 1,
			keys: []string{"2", "d", "$", "p"}, want: []string{"foo", "bar", "baz"}},
		{name: "여러 줄 P", data: "foo\nbar\nbaz", col: 1,
			keys: []string{"2", "d", "$", "P"}, want: []string{"oo", "barf", "baz"}},
		{name: "여러 줄 2p", data: "foo\nbar\nbaz", col: 1,
			keys: []string{"2", "d", "$", "2", "p"}, want: []string{"foo", "baroo", "bar", "baz"}},

		// register 가 비어 있으면 아무 일도 하지 않는다.
		{name: "빈 register 의 p", data: "abc", keys: []string{"p"}, want: []string{"abc"}},
		{name: "빈 register 의 P", data: "abc", keys: []string{"P"}, want: []string{"abc"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, linesOf(buf))
		})
	}
}

// 붙인 뒤 커서 자리다. vim 은 글자 단위 한 줄이면 마지막 글자, 여러 줄이면 첫 글자,
// 줄 단위면 붙인 첫 줄의 첫 비공백에 둔다.
func TestPasteCursor(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want [2]int
	}{
		{name: "한 줄이면 붙인 마지막 글자", data: "abc", keys: []string{"x", "p"}, want: [2]int{0, 1}},
		{name: "한 줄 p", data: "foo bar\nbaz", keys: []string{"d", "w", "p"}, want: [2]int{0, 4}},
		{name: "한 줄 P", data: "foo bar\nbaz", keys: []string{"d", "w", "P"}, want: [2]int{0, 3}},
		{name: "줄 끝에서 p", data: "ab", col: 1, keys: []string{"y", "l", "p"}, want: [2]int{0, 2}},
		{name: "3p 는 마지막 것의 끝", data: "abc", keys: []string{"y", "l", "3", "p"}, want: [2]int{0, 3}},
		{name: "빈 줄에 p", data: "ab\n\ncd", keys: []string{"y", "l", "j", "p"}, want: [2]int{1, 0}},

		{name: "여러 줄이면 붙인 첫 글자", data: "foo\nbar\nbaz", col: 1,
			keys: []string{"2", "d", "$", "p"}, want: [2]int{0, 1}},
		{name: "여러 줄 P", data: "foo\nbar\nbaz", col: 1,
			keys: []string{"2", "d", "$", "P"}, want: [2]int{0, 0}},

		{name: "줄 단위는 첫 비공백", data: "a\n  bb\nc", line: 1, keys: []string{"d", "d", "p"}, want: [2]int{2, 2}},
		{name: "줄 단위 P", data: "a\n  bb\nc", line: 1, keys: []string{"d", "d", "P"}, want: [2]int{1, 2}},
		{name: "파일 끝에 붙인 줄", data: "a\nb\nc", keys: []string{"y", "y", "G", "p"}, want: [2]int{3, 0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, [2]int{buf.cursor.Line, buf.cursor.Col})
		})
	}
}

// 복사한 뒤 커서 자리다. 뒤로 가는 motion 만 움직이고, 그 자리는 motion 이 커서를 두는 자리다.
//
// `yk` 가 지키는 칸은 이동 키가 남긴 desiredX 가라 커서를 손으로 놓지 않고 키로 옮겨간다.
// vim 의 curswant 와 같은 것이라, 자리를 놓기만 하면 vim 과 다른 것을 재게 된다.
func TestYankCursor(t *testing.T) {
	tests := []struct {
		name string
		data string
		keys []string
		want [2]int
	}{
		{name: "yw 는 제자리", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "w"}, want: [2]int{1, 4}},
		{name: "yy 는 제자리", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "y"}, want: [2]int{1, 4}},
		{name: "y$ 는 제자리", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "$"}, want: [2]int{1, 4}},
		{name: "yb 는 범위 시작으로", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "b"}, want: [2]int{1, 0}},
		{name: "y0 는 줄 시작으로", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "0"}, want: [2]int{1, 0}},
		{name: "yk 는 칸을 지킨다", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "k"}, want: [2]int{0, 4}},
		{name: "yk 는 짧은 줄에서 끌려온다", data: "ab\nbaz qux",
			keys: []string{"j", "4", "l", "y", "k"}, want: [2]int{0, 1}},
		{name: "ygg 는 첫 비공백으로", data: "  foo bar\nbaz qux",
			keys: []string{"j", "4", "l", "y", "g", "g"}, want: [2]int{0, 2}},
		{name: "yG 는 제자리", data: "  foo bar\nbaz qux\n  quux",
			keys: []string{"j", "4", "l", "y", "G"}, want: [2]int{1, 4}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, 0, 0, test.keys...)

			assert.Equal(t, test.want, [2]int{buf.cursor.Line, buf.cursor.Col})
		})
	}
}

func TestPasteMarksDirty(t *testing.T) {
	buf := pressFrom(t, "abc", 0, 0, "y", "l", "p")
	assert.True(t, buf.dirty)

	buf = pressFrom(t, "abc", 0, 0, "p")
	assert.False(t, buf.dirty, "빈 register 는 파일을 건드리지 않는다")
}
