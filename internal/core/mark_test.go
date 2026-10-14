package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"
)

// markText 는 mark 시험에 쓰는 글이다. 들여쓴 줄이 있어서 `'a` 와 백틱 a 가 갈린다.
const markText = "zero\n  one two\ntwo\nthree\nfour\n"

// markedEditor 는 둘째 줄의 `two` 에 `a` 를 찍고 끝 줄로 내려간 편집기다.
func markedEditor(t *testing.T) tea.Model {
	t.Helper()

	m := send(newTestEditor(markText, 40, 10), "j", "w", "w", "m", "a", "G")
	require.Equal(t, 4, bufferOf(t, m).Cursor.Line, "G 는 끝 줄이다")

	return m
}

// `'a` 는 그 줄의 첫 비공백, 백틱 a 는 찍은 칸이다.
func TestMarkJumpLineAndExact(t *testing.T) {
	assert.Equal(t, scheme.Cursor{Line: 1, Col: 2}, bufferOf(t, send(markedEditor(t), "'", "a")).Cursor)
	assert.Equal(t, scheme.Cursor{Line: 1, Col: 6}, bufferOf(t, send(markedEditor(t), "`", "a")).Cursor)
}

// 찍지 않은 이름이면 가지 않고 그렇다고 알린다. 안 먹은 키로 읽히지 않게 한다.
func TestMarkMissingNotifies(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "j", "'", "b")

	assert.Equal(t, 1, bufferOf(t, m).Cursor.Line)
	assert.Equal(t, "찍지 않은 mark 입니다: b", m.(viewEditorNormal).notice)
}

// 찍을 수 없는 이름은 아무 일도 하지 않는다. 대문자(파일을 넘는 mark) 는 두지 않았다(ADR-0154).
func TestMarkRejectsUpperName(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "j", "m", "A", "G", "'", "A")

	assert.Empty(t, bufferOf(t, m).Marks())
	assert.Equal(t, 4, bufferOf(t, m).Cursor.Line, "`'A` 는 아무 데도 가지 않는다")
}

// 한글 입력 상태에서도 찍고 간다. `ㅡ` 가 `m`, `ㅁ` 이 `a` 다(ADR-0008).
func TestMarkWithHangulKeys(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "ㅓ", "ㅡ", "ㅁ", "G", "'", "ㅁ")

	assert.Equal(t, 1, bufferOf(t, m).Cursor.Line)
}

// `'a` 는 멀리 뛰는 이동이라 `ctrl+o` 로 돌아온다(ADR-0082).
func TestMarkJumpIsRecorded(t *testing.T) {
	m := send(markedEditor(t), "'", "a")
	require.Equal(t, 1, bufferOf(t, m).Cursor.Line)

	m = send(m, "ctrl+o")

	assert.Equal(t, 4, bufferOf(t, m).Cursor.Line)
}

// `d'a` 는 줄 단위, `d` 백틱 a 는 글자 단위다. vim 과 같다.
func TestMarkAsOperatorRange(t *testing.T) {
	linewise := send(newTestEditor(markText, 40, 10), "j", "m", "a", "j", "j", "d", "'", "a")
	assert.Equal(t, []string{"zero", "four"}, linesOf(bufferOf(t, linewise)))

	charwise := send(newTestEditor(markText, 40, 10), "j", "w", "m", "a", "w", "d", "`", "a")
	assert.Equal(t, []string{"zero", "  two", "two", "three", "four"}, linesOf(bufferOf(t, charwise)))
}

// 찍지 않은 mark 로 지우려 하면 파일을 건드리지 않고 알린다.
func TestMarkMissingOperatorNotifies(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "d", "'", "a")

	assert.Equal(t, []string{"zero", "  one two", "two", "three", "four"}, linesOf(bufferOf(t, m)))
	assert.Equal(t, "찍지 않은 mark 입니다: a", m.(viewEditorNormal).notice)
}

// 찍는 것은 operator 뒤에 올 수 없다. `dma` 는 아무것도 아니다.
func TestMarkSetAfterOperatorIsNothing(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "d", "m", "a")

	assert.Empty(t, bufferOf(t, m).Marks())
	assert.Len(t, linesOf(bufferOf(t, m)), 5)
}

// 위에 줄이 늘면 mark 가 따라 내려가고, 그 줄을 지웠다가 되돌리면 mark 도 돌아온다.
func TestMarkFollowsEditsAndUndo(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "j", "j", "m", "a", "g", "g", "O", "esc")
	assert.Equal(t, 3, bufferOf(t, send(m, "'", "a")).Cursor.Line, "위에 한 줄이 늘었다")

	m = send(m, "'", "a", "d", "d", "g", "g", "'", "a")
	assert.Equal(t, "찍지 않은 mark 입니다: a", m.(viewEditorNormal).notice)

	m = send(m, "u", "g", "g", "'", "a")
	assert.Equal(t, 3, bufferOf(t, m).Cursor.Line, "`u` 가 mark 를 되살린다")
}

// visual 에서 `'a` 는 고른 범위를 그 줄까지 늘린다.
func TestMarkExtendsSelection(t *testing.T) {
	m := send(markedEditor(t), "V", "'", "a")
	require.IsType(t, viewEditorVisual{}, m)

	area, ok := bufferOf(t, m).SelectionRange()
	require.True(t, ok)
	assert.Equal(t, 1, area.Start.Line)
	assert.Equal(t, 4, area.End.Line)
}

// `:'a,'bd` 는 두 mark 사이 줄을 지운다(ADR-0154).
func TestMarkAddressInCommandLine(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), "j", "m", "a", "j", "j", "m", "b", ":")
	m = sendText(m, "'a,'bd")
	m = send(m, "enter")

	assert.Equal(t, []string{"zero", "four"}, linesOf(bufferOf(t, m)))
}

// 찍을 수 없는 이름은 무엇이 잘못인지 말한다.
func TestMarkAddressRejectsBadName(t *testing.T) {
	m := send(newTestEditor(markText, 40, 10), ":")
	m = sendText(m, "'Ad")
	m = send(m, "enter")

	assert.Contains(t, barOf(t, m)[1], "mark 이름이 아닙니다: 'A")
}
