package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `v` `V` 로 들어가고 `esc` 로 나온다. mode 이름이 갈래를 보여준다.
func TestVisualEntersAndLeaves(t *testing.T) {
	t.Run("v 는 글자 단위", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v")

		visual, ok := m.(viewEditorVisual)
		require.True(t, ok, "visual mode 여야 한다")
		assert.False(t, visual.activeBuffer().selection.linewise)
		assert.Equal(t, "VISUAL", modeOf(t, m))
	})

	t.Run("V 는 줄 단위", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "V")

		visual, ok := m.(viewEditorVisual)
		require.True(t, ok)
		assert.True(t, visual.activeBuffer().selection.linewise)
		assert.Contains(t, barOf(t, m)[0], "VISUAL LINE")
	})

	t.Run("esc 로 나오면 고른 것도 사라진다", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "esc")

		require.IsType(t, viewEditorNormal{}, m)
		assert.False(t, bufferOf(t, m).selection.active, "normal 에는 고른 범위가 없다")
	})

	t.Run("같은 키를 다시 치면 나간다", func(t *testing.T) {
		assert.IsType(t, viewEditorNormal{}, send(newTestEditor("foo", 80, 20), "v", "v"))
		assert.IsType(t, viewEditorNormal{}, send(newTestEditor("foo", 80, 20), "V", "V"))
	})

	t.Run("다른 키면 갈래만 바뀐다", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "V")

		visual, ok := m.(viewEditorVisual)
		require.True(t, ok, "visual 에 머문다")
		assert.True(t, visual.activeBuffer().selection.linewise)
		assert.Equal(t, 0, visual.activeBuffer().selection.line, "anchor 는 그대로다")
	})
}

// 고른 범위는 커서가 선 글자까지다. vim 의 visual 은 inclusive 다.
func TestVisualDeleteTakesRange(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []string
	}{
		{name: "vd 는 x 와 같다", keys: []string{"v", "d"}, want: []string{"oo bar", "baz qux"}},
		{name: "vld 는 두 글자", keys: []string{"v", "l", "d"}, want: []string{"o bar", "baz qux"}},
		{name: "v$d 는 줄 끝까지", keys: []string{"v", "$", "d"}, want: []string{"", "baz qux"}},
		{name: "vwd", keys: []string{"v", "w", "d"}, want: []string{"ar", "baz qux"}},
		{name: "vjd 는 줄을 넘는다", keys: []string{"v", "j", "d"}, want: []string{"az qux"}},
		{name: "x 는 d 와 같다", keys: []string{"v", "l", "x"}, want: []string{"o bar", "baz qux"}},
		// 뒤로 골라도 범위는 같다. anchor 가 끝이 된다.
		{name: "뒤로 고르기", keys: []string{"l", "l", "v", "h", "h", "d"},
			want: []string{" bar", "baz qux"}},
		{name: "숫자 접두", keys: []string{"v", "3", "l", "d"}, want: []string{"bar", "baz qux"}},
		// `v` 는 글자 단위라 `G` 도 줄 단위가 되지 않는다. 마지막 줄 첫 글자까지다. vim 과 같다.
		{name: "vG 는 마지막 줄 첫 글자까지", keys: []string{"v", "G", "d"}, want: []string{"az qux"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := send(newTestEditor("foo bar\nbaz qux", 80, 20), test.keys...)

			require.IsType(t, viewEditorNormal{}, m, "지우고 나면 normal 이다")
			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

// grapheme cluster 하나가 한 글자다. 한글·이모지도 통째로 든다.
func TestVisualDeleteTakesWholeCluster(t *testing.T) {
	m := send(newTestEditor("한글abc", 80, 20), "v", "d")

	assert.Equal(t, []string{"글abc"}, linesOf(bufferOf(t, m)))
}

// `V` 는 커서 칸과 상관없이 줄 전체다. `dd` 와 같은 자리로 간다.
func TestVisualLineDeleteTakesWholeLines(t *testing.T) {
	m := send(newTestEditor("foo\n\tbar\nbaz", 80, 20), "l", "V", "j", "d")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"baz"}, linesOf(buf))
	assert.Equal(t, 0, buf.cursorLine)
	assert.Equal(t, 0, buf.cursorCol, "메운 줄의 첫 비공백이다")
}

// `y` 는 파일을 건드리지 않고 커서를 범위의 시작에 놓는다.
func TestVisualYankFillsRegister(t *testing.T) {
	t.Run("글자 단위", func(t *testing.T) {
		after, ok := send(newTestEditor("foo bar\nbaz", 80, 20), "l", "l", "v", "h", "h", "y").(viewEditorNormal)
		require.True(t, ok)

		assert.Equal(t, [][]byte{[]byte("foo")}, after.registers.unnamed.lines)
		assert.False(t, after.registers.unnamed.linewise)
		assert.Equal(t, []string{"foo bar", "baz"}, linesOf(after.buffers[after.active]), "파일은 그대로다")
		assert.False(t, after.buffers[after.active].dirty)
		assert.Equal(t, 0, after.buffers[after.active].cursorCol, "커서는 범위의 시작이다")
	})

	t.Run("줄 단위", func(t *testing.T) {
		after, ok := send(newTestEditor("foo bar\nbaz", 80, 20), "V", "j", "y").(viewEditorNormal)
		require.True(t, ok)

		assert.Equal(t, [][]byte{[]byte("foo bar"), []byte("baz")}, after.registers.unnamed.lines)
		assert.True(t, after.registers.unnamed.linewise)
	})

	t.Run("줄 단위로 복사한 것은 줄로 붙는다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar", 80, 20), "V", "y", "p")

		assert.Equal(t, []string{"foo", "foo", "bar"}, linesOf(bufferOf(t, m)))
	})
}

// `c` 는 지우고 insert mode 로 간다. 지운 것과 이어 친 글자가 한 번의 `u` 로 함께 돌아간다.
func TestVisualChange(t *testing.T) {
	t.Run("글자 단위", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "l", "c")

		require.IsType(t, viewEditorInsert{}, m)
		assert.Equal(t, []string{" bar"}, linesOf(bufferOf(t, m)))
	})

	t.Run("줄 단위는 줄을 없애지 않고 들여쓰기를 남긴다", func(t *testing.T) {
		m := send(newTestEditor("func {\n\tfoo\n\tbar\n}", 80, 20), "j", "V", "j", "c")

		require.IsType(t, viewEditorInsert{}, m)
		assert.Equal(t, []string{"func {", "\t", "}"}, linesOf(bufferOf(t, m)))
	})

	t.Run("지운 것과 이어 친 것이 u 한 번에 돌아간다", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "l", "c", "X", "esc", "u")

		assert.Equal(t, []string{"foo bar"}, linesOf(bufferOf(t, m)))
	})

	t.Run("고른 것을 놓고 insert 로 간다", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "c")

		assert.False(t, bufferOf(t, m).selection.active)
	})
}

// visual 이 받지 않는 키는 아무 일도 하지 않고 그대로 머문다.
func TestVisualIgnoresUnboundKeys(t *testing.T) {
	for _, k := range []string{":", "/", "?", "ctrl+p", "ctrl+w", "i", "o", "p", "u"} {
		t.Run(k, func(t *testing.T) {
			m := send(newTestEditor("foo bar\nbaz", 80, 20), "v", k)

			assert.IsType(t, viewEditorVisual{}, m, "visual 에 머문다")
			assert.Equal(t, []string{"foo bar", "baz"}, linesOf(bufferOf(t, m)), "파일은 그대로다")
		})
	}
}

// `g` 는 visual 에서도 접두 키다. 짝이 없는 조합은 아무 일도 하지 않는다.
func TestVisualPrefixKey(t *testing.T) {
	t.Run("vgg 는 파일 처음까지 거슬러 고른다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar\nbaz", 80, 20), "j", "j", "v", "g", "g", "d")

		// 글자 단위라 커서가 선 `b` 까지다. 줄 단위로 지우려면 `Vgg` 다.
		assert.Equal(t, []string{"az"}, linesOf(bufferOf(t, m)))
	})

	t.Run("Vgg 는 줄 단위", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar\nbaz", 80, 20), "j", "j", "V", "g", "g", "d")

		assert.Equal(t, []string{""}, linesOf(bufferOf(t, m)))
	})

	t.Run("gt 는 짝이 없다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar", 80, 20), "v", "g", "t")

		assert.IsType(t, viewEditorVisual{}, m)
	})

	t.Run("showcmd 에 보인다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar", 80, 20), "v", "1", "2", "g")

		assert.Contains(t, barOf(t, m)[1], "12g")
	})
}

// 한글 입력 상태의 키는 두벌식 자리의 영문 키로 되돌려 받는다(ADR-0008).
func TestVisualTakesHangulKeys(t *testing.T) {
	t.Run("ㅍ 이 v 다", func(t *testing.T) {
		assert.IsType(t, viewEditorVisual{}, send(newTestEditor("foo", 80, 20), "ㅍ"))
	})

	t.Run("ㅇ 이 d 다", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "ㅍ", "ㅣ", "ㅇ")

		assert.Equal(t, []string{"o bar"}, linesOf(bufferOf(t, m)))
	})
}

// 고른 범위는 화면에 칠해진다.
func TestVisualPaintsSelection(t *testing.T) {
	// 줄번호 칸이 붙은 그대로 본다. textOf 는 행 맨 앞의 색을 걷어내므로 0 번 칸에서
	// 시작하는 선택이 보이지 않는다.
	t.Run("고른 구간만", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "l")

		assert.Contains(t, contentRowsOf(t, m)[0], styleSelection.Render("foo"))
		assert.NotContains(t, contentRowsOf(t, m)[0], styleSelection.Render("foo "), "고른 데까지만")
	})

	t.Run("줄 단위는 줄 끝 칸까지", func(t *testing.T) {
		m := send(newTestEditor("foo", 80, 20), "V")

		row := contentRowsOf(t, m)[0]
		assert.Contains(t, row, styleSelection.Render("foo"))
		assert.Contains(t, row, styleSelection.Render(" "), "개행 자리에 칸 하나")
	})

	t.Run("빈 줄도 칸 하나가 보인다", func(t *testing.T) {
		m := send(newTestEditor("\nfoo", 80, 20), "V")

		assert.Contains(t, contentRowsOf(t, m)[0], styleSelection.Render(" "))
	})

	t.Run("고르지 않은 줄은 칠하지 않는다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar", 80, 20), "V")

		assert.NotContains(t, contentRowsOf(t, m)[1], "48;5;238")
	})

	t.Run("나가면 사라진다", func(t *testing.T) {
		m := send(newTestEditor("foo bar", 80, 20), "v", "l", "esc")

		assert.NotContains(t, contentRowsOf(t, m)[0], "48;5;238")
	})
}

// visual 에서도 `"` 로 담을 자리를 고른다. 범위를 눈으로 고른 뒤에 오는 손이다(ADR-0058).
func TestVisualRegisterName(t *testing.T) {
	t.Run("이름을 대고 담는다", func(t *testing.T) {
		m := send(newTestEditor("foo bar\nbaz", 80, 20), "V", `"`, "a", "y")

		require.IsType(t, viewEditorNormal{}, m, "담고 나면 normal 로 돌아온다")
		assert.Equal(t, "foo bar⏎", previewOf(m.(viewEditorNormal).registers.byName("a")))
	})

	t.Run("숫자 이름에는 담지 못한다", func(t *testing.T) {
		m := send(newTestEditor("foo bar\nbaz", 80, 20), "V", `"`, "1", "y")

		assert.IsType(t, viewEditorVisual{}, m, "아무 일도 없이 visual 에 머문다")
		assert.False(t, m.(viewEditorVisual).registers.byName("1").filled())
	})

	t.Run("이름을 실은 이동은 없다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar\nbaz", 80, 20), "V", `"`, "a", "j", "y")

		// `j` 가 버려지므로 고른 것은 첫 줄뿐이고, 이름도 같이 버려져서 무명에만 담긴다.
		require.IsType(t, viewEditorNormal{}, m)
		assert.Equal(t, "foo⏎", previewOf(m.(viewEditorNormal).registers.unnamed))
		assert.False(t, m.(viewEditorNormal).registers.byName("a").filled())
	})

	t.Run("글자 하나가 아닌 키는 이름을 무른다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar", 80, 20), "v", `"`, "esc")

		assert.IsType(t, viewEditorVisual{}, m, "`\"` 를 무르는 것이라 visual 에 머문다")
	})

	t.Run("showcmd 에 보인다", func(t *testing.T) {
		m := send(newTestEditor("foo\nbar", 80, 20), "v", `"`)
		assert.Contains(t, barOf(t, m)[1], `"`)

		m = send(m, "a")
		assert.Contains(t, barOf(t, m)[1], `"a`)
	})

	t.Run("한글로 온 이름도 되돌린다", func(t *testing.T) {
		// `ㅁ` 이 `a` 다. 이름은 파일에 들어갈 글자가 아니라 키다(ADR-0008).
		m := send(newTestEditor("foo bar\nbaz", 80, 20), "V", `"`, "ㅁ", "y")

		require.IsType(t, viewEditorNormal{}, m)
		assert.Equal(t, "foo bar⏎", previewOf(m.(viewEditorNormal).registers.byName("a")))
	})
}
