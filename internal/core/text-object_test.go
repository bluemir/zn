package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 단어 text object 다. `iw` `aw` `iW` `aW` 가 operator 여섯에 붙는다(ADR-0091).

// atCol 은 커서를 그 칸에 두고 시작한다. `l` 을 세는 것보다 읽힌다.
func atCol(m tea.Model, col int) tea.Model {
	for range col {
		m = send(m, "l")
	}

	return m
}

// `diw` 는 커서가 든 단어만 지운다. 둘레의 공백은 그대로다.
func TestDeleteInnerWord(t *testing.T) {
	tests := []struct {
		name string
		data string
		col  int
		keys []string
		want []string
	}{
		{name: "가운데 낱말", data: "foo bar baz\n", col: 4, keys: []string{"d", "i", "w"}, want: []string{"foo  baz"}},
		{name: "낱말 끝 글자에서도 같다", data: "foo bar baz\n", col: 6, keys: []string{"d", "i", "w"}, want: []string{"foo  baz"}},
		{name: "첫 낱말", data: "foo bar\n", col: 0, keys: []string{"d", "i", "w"}, want: []string{" bar"}},
		{name: "마지막 낱말", data: "foo bar\n", col: 5, keys: []string{"d", "i", "w"}, want: []string{"foo "}},

		// `aw` 는 뒤 공백을 먹는다. 뒤에 없으면 앞 공백이다.
		{name: "aw 는 뒤 공백까지", data: "foo bar baz\n", col: 4, keys: []string{"d", "a", "w"}, want: []string{"foo baz"}},
		{name: "aw 가 줄 끝이면 앞 공백", data: "foo bar\n", col: 5, keys: []string{"d", "a", "w"}, want: []string{"foo"}},
		{name: "aw 가 첫 낱말이면 뒤 공백", data: "foo bar\n", col: 0, keys: []string{"d", "a", "w"}, want: []string{"bar"}},

		// 부류가 바뀌는 자리가 경계다. `w` 가 걷는 것과 같다(cluster.go).
		{name: "문장부호는 따로 한 낱말", data: "foo.bar\n", col: 0, keys: []string{"d", "i", "w"}, want: []string{".bar"}},
		{name: "문장부호 위에서는 그것만", data: "foo.bar\n", col: 3, keys: []string{"d", "i", "w"}, want: []string{"foobar"}},
		{name: "한글과 영문이 갈린다", data: "한글abc\n", col: 0, keys: []string{"d", "i", "w"}, want: []string{"abc"}},

		// `iW` 는 공백으로만 끊는다.
		{name: "iW 는 문장부호를 안 끊는다", data: "foo.bar baz\n", col: 0, keys: []string{"d", "i", "W"}, want: []string{" baz"}},
		{name: "aW 는 뒤 공백까지", data: "foo.bar baz\n", col: 0, keys: []string{"d", "a", "W"}, want: []string{"baz"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = newTestEditor(tt.data, 40, 10)
			m = atCol(m, tt.col)
			m = send(m, tt.keys...)

			assert.Equal(t, tt.want, linesOf(bufferOf(t, m)))
		})
	}
}

// **공백 위에서는 아무 일도 하지 않는다.** vim 은 공백 덩어리를 지운다(ADR-0091 §2).
func TestTextObjectOnBlankDoesNothing(t *testing.T) {
	for _, keys := range [][]string{
		{"d", "i", "w"}, {"d", "a", "w"}, {"c", "i", "w"}, {"y", "i", "w"},
	} {
		t.Run(keys[1]+keys[2]+" of "+keys[0], func(t *testing.T) {
			var m tea.Model = newTestEditor("foo   bar\n", 40, 10)
			m = atCol(m, 4) // 공백 가운데

			m = send(m, keys...)

			assert.Equal(t, []string{"foo   bar"}, linesOf(bufferOf(t, m)), "파일이 그대로다")
			assert.False(t, bufferOf(t, m).dirty, "dirty 도 세우지 않는다")
			assert.IsType(t, viewEditorNormal{}, m, "insert 로도 들어가지 않는다")
		})
	}
}

// 빈 줄과 줄 끝도 같이 걸린다. classAt 이 줄 끝을 공백으로 본다.
func TestTextObjectOnEmptyLineDoesNothing(t *testing.T) {
	var m tea.Model = newTestEditor("foo\n\nbar\n", 40, 10)
	m = send(m, "j") // 빈 줄

	m = send(m, "d", "i", "w")

	assert.Equal(t, []string{"foo", "", "bar"}, linesOf(bufferOf(t, m)))
	assert.False(t, bufferOf(t, m).dirty)
}

// operator 여섯에 한꺼번에 붙는다. 범위 하나를 operate 가 갈라 준다.
func TestTextObjectAcrossOperators(t *testing.T) {
	t.Run("ciw 는 지우고 insert 로", func(t *testing.T) {
		var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
		m = atCol(m, 4)

		m = send(m, "c", "i", "w")

		require.IsType(t, viewEditorInsert{}, m, "insert 로 들어간다")
		assert.Equal(t, []string{"foo  baz"}, linesOf(bufferOf(t, m)))
		assert.Equal(t, 4, bufferOf(t, m).cursor.Col, "지운 자리에서 이어 친다")

		m = send(m, "x")
		assert.Equal(t, []string{"foo x baz"}, linesOf(bufferOf(t, m)))
	})

	t.Run("yiw 는 파일을 건드리지 않는다", func(t *testing.T) {
		var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
		m = atCol(m, 4)

		m = send(m, "y", "i", "w")

		assert.Equal(t, []string{"foo bar baz"}, linesOf(bufferOf(t, m)))
		assert.Contains(t, barOf(t, m)[1], "3 글자 복사되었습니다")

		// 담긴 것이 그 낱말이다.
		m = send(m, "$", "p")
		assert.Equal(t, []string{"foo bar bazbar"}, linesOf(bufferOf(t, m)))
	})

	t.Run(">iw 는 걸친 줄을 민다", func(t *testing.T) {
		var m tea.Model = newTestEditor("foo bar\n", 40, 10)
		m = atCol(m, 4)

		m = send(m, ">", "i", "w")

		assert.Equal(t, []string{"\tfoo bar"}, linesOf(bufferOf(t, m)), "들여쓰기는 줄의 성질이다")
	})
}

// register 이름도 그대로 받는다. 싣는 자리가 built 하나다(ADR-0058).
func TestTextObjectWithRegister(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
	m = atCol(m, 4)

	m = send(m, "\"", "a", "y", "i", "w")

	m = send(m, "$", "\"", "a", "p")

	assert.Equal(t, []string{"foo bar bazbar"}, linesOf(bufferOf(t, m)))
}

// **count 를 버린다.** operator 앞뒤 어느 쪽에 쳐도 같다(ADR-0091 §4).
func TestTextObjectIgnoresCount(t *testing.T) {
	for _, keys := range [][]string{
		{"d", "i", "w"},
		{"3", "d", "i", "w"},
		{"d", "3", "i", "w"},
	} {
		t.Run(strings.Join(keys, ""), func(t *testing.T) {
			var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
			m = atCol(m, 4)

			m = send(m, keys...)

			assert.Equal(t, []string{"foo  baz"}, linesOf(bufferOf(t, m)))
		})
	}
}

// **`i` 를 홀로 치면 insert mode 다.** text object 가 그것을 빼앗지 않는다(ADR-0091 §5).
func TestBareInsertKeysStillInsert(t *testing.T) {
	for _, key := range []string{"i", "a"} {
		t.Run(key, func(t *testing.T) {
			var m tea.Model = newTestEditor("foo\n", 40, 10)

			m = send(m, key)

			assert.IsType(t, viewEditorInsert{}, m)
		})
	}
}

// 짝이 없는 조합은 아무 일도 하지 않는다. `dgt` 와 같다.
func TestTextObjectUnknownObjectDoesNothing(t *testing.T) {
	for _, key := range []string{"z", "\"", "(", "esc", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			var m tea.Model = newTestEditor("foo bar\n", 40, 10)
			m = atCol(m, 4)

			m = send(m, "d", "i", key)

			assert.Equal(t, []string{"foo bar"}, linesOf(bufferOf(t, m)))
			assert.IsType(t, viewEditorNormal{}, m, "normal 로 돌아온다")
		})
	}
}

// showcmd 가 기다리는 중임을 보여준다.
func TestTextObjectShowcmd(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar\n", 40, 10)

	m = send(m, "d", "i")

	normal, ok := m.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, "di", normal.keyState().showcmd())

	m = send(newTestEditor("foo bar\n", 40, 10), "\"", "a", "2", "d", "a")
	normal, ok = m.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, `"a2da`, normal.keyState().showcmd())
}

// 한글 자판에서도 낸다. `ㅇ` 이 `d`, `ㅑ` 가 `i`, `ㅈ` 이 `w` 다(ADR-0008).
func TestTextObjectInHangul(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
	m = atCol(m, 4)

	m = send(m, "ㅇ", "ㅑ", "ㅈ")

	assert.Equal(t, []string{"foo  baz"}, linesOf(bufferOf(t, m)))
}

// visual 에는 넣지 않았다. `viw` 는 아무 일도 하지 않는다(ADR-0091 §6).
func TestTextObjectNotInVisual(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
	m = atCol(m, 4)

	m = send(m, "v", "i", "w")

	require.IsType(t, viewEditorVisual{}, m, "visual 에 머문다")
	assert.Equal(t, []string{"foo bar baz"}, linesOf(bufferOf(t, m)))
}

// `u` 한 번에 돌아온다. 범위 하나가 되돌리기 한 구간이다.
func TestTextObjectUndo(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar baz\n", 40, 10)
	m = atCol(m, 4)

	m = send(m, "d", "a", "w")
	require.Equal(t, []string{"foo baz"}, linesOf(bufferOf(t, m)))

	m = send(m, "u")

	assert.Equal(t, []string{"foo bar baz"}, linesOf(bufferOf(t, m)))
}
