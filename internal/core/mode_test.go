package core

import (
	"strings"
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
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl+w":
		return tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}
	case "ctrl+p":
		return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+z":
		return tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	default:
		// 나머지는 글자 키다. Text 가 차 있으면 String() 이 그것을 그대로 준다.
		// 한글도 이 길로 온다 — Code 를 byte 가 아니라 rune 으로 세야 자모가 온전하다.
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

// send 는 키를 차례로 넣고 마지막 model 을 돌려준다.
// mode 가 model 교체로 나타나므로 중간에 type 이 바뀐다.
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
	case viewEditorSearch:
		return v.buffers[v.active]
	case viewPalette:
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

// o 는 아래에 빈 줄을 만들고 그 줄에서 insert mode 로 들어간다.
func TestOpenLineBelow(t *testing.T) {
	var m tea.Model = newTestEditor("ab\ncd\n", 40, 5)

	m = send(m, "o")

	require.IsType(t, viewEditorInsert{}, m)
	buf := bufferOf(t, m)
	assert.Equal(t, []string{"ab", "", "cd"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursorLine, "새로 만든 줄 위")
	assert.Equal(t, 0, buf.cursorCol)
}

// 커서가 줄 중간이나 들여쓴 줄에 있어도 새 줄은 빈 줄이다. 들여쓰기를 이어받지 않는다.
func TestOpenLineBelowIgnoresIndentAndCursor(t *testing.T) {
	var m tea.Model = newTestEditor("\tab\ncd\n", 40, 5)
	m = send(m, "right")

	m = send(m, "o", "x")

	assert.Equal(t, []string{"\tab", "x", "cd"}, linesOf(bufferOf(t, m)))
}

// 마지막 줄에서도 아래에 줄이 생긴다.
func TestOpenLineBelowAtLastLine(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "o", "x")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"ab", "x"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursorLine)
}

// O 는 위에 빈 줄을 만든다.
func TestOpenLineAbove(t *testing.T) {
	var m tea.Model = newTestEditor("ab\ncd\n", 40, 5)
	m = send(m, "j")
	require.Equal(t, 1, bufferOf(t, m).cursorLine)

	m = send(m, "O", "x")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"ab", "x", "cd"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursorLine, "새로 만든 줄 위")
}

// 첫 줄에서 O 를 누르면 파일 맨 앞에 줄이 생긴다.
func TestOpenLineAboveAtFirstLine(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "O", "x")

	buf := bufferOf(t, m)
	assert.Equal(t, []string{"x", "ab"}, linesOf(buf))
	assert.Equal(t, 0, buf.cursorLine)
}

// o 로 만든 줄과 거기에 친 글자는 한 번의 u 로 같이 사라진다. vim 과 같다.
func TestUndoOpenLineWithTyping(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "o", "x", "y", "esc", "u")

	assert.Equal(t, []string{"ab"}, linesOf(bufferOf(t, m)))
}

// 한글 상태에서도 낼 수 있다. `ㅐ` 가 `o` 이고 `ㅒ`(shift 자리) 가 `O` 다(ADR-0008).
func TestOpenLineWithHangulKeys(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "ㅐ")
	require.IsType(t, viewEditorInsert{}, m, "ㅐ 는 o 다")
	assert.Equal(t, []string{"ab", ""}, linesOf(bufferOf(t, m)))

	m = send(m, "esc")
	m = send(m, "ㅒ")
	require.IsType(t, viewEditorInsert{}, m, "ㅒ 는 O 다")
	assert.Equal(t, []string{"ab", "", ""}, linesOf(bufferOf(t, m)))
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

// cursorLineOf 는 활성 buffer 의 커서 줄이다.
func cursorLineOf(t *testing.T, m tea.Model) int {
	t.Helper()
	return bufferOf(t, m).cursorLine
}

// hjkl 은 화살표와 같은 이동이다. 위아래만 단위가 다르다(ADR-0006).
func TestNormalModeMovesWithHJKL(t *testing.T) {
	var m tea.Model = newTestEditor("abcde\nfghij\n", 40, 5)

	m = send(m, "l", "l")
	assert.Equal(t, 2, cursorColOf(t, m), "l 은 오른쪽")

	m = send(m, "j")
	assert.Equal(t, 1, cursorLineOf(t, m), "j 는 아래")
	assert.Equal(t, 2, cursorColOf(t, m), "칸은 그대로")

	m = send(m, "h")
	assert.Equal(t, 1, cursorColOf(t, m), "h 는 왼쪽")

	m = send(m, "k")
	assert.Equal(t, 0, cursorLineOf(t, m), "k 는 위")
}

func TestNormalModeHJKLStopAtEdges(t *testing.T) {
	var m tea.Model = newTestEditor("ab\ncd\n", 40, 5)

	m = send(m, "h", "k")
	assert.Equal(t, 0, cursorLineOf(t, m), "첫 줄 위로는 못 간다")
	assert.Equal(t, 0, cursorColOf(t, m), "줄 시작 왼쪽으로는 못 간다")

	m = send(m, "l", "l", "l", "j", "j")
	assert.Equal(t, 1, cursorLineOf(t, m), "마지막 줄 아래로는 못 간다")
	assert.Equal(t, 1, cursorColOf(t, m), "normal 은 마지막 글자 위에서 멈춘다")
}

// 숫자를 앞에 붙이면 그만큼 움직인다.
func TestNormalModeCountedMove(t *testing.T) {
	var m tea.Model = newTestEditor(strings.Repeat("abcdefghij\n", 30), 40, 20)

	m = send(m, "1", "0", "j")
	assert.Equal(t, 10, cursorLineOf(t, m), "10j")

	m = send(m, "5", "l")
	assert.Equal(t, 5, cursorColOf(t, m), "5l")

	m = send(m, "3", "h")
	assert.Equal(t, 2, cursorColOf(t, m), "3h")

	m = send(m, "4", "k")
	assert.Equal(t, 6, cursorLineOf(t, m), "4k")
}

// 줄 수보다 큰 숫자를 쳐도 양끝에서 멈춘다.
func TestNormalModeCountedMoveStopsAtEdges(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\nghi\n", 40, 5)

	m = send(m, "9", "9", "j")
	assert.Equal(t, 2, cursorLineOf(t, m), "마지막 줄")

	m = send(m, "2", "0", "k")
	assert.Equal(t, 0, cursorLineOf(t, m), "첫 줄")

	m = send(m, "9", "9", "l")
	assert.Equal(t, 2, cursorColOf(t, m), "마지막 글자 위")
}

// count 는 동작 하나에만 붙는다. 다음 키에 남으면 안 된다.
func TestNormalModeCountResets(t *testing.T) {
	var m tea.Model = newTestEditor(strings.Repeat("abc\n", 10), 40, 10)

	m = send(m, "3", "j", "j")

	assert.Equal(t, 4, cursorLineOf(t, m), "3j 로 3 줄, j 로 한 줄")
}

// count 를 받지 않는 키가 오면 숫자를 버린다. 그 키는 한 번만 동작한다.
func TestNormalModeCountDiscardedByOtherKey(t *testing.T) {
	var m tea.Model = newTestEditor(strings.Repeat("abc\n", 10), 40, 10)

	m = send(m, "3", "esc", "j")

	assert.Equal(t, 1, cursorLineOf(t, m), "esc 가 숫자를 버려서 j 는 한 줄만 간다")
}

// `0` 은 count 의 첫 자리가 될 수 없다. vim 에서 줄 시작으로 가는 키라 자리를 비워둔다.
func TestNormalModeZeroIsNotACount(t *testing.T) {
	var m tea.Model = newTestEditor(strings.Repeat("abc\n", 10), 40, 10)

	m = send(m, "0", "j")

	assert.Equal(t, 1, cursorLineOf(t, m), "0 은 없는 키라 j 만 동작한다")
}

// j/k 는 논리 줄, ↓/↑ 는 화면 행이다. wrap 된 줄에서 갈린다(ADR-0006).
func TestNormalModeVerticalUnitDiffersFromArrows(t *testing.T) {
	// 폭 10 이라 첫 줄이 화면 행 세 개가 된다.
	data := strings.Repeat("a", 25) + "\nsecond\n"

	m := send(newTestEditor(data, 10, 10), "down")
	assert.Equal(t, 0, cursorLineOf(t, m), "↓ 는 같은 줄의 다음 화면 행")

	m = send(newTestEditor(data, 10, 10), "j")
	assert.Equal(t, 1, cursorLineOf(t, m), "j 는 wrap 된 줄을 한 번에 건넌다")
}

// 단어 이동은 normal mode 의 clamp 를 지나 마지막 글자 위에 선다.
func TestNormalModeWordMotions(t *testing.T) {
	var m tea.Model = newTestEditor("foo bar.baz\nsecond line\n", 40, 5)

	m = send(m, "w")
	assert.Equal(t, 4, cursorColOf(t, m), "w 는 다음 단어")

	m = send(m, "w")
	assert.Equal(t, 7, cursorColOf(t, m), "문장부호도 단어 하나")

	m = send(m, "e")
	assert.Equal(t, 10, cursorColOf(t, m), "e 는 단어 끝")

	m = send(m, "b")
	assert.Equal(t, 8, cursorColOf(t, m), "b 는 단어 처음")

	m = send(m, "2", "w")
	assert.Equal(t, 1, cursorLineOf(t, m), "2w 는 줄을 넘어간다")
	assert.Equal(t, 7, cursorColOf(t, m), "다음 줄의 둘째 단어")
}

// 큰 단어는 공백으로만 끊는다.
func TestNormalModeBigWordMotions(t *testing.T) {
	var m tea.Model = newTestEditor("a.b(c) xy\n", 40, 5)

	m = send(m, "W")
	assert.Equal(t, 7, cursorColOf(t, m), "W 는 공백까지 통째로 건넌다")

	m = send(m, "B")
	assert.Equal(t, 0, cursorColOf(t, m))

	m = send(m, "E")
	assert.Equal(t, 5, cursorColOf(t, m), "E 는 큰 단어의 끝")
}

// 마지막 단어에서 w 를 누르면 줄 끝 다음이 아니라 마지막 글자 위에 선다.
func TestNormalModeWordForwardClampsAtEnd(t *testing.T) {
	var m tea.Model = newTestEditor("only\n", 40, 5)

	m = send(m, "w")

	assert.Equal(t, 3, cursorColOf(t, m))
}

func TestNormalModeLineMotions(t *testing.T) {
	var m tea.Model = newTestEditor("\tindented text\nsecond\n", 40, 5)

	m = send(m, "$")
	assert.Equal(t, 13, cursorColOf(t, m), "$ 는 마지막 글자 위")

	m = send(m, "0")
	assert.Equal(t, 0, cursorColOf(t, m), "0 은 줄 맨 앞. 들여쓰기도 지난다")

	m = send(m, "^")
	assert.Equal(t, 1, cursorColOf(t, m), "^ 는 들여쓰기를 건너뛴 첫 글자")

	m = send(m, "2", "$")
	assert.Equal(t, 1, cursorLineOf(t, m), "2$ 는 한 줄 아래의 줄 끝")
	assert.Equal(t, 5, cursorColOf(t, m))
}

// gg 와 G 는 그 줄의 첫 글자로 간다. 숫자는 되풀이가 아니라 줄 번호다.
func TestNormalModeGotoLine(t *testing.T) {
	var m tea.Model = newTestEditor("one\ntwo\n  three\nfour\nfive\n", 40, 10)

	m = send(m, "G")
	assert.Equal(t, 4, cursorLineOf(t, m), "G 는 마지막 줄")

	m = send(m, "g", "g")
	assert.Equal(t, 0, cursorLineOf(t, m), "gg 는 첫 줄")

	m = send(m, "3", "G")
	assert.Equal(t, 2, cursorLineOf(t, m), "3G 는 3 번째 줄")
	assert.Equal(t, 2, cursorColOf(t, m), "들여쓰기를 건너뛴 첫 글자")

	m = send(m, "2", "g", "g")
	assert.Equal(t, 1, cursorLineOf(t, m), "2gg 도 줄 번호다")

	m = send(m, "9", "9", "G")
	assert.Equal(t, 4, cursorLineOf(t, m), "줄 수를 넘으면 마지막 줄")
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

		assert.IsType(t, viewConfirmDiscard{}, next)
	}
}

// mode 를 옮겨도 editor 는 하나다. 전환 함수가 포인터를 그대로 넘긴다 (ADR-0026).
// 값으로 넘기던 때에는 넘기는 것을 빼먹으면 화면 크기와 커서가 조용히 초기화됐다.
func TestModeTransitionKeepsOneEditor(t *testing.T) {
	normal := newTestEditor("abc\n", 80, 5)

	assert.Same(t, normal.editor, send(normal, "i").(viewEditorInsert).editor, "insert")
	assert.Same(t, normal.editor, send(normal, ":").(viewEditorCommand).editor, "command")
	assert.Same(t, normal.editor, send(normal, "/").(viewEditorSearch).editor, "search")
	assert.Same(t, normal.editor, send(normal, "ctrl+p").(viewPalette).editor, "palette")
}
