package textarea

// 커서를 옮기는 것들이다. **화면 높이를 모른다** — 높이를 아는 이동은 buffer-screen.go 에 있다.
//
// # 논리 줄과 화면 행
//
// 이 편집기는 가로 스크롤 대신 줄바꿈으로 보여준다. 그래서 **줄 하나가 화면 행 여럿이 된다.**
//
//	buf.lines[7] = "아주 긴 한 줄이라 화면 폭에서 끊긴다…"
//
//	┌──────────────────────┐
//	│ 아주 긴 한 줄이라 화 │ ← 화면 행 0  ┐
//	│ 면 폭에서 끊긴다…    │ ← 화면 행 1  ┴ 논리 줄 하나
//	└──────────────────────┘
//
//   - **논리 줄**은 `buf.lines` 의 한 칸이다. `cursorLine` 이 그 index 이고, 파일에 든 줄과 같다.
//   - **화면 행**은 그 줄이 폭에서 끊긴 한 토막이다. `wrapOffsets` 가 끊는 자리를 내고
//     폭이 바뀌면 개수가 달라진다 — 파일에는 없는 것이라 담아 두지 않는다.
//
// **이름의 Line·Row 가 이 둘이다.** 저장소 전체가 이 두 낱말만 쓴다 — 「물리 줄」·「스크린 행」
// 같은 다른 말은 두지 않는다.
//
//	         논리 줄(Line)                    화면 행(Row)
//	위아래   j k    moveUpLine·moveDownLine   ↑ ↓      moveUpRow·moveDownRow
//	양끝     0 $    moveLineStart·moveLineEnd home end moveRowStart·moveRowEnd
//
// 가르는 규칙은 **글자 키는 논리 줄, 화살표와 특수 키는 눈에 보이는 행**이다. vim 이 j/k 와
// gj/gk 로 나눈 것을 키 모양으로 나눈 것이다 (ADR-0006, ADR-0076).
//
// 접히지 않은 줄에서는 양쪽이 같은 자리를 낸다. 그래서 **가름이 드러나는 것은 wrap 된 줄에서뿐**
// 이고, 시험도 그 줄을 일부러 만들어 본다(home-end-page_test.go).
//
// **넷 다 이름에 Row 나 Line 을 단다.** 접미 없는 `moveUp`·`moveDown` 이 화면 행 쪽이던 때가
// 있었는데, 이름 하나에 축이 둘(행이냐 줄이냐, count 를 받느냐) 겹쳐 있어서 `moveRowStart` 를
// 본 눈이 `moveUp` 을 줄로 읽었다. 넷 다 count 를 받게 하고 접미를 붙여 축 하나만 남겼다 (ADR-0081).

// prevOffset 은 현재 줄에서 offset 직전 글자의 시작을 돌려준다.
func (viewport Viewport) prevOffset(offset int) int {
	if offset == 0 {
		return 0
	}

	Line := viewport.Lines[viewport.Cursor.Line]
	offsets := WrapOffsets(Line, viewport.ContentWidth(), viewport.TabWidth())
	Row := rowIndexAt(offsets, offset)

	// 행 시작에서 왼쪽으로 가면 앞 행의 마지막 글자다.
	from := offsets[Row]
	if from == offset && Row > 0 {
		from = offsets[Row-1]
	}

	return PrevGlyphStart(Line, from, offset)
}

// clampToNormal 은 커서를 마지막 글자 위로 끌어온다.
//
// normal mode 의 커서는 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
// 그 칸은 insert mode 에서만 갈 수 있다. 빈 줄은 그대로 0 이다.
//
// desiredX 는 건드리지 않는다. 짧은 줄을 지나가도 원래 칸으로 돌아와야 한다.
func (viewport *Viewport) ClampToNormal() {
	Line := viewport.Lines[viewport.Cursor.Line]
	if len(Line) == 0 || viewport.Cursor.Col < len(Line) {
		return
	}

	viewport.Cursor.Col = viewport.prevOffset(len(Line))
}

// moveLeft, moveRight 는 grapheme cluster 단위로 n 글자 움직인다.
// rune 단위로 움직이면 결합 문자의 중간에 커서가 선다.
//
// 줄 양끝에 닿으면 남은 횟수를 버리고 거기서 멈춘다. vim 처럼 앞뒤 줄로 넘어가지 않는다.
func (viewport *Viewport) MoveLeft(n int) {
	if viewport.Cursor.Col == 0 {
		return
	}

	for range n {
		if viewport.Cursor.Col == 0 {
			break
		}

		viewport.Cursor.Col = viewport.prevOffset(viewport.Cursor.Col)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) MoveRight(n int) {
	Line := viewport.Lines[viewport.Cursor.Line]
	if viewport.Cursor.Col >= len(Line) {
		return
	}

	for range n {
		if viewport.Cursor.Col >= len(Line) {
			break
		}

		viewport.Cursor.Col += GlyphSize(Line, viewport.Cursor.Col)
	}

	viewport.UpdateDesiredCol()
}

// updateDesiredCol 은 좌우로 움직인 뒤 유지할 화면 칸을 갱신한다.
// 화면 행 안에서의 칸이라 wrap 된 줄에서도 위아래 이동이 보이는 대로 움직인다.
func (viewport *Viewport) UpdateDesiredCol() {
	Line := viewport.Lines[viewport.Cursor.Line]
	offsets := WrapOffsets(Line, viewport.ContentWidth(), viewport.TabWidth())
	Start, _ := rowRange(Line, offsets, rowIndexAt(offsets, viewport.Cursor.Col))

	viewport.desiredX = ScreenColAt(Line[Start:], viewport.Cursor.Col-Start, viewport.TabWidth())
}

// moveUpRow, moveDownRow 는 **화면 행** 단위로 n 개 움직인다. `↑`·`↓` 다.
// wrap 된 줄에서는 같은 줄 안에서 행만 옮긴다.
//
// 논리 줄 단위인 moveUpLine·moveDownLine 과 짝이다 — 이름의 Row·Line 이 그 갈림이고,
// 넷 다 n 을 받아서 이름에 남은 축은 그 하나뿐이다 (ADR-0006, ADR-0081).
//
// **한 걸음씩 되풀이한다.** 논리 줄 쪽은 줄 index 를 더하면 끝나지만 화면 행은 줄마다 몇
// 개인지가 달라서, 한 번에 셈할 수가 없다 — 지나는 줄을 다 재야 안다.
//
// desiredX 는 건드리지 않는다. 짧은 행을 지나가도 원래 칸으로 돌아오는 것이 그 필드의 목적이다.
func (viewport *Viewport) MoveUpRow(n int) {
	for range n {
		viewport.moveUpRowOnce()
	}
}

func (viewport *Viewport) MoveDownRow(n int) {
	for range n {
		viewport.moveDownRowOnce()
	}
}

func (viewport *Viewport) moveUpRowOnce() {
	offsets := WrapOffsets(viewport.Lines[viewport.Cursor.Line], viewport.ContentWidth(), viewport.TabWidth())

	if row := rowIndexAt(offsets, viewport.Cursor.Col); row > 0 {
		viewport.placeCursorInRow(ViewTop{Line: viewport.Cursor.Line, Row: row - 1}, offsets)
		return
	}
	if viewport.Cursor.Line == 0 {
		return
	}

	prev := viewport.Cursor.Line - 1
	prevOffsets := WrapOffsets(viewport.Lines[prev], viewport.ContentWidth(), viewport.TabWidth())
	viewport.placeCursorInRow(ViewTop{Line: prev, Row: len(prevOffsets) - 1}, prevOffsets)
}

func (viewport *Viewport) moveDownRowOnce() {
	offsets := WrapOffsets(viewport.Lines[viewport.Cursor.Line], viewport.ContentWidth(), viewport.TabWidth())

	if row := rowIndexAt(offsets, viewport.Cursor.Col); row+1 < len(offsets) {
		viewport.placeCursorInRow(ViewTop{Line: viewport.Cursor.Line, Row: row + 1}, offsets)
		return
	}
	if viewport.Cursor.Line+1 >= len(viewport.Lines) {
		return
	}

	next := viewport.Cursor.Line + 1
	viewport.placeCursorInRow(ViewTop{Line: next}, WrapOffsets(viewport.Lines[next], viewport.ContentWidth(), viewport.TabWidth()))
}

// moveLineStart, moveLineFirstNonBlank 는 줄 안에서 왼쪽으로 간다. vim 의 0, ^ 다.
func (viewport *Viewport) MoveLineStart() {
	viewport.Cursor.Col = 0
	viewport.UpdateDesiredCol()
}

// moveLineFirstNonBlank 는 들여쓰기를 건너뛴 첫 글자로 간다.
// 공백뿐인 줄은 줄 끝이 되고, clampToNormal 이 마지막 글자 위로 끌어온다. vim 과 같다.
func (viewport *Viewport) MoveLineFirstNonBlank() {
	Line := viewport.Lines[viewport.Cursor.Line]

	Col := 0
	for Col < len(Line) && (Line[Col] == ' ' || Line[Col] == '\t') {
		Col++
	}

	viewport.Cursor.Col = Col
	viewport.UpdateDesiredCol()
}

// moveLineEnd 는 줄 끝으로 간다. vim 의 $ 다.
// count 는 되풀이가 아니라 줄 수다 — `3$` 는 두 줄 아래의 줄 끝이다.
func (viewport *Viewport) MoveLineEnd(n int) {
	viewport.Cursor.Line = min(viewport.Cursor.Line+n-1, len(viewport.Lines)-1)
	viewport.Cursor.Col = len(viewport.Lines[viewport.Cursor.Line])
	viewport.UpdateDesiredCol()
}

// moveRowStart, moveRowEnd 는 **화면 행** 안에서 양끝으로 간다. `home` 과 `end` 다.
//
// 줄 단위(`0`·`$`) 와 갈리는 자리다 — wrap 된 긴 줄에서 이 둘은 지금 보고 있는 행의 끝까지만
// 간다. `↑`·`↓` 가 화면 행을 세는 것과 같은 가름이다(ADR-0006): 글자 키는 논리 줄이고
// 화살표와 특수 키는 눈에 보이는 행이다. 접히지 않은 줄에서는 `0`·`$` 와 같은 자리다.
//
// 행의 끝은 **다음 행이 시작하는 자리 바로 앞**이다. 그 자리에 서면 다음 글자가 다음 행
// 첫 칸이라, 줄 끝에서 `$` 가 서는 자리(줄 길이) 와 결이 같다 — normal 에서는
// clampToNormal 이 마지막 글자 위로 끌어온다.
func (viewport *Viewport) MoveRowStart() {
	offsets := WrapOffsets(viewport.Lines[viewport.Cursor.Line], viewport.ContentWidth(), viewport.TabWidth())
	Start, _ := rowRange(viewport.Lines[viewport.Cursor.Line], offsets, rowIndexAt(offsets, viewport.Cursor.Col))

	viewport.Cursor.Col = Start
	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) MoveRowEnd() {
	Line := viewport.Lines[viewport.Cursor.Line]
	offsets := WrapOffsets(Line, viewport.ContentWidth(), viewport.TabWidth())
	_, End := rowRange(Line, offsets, rowIndexAt(offsets, viewport.Cursor.Col))

	viewport.Cursor.Col = End
	viewport.UpdateDesiredCol()
}

// moveToLine 은 그 줄의 첫 글자로 간다. vim 의 gg, G 다. 범위를 넘으면 양끝으로 맞춘다.
func (viewport *Viewport) MoveToLine(Line int) {
	viewport.Cursor.Line = min(max(Line, 0), len(viewport.Lines)-1)
	viewport.MoveLineFirstNonBlank()
}

// moveUpLine, moveDownLine 은 논리 줄 단위로 움직인다.
// wrap 되어 화면 행이 여럿인 줄도 한 번에 건넌다. vim 의 j/k 다.
//
// 화면 행 단위인 moveUpRow/moveDownRow 와 나뉜다. vim 에서 j/k 와 gj/gk 가 나뉜 것과 같다(ADR-0006).
// 줄 index 를 직접 옮기므로 반복하지 않고 한 번에 끝난다 — 그쪽이 되풀이하는 까닭은 거기 적었다.
func (viewport *Viewport) MoveUpLine(n int) {
	viewport.placeCursorInLine(max(viewport.Cursor.Line-n, 0))
}

func (viewport *Viewport) MoveDownLine(n int) {
	viewport.placeCursorInLine(min(viewport.Cursor.Line+n, len(viewport.Lines)-1))
}

// placeCursorInLine 은 커서를 그 줄의 desiredX 칸에 놓는다.
//
// **updateDesiredCol 을 부르지 않는다. 그것이 이 함수의 요점이다.** 커서를 옮기는 자리는
// 대개 desiredX 를 새로 정하는데(37 곳) 여기와 placeCursorInRow 둘만 반대로 **기억한 것을
// 읽어 쓴다.** 긴 줄에서 `j` 로 짧은 줄을 지나 다시 `j` 를 쳤을 때 원래 칸으로 돌아오는 것이
// 그 덕이다 — 여기서 새로 정하면 짧은 줄의 끝이 기억이 되어 다시는 못 돌아온다.
//
// 그래서 이름이 닮은 moveToLine 과 합칠 수 없다. 그쪽은 첫 비공백으로 가며 desiredX 를
// 새로 정하는 `gg`·`G` 쪽이다(ADR-0100).
//
// **desiredX 는 화면 행 안에서 센 칸이다**(ADR-0108). 그 값이 늘 본문 폭보다 작으므로,
// wrap 된 줄의 둘째 행 이후에서 넘어오면 여기가 놓는 자리는 **언제나 다음 줄의 첫 화면 행**
// 이다. vim 은 이 칸을 줄 시작에서 세므로 그 자리에서 다르다. wrap 되지 않은 줄과 첫 화면
// 행에서는 두 틀이 같은 값이라 vim 과 같다.
//
// 그대로 두기로 정했다. `↑`/`↓` 가 화면 행 단위인 것과 앞뒤가 맞고(ADR-0006, ADR-0076),
// 줄 기준으로 바꾸면 placeCursorInRow 가 desiredX 를 갱신하게 되어 「기억한 것을 읽어
// 쓰는 자리」와 「새로 정하는 자리」의 가름이 흐려진다(ADR-0108 §3).
func (viewport *Viewport) placeCursorInLine(Line int) {
	viewport.Cursor.Line = Line
	viewport.Cursor.Col = OffsetAtScreenCol(viewport.Lines[Line], viewport.desiredX, viewport.TabWidth())
}

// placeCursorInRow 는 커서를 at 화면 행의 desiredX 칸으로 옮긴다.
// offsets 는 at.Line 의 wrap 자리다. 부르는 쪽이 이미 재 두었으므로 다시 재지 않는다.
func (viewport *Viewport) placeCursorInRow(at ViewTop, offsets []int) {
	start, end := rowRange(viewport.Lines[at.Line], offsets, at.Row)

	// 행 끝 칸을 넘어가면 다음 행의 시작 offset 이 되어 한 행을 더 내려간 것처럼 보인다.
	width := viewport.ContentWidth()

	col := viewport.desiredX
	if width > 0 && at.Row+1 < len(offsets) {
		col = min(col, width-1)
	}

	viewport.Cursor.Line = at.Line
	viewport.Cursor.Col = start + OffsetAtScreenCol(viewport.Lines[at.Line][start:end], col, viewport.TabWidth())
}
