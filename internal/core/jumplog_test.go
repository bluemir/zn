package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"

	"github.com/bluemir/zn/internal/textarea"
)

// logLines 는 기록을 「경로:줄」로 편 것이다. 견주기 쉬우라고 둔다.
func logLines(e *editor) []int {
	lines := make([]int, 0, len(e.logs.places))
	for _, place := range e.logs.places {
		lines = append(lines, place.line)
	}

	return lines
}

// **떠난 자리와 닿은 자리가 둘 다 남는다.** A 에서 B, B 에서 C 로 뛰면 A·B·C 다.
// 발자취가 끊기지 않는 것이 이 기록의 값이다(ADR-0074).
func TestJumpLogKeepsBothEnds(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})

	// A(first.go:2) 에서 B(second.go:4) 로
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))
	e.arrive()

	// B 에서 C(third.go:6) 로
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[2], 6))
	e.arrive()

	// 최근이 앞이다.
	require.Len(t, e.logs.places, 3)
	assert.Equal(t, []int{6, 4, 2}, logLines(e))
	assert.Equal(t, paths[2], e.logs.places[0].path, "가장 최근이 맨 앞이다")
	assert.Equal(t, paths[0], e.logs.places[2].path)
}

// 같은 자리를 또 방문하면 하나만 남기고 맨 위로 올린다. 같은 자리의 자는 파일 + 줄이다.
func TestJumpLogMovesRepeatToFront(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	for _, line := range []int{0, 2, 4} {
		e.activeBuffer().MoveTo(scheme.Cursor{Line: line})
		e.arrive()
	}
	require.Equal(t, []int{4, 2, 0}, logLines(e))

	// 가운데 것을 다시 방문한다.
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})
	e.arrive()

	assert.Equal(t, []int{2, 4, 0}, logLines(e), "쌓이지 않고 위로 올라온다")
	assert.Len(t, e.logs.places, 3)

	// 칸만 다르면 같은 자리다.
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2, Col: 3})
	e.arrive()

	assert.Len(t, e.logs.places, 3, "칸은 보지 않는다")

	// 파일이 다르면 줄이 같아도 다른 자리다.
	require.NoError(t, gotoFile(e, paths[1], 2))
	e.arrive()

	assert.Len(t, e.logs.places, 4)
}

// **jumplist 가 버린 앞쪽이 기록에는 남는다.** tasks.md 가 이 기록에 맡긴 몫이다(ADR-0070 §3).
func TestJumpLogKeepsWhatJumplistDrops(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})

	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))
	e.arrive()

	// 되돌아간다. 앞으로 갈 자리(second.go:4) 가 이력에 남아 있다.
	e.jumpBack()
	require.Equal(t, paths[0], e.activeBuffer().Path)

	// 여기서 새로 뛰면 jumplist 는 앞쪽을 버린다.
	e.recordJump()
	require.NoError(t, gotoFile(e, paths[2], 6))
	e.arrive()

	assert.Nil(t, e.jumpForward(), "jumplist 는 앞쪽을 버렸다")

	// 그래도 기록에는 남아 있다.
	found := false
	for _, place := range e.logs.places {
		if place.path == paths[1] && place.line == 4 {
			found = true
		}
	}
	assert.True(t, found, "버려진 자리가 기록에는 남아야 한다")
}

// `ctrl+o`·`ctrl+i` 로 되짚는 것도 방문이다. 그 자리가 맨 위로 올라온다.
func TestJumpLogRecordsRetrace(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	e.activeBuffer().MoveTo(scheme.Cursor{Line: 2})

	e.recordJump()
	require.NoError(t, gotoFile(e, paths[1], 4))
	e.arrive()

	require.Equal(t, paths[1], e.logs.places[0].path)

	e.jumpBack()

	assert.Equal(t, paths[0], e.logs.places[0].path, "되짚어 간 자리가 맨 위다")
	assert.Len(t, e.logs.places, 2, "이미 있던 자리라 늘지 않는다")
}

// 256 개가 넘으면 오래된 것부터 버린다. 보이는 것(16 줄) 보다 훨씬 많이 담는다.
func TestJumpLogDropsOldest(t *testing.T) {
	e, _ := jumpEditor(t)

	e.active = 0
	for i := range jumpLogMax + 50 {
		// 줄을 다 다르게 해서 중복 거르기에 걸리지 않게 한다.
		e.recordVisit(jumpPlace{path: "a.go", line: i})
	}

	require.Len(t, e.logs.places, jumpLogMax)
	assert.Equal(t, jumpLogMax+49, e.logs.places[0].line, "맨 앞이 가장 최근이다")
	assert.Equal(t, 50, e.logs.places[jumpLogMax-1].line, "오래된 것부터 버린다")
}

// 이름 없는 buffer 는 담지 않는다. 되돌아갈 때 열 파일이 없다.
func TestJumpLogSkipsUnnamedBuffer(t *testing.T) {
	e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("")}, width: 80, height: 20}

	e.arrive()

	assert.Empty(t, e.logs.places)
}

// 검색은 떠난 자리와 닿은 자리를 둘 다 남긴다.
func TestSearchRecordsVisit(t *testing.T) {
	m := newTestEditorFile("main.go",
		"package main\n\nfunc alpha() {}\n\nfunc beta() {}\n\nfunc alpha2() {}\n", 80, 20)
	e := m.editor

	send(m, "/", "a", "l", "p", "h", "a", "enter")

	require.Len(t, e.logs.places, 2)
	assert.Equal(t, 2, e.logs.places[0].line, "닿은 자리가 맨 앞이다")
	assert.Equal(t, 0, e.logs.places[1].line, "떠난 자리도 남는다")
}
