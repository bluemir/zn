package core

import (
	"bytes"
	"strconv"

	"github.com/bluemir/zn/internal/syntax"
)

// indentUnit 은 이 파일이 한 단계에 쓰는 공백이다. 정하는 근거가 셋이고 순서가 있다.
//
// syntaxCache.path 와 같은 손이다 — 게으르게 정하고 담아 두었다가 경로가 달라지면 다시
// 정한다. 이름 없이 열었다가 `:w foo.go` 로 이름이 붙는 길이 있다.
type indentUnit struct {
	// path 는 text 를 정할 때 본 경로다. text 가 비어 있는 것과 아직 안 정한 것을 이것이 가른다.
	path string
	set  bool

	text []byte
}

// indentText 는 이 파일의 한 단계다.
func (buf *Buffer) indentText() []byte {
	if buf.indent.set && buf.indent.path == buf.path {
		return buf.indent.text
	}

	buf.indent = indentUnit{path: buf.path, set: true, text: resolveIndentUnit(buf.path, buf.lines)}

	return buf.indent.text
}

// resolveIndentUnit 은 한 단계를 정한다. 근거 셋을 순서대로 본다.
//
// **적힌 것이 잰 것보다 앞선다.** `.editorconfig` 는 그 저장소가 쓰기로 한 것을 사람이 적어
// 둔 것이고, 재는 것은 이미 쓰인 것에서 되짚는 짐작이다. 짐작이 맞는 경우가 대부분이지만
// 새로 만드는 파일이나 아직 한 줄도 들여쓰지 않은 파일에서는 아무것도 알아낼 수 없다.
func resolveIndentUnit(path string, lines [][]byte) []byte {
	if unit := editorconfigUnit(path); unit != nil {
		return unit
	}

	if unit := measureIndentUnit(lines); unit != nil {
		return unit
	}

	if rule := syntax.IndentFor(path); rule != nil {
		return rule.Unit()
	}

	// 언어를 모르는 파일이다. 규칙이 없어서 단계가 쓰일 일도 없지만, insert 의 `tab` 키가
	// `\t` 를 넣는 것과 같은 답을 둔다.
	return []byte{'\t'}
}

// editorconfigUnit 은 `.editorconfig` 가 정한 한 단계다. 적힌 것이 없으면 nil 이다.
//
// 여기서 보는 키는 셋이다 — `indent_style` `indent_size` `tab_width`. 저장할 때의 모습을
// 정하는 셋은 다른 자리가 본다(editorconfig.go, ADR-0052). 읽어 오는 자리는 그 파일 하나다.
func editorconfigUnit(path string) []byte {
	def := editorconfigFor(path)
	if def == nil {
		return nil
	}

	switch def.IndentStyle {
	case "tab":
		return []byte{'\t'}
	case "space":
		size, err := strconv.Atoi(def.IndentSize)
		if err != nil || size < 1 {
			// `indent_size = tab` 이면 tab_width 가 칸 수다. 라이브러리가 이미 풀어 준다.
			size = def.TabWidth
		}
		if size < 1 {
			return nil
		}

		return bytes.Repeat([]byte{' '}, size)
	}

	return nil
}

// measureIndentUnitLimit 은 재려고 보는 줄 수다. 파일이 커도 앞쪽만 보면 답이 같고, 여는
// 순간이 아니라 첫 Enter 에서 도는 셈이라 길어지면 손에 걸린다.
const measureIndentUnitLimit = 500

// measureIndentUnit 은 파일이 이미 쓰고 있는 한 단계를 되짚는다. 알 수 없으면 nil 이다.
//
// tab 은 세기만 하면 된다 — tab 으로 들여쓴 줄이 하나라도 더 많으면 그 파일은 tab 이다.
// space 는 칸 수를 알아야 해서 **이웃한 두 줄의 들여쓰기 차이**를 본다. 들여쓰기의 절대값을
// 세면 깊이 들어간 줄이 답을 흐린다 — space 네 칸 파일에서 세 단계 들어간 줄은 12 다.
func measureIndentUnit(lines [][]byte) []byte {
	tabs, spaces := 0, 0
	steps := map[int]int{}

	seen, prev := 0, -1
	for _, line := range lines {
		if seen >= measureIndentUnitLimit {
			break
		}

		blank := leadingBlank(line)
		if len(blank) == len(line) {
			// 빈 줄과 공백뿐인 줄은 들여쓰기가 아니다. 이어짐도 끊지 않는다.
			continue
		}

		if len(blank) > 0 {
			seen++
			if blank[0] == '\t' {
				tabs++
			} else {
				spaces++
			}
		}

		if prev >= 0 && len(blank) > prev && blank[0] != '\t' {
			steps[len(blank)-prev]++
		}
		prev = len(blank)
	}

	if tabs < 1 && spaces < 1 {
		return nil
	}
	if tabs >= spaces {
		return []byte{'\t'}
	}

	// 가장 흔한 걸음이다. 같은 수면 좁은 쪽을 고른다 — 넓게 잡아 어긋나는 것보다 낫다.
	best, count := 0, 0
	for step, n := range steps {
		if n > count || (n == count && step < best) {
			best, count = step, n
		}
	}
	if best < 1 {
		return nil
	}

	return bytes.Repeat([]byte{' '}, best)
}

// indentForNewLine 은 새 줄이 가질 들여쓰기다.
//
// head 는 새 줄 **바로 위에 남을 내용** 이다. 줄 끝에서 가르면 그 줄 전체지만 줄 가운데서
// 가르면 커서 앞까지다 — `func f() {|}` 에서 Enter 를 치면 위에 남는 것이 `func f() {` 라
// 새 줄이 들어가야 한다. 줄 전체를 보면 괄호가 닫혀 있어서 그것을 놓친다.
//
// at 은 head 가 있는 줄이다. 문법 토큰을 그 줄에서 가져온다.
//
// 언어를 모르는 파일이면 앞 줄의 들여쓰기를 그대로 잇는다. vim 의 `autoindent` 다.
func (buf *Buffer) indentForNewLine(at int, head []byte) []byte {
	base := leadingBlank(head)

	rule := syntax.IndentFor(buf.path)
	if rule == nil || at < 0 || at >= len(buf.lines) {
		return base
	}

	// 토큰이 있어야 문자열·주석 안의 괄호를 거른다. 커서가 있는 줄은 화면 안이라 이미 훑여
	// 있는 것이 보통이지만, 담아둔 것이 없으면(첫 키, 방금 고친 줄) 여기서 채운다.
	//
	// 토큰의 자리가 줄 전체 기준이라 head 보다 뒤일 수 있다. codeBytes 가 넘는 자리를 잘라
	// 낸다(syntax/indent.go).
	buf.lexSyntaxTo(at)

	level, prefix := rule.Next(head, buf.syntaxTokens(at))

	return appendIndentLevel(base, buf.indentText(), level, prefix)
}

// appendIndentLevel 은 base 에서 level 단계만큼 들이거나 내고 prefix 를 붙인다.
//
// **한 번에 한 단계뿐이다.** 한 줄에서 두 겹을 열어도 한 단계다 — 겹을 세어 따라가려면 참
// 파서가 있어야 하고, 어긋났을 때 손으로 되돌리는 값이 얻는 것보다 크다.
func appendIndentLevel(base, unit []byte, level int, prefix []byte) []byte {
	next := shiftBlank(base, unit, level*blankColumns(unit))

	if len(prefix) < 1 {
		return next
	}

	return concat(next, prefix)
}

// blankColumns 는 공백이 화면에서 몇 칸인지다. tab 은 다음 tab stop 까지라 byte 수와 다르다.
//
// **화면 칸으로 세는 것이 이 파일의 기준이다.** 들여쓰기를 옮기는 모든 자리(`tab` 키,
// `>` `<`, 닫는 표시를 칠 때, 새 줄) 가 이것을 쓴다. byte 로 세면 space 로 들여쓴 줄에
// `<` 를 칠 때 한 단계가 아니라 한 칸만 떨어진다.
//
// 재는 자리가 **화면**인 것도 일부러다. `.editorconfig` 의 `tab_width` 가 아니라 zn 이
// 실제로 그리는 폭(`tabWidth`) 을 쓴다 — 눈에 보이는 것과 움직이는 것이 어긋나면 안 된다.
func blankColumns(blank []byte) int {
	return screenColAt(blank, len(blank))
}

// shiftBlank 는 들여쓰기를 칸으로 재서 by 칸만큼 늘리거나 줄인 것이다.
//
// **채우는 글자는 그 줄이 이미 쓰던 것이다** — tab 으로 시작했으면 tab, 아니면 space.
// 들여쓰기가 없던 줄만 unit 을 따른다. 칸 수는 맞추되 글자는 두는 것이라, tab 파일에 섞여
// 있는 space 들여쓰기가 `>` 한 번에 조용히 tab 으로 바뀌지 않는다.
//
// by 가 0 이면 손대지 않는다. 다시 지으면 `  \t` 처럼 섞인 것이 뜻 없이 바뀐다.
func shiftBlank(blank, unit []byte, by int) []byte {
	if by == 0 {
		return blank
	}

	useTab := len(unit) > 0 && unit[0] == '\t'
	if len(blank) > 0 {
		useTab = blank[0] == '\t'
	}

	return makeBlank(max(blankColumns(blank)+by, 0), useTab)
}

// makeBlank 는 cols 칸짜리 공백이다.
// tab 으로 채울 때 남는 칸은 space 다 — tab 은 칸 경계까지만 미는 글자라 그 아래를 못 만든다.
func makeBlank(cols int, useTab bool) []byte {
	if !useTab {
		return bytes.Repeat([]byte{' '}, cols)
	}

	return append(
		bytes.Repeat([]byte{'\t'}, cols/tabWidth),
		bytes.Repeat([]byte{' '}, cols%tabWidth)...,
	)
}

// insertNewLine 은 커서 자리에서 줄을 가르고 새 줄에 이 파일의 규칙이 정한 들여쓰기를 넣는다.
//
// insert 를 **한 번**만 부른다. 그것이 여러 줄을 이미 한 되돌리기 구간으로 다루므로(edit.go)
// Enter 와 들여쓰기가 `u` 한 번에 같이 사라지고, 커서도 들여쓰기 다음 칸에 알아서 선다.
func (buf *Buffer) insertNewLine(width int) {
	line := buf.lines[buf.cursorLine]
	indent := buf.indentForNewLine(buf.cursorLine, line[:buf.cursorCol])

	buf.insert(concat([]byte{'\n'}, indent), width)
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
func (buf *Buffer) reindentClosing(typed []byte, width int) {
	rule := syntax.IndentFor(buf.path)
	if rule == nil {
		return
	}

	line := buf.lines[buf.cursorLine]
	indent := leadingBlank(line)
	if buf.cursorCol < len(indent) || buf.cursorCol > len(line) {
		return
	}

	before := line[len(indent):buf.cursorCol]
	head := concat(before, typed)
	if rule.Close(head) <= rule.Close(before) {
		return
	}

	unit := buf.indentText()
	pulled := shiftBlank(indent, unit, -blankColumns(unit))
	if len(pulled) == len(indent) {
		return
	}

	next := concat(pulled, line[len(indent):])

	// 열린 구간이 넓어질 뿐이라 이어 치는 글자와 한 번의 `u` 로 같이 돌아간다.
	buf.beginEdit(buf.cursorLine, 1)
	buf.replaceLines(buf.cursorLine, 1, [][]byte{next})

	buf.cursorCol -= len(indent) - len(pulled)
	buf.updateDesiredCol(width)
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
func (buf *Buffer) insertIndent(width int) {
	line := buf.lines[buf.cursorLine]

	if rule := syntax.IndentFor(buf.path); rule != nil && rule.TabIndentsLine(line) {
		buf.shiftLines(buf.cursorLine, buf.cursorLine, indentRight, width)
		return
	}

	unit := buf.indentText()
	step := blankColumns(unit)

	if unit[0] == '\t' {
		buf.insert([]byte{'\t'}, width)
		return
	}

	col := screenColAt(line, buf.cursorCol)
	buf.insert(makeBlank(step-col%step, false), width)
}

// outdentLine 은 `shift+tab` 이다. 지금 줄을 한 단계 내어쓴다.
//
// insert mode 에서 내어쓰는 유일한 길이다. 이것이 없으면 `esc` 로 나가 `<<` 를 치고 다시
// 들어와야 한다.
func (buf *Buffer) outdentLine(width int) {
	before := len(leadingBlank(buf.lines[buf.cursorLine]))
	col := buf.cursorCol

	buf.shiftLines(buf.cursorLine, buf.cursorLine, indentLeft, width)

	// shiftLines 는 커서를 들여쓰기 다음에 세운다. insert 에서는 치던 자리를 지켜야 하므로
	// 줄어든 만큼 왼쪽으로 옮긴다. 들여쓰기 안에 있었으면 그 끝에 선다.
	after := len(leadingBlank(buf.lines[buf.cursorLine]))

	buf.cursorCol = max(col+after-before, after)
	buf.updateDesiredCol(width)
}

// deleteIndentBackward 는 커서 앞이 공백뿐일 때의 `backspace` 다. 한 칸이 아니라 **앞 단위
// 경계까지** 지운다. 지울 것이 없으면 false 이고 그때는 보통 backspace 다.
//
// space 로 들여쓴 파일에서 tab 한 번이 넣은 것을 backspace 네 번으로 지우는 어긋남을 없앤다.
// 커서 앞에 글자가 하나라도 있으면 걸리지 않는다 — 글 가운데 공백은 들여쓰기가 아니다.
func (buf *Buffer) deleteIndentBackward(width int) bool {
	line := buf.lines[buf.cursorLine]
	if buf.cursorCol < 1 || buf.cursorCol > len(leadingBlank(line)) {
		return false
	}

	blank := line[:buf.cursorCol]
	step := blankColumns(buf.indentText())

	// 경계에 서 있으면 한 단계 앞으로, 아니면 바로 앞 경계로 간다.
	cols := blankColumns(blank)
	target := (cols - 1) / step * step

	pulled := makeBlank(target, blank[0] == '\t')
	if len(pulled) >= len(blank) {
		return false
	}

	buf.beginEdit(buf.cursorLine, 1)
	buf.replaceLines(buf.cursorLine, 1, [][]byte{concat(pulled, line[buf.cursorCol:])})

	buf.cursorCol = len(pulled)
	buf.updateDesiredCol(width)

	return true
}
