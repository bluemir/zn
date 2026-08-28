package core

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/syntax"
)

// tab 과 space 는 화면에서 똑같은 빈 칸이라, 들여쓰기가 둘로 섞여도 알아챌 수 없다.
// 마커를 찍어 눈으로 가른다.
//
// 둘 다 East Asian Width 가 Neutral 이라 어느 터미널에서나 한 칸이다. Ambiguous 인 글자
// (`·` U+00B7, `→` U+2192 …) 는 ambiguous 를 두 칸으로 잡는 터미널에서 들여쓰기를 밀어낸다.
const (
	markerTab   = "»" // U+00BB
	markerSpace = "⋅" // U+22C5 DOT OPERATOR
)

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
func expandRow(line []byte, start, end, col int, mark whitespaceMark, tab int) ([]screenPart, int) {
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
		size, width := clusterAt(line, offset, col, tab)

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

// rowHighlight 는 행 하나에 칠할 것들이다. 검색 매칭과 visual 선택이 같은 walker 를 지난다.
type rowHighlight struct {
	// matches 는 그 줄에서 찾은 자리들이다. 줄 전체 기준이라 이 행 밖으로 넘어가는 것이 섞여
	// 있고, wrap 된 줄에서 행 경계에 걸친 매칭은 양쪽 행에 나뉘어 칠해진다.
	matches [][]int

	// cursorCol 은 커서가 이 줄에서 선 칸이다. 커서가 다른 줄이면 -1 이다.
	// 커서가 선 매칭만 색이 다르다.
	cursorCol int

	// selection 은 visual 이 고른 byte 구간이다. 고르지 않은 줄이면 nil 이다(selection.go).
	selection []int

	// toLineEnd 는 선택이 개행까지인지다. 줄 끝에 빈 칸 하나를 더 칠한다.
	toLineEnd bool

	// tokens 는 그 줄의 문법 토큰이다. 강조하지 않는 파일이면 nil 이다(syntax.go).
	// matches 와 같이 줄 전체 기준이라 이 행 밖으로 넘어가는 것이 섞여 있다.
	tokens []syntax.Token
}

// rowSegment 는 행의 한 구간과 거기에 입힐 색이다.
type rowSegment struct {
	start, end int
	style      lipgloss.Style
}

// topSegments 는 문법 강조 위에 덮이는 구간들을 앞에서부터 겹치지 않게 늘어놓는다.
// 검색 매칭과 visual 선택이다.
//
// 겹치는 두 목록을 walker 에 그냥 넘길 수 없어서 여기서 한 줄로 편다 — walker 는 왼쪽에서
// 오른쪽으로 한 번만 지나간다.
//
// **겹친 자리는 검색이 이긴다.** 선택 배경이 찾은 자리를 덮으면 `n` 이 데려다 놓은 곳이
// 어디인지 보이지 않는다.
//
// 빈 자리가 남는다 — 그것을 문법 색으로 메우는 것이 segments 다.
func (hl rowHighlight) topSegments(start, end int) []rowSegment {
	segments := []rowSegment{}

	put := func(from, to int, style lipgloss.Style) {
		if from < to {
			segments = append(segments, rowSegment{start: from, end: to, style: style})
		}
	}

	// selectStart 는 선택에서 아직 칠하지 않은 앞자리다. 매칭이 지나갈 때마다 그만큼 밀린다.
	selectStart, selectEnd := 0, 0
	if hl.selection != nil {
		selectStart, selectEnd = max(hl.selection[0], start), min(hl.selection[1], end)
	}

	for _, match := range hl.matches {
		from, to := max(match[0], start), min(match[1], end)
		if from >= to {
			continue
		}

		put(selectStart, min(from, selectEnd), styleSelection)
		selectStart = max(selectStart, to)

		style := styleSearchMatch
		if match[0] == hl.cursorCol {
			style = styleSearchCurrent
		}

		put(from, to, style)
	}

	put(selectStart, selectEnd, styleSelection)

	return segments
}

// segments 는 이 행을 빈틈 없이 덮는 구간들이다. 앞에서부터 겹치지 않게 늘어선다.
//
// **문법 강조는 가장 아래 층이다.** 위 층(검색·선택) 이 덮지 않은 자리만 문법 색이 메운다.
// 검색과 선택은 글자색과 배경을 함께 정하고 문법은 글자색만 정하는데, 겹친 자리에서 둘을
// 섞으면 고른 범위 안의 글자색이 자리마다 달라져 범위 자체가 흐려진다. 고른 자리가 문법
// 색을 잃는 것은 vim 도 같다.
//
// 빈틈이 없어서 그리는 쪽이 사이를 메울 자리가 없다 — 색 없는 자리는 아무 속성 없는 style 인
// 구간으로 들어온다. 그 style 은 글자를 그대로 두므로, 강조가 없을 때의 화면 글자는 문법
// 강조를 넣기 전과 한 글자도 다르지 않다.
func (hl rowHighlight) segments(start, end int) []rowSegment {
	top := hl.topSegments(start, end)

	segments := make([]rowSegment, 0, len(top)*2+1)

	// at 은 아직 덮지 않은 앞자리다.
	at := start
	for _, segment := range top {
		segments = hl.appendSyntax(segments, at, segment.start)
		segments = append(segments, segment)
		at = segment.end
	}

	return hl.appendSyntax(segments, at, end)
}

// appendSyntax 는 [from, to) 를 토큰 경계로 잘라 붙인다.
// 토큰 사이와, 색을 정하지 않은 갈래는 아무 속성 없는 구간이 된다.
//
// 행 밖은 잘라낸다. 줄 전체 기준인 토큰이라 wrap 된 줄에서 행 경계에 걸친 토큰은 양쪽 행에
// 나뉘어 칠해진다 — 검색 매칭·선택을 max/min 으로 자르는 것과 같은 자리다(topSegments).
// 자르는 김에 줄 밖을 가리키는 토큰도 함께 접히므로, lexer 가 어긋난 자리를 줘도 그리는
// 쪽이 없는 byte 를 집지 않는다(expandRow).
func (hl rowHighlight) appendSyntax(segments []rowSegment, from, to int) []rowSegment {
	if from >= to {
		return segments
	}

	at := from
	for _, token := range hl.tokens {
		left, right := max(token.Start, from), min(token.End, to)
		if left >= right {
			continue
		}

		style, ok := styleSyntax[token.Kind]
		if !ok {
			// 색을 정하지 않은 갈래다. 조각을 나누지 않고 뒤 구간에 흘려보낸다 — 화면에
			// 달라지는 것이 없으니 escape 를 덜 낸다.
			continue
		}

		if at < left {
			segments = append(segments, rowSegment{start: at, end: left})
		}
		segments = append(segments, rowSegment{start: left, end: right, style: style})
		at = right
	}

	if at < to {
		segments = append(segments, rowSegment{start: at, end: to})
	}

	return segments
}

// renderRow 는 화면 행 하나를 그린다. 강조가 걸쳐 있으면 그 구간만 색을 입힌다.
//
// width 는 편집 영역의 너비다. 줄 끝에 덧붙이는 선택 칸이 그 안에 드는지 보는 데 쓴다.
// tab 은 이 파일의 tab 폭이다(`buf.tabWidth()`, ADR-0096).
func renderRow(line []byte, row screenRow, width int, hl rowHighlight, tab int) string {
	mark := markWhitespace(line)

	out := strings.Builder{}
	col := 0
	offset := row.start

	// put 은 offset 부터 end 까지를 그 색으로 그린다.
	put := func(end int, style lipgloss.Style) {
		if end <= offset {
			return
		}

		parts, next := expandRow(line, offset, end, col, mark, tab)
		out.WriteString(renderParts(parts, style))
		col, offset = next, end
	}

	// segments 가 행을 빈틈 없이 덮으므로 사이를 메울 자리가 없다.
	for _, segment := range hl.segments(row.start, row.end) {
		put(segment.end, segment.style)
	}

	// 선택이 개행까지면 줄 끝에 칸 하나를 더 칠한다. `V` 로 고른 빈 줄은 칠할 글자가 없어서
	// 이 칸이 없으면 골랐다는 것이 화면에 드러나지 않는다. vim 과 같다.
	//
	// 줄의 마지막 행에만 붙이고, 편집 영역을 넘으면 붙이지 않는다 — 한 칸이 넘치면 줄바꿈이
	// 어긋난다.
	if hl.toLineEnd && row.end == len(line) && col < width {
		out.WriteString(styleSelection.Render(" "))
	}

	return out.String()
}
