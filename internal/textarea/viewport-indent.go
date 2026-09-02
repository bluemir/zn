package textarea

import (
	"bytes"

	"github.com/bluemir/zn/internal/syntax"
)

// 들여쓰기를 buffer 에 적용하는 자리다. 무엇을 한 단계로 삼는지(indent.go) 와 언어별 규칙
// (internal/syntax) 은 밖에 있고, 여기는 그것으로 줄을 들이고 내는 일만 한다 (ADR-0047, ADR-0048).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-indent.go 에 있다 (ADR-0121).

// insertNewLine 은 커서 자리에서 줄을 가르고 새 줄에 이 파일의 규칙이 정한 들여쓰기를 넣는다.
//
// insert 를 **한 번**만 부른다. 그것이 여러 줄을 이미 한 되돌리기 구간으로 다루므로(edit.go)
// Enter 와 들여쓰기가 `u` 한 번에 같이 사라지고, 커서도 들여쓰기 다음 칸에 알아서 선다.
func (viewport *Viewport) InsertNewLine() {
	line := viewport.Lines[viewport.Cursor.Line]
	indent := viewport.indentForNewLine(viewport.Cursor.Line, line[:viewport.Cursor.Col])

	viewport.Insert(concat([]byte{'\n'}, indent))
}

// reindentClosing 은 방금 친 글자가 그 줄을 닫는 줄로 만들었으면 한 단계 당긴다.
//
// insert mode 에서 글자를 넣기 **전에** 부른다. typed 는 이제 넣을 것이고, Enter 면 `\n` 이다 —
// 낱말로 닫는 언어가 낱말이 끝났음을 그것으로 안다(syntax.Indent).
//
// **친 글자가 답을 바꾼 순간에만 움직인다.** Close 는 「줄 앞에 이것이 있으면 나온 줄이다」라
// 앞부분만 보므로 `}` 뒤에 무엇을 더 쳐도 계속 1 이다. 친 것 앞뒤를 견주면 `}` 를 치는
// 그 한 번만 걸리고, 이어 치는 글자에는 줄이 또 당겨지지 않는다.
//
// 화면에 이미 있는 글자가 움직이는 자리는 여기 하나뿐이라 좁게 잡는다.
// **여기서 문맥을 채우지 않는다.** 글자마다 부르는 자리라 lexSyntaxTo 를 끼우면 아직 다
// 치지 않은 줄에서 수렴 판정이 돌아 캐시가 흔들린다. 커서 줄은 화면 안이라 그리는 쪽이
// 이미 채워 두었고, 없으면 파일 언어의 규칙으로 물러난다(buffer-syntax.go).
func (viewport *Viewport) ReindentClosing(typed []byte) {
	rule := viewport.indentRuleAt(viewport.Cursor.Line)
	if rule == nil {
		return
	}

	line := viewport.Lines[viewport.Cursor.Line]
	indent := leadingBlank(line)
	if viewport.Cursor.Col < len(indent) || viewport.Cursor.Col > len(line) {
		return
	}

	before := line[len(indent):viewport.Cursor.Col]
	head := concat(before, typed)
	if rule.Close(head) <= rule.Close(before) {
		return
	}

	unit, tab := viewport.indentText(), viewport.TabWidth()
	pulled := shiftBlank(indent, unit, -blankColumns(unit, tab), tab)
	if len(pulled) == len(indent) {
		return
	}

	next := concat(pulled, line[len(indent):])

	// 열린 구간이 넓어질 뿐이라 이어 치는 글자와 한 번의 `u` 로 같이 돌아간다.
	viewport.BeginEdit(viewport.Cursor.Line, 1)
	viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{next})

	viewport.Cursor.Col -= len(indent) - len(pulled)
	viewport.UpdateDesiredCol()
}

// insertIndent 는 `tab` 키다. 커서를 **다음 단위 경계**까지 민다.
//
// 한 단위를 그대로 넣지 않고 경계까지 채우는 것은, 어긋난 자리에서 tab 을 한 번 치면 다시
// 맞게 되돌아오기 때문이다. 들여쓰기가 스스로 가지런해진다.
//
// 단위가 tab 이면 tab 하나를 넣는다 — 그때 한 단위는 화면 tab 폭과 같아서 글자 하나가
// 정확히 다음 경계까지 민다. space 면 모자란 칸만큼 넣는다.
//
// markdown 의 목록 줄에서는 커서 자리가 아니라 줄 전체가 한 단계 들어간다(syntax.Indent).
func (viewport *Viewport) InsertIndent() {
	line := viewport.Lines[viewport.Cursor.Line]

	// 규칙이 문맥을 따라오므로 담아둔 것이 있어야 한다. 글자마다가 아니라 `tab` 을 칠 때만
	// 지나는 자리라 채워도 된다 — reindentClosing 이 못 하는 것이 이것이다.
	viewport.LexSyntaxTo(viewport.Cursor.Line)

	if rule := viewport.indentRuleAt(viewport.Cursor.Line); rule != nil && rule.TabIndentsLine(line) {
		viewport.ShiftLines(viewport.Cursor.Line, viewport.Cursor.Line, IndentRight)
		return
	}

	unit, tab := viewport.indentText(), viewport.TabWidth()
	step := blankColumns(unit, tab)

	if unit[0] == '\t' {
		viewport.Insert([]byte{'\t'})
		return
	}

	col := ScreenColAt(line, viewport.Cursor.Col, tab)
	viewport.Insert(makeBlank(step-col%step, false, tab))
}

// outdentLine 은 `shift+tab` 이다. 지금 줄을 한 단계 내어쓴다.
//
// insert mode 에서 내어쓰는 유일한 길이다. 이것이 없으면 `esc` 로 나가 `<<` 를 치고 다시
// 들어와야 한다.
func (viewport *Viewport) OutdentLine() {
	before := len(leadingBlank(viewport.Lines[viewport.Cursor.Line]))
	col := viewport.Cursor.Col

	viewport.ShiftLines(viewport.Cursor.Line, viewport.Cursor.Line, IndentLeft)

	// shiftLines 는 커서를 들여쓰기 다음에 세운다. insert 에서는 치던 자리를 지켜야 하므로
	// 줄어든 만큼 왼쪽으로 옮긴다. 들여쓰기 안에 있었으면 그 끝에 선다.
	after := len(leadingBlank(viewport.Lines[viewport.Cursor.Line]))

	viewport.Cursor.Col = max(col+after-before, after)
	viewport.UpdateDesiredCol()
}

// deleteIndentBackward 는 커서 앞이 공백뿐일 때의 `backspace` 다. 한 칸이 아니라 **앞 단위
// 경계까지** 지운다. 지울 것이 없으면 false 이고 그때는 보통 backspace 다.
//
// space 로 들여쓴 파일에서 tab 한 번이 넣은 것을 backspace 네 번으로 지우는 어긋남을 없앤다.
// 커서 앞에 글자가 하나라도 있으면 걸리지 않는다 — 글 가운데 공백은 들여쓰기가 아니다.
func (viewport *Viewport) DeleteIndentBackward() bool {
	line := viewport.Lines[viewport.Cursor.Line]
	if viewport.Cursor.Col < 1 || viewport.Cursor.Col > len(leadingBlank(line)) {
		return false
	}

	blank := line[:viewport.Cursor.Col]
	tab := viewport.TabWidth()
	step := blankColumns(viewport.indentText(), tab)

	// 경계에 서 있으면 한 단계 앞으로, 아니면 바로 앞 경계로 간다.
	cols := blankColumns(blank, tab)
	target := (cols - 1) / step * step

	pulled := makeBlank(target, blank[0] == '\t', tab)
	if len(pulled) >= len(blank) {
		return false
	}

	viewport.BeginEdit(viewport.Cursor.Line, 1)
	viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{concat(pulled, line[viewport.Cursor.Col:])})

	viewport.Cursor.Col = len(pulled)
	viewport.UpdateDesiredCol()

	return true
}

// shiftLines 는 [from, to] 를 한 단계 밀거나 당긴다. `>` `<` 가 쓴다.
//
// 언어를 보지 않는다 — 손으로 미는 것이라 규칙이 끼어들 자리가 없다. 규칙으로 다시 계산하는
// 것은 reindentLines 다.
//
// 재는 것은 화면 칸이다(blankColumns). space 로 들여쓴 줄이 tab 파일에 섞여 있어도 한 단계는
// 한 단계다 — 칸 수만 맞추고 그 줄이 쓰던 글자는 그대로 둔다(shiftBlank).
//
// **빈 줄은 건드리지 않는다.** 밀면 줄 끝 공백만 남고, 당길 것은 애초에 없다. vim 과 같다.
func (viewport *Viewport) ShiftLines(from, to int, direction IndentDirection) {
	unit, tab := viewport.indentText(), viewport.TabWidth()

	by := blankColumns(unit, tab)
	if direction == IndentLeft {
		by = -by
	}

	next := make([][]byte, 0, to-from+1)
	for _, line := range viewport.Lines[from : to+1] {
		indent := leadingBlank(line)
		if len(indent) == len(line) {
			next = append(next, line)
			continue
		}

		next = append(next, concat(shiftBlank(indent, unit, by, tab), line[len(indent):]))
	}

	viewport.replaceIndented(from, to, next)
}

// reindentLines 는 [from, to] 를 언어 규칙이 정한 자리로 다시 들여쓴다. `=` 가 쓴다.
//
// 기준은 **범위 바로 위 줄의 지금 들여쓰기** 다. 그 줄은 건드리지 않는다 — 고른 것 밖이고,
// 규칙은 절대 자리를 모르고 앞 줄과의 차이만 안다.
//
// 규칙이 없는 파일과 markdown 은 아무 일도 하지 않는다(syntax.Indent 의 Reindents).
//
// **판정이 줄마다다.** markdown 문서 안의 코드펜스는 안쪽 언어의 규칙을 받으므로(ADR-0102)
// 산문은 그대로 두고 그 안만 정리한다. 건드리지 않는 줄도 기준선은 이어 간다.
func (viewport *Viewport) ReindentLines(from, to int) {
	if viewport.Language.Indent() == nil {
		return
	}

	viewport.LexSyntaxTo(to)
	unit, tab := viewport.indentText(), viewport.TabWidth()

	// prev 는 마지막으로 자리를 정한 줄이다. 줄 내용과 토큰은 **원래 것**이고 들여쓰기만 새것이다.
	// Next 는 「이 줄이 블록을 여는가」만 보므로 앞이 몇 칸이었는지와 무관하다.
	var prevLine []byte
	var prevTokens []syntax.Token
	var prevIndent []byte

	if from > 0 {
		prevLine = viewport.Lines[from-1]
		prevTokens = viewport.SyntaxTokens(from - 1)
		prevIndent = leadingBlank(prevLine)
	}

	next := make([][]byte, 0, to-from+1)
	for i := from; i <= to; i++ {
		line := viewport.Lines[i]
		body := line[len(leadingBlank(line)):]

		// 빈 줄은 비운 채로 둔다. 들여쓰기를 붙이면 줄 끝 공백이 된다.
		// 앞 줄 자리는 그대로 이어 간다 — 빈 줄 하나가 블록을 끊지 않는다.
		if len(body) < 1 {
			next = append(next, []byte{})
			continue
		}

		// 이 줄의 자리를 정할 규칙이 없으면 그대로 둔다. markdown 문서의 산문과 목록이다 —
		// 깊이를 글쓴이가 정한 것이라 앞 줄에서 되짚을 수 없다(syntax.Indent 의 Reindents).
		rule := viewport.indentRuleAt(i)
		if rule == nil || !rule.Reindents() {
			next = append(next, line)
			prevLine, prevTokens, prevIndent = line, viewport.SyntaxTokens(i), leadingBlank(line)

			continue
		}

		// **여는 것은 앞 줄의 규칙이 본다.** 코드펜스를 여는 줄(```` ```go ````) 은 markdown 이고
		// 그 다음 줄부터가 안쪽 언어다. 이 줄의 규칙으로 앞 줄을 보면 경계에서 답이 갈린다.
		level, prefix := 0, []byte(nil)
		if prev := viewport.indentRuleAt(i - 1); prev != nil {
			level, prefix = prev.Next(prevLine, prevTokens)
		}

		indent := appendIndentLevel(prevIndent, unit, level, prefix, tab)

		// 줄 전체를 넘긴다. 「이 줄이 닫는 줄인가」는 앞부분만 보므로 끝을 알릴 것이 없지만,
		// 낱말로 닫는 언어는 낱말이 끝났음을 알아야 해서 줄끝을 붙인다.
		if rule.Close(concat(body, []byte{'\n'})) > 0 {
			indent = shiftBlank(indent, unit, -blankColumns(unit, tab), tab)
		}

		next = append(next, concat(indent, body))

		prevLine, prevTokens, prevIndent = line, viewport.SyntaxTokens(i), indent
	}

	viewport.replaceIndented(from, to, next)
}

// replaceIndented 는 다시 들여쓴 줄들을 갈아끼운다. shiftLines 와 reindentLines 가 나눠 쓴다.
//
// 바뀐 것이 없으면 손대지 않는다. 그냥 갈아끼우면 dirty 가 서고 redo 가 날아간다 —
// trimTrailingSpace 와 같은 자리다(edit.go).
//
// 커서는 첫 줄의 들여쓰기 다음이다. vim 과 같다.
func (viewport *Viewport) replaceIndented(from, to int, next [][]byte) {
	same := true
	for i, line := range next {
		if !bytes.Equal(line, viewport.Lines[from+i]) {
			same = false
			break
		}
	}
	if same {
		return
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
	viewport.EndEdit()
	viewport.BeginEdit(from, to-from+1)

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	viewport.ReplaceLines(from, to-from+1, next)
	viewport.EndEdit()

	viewport.Cursor.Line = from
	viewport.Cursor.Col = len(leadingBlank(viewport.Lines[from]))
	viewport.UpdateDesiredCol()
}
