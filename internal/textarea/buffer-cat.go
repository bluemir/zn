package textarea

import "github.com/bluemir/zn/internal/scheme"

// `\c`·`:cat` 이 내보낼 글을 return 한다. 무엇을 낼지는 cat.go 가 정하고 여기는 뜨기만 한다 (ADR-0085).
// catLines 는 범위가 가리키는 글이다. 줄 단위면 그 줄들 전부, 글자 단위면 고른 조각이다.
func (buf Buffer) CatLines(area scheme.MotionRange) [][]byte {
	if area.Linewise {
		return buf.lines[area.Start.Line : area.End.Line+1]
	}

	return buf.textBetween(area.Start, area.End)
}
