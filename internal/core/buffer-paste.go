package core

import (
	"bytes"
)

// 붙여넣는 것들이다. `p`·`P` 다. register 가 줄 단위인지 글자 단위인지에 따라 갈린다 (ADR-0018).

// pasteAfter 는 register 를 커서 뒤에 붙인다. vim 의 `p` 다.
//
// 줄 단위면 커서 줄 아래에 줄로 끼우고, 글자 단위면 커서가 선 글자 뒤에 끼운다.
// count 는 되풀이다 — `3p` 는 세 번 붙인다(ADR-0017).
func (buf *viewport) pasteAfter(reg register, count, width int) {
	if len(reg.lines) == 0 {
		return
	}

	if reg.linewise {
		buf.pasteLines(buf.cursorLine+1, reg, count, width)

		return
	}

	// 커서가 선 글자 뒤다. 빈 줄이나 줄 끝이면 그 자리가 곧 줄 끝이다.
	line := buf.lines[buf.cursorLine]
	col := buf.cursorCol
	if col < len(line) {
		col += glyphSize(line, col)
	}

	buf.pasteText(col, reg, count, width)
}

// pasteBefore 는 register 를 커서 앞에 붙인다. vim 의 `P` 다.
func (buf *viewport) pasteBefore(reg register, count, width int) {
	if len(reg.lines) == 0 {
		return
	}

	if reg.linewise {
		buf.pasteLines(buf.cursorLine, reg, count, width)

		return
	}

	buf.pasteText(buf.cursorCol, reg, count, width)
}

// pasteLines 는 at 자리에 register 의 줄을 count 번 끼운다.
// 커서는 붙인 첫 줄의 첫 비공백이다. vim 과 같다.
func (buf *viewport) pasteLines(at int, reg register, count, width int) {
	lines := make([][]byte, 0, len(reg.lines)*count)
	for range count {
		lines = append(lines, reg.lines...)
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 붙여넣기는 언제나 제 구간이다.
	// `3p` 도 한 구간이라 `u` 한 번에 전부 사라진다.
	buf.endEdit()
	buf.insertLines(at, lines)
	buf.endEdit()

	buf.cursorLine = at
	buf.moveLineFirstNonBlank(width)
	buf.clampToNormal(width)
}

// pasteText 는 지금 줄의 col 칸에 register 를 글자로 끼운다.
//
// 줄바꿈을 가르는 곳은 insert 하나뿐이라(ADR-0001) 여러 줄 register 도 그대로 먹는다.
func (buf *viewport) pasteText(col int, reg register, count, width int) {
	text := bytes.Repeat(bytes.Join(reg.lines, []byte{'\n'}), count)
	if len(text) == 0 {
		return
	}

	startLine, startCol := buf.cursorLine, col

	buf.endEdit()
	buf.cursorCol = col
	buf.insert(text, width)
	buf.endEdit()

	// 여러 줄이면 커서는 붙인 첫 글자다. vim 과 같다.
	if len(reg.lines) > 1 {
		buf.cursorLine, buf.cursorCol = startLine, startCol
		buf.updateDesiredCol(width)

		return
	}

	// 한 줄이면 붙인 마지막 글자 위다. insert 는 그 다음 칸에 커서를 두고 나온다.
	buf.cursorCol = buf.prevOffset(buf.cursorCol, width)
	buf.updateDesiredCol(width)
}
