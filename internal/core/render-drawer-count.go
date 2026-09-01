package core

import (
	"fmt"
	"strings"
)

// renderCountBorder 는 아랫 테두리 오른쪽에 「몇 번째/전부」를 얹은 줄이다.
//
// **목록 행을 하나도 쓰지 않는다.** 테두리는 어차피 그리는 줄이고, 검색 입력 박스가 위
// 테두리에 제목을 얹는 것과 같은 장치다(view-grep-input.go).
//
// 담는 것이 보이는 것보다 훨씬 많은 판(검색·되돌아간 자리·방문한 자리) 이 나눠 쓴다.
// 아래 줄에도 같은 숫자를 적으면 한 값이 두 자리에 있게 되므로 그쪽에서는 뺀다 — 숫자가
// 설명하는 대상 옆에 있는 것이 낫고, 아래 줄은 키 안내에 자리를 넘긴다(ADR-0079).
//
// **오른쪽에 붙인다.** 왼쪽 아래는 판마다 다른 것이 올 수 있고(위 테두리의 제목이 왼쪽이다)
// 오른쪽 아래는 어느 판에서나 비어 있다.
//
// 자리에 안 들어가면 테두리만 그린다. 반쯤 잘린 숫자는 숫자로 읽히지 않는다 — 빈 화면의
// 로고를 자르지 않고 버리는 것과 같은 태도다(ADR-0061).
func renderCountBorder(chars boxSet, width, at, total int) string {
	room := width - 2
	plain := chars.bottomLeft + strings.Repeat(chars.horizontal, room) + chars.bottomRight

	// 담긴 것이 없으면 셀 것도 없다.
	if total <= 0 {
		return plain
	}

	label := fmt.Sprintf(" %s/%s ", formatCount(at), formatCount(total))
	if widthOf(label) > room {
		return plain
	}

	// 오른쪽 끝에서 한 칸 띄운다. 모서리에 붙으면 테두리의 일부처럼 읽힌다.
	tail := 1
	head := room - widthOf(label) - tail
	if head < 0 {
		head, tail = room-widthOf(label), 0
	}

	return chars.bottomLeft +
		strings.Repeat(chars.horizontal, head) + label + strings.Repeat(chars.horizontal, tail) +
		chars.bottomRight
}
