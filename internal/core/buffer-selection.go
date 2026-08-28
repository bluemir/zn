package core

// visual mode 가 고른 범위다. anchor 는 Buffer 가 들고 반대쪽 끝은 커서다 (ADR-0037).

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

	// **줄 단위여도 칸을 담는다.** 고치는 자리는 줄 전체를 쓰지만(selectionOn) 복사가 커서를
	// 범위의 시작으로 옮길 때 그 칸이 필요하다 — 어느 쪽 끝에서 골랐든 처음 짚은 자리로
	// 돌아가는 것이 vim 의 visual `y` 다(buffer-yank.go 의 moveToRangeStart, ADR-0100).
	if buf.selection.linewise {
		return motionRange{
			startLine: startLine, startCol: startCol,
			endLine: endLine, endCol: endCol,
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
