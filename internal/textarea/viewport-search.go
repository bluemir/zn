package textarea

// buffer 안에서 패턴을 찾는 것들이다. 패턴을 짓고 화면에 칠하는 것은 search.go 에 있다 (ADR-0042).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-search.go 에 있다 (ADR-0121).

// moveTo 는 커서를 그 자리로 옮긴다. 검색이 찾은 자리로 뛸 때 쓴다.
func (buf *Viewport) MoveTo(Line, Col int) {
	buf.Cursor.Line = min(max(Line, 0), len(buf.Lines)-1)
	buf.Cursor.Col = min(max(Col, 0), len(buf.Lines[buf.Cursor.Line]))
	buf.UpdateDesiredCol()
}

// wordUnderCursor 는 `*` `#` 가 찾을 단어와 그 단어가 시작하는 자리다.
//
// 커서가 단어 위가 아니면 그 줄에서 오른쪽으로 첫 단어를 찾는다. 줄에 단어가 없으면 false 다.
// vim 과 같다 — 들여쓰기 위에서 눌러도 그 줄의 첫 낱말을 찾아준다.
func (buf Viewport) WordUnderCursor() (string, int, bool) {
	Line := buf.Lines[buf.Cursor.Line]

	Col := buf.Cursor.Col
	for Col < len(Line) && !isWordClass(glyphClass(Line, Col)) {
		Col += GlyphSize(Line, Col)
	}
	if Col >= len(Line) {
		return "", 0, false
	}

	// 단어는 한 부류가 이어지는 구간이다. `한글abc` 가 두 단어인 것도 여기서 따라온다.
	class := glyphClass(Line, Col)

	Start := Col
	for Start > 0 {
		prev := PrevGlyphStart(Line, 0, Start)
		if glyphClass(Line, prev) != class {
			break
		}

		Start = prev
	}

	End := Col
	for End < len(Line) && glyphClass(Line, End) == class {
		End += GlyphSize(Line, End)
	}

	return string(Line[Start:End]), Start, true
}
