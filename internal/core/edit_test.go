package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linesOf 는 buffer 의 줄들을 비교하기 쉽게 문자열로 바꾼다.
func linesOf(buf Buffer) []string {
	out := make([]string, len(buf.lines))
	for i, line := range buf.lines {
		out[i] = string(line)
	}
	return out
}

func TestInsertText(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		at     int // 커서 byte offset
		text   string
		want   string
		cursor int
	}{
		{name: "줄 중간", data: "abc\n", at: 1, text: "X", want: "aXbc", cursor: 2},
		{name: "줄 시작", data: "abc\n", at: 0, text: "X", want: "Xabc", cursor: 1},
		{name: "줄 끝", data: "abc\n", at: 3, text: "X", want: "abcX", cursor: 4},
		{name: "빈 줄", data: "\n", at: 0, text: "X", want: "X", cursor: 1},
		{name: "여러 글자", data: "abc\n", at: 3, text: "def", want: "abcdef", cursor: 6},
		{name: "한글", data: "abc\n", at: 0, text: "한", want: "한abc", cursor: 3},
		{name: "ZWJ 이모지", data: "x\n", at: 1, text: "\U0001F468‍\U0001F469‍\U0001F466", want: "x\U0001F468‍\U0001F469‍\U0001F466", cursor: 19},
		{name: "tab", data: "ab\n", at: 0, text: "\t", want: "\tab", cursor: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.data))
			buf.cursorCol = test.at

			buf.insert([]byte(test.text), wide)

			assert.Equal(t, test.want, string(buf.lines[0]))
			assert.Equal(t, test.cursor, buf.cursorCol)
		})
	}
}

// Enter 는 줄바꿈을 넣는 것이므로 insert 와 같은 경로다.
func TestInsertNewlineSplitsLine(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abcd\nnext\n"))
	buf.cursorCol = 2

	buf.insert([]byte("\n"), wide)

	assert.Equal(t, []string{"ab", "cd", "next"}, linesOf(buf))
	assert.Equal(t, 1, buf.cursorLine)
	assert.Equal(t, 0, buf.cursorCol)
}

func TestInsertNewlineAtLineEnd(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\n"))
	buf.cursorCol = 2

	buf.insert([]byte("\n"), wide)

	assert.Equal(t, []string{"ab", ""}, linesOf(buf))
	assert.Equal(t, 1, buf.cursorLine)
}

// 붙여넣기는 여러 줄일 수 있다. 같은 함수가 처리한다.
func TestInsertMultipleLines(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ad\n"))
	buf.cursorCol = 1

	buf.insert([]byte("b\nmiddle\nc"), wide)

	assert.Equal(t, []string{"ab", "middle", "cd"}, linesOf(buf))
	assert.Equal(t, 2, buf.cursorLine)
	assert.Equal(t, 1, buf.cursorCol, "마지막 조각 뒤")
}

func TestDeleteBackward(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		at     int
		want   string
		cursor int
	}{
		{name: "ascii", line: "abc", at: 2, want: "ac", cursor: 1},
		{name: "줄 끝", line: "abc", at: 3, want: "ab", cursor: 2},
		{name: "한글은 3 byte 를 한 번에", line: "한글", at: 6, want: "한", cursor: 3},
		{name: "ZWJ 이모지는 18 byte 를 한 번에", line: "x\U0001F468‍\U0001F469‍\U0001F466", at: 19, want: "x", cursor: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.line+"\n"))
			buf.cursorCol = test.at

			buf.deleteBackward(wide)

			assert.Equal(t, test.want, string(buf.lines[0]))
			assert.Equal(t, test.cursor, buf.cursorCol)
		})
	}
}

func TestDeleteBackwardJoinsLines(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\ncd\n"))
	buf.cursorLine = 1
	buf.cursorCol = 0

	buf.deleteBackward(wide)

	assert.Equal(t, []string{"abcd"}, linesOf(buf))
	assert.Equal(t, 0, buf.cursorLine)
	assert.Equal(t, 2, buf.cursorCol, "합쳐진 지점")
}

func TestDeleteBackwardAtStartOfFileDoesNothing(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\n"))

	buf.deleteBackward(wide)

	assert.Equal(t, []string{"ab"}, linesOf(buf))
	assert.Equal(t, 0, buf.cursorCol)
	assert.Empty(t, buf.undo, "바뀐 것이 없으면 undo 기록도 없다")
}

// ADR-0001 의 핵심 불변식이다. 편집해도 backing buffer 는 그대로여야 한다.
func TestEditKeepsBackingBuffer(t *testing.T) {
	data := []byte("abc\ndef\n")
	buf := newBuffer("test.txt", data)
	buf.cursorCol = 1

	buf.insert([]byte("XYZ"), wide)

	assert.Equal(t, "abc\ndef\n", string(data), "data 는 안 바뀐다")
	assert.Equal(t, "aXYZbc", string(buf.lines[0]))
}

// 안 건드린 줄은 여전히 data 를 가리켜야 한다. 복사하면 큰 파일에서 메모리가 뛴다.
func TestEditKeepsUntouchedLinesAliased(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\ndef\n"))
	buf.cursorCol = 1
	buf.insert([]byte("X"), wide)

	buf.data[4] = 'D'
	assert.Equal(t, "Def", string(buf.lines[1]), "안 건드린 줄이 data 를 안 가리킨다")
}

// 이어지는 타이핑은 u 한 번에 되돌아간다.
func TestUndoTypingRun(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\n"))
	buf.cursorCol = 3

	for _, c := range []string{"d", "e", "f"} {
		buf.insert([]byte(c), wide)
	}
	require.Equal(t, "abcdef", string(buf.lines[0]))

	assert.True(t, buf.applyUndo(wide))
	assert.Equal(t, "abc", string(buf.lines[0]), "타이핑 구간 전체가 한 번에")
	assert.Equal(t, 3, buf.cursorCol, "커서가 구간 시작 자리로")

	assert.False(t, buf.applyUndo(wide), "더 되돌릴 것이 없다")
}

// 커서를 옮기면 구간이 끊긴다. vim 과 같다.
func TestCursorMoveBreaksUndoRun(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\n"))
	buf.cursorCol = 3

	buf.insert([]byte("d"), wide)
	buf.endEdit() // 화살표 이동이 하는 일
	buf.insert([]byte("e"), wide)
	require.Equal(t, "abcde", string(buf.lines[0]))

	require.True(t, buf.applyUndo(wide))
	assert.Equal(t, "abcd", string(buf.lines[0]), "한 번에 하나씩")

	require.True(t, buf.applyUndo(wide))
	assert.Equal(t, "abc", string(buf.lines[0]))
}

func TestUndoNewline(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abcd\n"))
	buf.cursorCol = 2

	buf.insert([]byte("\n"), wide)
	require.Equal(t, []string{"ab", "cd"}, linesOf(buf))

	require.True(t, buf.applyUndo(wide))
	assert.Equal(t, []string{"abcd"}, linesOf(buf))
	assert.Equal(t, 0, buf.cursorLine)
}

// 줄 합치기는 열린 구간의 범위 밖을 건드린다. 구간이 넓어져야 한 번에 되돌아간다.
func TestUndoLineJoinWithinRun(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\ncd\n"))
	buf.cursorLine = 1
	buf.cursorCol = 0

	buf.insert([]byte("X"), wide) // "ab", "Xcd"
	buf.cursorCol = 0             // 줄 시작으로
	buf.deleteBackward(wide)      // 앞 줄과 합침 → "abXcd"
	buf.insert([]byte("Y"), wide) // "abYXcd"
	require.Equal(t, []string{"abYXcd"}, linesOf(buf))

	require.True(t, buf.applyUndo(wide))
	assert.Equal(t, []string{"ab", "cd"}, linesOf(buf), "구간 전체가 한 번에 되돌아간다")
	assert.False(t, buf.applyUndo(wide))
}

func TestRedo(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\n"))
	buf.cursorCol = 3

	buf.insert([]byte("d"), wide)
	require.True(t, buf.applyUndo(wide))
	require.Equal(t, "abc", string(buf.lines[0]))

	assert.True(t, buf.applyRedo(wide))
	assert.Equal(t, "abcd", string(buf.lines[0]))

	assert.False(t, buf.applyRedo(wide), "더 다시 적용할 것이 없다")
}

func TestRedoClearedByNewEdit(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\n"))
	buf.cursorCol = 3

	buf.insert([]byte("d"), wide)
	require.True(t, buf.applyUndo(wide))
	require.NotEmpty(t, buf.redo)

	buf.insert([]byte("Z"), wide)

	assert.Empty(t, buf.redo, "새 편집이 앞날을 지운다")
	assert.False(t, buf.applyRedo(wide))
}

// undo 기록은 옛 줄을 참조로만 담는다. 되돌린 내용이 원본 data 를 그대로 가리켜야 한다.
func TestUndoRestoresAliasedLine(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\ndef\n"))
	buf.cursorCol = 1

	buf.insert([]byte("X"), wide)
	require.True(t, buf.applyUndo(wide))

	buf.data[0] = 'A'
	assert.Equal(t, "Abc", string(buf.lines[0]), "되돌린 줄이 data 를 가리키지 않는다")
}

// 편집한 뒤 저장해도 줄끝 형식과 파일 끝 줄끝이 유지되어야 한다.
func TestSaveAfterEdit(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "LF", data: "ab\ncd\n", want: "abX\ncd\n"},
		{name: "CRLF", data: "ab\r\ncd\r\n", want: "abX\r\ncd\r\n"},
		{name: "줄끝 없이 끝남", data: "ab\ncd", want: "abX\ncd"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.txt")
			require.NoError(t, os.WriteFile(path, []byte(test.data), 0644))

			buf, err := OpenBuffer(path)
			require.NoError(t, err)

			buf.cursorCol = 2
			buf.insert([]byte("X"), wide)
			require.NoError(t, buf.Save())

			saved, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, test.want, string(saved))
		})
	}
}

// 줄이 늘어나도 저장이 맞아야 한다.
func TestSaveAfterNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abcd\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.cursorCol = 2
	buf.insert([]byte("\n"), wide)
	require.NoError(t, buf.Save())

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "ab\ncd\n", string(saved))
}

// 화면보다 많은 줄을 넣어도 커서가 따라가고 화면이 스크롤되어야 한다.
func TestInsertManyLinesScrolls(t *testing.T) {
	buf := newBuffer("test.txt", []byte("start\n"))
	buf.cursorCol = 5

	buf.insert([]byte(strings.Repeat("\nx", 20)), wide)
	buf.scrollTo(wide, 10)

	assert.Equal(t, 20, buf.cursorLine)
	assert.Positive(t, buf.top, "커서가 화면 아래로 나가면 스크롤된다")
}

// --- 키를 통한 경로 ---

func TestTypingThroughKeys(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "Y")

	assert.Equal(t, "XYabc", string(bufferOf(t, m).lines[0]))
	assert.Equal(t, 2, cursorColOf(t, m))
}

func TestNormalModeDoesNotType(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "X")

	assert.Equal(t, "abc", string(bufferOf(t, m).lines[0]), "normal 에서는 글자가 안 들어간다")
}

func TestEnterKeySplitsLine(t *testing.T) {
	var m tea.Model = newTestEditor("abcd\n", 40, 5)

	m = send(m, "right", "right", "i", "enter")

	assert.Equal(t, []string{"ab", "cd"}, linesOf(bufferOf(t, m)))
}

func TestBackspaceKey(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "a", "backspace")

	assert.Equal(t, "bc", string(bufferOf(t, m).lines[0]))
}

func TestTabKeyInsertsTab(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "i", "tab")

	assert.Equal(t, "\tab", string(bufferOf(t, m).lines[0]))
}

// modifier 조합은 Text 가 비어 있어서 글자로 들어가지 않는다.
func TestCtrlKeyDoesNotType(t *testing.T) {
	m := newTestEditor("abc\n", 40, 5)
	insert, _ := m.Update(key("i"))

	next, _ := insert.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	assert.Equal(t, "abc", string(bufferOf(t, next).lines[0]))
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
	require.Equal(t, "XYabc", string(bufferOf(t, m).lines[0]))

	m = send(m, "u")
	assert.Equal(t, "abc", string(bufferOf(t, m).lines[0]), "타이핑 구간 전체가 한 번에")

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	assert.Equal(t, "XYabc", string(bufferOf(t, updated).lines[0]))
}

// insert mode 에서 화살표로 움직이면 undo 구간이 끊긴다.
func TestArrowInInsertBreaksUndoRun(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "right", "Y", "esc")
	require.Equal(t, "XaYbc", string(bufferOf(t, m).lines[0]))

	m = send(m, "u")
	assert.Equal(t, "Xabc", string(bufferOf(t, m).lines[0]), "한 번에 하나씩")

	m = send(m, "u")
	assert.Equal(t, "abc", string(bufferOf(t, m).lines[0]))
}

// undo 뒤 커서가 줄 끝을 넘지 않아야 한다. normal 커서는 글자 위에 있다.
func TestUndoClampsCursorInNormalMode(t *testing.T) {
	var m tea.Model = newTestEditor("ab\n", 40, 5)

	m = send(m, "right", "a", "X", "Y", "esc", "u")

	buf := bufferOf(t, m)
	assert.Equal(t, "ab", string(buf.lines[0]))
	assert.Less(t, buf.cursorCol, len(buf.lines[0])+1)
}
