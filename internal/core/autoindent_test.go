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
	assert.Equal(t, 2, buf.cursorCol, "들여쓰기 다음에 글자가 하나")
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
	assert.Equal(t, 1, buf.cursorCol, "`}` 뒤")
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
	assert.Equal(t, 1, buf.cursorLine)
	assert.Equal(t, 2, buf.cursorCol)
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
	assert.Equal(t, 1, buf.cursorCol, "들여쓰기 다음")
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
