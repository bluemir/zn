package textarea

import (
	"github.com/bluemir/zn/internal/scheme"
)

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
func (buf Buffer) ClassAt(at scheme.Cursor, kind WordKind) charClass {
	if at.Col >= len(buf.lines[at.Line]) {
		return ClassBlank
	}

	class := glyphClass(buf.lines[at.Line], at.Col)
	if kind == WordBig && class != ClassBlank {
		// 큰 단어는 공백으로만 끊으므로 공백이 아닌 것은 전부 한 부류다.
		return classWord
	}

	return class
}

// nextPos, prevPos 는 한 글자 앞뒤의 자리다. 줄 끝에서는 줄을 넘는다.
//
// 줄 끝(col == len(line)) 도 자리 하나로 센다. 그 자리가 줄바꿈이고, 단어 이동에서는 공백이다.
// 파일 양끝에서는 더 갈 곳이 없어서 false 다.
//
// **자리를 Cursor 하나로 주고받는다.** 이름 없는 `int` 둘을 돌려주던 때는 부르는 쪽이
// 줄과 칸을 거꾸로 받아도 컴파일이 되었다(ADR-0122). 걷는 쪽도 짝을 손으로 굴리지 않는다.
func (buf Buffer) nextPos(at scheme.Cursor) (scheme.Cursor, bool) {
	if at.Col < len(buf.lines[at.Line]) {
		return scheme.Cursor{Line: at.Line, Col: at.Col + GlyphSize(buf.lines[at.Line], at.Col)}, true
	}
	if at.Line+1 >= len(buf.lines) {
		return at, false
	}

	return scheme.Cursor{Line: at.Line + 1}, true
}

func (buf Buffer) prevPos(at scheme.Cursor) (scheme.Cursor, bool) {
	if at.Col > 0 {
		// grapheme cluster 는 뒤에서 앞으로 읽을 수 없어서 줄 시작에서부터 훑는다.
		return scheme.Cursor{Line: at.Line, Col: PrevGlyphStart(buf.lines[at.Line], 0, at.Col)}, true
	}
	if at.Line == 0 {
		return at, false
	}

	return scheme.Cursor{Line: at.Line - 1, Col: len(buf.lines[at.Line-1])}, true
}

// AroundWord 는 `aw` 가 단어에 더 먹는 공백까지 넓힌 범위다.
//
// **뒤 공백을 먹고, 없으면 앞 공백을 먹는다.** vim 과 같다. 줄 가운데 낱말을 `daw` 로 지우면
// 공백이 하나만 남고, 줄 끝 낱말이면 앞 공백을 먹어서 줄 끝에 공백이 남지 않는다.
//
// 넓히는 것이라 받는 것도 내놓는 것도 범위다. 단어는 줄 안의 것이라 양끝의 줄이 같다.
func (buf Buffer) AroundWord(area scheme.MotionRange, kind WordKind) scheme.MotionRange {
	text := buf.lines[area.Start.Line]

	after := area.End.Col
	for after < len(text) && buf.ClassAt(scheme.Cursor{Line: area.End.Line, Col: after}, kind) == ClassBlank {
		after += GlyphSize(text, after)
	}

	if after > area.End.Col {
		area.End.Col = after

		return area
	}

	for area.Start.Col > 0 {
		prev := PrevGlyphStart(text, 0, area.Start.Col)
		if buf.ClassAt(scheme.Cursor{Line: area.Start.Line, Col: prev}, kind) != ClassBlank {
			break
		}

		area.Start.Col = prev
	}

	return area
}
