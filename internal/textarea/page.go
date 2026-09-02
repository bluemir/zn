package textarea

// pageDirection 은 화면 이동이 가는 쪽이다. `indentDirection` 과 같은 손이다.
type PageDirection int

const (
	PageDown PageDirection = iota // `ctrl+d` `ctrl+f`
	PageUp                        // `ctrl+u` `ctrl+b`
)

// pageSpan 은 한 번에 옮기는 크기다. 반 화면과 한 화면 둘이다.
type PageSpan int

const (
	PageHalf PageSpan = iota // `ctrl+d` `ctrl+u`
	PageFull                 // `ctrl+f` `ctrl+b`
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
func PageRows(span PageSpan, height int) int {
	if span == PageFull {
		return max(height-pageOverlapRows, 1)
	}

	return max(height/2, 1)
}
