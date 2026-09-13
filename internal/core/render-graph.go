package core

import (
	"strings"
)

// 커밋 기록의 그래프 칸을 글자로 옮기는 자리다(ADR-0141).
//
// **여기서 하는 일은 표를 한 번 보는 것뿐이다.** 어느 칸이 무엇인지는 훑기가 이미 정했고
// (git-graph-draw.go) 여기는 그 갈래마다 어느 글자를 쓸지만 안다. 글자는 터미널에 따라
// 갈리는 판단이라 훑기가 들 것이 아니다(ADR-0028, ADR-0115 §5).

// graphWidthOf 는 커밋 하나를 그리는 데 필요한 칸 수다.
//
// **커밋마다 잰다.** 화면 전체를 한 번에 재면 저 아래 갈래 하나 때문에 위쪽 커밋이 통째로
// 오른쪽으로 밀리고, 훑어 내려갈 때마다 본문이 좌우로 흔들린다. git 의 `--graph` 도
// 줄마다 따로 잰다.
func graphWidthOf(row graphRow) int {
	width := 0
	for _, line := range row.graph {
		width = max(width, len(line))
	}

	return width
}

// renderGraphLine 은 그래프 행 하나를 글자로 옮기고 width 칸에 맞춘다.
//
// 폭을 맞추는 것은 뒤에 커밋 이름이 붙기 때문이다. 행마다 길이가 다르면 이름이 들쭉날쭉해진다.
func renderGraphLine(chars boxSet, line []graphSymbol, width int) string {
	out := strings.Builder{}

	for _, cell := range line {
		out.WriteString(graphChar(chars, cell))
	}

	if pad := width - len(line); pad > 0 {
		out.WriteString(strings.Repeat(" ", pad))
	}

	return out.String()
}

// graphChar 는 칸 하나의 글자다.
//
// **물러설 자리가 곧 git 이 쓰는 글자다.** `* | / \ _ - .` 는 전부 East Asian Width 가
// Narrow 라 어느 터미널에서도 한 칸이고, 폰트에 글리프가 없어 두부가 될 일도 없다(ADR-0028).
func graphChar(chars boxSet, cell graphSymbol) string {
	switch cell {
	case graphNode:
		return chars.dot
	case graphVertical:
		return chars.vertical
	case graphSlashUp:
		return chars.diagonalUp
	case graphSlashDown:
		return chars.diagonalDown
	case graphUnderscore, graphDash:
		return chars.horizontal
	case graphDot:
		return chars.topRight
	default:
		return " "
	}
}
