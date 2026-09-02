package textarea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linesOf 는 buffer 의 줄들을 비교하기 쉽게 문자열로 바꾼다.
func linesOf(buf Viewport) []string {
	out := make([]string, len(buf.Lines))
	for i, Line := range buf.Lines {
		out[i] = string(Line)
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
		Cursor int
	}{
		{name: "줄 중간", data: "abc\n", at: 1, text: "X", want: "aXbc", Cursor: 2},
		{name: "줄 시작", data: "abc\n", at: 0, text: "X", want: "Xabc", Cursor: 1},
		{name: "줄 끝", data: "abc\n", at: 3, text: "X", want: "abcX", Cursor: 4},
		{name: "빈 줄", data: "\n", at: 0, text: "X", want: "X", Cursor: 1},
		{name: "여러 글자", data: "abc\n", at: 3, text: "def", want: "abcdef", Cursor: 6},
		{name: "한글", data: "abc\n", at: 0, text: "한", want: "한abc", Cursor: 3},
		{name: "ZWJ 이모지", data: "x\n", at: 1, text: "\U0001F468‍\U0001F469‍\U0001F466", want: "x\U0001F468‍\U0001F469‍\U0001F466", Cursor: 19},
		{name: "tab", data: "ab\n", at: 0, text: "\t", want: "\tab", Cursor: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("test.txt", []byte(test.data))
			buf.Cursor.Col = test.at

			buf.Insert([]byte(test.text))

			assert.Equal(t, test.want, string(buf.Lines[0]))
			assert.Equal(t, test.Cursor, buf.Cursor.Col)
		})
	}
}

// Enter 는 줄바꿈을 넣는 것이므로 insert 와 같은 경로다.
func TestInsertNewlineSplitsLine(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abcd\nnext\n"))
	buf.Cursor.Col = 2

	buf.Insert([]byte("\n"))

	assert.Equal(t, []string{"ab", "cd", "next"}, linesOf(buf))
	assert.Equal(t, 1, buf.Cursor.Line)
	assert.Equal(t, 0, buf.Cursor.Col)
}

func TestInsertNewlineAtLineEnd(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\n"))
	buf.Cursor.Col = 2

	buf.Insert([]byte("\n"))

	assert.Equal(t, []string{"ab", ""}, linesOf(buf))
	assert.Equal(t, 1, buf.Cursor.Line)
}

// 붙여넣기는 여러 줄일 수 있다. 같은 함수가 처리한다.
func TestInsertMultipleLines(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ad\n"))
	buf.Cursor.Col = 1

	buf.Insert([]byte("b\nmiddle\nc"))

	assert.Equal(t, []string{"ab", "middle", "cd"}, linesOf(buf))
	assert.Equal(t, 2, buf.Cursor.Line)
	assert.Equal(t, 1, buf.Cursor.Col, "마지막 조각 뒤")
}

func TestDeleteBackward(t *testing.T) {
	tests := []struct {
		name   string
		Line   string
		at     int
		want   string
		Cursor int
	}{
		{name: "ascii", Line: "abc", at: 2, want: "ac", Cursor: 1},
		{name: "줄 끝", Line: "abc", at: 3, want: "ab", Cursor: 2},
		{name: "한글은 3 byte 를 한 번에", Line: "한글", at: 6, want: "한", Cursor: 3},
		{name: "ZWJ 이모지는 18 byte 를 한 번에", Line: "x\U0001F468‍\U0001F469‍\U0001F466", at: 19, want: "x", Cursor: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("test.txt", []byte(test.Line+"\n"))
			buf.Cursor.Col = test.at

			buf.DeleteBackward()

			assert.Equal(t, test.want, string(buf.Lines[0]))
			assert.Equal(t, test.Cursor, buf.Cursor.Col)
		})
	}
}

func TestDeleteBackwardJoinsLines(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\ncd\n"))
	buf.Cursor.Line = 1
	buf.Cursor.Col = 0

	buf.DeleteBackward()

	assert.Equal(t, []string{"abcd"}, linesOf(buf))
	assert.Equal(t, 0, buf.Cursor.Line)
	assert.Equal(t, 2, buf.Cursor.Col, "합쳐진 지점")
}

func TestDeleteBackwardAtStartOfFileDoesNothing(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\n"))

	buf.DeleteBackward()

	assert.Equal(t, []string{"ab"}, linesOf(buf))
	assert.Equal(t, 0, buf.Cursor.Col)
	assert.Empty(t, buf.undo, "바뀐 것이 없으면 undo 기록도 없다")
}

// ADR-0001 의 핵심 불변식이다. 편집해도 backing buffer 는 그대로여야 한다.
func TestEditKeepsBackingBuffer(t *testing.T) {
	data := []byte("abc\ndef\n")
	buf := NewBuffer("test.txt", data)
	buf.Cursor.Col = 1

	buf.Insert([]byte("XYZ"))

	assert.Equal(t, "abc\ndef\n", string(data), "data 는 안 바뀐다")
	assert.Equal(t, "aXYZbc", string(buf.Lines[0]))
}

// 안 건드린 줄은 여전히 data 를 가리켜야 한다. 복사하면 큰 파일에서 메모리가 뛴다.
func TestEditKeepsUntouchedLinesAliased(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\ndef\n"))
	buf.Cursor.Col = 1
	buf.Insert([]byte("X"))

	buf.data[4] = 'D'
	assert.Equal(t, "Def", string(buf.Lines[1]), "안 건드린 줄이 data 를 안 가리킨다")
}

// 이어지는 타이핑은 u 한 번에 되돌아간다.
func TestUndoTypingRun(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\n"))
	buf.Cursor.Col = 3

	for _, c := range []string{"d", "e", "f"} {
		buf.Insert([]byte(c))
	}
	require.Equal(t, "abcdef", string(buf.Lines[0]))

	assert.True(t, buf.ApplyUndo())
	assert.Equal(t, "abc", string(buf.Lines[0]), "타이핑 구간 전체가 한 번에")
	assert.Equal(t, 3, buf.Cursor.Col, "커서가 구간 시작 자리로")

	assert.False(t, buf.ApplyUndo(), "더 되돌릴 것이 없다")
}

// 커서를 옮기면 구간이 끊긴다. vim 과 같다.
func TestCursorMoveBreaksUndoRun(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\n"))
	buf.Cursor.Col = 3

	buf.Insert([]byte("d"))
	buf.EndEdit() // 화살표 이동이 하는 일
	buf.Insert([]byte("e"))
	require.Equal(t, "abcde", string(buf.Lines[0]))

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, "abcd", string(buf.Lines[0]), "한 번에 하나씩")

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, "abc", string(buf.Lines[0]))
}

func TestUndoNewline(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abcd\n"))
	buf.Cursor.Col = 2

	buf.Insert([]byte("\n"))
	require.Equal(t, []string{"ab", "cd"}, linesOf(buf))

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, []string{"abcd"}, linesOf(buf))
	assert.Equal(t, 0, buf.Cursor.Line)
}

// 줄 합치기는 열린 구간의 범위 밖을 건드린다. 구간이 넓어져야 한 번에 되돌아간다.
func TestUndoLineJoinWithinRun(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\ncd\n"))
	buf.Cursor.Line = 1
	buf.Cursor.Col = 0

	buf.Insert([]byte("X")) // "ab", "Xcd"
	buf.Cursor.Col = 0      // 줄 시작으로
	buf.DeleteBackward()    // 앞 줄과 합침 → "abXcd"
	buf.Insert([]byte("Y")) // "abYXcd"
	require.Equal(t, []string{"abYXcd"}, linesOf(buf))

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, []string{"ab", "cd"}, linesOf(buf), "구간 전체가 한 번에 되돌아간다")
	assert.False(t, buf.ApplyUndo())
}

func TestRedo(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\n"))
	buf.Cursor.Col = 3

	buf.Insert([]byte("d"))
	require.True(t, buf.ApplyUndo())
	require.Equal(t, "abc", string(buf.Lines[0]))

	assert.True(t, buf.ApplyRedo())
	assert.Equal(t, "abcd", string(buf.Lines[0]))

	assert.False(t, buf.ApplyRedo(), "더 다시 적용할 것이 없다")
}

func TestRedoClearedByNewEdit(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\n"))
	buf.Cursor.Col = 3

	buf.Insert([]byte("d"))
	require.True(t, buf.ApplyUndo())
	require.NotEmpty(t, buf.redo)

	buf.Insert([]byte("Z"))

	assert.Empty(t, buf.redo, "새 편집이 앞날을 지운다")
	assert.False(t, buf.ApplyRedo())
}

// undo 기록은 옛 줄을 참조로만 담는다. 되돌린 내용이 원본 data 를 그대로 가리켜야 한다.
func TestUndoRestoresAliasedLine(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\ndef\n"))
	buf.Cursor.Col = 1

	buf.Insert([]byte("X"))
	require.True(t, buf.ApplyUndo())

	buf.data[0] = 'A'
	assert.Equal(t, "Abc", string(buf.Lines[0]), "되돌린 줄이 data 를 가리키지 않는다")
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
			Path := filepath.Join(t.TempDir(), "test.txt")
			require.NoError(t, os.WriteFile(Path, []byte(test.data), 0644))

			buf, err := OpenBuffer(Path)
			require.NoError(t, err)

			buf.Cursor.Col = 2
			buf.Insert([]byte("X"))
			require.NoError(t, saveBuffer(t, &buf))

			saved, err := os.ReadFile(Path)
			require.NoError(t, err)
			assert.Equal(t, test.want, string(saved))
		})
	}
}

// 줄이 늘어나도 저장이 맞아야 한다.
func TestSaveAfterNewline(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abcd\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Cursor.Col = 2
	buf.Insert([]byte("\n"))
	require.NoError(t, saveBuffer(t, &buf))

	saved, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "ab\ncd\n", string(saved))
}

// 화면보다 많은 줄을 넣어도 커서가 따라가고 화면이 스크롤되어야 한다.
func TestInsertManyLinesScrolls(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("start\n"))
	buf.Cursor.Col = 5

	buf.Insert([]byte(strings.Repeat("\nx", 20)))
	buf.ScrollTo(10)

	assert.Equal(t, 20, buf.Cursor.Line)
	assert.Positive(t, buf.Top.Line, "커서가 화면 아래로 나가면 스크롤된다")
}

// 줄 끝 공백 지우기는 사이에 안 바뀐 줄이 껴 있어도 한 번에 끝난다.
func TestTrimTrailingSpace(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a  \nb\nc\t\t\n"))

	count := buf.TrimTrailingSpace(0, len(buf.Lines))

	assert.Equal(t, 2, count)
	assert.Equal(t, "a", string(buf.Lines[0]))
	assert.Equal(t, "b", string(buf.Lines[1]), "안 바뀐 줄은 그대로")
	assert.Equal(t, "c", string(buf.Lines[2]), "줄 끝 tab 도 지운다")
}

// 여러 줄을 지워도 `u` 한 번에 전부 돌아온다. 이 기능의 핵심이다.
func TestTrimTrailingSpaceUndoesAsOne(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a  \nb\nc\t\t\n"))
	buf.TrimTrailingSpace(0, len(buf.Lines))

	require.True(t, buf.ApplyUndo())

	assert.Equal(t, []string{"a  ", "b", "c\t\t"}, linesOf(buf))
	assert.False(t, buf.ApplyUndo(), "되돌릴 것이 하나뿐이었다")
}

func TestTrimTrailingSpaceRedo(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a  \nb  \n"))
	buf.TrimTrailingSpace(0, len(buf.Lines))
	buf.ApplyUndo()

	require.True(t, buf.ApplyRedo())

	assert.Equal(t, []string{"a", "b"}, linesOf(buf))
}

// 지울 것이 없으면 아무 흔적도 남기지 않는다. dirty 가 서면 `[+]` 가 헛되이 붙는다.
func TestTrimTrailingSpaceNoop(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a\nb\n"))

	count := buf.TrimTrailingSpace(0, len(buf.Lines))

	assert.Equal(t, 0, count)
	assert.False(t, buf.Dirty)
	assert.Empty(t, buf.undo)
}

// 공백뿐인 줄은 빈 줄이 된다. vim 의 `:%s/\s\+$//e` 와 같다.
func TestTrimTrailingSpaceEmptiesBlankLine(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("   \n"))

	buf.TrimTrailingSpace(0, len(buf.Lines))

	assert.Equal(t, "", string(buf.Lines[0]))
}

// 커서가 잘려나간 자리에 있었으면 당겨지고, `u` 로 원래 칸에 돌아온다.
func TestTrimTrailingSpaceMovesCursor(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab    \n"))
	buf.Cursor.Col = 5

	buf.TrimTrailingSpace(0, len(buf.Lines))
	assert.Equal(t, 2, buf.Cursor.Col)

	buf.ApplyUndo()
	assert.Equal(t, 5, buf.Cursor.Col)
}

// 줄 가운데의 이어진 공백만 한 칸으로 줄인다.
func TestSqueezeSpaces(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a   b\tc\nd e\n\tf    g\n"))

	count := buf.SqueezeSpaces(0, len(buf.Lines))

	assert.Equal(t, 2, count)
	assert.Equal(t, "a b c", string(buf.Lines[0]), "space 든 tab 이든 빈 칸 하나")
	assert.Equal(t, "d e", string(buf.Lines[1]), "한 칸짜리는 그대로라 안 세어진다")
	assert.Equal(t, "\tf g", string(buf.Lines[2]), "들여쓰기 tab 은 살아 있다")
}

// **들여쓰기는 건드리지 않는다.** 줄이면 코드가 깨진다.
func TestSqueezeSpacesKeepsIndent(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("    if a   b:\n\t\treturn   1\n"))

	buf.SqueezeSpaces(0, len(buf.Lines))

	assert.Equal(t, "    if a b:", string(buf.Lines[0]), "space 네 칸 들여쓰기가 남는다")
	assert.Equal(t, "\t\treturn 1", string(buf.Lines[1]), "tab 두 개도 남는다")
}

// **줄 끝은 건드리지 않는다.** 그것은 「줄 끝 공백 지우기」의 몫이다.
func TestSqueezeSpacesKeepsTrailing(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a   b   \n"))

	buf.SqueezeSpaces(0, len(buf.Lines))

	assert.Equal(t, "a b   ", string(buf.Lines[0]))

	// 둘을 이어 쓰면 둘 다 사라진다. 한 명령이 두 가지를 하지 않는 대신이다.
	buf.TrimTrailingSpace(0, len(buf.Lines))
	assert.Equal(t, "a b", string(buf.Lines[0]))
}

// 공백뿐인 줄과 빈 줄은 줄일 가운데가 없다.
func TestSqueezeSpacesLeavesBlankLines(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("    \n\n"))

	count := buf.SqueezeSpaces(0, len(buf.Lines))

	assert.Equal(t, 0, count)
	assert.False(t, buf.Dirty, "흔적을 남기지 않는다")
	assert.Empty(t, buf.undo)
}

// 여러 줄을 줄여도 `u` 한 번에 전부 돌아온다.
func TestSqueezeSpacesUndoesAsOne(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a   b\nc\nd   e\n"))
	buf.SqueezeSpaces(0, len(buf.Lines))
	require.Equal(t, []string{"a b", "c", "d e"}, linesOf(buf))

	require.True(t, buf.ApplyUndo())

	assert.Equal(t, []string{"a   b", "c", "d   e"}, linesOf(buf))
	assert.False(t, buf.ApplyUndo(), "되돌릴 것이 하나뿐이었다")
}

// 줄을 오름차순으로 다시 늘어놓는다. byte 순이라 한글은 가나다 순이다.
func TestSortLines(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("다\n나\n가\n"))

	moved := buf.SortLines(0, len(buf.Lines))

	assert.Equal(t, 2, moved, "가운데 줄은 제자리라 안 세어진다")
	assert.Equal(t, []string{"가", "나", "다"}, linesOf(buf))
}

// 대문자가 소문자보다 앞이다. byte 순이라는 것이 그대로 드러나는 자리다.
func TestSortLinesIsByteOrder(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("b\nA\na\nB\n"))

	buf.SortLines(0, len(buf.Lines))

	assert.Equal(t, []string{"A", "B", "a", "b"}, linesOf(buf))
}

// 구간 밖은 건드리지 않는다. 팔레트가 visual 에서 고른 범위를 대는 자리다(ADR-0111).
func TestSortLinesInRange(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("c\nb\nd\na\n"))

	moved := buf.SortLines(0, 3)

	assert.Equal(t, 2, moved)
	assert.Equal(t, []string{"b", "c", "d", "a"}, linesOf(buf), "마지막 줄은 자리를 지킨다")
}

// 구간 안이 이미 정렬되어 있으면 밖이 어떻든 아무 흔적도 남기지 않는다.
func TestSortLinesInRangeNoop(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a\nb\nz\nc\n"))

	moved := buf.SortLines(0, 3)

	assert.Equal(t, 0, moved)
	assert.False(t, buf.Dirty)
	assert.Empty(t, buf.undo)
}

// 줄 끝 공백과 중복 공백도 구간 밖을 두고 간다.
func TestTrimAndSqueezeInRange(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a  \nb  \n"))
	require.Equal(t, 1, buf.TrimTrailingSpace(0, 1))
	assert.Equal(t, []string{"a", "b  "}, linesOf(buf))

	other := NewBuffer("test.txt", []byte("a  b\nc  d\n"))
	require.Equal(t, 1, other.SqueezeSpaces(1, 2))
	assert.Equal(t, []string{"a  b", "c d"}, linesOf(other))
}

// 이미 정렬되어 있으면 아무 흔적도 남기지 않는다.
func TestSortLinesNoop(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a\nb\nc\n"))

	moved := buf.SortLines(0, len(buf.Lines))

	assert.Equal(t, 0, moved)
	assert.False(t, buf.Dirty)
	assert.Empty(t, buf.undo)
}

// 정렬은 `u` 한 번에 통째로 돌아간다. 줄이 자리를 바꾸는 일이라 구간이 파일 전체다.
func TestSortLinesUndoesAsOne(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("c\na\nb\n"))
	buf.SortLines(0, len(buf.Lines))
	require.Equal(t, []string{"a", "b", "c"}, linesOf(buf))

	require.True(t, buf.ApplyUndo())

	assert.Equal(t, []string{"c", "a", "b"}, linesOf(buf))
	assert.False(t, buf.ApplyUndo())
}

// 두 번 돌려도 결과가 같다. 안정 정렬이라 같은 줄끼리 자리를 바꾸지 않는다.
func TestSortLinesIsStable(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("b\na\nb\na\n"))

	require.NotZero(t, buf.SortLines(0, len(buf.Lines)))
	require.Equal(t, []string{"a", "a", "b", "b"}, linesOf(buf))

	assert.Equal(t, 0, buf.SortLines(0, len(buf.Lines)), "두 번째는 바꿀 것이 없다")
}

// 앞의 타이핑 구간과 섞이지 않는다. 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
func TestTrimTrailingSpaceDoesNotJoinOpenEdit(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a  \n"))
	buf.Insert([]byte("X"))
	require.Equal(t, "Xa  ", string(buf.Lines[0]))

	buf.TrimTrailingSpace(0, len(buf.Lines))
	require.Equal(t, "Xa", string(buf.Lines[0]))

	buf.ApplyUndo()
	assert.Equal(t, "Xa  ", string(buf.Lines[0]), "공백만 돌아온다")

	buf.ApplyUndo()
	assert.Equal(t, "a  ", string(buf.Lines[0]), "타이핑은 그 다음이다")
}
