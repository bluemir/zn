package core

// 단어 단위로 커서를 옮긴다. vim 의 `w`·`b`·`e` 다.
// 무엇이 한 단어인지는 glyph-class.go 가 정하고, 여기는 그 경계를 따라 걷는 일만 한다.
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-word.go 다 (ADR-0121).

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
