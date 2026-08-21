package core

import (
	"fmt"
	"unicode/utf8"
)

// yankByMotion 은 motion 이 가리키는 범위를 register 에 담는다. `y` 뒤에 붙은 키가 motion 이다.
//
// 파일을 건드리지 않으므로 dirty 도 되돌리기도 없다. 범위 계산은 `d` 와 같은 것을 쓴다 —
// 규칙이 두 벌이 되면 `dw` 와 `yw` 가 갈린다(ADR-0017).
//
// 모르는 motion 이거나 복사할 것이 없으면 false 다.
func (buf *Buffer) yankByMotion(m motion, count, width int) (register, bool) {
	area, ok := m.span(*buf, count, width)
	if !ok {
		return register{}, false
	}

	return buf.yankRange(area, width)
}

// yankRange 는 잡아 둔 범위를 register 에 담는다.
// motion 이 잡은 것도 visual 이 고른 것도 여기로 온다(ADR-0037).
func (buf *Buffer) yankRange(area motionRange, width int) (register, bool) {
	if area.linewise {
		lines := append([][]byte(nil), buf.lines[area.startLine:area.endLine+1]...)
		buf.moveToRangeStart(area, width)

		return register{lines: lines, linewise: true}, true
	}

	if area.startLine == area.endLine && area.startCol == area.endCol {
		return register{}, false
	}

	lines := buf.textBetween(area.startLine, area.startCol, area.endLine, area.endCol)
	buf.moveToRangeStart(area, width)

	return register{lines: lines}, true
}

// yankLines 는 [from, to] 줄을 register 에 담는다. `:[범위]y` 가 쓴다.
//
// 커서를 움직이지 않는다. motion 쪽은 이동이 잡은 범위라 커서가 따라가야 하지만(`yb` 는
// 앞으로 간다) 여기는 손으로 친 줄 번호라 따라갈 이동이 없다. 파일도 건드리지 않는다.
func (buf Buffer) yankLines(from, to int) register {
	return register{lines: append([][]byte(nil), buf.lines[from:to+1]...), linewise: true}
}

// moveToRangeStart 는 뒤로 가는 motion 이었으면 커서를 범위의 시작으로 옮긴다.
// 앞으로 가는 motion 이면 제자리다. vim 의 `y` 가 그렇다 — `yw` 는 안 움직이고 `yb` 는 앞으로 간다.
//
// 자리는 motion 이 커서를 둔 곳 그대로다. 그래서 `yk` 는 칸을 지키고 `ygg` 는 첫 비공백으로 간다.
func (buf *Buffer) moveToRangeStart(area motionRange, width int) {
	if area.targetLine > buf.cursorLine ||
		(area.targetLine == buf.cursorLine && area.targetCol >= buf.cursorCol) {
		return
	}

	buf.cursorLine, buf.cursorCol = area.targetLine, area.targetCol

	// 줄 단위 motion 의 칸은 desiredCol 을 이미 따라간 값이라 다시 잡지 않는다.
	// 글자 단위는 좌우로 움직인 것이라 이동 키와 같이 desiredCol 을 갱신한다.
	if !area.linewise {
		buf.updateDesiredCol(width)
	}

	buf.clampToNormal(width)
}

// copiedMessage 는 `y` 가 statusBar 아래 줄에 띄우는 알림이다. 다음 키를 누르면 사라진다.
//
// 복사는 화면에 아무 자국을 남기지 않는다 — 지우기와 달리 글자가 그대로 있어서, 손이 친 `y`
// 가 먹었는지 범위를 어디까지 잡았는지 볼 길이 없다. 그래서 한 줄짜리도 알린다.
// vim 은 `report` 만큼 넘을 때만 알리지만, 그 기준은 "파일이 크게 바뀌었다" 는 경고 쪽이다.
//
// 줄 단위면 줄 수, 글자 단위면 글자 수다. 붙여넣기가 줄로 들어갈지 글자로 들어갈지가 곧
// 이 갈래라, 알림이 그것을 같이 보여 준다.
func (reg register) copiedMessage() string {
	if reg.linewise {
		return fmt.Sprintf("%d 줄 복사되었습니다", len(reg.lines))
	}

	return fmt.Sprintf("%d 글자 복사되었습니다", reg.charCount())
}

// charCount 는 글자 단위 register 에 담긴 글자 수다. byte 가 아니라 rune 으로 센다 —
// 한글 한 자가 3 글자로 세어지면 안 된다.
//
// 여러 줄에 걸친 것이면 사이의 줄바꿈도 한 글자다. 담긴 그대로가 붙여넣기로 나가는 글자다.
func (reg register) charCount() int {
	count := max(len(reg.lines)-1, 0)
	for _, line := range reg.lines {
		count += utf8.RuneCount(line)
	}

	return count
}
