package core

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEditor 의 height 는 편집 내용을 그릴 높이다. tabline 과 statusBar 는 별도로 얹힌다.
func newTestEditor(data string, width, height int) viewEditorNormal {
	return viewEditorNormal{
		editor: editor{
			buffers: []Buffer{newBuffer("test.txt", []byte(data))},
			width:   width,
			height:  height + tablineHeight + statusBarHeight,
		},
	}
}

// textOf 는 tabline·statusBar 와 sidebar·줄번호 칸을 뺀 파일 내용만 돌려준다.
// 색은 그대로 둔다 — 본문에 색이 없다는 것도 봐야 하기 때문이다. 줄번호는 gutterOf 로 따로 본다.
func textOf(t *testing.T, m tea.Model) string {
	t.Helper()

	rows := contentRowsOf(t, m)
	for i, row := range rows {
		rows[i] = leadingStyle.ReplaceAllString(ansi.TruncateLeft(row, gutterWidthOf(m), ""), "")
	}

	return strings.Join(rows, "\n")
}

// leadingStyle 는 행 맨 앞의 색 지정이다.
//
// TruncateLeft 는 자른 자리에 그때까지 켜져 있던 색을 다시 켜주므로, 줄번호 색이 본문 앞에
// 빈 껍데기로 남는다. 그것만 걷어낸다 — 본문 안에 색이 들어오면 그대로 보인다.
var leadingStyle = regexp.MustCompile(`^(\x1b\[[0-9;]*m)+`)

// gutterOf 는 편집 영역 각 행의 sidebar·줄번호 칸이다. 색은 뺀다.
func gutterOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	rows := contentRowsOf(t, m)
	for i, row := range rows {
		rows[i] = ansi.Strip(ansi.Truncate(row, gutterWidthOf(m), ""))
	}

	return rows
}

// contentRowsOf 는 편집 영역 행들이다. tabline 과 statusBar 는 뺀다.
func contentRowsOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	rows := strings.Split(m.View().Content, "\n")
	require.GreaterOrEqual(t, len(rows), tablineHeight+statusBarHeight)

	return rows[tablineHeight : len(rows)-statusBarHeight]
}

// gutterWidthOf 는 본문 앞에 붙는 칸의 폭이다. sidebar 와 줄번호 칸을 합친 것이다.
func gutterWidthOf(m tea.Model) int {
	width := 0
	if e, ok := m.(interface{ sidebarLeft() int }); ok {
		width += e.sidebarLeft()
	}
	if e, ok := m.(interface{ lineNumberWidth() int }); ok {
		width += e.lineNumberWidth()
	}

	return width
}

// tablineOf 는 화면 맨 위 tabline 줄에서 tab 들만 돌려준다.
// 색과 줄 끝을 채우는 빈 칸은 뺀다. 그 둘은 splitByReverse 로 따로 본다.
func tablineOf(t *testing.T, view tea.View) string {
	t.Helper()

	return strings.TrimRight(ansi.Strip(rawTablineOf(t, view)), " ")
}

// rawTablineOf 는 tabline 줄을 색이 붙은 그대로 돌려준다.
func rawTablineOf(t *testing.T, view tea.View) string {
	t.Helper()

	rows := strings.Split(view.Content, "\n")
	require.NotEmpty(t, rows)

	return rows[0]
}

func TestViewEditorRendersVisibleLines(t *testing.T) {
	m := newTestEditor("a\nb\nc\nd\ne\n", 80, 3)

	view := m.View()

	assert.Equal(t, "a\nb\nc", textOf(t, m), "화면 높이만큼만 그린다")
	assert.True(t, view.AltScreen)
	assert.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
}

// 화면보다 긴 줄은 잘리지 않고 다음 행으로 넘어간다.
func TestViewEditorWrapsLongLines(t *testing.T) {
	m := newTestEditor("0123456789\nshort\n", 4, 4)

	assert.Equal(t, "0123\n4567\n89\nshor", textOf(t, m))
}

// 두 칸 글자가 경계에 걸치면 그 글자는 다음 행으로 넘어가야 한다.
// 반 칸만 그리면 그 아래가 전부 밀린다.
func TestViewEditorWrapsWideCharAtBoundary(t *testing.T) {
	m := newTestEditor("한글\n", 3, 2)

	assert.Equal(t, "한\n글", textOf(t, m))
}

func TestViewEditorCursorPosition(t *testing.T) {
	m := newTestEditor("한글abc\nsecond\n", 80, 5)
	buf := &m.buffers[0]

	// X 는 줄번호 칸만큼, Y 는 tabline 한 줄만큼 밀린다. 첫 줄이 화면 1 행이다.
	left := m.lineNumberWidth()
	require.Positive(t, left, "이 너비에서는 줄번호가 그려진다")

	require.NotNil(t, m.View().Cursor)
	assert.Equal(t, tea.Position{X: left, Y: 1}, m.View().Cursor.Position)

	// 한글 한 글자 = 두 칸
	buf.moveRight(1, m.width)
	assert.Equal(t, tea.Position{X: left + 2, Y: 1}, m.View().Cursor.Position)

	buf.moveDown(1, m.width)
	assert.Equal(t, tea.Position{X: left + 2, Y: 2}, m.View().Cursor.Position)
}

// 스크롤된 뒤에도 커서는 화면 기준으로 그려져야 한다.
func TestViewEditorCursorAfterScroll(t *testing.T) {
	m := newTestEditor(strings.Repeat("line\n", 100), 80, 10)
	buf := &m.buffers[0]

	buf.moveDown(20, m.width)
	buf.scrollTo(m.width, m.textHeight())

	require.Equal(t, 11, buf.top)
	assert.Equal(t, 10, m.View().Cursor.Position.Y, "커서는 편집 영역 맨 아래 줄")
}

// wrap 된 줄 안에서 아래로 내려가면 커서가 다음 화면 행에 있어야 한다.
func TestViewEditorCursorInWrappedLine(t *testing.T) {
	m := newTestEditor("0123456789\n", 4, 4)
	buf := &m.buffers[0]

	buf.moveRight(1, m.width)
	assert.Equal(t, tea.Position{X: 1, Y: 1}, m.View().Cursor.Position)

	buf.moveDown(1, m.width)
	assert.Equal(t, tea.Position{X: 1, Y: 2}, m.View().Cursor.Position, "같은 줄의 두 번째 행")
}

func TestViewEditorHandlesWindowSize(t *testing.T) {
	m := newTestEditor(strings.Repeat("line\n", 100), 0, 0)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 5})

	resized, ok := updated.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 40, resized.width)
	assert.Equal(t, 5, resized.height)
	assert.Equal(t, 5, strings.Count(resized.View().Content, "\n")+1, "tabline + 편집 내용 + statusBar 가 화면을 채운다")
}

// 화면 크기를 받기 전에도 render 가 죽지 않아야 한다.
func TestViewEditorBeforeWindowSize(t *testing.T) {
	m := newTestEditor("a\nb\n", 0, 0)

	assert.NotPanics(t, func() { m.View() })
}

func TestViewEditorArrowKeysMoveCursor(t *testing.T) {
	m := newTestEditor("abc\ndef\n", 80, 5)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	moved := updated.(viewEditorNormal)
	assert.Equal(t, 1, moved.buffers[0].cursorCol)

	updated, _ = moved.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	moved = updated.(viewEditorNormal)
	assert.Equal(t, 1, moved.buffers[0].cursorLine)
}

// expandedRow 는 줄 하나를 화면 글자로 펼친 것이다. 색은 빼고 글자만 본다.
func expandedRow(row string) string {
	line := []byte(row)
	parts, _ := expandRow(line, 0, len(line), 0, markWhitespace(line))

	out := strings.Builder{}
	for _, part := range parts {
		out.WriteString(part.text)
	}

	return out.String()
}

// tab 을 그대로 넘기면 bubbletea 가 버려서 들여쓰기가 사라진다. 공백으로 펼쳐야 한다.
func TestExpandTabs(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want string
	}{
		{name: "tab 없으면 그대로", row: "abc", want: "abc"},
		{name: "줄 앞 tab", row: "\tabc", want: "»   abc"},
		{name: "글자 뒤 tab 은 남은 칸만", row: "ab\tc", want: "ab  c"},
		{name: "tab 두 개", row: "\t\ta", want: "»   »   a"},
		{name: "3 칸 뒤 tab 은 1 칸", row: "012\tx", want: "012 x"},
		{name: "한글 뒤 tab", row: "한글\tx", want: "한글    x"}, // 한글 4 칸 + 4 칸
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, expandedRow(test.row))
		})
	}
}

// tab 과 space 는 화면에서 같은 빈 칸이라 마커로 가른다.
// 줄 가운데 공백은 찍지 않는다 — 산문과 주석이 점으로 뒤덮인다.
func TestExpandWhitespaceMarkers(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want string
	}{
		{name: "space 들여쓰기", row: "  ab", want: "··ab"},
		{name: "tab 들여쓰기", row: "\tab", want: "»   ab"},
		{name: "섞인 들여쓰기", row: "\t  ab", want: "»   ··ab"},
		{name: "가운데 공백은 그대로", row: "a b c", want: "a b c"},
		{name: "가운데 tab 은 빈 칸으로만", row: "a\tb", want: "a   b"},
		{name: "줄 끝 공백", row: "ab  ", want: "ab··"},
		{name: "줄 끝 tab", row: "ab\t", want: "ab» "},
		{name: "공백뿐인 줄은 전부 마커", row: "  \t", want: "··» "},
		{name: "빈 줄", row: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, expandedRow(test.row))
		})
	}
}

// 마커와 본문은 색이 달라서 조각이 갈린다. 조각 경계가 어긋나면 색이 본문으로 번진다.
func TestExpandRowSplitsMarkerParts(t *testing.T) {
	line := []byte("\tab  ")
	parts, col := expandRow(line, 0, len(line), 0, markWhitespace(line))

	assert.Equal(t, []screenPart{
		{text: "»   ", marker: true},
		{text: "ab", marker: false},
		{text: "··", marker: true},
	}, parts)
	assert.Equal(t, 8, col, "tab 4 칸 + ab 2 칸 + 공백 2 칸")
}

func TestViewEditorRendersTabs(t *testing.T) {
	m := newTestEditor("func main() {\n\tprintln()\n}\n", 40, 3)

	assert.Equal(t, "func main() {\n»   println()\n}", ansi.Strip(textOf(t, m)))
}

// 편집 영역 왼쪽에 절대번호와 상대번호가 나란히 붙는다.
// 상대번호가 본문에 붙는 쪽이라 이동 거리를 셀 때 눈이 덜 움직인다.
func TestLineNumbers(t *testing.T) {
	var m tea.Model = newTestEditor("one\ntwo\nthree\nfour\nfive\n", 40, 5)

	assert.Equal(t, []string{
		"  1  0 ",
		"  2  1 ",
		"  3  2 ",
		"  4  3 ",
		"  5  4 ",
	}, gutterOf(t, m), "커서가 첫 줄이면 상대번호가 아래로 늘어난다")

	m = send(m, "2", "j")

	assert.Equal(t, []string{
		"  1  2 ",
		"  2  1 ",
		"  3  0 ",
		"  4  1 ",
		"  5  2 ",
	}, gutterOf(t, m), "커서 줄이 0 이고 위아래로 멀어진다")
}

// 상대번호만 흐린 색이다. 절대번호와 본문은 색이 없다.
func TestLineNumberRelativeIsDimmed(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 40, 2)

	row := contentRowsOf(t, m)[1]

	assert.Equal(t, "  2 \x1b[38;5;244m 1\x1b[m one"[:4], ansi.Strip(row)[:4], "절대번호는 맨 앞")
	assert.Contains(t, row, "\x1b[38;5;244m 1", "상대번호에만 색이 붙는다")
	assert.NotContains(t, textOf(t, m), "\x1b[", "본문에는 색이 가지 않는다")
}

// wrap 되어 이어지는 행은 번호 칸이 빈 칸이다. 번호가 있는 행이 곧 논리 줄의 시작이다.
func TestLineNumbersBlankOnWrappedRows(t *testing.T) {
	m := newTestEditor(strings.Repeat("a", 30)+"\nnext\n", 34, 3)
	require.Positive(t, m.lineNumberWidth())

	assert.Equal(t, []string{"  1  0 ", "       ", "  2  1 "}, gutterOf(t, m))
}

// 자릿수는 줄 수와 화면 높이를 따라간다. 짧은 파일에서도 최소 폭은 지킨다.
func TestLineNumberWidthFollowsFileSize(t *testing.T) {
	short := newTestEditor("a\n", 80, 5)
	assert.Equal(t, minAbsoluteDigits+1+minRelativeDigits+1, short.lineNumberWidth())

	long := newTestEditor(strings.Repeat("a\n", 1200), 80, 5)
	assert.Equal(t, 4+1+minRelativeDigits+1, long.lineNumberWidth(), "1200 줄이면 절대번호가 네 자리")

	tall := newTestEditor("a\n", 80, 120)
	assert.Equal(t, minAbsoluteDigits+1+3+1, tall.lineNumberWidth(), "화면이 높으면 상대번호가 세 자리")
}

// 번호 칸을 떼고 나면 본문이 남지 않는 좁은 화면에서는 그리지 않는다.
func TestLineNumbersHiddenOnNarrowScreen(t *testing.T) {
	m := newTestEditor("abc\n", 20, 3)

	assert.Zero(t, m.lineNumberWidth())
	assert.Equal(t, m.textWidth(), m.contentWidth(), "본문이 편집 영역을 다 쓴다")
	assert.Equal(t, "abc\n\n", textOf(t, m), "파일보다 화면이 길면 남는 행은 빈 줄이다")
}

// 줄번호 칸만큼 본문이 좁아지므로 줄바꿈도 그 너비로 일어나야 한다.
func TestLineNumbersNarrowContentWrapsEarlier(t *testing.T) {
	m := newTestEditor(strings.Repeat("a", 30)+"\n", 34, 3)

	assert.Equal(t, 34-m.lineNumberWidth(), m.contentWidth())
	assert.Equal(t, strings.Repeat("a", 27)+"\naaa\n", textOf(t, m), "본문 너비에서 접힌다")
}
