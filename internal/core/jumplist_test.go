package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"

	"github.com/bluemir/zn/internal/textarea"
)

// jumpEditor 는 파일 셋이 열린 편집기다. 파일을 넘는 이력을 재는 데 쓴다.
func jumpEditor(t *testing.T) (*editor, []string) {
	t.Helper()

	dir := t.TempDir()

	paths := []string{}
	buffers := []textarea.Viewport{}
	for _, name := range []string{"first.go", "second.go", "third.go"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path,
			[]byte("package main\n\nfunc a() {}\n\nfunc b() {}\n\nfunc c() {}\n"), 0644))

		buf, err := textarea.OpenBuffer(path)
		require.NoError(t, err)

		paths = append(paths, path)
		buffers = append(buffers, buf)
	}

	return &editor{
		boxChars: boxUnicode,
		buffers:  buffers,
		width:    80,
		height:   20,
	}, paths
}

// at 은 지금 활성 파일과 커서 줄이다. 시험이 자리를 견주는 방법이다.
func at(e *editor) (string, int) {
	return e.activeBuffer().Path, e.activeBuffer().Cursor.Line
}

// 뛰기 전 자리를 담고, `ctrl+o` 가 그리로 되돌아간다.
func TestJumpBackReturnsToRecordedPlace(t *testing.T) {
	e, paths := jumpEditor(t)

	// first.go 의 2 번째 줄에서 second.go 로 뛴다.
	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})

	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))

	path, line := at(e)
	require.Equal(t, paths[1], path)
	require.Equal(t, 4, line)

	// 되돌아간다.
	e.jumpBack()

	path, line = at(e)
	assert.Equal(t, paths[0], path, "파일을 넘어 되돌아간다")
	assert.Equal(t, 2, line)
}

// `ctrl+i` 는 되돌아온 것을 앞으로 되짚는다.
func TestJumpForwardRetracesBack(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))

	e.jumpBack()
	path, line := at(e)
	require.Equal(t, paths[0], path)
	require.Equal(t, 2, line)

	e.jumpForward()

	path, line = at(e)
	assert.Equal(t, paths[1], path, "뛰었던 자리로 다시 간다")
	assert.Equal(t, 4, line)
}

// 양끝에서는 알리고 멈춘다. 커서는 그대로다.
func TestJumpAtBothEnds(t *testing.T) {
	e, _ := jumpEditor(t)

	assert.Nil(t, e.jumpBack())
	assert.Equal(t, "되돌아갈 자리가 없습니다", e.notice)

	assert.Nil(t, e.jumpForward())
	assert.Equal(t, "앞으로 갈 자리가 없습니다", e.notice)
}

// **앞쪽을 버린다.** 되짚어 들어와 있다가 새로 뛰면 `ctrl+i` 로 갈 자리가 사라진다 —
// 브라우저의 뒤로/앞으로와 같은 모양이다(ADR-0070).
func TestNewJumpDropsForwardHistory(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))

	e.jumpBack() // first.go:2 로 돌아왔다. 앞으로 갈 자리(second.go:4) 가 남아 있다.
	require.Equal(t, 2, len(e.jumps.places))

	// 여기서 새로 뛴다.
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[2], 6))

	// 앞쪽이 사라졌다.
	assert.Nil(t, e.jumpForward())
	assert.Equal(t, "앞으로 갈 자리가 없습니다", e.notice)

	// 뒤로는 그대로 간다.
	e.jumpBack()
	path, line := at(e)
	assert.Equal(t, paths[0], path)
	assert.Equal(t, 2, line)
}

// 같은 줄이 잇달아 오면 새로 담지 않는다. `n` 을 몇 번 치는 사이에 이력이 그 줄로 채워지면 안 된다.
func TestRecordJumpSkipsSameLine(t *testing.T) {
	e, _ := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})

	e.recordJump()
	e.recordJump()
	e.recordJump()

	assert.Len(t, e.jumps.places, 1)

	// 줄이 달라지면 담는다.
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 4})
	e.recordJump()

	assert.Len(t, e.jumps.places, 2)
}

// 이름 없는 buffer 는 담지 않는다. 되돌아갈 때 열 파일이 없다.
func TestRecordJumpSkipsUnnamedBuffer(t *testing.T) {
	e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("")}, width: 80, height: 20}

	e.recordJump()

	assert.Empty(t, e.jumps.places)
}

// 백 개가 넘으면 오래된 것부터 버린다.
func TestJumpListDropsOldest(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	for i := range jumpListMax + 20 {
		// 줄을 번갈아 바꾸어 같은 줄 거르기에 걸리지 않게 한다.
		e.activeBuffer().MoveTo(scheme.Cursor{Line: i % 6})
		e.recordJump()
	}

	assert.Len(t, e.jumps.places, jumpListMax)
	assert.Equal(t, paths[0], e.jumps.places[0].path)
}

// 되짚는 이동은 이력에 담기지 않는다. 담기면 되돌아갈 수가 없다.
func TestGoToPlaceDoesNotRecord(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))

	before := len(e.jumps.places)

	e.jumpBack()
	e.jumpForward()
	e.jumpBack()

	// `ctrl+o` 가 처음에 지금 자리를 담은 하나만 늘었다.
	assert.Equal(t, before+1, len(e.jumps.places))
}

// gotoFile 은 시험이 「뛰었다」를 흉내내는 것이다. 이력에는 손대지 않는다.
func gotoFile(e *editor, path string, line int) error {
	if _, err := e.openTab(path); err != nil {
		return err
	}

	e.activeBuffer().MoveTo(scheme.Cursor{Line: line})

	return nil
}

// 검색이 뛰면 이력에 담긴다. `/` `?` `n` `N` `*` `#` 이 모두 jumpToMatch 를 지난다(ADR-0070).
func TestSearchRecordsJump(t *testing.T) {
	m := newTestEditorFile("main.go",
		"package main\n\nfunc alpha() {}\n\nfunc beta() {}\n\nfunc alpha2() {}\n", 80, 20)
	e := m.editor

	// `/alpha` 를 쳐서 찾는다.
	next := send(m, "/", "a", "l", "p", "h", "a", "enter")
	require.IsType(t, viewEditorNormal{}, next)

	require.Len(t, e.jumps.places, 1, "뛰기 전 자리가 담긴다")
	assert.Equal(t, 0, e.jumps.places[0].line)
	assert.Equal(t, 2, e.activeBuffer().Cursor.Line, "alpha 를 찾았다")

	// `n` 으로 다음 것을 찾으면 또 담긴다.
	send(next, "n")

	require.Len(t, e.jumps.places, 2)
	assert.Equal(t, 2, e.jumps.places[1].line, "직전에 서 있던 줄이다")
	assert.Equal(t, 6, e.activeBuffer().Cursor.Line)

	// `ctrl+o` 로 처음 자리까지 되짚어 간다.
	e.jumpBack()
	assert.Equal(t, 2, e.activeBuffer().Cursor.Line)

	e.jumpBack()
	assert.Equal(t, 0, e.activeBuffer().Cursor.Line)
}

// 못 찾으면 담지 않는다. 커서가 그대로라 담을 것도 없다.
func TestFailedSearchDoesNotRecord(t *testing.T) {
	m := newTestEditorFile("main.go", "package main\n", 80, 20)
	e := m.editor

	send(m, "/", "z", "z", "z", "enter")

	assert.Empty(t, e.jumps.places)
	assert.Equal(t, "찾을 수 없음: zzz", e.notice)
}

// jumpText 는 스무 줄짜리 본문이다. `G`·`gg` 가 멀리 뛰는 것을 보려면 화면보다 길어야 한다.
func jumpText() string {
	lines := []string{}
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}

	return strings.Join(lines, "\n") + "\n"
}

// `G` 와 `gg` 가 이력에 담기고 `ctrl+o` 가 뛰기 전 줄로 되돌아간다(ADR-0082).
func TestGotoLineMotionsRecordJump(t *testing.T) {
	m := newTestEditorFile("main.txt", jumpText(), 80, 10)
	e := m.editor

	// 5 번째 줄에서 `G` 로 끝까지 뛴다.
	next := send(m, "5", "j")
	require.Equal(t, 5, e.activeBuffer().Cursor.Line)

	next = send(next, "G")
	require.Equal(t, 19, e.activeBuffer().Cursor.Line, "마지막 줄이다")
	require.Len(t, e.jumps.places, 1)
	assert.Equal(t, 5, e.jumps.places[0].line, "뛰기 전 자리가 담긴다")

	// `gg` 로 첫 줄까지 뛰면 또 담긴다.
	next = send(next, "g", "g")
	require.Equal(t, 0, e.activeBuffer().Cursor.Line)
	require.Len(t, e.jumps.places, 2)
	assert.Equal(t, 19, e.jumps.places[1].line)

	// `ctrl+o` 두 번이면 처음 자리다.
	next = send(next, "ctrl+o")
	assert.Equal(t, 19, e.activeBuffer().Cursor.Line)

	send(next, "ctrl+o")
	assert.Equal(t, 5, e.activeBuffer().Cursor.Line)
}

// 줄 번호를 준 `10gg`·`3G` 도 담는다. 숫자가 붙어도 멀리 뛰는 것은 같다.
func TestGotoLineWithCountRecordsJump(t *testing.T) {
	m := newTestEditorFile("main.txt", jumpText(), 80, 10)
	e := m.editor

	next := send(m, "1", "0", "g", "g")
	require.Equal(t, 9, e.activeBuffer().Cursor.Line, "10 번째 줄이다")
	require.Len(t, e.jumps.places, 1)
	assert.Equal(t, 0, e.jumps.places[0].line)

	send(next, "3", "G")
	require.Equal(t, 2, e.activeBuffer().Cursor.Line)
	require.Len(t, e.jumps.places, 2)
	assert.Equal(t, 9, e.jumps.places[1].line)
}

// 아무 데도 가지 않았으면 담지 않는다. 파일 끝에서 `G` 를 또 치는 자리다 —
// 담으면 `ctrl+o` 한 번이 제자리걸음이 된다.
func TestGotoLineInPlaceDoesNotRecord(t *testing.T) {
	m := newTestEditorFile("main.txt", jumpText(), 80, 10)
	e := m.editor

	next := send(m, "G")
	require.Len(t, e.jumps.places, 1)

	next = send(next, "G")
	assert.Len(t, e.jumps.places, 1, "이미 마지막 줄이라 담을 것이 없다")

	// 이력이 하나뿐이라 `ctrl+o` 한 번이 처음 자리로 간다.
	send(next, "ctrl+o")
	assert.Equal(t, 0, e.activeBuffer().Cursor.Line)
}

// 고르는 중의 `G` 는 담지 않는다. 뛰는 것이 아니라 범위를 늘리는 것이다.
func TestVisualGotoLineDoesNotRecordJump(t *testing.T) {
	m := newTestEditorFile("main.txt", jumpText(), 80, 10)
	e := m.editor

	next := send(m, "5", "j", "v", "G")
	require.IsType(t, viewEditorVisual{}, next)
	require.Equal(t, 19, e.activeBuffer().Cursor.Line, "범위가 끝까지 늘었다")

	assert.Empty(t, e.jumps.places)
}

// `:5` 도 담는다. 줄 번호로 뛰는 것은 `gg`·`G` 와 같은 일이다.
func TestGoToLineCommandRecordsJump(t *testing.T) {
	m := newTestEditorFile("main.txt", jumpText(), 80, 10)
	e := m.editor

	next := send(m, "G")
	require.Equal(t, 19, e.activeBuffer().Cursor.Line)

	next = send(next, ":", "5", "enter")
	require.IsType(t, viewEditorNormal{}, next)
	require.Equal(t, 4, e.activeBuffer().Cursor.Line, "5 번째 줄이다")

	require.Len(t, e.jumps.places, 2)
	assert.Equal(t, 19, e.jumps.places[1].line)

	send(next, "ctrl+o")
	assert.Equal(t, 19, e.activeBuffer().Cursor.Line)
}

// 떠난 자리와 닿은 자리가 둘 다 방문 기록에 남는다. 최근이 위다(ADR-0074).
func TestGotoLineRecordsVisits(t *testing.T) {
	m := newTestEditorFile("main.txt", jumpText(), 80, 10)
	e := m.editor

	send(m, "5", "j", "G")

	require.Len(t, e.logs.places, 2)
	assert.Equal(t, 19, e.logs.places[0].line, "닿은 자리가 맨 앞이다")
	assert.Equal(t, 5, e.logs.places[1].line, "떠난 자리다")
}
