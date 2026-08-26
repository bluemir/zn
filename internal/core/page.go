package core

// pageDirection 은 화면 이동이 가는 쪽이다. `indentDirection` 과 같은 손이다.
type pageDirection int

const (
	pageDown pageDirection = iota // `ctrl+d` `ctrl+f`
	pageUp                        // `ctrl+u` `ctrl+b`
)

// pageSpan 은 한 번에 옮기는 크기다. 반 화면과 한 화면 둘이다.
type pageSpan int

const (
	pageHalf pageSpan = iota // `ctrl+d` `ctrl+u`
	pageFull                 // `ctrl+f` `ctrl+b`
)

// pageOverlapRows 는 한 화면을 넘길 때 앞 화면에서 다시 보여주는 행 수다.
//
// vim 의 `ctrl+f` 가 남기는 두 행이고 `less` 도 같다. 넘긴 자리가 어떻게 이어지는지 보여주는
// 것이라, 이것이 0 이면 두 화면 사이의 한 줄이 눈에 이어져 들어오지 않는다(ADR-0063).
//
// 반 화면에는 이 겹침이 없다. 절반씩 가면 앞 화면의 아래쪽 절반이 그대로 위쪽 절반이 되어
// 겹침이 이미 화면의 반이다.
const pageOverlapRows = 2

// pageRows 는 그 크기가 몇 화면 행인지다.
//
// 반 화면은 vim 의 `scroll` 기본값과 같이 높이의 절반이다. 어느 쪽이든 한 행 아래로는
// 내려가지 않는다 — 0 으로 두면 좁은 화면에서 키가 안 먹는 것처럼 보인다.
func pageRows(span pageSpan, height int) int {
	if span == pageFull {
		return max(height-pageOverlapRows, 1)
	}

	return max(height/2, 1)
}

// movePage 는 고른 항목과 트리 화면을 한 번에 같이 옮긴다. 편집 영역의 것과 같다.
//
// 트리는 줄을 접지 않아서 한 항목이 한 행이다. 그래서 화면 행과 항목 번호가 같은 수다.
//
// top 을 옮기고 나서 부르는 쪽의 scrollTo 가 둘을 맞춘다 — 여기서는 더하기만 하고 범위
// 맞추기를 되풀이하지 않는다(view-sidebar.go).
func (s *sidebar) movePage(direction pageDirection, span pageSpan, count, height int) {
	rows := len(s.rows())
	if rows == 0 || height < 1 {
		return
	}

	n := pageRows(span, height) * max(count, 1)
	if direction == pageUp {
		n = -n
	}

	s.selected = max(0, min(s.selected+n, rows-1))

	// 마지막 항목이 화면 맨 아래에 오는 자리가 끝이다. 편집 영역과 같은 한계다.
	s.top = max(0, min(s.top+n, max(0, rows-height)))
}
