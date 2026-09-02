package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			assert.Equal(t, test.want, [2]int{buf.cursor.line, buf.cursor.col})
		})
	}
}

// `y` 는 register 만 채운다. 파일도 dirty 도 건드리지 않는다.
func TestYankFillsRegister(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		want     [][]byte
		linewise bool
	}{
		{name: "yy", keys: []string{"y", "y"}, want: [][]byte{[]byte("foo bar")}, linewise: true},
		{name: "2yy", keys: []string{"2", "y", "y"},
			want: [][]byte{[]byte("foo bar"), []byte("baz")}, linewise: true},
		{name: "yw", keys: []string{"y", "w"}, want: [][]byte{[]byte("foo ")}},
		{name: "y$", keys: []string{"y", "$"}, want: [][]byte{[]byte("foo bar")}},
		// `w` 는 operator 와 함께 쓰이면 마지막 걸음이 줄을 넘지 않는다. `dw` 와 같다(ADR-0013).
		{name: "y2w 는 줄 끝에서 멈춘다", keys: []string{"y", "2", "w"}, want: [][]byte{[]byte("foo bar")}},
		{name: "ygg 는 줄 단위", keys: []string{"y", "g", "g"},
			want: [][]byte{[]byte("foo bar")}, linewise: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTestEditor("foo bar\nbaz", 80, 20)

			after, ok := send(m, test.keys...).(viewEditorNormal)
			require.True(t, ok)

			assert.Equal(t, test.want, after.registers.unnamed.lines)
			assert.Equal(t, test.linewise, after.registers.unnamed.linewise)
			assert.Equal(t, []string{"foo bar", "baz"}, linesOf(after.buffers[after.active]), "파일은 그대로다")
			assert.False(t, after.buffers[after.active].dirty, "복사는 파일을 바꾸지 않는다")
		})
	}
}

// 복사한 뒤 커서 자리다. 뒤로 가는 motion 만 움직이고, 그 자리는 motion 이 커서를 두는 자리다.
//
// `yk` 가 지키는 칸은 이동 키가 남긴 desiredX 이라 커서를 손으로 놓지 않고 키로 옮겨간다.
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

			assert.Equal(t, test.want, [2]int{buf.cursor.line, buf.cursor.col})
		})
	}
}

// 붙여넣기 한 번이 되돌리기 한 구간이다. `3p` 도 `u` 한 번에 전부 사라진다.
func TestPasteUndoIsOwnStep(t *testing.T) {
	m := newTestEditor("foo\nbar", 80, 20)

	after := send(m, "y", "y", "i", "X", "esc", "3", "p")
	require.Equal(t, []string{"Xfoo", "foo", "foo", "foo", "bar"}, linesOf(bufferOf(t, after)))

	after = send(after, "u")
	assert.Equal(t, []string{"Xfoo", "bar"}, linesOf(bufferOf(t, after)), "붙인 세 줄이 한 번에 사라진다")

	after = send(after, "u")
	assert.Equal(t, []string{"foo", "bar"}, linesOf(bufferOf(t, after)), "타이핑이 돌아온다")

	after = send(after, "ctrl+r", "ctrl+r")
	assert.Equal(t, []string{"Xfoo", "foo", "foo", "foo", "bar"}, linesOf(bufferOf(t, after)))
}

// 글자 단위 붙여넣기도 제 구간이다.
func TestPasteTextUndo(t *testing.T) {
	m := newTestEditor("abc", 80, 20)

	after := send(m, "x", "p")
	require.Equal(t, []string{"bac"}, linesOf(bufferOf(t, after)))

	after = send(after, "u")
	assert.Equal(t, []string{"bc"}, linesOf(bufferOf(t, after)), "붙인 것만 돌아간다")

	after = send(after, "u")
	assert.Equal(t, []string{"abc"}, linesOf(bufferOf(t, after)), "지운 것이 돌아온다")
}

func TestPasteMarksDirty(t *testing.T) {
	buf := pressFrom(t, "abc", 0, 0, "y", "l", "p")
	assert.True(t, buf.dirty)

	buf = pressFrom(t, "abc", 0, 0, "p")
	assert.False(t, buf.dirty, "빈 register 는 파일을 건드리지 않는다")
}

// register 가 editor 에 있어서 tab 을 넘어 붙는다. 그것이 Buffer 밖에 둔 이유다(ADR-0013).
func TestPasteAcrossTabs(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")

	after := send(m, "y", "y", "g", "t", "p")

	buf := bufferOf(t, after)
	assert.Equal(t, []string{"a", "a", "b", "c"}, linesOf(buf))
}

// `y` 는 무엇을 복사했는지 아래 줄에 알린다. 복사는 화면에 자국을 남기지 않아서
// 알림이 없으면 키가 먹었는지 볼 길이 없다.
func TestYankTellsWhatWasCopied(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "yy", keys: []string{"y", "y"}, want: "1 줄 복사되었습니다"},
		{name: "2yy", keys: []string{"2", "y", "y"}, want: "2 줄 복사되었습니다"},
		{name: "yw", keys: []string{"y", "w"}, want: "4 글자 복사되었습니다"},
		{name: "y$", keys: []string{"y", "$"}, want: "7 글자 복사되었습니다"},
		// 한글 한 자는 3 byte 지만 한 글자다.
		{name: "한글 줄의 y$", keys: []string{"j", "j", "y", "$"}, want: "3 글자 복사되었습니다"},
		{name: "visual 의 y", keys: []string{"v", "l", "y"}, want: "2 글자 복사되었습니다"},
		{name: "visual line 의 y", keys: []string{"V", "j", "y"}, want: "2 줄 복사되었습니다"},
		// 줄을 넘어 고른 글자 단위는 사이의 줄바꿈도 한 글자다. `r` + 줄바꿈 + `baz` 다.
		{name: "줄을 넘는 visual 의 y", keys: []string{"$", "v", "j", "y"}, want: "5 글자 복사되었습니다"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := send(newTestEditor("foo bar\nbaz\n가나다", 80, 20), test.keys...)

			assert.Contains(t, barOf(t, m)[1], test.want)
		})
	}
}

// 알림은 다음 키에 사라지고, 복사할 것이 없으면 애초에 뜨지 않는다.
func TestYankMessageIsOnlyForWhatWasCopied(t *testing.T) {
	t.Run("다음 키를 누르면 사라진다", func(t *testing.T) {
		m := send(newTestEditor("foo bar\nbaz", 80, 20), "y", "y", "j")

		assert.NotContains(t, barOf(t, m)[1], "복사되었습니다")
	})

	t.Run("복사한 것이 없으면 알리지 않는다", func(t *testing.T) {
		// 빈 줄의 `y$` 는 잡을 범위가 없다.
		m := send(newTestEditor("\nbaz", 80, 20), "y", "$")

		assert.NotContains(t, barOf(t, m)[1], "복사되었습니다")
	})
}

// 이름을 대면 그 register 를 붙인다. 링에 남은 옛것을 꺼내는 것이 숫자 register 의 쓸모다(ADR-0058).
func TestPasteFromNumberedRegister(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []string
	}{
		// 셋을 지운 뒤라 남은 것은 `four` 하나다. 링은 `"1`=three `"2`=two `"3`=one 이다.
		{name: `"1p 는 마지막에 지운 것`, keys: []string{`"`, "1", "p"},
			want: []string{"four", "three"}},
		{name: `"3p 는 처음에 지운 것`, keys: []string{`"`, "3", "p"},
			want: []string{"four", "one"}},
		{name: `"3P 는 위에 붙인다`, keys: []string{`"`, "3", "P"},
			want: []string{"one", "four"}},
		// 이름을 대지 않은 것은 무명이고, 무명은 마지막으로 지운 것이다.
		{name: "p 는 무명", keys: []string{"p"},
			want: []string{"four", "three"}},
		// `"0` 은 복사 전용이라 세 번 지워도 그대로다.
		{name: `"0p 는 복사한 것`, keys: []string{`"`, "0", "p"},
			want: []string{"four", "one"}},
		{name: `2"1p 는 두 번 붙인다`, keys: []string{"2", `"`, "1", "p"},
			want: []string{"four", "three", "three"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// `yy` 로 `"0` 을 채우고 `dd` 셋으로 링을 채운다.
			m := send(newTestEditor("one\ntwo\nthree\nfour", 80, 20),
				"y", "y", "d", "d", "d", "d", "d", "d")

			after := send(m, test.keys...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, after)))
		})
	}
}

// 빈 register 를 붙이면 아무 일도 하지 않는다. 무명이 비었을 때와 같은 규칙이다(ADR-0017).
//
// 아직 없는 이름(`"a`) 도 여기로 온다 — 문자 register 를 넣기 전까지 늘 비어 있다.
func TestPasteFromEmptyRegisterIsQuiet(t *testing.T) {
	for _, name := range []string{"9", "a"} {
		t.Run(`"`+name, func(t *testing.T) {
			m := send(newTestEditor("one\ntwo", 80, 20), `"`, name, "p")

			buf := bufferOf(t, m)
			assert.Equal(t, []string{"one", "two"}, linesOf(buf))
			assert.False(t, buf.dirty, "붙인 것이 없으면 dirty 도 서지 않는다")
		})
	}
}

// 이름을 고른 뒤 붙여넣기가 아닌 키가 오면 파일도 register 도 그대로다(ADR-0058).
func TestNamedRegisterOnlyFeedsPaste(t *testing.T) {
	m := send(newTestEditor("one\ntwo\nthree", 80, 20), "d", "d")

	after := send(m, `"`, "1", "y", "y")

	assert.Equal(t, []string{"two", "three"}, linesOf(bufferOf(t, after)), "파일은 그대로다")
	assert.Equal(t, map[string]string{
		`""`: "one⏎",
		`"1`: "one⏎",
	}, registersOf(t, after), "담기지도 않았다")
	assert.NotContains(t, barOf(t, after)[1], "복사되었습니다")
}
