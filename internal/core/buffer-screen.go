package core

import (
	"slices"
)

// 화면을 보는 것들이다. `top`·`topRow` 를 옮기거나 화면 좌표와 오가고, 하나같이 height 를 받는다 —
// Buffer 메서드 110 개 중 높이를 아는 것이 여기 든 열뿐이다.
//
// **화면 분할이 들어오면 커서·top·selection 과 함께 Buffer 밖으로 나갈 짐이 이 파일이다**
// (Buffer 주석, ADR-0080 뒤의 docs/tasks.md).
//
// 여기서 세는 단위는 논리 줄이 아니라 **화면 행**이다. `top` 이 논리 줄 index 이고 `topRow` 가
// 그 줄의 몇 번째 행부터 그리는지라, 둘이 짝이어야 자리 하나가 정해진다.
// 두 낱말의 뜻은 buffer-move.go 머리글에 있다.

// scrollTo 는 커서가 화면 안에 들어오도록 top 을 최소한으로 움직인다.
// 커서가 이미 화면 안이면 아무것도 하지 않는다.
func (buf *Buffer) scrollTo(width, height int) {
	if height < 1 {
		return
	}

	buf.clampTop(width)

	cursorRow := rowIndexAt(wrapOffsets(buf.lines[buf.cursorLine], width, buf.tabWidth()), buf.cursorCol)

	// 커서에서 height-1 행 위로 올라간 지점이 top 의 하한이다.
	// 뒤에서 앞으로 세기 때문에 화면 높이만큼만 훑는다.
	limitLine, limitRow := buf.retreatRows(buf.cursorLine, cursorRow, height-1, width)
	if rowBefore(buf.top, buf.topRow, limitLine, limitRow) {
		buf.top, buf.topRow = limitLine, limitRow
	}

	// 위쪽은 sticky 머리줄이 덮는 만큼 더 올라간다(ADR-0049).
	//
	// **한 번으로는 안 맞는다.** 붙는 줄 수는 top 에서 나오고 top 은 그 줄 수에서 나온다.
	// 그리고 단순히 대입하면 **진동한다** — 감싸는 줄 수가 줄 번호에 대해 단조가 아니라
	// top 이 두 값을 오간다.
	//
	// top 이 **위로만 가는 갈래만** 두어 끝낸다. 한 바퀴마다 top 이 최소 한 행 올라가고,
	// top 이 0 이면 감싸는 것이 없어 멈춘다. 바퀴 수를 못 박아 두는 것은 판정이 틀려도
	// 편집기가 멈추지 않게 하는 자물쇠다.
	//
	// 머리줄이 없으면(강조하지 않는 파일) margin 이 0 이라 첫 바퀴가 곧 예전의
	// 「위로 벗어나면 커서 행을 최상단으로」다. 그 갈래를 이것이 대신한다.
	for range stickyMaxRows(height) + 1 {
		margin := len(buf.stickyAt(buf.top, height))

		wantLine, wantRow := buf.retreatRows(buf.cursorLine, cursorRow, margin, width)
		if !rowBefore(wantLine, wantRow, buf.top, buf.topRow) {
			return
		}

		buf.top, buf.topRow = wantLine, wantRow
	}
}

// clampTop 은 top, topRow 를 지금 너비에서 실제로 있는 화면 행으로 맞춘다.
//
// topRow 는 그 줄이 몇 번째 wrap 행부터 그려지는지인데, 화면이 넓어지면 그 줄의 wrap 행 수가
// 줄어서 예전 topRow 가 없는 행을 가리키게 된다. 그대로 두면 visibleRows 가 그 줄을 통째로
// 건너뛰어서 panic 없이 화면이 한 줄씩 밀린다.
//
// 폭이 바뀌는 경로가 여럿(터미널 리사이즈, tab 전환, sidebar 여닫기)이라
// 부르는 쪽마다 챙기지 않고 scrollTo 안에서 한 번에 맞춘다.
func (buf *Buffer) clampTop(width int) {
	buf.top = min(buf.top, len(buf.lines)-1)
	buf.topRow = min(buf.topRow, len(wrapOffsets(buf.lines[buf.top], width, buf.tabWidth()))-1)
}

// retreatRows 는 (line,row) 에서 화면 행 n 개 위로 올라간 위치를 돌려준다.
func (buf Buffer) retreatRows(line, row, n, width int) (int, int) {
	for range n {
		switch {
		case row > 0:
			row--
		case line > 0:
			line--
			row = len(wrapOffsets(buf.lines[line], width, buf.tabWidth())) - 1
		default:
			return 0, 0
		}
	}
	return line, row
}

// advanceRows 는 (line,row) 에서 화면 행 n 개 아래로 내려간 위치를 돌려준다.
// 파일 끝을 넘으면 마지막 줄의 마지막 행에서 멈춘다. retreatRows 의 반대 방향이다.
func (buf Buffer) advanceRows(line, row, n, width int) (int, int) {
	for range n {
		last := len(wrapOffsets(buf.lines[line], width, buf.tabWidth())) - 1

		switch {
		case row < last:
			row++
		case line < len(buf.lines)-1:
			line++
			row = 0
		default:
			return line, row
		}
	}
	return line, row
}

// scrollBy 는 화면을 n 행 굴린다. 위로 굴릴 때는 n 이 음수다.
//
// 커서는 그대로 두고 화면만 움직인다. 화면 밖으로 밀려나면 그때만 화면 안 끝 행으로
// 끌어온다 — vim 의 휠과 같다. scrollTo 가 커서를 따라 화면을 옮기는 것의 반대다.
func (buf *Buffer) scrollBy(n, width, height int) {
	if height < 1 || n == 0 {
		return
	}

	// 폭이 바뀐 뒤일 수 있다. scrollTo 와 같은 이유로 여기서 한 번 맞춘다.
	buf.clampTop(width)

	if n < 0 {
		buf.top, buf.topRow = buf.retreatRows(buf.top, buf.topRow, -n, width)
	} else {
		buf.top, buf.topRow = buf.advanceRows(buf.top, buf.topRow, n, width)
	}

	// 커서가 아직 화면 안이고 머리줄 아래면 건드릴 것이 없다.
	// 머리줄이 없으면 sticky 가 0 이라 예전과 같은 물음이다(ADR-0049).
	sticky := len(buf.stickyAt(buf.top, height))
	if _, y, ok := buf.cursorScreenPos(width, height); ok && y >= sticky {
		return
	}

	// 화면 밖으로 밀려났거나 머리줄에 덮였다. 밀려난 쪽 끝 행으로 데려온다.
	//
	// 아래로 굴리면(n>0) 화면이 커서를 지나쳐 내려가므로 커서는 화면 위로 벗어난다 —
	// 맨 윗줄로 데려온다. 위로 굴리면 그 반대다.
	//
	// 맨 윗줄이 아니라 **머리줄 바로 아래 행**이다. 맨 윗줄은 머리줄이 덮고 있어서, 거기에
	// 두면 커서가 커서 줄이 아닌 글자 위에 선다. 머리줄이 없으면 sticky 가 0 이라 맨 윗줄이다.
	line, row := buf.advanceRows(buf.top, buf.topRow, sticky, width)
	if n < 0 {
		line, row = buf.advanceRows(buf.top, buf.topRow, height-1, width)
	}

	// 칸은 desiredCol 을 살린다. j/k 로 그 행에 온 것과 같은 자리에 선다.
	buf.placeCursorInRow(line, wrapOffsets(buf.lines[line], width, buf.tabWidth()), row, width)
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
	cursorRow := rowIndexAt(wrapOffsets(buf.lines[buf.cursorLine], width, buf.tabWidth()), buf.cursorCol)

	if direction == pageUp {
		buf.top, buf.topRow = buf.retreatRows(buf.top, buf.topRow, rows, width)

		line, row := buf.retreatRows(buf.cursorLine, cursorRow, rows, width)
		buf.placeCursorInRow(line, wrapOffsets(buf.lines[line], width, buf.tabWidth()), row, width)

		buf.scrollTo(width, height)

		return
	}

	buf.top, buf.topRow = buf.advanceRows(buf.top, buf.topRow, rows, width)

	line, row := buf.advanceRows(buf.cursorLine, cursorRow, rows, width)
	buf.placeCursorInRow(line, wrapOffsets(buf.lines[line], width, buf.tabWidth()), row, width)

	// 파일 끝을 지나서까지 굴리지 않는다. 마지막 행이 화면 맨 아래에 오는 자리가 끝이고
	// 거기서부터는 커서만 내려간다 — vim 의 `ctrl+d`·`ctrl+f` 와 같다. 휠에는 이 한계가
	// 없어서 마지막 줄을 화면 맨 위까지 올릴 수 있는데(vim 의 `ctrl+e`), 이동 키는 눌러도
	// 아무것도 새로 보이지 않는 빈 행을 만들지 않는다.
	lastLine := len(buf.lines) - 1
	lastRow := len(wrapOffsets(buf.lines[lastLine], width, buf.tabWidth())) - 1

	limitLine, limitRow := buf.retreatRows(lastLine, lastRow, height-1, width)
	if rowBefore(limitLine, limitRow, buf.top, buf.topRow) {
		buf.top, buf.topRow = limitLine, limitRow
	}

	buf.scrollTo(width, height)
}

// visibleRows 는 화면에 그릴 행들을 위에서부터 돌려준다.
func (buf Buffer) visibleRows(width, height int) []screenRow {
	if height < 1 {
		return nil
	}

	rows := make([]screenRow, 0, height)
	line, row := buf.top, buf.topRow

	for len(rows) < height && line < len(buf.lines) {
		offsets := wrapOffsets(buf.lines[line], width, buf.tabWidth())
		if row >= len(offsets) {
			line, row = line+1, 0
			continue
		}

		start, end := rowRange(buf.lines[line], offsets, row)
		rows = append(rows, screenRow{line: line, start: start, end: end})
		row++
	}

	return rows
}

// cursorScreenPos 는 커서의 화면 좌표를 돌려준다. 커서가 화면 밖이면 ok 가 false 다.
func (buf Buffer) cursorScreenPos(width, height int) (x, y int, ok bool) {
	line := buf.lines[buf.cursorLine]

	for y, row := range buf.visibleRows(width, height) {
		if row.line != buf.cursorLine || buf.cursorCol < row.start {
			continue
		}
		// 행 경계의 offset 은 앞 행의 끝이 아니라 다음 행의 시작으로 본다.
		// 줄 끝일 때만 마지막 행의 끝에 놓는다.
		if buf.cursorCol > row.end || (buf.cursorCol == row.end && row.end != len(line)) {
			continue
		}

		return screenColAt(line[row.start:row.end], buf.cursorCol-row.start, buf.tabWidth()), y, true
	}

	return 0, 0, false
}

// positionAt 은 본문 안 화면 좌표 (x, y) 에 있는 줄과 byte offset 이다.
// 그 자리에 행이 없으면 ok 가 false 다. cursorScreenPos 의 반대 방향이다.
//
// 파일 마지막 줄 아래 빈 자리는 없는 자리로 본다. 마지막 줄로 끌어당기지 않는다 —
// 아무것도 없는 곳을 눌렀는데 커서가 움직이면 어디를 눌렀는지와 어긋난다.
//
// x 가 음수면 줄 시작이다. 줄번호 칸을 누른 경우가 그렇게 들어온다.
func (buf Buffer) positionAt(x, y, width, height int) (line, col int, ok bool) {
	rows := buf.visibleRows(width, height)
	if y < 0 || y >= len(rows) {
		return 0, 0, false
	}

	row := rows[y]

	// 행 안에서 잘라서 센다. tab 이 다음 tab stop 까지 벌어지는 기준이 논리 줄이 아니라
	// 화면 행의 시작이라(wrapOffsets 주석) 줄을 통째로 넘기면 tab 으로 들여쓴 줄에서 어긋난다.
	return row.line, row.start + offsetAtScreenCol(buf.lines[row.line][row.start:row.end], max(0, x), buf.tabWidth()), true
}

// stickyAt 은 line 을 화면 맨 위로 그릴 때 그 위에 붙는 머리줄들이다.
// 바깥쪽이 앞이고 line 자신은 들지 않는다. 강조하지 않는 파일이면 nil 이다.
//
// **기준은 커서가 아니라 화면 맨 윗줄이다.** 그래서 위로 밀려 안 보이게 된 것만 붙는다 —
// 화면에 이미 있는 제목이 위에 한 번 더 그려지지 않고, 파일 첫 화면에서는 아무것도 안 붙어
// 자리를 안 먹는다. VSCode 와 같다.
//
// **한 프레임에 여러 번 불린다.** 그리는 쪽이 부르고, scrollTo 가 수렴하는 동안 화면 높이의
// 절반만큼 더 부른다. 그래서 위로 훑다가 깊이 0 에서 멈춘다 — 파일을 처음부터 다시 읽지 않는다.
func (buf *Buffer) stickyAt(line, height int) []int {
	// 맨 윗줄이면 위에 붙일 것이 없다. 본문이 두 행보다 낮으면 머리줄에 내줄 자리가 없다.
	if line < 1 || line >= len(buf.lines) || height < 2 {
		return nil
	}

	// **여기서 캐시를 채운다.** scrollTo 는 Update 에서 돌고 토큰은 View 에서 채워진다.
	// 채우지 않으면 두 쪽의 답이 한 프레임 어긋나서 커서가 머리줄 아래에 그려진다.
	// receiver 가 포인터인 이유가 이것뿐이다 — editorView 와 같은 손이다(ADR-0039).
	//
	// 위로만 훑으므로 line 까지면 넉넉하다. lexSyntaxTo 는 언제나 0 번 줄부터 내려온다.
	buf.lexSyntaxTo(line)

	// 뼈대 규칙이 없는 언어다. 강조도 같이 없다 — 둘 다 표의 같은 줄에서 오므로 같이 있고
	// 같이 없다(TestOutlineForEveryLanguage 가 지킨다). 표를 통째로 들고 있으니 물음도
	// 하나로 줄었다 — 전에는 「강조하는가」와 「뼈대가 있는가」를 따로 물었다 (ADR-0080).
	rule := buf.language.Outline()
	if rule == nil {
		return nil
	}

	// limit 은 「여기보다 얕아야 바깥이다」다. 지나온 줄 중 가장 얕은 깊이다.
	//
	// **머리줄만이 아니라 지나는 줄마다 이것을 낮춘다.** 이미 닫힌 블록을 걸러내는 것이
	// 그것이다. 머리줄만 보고 낮추면 이런 자리가 틀린다(ADR-0049):
	//
	//	func alpha() {
	//	    if a {
	//	        x()
	//	    }
	//	    s := "text" +
	//	        "more"      <- 여기서 위를 보면 닫힌 `if a {` 가 붙는다
	//
	// 닫는 줄(`}`) 이 그 블록과 같은 깊이라, 지나면서 한계를 낮추면 그 위의 여는 줄이 저절로
	// 걸러진다. 파일 전체의 괄호를 세지 않고 닫힘을 아는 것이 이 한 줄이다.
	limit := rule.Depth(buf.lines[line], buf.syntaxTokens(line))

	heads := []int{}
	for at := line - 1; at >= 0 && limit > 0; at-- {
		depth := rule.Depth(buf.lines[at], buf.syntaxTokens(at))
		if depth >= limit {
			continue
		}

		limit = depth

		// 한계를 낮춘 줄에만 묻는다. 머리줄은 반드시 더 얕으므로 나머지는 물어볼 필요가 없고,
		// 이 물음이 줄을 복제하는 규칙(makeOutline) 이 있어서 값이 싸지 않다.
		if rule.Heads(buf.lines[at], buf.syntaxTokens(at)) {
			heads = append(heads, at)
		}
	}

	if len(heads) < 1 {
		return nil
	}

	// 위로 훑었으므로 안쪽부터 담겼다. 바깥쪽이 앞이다.
	slices.Reverse(heads)

	// 넘치면 **바깥쪽부터 버린다.** 가장 안쪽이 방금 화면 위로 사라진 것이라 잃으면 가장
	// 아깝고, 가장 바깥쪽(`# 제목`, `package`) 은 파일을 열 때부터 알고 있는 것이다.
	if over := len(heads) - stickyMaxRows(height); over > 0 {
		heads = heads[over:]
	}

	return heads
}

// place 는 지금 보고 있는 자리다. 무르는 자리가 이것을 담아 두었다가 moveToPlace 로 되돌린다.
func (buf Buffer) place() viewPlace {
	return viewPlace{
		cursorLine: buf.cursorLine, cursorCol: buf.cursorCol,
		top: buf.top, topRow: buf.topRow,
	}
}

// moveToPlace 는 담아 둔 자리로 커서와 화면을 되돌린다.
//
// **`desiredCol` 을 다시 맞춘다.** 커서를 옮기는 자리라 그 불변이 여기서 끝나야 한다 —
// 밖에서 필드를 직접 쓰면 그 겹이 이것을 같이 져야 하고, 잊으면 되돌린 뒤 `j` 가 엉뚱한
// 칸으로 간다(ADR-0100).
func (buf *Buffer) moveToPlace(at viewPlace, width int) {
	buf.cursorLine, buf.cursorCol = at.cursorLine, at.cursorCol
	buf.top, buf.topRow = at.top, at.topRow
	buf.updateDesiredCol(width)
}
