package core

import (
	"charm.land/lipgloss/v2"
)

// selection 은 visual mode 가 고른 범위의 반대쪽 끝(anchor) 이다. 이쪽 끝은 커서라,
// 이동 키가 커서를 옮기면 범위가 그만큼 따라 자란다.
//
// **Buffer 가 든다.** 이 좌표는 그 Buffer 의 lines 안에서만 뜻이 있고, 커서·스크롤과 같이
// tab 을 오가도 파일에 붙어 있어야 한다. register 와 마지막 검색이 editor 에 있는 것은
// 그 둘이 tab 을 넘기 때문이고(spec.md) 이것은 넘지 않는다.
//
// mode model(viewEditorVisual) 에 두지 않은 것은 동작이 `run(e *editor)` 만 받기 때문이다
// (ADR-0034, ADR-0037).
type selection struct {
	active   bool
	linewise bool // `V` 로 연 것인가
	line     int  // anchor 의 줄
	col      int  // anchor 의 byte offset
}

// styleSelection 은 고른 범위의 색이다.
//
// 반전은 statusBar·tabline 이 이미 쓰고 있어서(ADR-0004) 본문에 쓰면 그 둘과 같은 모양이 된다.
// 검색 강조가 256색 고정값을 고른 것과 같은 자리다(ADR-0010).
//
// 글자색까지 고정하는 것도 검색과 같은 이유다 — 배경만 정하면 밝은 테마에서 읽히지 않는다.
var styleSelection = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("231"))

// selectionRange 는 고른 범위다. 고른 것이 없으면 false 다.
//
// anchor 와 커서 중 앞선 것이 시작이다. 어느 쪽으로 골랐든 범위는 같아서, 지우고 복사하는
// 쪽은 방향을 몰라도 된다.
//
// **커서가 선 글자를 범위에 넣는다.** vim 의 visual 은 inclusive 라 `vd` 가 `x` 와 같다.
// `e` 가 span 에서 includeCursorCluster 를 부르는 것과 같은 자리다(ADR-0013).
func (buf Buffer) selectionRange() (motionRange, bool) {
	if !buf.selection.active {
		return motionRange{}, false
	}

	startLine, startCol := buf.selection.line, buf.selection.col
	endLine, endCol := buf.cursorLine, buf.cursorCol

	if endLine < startLine || (endLine == startLine && endCol < startCol) {
		startLine, startCol, endLine, endCol = endLine, endCol, startLine, startCol
	}

	// 커서는 범위의 시작으로 간다. 어느 쪽 끝에서 골랐든 같다 — vim 의 visual `y` 가 그렇다.
	// 복사가 moveToRangeStart 로 이 자리를 쓴다(yank.go).
	if buf.selection.linewise {
		return motionRange{
			startLine: startLine, endLine: endLine,
			targetLine: startLine, targetCol: startCol,
			linewise: true,
		}, true
	}

	// 줄 끝에서는 밀 글자가 없다. 빈 줄을 고른 것이라 범위가 비어 있는 그대로다.
	if line := buf.lines[endLine]; endCol < len(line) {
		endCol += clusterSize(line, endCol)
	}

	return motionRange{
		startLine: startLine, startCol: startCol,
		endLine: endLine, endCol: endCol,
		targetLine: startLine, targetCol: startCol,
	}, true
}

// selectionOn 은 그 줄에서 고른 byte 구간이다. 고르지 않은 줄이면 ok 가 false 다.
//
// toEnd 는 개행까지 든 줄인지다. 그리는 쪽이 줄 끝에 빈 칸 하나를 더 칠한다 —
// `V` 로 고른 빈 줄은 칠할 글자가 없어서 그 칸이 없으면 아무것도 보이지 않는다.
func (buf Buffer) selectionOn(area motionRange, line int) (span []int, toEnd, ok bool) {
	if line < area.startLine || line > area.endLine {
		return nil, false, false
	}

	end := len(buf.lines[line])

	if area.linewise {
		return []int{0, end}, true, true
	}

	start := 0
	if line == area.startLine {
		start = area.startCol
	}

	if line == area.endLine {
		return []int{start, area.endCol}, false, true
	}

	return []int{start, end}, true, true
}

// startSelection 은 커서 자리를 anchor 로 삼아 범위를 연다. `v` `V` 와 드래그가 여기로 온다.
func (e *editor) startSelection(linewise bool) {
	buf := e.activeBuffer()

	buf.selection = selection{
		active:   true,
		linewise: linewise,
		line:     buf.cursorLine,
		col:      buf.cursorCol,
	}
}
