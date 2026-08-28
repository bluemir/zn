package core

// 복사하는 것들이다. `y`·`yy` 다. 지우기와 범위 잡는 법이 같아서 모양이 나란하다 (ADR-0017).

// yankRange 는 잡아 둔 범위를 register 에 담는다. 복사할 것이 없으면 false 다.
//
// **범위를 잡는 것은 부르는 쪽이다.** motion 이 잡은 것(`yw`) 도 visual 이 고른 것(`y`) 도
// 여기로 온다 — 둘이 범위를 얻는 길만 다르고 그다음은 같다(action.go, ADR-0037).
//
// **아무것도 건드리지 않는다. 값 receiver 인 것이 그 증거다.** 복사는 읽는 일이고, vim 의
// `y` 가 커서를 옮기는 것은 그 위에 얹힌 별개의 걸음이다 — `yb` 는 앞으로 가고 `yw` 는
// 제자리인데, 그 규칙은 복사의 성질이 아니라 키의 규칙이다. 「복사하고, 커서를 옮긴다」를
// 부르는 쪽이 그 차례로 한다(action.go, ADR-0100).
func (buf Buffer) yankRange(area motionRange) (register, bool) {
	if area.linewise {
		return register{lines: append([][]byte(nil), buf.lines[area.startLine:area.endLine+1]...), linewise: true}, true
	}

	if area.startLine == area.endLine && area.startCol == area.endCol {
		return register{}, false
	}

	return register{lines: buf.textBetween(area.startLine, area.startCol, area.endLine, area.endCol)}, true
}

// yankLines 는 [from, to] 줄을 register 에 담는다. `:[범위]y` 가 쓴다.
//
// 커서를 움직이지 않는다. motion 쪽은 이동이 잡은 범위라 커서가 따라가야 하지만(`yb` 는
// 앞으로 간다) 여기는 손으로 친 줄 번호라 따라갈 이동이 없다. 파일도 건드리지 않는다.
func (buf Buffer) yankLines(from, to int) register {
	return register{lines: append([][]byte(nil), buf.lines[from:to+1]...), linewise: true}
}

// moveToRangeStart 는 범위의 시작이 지금 커서보다 앞이면 그리로 옮긴다. 아니면 제자리다.
// vim 의 `y` 가 그렇다 — `yw` 는 안 움직이고 `yb` 는 앞으로 간다.
//
// **범위의 시작이 곧 이동이 닿은 자리다.** `span` 이 커서 자리와 이동이 닿은 자리로 범위를
// 짓기 때문에(charSpan·lineSpan) 뒤로 간 이동은 닿은 자리가 시작이 된다. 그래서 `yk` 는
// 칸을 지키고 `ygg` 는 첫 비공백으로 간다 — 줄 단위 범위도 칸을 담아 두는 까닭이다.
//
// 「앞이 아니면 안 옮긴다」가 곧 「앞으로 가는 이동은 제자리」이고, `yy` 처럼 시작이 지금
// 자리인 것도 같은 조건에 걸려 안 움직인다(ADR-0100).
func (buf *Buffer) moveToRangeStart(area motionRange, width int) {
	if area.startLine > buf.cursorLine ||
		(area.startLine == buf.cursorLine && area.startCol >= buf.cursorCol) {
		return
	}

	buf.cursorLine, buf.cursorCol = area.startLine, area.startCol

	// 줄 단위 motion 의 칸은 desiredCol 을 이미 따라간 값이라 다시 잡지 않는다.
	// 글자 단위는 좌우로 움직인 것이라 이동 키와 같이 desiredCol 을 갱신한다.
	if !area.linewise {
		buf.updateDesiredCol(width)
	}

	buf.clampToNormal(width)
}
