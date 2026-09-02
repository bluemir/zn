package core

// 복사하는 것들이다. `y`·`yy` 다. 지우기와 범위 잡는 법이 같아서 모양이 나란하다 (ADR-0017).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-yank.go 에 있다 (ADR-0121).

// moveToRangeStart 는 범위의 시작이 지금 커서보다 앞이면 그리로 옮긴다. 아니면 제자리다.
// vim 의 `y` 가 그렇다 — `yw` 는 안 움직이고 `yb` 는 앞으로 간다.
//
// **범위의 시작이 곧 이동이 닿은 자리다.** `span` 이 커서 자리와 이동이 닿은 자리로 범위를
// 짓기 때문에(charSpan·lineSpan) 뒤로 간 이동은 닿은 자리가 시작이 된다. 그래서 `yk` 는
// 칸을 지키고 `ygg` 는 첫 비공백으로 간다 — 줄 단위 범위도 칸을 담아 두는 까닭이다.
//
// 「앞이 아니면 안 옮긴다」가 곧 「앞으로 가는 이동은 제자리」이고, `yy` 처럼 시작이 지금
// 자리인 것도 같은 조건에 걸려 안 움직인다(ADR-0100).
func (buf *viewport) moveToRangeStart(area motionRange, width int) {
	if area.start.line > buf.cursor.line ||
		(area.start.line == buf.cursor.line && area.start.col >= buf.cursor.col) {
		return
	}

	buf.cursor.line, buf.cursor.col = area.start.line, area.start.col

	// 줄 단위 motion 의 칸은 desiredX 을 이미 따라간 값이라 다시 잡지 않는다.
	// 글자 단위는 좌우로 움직인 것이라 이동 키와 같이 desiredX 을 갱신한다.
	if !area.linewise {
		buf.updateDesiredCol(width)
	}

	buf.clampToNormal(width)
}
