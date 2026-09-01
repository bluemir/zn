package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cursorOf 는 활성 buffer 의 커서 자리다. 줄과 byte offset 이다.
func cursorOf(t *testing.T, m tea.Model) (int, int) {
	t.Helper()

	buf := bufferOf(t, m)

	return buf.cursorLine, buf.cursorCol
}

// typeInto 는 글자를 한 자씩 키로 넣는다. 검색은 치는 도중의 화면도 봐야 해서 통째로 넣지 않는다.
func typeInto(m tea.Model, text string) tea.Model {
	return send(m, strings.Split(text, "")...)
}

func TestSplitSearchFlags(t *testing.T) {
	cases := []struct {
		input   string
		pattern string
		flags   string
	}{
		{"abcd/i", "abcd", "i"},
		{"http://", "http://", ""},  // 뒤가 비어서 flag 가 아니다
		{"a/b", "a", "b"},           // 모르는 flag 라 parseSearchPattern 이 오류를 낸다
		{`a\/b`, `a\/b`, ""},        // `\` 로 막은 것은 구분자가 아니다
		{"x/", "x/", ""},            // 뒤가 비어서 flag 가 아니다
		{"a/b c", "a/b c", ""},      // 영문자가 아닌 것이 섞이면 패턴의 일부다
		{`a\\/i`, `a\\/i`[:3], "i"}, // `\\` 뒤의 `/` 는 막히지 않았다
		{"abcd", "abcd", ""},        // 구분자가 없다
		{"/i", "", "i"},             // 패턴이 비어도 flag 는 뗀다
	}

	for _, c := range cases {
		pattern, flags := splitSearchFlags(c.input)

		assert.Equal(t, c.pattern, pattern, c.input)
		assert.Equal(t, c.flags, flags, c.input)
	}
}

func TestParseSearchPatternIgnoreCaseFlag(t *testing.T) {
	sensitive, err := parseSearchPattern("abcd")
	require.NoError(t, err)
	assert.False(t, sensitive.MatchString("ABCD"), "flag 가 없으면 대소문자를 구분한다")

	ignore, err := parseSearchPattern("abcd/i")
	require.NoError(t, err)
	assert.True(t, ignore.MatchString("ABCD"))
}

func TestParseSearchPatternKeepsEscapedSlash(t *testing.T) {
	pattern, err := parseSearchPattern(`a\/b`)
	require.NoError(t, err)

	assert.True(t, pattern.MatchString("a/b"))
}

func TestParseSearchPatternRejectsUnknownFlag(t *testing.T) {
	_, err := parseSearchPattern("a/b")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "모르는 flag")
}

func TestParseSearchPatternRejectsBrokenPattern(t *testing.T) {
	_, err := parseSearchPattern("(")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "잘못된 패턴")
}

// `/` 로 친 패턴은 정규식이다.
func TestSearchMovesToMatch(t *testing.T) {
	var m tea.Model = newTestEditor("aaa\nbbb\nfunc name\n", 40, 5)

	m = typeInto(m, "/func.*e")
	m = send(m, "enter")

	line, col := cursorOf(t, m)
	assert.Equal(t, 2, line)
	assert.Equal(t, 0, col)
	assert.IsType(t, viewEditorNormal{}, m)
}

// 커서 자리의 매칭에는 다시 걸리지 않는다. 걸리면 `n` 이 제자리걸음을 한다.
func TestSearchSkipsMatchUnderCursor(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nfoo\n", 40, 5)

	m = typeInto(m, "/foo")
	m = send(m, "enter")

	line, _ := cursorOf(t, m)
	assert.Equal(t, 1, line)
}

// 파일 끝에 닿으면 처음으로 돌아가고 그 사실을 아래 줄에 알린다.
func TestSearchWrapsAroundAndTells(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nbar\n", 40, 5)

	m = typeInto(m, "/foo")
	m = send(m, "enter")

	line, _ := cursorOf(t, m)
	assert.Equal(t, 0, line)
	assert.Contains(t, barOf(t, m)[1], "아래에서 처음으로 돌아옴")
}

func TestSearchBackwardWrapsToEnd(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nbar\nfoo\n", 40, 5)

	m = typeInto(m, "?foo")
	m = send(m, "enter")

	line, _ := cursorOf(t, m)
	assert.Equal(t, 2, line, "위로 찾다가 파일 처음을 지나 끝에서 이어 본다")
	assert.Contains(t, barOf(t, m)[1], "위에서 끝으로 돌아옴")
}

func TestSearchNotFoundTells(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)

	m = typeInto(m, "/zzz")
	m = send(m, "enter")

	line, col := cursorOf(t, m)
	assert.Equal(t, 0, line, "못 찾으면 커서를 두고 알리기만 한다")
	assert.Equal(t, 0, col)
	assert.Contains(t, barOf(t, m)[1], "찾을 수 없음: zzz")
}

func TestSearchBrokenPatternTells(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = typeInto(m, "/(")
	m = send(m, "enter")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "잘못된 패턴")
}

// 치는 동안 첫 매칭으로 커서가 따라간다. vim 의 incsearch 다.
func TestSearchIncrementalPreview(t *testing.T) {
	var m tea.Model = newTestEditor("aaa\nbbb\nccc\n", 40, 5)

	m = typeInto(m, "/cc")

	line, _ := cursorOf(t, m)
	assert.Equal(t, 2, line)
	assert.IsType(t, viewEditorSearch{}, m)
}

// 글자를 지우면 시작 자리에서 다시 찾는다. 옮긴 자리에서 이어 찾으면 앞으로 돌아오지 못한다.
func TestSearchPreviewSearchesFromOrigin(t *testing.T) {
	var m tea.Model = newTestEditor("ab\nb\n", 40, 5)

	m = typeInto(m, "/b")
	line, col := cursorOf(t, m)
	require.Equal(t, 0, line)
	require.Equal(t, 1, col, "커서 다음 자리의 첫 매칭이다")

	m = typeInto(m, "b") // `/bb` 라 매칭이 없다
	line, col = cursorOf(t, m)
	assert.Equal(t, 0, line, "못 찾으면 시작 자리에 머문다")
	assert.Equal(t, 0, col)
}

func TestSearchEscRestoresCursor(t *testing.T) {
	var m tea.Model = newTestEditor("aaa\nbbb\nccc\n", 40, 5)
	m = send(m, "j") // 둘째 줄에서 시작한다

	m = typeInto(m, "/ccc")
	require.Equal(t, 2, bufferOf(t, m).cursorLine)

	m = send(m, "esc")

	line, _ := cursorOf(t, m)
	assert.Equal(t, 1, line)
	assert.IsType(t, viewEditorNormal{}, m)
}

// `/` 만 치고 지우면 검색에서 나간다. command mode 와 같다.
func TestSearchBackspaceOnEmptyLeaves(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "/", "backspace")

	assert.IsType(t, viewEditorNormal{}, m)
}

func TestSearchNextAndPrevious(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nbar\nfoo\nbaz\nfoo\n", 40, 8)

	m = typeInto(m, "/foo")
	m = send(m, "enter")
	require.Equal(t, 2, bufferOf(t, m).cursorLine)

	m = send(m, "n")
	assert.Equal(t, 4, bufferOf(t, m).cursorLine)

	m = send(m, "N")
	assert.Equal(t, 2, bufferOf(t, m).cursorLine, "N 은 반대 방향이다")
}

// `?` 로 찾았으면 `n` 도 위로 간다.
func TestSearchNextFollowsDirection(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nbar\nfoo\nbaz\nfoo\n", 40, 8)
	m = send(m, "G") // 마지막 줄

	m = typeInto(m, "?foo")
	m = send(m, "enter")
	require.Equal(t, 2, bufferOf(t, m).cursorLine)

	m = send(m, "n")
	assert.Equal(t, 0, bufferOf(t, m).cursorLine)
}

func TestSearchNextWithCount(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nfoo\nfoo\nfoo\n", 40, 8)

	m = typeInto(m, "/foo")
	m = send(m, "enter")
	require.Equal(t, 1, bufferOf(t, m).cursorLine)

	m = send(m, "2", "n")
	assert.Equal(t, 3, bufferOf(t, m).cursorLine)
}

func TestSearchNextWithoutSearchTells(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "n")

	assert.Contains(t, barOf(t, m)[1], "이전 검색이 없습니다")
}

// `n` 은 mode 를 바꾸지 않는다. 그래서 겹모음 `ㅝ`(`n` `j`) 의 뒤 동작이 이어서 실행된다.
//
// 검색이 없어서 알리기만 할 때도 같다 — 알림이 뒤 동작을 삼키면 `ㅝ` 가 아래로 못 간다.
// `jumpToMatch` 가 model 을 돌려주던 동안에는 그 구별이 `press` 의 type 확인에 달려 있었다
// (ADR-0008, ADR-0034).
func TestHangulSearchThenMoveRunsBoth(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)

	m = send(m, "ㅝ")

	require.IsType(t, viewEditorNormal{}, m, "mode 는 그대로다")
	assert.Contains(t, barOf(t, m)[1], "이전 검색이 없습니다")
	assert.Equal(t, 1, bufferOf(t, m).cursorLine, "뒤의 `j` 도 실행된다")
}

// 빈 채로 Enter 는 마지막 검색을 그 방향으로 되풀이한다.
func TestSearchEmptyRepeatsLastPattern(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nbar\nfoo\n", 40, 5)

	m = typeInto(m, "/foo")
	m = send(m, "enter")
	require.Equal(t, 2, bufferOf(t, m).cursorLine)

	m = send(m, "?", "enter")
	assert.Equal(t, 0, bufferOf(t, m).cursorLine, "`?` 로 들어갔으니 위로 찾는다")
}

// `*` 는 커서 아래 단어를 그대로 찾는다. 단어 전체가 맞아야 한다.
func TestSearchWordUnderCursor(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nfoobar\nfoo\n", 40, 5)

	m = send(m, "*")

	assert.Equal(t, 2, bufferOf(t, m).cursorLine, "foobar 는 건너뛴다")
}

func TestSearchWordBackward(t *testing.T) {
	var m tea.Model = newTestEditor("foo\nbar\nfoo\n", 40, 5)
	m = send(m, "G")

	m = send(m, "#")

	assert.Equal(t, 0, bufferOf(t, m).cursorLine)
}

// 한글 단어는 `\b` 를 붙이지 않는다. 붙이면 아무것도 찾지 못한다.
func TestSearchWordHangul(t *testing.T) {
	var m tea.Model = newTestEditor("한글\nabc\n한글\n", 40, 5)

	m = send(m, "*")

	assert.Equal(t, 2, bufferOf(t, m).cursorLine)
}

// 커서가 단어 위가 아니면 그 줄에서 오른쪽으로 첫 단어를 찾는다.
func TestSearchWordSkipsBlank(t *testing.T) {
	var m tea.Model = newTestEditor("  foo\nfoo\n", 40, 5)

	m = send(m, "*")

	assert.Equal(t, 1, bufferOf(t, m).cursorLine)
}

func TestSearchWordOnEmptyLineTells(t *testing.T) {
	var m tea.Model = newTestEditor("\nfoo\n", 40, 5)

	m = send(m, "*")

	assert.Contains(t, barOf(t, m)[1], "커서 아래에 단어가 없습니다")
}

func TestWordUnderCursorSplitsByClass(t *testing.T) {
	buf := newBuffer("test.txt", []byte("한글abc\n"))

	word, _, ok := buf.wordUnderCursor()

	require.True(t, ok)
	assert.Equal(t, "한글", word, "`한글abc` 는 두 단어다")
}

// 찾은 자리는 모두 칠하고, 커서가 선 것만 색이 다르다.
func TestSearchHighlightsMatches(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar foo\n", 40, 3)

	m = typeInto(m, "/foo")
	m = send(m, "enter")

	rows := contentRowsOf(t, m)
	require.NotEmpty(t, rows)

	assert.Equal(t, "foo bar foo", ansi.Strip(rows[0])[gutterWidthOf(m):], "글자는 그대로다")
	assert.Contains(t, rows[0], styleSearchCurrent.Render("foo"), "커서가 선 매칭")
	assert.Contains(t, rows[0], styleSearchMatch.Render("foo"), "나머지 매칭")
}

// 강조는 커서를 옮겨도 남는다. `:noh` 로만 꺼진다.
func TestSearchHighlightClearedByNoh(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar\n", 40, 3)

	m = typeInto(m, "/foo")
	m = send(m, "enter")
	require.Contains(t, contentRowsOf(t, m)[0], styleSearchCurrent.Render("foo"))

	m = typeInto(m, ":noh")
	m = send(m, "enter")

	assert.NotContains(t, contentRowsOf(t, m)[0], styleSearchMatch.Render("foo"))
	assert.NotContains(t, contentRowsOf(t, m)[0], styleSearchCurrent.Render("foo"))

	m = send(m, "n")
	assert.Contains(t, barOf(t, m)[1], "아래에서 처음으로", "마지막 검색은 남아 있다")
}

// wrap 된 줄에서 행 경계에 걸친 매칭은 양쪽 행에 나뉘어 칠해진다.
func TestSearchHighlightAcrossWrap(t *testing.T) {
	var m tea.Model = newTestEditor("abcdef\n", 4, 3)

	m = typeInto(m, "/cdef")
	m = send(m, "enter")

	rows := contentRowsOf(t, m)
	require.GreaterOrEqual(t, len(rows), 2)

	assert.Equal(t, "abcd", ansi.Strip(rows[0])[gutterWidthOf(m):])
	assert.Equal(t, "ef", ansi.Strip(rows[1])[gutterWidthOf(m):])
	assert.Contains(t, rows[0], styleSearchCurrent.Render("cd"))
	assert.Contains(t, rows[1], styleSearchCurrent.Render("ef"))
}

// tab 이 든 줄은 강조 조각을 이어 그려도 칸이 어긋나지 않아야 한다.
func TestSearchHighlightKeepsTabWidth(t *testing.T) {
	var m tea.Model = newTestEditor("\tfoo\n", 40, 3)

	m = typeInto(m, "/foo")
	m = send(m, "enter")

	row := ansi.Strip(contentRowsOf(t, m)[0])[gutterWidthOf(m):]

	assert.Equal(t, markerTab+strings.Repeat(" ", defaultTabWidth-1)+"foo", row)
}

// 강조 구간에 공백 마커가 끼어도 그 뒤 글자가 강조를 잃지 않는다.
//
// 마커에만 색을 얹으면 그 색을 끝내는 리셋이 강조까지 함께 꺼버린다. 조각마다 style 을
// 한 번씩만 입혀야 마커 뒤에서 강조가 이어진다.
func TestRenderPartsKeepsStyleAcrossMarker(t *testing.T) {
	parts := []screenPart{
		{text: "⋅⋅", kind: partMarker},
		{text: "ab"},
	}

	got := renderParts(parts, styleSearchMatch)

	assert.Contains(t, got, styleSearchMatch.Render("ab"), "마커 뒤 글자가 강조를 그대로 쓴다")
	assert.Equal(t, "⋅⋅ab", ansi.Strip(got))
}

// 강조 밖의 마커는 흐린 색이다.
func TestRenderPartsDimsMarker(t *testing.T) {
	var plain lipgloss.Style

	got := renderParts([]screenPart{{text: "⋅⋅", kind: partMarker}, {text: "ab"}}, plain)

	assert.Contains(t, got, plain.Foreground(colorWhitespace).Render("⋅⋅"))
	assert.Equal(t, "⋅⋅ab", ansi.Strip(got))
}

// 제어문자는 마커와 반대로 눈에 띄는 색이다. 흐리게 두면 진짜 `^[` 두 글자와 갈리지 않는다
// (ADR-0118).
func TestRenderPartsColorsControl(t *testing.T) {
	var plain lipgloss.Style

	got := renderParts([]screenPart{{text: "^[", kind: partControl}, {text: "ab"}}, plain)

	assert.Contains(t, got, plain.Foreground(colorControl).Render("^["))
	assert.NotContains(t, got, plain.Foreground(colorWhitespace).Render("^["), "마커 색이 아니다")
	assert.Equal(t, "^[ab", ansi.Strip(got))
}

// 검색은 tab 을 넘어 남는다. `n` 이 다른 tab 에서도 같은 것을 찾는다.
func TestSearchSurvivesTabSwitch(t *testing.T) {
	m := viewEditorNormal{
		editor: &editor{
			buffers: []viewport{
				newBuffer("a.txt", []byte("foo\nbar\n")),
				newBuffer("b.txt", []byte("baz\nfoo\n")),
			},
			width:  40,
			height: 5 + tablineHeight + statusBarHeight,
		},
	}

	var model tea.Model = m
	model = typeInto(model, "/foo")
	model = send(model, "enter")

	model = send(model, "g", "t", "n")

	assert.Equal(t, 1, bufferOf(t, model).cursorLine)
}
