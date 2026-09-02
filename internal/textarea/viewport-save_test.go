package textarea

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 쓰기 직전에 포매터를 통과시키는 자리를 보는 시험이다(viewport-save.go).
//
// **셸을 부르지 않는다.** 창이 아는 것은 「byte 를 넣으면 byte 가 나온다」뿐이라(SaveFormat)
// 그 자리에 Go 함수를 끼우면 된다. 명령을 찾고 돌리는 것은 core 의 일이고 그쪽 시험이 본다
// (ADR-0129).

// pipe 는 글을 통과시키는 포매터다.
func pipe(f func(in string) string) *SaveFormat {
	return &SaveFormat{Name: "시험", Run: func(in []byte) ([]byte, error) {
		return []byte(f(string(in))), nil
	}}
}

// failing 은 늘 실패하는 포매터다.
func failing(msg string) *SaveFormat {
	return &SaveFormat{Name: "시험", Run: func([]byte) ([]byte, error) {
		return nil, errors.New(msg)
	}}
}

// 쓰기 직전에 포매터를 통과시키는 자리를 보는 시험이다(viewport-save.go).

func TestSaveHookReplacesBuffer(t *testing.T) {
	buf := NewBuffer("a.go", []byte("가나\nabc\n"))

	note := buf.applySaveHook(pipe(strings.ToUpper))

	assert.Equal(t, "가나\nABC", strings.Join(linesOf(buf), "\n"))
	assert.Equal(t, "시험: 1 줄 맞춤", note)
	assert.True(t, buf.Dirty)
}

// 줄 수가 달라지면 얼마나 늘고 줄었는지도 적는다.
func TestSaveHookNoteCountsLines(t *testing.T) {
	grown := NewBuffer("a.go", []byte("한 줄\n"))
	assert.Equal(t, "시험: 1 줄 맞춤, 1 줄 늘어남", grown.applySaveHook(pipe(func(in string) string { return in + "뒤에\n" })))

	shrunk := NewBuffer("a.go", []byte("첫 줄\n둘째 줄\n"))
	assert.Equal(t, "시험: 1 줄 맞춤, 1 줄 줄어듦", shrunk.applySaveHook(pipe(func(in string) string {
		first, _, _ := strings.Cut(in, "\n")

		return first + "\n"
	})))
}

// 바뀐 것이 없으면 아무 말도 하지 않고 buffer 도 건드리지 않는다.
func TestSaveHookQuietWhenNothingChanged(t *testing.T) {
	buf := NewBuffer("a.go", []byte("그대로\n"))

	assert.Empty(t, buf.applySaveHook(pipe(func(in string) string { return in })))
	assert.False(t, buf.Dirty, "dirty 가 서면 저장할 것이 없는데 있다고 보인다")
	assert.Empty(t, buf.undo, "되돌릴 것도 생기지 않는다")
}

// 포매터가 고친 것은 한 동작이라 `u` 한 번에 통째로 돌아간다.
func TestSaveHookUndoesInOneStep(t *testing.T) {
	buf := NewBuffer("a.go", []byte("첫 줄\n둘째\n셋째\n"))

	buf.applySaveHook(pipe(func(in string) string { return strings.ReplaceAll(in, "줄", "칸") }))
	require.NotEqual(t, "첫 줄", string(buf.Lines[0]))

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, "첫 줄\n둘째\n셋째", strings.Join(linesOf(buf), "\n"))
}

// 앞의 타이핑과 한 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
func TestSaveHookUndoDoesNotSwallowTyping(t *testing.T) {
	buf := NewBuffer("a.go", []byte("abc\n"))
	buf.Insert([]byte("XY"))

	buf.applySaveHook(pipe(strings.ToUpper))

	require.True(t, buf.ApplyUndo(), "포매터가 한 것부터 돌아온다")
	assert.Equal(t, "XYabc", string(buf.Lines[0]))

	require.True(t, buf.ApplyUndo(), "그 앞의 타이핑은 따로 남아 있다")
	assert.Equal(t, "abc", string(buf.Lines[0]))
}

// 파일이 짧아져도 커서는 범위 안에 남는다.
func TestSaveHookKeepsCursorInRange(t *testing.T) {
	buf := NewBuffer("a.go", []byte("첫 줄\n둘째 줄\n셋째 줄\n"))
	buf.Cursor.Line, buf.Cursor.Col = 2, 6

	buf.applySaveHook(pipe(func(in string) string {
		first, _, _ := strings.Cut(in, "\n")

		return first + "\n"
	}))

	assert.Equal(t, 0, buf.Cursor.Line)
	assert.LessOrEqual(t, buf.Cursor.Col, len(buf.Lines[0]))
}

// 실패하면 buffer 를 건드리지 않는다. 저장은 그대로 가고 까닭만 아래 줄에 뜬다.
func TestSaveHookFailureKeepsBuffer(t *testing.T) {
	buf := NewBuffer("a.go", []byte("고치다 만 글\n"))

	note := buf.applySaveHook(failing("<standard input>:1:1: expected declaration"))

	assert.Equal(t, "시험: <standard input>:1:1: expected declaration", note)
	assert.Equal(t, "고치다 만 글", string(buf.Lines[0]))
	assert.False(t, buf.Dirty)
}

// 읽기 전용 파일은 손대지 않는다. 쓰기가 어차피 실패하는데 buffer 만 바뀌면 되돌릴 길이 없다.
func TestSaveHookSkipsReadOnly(t *testing.T) {
	buf := NewBuffer("a.go", []byte("abc\n"))
	buf.ReadOnly = true

	assert.Empty(t, buf.applySaveHook(pipe(strings.ToUpper)))
	assert.Equal(t, "abc", string(buf.Lines[0]))
}

// 마지막 줄바꿈은 포매터가 정하지 않는다. 그것은 `.editorconfig` 의 몫이다(ADR-0052).
func TestSaveHookKeepsFinalNewlineFact(t *testing.T) {
	buf := NewBuffer("a.go", []byte("abc"))
	require.False(t, buf.finalLineEnding)

	buf.applySaveHook(pipe(strings.ToUpper))

	assert.Equal(t, "ABC", string(buf.Lines[0]))
	assert.False(t, buf.finalLineEnding, "포매터가 붙인 줄바꿈이 사실을 뒤집지 않는다")
}
