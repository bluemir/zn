package core

// 단어 단위로 커서를 옮긴다. vim 의 `w`·`b`·`e` 다.
// 무엇이 한 단어인지는 glyph-class.go 가 정하고, 여기는 그 경계를 따라 걷는 일만 한다.

// classAt 은 그 자리 글자의 부류다. 줄 끝(줄바꿈 자리) 은 공백으로 본다.
// 줄바꿈을 공백으로 보면 줄을 넘는 단어 이동이 한 줄 안의 이동과 같은 규칙이 된다.
//
// **`W` 가 부류를 접는 자리가 여기다.** `glyphClass` 는 네 부류를 그대로 주고, 그것을
// 둘로 접는 것은 그 키의 규칙이라 이 겹이 한다 (ADR-0117).
func (buf Buffer) classAt(line, col int, kind wordKind) charClass {
	if col >= len(buf.lines[line]) {
		return classBlank
	}

	class := glyphClass(buf.lines[line], col)
	if kind == wordBig && class != classBlank {
		// 큰 단어는 공백으로만 끊으므로 공백이 아닌 것은 전부 한 부류다.
		return classWord
	}

	return class
}

// nextPos, prevPos 는 한 글자 앞뒤의 자리다. 줄 끝에서는 줄을 넘는다.
//
// 줄 끝(col == len(line)) 도 자리 하나로 센다. 그 자리가 줄바꿈이고, 단어 이동에서는 공백이다.
// 파일 양끝에서는 더 갈 곳이 없어서 false 다.
func (buf Buffer) nextPos(line, col int) (int, int, bool) {
	if col < len(buf.lines[line]) {
		return line, col + glyphSize(buf.lines[line], col), true
	}
	if line+1 >= len(buf.lines) {
		return line, col, false
	}

	return line + 1, 0, true
}

func (buf Buffer) prevPos(line, col int) (int, int, bool) {
	if col > 0 {
		// grapheme cluster 는 뒤에서 앞으로 읽을 수 없어서 줄 시작에서부터 훑는다.
		return line, prevGlyphStart(buf.lines[line], 0, col), true
	}
	if line == 0 {
		return line, col, false
	}

	return line - 1, len(buf.lines[line-1]), true
}

// moveWordForward 는 다음 단어의 첫 글자로 간다. vim 의 w/W 다.
func (buf *Buffer) moveWordForward(n int, kind wordKind, width int) {
	for range n {
		buf.wordForward(kind)
	}

	buf.updateDesiredCol(width)
}

func (buf *Buffer) wordForward(kind wordKind) {
	line, col := buf.cursorLine, buf.cursorCol

	// 지금 글자와 같은 부류가 이어지는 동안 앞으로 간다.
	// 공백에서 시작했으면 건너뛸 단어가 없으므로 아래 공백 건너뛰기로 바로 간다.
	class := buf.classAt(line, col, kind)
	for class != classBlank && buf.classAt(line, col, kind) == class {
		next, nextCol, ok := buf.nextPos(line, col)
		if !ok {
			buf.cursorLine, buf.cursorCol = line, col

			return
		}

		line, col = next, nextCol
	}

	// 공백을 건너뛴다. 빈 줄은 그 자체로 단어라 거기서 멈춘다.
	for buf.classAt(line, col, kind) == classBlank {
		if col == 0 && len(buf.lines[line]) == 0 && line != buf.cursorLine {
			break
		}

		next, nextCol, ok := buf.nextPos(line, col)
		if !ok {
			break
		}

		line, col = next, nextCol
	}

	buf.cursorLine, buf.cursorCol = line, col
}

// moveWordBackward 는 단어의 첫 글자로 되돌아간다. vim 의 b/B 다.
func (buf *Buffer) moveWordBackward(n int, kind wordKind, width int) {
	for range n {
		buf.wordBackward(kind)
	}

	buf.updateDesiredCol(width)
}

func (buf *Buffer) wordBackward(kind wordKind) {
	line, col, ok := buf.prevPos(buf.cursorLine, buf.cursorCol)
	if !ok {
		buf.cursorCol = 0

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
		buf.cursorLine, buf.cursorCol = line, 0

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

	buf.cursorLine, buf.cursorCol = line, col
}

// moveWordEnd 는 단어의 마지막 글자로 간다. vim 의 e/E 다.
//
// w/b 와 달리 빈 줄에서 멈추지 않는다. 빈 줄에는 끝낼 단어가 없기 때문이다. vim 과 같다.
func (buf *Buffer) moveWordEnd(n int, kind wordKind, width int) {
	for range n {
		buf.wordEnd(kind)
	}

	buf.updateDesiredCol(width)
}

func (buf *Buffer) wordEnd(kind wordKind) {
	line, col, ok := buf.nextPos(buf.cursorLine, buf.cursorCol)
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

	buf.cursorLine, buf.cursorCol = line, col
}
