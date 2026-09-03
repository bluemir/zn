package textarea

import "github.com/bluemir/zn/internal/scheme"

// `\c`·`:cat` 이 내보낼 글을 뜨는 자리다. 무엇을 낼지는 cat.go 가 정하고 여기는 뜨기만 한다
// (ADR-0085).

// catLines 는 범위가 가리키는 글이다. 줄 단위면 그 줄들 전부, 글자 단위면 고른 조각이다.
//
// **복사하지 않는다.** 돌려주는 것은 buf.lines 를 가리키는 subslice 라, 받은 쪽은 찍고 나서
// 버린다 — register 에 담는 yankRange 와 갈리는 자리다(ADR-0001, ADR-0017).
//
// 커서도 파일도 건드리지 않는다. 그래서 값 receiver 다 — yankRange 가 포인터인 것은
// moveToRangeStart 때문인데, 커서를 옮길지는 부르는 쪽마다 다르다(visual 만 옮긴다).
func (buf Buffer) CatLines(area scheme.MotionRange) [][]byte {
	if area.Linewise {
		return buf.lines[area.Start.Line : area.End.Line+1]
	}

	return buf.textBetween(area.Start, area.End)
}
