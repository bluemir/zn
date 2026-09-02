package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newIndentEditor 는 `.editorconfig` 를 정해 둔 자리에서 편집기를 연다.
//
// 한 단계가 무엇인지가 상위 디렉터리의 `.editorconfig` 에 매여 있어서(indent.go), 이 저장소의
// 것을 그대로 쓰면 저장소 설정이 바뀔 때 시험이 같이 흔들린다.
func newIndentEditor(t *testing.T, name, style, data string) tea.Model {
	t.Helper()

	path := writeEditorconfig(t, "[*]\n"+style+"\n", name)

	return newTestEditorFile(path, data, 60, 10)
}

func TestEnterIndentsAfterOpeningBrace(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n}\n")

	m = send(m, "$", "a", "enter", "x")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"func f() {", "\tx", "}"}, linesOf(buf))
	assert.Equal(t, 2, buf.cursor.Col, "들여쓰기 다음에 글자가 하나")
}

func TestEnterKeepsIndentOnPlainLine(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n\ta()\n}\n")

	m = send(m, "j", "$", "a", "enter", "b")

	assert.Equal(t, []string{"func f() {", "\ta()", "\tb", "}"}, linesOf(bufferOf(t, m)))
}

func TestTypingClosingBracePullsTheLineBack(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n\ta()\n")

	// 줄 끝에서 Enter 를 치면 새 줄이 tab 하나를 받는다. 거기서 `}` 를 치면 도로 나온다.
	m = send(m, "j", "$", "a", "enter", "}")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"func f() {", "\ta()", "}"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursor.Col, "`}` 뒤")
}

func TestTypingBraceMidLineDoesNotMove(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n\ta()\n")

	// 커서 앞에 글자가 있으면 그 줄은 움직이지 않는다.
	m = send(m, "j", "$", "a", "enter", "x", "}")

	assert.Equal(t, []string{"func f() {", "\ta()", "\tx}"}, linesOf(bufferOf(t, m)))
}

func TestOneUndoTakesTheIndentWithIt(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n")

	m = send(m, "$", "a", "enter", "a", "}", "esc", "u")

	assert.Equal(t, []string{"func f() {"}, linesOf(bufferOf(t, m)),
		"Enter·들여쓰기·글자·당기기가 한 번에 돌아간다")
}

func TestOpenLineAboveTakesTheIndentFromTheLineAbove(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n\ta()\n}\n")

	m = send(m, "j", "O", "b")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"func f() {", "\tb", "\ta()", "}"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursor.Line)
	assert.Equal(t, 2, buf.cursor.Col)
}

func TestOpenLineAboveAtFirstLineHasNoIndent(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "\tab\n")

	m = send(m, "O", "x")

	assert.Equal(t, []string{"x", "\tab"}, linesOf(bufferOf(t, m)))
}

func TestReplaceWithNewlineIndents(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {X}\n")

	// `X` 자리에서 `r<Enter>` 를 치면 그 글자가 사라지고 줄이 갈린다.
	m = send(m, "$", "h", "r", "enter")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"func f() {", "\t}"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursor.Col, "들여쓰기 다음")
}

func TestPasteIsNotIndented(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n")

	m = send(m, "$", "a", "enter")
	insert, ok := m.(viewEditorInsert)
	require.True(t, ok)

	next, _ := insert.Update(tea.PasteMsg{Content: "a()\nb()"})

	assert.Equal(t, []string{"func f() {", "\ta()", "b()"}, linesOf(bufferOf(t, next)),
		"붙여넣은 줄에 들여쓰기가 겹치지 않는다")
}

func TestMarkdownListContinuesOnEnter(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2", "- 첫째\n")

	m = send(m, "$", "a", "enter", "x")

	assert.Equal(t, []string{"- 첫째", "- x"}, linesOf(bufferOf(t, m)))
}

func TestMarkdownNumberedListCountsUp(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2", "1. 첫째\n")

	m = send(m, "$", "a", "enter", "x")

	assert.Equal(t, []string{"1. 첫째", "2. x"}, linesOf(bufferOf(t, m)))
}

func TestMarkdownEmptyItemEndsTheList(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2", "- \n")

	m = send(m, "$", "a", "enter", "x")

	assert.Equal(t, []string{"- ", "x"}, linesOf(bufferOf(t, m)),
		"빈 항목에서 Enter 를 치면 표시를 잇지 않는다")
}

// markdown 코드펜스 안은 안쪽 언어의 규칙을 받는다. 강조를 넘기는 것과 같은 자리다(ADR-0102).
func TestFencedCodeTakesTheInnerRule(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"글\n\n```go\nfunc f() {\n```\n")

	// 코드펜스 안의 `{` 다음에서 Enter 를 친다. markdown 규칙이면 여기서 안 들어간다.
	m = send(m, "j", "j", "j", "$", "a", "enter", "x")

	assert.Equal(t, []string{"글", "", "```go", "func f() {", "  x", "```"},
		linesOf(bufferOf(t, m)))
}

// 코드펜스 안에서 `}` 를 치면 go 규칙이 줄을 당긴다.
//
// **이 자리는 담아둔 문맥에 기댄다.** 글자마다 지나는 자리라 거기서 문맥을 채우면 캐시의
// 수렴 판정이 흔들려서, 채우지 않고 있는 것을 쓴다(ADR-0102). 화면을 한 번 그리면 채워지고
// 편집기는 키를 받기 전에 늘 한 번 그린다.
func TestClosingBraceInsideAFencePullsBack(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"```go\nfunc f() {\n```\n")

	m = send(m, "j", "$", "a", "enter")
	contentRowsOf(t, m) // 편집기는 키마다 한 번 그린다. 그리면서 문맥이 새 줄까지 찬다

	m = send(m, "}")

	assert.Equal(t, []string{"```go", "func f() {", "}", "```"},
		linesOf(bufferOf(t, m)), "Enter 로 들어간 줄이 `}` 에 도로 나온다")
}

// 코드펜스 밖은 그대로 markdown 이다. 안쪽 규칙이 펜스 밖으로 새면 목록이 끊긴다.
func TestOutsideTheFenceStaysMarkdown(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"```go\nfunc f() {\n}\n```\n- 항목\n")

	m = send(m, "j", "j", "j", "j", "$", "a", "enter", "x")

	assert.Equal(t, []string{"```go", "func f() {", "}", "```", "- 항목", "- x"},
		linesOf(bufferOf(t, m)), "펜스를 닫은 뒤에는 목록 표시를 잇는다")
}

// 언어를 적지 않은 펜스는 markdown 규칙 그대로다. 안이 무엇인지 우리가 모른다.
func TestFenceWithoutALanguageKeepsMarkdown(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"```\nfunc f() {\n```\n")

	m = send(m, "j", "$", "a", "enter", "x")

	assert.Equal(t, []string{"```", "func f() {", "x", "```"}, linesOf(bufferOf(t, m)))
}

// html 의 `<script>` 안은 js 규칙이다. `<style>` 안은 css 규칙이고 둘이 같은 자리다.
func TestScriptTakesTheJavaScriptRule(t *testing.T) {
	m := newIndentEditor(t, "a.html", "indent_style = space\nindent_size = 2",
		"<body>\n<script>\nfunction f() {\n</script>\n")

	m = send(m, "j", "j", "$", "a", "enter", "x")

	assert.Equal(t, []string{"<body>", "<script>", "function f() {", "  x", "</script>"},
		linesOf(bufferOf(t, m)))
}

func TestUnknownLanguageJustCopiesTheIndent(t *testing.T) {
	m := newIndentEditor(t, "a.txt", "indent_style = tab", "  \tab {\n")

	m = send(m, "$", "a", "enter", "x")

	assert.Equal(t, []string{"  \tab {", "  \tx"}, linesOf(bufferOf(t, m)),
		"규칙이 없으면 앞 줄 공백을 그대로 잇는다")
}

func TestSpaceIndentFollowsTheEditorconfig(t *testing.T) {
	m := newIndentEditor(t, "a.js",
		"indent_style = space\nindent_size = 2", "function f() {\n")

	m = send(m, "$", "a", "enter", "a", "enter", "}")

	assert.Equal(t, []string{"function f() {", "  a", "}"}, linesOf(bufferOf(t, m)))
}

func TestEnterMidLineSeesOnlyWhatStaysAbove(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {}\n")

	// `{` 와 `}` 사이에서 가른다. 줄 전체는 괄호가 닫혀 있지만 위에 남는 것은 열려 있다.
	m = send(m, "$", "i", "enter")

	assert.Equal(t, []string{"func f() {", "\t}"}, linesOf(bufferOf(t, m)))
}
