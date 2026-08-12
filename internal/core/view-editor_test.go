package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEditor 의 height 는 편집 내용을 그릴 높이다. statusBar 는 별도로 얹힌다.
func newTestEditor(data string, width, height int) viewEditorNormal {
	return viewEditorNormal{
		editor: editor{
			buffers: []Buffer{newBuffer("test.txt", []byte(data))},
			width:   width,
			height:  height + statusBarHeight,
		},
	}
}

// textOf 는 statusBar 를 뺀 편집 내용만 돌려준다.
func textOf(t *testing.T, view tea.View) string {
	t.Helper()

	rows := strings.Split(view.Content, "\n")
	require.GreaterOrEqual(t, len(rows), statusBarHeight)

	return strings.Join(rows[:len(rows)-statusBarHeight], "\n")
}

func TestViewEditorRendersVisibleLines(t *testing.T) {
	m := newTestEditor("a\nb\nc\nd\ne\n", 80, 3)

	view := m.View()

	assert.Equal(t, "a\nb\nc", textOf(t, view), "화면 높이만큼만 그린다")
	assert.True(t, view.AltScreen)
	assert.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
}

// 화면보다 긴 줄은 잘리지 않고 다음 행으로 넘어간다.
func TestViewEditorWrapsLongLines(t *testing.T) {
	m := newTestEditor("0123456789\nshort\n", 4, 4)

	view := m.View()

	assert.Equal(t, "0123\n4567\n89\nshor", textOf(t, view))
}

// 두 칸 글자가 경계에 걸치면 그 글자는 다음 행으로 넘어가야 한다.
// 반 칸만 그리면 그 아래가 전부 밀린다.
func TestViewEditorWrapsWideCharAtBoundary(t *testing.T) {
	m := newTestEditor("한글\n", 3, 2)

	view := m.View()

	assert.Equal(t, "한\n글", textOf(t, view))
}

func TestViewEditorCursorPosition(t *testing.T) {
	m := newTestEditor("한글abc\nsecond\n", 80, 5)
	buf := &m.buffers[0]

	require.NotNil(t, m.View().Cursor)
	assert.Equal(t, tea.Position{X: 0, Y: 0}, m.View().Cursor.Position)

	// 한글 한 글자 = 두 칸
	buf.moveRight(m.width)
	assert.Equal(t, tea.Position{X: 2, Y: 0}, m.View().Cursor.Position)

	buf.moveDown(1, m.width)
	assert.Equal(t, tea.Position{X: 2, Y: 1}, m.View().Cursor.Position)
}

// 스크롤된 뒤에도 커서는 화면 기준으로 그려져야 한다.
func TestViewEditorCursorAfterScroll(t *testing.T) {
	m := newTestEditor(strings.Repeat("line\n", 100), 80, 10)
	buf := &m.buffers[0]

	buf.moveDown(20, m.width)
	buf.scrollTo(m.width, m.textHeight())

	require.Equal(t, 11, buf.top)
	assert.Equal(t, 9, m.View().Cursor.Position.Y, "커서는 화면 맨 아래 줄")
}

// wrap 된 줄 안에서 아래로 내려가면 커서가 다음 화면 행에 있어야 한다.
func TestViewEditorCursorInWrappedLine(t *testing.T) {
	m := newTestEditor("0123456789\n", 4, 4)
	buf := &m.buffers[0]

	buf.moveRight(m.width)
	assert.Equal(t, tea.Position{X: 1, Y: 0}, m.View().Cursor.Position)

	buf.moveDown(1, m.width)
	assert.Equal(t, tea.Position{X: 1, Y: 1}, m.View().Cursor.Position, "같은 줄의 두 번째 행")
}

func TestViewEditorHandlesWindowSize(t *testing.T) {
	m := newTestEditor(strings.Repeat("line\n", 100), 0, 0)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 5})

	resized, ok := updated.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 40, resized.width)
	assert.Equal(t, 5, resized.height)
	assert.Equal(t, 5, strings.Count(resized.View().Content, "\n")+1, "편집 내용 + statusBar 가 화면을 채운다")
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

// tab 을 그대로 넘기면 bubbletea 가 버려서 들여쓰기가 사라진다. 공백으로 펼쳐야 한다.
func TestExpandTabs(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want string
	}{
		{name: "tab 없으면 그대로", row: "abc", want: "abc"},
		{name: "줄 앞 tab", row: "\tabc", want: "        abc"},
		{name: "글자 뒤 tab 은 남은 칸만", row: "ab\tc", want: "ab      c"},
		{name: "tab 두 개", row: "\t\ta", want: "                a"},
		{name: "7 칸 뒤 tab 은 1 칸", row: "0123456\tx", want: "0123456 x"},
		{name: "한글 뒤 tab", row: "한글\tx", want: "한글    x"}, // 한글 4 칸 + 4 칸
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, expandTabs([]byte(test.row)))
		})
	}
}

func TestViewEditorRendersTabs(t *testing.T) {
	m := newTestEditor("func main() {\n\tprintln()\n}\n", 40, 3)

	assert.Equal(t, "func main() {\n        println()\n}", textOf(t, m.View()))
}
