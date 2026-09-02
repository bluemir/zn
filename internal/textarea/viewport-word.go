package textarea

// 단어 단위로 커서를 옮긴다. vim 의 `w`·`b`·`e` 다.
// 무엇이 한 단어인지는 glyph-class.go 가 정하고, 여기는 그 경계를 따라 걷는 일만 한다.
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-word.go 에 있다 (ADR-0121).

// moveWordForward 는 다음 단어의 첫 글자로 간다. vim 의 w/W 다.
func (viewport *Viewport) MoveWordForward(n int, kind WordKind) {
	for range n {
		viewport.WordForward(kind)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) WordForward(kind WordKind) {
	Line, Col := viewport.Cursor.Line, viewport.Cursor.Col

	// 지금 글자와 같은 부류가 이어지는 동안 앞으로 간다.
	// 공백에서 시작했으면 건너뛸 단어가 없으므로 아래 공백 건너뛰기로 바로 간다.
	class := viewport.ClassAt(Line, Col, kind)
	for class != ClassBlank && viewport.ClassAt(Line, Col, kind) == class {
		next, nextCol, ok := viewport.nextPos(Line, Col)
		if !ok {
			viewport.Cursor.Line, viewport.Cursor.Col = Line, Col

			return
		}

		Line, Col = next, nextCol
	}

	// 공백을 건너뛴다. 빈 줄은 그 자체로 단어라 거기서 멈춘다.
	for viewport.ClassAt(Line, Col, kind) == ClassBlank {
		if Col == 0 && len(viewport.Lines[Line]) == 0 && Line != viewport.Cursor.Line {
			break
		}

		next, nextCol, ok := viewport.nextPos(Line, Col)
		if !ok {
			break
		}

		Line, Col = next, nextCol
	}

	viewport.Cursor.Line, viewport.Cursor.Col = Line, Col
}

// moveWordBackward 는 단어의 첫 글자로 되돌아간다. vim 의 b/B 다.
func (viewport *Viewport) MoveWordBackward(n int, kind WordKind) {
	for range n {
		viewport.wordBackward(kind)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) wordBackward(kind WordKind) {
	Line, Col, ok := viewport.prevPos(viewport.Cursor.Line, viewport.Cursor.Col)
	if !ok {
		viewport.Cursor.Col = 0

		return
	}

	// 공백을 거꾸로 건너뛴다. 빈 줄은 그 자체로 단어다.
	for viewport.ClassAt(Line, Col, kind) == ClassBlank && len(viewport.Lines[Line]) > 0 {
		prev, prevCol, ok := viewport.prevPos(Line, Col)
		if !ok {
			break
		}

		Line, Col = prev, prevCol
	}
	if len(viewport.Lines[Line]) == 0 {
		viewport.Cursor.Line, viewport.Cursor.Col = Line, 0

		return
	}

	// 같은 부류가 시작하는 자리까지 거꾸로 간다.
	class := viewport.ClassAt(Line, Col, kind)
	for {
		prev, prevCol, ok := viewport.prevPos(Line, Col)
		if !ok || prev != Line || viewport.ClassAt(prev, prevCol, kind) != class {
			break
		}

		Line, Col = prev, prevCol
	}

	viewport.Cursor.Line, viewport.Cursor.Col = Line, Col
}

// moveWordEnd 는 단어의 마지막 글자로 간다. vim 의 e/E 다.
//
// w/b 와 달리 빈 줄에서 멈추지 않는다. 빈 줄에는 끝낼 단어가 없기 때문이다. vim 과 같다.
func (viewport *Viewport) MoveWordEnd(n int, kind WordKind) {
	for range n {
		viewport.WordEnd(kind)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) WordEnd(kind WordKind) {
	Line, Col, ok := viewport.nextPos(viewport.Cursor.Line, viewport.Cursor.Col)
	if !ok {
		return
	}

	// 공백을 건너뛴다. 끝낼 단어가 더 없으면 제자리에 둔다.
	for viewport.ClassAt(Line, Col, kind) == ClassBlank {
		next, nextCol, ok := viewport.nextPos(Line, Col)
		if !ok {
			return
		}

		Line, Col = next, nextCol
	}

	// 같은 부류가 끝나는 자리까지 간다.
	class := viewport.ClassAt(Line, Col, kind)
	for {
		next, nextCol, ok := viewport.nextPos(Line, Col)
		if !ok || next != Line || viewport.ClassAt(next, nextCol, kind) != class {
			break
		}

		Line, Col = next, nextCol
	}

	viewport.Cursor.Line, viewport.Cursor.Col = Line, Col
}
