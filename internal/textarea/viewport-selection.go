package textarea

import "github.com/bluemir/zn/internal/scheme"

// visual mode 가 고른 범위다. anchor 는 Buffer 가 들고 반대쪽 끝은 커서다 (ADR-0037).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-selection.go 에 있다 (ADR-0121).

// selectionRange 는 고른 범위다. 고른 것이 없으면 false 다.
//
// anchor 와 커서 중 앞선 것이 시작이다. 어느 쪽으로 골랐든 범위는 같아서, 지우고 복사하는
// 쪽은 방향을 몰라도 된다.
//
// **커서가 선 글자를 범위에 넣는다.** vim 의 visual 은 inclusive 라 `vd` 가 `x` 와 같다.
// `e` 가 span 에서 includeCursorCluster 를 부르는 것과 같은 자리다(ADR-0013).
func (buf Viewport) SelectionRange() (scheme.MotionRange, bool) {
	if !buf.Selection.Active {
		return scheme.MotionRange{}, false
	}

	startLine, startCol := buf.Selection.Line, buf.Selection.Col
	endLine, endCol := buf.Cursor.Line, buf.Cursor.Col

	if endLine < startLine || (endLine == startLine && endCol < startCol) {
		startLine, startCol, endLine, endCol = endLine, endCol, startLine, startCol
	}

	// **줄 단위여도 칸을 담는다.** 고치는 자리는 줄 전체를 쓰지만(selectionOn) 복사가 커서를
	// 범위의 시작으로 옮길 때 그 칸이 필요하다 — 어느 쪽 끝에서 골랐든 처음 짚은 자리로
	// 돌아가는 것이 vim 의 visual `y` 다(buffer-yank.go 의 moveToRangeStart, ADR-0100).
	if buf.Selection.Linewise {
		return scheme.MotionRange{Start: scheme.Cursor{Line: startLine, Col: startCol}, End: scheme.Cursor{Line: endLine, Col: endCol}, Linewise: true}, true
	}

	// 줄 끝에서는 밀 글자가 없다. 빈 줄을 고른 것이라 범위가 비어 있는 그대로다.
	if Line := buf.Lines[endLine]; endCol < len(Line) {
		endCol += GlyphSize(Line, endCol)
	}

	return scheme.MotionRange{Start: scheme.Cursor{Line: startLine, Col: startCol}, End: scheme.Cursor{Line: endLine, Col: endCol}}, true
}

// startSelection 은 커서 자리를 anchor 로 삼아 범위를 연다. `v`·`V` 와 드래그가 여기로 온다.
func (buf *Viewport) StartSelection(Linewise bool) {
	buf.Selection = Selection{
		Active:   true,
		Linewise: Linewise,
		Line:     buf.Cursor.Line,
		Col:      buf.Cursor.Col,
	}
}

// clearSelection 은 고른 범위를 닫는다.
//
// **닫는 자리가 넷이다** — normal 로 나갈 때, insert 로 들어갈 때, 팔레트를 열 때, 클릭할 때.
// 밖에서 필드를 비우면 「비운다」가 무엇인지가 그 넷에 흩어진다(ADR-0100).
func (buf *Viewport) ClearSelection() {
	buf.Selection = Selection{}
}

// selection 은 visual mode 가 고른 범위의 반대쪽 끝(anchor) 이다. 이쪽 끝은 커서라,
// 이동 키가 커서를 옮기면 범위가 그만큼 따라 자란다.
//
// **viewport 가 든다.** 이 좌표는 그 Buffer 의 lines 안에서만 뜻이 있고, 커서·스크롤과 같이
// tab 을 오가도 파일에 붙어 있어야 한다. register 와 마지막 검색이 editor 에 있는 것은
// 그 둘이 tab 을 넘기 때문이고(spec.md) 이것은 넘지 않는다.
//
// mode model(viewEditorVisual) 에 두지 않은 것은 동작이 `run(e *editor)` 만 받기 때문이다
// (ADR-0034, ADR-0037).
type Selection struct {
	Active   bool
	Linewise bool // `V` 로 연 것인가
	Line     int  // anchor 의 줄
	Col      int  // anchor 의 byte offset
}
