package core

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catOf 는 범위를 내보내고 터미널에 찍힌 것을 준다. shellRun 을 보는 손과 같다.
func catOf(t *testing.T, buf viewport, area scheme.MotionRange) string {
	t.Helper()

	out := &bytes.Buffer{}
	run := &catRun{header: catHeader(buf, area), lines: buf.catLines(area)}
	run.SetStdin(strings.NewReader("\n"))
	run.SetStdout(out)
	run.SetStderr(out)

	require.NoError(t, run.Run())

	return out.String()
}

// 내는 것은 화면이 아니라 파일의 byte 그대로다 — 줄번호도 공백 마커도 색도 없고 tab 은 tab 이다.
func TestCatRunPrintsFileBytes(t *testing.T) {
	buf := newBuffer("a.go", []byte("package main\n\tif x {\n"))

	text := catOf(t, buf, scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 1}, Linewise: true})

	assert.Contains(t, text, "package main\n")
	assert.Contains(t, text, "\tif x {\n", "tab 이 칸으로 펼쳐지지 않는다")
	assert.NotContains(t, text, markerTab)
	assert.NotContains(t, text, markerSpace)
	assert.NotContains(t, text, "\x1b[", "색이 섞이지 않는다")
}

// 본문 줄끝은 `\n` 이다. 두르는 두 줄만 `\r\n` 으로 shellRun 과 맞춘다 —
// Run 이 도는 자리는 이미 cooked 이라 `ONLCR` 이 `\n` 을 알아서 `\r\n` 으로 만든다.
func TestCatRunEndsBodyLinesWithLF(t *testing.T) {
	buf := newBuffer("a.go", []byte("aaa\nbbb\n"))

	text := catOf(t, buf, scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 1}, Linewise: true})

	assert.Contains(t, text, "aaa\nbbb\n", "본문 줄 사이에 `\\r` 이 없다")
	assert.True(t, strings.HasPrefix(text, "\r\n"), "머리말은 `\\r\\n` 으로 두른다")
}

// 머리말이 먼저고 묻는 것이 맨 나중이다. `:!` 와 같은 차례다 —
// 멈춰 서지 않으면 돌아가는 순간 대체 화면이 덮는다.
func TestCatRunEchoesHeaderAndWaitsForEnter(t *testing.T) {
	buf := newBuffer("a.go", []byte("aaa\nbbb\n"))

	text := catOf(t, buf, scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 1}, Linewise: true})

	assert.Less(t, strings.Index(text, ":cat"), strings.Index(text, "aaa"), "머리말이 본문보다 먼저다")
	assert.Less(t, strings.Index(text, "bbb"), strings.Index(text, "계속하려면 Enter"), "묻는 것은 맨 나중이다")
}

// 머리말은 무엇을 낸 것인지다. 주 화면에는 지난 출력이 남아 있어서 이것이 없으면 갈리지 않는다.
func TestCatHeader(t *testing.T) {
	tests := []struct {
		name string
		path string
		area scheme.MotionRange
		want string
	}{
		{name: "줄 범위는 1 부터다", path: "a.go",
			area: scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 6}, Linewise: true}, want: ":cat a.go 1-7"},
		{name: "한 줄", path: "a.go",
			area: scheme.MotionRange{Start: scheme.Cursor{Line: 2}, End: scheme.Cursor{Line: 2}, Linewise: true}, want: ":cat a.go 3-3"},
		{name: "이름 없는 buffer", path: "",
			area: scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 0}, Linewise: true}, want: ":cat [No Name] 1-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer(test.path, []byte("a\nb\nc\nd\ne\nf\ng\n"))

			assert.Equal(t, test.want, catHeader(buf, test.area))
		})
	}
}

// 줄 단위면 그 줄들 전부, 글자 단위면 고른 조각이다.
func TestCatLines(t *testing.T) {
	buf := newBuffer("a.go", []byte("abcdef\nghijkl\nmnopqr\n"))

	tests := []struct {
		name string
		area scheme.MotionRange
		want []string
	}{
		{name: "줄 단위", area: scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 1}, Linewise: true},
			want: []string{"abcdef", "ghijkl"}},
		{name: "줄 하나", area: scheme.MotionRange{Start: scheme.Cursor{Line: 2}, End: scheme.Cursor{Line: 2}, Linewise: true},
			want: []string{"mnopqr"}},
		{name: "한 줄 안의 글자", area: scheme.MotionRange{Start: scheme.Cursor{Line: 0, Col: 1}, End: scheme.Cursor{Line: 0, Col: 4}},
			want: []string{"bcd"}},
		{name: "여러 줄에 걸친 글자", area: scheme.MotionRange{Start: scheme.Cursor{Line: 0, Col: 4}, End: scheme.Cursor{Line: 1, Col: 2}},
			want: []string{"ef", "gh"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := make([]string, 0, len(test.want))
			for _, line := range buf.catLines(test.area) {
				got = append(got, string(line))
			}

			assert.Equal(t, test.want, got)
		})
	}
}

// 파일도 커서도 건드리지 않는다.
func TestCatLinesTouchesNothing(t *testing.T) {
	buf := newBuffer("a.go", []byte("abc\ndef\n"))
	buf.cursor.Line, buf.cursor.Col = 1, 2

	buf.catLines(scheme.MotionRange{Start: scheme.Cursor{Line: 0}, End: scheme.Cursor{Line: 1}, Linewise: true})

	assert.Equal(t, []string{"abc", "def"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursor.Line)
	assert.Equal(t, 2, buf.cursor.Col)
	assert.False(t, buf.dirty)
}

// 보이는 줄들이다. wrap 된 줄은 한 조각만 보여도 그 줄 전체가 든다 —
// 내는 것이 화면 행이 아니라 파일의 줄이라서다.
func TestVisibleRange(t *testing.T) {
	m := newTestEditor("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", 40, 4)

	area := m.visibleRange()

	assert.True(t, area.Linewise)
	assert.Equal(t, 0, area.Start.Line)
	assert.Equal(t, 3, area.End.Line, "화면 높이만큼이다")
}

// 굴려 놓은 자리를 따라간다. 화면 첫 줄이 파일 첫 줄이 아니어도 된다.
func TestVisibleRangeFollowsScroll(t *testing.T) {
	m := newTestEditor("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", 40, 4)
	m.buffers[0].top.line = 5

	area := m.visibleRange()

	assert.Equal(t, 5, area.Start.Line)
	assert.Equal(t, 8, area.End.Line)
}

// 긴 줄은 화면 행 여럿을 먹는다. 세는 것은 줄이라 든 줄 수가 화면 높이보다 적어진다.
func TestVisibleRangeCountsLinesNotRows(t *testing.T) {
	m := newTestEditor(strings.Repeat("x", 200)+"\nshort\nalso\n", 40, 8)

	area := m.visibleRange()

	assert.Equal(t, 0, area.Start.Line)
	assert.Equal(t, 1, area.End.Line, "긴 줄이 행 일곱을 먹어서 여덟 행에 줄 둘만 든다")
}

// 파일 끝에서는 있는 만큼이다.
func TestVisibleRangeStopsAtLastLine(t *testing.T) {
	m := newTestEditor("1\n2\n", 40, 8)

	area := m.visibleRange()

	assert.Equal(t, 0, area.Start.Line)
	assert.Equal(t, 1, area.End.Line)
}

// `\c` 는 두 키짜리 leader 조합이다. 나머지 셋(`\gd` `\gr` `\rn`) 은 세 키다.
func TestCatKeyInNormal(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: "`\\c`", keys: []string{"\\", "c"}, want: actionCat{}},
		{name: "한글 자판의 원화 기호", keys: []string{"₩", "ㅊ"}, want: actionCat{}},

		// 짝이 없는 조합은 아무 일도 하지 않는다. `\` 뒤의 한 글자에서 끝난다.
		{name: "짝 없는 leader 조합", keys: []string{"\\", "z"}, want: nil},

		// operator 뒤에는 올 수 없다. motion 이 아니라 홀로 서는 동작이다.
		{name: "`d\\c` 는 아무것도 아니다", keys: []string{"d", "\\", "c"}, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			built, state := pressAll(test.keys...)

			assert.Equal(t, test.want, built)
			assert.IsType(t, normalStart{}, state, "조합이 그 자리에서 끝난다")
		})
	}
}

// visual 도 같은 손으로 부른다. **leader 를 받는 첫 자리다**(ADR-0085).
func TestCatKeyInVisual(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: "`\\c`", keys: []string{"\\", "c"}, want: actionVisualCat{}},
		{name: "한글 자판의 원화 기호", keys: []string{"₩", "ㅊ"}, want: actionVisualCat{}},

		// 숫자를 받지 않는다. 고른 범위가 이미 정해져 있어서 되풀이할 것이 없다.
		{name: "숫자는 버린다", keys: []string{"2", "\\", "c"}, want: actionVisualCat{}},

		// 이름을 실을 수 없는 동작이다. 담는 일이 아니라 내는 일이다.
		{name: "register 이름을 대면 아무 일도 없다", keys: []string{`"`, "a", "\\", "c"}, want: nil},

		{name: "짝 없는 leader 조합", keys: []string{"\\", "z"}, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var state visualState = visualStart{}

			var built action
			for _, k := range test.keys {
				var actions []action
				actions, state = state.press(k)

				built = nil
				if len(actions) > 0 {
					built = actions[len(actions)-1]
				}
			}

			assert.Equal(t, test.want, built)
			assert.IsType(t, visualStart{}, state)
		})
	}
}

// **`2\c` 의 `\` 를 숫자 상태가 안 받으면 뒤의 `c` 가 고른 것을 지운다.** 두 상태가 같이 안다.
func TestCatKeyAfterCountDoesNotChange(t *testing.T) {
	m := send(newTestEditor("aaa\nbbb\nccc\n", 60, 8), "V", "2", "\\", "c")

	assert.IsType(t, viewEditorNormal{}, m, "cat 은 normal 로 나온다")
	assert.Equal(t, []string{"aaa", "bbb", "ccc"}, linesOf(bufferOf(t, m)), "파일을 건드리지 않는다")
	assert.False(t, bufferOf(t, m).dirty)
}

// visual 에서 부르면 normal 로 나오고 커서는 고른 범위의 시작이다. `y` 와 같은 자리다.
func TestCatFromVisualLeavesToNormalAtRangeStart(t *testing.T) {
	m := send(newTestEditor("aaa\nbbb\nccc\nddd\n", 60, 8), "j", "V", "j", "\\", "c")

	require.IsType(t, viewEditorNormal{}, m)

	buf := bufferOf(t, m)
	assert.Equal(t, 1, buf.cursor.Line)
	assert.False(t, buf.selection.active, "고른 것은 놓는다")
	assert.Equal(t, []string{"aaa", "bbb", "ccc", "ddd"}, linesOf(buf))
}

// normal 에서 부르면 화면도 커서도 그대로다. 보여주기만 하는 일이다.
func TestCatFromNormalTouchesNothing(t *testing.T) {
	m := send(newTestEditor("aaa\nbbb\nccc\n", 60, 8), "j", "\\", "c")

	require.IsType(t, viewEditorNormal{}, m)

	buf := bufferOf(t, m)
	assert.Equal(t, 1, buf.cursor.Line)
	assert.False(t, buf.dirty)
}

// `:cat` 은 줄 범위를 받는다. 범위를 치지 않으면 화면에 보이는 줄들이다.
func TestCatCommand(t *testing.T) {
	for _, input := range []string{"cat", "%cat", "1,2cat", "5cat", ".cat", "$cat"} {
		t.Run(input, func(t *testing.T) {
			m := runCommand(newTestEditor("a\nb\nc\nd\ne\n", 60, 8), input)

			require.IsType(t, viewEditorNormal{}, m, barOf(t, m)[1])
			assert.NotContains(t, barOf(t, m)[1], "알 수 없는 명령")
			assert.NotContains(t, barOf(t, m)[1], "줄 범위를 받지 않습니다")
			assert.Equal(t, []string{"a", "b", "c", "d", "e"}, linesOf(bufferOf(t, m)))
		})
	}
}

// 없는 줄은 다른 범위 명령과 같이 거절한다.
func TestCatCommandRejectsMissingLine(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "9cat")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "그런 줄이 없습니다: 9")
}

// 인자는 받지 않는다. 범위 말고 댈 것이 없다.
func TestCatCommandTakesNoArgument(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "cat foo")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "알 수 없는 명령")
}

// **감싸는 머리줄이 덮은 줄도 든다.** 눈에는 없지만 범위 안이다(ADR-0049, ADR-0085).
//
// 빼면 `\c` 가 맨 위 몇 줄을 조용히 떨어뜨려서 붙여넣은 코드에 구멍이 난다. 머리줄이 든
// 줄을 대신 넣는 것도 아니다 — 그것은 화면 밖 다른 자리라 이어지지 않은 두 덩이가 붙는다.
func TestVisibleRangeIncludesLinesUnderSticky(t *testing.T) {
	data := "func alpha() {\n" + // 0
		"\tif a {\n" + // 1
		"\t\tone()\n" + // 2
		"\t\ttwo()\n" + // 3
		"\t\tthree()\n" + // 4
		"\t\tfour()\n" + // 5
		"\t}\n" + // 6
		"}\n" // 7

	m := newTestEditorFile("a.go", data, 40, 4)
	m.activeBuffer().top.line = 3

	require.NotEmpty(t, m.activeBuffer().stickyAt(3, 4), "머리줄이 붙는 화면이어야 한다")

	area := m.visibleRange()

	assert.Equal(t, 3, area.Start.Line, "머리줄이 덮은 줄이 그대로 시작이다")
	assert.Equal(t, 6, area.End.Line)
}
