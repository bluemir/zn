package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

// tablineRow 는 tabline 한 줄과 그 줄의 어디에 무엇이 그려졌는지다.
//
// 자리표를 그리는 자리에서 같이 내는 것은 클릭 때문이다. 배치 계산을 두 벌 두면
// `+`(dirty) 하나로 칸이 밀렸을 때 클릭이 옆 tab 으로 간다.
type tablineRow struct {
	line string

	// tabs 는 tab 마다 그려진 칸 범위 [start, end) 다. 밀려나서 안 그려진 tab 은 빈 범위다.
	tabs [][2]int

	// left, right 는 가려짐 표시가 그려진 칸 범위다. 가린 것이 없으면 빈 범위다.
	left, right [2]int
}

// inSpan 은 칸 col 이 그 범위 안인지다. 빈 범위는 어느 칸도 품지 않는다.
func inSpan(span [2]int, col int) bool {
	return col >= span[0] && col < span[1]
}

// tabLabel 은 tabline 에 그리는 tab 한 칸의 글자다. `번호 파일이름` 이고 경로는 쓰지 않는다.
func (e editor) tabLabel(index int) string {
	buf := e.buffers[index]

	name := filepath.Base(buf.path)
	if buf.path == "" {
		name = "[No Name]"
	}
	if buf.dirty {
		name += "+"
	}

	return fmt.Sprintf(" %d %s ", index+1, name)
}

// tabWindow 는 tabline 에 그릴 것들이다. scroll 자리부터 count 개의 tab 을 그리고
// 양끝에 가려짐 표시를 붙인다.
//
// 표시보다 tab 이 먼저다. 좁아서 하나를 버려야 하면 표시가 빠져 빈 문자열이 된다 —
// 지금 보고 있는 파일 이름이 몇 개가 가려졌는지보다 중요하다.
type tabWindow struct {
	count       int
	left, right string
}

// layoutTabs 는 scroll 자리부터 width 칸에 들어가는 만큼을 배치한다.
//
// 반쯤 걸친 tab 은 넣지 않는다. 잘린 이름은 어느 파일인지 알려주지도 못하면서
// 가려진 개수에서도 빠져 `n>` 의 숫자를 틀리게 만든다.
//
// 오른쪽 표시의 폭이 가려진 개수의 자릿수를 타므로 한 번에 셀 수 없다. 표시가 없다고 보고
// 채운 뒤, 그 자리가 모자라면 tab 을 하나씩 물린다. 물릴 때마다 개수가 늘어 표시가
// 길어질 수 있으므로 다시 본다.
func (e editor) layoutTabs(scroll, width int) tabWindow {
	window := tabWindow{}
	if scroll > 0 {
		window.left = fmt.Sprintf("<%d", scroll)
	}

	// cost 는 그 tab 이 먹는 칸이다. 앞에 이미 그린 것이 있으면 구분선 한 칸이 붙는다.
	cost := func(index, drawn int, left string) int {
		cells := screenWidthOf(e.tabLabel(index))
		if drawn > 0 || left != "" {
			cells++
		}

		return cells
	}

	fill := func(left string) (count, used int) {
		rest := width
		if left != "" {
			rest -= screenWidthOf(left) + 1 // 표시와 그 뒤 구분선
		}

		for i := scroll; i < len(e.buffers); i++ {
			next := cost(i, count, left)
			if used+next > rest {
				break
			}

			used += next
			count++
		}

		// 오른쪽 표시 자리를 만드느라 tab 을 물린다. 마지막 하나는 물리지 않는다.
		for count > 1 && scroll+count < len(e.buffers) {
			hidden := len(e.buffers) - scroll - count
			if used+1+screenWidthOf(fmt.Sprintf("%d>", hidden)) <= rest {
				break
			}

			count--
			used -= cost(scroll+count, count, left)
		}

		return count, used
	}

	count, used := fill(window.left)

	// tab 이 하나도 안 들어가면 왼쪽 표시를 떼고 그 자리를 tab 에 준다.
	if count == 0 && window.left != "" {
		window.left = ""
		count, used = fill("")
	}

	// 그래도 안 들어갈 만큼 좁으면 잘려도 하나는 그린다. 활성 tab 이 아예 사라지는 것보다 낫다.
	if count == 0 {
		count = 1
	}

	window.count = count

	if hidden := len(e.buffers) - scroll - count; hidden > 0 {
		right := fmt.Sprintf("%d>", hidden)
		rest := width - used
		if window.left != "" {
			rest -= screenWidthOf(window.left) + 1
		}

		if 1+screenWidthOf(right) <= rest {
			window.right = right
		}
	}

	return window
}

// renderTabline 은 편집 영역 맨 위 한 줄을 그린다. 열린 파일과 지금 보고 있는 것을 보여준다.
//
// 보고 있는 tab 만 편집 내용과 같은 색이고 나머지는 반전이다. vim 의 TabLine/TabLineSel 과 같다.
// 활성 tab 이 아래 내용과 이어져 보이는 것이 tab 이라는 비유 자체다.
//
// width 는 화면 너비가 아니라 편집 영역 너비다. sidebar 가 열려 있으면 그만큼 좁다.
// 다 그릴 수 없으면 tabScroll 자리부터 그리고 남은 것은 양끝의 `<n`·`n>` 이 알린다(ADR-0029).
// 두 표시는 줄의 양 끝에 붙고 그 사이에 남는 칸은 잘린 tab 자리라 점으로 채운다.
func (e editor) renderTabline(width int) tablineRow {
	row := tablineRow{tabs: make([][2]int, len(e.buffers))}

	line := strings.Builder{}
	col := 0

	// put 은 남은 칸만큼만 쓰고 그린 자리를 돌려준다. 넘치는 부분은 버린다.
	// 색을 입힌 뒤에는 escape 가 섞여서 폭을 셀 수 없으므로 자르는 것이 먼저다.
	put := func(text string, active bool) [2]int {
		if width > 0 {
			text = text[:offsetAtScreenCol([]byte(text), width-col)]
		}
		if text == "" {
			return [2]int{}
		}

		start := col
		if active {
			line.WriteString(text)
		} else {
			line.WriteString(reverse.Render(text))
		}
		col += screenWidthOf(text)

		return [2]int{start, col}
	}

	scroll := min(max(e.tabScroll, 0), len(e.buffers)-1)
	window := e.layoutTabs(scroll, width)

	if window.left != "" {
		row.left = put(window.left, false)
	}

	for i := scroll; i < scroll+window.count; i++ {
		// 이미 그린 것이 있으면 그 사이를 가른다. 가려짐 표시와 tab 사이도 같다.
		if col > 0 {
			put(e.boxChars.vertical, false)
		}

		row.tabs[i] = put(e.tabLabel(i), i == e.active)
	}

	// 오른쪽 표시는 줄 맨 끝에 붙인다. 마지막 tab 뒤에 두면 tab 이름 길이에 따라 자리가 달라져서
	// 누를 때마다 다른 칸을 겨눠야 한다. 왼쪽 표시가 늘 0 칸인 것과 짝이 맞는다.
	tail := 0
	if window.right != "" {
		tail = 1 + screenWidthOf(window.right) // 구분선과 표시
	}

	// 그 사이에 남는 칸은 통째로 들어가지 못한 다음 tab 의 자리다. 점으로 채워 거기서 잘렸다고 알린다.
	// 한 칸뿐이면 구분선만 남아 오른쪽 표시의 구분선과 붙어 버리므로 비운다.
	// 남은 칸도 채워야 줄 전체가 한 덩어리로 보인다.
	if gap := width - col - tail; gap > 0 {
		hidden := len(e.buffers) - scroll - window.count
		if hidden > 0 && gap >= 2 && col > 0 {
			put(e.boxChars.vertical, false)
			put(strings.Repeat(".", gap-1), false)
		} else {
			put(strings.Repeat(" ", gap), false)
		}
	}

	if window.right != "" {
		if col > 0 {
			put(e.boxChars.vertical, false)
		}

		row.right = put(window.right, false)
	}

	row.line = line.String()

	return row
}

// scrollTabsTo 는 활성 tab 이 tabline 에 온전히 보이도록 tabScroll 을 맞춘다.
// 보고 있는 tab 이 바뀌거나 편집 영역 너비가 바뀌는 자리가 부른다. buffer 의 scrollTo 와 같다.
//
// 최소한만 민다. 화면 밖으로 나간 만큼만 따라가야 tabline 이 덜 흔들린다.
// 뒤쪽이 남아 도는 것도 당긴다 — tab 을 닫거나 화면이 넓어져 오른쪽에 빈 칸이 생기면
// 왼쪽에 가려둔 것을 도로 보여준다.
func (e *editor) scrollTabsTo() {
	width := e.textWidth()

	e.tabScroll = min(max(e.tabScroll, 0), len(e.buffers)-1)

	if e.tabScroll > e.active {
		e.tabScroll = e.active
	}

	// 한 칸씩 미는 것은 tab 마다 폭이 달라서다. 몇 개를 밀면 되는지 셈으로 알 수 없다.
	for e.tabScroll < e.active && e.active >= e.tabScroll+e.layoutTabs(e.tabScroll, width).count {
		e.tabScroll++
	}

	for e.tabScroll > 0 && e.tabScroll-1+e.layoutTabs(e.tabScroll-1, width).count >= len(e.buffers) {
		e.tabScroll--
	}
}
