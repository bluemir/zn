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

// movePage 는 화면과 커서를 한 번에 **같이** 옮긴다. vim 의 `ctrl+d`·`ctrl+u`(반 화면) 와
// `ctrl+f`·`ctrl+b`(한 화면) 다(ADR-0062, ADR-0063).
//
// 휠(scrollBy) 과 갈리는 자리다 — 휠은 화면만 굴리고 커서는 밀려날 때만 따라오는데(ADR-0012),
// 이쪽은 커서가 화면 안 같은 자리에 남아야 한다. 그래서 top 과 커서를 같은 행 수로 옮긴다.
// vim 의 `ctrl+f` 는 커서를 새 화면 첫 행에 놓는데, 그러면 반 화면과 커서 규칙이 갈린다.
//
// 세는 것은 논리 줄이 아니라 **화면 행**이다. 한 화면은 눈에 보이는 만큼이라 wrap 된 긴 줄이
// 화면을 다 차지하면 그 줄 안에서 움직이는 것이 맞다 — `↓` 와 같은 단위다(ADR-0006).
//
// count 는 되풀이다. `3ctrl+f` 는 한 화면 세 번이다.
func (buf *Buffer) movePage(direction pageDirection, span pageSpan, count, width, height int) {
	if height < 1 {
		return
	}

	rows := pageRows(span, height) * max(count, 1)

	// 폭이 바뀐 뒤일 수 있다. scrollBy 와 같은 이유로 여기서 한 번 맞춘다.
	buf.clampTop(width)

	// 커서도 화면 행으로 옮긴다. 되풀이해 한 행씩 가는 moveUp/moveDown 을 쓰지 않는 것은
	// 저쪽이 파일 끝에 닿아도 남은 횟수를 다 도는데, 여기서는 그 횟수가 숫자 곱 한 화면이라
	// 커질 수 있어서다. advanceRows·retreatRows 는 끝에서 곧바로 돌아온다.
	cursorRow := rowIndexAt(wrapOffsets(buf.lines[buf.cursorLine], width), buf.cursorCol)

	if direction == pageUp {
		buf.top, buf.topRow = buf.retreatRows(buf.top, buf.topRow, rows, width)

		line, row := buf.retreatRows(buf.cursorLine, cursorRow, rows, width)
		buf.placeCursorInRow(line, wrapOffsets(buf.lines[line], width), row, width)

		buf.scrollTo(width, height)

		return
	}

	buf.top, buf.topRow = buf.advanceRows(buf.top, buf.topRow, rows, width)

	line, row := buf.advanceRows(buf.cursorLine, cursorRow, rows, width)
	buf.placeCursorInRow(line, wrapOffsets(buf.lines[line], width), row, width)

	// 파일 끝을 지나서까지 굴리지 않는다. 마지막 행이 화면 맨 아래에 오는 자리가 끝이고
	// 거기서부터는 커서만 내려간다 — vim 의 `ctrl+d`·`ctrl+f` 와 같다. 휠에는 이 한계가
	// 없어서 마지막 줄을 화면 맨 위까지 올릴 수 있는데(vim 의 `ctrl+e`), 이동 키는 눌러도
	// 아무것도 새로 보이지 않는 빈 행을 만들지 않는다.
	lastLine := len(buf.lines) - 1
	lastRow := len(wrapOffsets(buf.lines[lastLine], width)) - 1

	limitLine, limitRow := buf.retreatRows(lastLine, lastRow, height-1, width)
	if rowBefore(limitLine, limitRow, buf.top, buf.topRow) {
		buf.top, buf.topRow = limitLine, limitRow
	}

	buf.scrollTo(width, height)
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
