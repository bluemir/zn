package core

// visual mode 가 고른 범위다. anchor 는 Buffer 가 들고 반대쪽 끝은 커서다 (ADR-0037).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-selection.go 다 (ADR-0121).

// selectionOn 은 그 줄에서 고른 byte 구간이다. 고르지 않은 줄이면 ok 가 false 다.
//
// toEnd 는 개행까지 든 줄인지다. 그리는 쪽이 줄 끝에 빈 칸 하나를 더 칠한다 —
// `V` 로 고른 빈 줄은 칠할 글자가 없어서 그 칸이 없으면 아무것도 보이지 않는다.
func (buf Buffer) selectionOn(area motionRange, line int) (span []int, toEnd, ok bool) {
	if line < area.start.line || line > area.end.line {
		return nil, false, false
	}

	end := len(buf.lines[line])

	if area.linewise {
		return []int{0, end}, true, true
	}

	start := 0
	if line == area.start.line {
		start = area.start.col
	}

	if line == area.end.line {
		return []int{start, area.end.col}, false, true
	}

	return []int{start, end}, true, true
}
