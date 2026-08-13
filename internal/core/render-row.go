package core

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// tab 과 space 는 화면에서 똑같은 빈 칸이라, 들여쓰기가 둘로 섞여도 알아챌 수 없다.
// 마커를 찍어 눈으로 가른다.
const (
	markerTab   = "»"
	markerSpace = "·"
)

// colorWhitespace 는 공백 마커의 색이다. 본문보다 흐려야 코드를 읽는 데 끼어들지 않는다.
// 검색 강조·sidebar·상대 줄번호와 같이 256색 고정값이다(ADR-0005, ADR-0007).
var colorWhitespace = lipgloss.Color("240")

// whitespaceMark 는 한 줄에서 마커를 붙일 구간이다. 들여쓰기와 줄 끝 공백뿐이다.
//
// 줄 가운데 공백까지 찍으면 산문과 주석이 점으로 뒤덮여 읽기 어렵다. 반면 들여쓰기는
// tab 과 space 가 섞였는지가 중요하고, 줄 끝 공백은 보이지 않으면 지울 생각을 못 한다.
type whitespaceMark struct {
	leadEnd    int // 이 자리 앞은 들여쓰기다
	trailStart int // 이 자리부터는 줄 끝 공백이다
}

func markWhitespace(line []byte) whitespaceMark {
	lead := 0
	for lead < len(line) && (line[lead] == ' ' || line[lead] == '\t') {
		lead++
	}

	// 공백뿐인 줄은 들여쓰기가 줄 전체다. 줄 끝을 따로 세면 두 구간이 겹친다.
	if lead == len(line) {
		return whitespaceMark{leadEnd: lead, trailStart: lead}
	}

	trail := len(line)
	for trail > lead && (line[trail-1] == ' ' || line[trail-1] == '\t') {
		trail--
	}

	return whitespaceMark{leadEnd: lead, trailStart: trail}
}

// marks 는 그 자리의 공백에 마커를 찍을지다.
func (mark whitespaceMark) marks(offset int) bool {
	return offset < mark.leadEnd || offset >= mark.trailStart
}

// screenPart 는 화면 행에서 style 을 한 번에 입히는 조각이다.
//
// 마커에만 색을 덧씌우면 그 색을 끝내는 리셋(`ESC[m`) 이 바깥 style 까지 함께 꺼버려서,
// 검색으로 강조한 구간이 마커 뒤에서 끊긴다. 조각으로 나눠 조각마다 style 을 한 번씩만
// 입혀야 리셋이 조각 경계에서만 일어난다.
type screenPart struct {
	text   string
	marker bool
}

// expandRow 는 line 의 start..end 를 화면에 그릴 조각들로 펼치고, 펼친 뒤의 칸을 같이 돌려준다.
//
// bubbletea 의 셀 렌더러는 폭 0 인 제어문자를 셀에 담지 못해 버린다. tab 을 그대로 넘기면
// 화면에서 들여쓰기가 사라진다. Go 소스는 tab 들여쓰기라 바로 드러난다.
//
// col 은 start 가 놓이는 화면 칸이다. 행을 조각내어 그릴 때(검색 강조) 조각마다 시작 칸이
// 다른데 tab 이 다음 tab stop 까지 밀어내는 폭은 시작 칸에 달려 있다. 조각을 각각 0 칸부터
// 세면 들여쓰기가 어긋난다.
func expandRow(line []byte, start, end, col int, mark whitespaceMark) ([]screenPart, int) {
	parts := []screenPart{}
	text := strings.Builder{}
	marker := false

	flush := func() {
		if text.Len() < 1 {
			return
		}

		parts = append(parts, screenPart{text: text.String(), marker: marker})
		text.Reset()
	}

	for offset := start; offset < end; {
		size, width := clusterAt(line, offset, col)

		char := line[offset]
		isMarker := (char == '\t' || char == ' ') && mark.marks(offset)

		if isMarker != marker {
			flush()
			marker = isMarker
		}

		switch {
		case char == '\t' && isMarker:
			// 마커는 tab 이 시작하는 한 칸뿐이다. 남는 칸까지 점으로 채우면 space 와 같아 보인다.
			text.WriteString(markerTab)
			text.WriteString(strings.Repeat(" ", width-1))
		case char == '\t':
			text.WriteString(strings.Repeat(" ", width))
		case isMarker:
			text.WriteString(markerSpace)
		default:
			text.Write(line[offset : offset+size])
		}

		col += width
		offset += size
	}

	flush()

	return parts, col
}

// renderParts 는 조각들을 style 로 그린다. 마커 조각만 흐린 색으로 덮어쓴다.
// 아무 속성 없는 style 은 글자를 그대로 둔다.
func renderParts(parts []screenPart, style lipgloss.Style) string {
	out := strings.Builder{}

	for _, part := range parts {
		if part.marker {
			out.WriteString(style.Foreground(colorWhitespace).Render(part.text))
			continue
		}

		out.WriteString(style.Render(part.text))
	}

	return out.String()
}
