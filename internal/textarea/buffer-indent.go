package textarea

// 들여쓰기를 buffer 에 적용하는 자리다. 무엇을 한 단계로 삼는지(indent.go) 와 언어별 규칙
// (internal/syntax) 은 밖에 있고, 여기는 그것으로 줄을 들이고 내는 일만 한다 (ADR-0047, ADR-0048).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-indent.go 다 (ADR-0121).

// indentText 는 이 파일의 한 단계다.
func (buf *Buffer) indentText() []byte {
	if buf.indent.set && buf.indent.Path == buf.Path {
		return buf.indent.text
	}

	buf.indent = indentUnit{Path: buf.Path, set: true, text: resolveIndentUnit(buf.Path, buf.Language, buf.lines)}

	return buf.indent.text
}

// tabWidth 는 이 파일에서 tab 하나가 미는 화면 칸 수다.
//
// 이 파일 내용을 재거나 그리는 자리는 전부 이것을 넘긴다 — 그리는 폭(render-row.go), 커서와
// 줄바꿈(buffer-move.go·buffer-screen.go), 들여쓰기 한 단계(아래) 가 같은 답을 써야 한다.
// tabline 의 제목이나 statusBar 처럼 파일 내용이 아닌 글은 넘기지 않는다(lines 패키지 머리글).
//
// **값 receiver 다.** 그리는 쪽의 visibleRows·advanceRows 가 값 receiver 라 여기가 포인터면
// 그 자리에서 부를 수 없다. 정하는 것은 buffer 를 지을 때 끝나 있다(resolveTabWidth).
//
// 0 을 막는 자리가 하나 있다. `Buffer{path: "b.txt"}` 처럼 손으로 지은 것은 이 칸이 비는데,
// 그대로 넘기면 glyphAt 의 나머지 연산이 죽는다. 빈 칸은 「아직 정하지 않았다」로 읽는다.
func (buf Buffer) TabWidth() int {
	if buf.tab < 1 {
		return DefaultTabWidth
	}

	return buf.tab
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

	if at < 0 || at >= len(buf.lines) {
		return base
	}

	// 토큰이 있어야 문자열·주석 안의 괄호를 거른다. 커서가 있는 줄은 화면 안이라 이미 훑여
	// 있는 것이 보통이지만, 담아둔 것이 없으면(첫 키, 방금 고친 줄) 여기서 채운다.
	//
	// 토큰의 자리가 줄 전체 기준이라 head 보다 뒤일 수 있다. codeBytes 가 넘는 자리를 잘라
	// 낸다(syntax/indent.go).
	buf.LexSyntaxTo(at)

	// 규칙은 그 줄이 시작한 문맥이 고른다. markdown 코드펜스 안에서 Enter 를 치면 안쪽 언어의
	// 규칙이 온다(buffer-syntax.go).
	rule := buf.indentRuleAt(at)
	if rule == nil {
		return base
	}

	level, prefix := rule.Next(head, buf.SyntaxTokens(at))

	return appendIndentLevel(base, buf.indentText(), level, prefix, buf.TabWidth())
}
