package textarea

// 화면을 보는 것들이다. `top`·`topRow` 를 옮기거나 화면 좌표와 오가고, 하나같이 height 를 받는다 —
// Buffer 메서드 110 개 중 높이를 아는 것이 여기 든 열뿐이다.
//
// **화면 분할이 들어오면 커서·top·selection 과 함께 Buffer 밖으로 나갈 짐이 이 파일이다**
// (Buffer 주석, ADR-0080 뒤의 docs/tasks.md).
//
// 여기서 세는 단위는 논리 줄이 아니라 **화면 행**이다. `top` 이 논리 줄 index 이고 `topRow` 가
// 그 줄의 몇 번째 행부터 그리는지라, 둘이 짝이어야 자리 하나가 정해진다.
// 두 낱말의 뜻은 buffer-move.go 머리글에 있다.
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-screen.go 에 있다 (ADR-0121).

// scrollTo 는 커서가 화면 안에 들어오도록 top 을 최소한으로 움직인다.
// 커서가 이미 화면 안이면 아무것도 하지 않는다.
func (viewport *Viewport) ScrollTo(Height int) {
	if Height < 1 {
		return
	}

	viewport.clampTop()

	Width := viewport.ContentWidth()

	cursorRow := rowIndexAt(WrapOffsets(viewport.Lines[viewport.Cursor.Line], Width, viewport.TabWidth()), viewport.Cursor.Col)

	// 커서에서 height-1 행 위로 올라간 지점이 top 의 하한이다.
	// 뒤에서 앞으로 세기 때문에 화면 높이만큼만 훑는다.
	limitLine, limitRow := viewport.retreatRows(viewport.Cursor.Line, cursorRow, Height-1, Width)
	if rowBefore(viewport.Top.Line, viewport.Top.Row, limitLine, limitRow) {
		viewport.Top.Line, viewport.Top.Row = limitLine, limitRow
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
	for range stickyMaxRows(Height) + 1 {
		margin := len(viewport.StickyAt(viewport.Top.Line, Height))

		wantLine, wantRow := viewport.retreatRows(viewport.Cursor.Line, cursorRow, margin, Width)
		if !rowBefore(wantLine, wantRow, viewport.Top.Line, viewport.Top.Row) {
			return
		}

		viewport.Top.Line, viewport.Top.Row = wantLine, wantRow
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
func (viewport *Viewport) clampTop() {
	viewport.Top.Line = min(viewport.Top.Line, len(viewport.Lines)-1)
	viewport.Top.Row = min(viewport.Top.Row, len(WrapOffsets(viewport.Lines[viewport.Top.Line], viewport.ContentWidth(), viewport.TabWidth()))-1)
}

// scrollBy 는 화면을 n 행 굴린다. 위로 굴릴 때는 n 이 음수다.
//
// 커서는 그대로 두고 화면만 움직인다. 화면 밖으로 밀려나면 그때만 화면 안 끝 행으로
// 끌어온다 — vim 의 휠과 같다. scrollTo 가 커서를 따라 화면을 옮기는 것의 반대다.
func (viewport *Viewport) ScrollBy(n, Height int) {
	if Height < 1 || n == 0 {
		return
	}

	// 폭이 바뀐 뒤일 수 있다. scrollTo 와 같은 이유로 여기서 한 번 맞춘다.
	viewport.clampTop()

	Width := viewport.ContentWidth()

	if n < 0 {
		viewport.Top.Line, viewport.Top.Row = viewport.retreatRows(viewport.Top.Line, viewport.Top.Row, -n, Width)
	} else {
		viewport.Top.Line, viewport.Top.Row = viewport.advanceRows(viewport.Top.Line, viewport.Top.Row, n, Width)
	}

	// 커서가 아직 화면 안이고 머리줄 아래면 건드릴 것이 없다.
	// 머리줄이 없으면 sticky 가 0 이라 예전과 같은 물음이다(ADR-0049).
	sticky := len(viewport.StickyAt(viewport.Top.Line, Height))
	if _, y, ok := viewport.CursorScreenPos(Height); ok && y >= sticky {
		return
	}

	// 화면 밖으로 밀려났거나 머리줄에 덮였다. 밀려난 쪽 끝 행으로 데려온다.
	//
	// 아래로 굴리면(n>0) 화면이 커서를 지나쳐 내려가므로 커서는 화면 위로 벗어난다 —
	// 맨 윗줄로 데려온다. 위로 굴리면 그 반대다.
	//
	// 맨 윗줄이 아니라 **머리줄 바로 아래 행**이다. 맨 윗줄은 머리줄이 덮고 있어서, 거기에
	// 두면 커서가 커서 줄이 아닌 글자 위에 선다. 머리줄이 없으면 sticky 가 0 이라 맨 윗줄이다.
	Line, Row := viewport.advanceRows(viewport.Top.Line, viewport.Top.Row, sticky, Width)
	if n < 0 {
		Line, Row = viewport.advanceRows(viewport.Top.Line, viewport.Top.Row, Height-1, Width)
	}

	// 칸은 desiredX 를 살린다. j/k 로 그 행에 온 것과 같은 자리에 선다.
	viewport.placeCursorInRow(Line, WrapOffsets(viewport.Lines[Line], Width, viewport.TabWidth()), Row)
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
func (viewport *Viewport) MovePage(direction PageDirection, span PageSpan, count, Height int) {
	if Height < 1 {
		return
	}

	rows := PageRows(span, Height) * max(count, 1)

	// 폭이 바뀐 뒤일 수 있다. scrollBy 와 같은 이유로 여기서 한 번 맞춘다.
	viewport.clampTop()

	Width := viewport.ContentWidth()

	// 커서도 화면 행으로 옮긴다. 되풀이해 한 행씩 가는 moveUp/moveDown 을 쓰지 않는 것은
	// 저쪽이 파일 끝에 닿아도 남은 횟수를 다 도는데, 여기서는 그 횟수가 숫자 곱 한 화면이라
	// 커질 수 있어서다. advanceRows·retreatRows 는 끝에서 곧바로 돌아온다.
	cursorRow := rowIndexAt(WrapOffsets(viewport.Lines[viewport.Cursor.Line], Width, viewport.TabWidth()), viewport.Cursor.Col)

	if direction == PageUp {
		viewport.Top.Line, viewport.Top.Row = viewport.retreatRows(viewport.Top.Line, viewport.Top.Row, rows, Width)

		Line, Row := viewport.retreatRows(viewport.Cursor.Line, cursorRow, rows, Width)
		viewport.placeCursorInRow(Line, WrapOffsets(viewport.Lines[Line], Width, viewport.TabWidth()), Row)

		viewport.ScrollTo(Height)

		return
	}

	viewport.Top.Line, viewport.Top.Row = viewport.advanceRows(viewport.Top.Line, viewport.Top.Row, rows, Width)

	Line, Row := viewport.advanceRows(viewport.Cursor.Line, cursorRow, rows, Width)
	viewport.placeCursorInRow(Line, WrapOffsets(viewport.Lines[Line], Width, viewport.TabWidth()), Row)

	// 파일 끝을 지나서까지 굴리지 않는다. 마지막 행이 화면 맨 아래에 오는 자리가 끝이고
	// 거기서부터는 커서만 내려간다 — vim 의 `ctrl+d`·`ctrl+f` 와 같다. 휠에는 이 한계가
	// 없어서 마지막 줄을 화면 맨 위까지 올릴 수 있는데(vim 의 `ctrl+e`), 이동 키는 눌러도
	// 아무것도 새로 보이지 않는 빈 행을 만들지 않는다.
	lastLine := len(viewport.Lines) - 1
	lastRow := len(WrapOffsets(viewport.Lines[lastLine], Width, viewport.TabWidth())) - 1

	limitLine, limitRow := viewport.retreatRows(lastLine, lastRow, Height-1, Width)
	if rowBefore(limitLine, limitRow, viewport.Top.Line, viewport.Top.Row) {
		viewport.Top.Line, viewport.Top.Row = limitLine, limitRow
	}

	viewport.ScrollTo(Height)
}

// visibleRows 는 화면에 그릴 행들을 위에서부터 돌려준다.
func (viewport Viewport) VisibleRows(Height int) []ScreenRow {
	if Height < 1 {
		return nil
	}

	rows := make([]ScreenRow, 0, Height)
	Line, Row := viewport.Top.Line, viewport.Top.Row

	for len(rows) < Height && Line < len(viewport.Lines) {
		offsets := WrapOffsets(viewport.Lines[Line], viewport.ContentWidth(), viewport.TabWidth())
		if Row >= len(offsets) {
			Line, Row = Line+1, 0
			continue
		}

		Start, End := rowRange(viewport.Lines[Line], offsets, Row)
		rows = append(rows, ScreenRow{Line: Line, Start: Start, End: End})
		Row++
	}

	return rows
}

// cursorScreenPos 는 커서의 화면 좌표를 돌려준다. 커서가 화면 밖이면 ok 가 false 다.
func (viewport Viewport) CursorScreenPos(Height int) (x, y int, ok bool) {
	Line := viewport.Lines[viewport.Cursor.Line]

	for y, Row := range viewport.VisibleRows(Height) {
		if Row.Line != viewport.Cursor.Line || viewport.Cursor.Col < Row.Start {
			continue
		}
		// 행 경계의 offset 은 앞 행의 끝이 아니라 다음 행의 시작으로 본다.
		// 줄 끝일 때만 마지막 행의 끝에 놓는다.
		if viewport.Cursor.Col > Row.End || (viewport.Cursor.Col == Row.End && Row.End != len(Line)) {
			continue
		}

		return ScreenColAt(Line[Row.Start:Row.End], viewport.Cursor.Col-Row.Start, viewport.TabWidth()), y, true
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
func (viewport Viewport) PositionAt(x, y, Height int) (Line, Col int, ok bool) {
	rows := viewport.VisibleRows(Height)
	if y < 0 || y >= len(rows) {
		return 0, 0, false
	}

	Row := rows[y]

	// 행 안에서 잘라서 센다. tab 이 다음 tab stop 까지 벌어지는 기준이 논리 줄이 아니라
	// 화면 행의 시작이라(wrapOffsets 주석) 줄을 통째로 넘기면 tab 으로 들여쓴 줄에서 어긋난다.
	return Row.Line, Row.Start + OffsetAtScreenCol(viewport.Lines[Row.Line][Row.Start:Row.End], max(0, x), viewport.TabWidth()), true
}

// stickyMaxRows 는 머리줄이 먹을 수 있는 최대 행 수다. 편집 영역의 절반이다.
//
// **설정이 아니라 자물쇠다.** 이 편집기에는 설정이 없고(README) 머리줄은 늘 켜져 있다.
// 그래서 깊게 중첩된 코드에서 감싸는 것이 열 겹일 때 화면이 통째로 머리줄이 되어 정작
// 보려던 줄이 남지 않는 일을 막을 자리가 여기밖에 없다. 절반은 「본문이 머리줄보다 적어지지
// 않는다」는 뜻이다.
//
// 40 행 화면에서 20 겹이 필요하므로 실제로 걸릴 일은 거의 없다. 그래서 정책이 아니라 자물쇠다.
func stickyMaxRows(Height int) int {
	return Height / 2
}
