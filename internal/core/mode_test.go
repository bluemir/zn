package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	default:
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
}

// send 는 키를 차례로 넣고 마지막 model 을 돌려준다.
// mode 가 model 교체로 나타나므로 중간에 型 이 바뀐다.
func send(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

// bufferOf 는 어느 mode 든 활성 buffer 를 꺼낸다.
func bufferOf(t *testing.T, m tea.Model) Buffer {
	t.Helper()

	switch v := m.(type) {
	case viewEditorNormal:
		return v.buffers[v.active]
	case viewEditorInsert:
		return v.buffers[v.active]
	default:
		t.Fatalf("편집 화면이 아니다: %T", m)
		return Buffer{}
	}
}

// cursorColOf 는 활성 buffer 의 커서 offset 이다.
func cursorColOf(t *testing.T, m tea.Model) int {
	t.Helper()
	return bufferOf(t, m).cursorCol
}

func TestStartsInNormalMode(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	assert.IsType(t, viewEditorNormal{}, m)
}

func TestEnterInsertWithI(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)
	m = send(m, "right") // 커서를 b 위로

	before := cursorColOf(t, m)
	m = send(m, "i")

	assert.IsType(t, viewEditorInsert{}, m)
	assert.Equal(t, before, cursorColOf(t, m), "i 는 커서를 옮기지 않는다")
}

// a 는 커서 글자 뒤에 넣으므로 커서가 한 글자 오른쪽으로 간다.
func TestEnterInsertWithA(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "a")

	assert.IsType(t, viewEditorInsert{}, m)
	assert.Equal(t, 1, cursorColOf(t, m))
}

// a 를 줄 끝 글자에서 누르면 줄 끝 다음 칸으로 간다. insert mode 에서만 갈 수 있는 자리다.
func TestEnterInsertWithAAtLineEnd(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)
	m = send(m, "right") // 마지막 글자 b 위
	require.Equal(t, 1, cursorColOf(t, m))

	m = send(m, "a")

	assert.Equal(t, 2, cursorColOf(t, m), "줄 끝 다음 칸")
}

func TestEnterInsertWithAOnEmptyLine(t *testing.T) {
	var m tea.Model = newTestEditor("\n", 40, 5)

	m = send(m, "a")

	assert.IsType(t, viewEditorInsert{}, m)
	assert.Equal(t, 0, cursorColOf(t, m), "빈 줄에서는 움직일 곳이 없다")
}

// a 는 한글도 한 글자로 건너뛴다.
func TestEnterInsertWithAOverHangul(t *testing.T) {
	var m tea.Model = newTestEditor("한글\n", 40, 5)

	m = send(m, "a")

	assert.Equal(t, 3, cursorColOf(t, m), "한 글자 = 3 byte")
}

// esc 는 normal 로 돌아오면서 커서를 왼쪽 글자 위로 옮긴다. vim 과 같다.
func TestEscapeReturnsToNormal(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)
	m = send(m, "right", "a") // 줄 끝 다음 칸(offset 2)
	require.Equal(t, 2, cursorColOf(t, m))

	m = send(m, "esc")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 1, cursorColOf(t, m), "줄 끝 다음 칸에서 마지막 글자 위로")
}

// a<Esc> 는 제자리로 돌아온다. a 가 오른쪽으로 한 칸, esc 가 왼쪽으로 한 칸이다.
func TestAppendThenEscapeKeepsPosition(t *testing.T) {
	var m tea.Model = newTestEditor("abcde\n", 40, 5)
	m = send(m, "right", "right")
	before := cursorColOf(t, m)
	require.Equal(t, 2, before)

	m = send(m, "a", "esc")

	assert.Equal(t, before, cursorColOf(t, m))
}

// i<Esc> 는 한 글자 왼쪽으로 간다. vim 의 동작이다.
func TestInsertThenEscapeMovesLeft(t *testing.T) {
	var m tea.Model = newTestEditor("abcde\n", 40, 5)
	m = send(m, "right", "right")
	require.Equal(t, 2, cursorColOf(t, m))

	m = send(m, "i", "esc")

	assert.Equal(t, 1, cursorColOf(t, m))
}

// 줄 시작에서는 더 왼쪽이 없어서 제자리다.
func TestEscapeAtLineStartStays(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "esc")

	assert.Equal(t, 0, cursorColOf(t, m))
}

// esc 는 한글도 한 글자 단위로 옮긴다.
func TestEscapeMovesByClusterOverHangul(t *testing.T) {
	var m tea.Model = newTestEditor("한글다\n", 40, 5)
	m = send(m, "right") // 글 위 (offset 3)
	require.Equal(t, 3, cursorColOf(t, m))

	m = send(m, "a", "esc")

	assert.Equal(t, 3, cursorColOf(t, m), "a 로 6, esc 로 다시 3")
}

func TestEscapeOnEmptyLineStaysAtZero(t *testing.T) {
	var m tea.Model = newTestEditor("\n", 40, 5)
	m = send(m, "i", "esc")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 0, cursorColOf(t, m))
}

// normal mode 에서는 줄 끝 다음 칸으로 갈 수 없다.
func TestNormalModeCursorCannotPassLineEnd(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "right", "right", "right")

	assert.Equal(t, 1, cursorColOf(t, m), "마지막 글자에서 멈춘다")
}

// insert mode 에서는 줄 끝 다음 칸까지 갈 수 있다.
func TestInsertModeCursorReachesLineEnd(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "i", "right", "right", "right")

	assert.Equal(t, 2, cursorColOf(t, m))
}

// 긴 줄에서 짧은 줄로 내려가면 normal mode 는 마지막 글자 위에 선다.
func TestNormalModeClampsWhenMovingToShorterLine(t *testing.T) {
	var m tea.Model = newTestEditor("abcde\nx\n", 40, 5)
	m = send(m, "right", "right", "right") // offset 3
	require.Equal(t, 3, cursorColOf(t, m))

	m = send(m, "down")

	assert.Equal(t, 0, cursorColOf(t, m), "한 글자 줄에서는 그 글자 위")
}

// 짧은 줄을 지나가도 원래 칸으로 돌아와야 한다. clamp 가 desiredCol 을 건드리면 깨진다.
func TestNormalModeKeepsDesiredColThroughShortLine(t *testing.T) {
	var m tea.Model = newTestEditor("abcde\nx\nabcde\n", 40, 5)
	m = send(m, "right", "right", "right")
	require.Equal(t, 3, cursorColOf(t, m))

	m = send(m, "down", "down")

	assert.Equal(t, 3, cursorColOf(t, m), "긴 줄로 돌아오면 원래 칸")
}

// insert mode 에서 i 나 a 는 mode 를 바꾸지 않고 글자로 들어간다.
func TestInsertModeTypesModeKeys(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "i", "a")

	assert.IsType(t, viewEditorInsert{}, m)
	assert.Equal(t, "iaabc", string(bufferOf(t, m).lines[0]))
	assert.Equal(t, 2, cursorColOf(t, m))
}

// mode 는 커서 모양으로 드러난다. statusBar 가 없어서 이게 유일한 표시다.
func TestCursorShapeShowsMode(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)
	require.NotNil(t, m.View().Cursor)
	assert.Equal(t, tea.CursorBlock, m.View().Cursor.Shape, "normal 은 블록")

	m = send(m, "i")
	assert.Equal(t, tea.CursorBar, m.View().Cursor.Shape, "insert 는 막대")

	m = send(m, "esc")
	assert.Equal(t, tea.CursorBlock, m.View().Cursor.Shape)
}

// ctrl+c 는 :q 와 같은 경로다. 바뀐 것이 없으면 묻지 않고 나간다.
func TestQuitFromBothModesWhenClean(t *testing.T) {
	for _, keys := range [][]string{{}, {"i"}} {
		m := send(newTestEditor("abc\n", 40, 5), keys...)

		next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

		assert.IsType(t, finalExit{}, next)
	}
}

// 저장하지 않은 변경이 있을 때만 확인창이 뜬다.
func TestQuitFromBothModesWhenDirty(t *testing.T) {
	for _, keys := range [][]string{{"i", "X", "esc"}, {"i", "X"}} {
		m := send(newTestEditor("abc\n", 40, 5), keys...)

		next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

		assert.IsType(t, viewQuitConfirm{}, next)
	}
}
