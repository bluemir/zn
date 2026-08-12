package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// barOf 는 화면 아래 statusBar 두 줄을 돌려준다.
func barOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	rows := strings.Split(m.View().Content, "\n")
	require.GreaterOrEqual(t, len(rows), statusBarHeight)

	return rows[len(rows)-statusBarHeight:]
}

// mode 이름은 각 mode 의 View 가 직접 넘긴다. 바깥에서 mode 를 물을 필요가 없다(ADR-0002).
func TestStatusBarShowsMode(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 3)

	assert.Contains(t, barOf(t, m)[0], "NORMAL")

	m = send(m, "i")
	assert.Contains(t, barOf(t, m)[0], "INSERT")

	m = send(m, "esc")
	assert.Contains(t, barOf(t, m)[0], "NORMAL")
}

func TestStatusBarShowsPath(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)

	assert.Contains(t, barOf(t, m)[0], "test.txt")
}

func TestStatusBarShowsNoNameForEmptyPath(t *testing.T) {
	m := viewEditorNormal{
		editor: editor{
			buffers: []Buffer{newEmptyBuffer("")},
			width:   40,
			height:  3 + statusBarHeight,
		},
	}

	assert.Contains(t, barOf(t, m)[0], "[No Name]")
}

// 줄과 칸은 1 부터 세고, 칸은 화면 칸이라 한글은 2 씩 늘어난다.
func TestStatusBarShowsCursorPosition(t *testing.T) {
	var m tea.Model = newTestEditor("한글abc\nsecond\n", 40, 3)

	assert.Contains(t, barOf(t, m)[1], "1:1")

	m = send(m, "right")
	assert.Contains(t, barOf(t, m)[1], "1:3", "한글 한 글자 뒤는 3 칸")

	m = send(m, "down")
	assert.Contains(t, barOf(t, m)[1], "2:3")
}

func TestStatusBarShowsLineCount(t *testing.T) {
	m := newTestEditor("a\nb\nc\n", 40, 3)

	assert.Contains(t, barOf(t, m)[1], "3 줄")
}

// statusBar 가 화면 너비를 넘으면 터미널이 줄바꿈해서 화면이 밀린다.
func TestStatusBarTruncatesToWidth(t *testing.T) {
	m := viewEditorNormal{
		editor: editor{
			buffers: []Buffer{newBuffer(strings.Repeat("long-path/", 20)+"file.txt", []byte("abc\n"))},
			width:   20,
			height:  3 + statusBarHeight,
		},
	}

	for _, row := range barOf(t, m) {
		assert.LessOrEqual(t, screenColAt([]byte(row), len(row)), 20)
	}
}

// 파일이 화면보다 짧아도 statusBar 는 화면 아래에 붙어 있어야 한다.
func TestStatusBarStaysAtBottom(t *testing.T) {
	m := newTestEditor("only\n", 40, 5)

	rows := strings.Split(m.View().Content, "\n")

	require.Len(t, rows, 5+statusBarHeight)
	assert.Equal(t, "only", rows[0])
	assert.Contains(t, rows[5], "NORMAL", "빈 줄로 채우고 맨 아래에 붙인다")
}

// statusBar 가 자리를 차지하므로 편집 내용 높이는 그만큼 줄어든다.
func TestTextHeightExcludesStatusBar(t *testing.T) {
	m := newTestEditor("", 40, 0)
	m.height = 10

	assert.Equal(t, 10-statusBarHeight, m.textHeight())
}

// 화면이 statusBar 보다 작아도 죽지 않아야 한다.
func TestTinyScreenDoesNotPanic(t *testing.T) {
	for _, height := range []int{0, 1, 2, 3} {
		m := viewEditorNormal{
			editor: editor{
				buffers: []Buffer{newBuffer("t", []byte("a\nb\n"))},
				width:   10,
				height:  height,
			},
		}

		assert.NotPanics(t, func() { m.View() }, "height=%d", height)
	}
}
