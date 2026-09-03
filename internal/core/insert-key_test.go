package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insert mode 에서 키를 눌러 보는 시험이다. 글을 고치는 것 자체는 buffer-edit_test.go 가 본다.

// --- 키를 통한 경로 ---

func TestTypingThroughKeys(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "Y")

	assert.Equal(t, "XYabc", string(bufferOf(t, m).Line(0)))
	assert.Equal(t, 2, cursorColOf(t, m))
}

func TestNormalModeDoesNotType(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "X")

	assert.Equal(t, "abc", string(bufferOf(t, m).Line(0)), "normal 에서는 글자가 안 들어간다")
}

func TestEnterKeySplitsLine(t *testing.T) {
	var m tea.Model = newTestEditor("abcd\n", 40, 5)

	m = send(m, "right", "right", "i", "enter")

	assert.Equal(t, []string{"ab", "cd"}, linesOf(bufferOf(t, m)))
}

func TestBackspaceKey(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "a", "backspace")

	assert.Equal(t, "bc", string(bufferOf(t, m).Line(0)))
}

// tab 키는 tab 글자가 아니라 이 파일의 한 단계를 넣는다(indent.go).
// 무엇이 한 단계인지는 `.editorconfig` 가 정하므로 여기서 놓아 준다.
func TestTabKeyInsertsOneIndentUnit(t *testing.T) {
	var m tea.Model = newIndentEditor(t, "a.txt", "indent_style = tab", "ab\n")

	m = send(m, "i", "tab")

	assert.Equal(t, "\tab", string(bufferOf(t, m).Line(0)))
}

// modifier 조합은 Text 가 비어 있어서 글자로 들어가지 않는다.
func TestCtrlKeyDoesNotType(t *testing.T) {
	m := newTestEditor("abc\n", 40, 5)
	insert, _ := m.Update(key("i"))

	next, _ := insert.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	assert.Equal(t, "abc", string(bufferOf(t, next).Line(0)))
}

func TestPasteInsertsMultipleLines(t *testing.T) {
	m := newTestEditor("ad\n", 40, 5)
	insert, _ := m.Update(key("i"))
	insert, _ = insert.Update(key("right"))

	next, _ := insert.Update(tea.PasteMsg{Content: "b\nX\nc"})

	assert.Equal(t, []string{"ab", "X", "cd"}, linesOf(bufferOf(t, next)))
}

// u 로 되돌리고 ctrl+r 로 다시 적용한다.
func TestUndoRedoThroughKeys(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "Y", "esc")
	require.Equal(t, "XYabc", string(bufferOf(t, m).Line(0)))

	m = send(m, "u")
	assert.Equal(t, "abc", string(bufferOf(t, m).Line(0)), "타이핑 구간 전체가 한 번에")

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	assert.Equal(t, "XYabc", string(bufferOf(t, updated).Line(0)))
}

// insert mode 에서 화살표로 움직이면 undo 구간이 끊긴다.
func TestArrowInInsertBreaksUndoRun(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "right", "Y", "esc")
	require.Equal(t, "XaYbc", string(bufferOf(t, m).Line(0)))

	m = send(m, "u")
	assert.Equal(t, "Xabc", string(bufferOf(t, m).Line(0)), "한 번에 하나씩")

	m = send(m, "u")
	assert.Equal(t, "abc", string(bufferOf(t, m).Line(0)))
}

// undo 뒤 커서가 줄 끝을 넘지 않아야 한다. normal 커서는 글자 위에 있다.
func TestUndoClampsCursorInNormalMode(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "right", "a", "X", "Y", "esc", "u")

	buf := bufferOf(t, m)
	assert.Equal(t, "ab", string(buf.Line(0)))
	assert.Less(t, buf.Cursor.Col, len(buf.Line(0))+1)
}
