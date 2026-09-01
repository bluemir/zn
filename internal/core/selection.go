package core

// selection 은 visual mode 가 고른 범위의 반대쪽 끝(anchor) 이다. 이쪽 끝은 커서라,
// 이동 키가 커서를 옮기면 범위가 그만큼 따라 자란다.
//
// **viewport 가 든다.** 이 좌표는 그 Buffer 의 lines 안에서만 뜻이 있고, 커서·스크롤과 같이
// tab 을 오가도 파일에 붙어 있어야 한다. register 와 마지막 검색이 editor 에 있는 것은
// 그 둘이 tab 을 넘기 때문이고(spec.md) 이것은 넘지 않는다.
//
// mode model(viewEditorVisual) 에 두지 않은 것은 동작이 `run(e *editor)` 만 받기 때문이다
// (ADR-0034, ADR-0037).
type selection struct {
	active   bool
	linewise bool // `V` 로 연 것인가
	line     int  // anchor 의 줄
	col      int  // anchor 의 byte offset
}

// startSelection 은 커서 자리를 anchor 로 삼아 범위를 연다. `v` `V` 와 드래그가 여기로 온다.
func (e *editor) startSelection(linewise bool) {
	e.activeBuffer().startSelection(linewise)
}
