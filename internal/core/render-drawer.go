package core

import "strings"

// drawer 는 편집 화면 아래에 얹는 판이다. 목록 한 종류만 담는 판 넷이 이 틀을 나눠 쓴다 —
// 검색·방문한 자리·되돌아간 자리·`GOTO` 다(ADR-0101).
//
// 각 행이 정확히 width 칸이다. 테두리는 손으로 붙인다. lipgloss 의 Border 는 안쪽 내용의
// 폭을 스스로 재는데 강조 escape 가 이미 섞여 있어서 그 계산을 믿을 수 없다.
//
// 특수문자 판(view-symbol.go) 과 register 판(view-registers.go) 은 이것을 쓰지 않는다.
// 저쪽은 안쪽이 목록 하나가 아니고(입력줄·가름줄·격자·이름줄), 이쪽은 고른 줄이 없어서
// 아랫 테두리에 얹을 「몇 번째」가 없다.
type drawer struct {
	chars  boxSet
	width  int    // 판 전체 폭. textWidth() 다
	height int    // 목록에 쓸 행 수. 담긴 것이 모자라면 빈 행으로 채운다
	top    int    // 첫 행이 담긴 것 중 몇 번째인가. 0 부터다
	count  int    // 담긴 것 전부
	empty  string // 담긴 것이 없을 때 한 줄로 알릴 말

	// row 는 at 번째 줄이다. inner 는 테두리 둘과 좌우 한 칸씩을 뺀 폭이고 돌려주는 것은
	// 정확히 그만큼이어야 한다.
	//
	// 고른 줄을 반전하는 것도 여기서 한다. 칸을 먼저 채우고 그다음에 강조를 입혀야 하는데
	// (lines.WidthOf 가 escape 까지 센다) 그 순서를 아는 것이 줄을 그리는 쪽이다.
	row func(at, inner int) string

	// at 은 아랫 테두리에 얹을 「몇 번째를 보고 있는가」다. 1 부터이고 0 이면 얹지 않는다
	// (render-drawer-count.go, ADR-0079).
	//
	// 고른 줄이 없는 판이 0 이다. 지금 넷은 모두 고른 줄이 있어서 0 을 쓰는 자리가 없지만,
	// register 판이나 `:jobs` 가 이 틀로 오면 그때가 0 이다(docs/tasks.md).
	at int
}

func (d drawer) render() string {
	inner := d.width - 4 // 테두리 둘과 좌우 한 칸씩
	line := strings.Repeat(d.chars.horizontal, d.width-2)

	rows := []string{d.chars.topLeft + line + d.chars.topRight}
	rows = append(rows, d.listRows(inner)...)

	if d.at > 0 {
		// 아랫 테두리가 몇 번째를 보고 있는지 든다. 담는 것이 보이는 것보다 많은 판에서는
		// 목록만으로 어디쯤인지 모른다(render-drawer-count.go, ADR-0079).
		rows = append(rows, renderCountBorder(d.chars, d.width, d.at, d.count))
	} else {
		rows = append(rows, d.chars.bottomLeft+line+d.chars.bottomRight)
	}

	return strings.Join(rows, "\n")
}

// listRows 는 목록 행들이다. 담긴 것이 없으면 그 사실을 한 줄로 알린다.
func (d drawer) listRows(inner int) []string {
	side := d.chars.vertical

	body := make([]string, 0, d.height)
	for at := d.top; at < d.count && len(body) < d.height; at++ {
		body = append(body, d.row(at, inner))
	}

	if d.count == 0 && d.height > 0 {
		// padTo 는 채우기만 하고 자르지 않는다. 좁은 편집 영역에서는 이 문구가 상자보다
		// 넓어서 오른쪽 테두리를 밀어낸다 — 먼저 자른다.
		body = append(body, styleDetail.Render(padTo(truncateToWidth(d.empty, inner), inner)))
	}

	for len(body) < d.height {
		body = append(body, strings.Repeat(" ", inner))
	}

	rows := make([]string, 0, len(body))
	for _, text := range body {
		rows = append(rows, side+" "+text+" "+side)
	}

	return rows
}
