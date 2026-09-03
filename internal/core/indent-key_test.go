package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

func TestTabFillsToTheNextUnitBoundary(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		style string
		data  string
		keys  []string
		want  []string
	}{
		{name: "tab 단위면 tab 글자 하나", file: "a.go", style: "indent_style = tab",
			data: "\n", keys: []string{"i", "tab", "x"}, want: []string{"\tx"}},
		{name: "space 단위는 칸을 채운다", file: "a.js",
			style: "indent_style = space\nindent_size = 4",
			data:  "\n", keys: []string{"i", "tab", "x"}, want: []string{"    x"}},
		{name: "어긋난 자리에서는 경계까지만", file: "a.js",
			style: "indent_style = space\nindent_size = 4",
			data:  "abc\n", keys: []string{"$", "a", "tab", "x"}, want: []string{"abc x"}},
		{name: "경계에 있으면 한 단위", file: "a.js",
			style: "indent_style = space\nindent_size = 4",
			data:  "abcd\n", keys: []string{"$", "a", "tab", "x"}, want: []string{"abcd    x"}},
		{name: "한글은 두 칸으로 센다", file: "a.js",
			style: "indent_style = space\nindent_size = 4",
			data:  "한\n", keys: []string{"$", "a", "tab", "x"}, want: []string{"한  x"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newIndentEditor(t, test.file, test.style, test.data)

			m = send(m, test.keys...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

func TestShiftTabOutdentsTheLine(t *testing.T) {
	m := newIndentEditor(t, "a.js", "indent_style = space\nindent_size = 2", "    foo\n")

	m = send(m, "$", "a", "shift+tab")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"  foo"}, linesOf(buf))
	assert.Equal(t, 5, buf.Cursor.Col, "치던 자리가 두 칸 왼쪽으로 따라온다")
}

func TestShiftTabStopsAtTheLeftEdge(t *testing.T) {
	m := newIndentEditor(t, "a.js", "indent_style = space\nindent_size = 4", "  foo\n")

	m = send(m, "i", "shift+tab", "shift+tab")

	assert.Equal(t, []string{"foo"}, linesOf(bufferOf(t, m)))
}

func TestBackspaceEatsAWholeUnit(t *testing.T) {
	tests := []struct {
		name  string
		style string
		data  string
		keys  []string
		want  []string
	}{
		{name: "들여쓰기 안이면 한 단위", style: "indent_style = space\nindent_size = 4",
			data: "        foo\n", keys: []string{"^", "i", "backspace"},
			want: []string{"    foo"}},
		{name: "어긋나 있으면 앞 경계까지", style: "indent_style = space\nindent_size = 4",
			data: "      foo\n", keys: []string{"^", "i", "backspace"},
			want: []string{"    foo"}},
		{name: "글자 앞의 공백은 들여쓰기가 아니다", style: "indent_style = space\nindent_size = 4",
			data: "a    b\n", keys: []string{"$", "i", "backspace"},
			want: []string{"a   b"}},
		{name: "줄 앞이면 앞 줄과 합친다", style: "indent_style = space\nindent_size = 4",
			data: "a\nb\n", keys: []string{"j", "i", "backspace"}, want: []string{"ab"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newIndentEditor(t, "a.js", test.style, test.data)

			m = send(m, test.keys...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

func TestTabIndentsMarkdownListItem(t *testing.T) {
	tests := []struct {
		name string
		data string
		keys []string
		want []string
	}{
		{name: "항목 가운데에서 쳐도 줄이 들어간다", data: "- 첫째\n",
			keys: []string{"$", "i", "tab"}, want: []string{"  - 첫째"}},
		{name: "빈 항목에서도 된다", data: "- \n",
			keys: []string{"$", "a", "tab"}, want: []string{"  - "}},
		{name: "shift+tab 으로 되돌린다", data: "  - 첫째\n",
			keys: []string{"$", "i", "shift+tab"}, want: []string{"- 첫째"}},
		{name: "목록이 아닌 줄은 커서 자리에 넣는다. 7 칸이라 다음 경계는 한 칸 뒤다", data: "보통 글\n",
			keys: []string{"$", "a", "tab", "x"}, want: []string{"보통 글 x"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newIndentEditor(t, "a.md",
				"indent_style = space\nindent_size = 2", test.data)

			m = send(m, test.keys...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

// 코드펜스 안에서 `tab` 은 줄 전체가 아니라 커서 자리에 든다.
//
// 「목록 줄에서는 줄 전체」는 markdown 규칙인데(syntax.Indent 의 TabIndentsLine) 펜스 안은
// go 규칙이라 거짓이다. 규칙이 문맥을 따라오는지가 이 키에서도 갈린다(ADR-0102).
func TestTabInsideAFenceGoesAtTheCursor(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"```go\n- 목록처럼 보이는 줄\n```\n")

	m = send(m, "j", "$", "a", "tab", "x")

	assert.Equal(t, []string{"```go", "- 목록처럼 보이는 줄  x", "```"},
		linesOf(bufferOf(t, m)), "줄이 들어가지 않고 커서 자리에 칸이 든다")
}

func TestShiftKeepsTheCharacterKindOfTheLine(t *testing.T) {
	// tab 파일에 space 로 들여쓴 줄이 섞여 있어도 칸만 맞추고 글자는 둔다.
	m := newIndentEditor(t, "a.go", "indent_style = tab", "    foo\n\tbar\n")

	m = send(m, "V", "j", ">")

	assert.Equal(t, []string{"        foo", "\t\tbar"}, linesOf(bufferOf(t, m)))
}

// tab 폭이 파일마다 갈리는지 본다. 재는 자리와 그리는 자리가 같은 답을 써야 한다(ADR-0096).
func TestTabWidthFollowsEditorconfig(t *testing.T) {
	path := writeEditorconfig(t, "[*]\nindent_style = tab\ntab_width = 8\n", "a.go")
	buf := textarea.NewBuffer(path, []byte("\tab\n"))

	require.Equal(t, 8, buf.TabWidth())

	assert.Equal(t, 8, textarea.ScreenColAt(buf.Line(0), 1, buf.TabWidth()), "tab 하나가 8 칸이다")
	assert.Equal(t, []int{0, 1}, textarea.WrapOffsets(buf.Line(0), 6, buf.TabWidth()),
		"8 칸짜리 tab 은 너비 6 을 넘어 그 뒤가 다음 행으로 간다")

	_, col := expandRow(buf.Line(0), 0, len(buf.Line(0)), 0, markWhitespace(buf.Line(0)), buf.TabWidth())
	assert.Equal(t, 10, col, "그린 뒤의 칸도 8 + `ab` 다")
}
