package textarea

import (
	"github.com/bluemir/zn/internal/scheme"
)

// buffer 안에서 패턴을 찾는 것들이다. 패턴을 짓고 화면에 칠하는 것은 search.go 에 있다 (ADR-0042).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-search.go 에 있다 (ADR-0121).

// MoveTo 는 커서를 그 자리로 옮긴다. 검색이 찾은 자리로 뛸 때 쓴다.
// 파일 밖이면 안으로 당긴다.
//
// **Cursor 로 받는다.** `int` 둘로 두면 서명만 보고 그것이 (줄, 칸) 인지 (x, y) 인지 알 수
// 없다. 화면 자리로 옮기는 것은 MoveToCell 이 아니라 PositionAt 을 지나야 한다.
func (viewport *Viewport) MoveTo(at scheme.Cursor) {
	viewport.Cursor.Line = min(max(at.Line, 0), len(viewport.Lines)-1)
	viewport.Cursor.Col = min(max(at.Col, 0), len(viewport.Lines[viewport.Cursor.Line]))
	viewport.UpdateDesiredCol()
}

// wordUnderCursor 는 `*` `#` 가 찾을 단어와 그 단어가 시작하는 자리다.
//
// 커서가 단어 위가 아니면 그 줄에서 오른쪽으로 첫 단어를 찾는다. 줄에 단어가 없으면 false 다.
// vim 과 같다 — 들여쓰기 위에서 눌러도 그 줄의 첫 낱말을 찾아준다.
func (viewport Viewport) WordUnderCursor() (string, int, bool) {
	line := viewport.Lines[viewport.Cursor.Line]

	col := viewport.Cursor.Col
	for col < len(line) && !isWordClass(glyphClass(line, col)) {
		col += GlyphSize(line, col)
	}
	if col >= len(line) {
		return "", 0, false
	}

	// 단어는 한 부류가 이어지는 구간이다. `한글abc` 가 두 단어인 것도 여기서 따라온다.
	class := glyphClass(line, col)

	start := col
	for start > 0 {
		prev := PrevGlyphStart(line, 0, start)
		if glyphClass(line, prev) != class {
			break
		}

		start = prev
	}

	end := col
	for end < len(line) && glyphClass(line, end) == class {
		end += GlyphSize(line, end)
	}

	return string(line[start:end]), start, true
}
