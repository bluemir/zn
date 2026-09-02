package core

// 단어 단위로 커서를 옮긴다. vim 의 `w`·`b`·`e` 다.
// 무엇이 한 단어인지는 glyph-class.go 가 정하고, 여기는 그 경계를 따라 걷는 일만 한다.
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-word.go 에 있다 (ADR-0121).

// moveWordForward 는 다음 단어의 첫 글자로 간다. vim 의 w/W 다.
func (buf *viewport) moveWordForward(n int, kind wordKind) {
	for range n {
		buf.wordForward(kind)
	}

	buf.updateDesiredCol()
}

func (buf *viewport) wordForward(kind wordKind) {
	line, col := buf.cursor.Line, buf.cursor.Col

	// 지금 글자와 같은 부류가 이어지는 동안 앞으로 간다.
	// 공백에서 시작했으면 건너뛸 단어가 없으므로 아래 공백 건너뛰기로 바로 간다.
	class := buf.classAt(line, col, kind)
	for class != classBlank && buf.classAt(line, col, kind) == class {
		next, nextCol, ok := buf.nextPos(line, col)
		if !ok {
			buf.cursor.Line, buf.cursor.Col = line, col

			return
		}

		line, col = next, nextCol
	}

	// 공백을 건너뛴다. 빈 줄은 그 자체로 단어라 거기서 멈춘다.
	for buf.classAt(line, col, kind) == classBlank {
		if col == 0 && len(buf.lines[line]) == 0 && line != buf.cursor.Line {
			break
		}

		next, nextCol, ok := buf.nextPos(line, col)
		if !ok {
			break
		}

		line, col = next, nextCol
	}

	buf.cursor.Line, buf.cursor.Col = line, col
}

// moveWordBackward 는 단어의 첫 글자로 되돌아간다. vim 의 b/B 다.
func (buf *viewport) moveWordBackward(n int, kind wordKind) {
	for range n {
		buf.wordBackward(kind)
	}

	buf.updateDesiredCol()
}

func (buf *viewport) wordBackward(kind wordKind) {
	line, col, ok := buf.prevPos(buf.cursor.Line, buf.cursor.Col)
	if !ok {
		buf.cursor.Col = 0

		return
	}

	// 공백을 거꾸로 건너뛴다. 빈 줄은 그 자체로 단어다.
	for buf.classAt(line, col, kind) == classBlank && len(buf.lines[line]) > 0 {
		prev, prevCol, ok := buf.prevPos(line, col)
		if !ok {
			break
		}

		line, col = prev, prevCol
	}
	if len(buf.lines[line]) == 0 {
		buf.cursor.Line, buf.cursor.Col = line, 0

		return
	}

	// 같은 부류가 시작하는 자리까지 거꾸로 간다.
	class := buf.classAt(line, col, kind)
	for {
		prev, prevCol, ok := buf.prevPos(line, col)
		if !ok || prev != line || buf.classAt(prev, prevCol, kind) != class {
			break
		}

		line, col = prev, prevCol
	}

	buf.cursor.Line, buf.cursor.Col = line, col
}

// moveWordEnd 는 단어의 마지막 글자로 간다. vim 의 e/E 다.
//
// w/b 와 달리 빈 줄에서 멈추지 않는다. 빈 줄에는 끝낼 단어가 없기 때문이다. vim 과 같다.
func (buf *viewport) moveWordEnd(n int, kind wordKind) {
	for range n {
		buf.wordEnd(kind)
	}

	buf.updateDesiredCol()
}

func (buf *viewport) wordEnd(kind wordKind) {
	line, col, ok := buf.nextPos(buf.cursor.Line, buf.cursor.Col)
	if !ok {
		return
	}

	// 공백을 건너뛴다. 끝낼 단어가 더 없으면 제자리에 둔다.
	for buf.classAt(line, col, kind) == classBlank {
		next, nextCol, ok := buf.nextPos(line, col)
		if !ok {
			return
		}

		line, col = next, nextCol
	}

	// 같은 부류가 끝나는 자리까지 간다.
	class := buf.classAt(line, col, kind)
	for {
		next, nextCol, ok := buf.nextPos(line, col)
		if !ok || next != line || buf.classAt(next, nextCol, kind) != class {
			break
		}

		line, col = next, nextCol
	}

	buf.cursor.Line, buf.cursor.Col = line, col
}
