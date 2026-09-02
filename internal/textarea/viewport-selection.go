package textarea

import "github.com/bluemir/zn/internal/scheme"

// visual mode 가 고른 범위다. anchor 는 Buffer 가 들고 반대쪽 끝은 커서다 (ADR-0037).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-selection.go 에 있다 (ADR-0121).

// SelectionRange 는 고른 범위다. 고른 것이 없으면 false 다.
//
// 양끝 중 앞선 것이 시작이다. 어느 쪽으로 골랐든 범위는 같아서, 지우고 복사하는
// 쪽은 방향을 몰라도 된다.
//
// **끝에 선 글자를 범위에 넣는다.** vim 의 visual 은 inclusive 라 `vd` 가 `x` 와 같다.
// `e` 가 span 에서 includeCursorCluster 를 부르는 것과 같은 자리다(ADR-0013).
func (viewport Viewport) SelectionRange() (scheme.MotionRange, bool) {
	if !viewport.Selection.Active {
		return scheme.MotionRange{}, false
	}

	start, end := viewport.Selection.From, viewport.Selection.To
	if end.Line < start.Line || (end.Line == start.Line && end.Col < start.Col) {
		start, end = end, start
	}

	// **줄 단위여도 칸을 담는다.** 고치는 자리는 줄 전체를 쓰지만(selectionOn) 복사가 커서를
	// 범위의 시작으로 옮길 때 그 칸이 필요하다 — 어느 쪽 끝에서 골랐든 처음 짚은 자리로
	// 돌아가는 것이 vim 의 visual `y` 다(buffer-yank.go 의 moveToRangeStart, ADR-0100).
	if viewport.Selection.Linewise {
		return scheme.MotionRange{Start: start, End: end, Linewise: true}, true
	}

	// 줄 끝에서는 밀 글자가 없다. 빈 줄을 고른 것이라 범위가 비어 있는 그대로다.
	if line := viewport.Lines[end.Line]; end.Col < len(line) {
		end.Col += GlyphSize(line, end.Col)
	}

	return scheme.MotionRange{Start: start, End: end}, true
}

// StartSelection 은 커서 자리에서 범위를 연다. `v`·`V` 와 드래그가 여기로 온다.
// 양끝이 다 커서 자리라 아직 한 글자다.
func (viewport *Viewport) StartSelection(linewise bool) {
	viewport.Selection = Selection{
		Active:   true,
		Linewise: linewise,
		From:     viewport.Cursor,
		To:       viewport.Cursor,
	}
}

// ExtendSelection 은 움직이는 끝을 커서 자리로 늘린다. From 은 그대로다.
//
// **고른 채로 커서를 옮긴 뒤에 부른다.** visual mode 에서 커서를 옮기는 길이 셋이고
// (키·드래그·normal 에서 드래그로 열기) 그 셋이 이것을 부른다. 커서를 쓰는 자리가
// 창 안에 마흔아홉이라 그쪽에 붙일 목구멍이 없다 — `UpdateDesiredCol` 도 서른여덟만 지난다.
//
// **빠뜨리면 시험이 잡는다.** 셋 중 하나를 안 부르면 `internal/core` 의 시험 함수 스물넷이
// 빨개진다 — 키를 쳐서 고르고 지우는 시험이 그만큼 있다.
//
// 고른 것이 없으면 아무 일도 하지 않는다. 부르는 쪽이 mode 를 다시 묻지 않아도 된다.
func (viewport *Viewport) ExtendSelection() {
	if !viewport.Selection.Active {
		return
	}

	viewport.Selection.To = viewport.Cursor
}

// clearSelection 은 고른 범위를 닫는다.
//
// **닫는 자리가 넷이다** — normal 로 나갈 때, insert 로 들어갈 때, 팔레트를 열 때, 클릭할 때.
// 밖에서 필드를 비우면 「비운다」가 무엇인지가 그 넷에 흩어진다(ADR-0100).
func (viewport *Viewport) ClearSelection() {
	viewport.Selection = Selection{}
}

// Selection 은 visual mode 가 고른 범위다. 양끝을 다 든다.
//
// **From 은 처음 짚은 자리이고 To 는 움직이는 끝이다.** 어느 쪽이 앞인지는 정해져 있지 않다 —
// 위로 골랐으면 To 가 From 보다 앞이다. 앞뒤를 맞추는 것은 SelectionRange 가 한다.
//
// **To 를 담는다.** 지금은 그 값이 늘 커서와 같아서 담지 않을 수도 있는데, 그것은 이 편집기가
// 아직 「고르는 중」만 다루기 때문이다. **고른 범위를 되찾는 것(vim 의 `gv`) 이 오면 커서는
// 딴 데 있고 범위만 되살아난다.** 그때 커서에서 To 를 찾아올 길이 없다.
//
// 담지 않으면 이 type 이 「고른 범위」가 아니라 「한쪽 끝」이 되고, 나머지 반쪽을 읽는 쪽이
// 커서에서 찾아 와야 한다. 담는 대신 늘리는 문(ExtendSelection) 을 하나 두었다.
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

	From scheme.Cursor // 처음 짚은 자리. vim 이 anchor 라 부르는 것이다
	To   scheme.Cursor // 움직이는 끝. 이동 키와 드래그가 여기를 옮긴다
}
