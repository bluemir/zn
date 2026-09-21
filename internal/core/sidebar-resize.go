package core

import (
	"github.com/bluemir/zn/internal/textarea"
)

// 트리 폭은 구분선을 끌어서 바꾼다(ADR-0145). ADR-0012 가 「mouse 로 sidebar 너비를 바꾸는
// 것」을 열어 두었던 자리다.
//
// 끄는 손은 tab 을 끄는 것(ADR-0090) 과 같은 틀이다 — 누를 때 시작한 자리를 적어 두고
// (editor.draggingSidebar) motion 마다 지금 자리로 옮기며, 놓을 때 지운다.

// sidebarMinWidth 는 끌어서 줄일 수 있는 가장 좁은 폭이다.
//
// 16 칸이면 이름에 13 칸이 남는다. 깊이 2 짜리 항목이 들여쓰기 4 칸과 펼침 표시 2 칸을
// 쓰고도 이름 일곱 칸이 보이는 폭이다. 더 좁히면 트리가 이름을 못 보여주는 목록이 된다.
//
// 더 왼쪽으로 끌어도 여기서 멈춘다. 끌어서 닫는 길은 만들지 않는다 — 트리를 잃는 일이고
// `:tree` 가 이미 그 일을 한다(ADR-0060 이 tab 을 끌어내서 닫지 않기로 한 것과 같다).
const sidebarMinWidth = 16

// sidebarDividerCol 은 구분선이 서는 화면 칸이다. 오른쪽에서 두 번째다(treeRow.render).
//
// **그리는 쪽과 판별하는 쪽이 어긋나면 한 칸 옆을 잡은 것이 된다.** 그 어긋남은 화면에
// 드러나지 않아서 찾기 어렵다 — 수치를 새로 두지 않고 한 자리에서 낸다(ADR-0012).
func (e editor) sidebarDividerCol() int {
	return e.sidebar.width - 2
}

// resizeSidebarTo 는 구분선이 화면 칸 x 에 서도록 트리 폭을 정한다.
//
// 좁히는 쪽은 sidebarMinWidth 에서 멈추고, 넓히는 쪽은 편집 영역이 MinTextWidth 만큼 남는
// 자리에서 멈춘다. **끌어서 트리를 감출 수 있으면 안 된다** — sidebarVisible 이 폭을 보고
// 정하므로 한 칸만 더 넓히면 트리가 통째로 사라지고, 사라지면 다시 잡을 구분선도 없다.
func (e *editor) resizeSidebarTo(x int) {
	width := x + 2

	e.sidebar.width = max(sidebarMinWidth, min(width, e.width-textarea.MinTextWidth))

	// 편집 영역 너비가 바뀌었다. 창들이 그 폭으로 줄을 다시 접어야 한다(ADR-0123).
	e.layoutViews()
}
