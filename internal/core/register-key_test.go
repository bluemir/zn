package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `y`·`p` 와 register 칸을 키로 보는 시험이다.

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
