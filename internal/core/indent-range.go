package core

import (
	"bytes"

	"github.com/bluemir/zn/internal/syntax"
)

// indentDirection 은 범위를 미는 쪽이다. `searchDirection` 과 같은 손이다.
type indentDirection int

const (
	indentRight indentDirection = iota // `>`
	indentLeft                         // `<`
)

// shiftLines 는 [from, to] 를 한 단계 밀거나 당긴다. `>` `<` 가 쓴다.
//
// 언어를 보지 않는다 — 손으로 미는 것이라 규칙이 끼어들 자리가 없다. 규칙으로 다시 계산하는
// 것은 reindentLines 다.
//
// 재는 것은 화면 칸이다(blankColumns). space 로 들여쓴 줄이 tab 파일에 섞여 있어도 한 단계는
// 한 단계다 — 칸 수만 맞추고 그 줄이 쓰던 글자는 그대로 둔다(shiftBlank).
//
// **빈 줄은 건드리지 않는다.** 밀면 줄 끝 공백만 남고, 당길 것은 애초에 없다. vim 과 같다.
func (buf *Buffer) shiftLines(from, to int, direction indentDirection, width int) {
	unit := buf.indentText()

	by := blankColumns(unit)
	if direction == indentLeft {
		by = -by
	}

	next := make([][]byte, 0, to-from+1)
	for _, line := range buf.lines[from : to+1] {
		indent := leadingBlank(line)
		if len(indent) == len(line) {
			next = append(next, line)
			continue
		}

		next = append(next, concat(shiftBlank(indent, unit, by), line[len(indent):]))
	}

	buf.replaceIndented(from, to, next, width)
}

// reindentLines 는 [from, to] 를 언어 규칙이 정한 자리로 다시 들여쓴다. `=` 가 쓴다.
//
// 기준은 **범위 바로 위 줄의 지금 들여쓰기** 다. 그 줄은 건드리지 않는다 — 고른 것 밖이고,
// 규칙은 절대 자리를 모르고 앞 줄과의 차이만 안다.
//
// 규칙이 없는 파일과 markdown 은 아무 일도 하지 않는다(syntax.Indent 의 Reindents).
func (buf *Buffer) reindentLines(from, to, width int) {
	rule := syntax.IndentFor(buf.path)
	if rule == nil || !rule.Reindents() {
		return
	}

	buf.lexSyntaxTo(to)
	unit := buf.indentText()

	// prev 는 마지막으로 자리를 정한 줄이다. 줄 내용과 토큰은 **원래 것**이고 들여쓰기만 새것이다.
	// Next 는 「이 줄이 블록을 여는가」만 보므로 앞이 몇 칸이었는지와 무관하다.
	var prevLine []byte
	var prevTokens []syntax.Token
	var prevIndent []byte

	if from > 0 {
		prevLine = buf.lines[from-1]
		prevTokens = buf.syntaxTokens(from - 1)
		prevIndent = leadingBlank(prevLine)
	}

	next := make([][]byte, 0, to-from+1)
	for i := from; i <= to; i++ {
		line := buf.lines[i]
		body := line[len(leadingBlank(line)):]

		// 빈 줄은 비운 채로 둔다. 들여쓰기를 붙이면 줄 끝 공백이 된다.
		// 앞 줄 자리는 그대로 이어 간다 — 빈 줄 하나가 블록을 끊지 않는다.
		if len(body) < 1 {
			next = append(next, []byte{})
			continue
		}

		level, prefix := rule.Next(prevLine, prevTokens)
		indent := appendIndentLevel(prevIndent, unit, level, prefix)

		// 줄 전체를 넘긴다. 「이 줄이 닫는 줄인가」는 앞부분만 보므로 끝을 알릴 것이 없지만,
		// 낱말로 닫는 언어는 낱말이 끝났음을 알아야 해서 줄끝을 붙인다.
		if rule.Close(concat(body, []byte{'\n'})) > 0 {
			indent = shiftBlank(indent, unit, -blankColumns(unit))
		}

		next = append(next, concat(indent, body))

		prevLine, prevTokens, prevIndent = line, buf.syntaxTokens(i), indent
	}

	buf.replaceIndented(from, to, next, width)
}

// replaceIndented 는 다시 들여쓴 줄들을 갈아끼운다. shiftLines 와 reindentLines 가 나눠 쓴다.
//
// 바뀐 것이 없으면 손대지 않는다. 그냥 갈아끼우면 dirty 가 서고 redo 가 날아간다 —
// trimTrailingSpace 와 같은 자리다(edit.go).
//
// 커서는 첫 줄의 들여쓰기 다음이다. vim 과 같다.
func (buf *Buffer) replaceIndented(from, to int, next [][]byte, width int) {
	same := true
	for i, line := range next {
		if !bytes.Equal(line, buf.lines[from+i]) {
			same = false
			break
		}
	}
	if same {
		return
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
	buf.endEdit()
	buf.beginEdit(from, to-from+1)

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	buf.replaceLines(from, to-from+1, next)
	buf.endEdit()

	buf.cursorLine = from
	buf.cursorCol = len(leadingBlank(buf.lines[from]))
	buf.updateDesiredCol(width)
}

// concat 은 두 조각을 이은 새 것이다.
//
// `append(a, b...)` 를 그대로 쓸 수 없다. 여기서 이어 붙이는 앞쪽은 거의 언제나 Buffer 가 든
// read-only data 의 subslice 라, 남는 자리가 있으면 append 가 **다음 줄 위에 쓴다**(ADR-0001).
func concat(head, tail []byte) []byte {
	out := make([]byte, 0, len(head)+len(tail))
	out = append(out, head...)

	return append(out, tail...)
}
