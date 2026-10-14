package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openMarks 는 첫 줄에 `b`, 셋째 줄에 `a` 를 찍고 끝 줄에서 `:marks` 를 연 판이다.
func openMarks(t *testing.T) tea.Model {
	t.Helper()

	m := send(newTestEditor(markText, 40, 12), "m", "b", "j", "j", "l", "m", "a", "G", ":")
	m = sendText(m, "marks")
	m = send(m, "enter")
	require.IsType(t, viewMarks{}, m)

	return m
}

// 이름 순으로 이름·자리·그 줄의 글을 보인다. 여는 것만으로는 커서를 옮기지 않는다.
func TestMarksListsByName(t *testing.T) {
	m := openMarks(t)

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "▸ a 3:2")
	assert.Contains(t, content, "  b 1:1")
	assert.Less(t, strings.Index(content, "a 3:2"), strings.Index(content, "b 1:1"))
	assert.Equal(t, 4, bufferOf(t, m).Cursor.Line)
}

// `j`/`k` 가 커서를 그 mark 로 옮겨 보여주고, `esc` 는 연 자리로 되돌린다.
func TestMarksPreviewAndCancel(t *testing.T) {
	m := send(openMarks(t), "j")
	assert.Equal(t, 0, bufferOf(t, m).Cursor.Line, "`b` 로 갔다")

	m = send(m, "k")
	assert.Equal(t, 2, bufferOf(t, m).Cursor.Line, "`a` 로 갔다")

	m = send(m, "esc")
	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 4, bufferOf(t, m).Cursor.Line)
}

// `enter` 는 그 자리로 확정하고 되돌아오기 이력에 연 자리를 담는다.
func TestMarksConfirmRecordsJump(t *testing.T) {
	m := send(openMarks(t), "enter")
	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 2, bufferOf(t, m).Cursor.Line)
	assert.Equal(t, 1, bufferOf(t, m).Cursor.Col, "찍은 칸이다")

	m = send(m, "ctrl+o")
	assert.Equal(t, 4, bufferOf(t, m).Cursor.Line)
}

// `x` 는 고른 mark 를 지운다. 판은 열린 채 남는다.
func TestMarksDeleteSelected(t *testing.T) {
	m := send(openMarks(t), "x")
	require.IsType(t, viewMarks{}, m)

	marks := bufferOf(t, m).Marks()
	require.Len(t, marks, 1)
	assert.Equal(t, "b", marks[0].Name)
	assert.Contains(t, ansi.Strip(m.View().Content), "mark 를 지웠습니다: a")
}

// 찍은 것이 없어도 열린다. 「없다」를 보여주는 것이 아무 일도 안 나는 것보다 낫다.
func TestMarksOpensWhenEmpty(t *testing.T) {
	m := send(newTestEditor(markText, 40, 12), ":")
	m = sendText(m, "marks")
	m = send(m, "enter")
	require.IsType(t, viewMarks{}, m)

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "찍은 mark 가 없습니다")
	assert.Contains(t, content, "mark 0 개")

	m = send(m, "enter")
	assert.IsType(t, viewEditorNormal{}, m)
}

// 팔레트의 「mark 목록」도 같은 판을 연다.
func TestMarksFromPalette(t *testing.T) {
	m := send(newTestEditor(markText, 40, 12), "m", "a", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(typeInto(m, ">marks"), "enter")

	assert.IsType(t, viewMarks{}, m)
}
