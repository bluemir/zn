package textarea

import "github.com/bluemir/zn/internal/scheme"

// 복사하는 것들이다. `y`·`yy` 다. 지우기와 범위 잡는 법이 같아서 모양이 나란하다 (ADR-0017).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-yank.go 다 (ADR-0121).

// yankRange 는 잡아 둔 범위를 register 에 담는다. 복사할 것이 없으면 false 다.
//
// **범위를 잡는 것은 부르는 쪽이다.** motion 이 잡은 것(`yw`) 도 visual 이 고른 것(`y`) 도
// 여기로 온다 — 둘이 범위를 얻는 길만 다르고 그다음은 같다(action.go, ADR-0037).
//
// **아무것도 건드리지 않는다. 값 receiver 인 것이 그 증거다.** 복사는 읽는 일이고, vim 의
// `y` 가 커서를 옮기는 것은 그 위에 얹힌 별개의 걸음이다 — `yb` 는 앞으로 가고 `yw` 는
// 제자리인데, 그 규칙은 복사의 성질이 아니라 키의 규칙이다. 「복사하고, 커서를 옮긴다」를
// 부르는 쪽이 그 차례로 한다(action.go, ADR-0100).
func (buf Buffer) YankRange(area scheme.MotionRange) (TextBlock, bool) {
	if area.Linewise {
		return TextBlock{Lines: append([][]byte(nil), buf.lines[area.Start.Line:area.End.Line+1]...), Linewise: true}, true
	}

	if area.Start.Line == area.End.Line && area.Start.Col == area.End.Col {
		return TextBlock{}, false
	}

	return TextBlock{Lines: buf.textBetween(area.Start, area.End)}, true
}

// yankLines 는 [from, to] 줄을 register 에 담는다. `:[범위]y` 가 쓴다.
//
// 커서를 움직이지 않는다. motion 쪽은 이동이 잡은 범위라 커서가 따라가야 하지만(`yb` 는
// 앞으로 간다) 여기는 손으로 친 줄 번호라 따라갈 이동이 없다. 파일도 건드리지 않는다.
func (buf Buffer) yankLines(from, to int) TextBlock {
	return TextBlock{Lines: append([][]byte(nil), buf.lines[from:to+1]...), Linewise: true}
}
