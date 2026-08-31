package syntax

import (
	"bytes"
	"fmt"
	"strconv"
)

// mdNormal 은 markdown 의 보통 문맥이다. 코드펜스 안이 아니다.
//
// afterTableRow 는 앞 줄이 표의 한 행이었는지다. **표에서 앞 줄을 봐야 하는 것은 이것 하나다**
// — 이어지는 `|` 줄의 첫 줄이 머리 행이라 굵게 그린다. 나머지는 줄 하나로 정해진다.
//
// 상태를 새로 만들지 않고 칸을 하나 든 것은, 표 안에서도 markdown 이 하던 일을 다 해야 하기
// 때문이다. 표 다음 줄에 코드펜스가 열리거나 제목이 올 수 있다.
type mdNormal struct {
	afterTableRow bool
}

// mdFence 는 코드펜스 안이다.
//
// 여는 표시의 글자와 길이를 든다. “ ``` “ 는 `~~~` 로 닫히지 않고, 백틱 넷으로 연 것은
// 셋으로 닫히지 않는다(CommonMark). 둘 다 견줄 수 있는 값이라 문맥 수렴에 쓸 수 있다.
type mdFence struct {
	marker byte
	size   int

	// inner 는 안쪽 언어의 문맥이다. nil 이면 언어를 모르는 것이라 색을 입히지 않는다.
	//
	// 이 칸이 interface 라 **안에 들어오는 것은 반드시 `==` 로 견줄 수 있어야 한다**
	// (ADR-0039, ADR-0040). core 가 나가는 문맥을 캐시에 든 것과 견주는 자리에서 견줄 수
	// 없는 값은 그 자리에서 panic 이다.
	inner State
}

// mdMaxFenceIndent 는 코드펜스로 볼 수 있는 들여쓰기 칸이다. 넷이면 들여쓴 코드블록이다.
const mdMaxFenceIndent = 3

func (mdNormal) Indent() Indent { return mdIndent{} }

// Indent 는 코드펜스 안에서 안쪽 언어의 규칙이다. 강조를 넘기는 것과 같은 자리다.
//
// 언어를 적지 않았거나 모르는 이름이면(```` ```text ````) markdown 규칙이다. 그 안은 우리가
// 무엇인지 모르는 글이고, markdown 규칙은 목록을 잇는 것이라 목록이 아닌 줄에 아무 일도
// 하지 않는다.
//
// **닫는 펜스 줄도 이 규칙을 받는다.** 그 줄을 시작한 문맥이 아직 펜스 안이라서다. 안쪽
// 언어의 규칙에게 ```` ``` ```` 은 여는 것도 닫는 것도 아니라 답이 0 이고, 그것이 맞는 답이다.
func (s mdFence) Indent() Indent {
	if s.inner == nil {
		return mdIndent{}
	}

	return s.inner.Indent()
}

// mdIndentOf 는 줄 앞의 빈 칸 수다. 코드펜스·제목·표가 다 이 자리부터 시작한다.
func mdIndentOf(line []byte) int {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}

	return indent
}

func (s mdNormal) Lex(line []byte) ([]Token, State) {
	indent := mdIndentOf(line)

	// 코드펜스를 연다. 안쪽은 markdown 이 아니므로 문맥이 바뀐다.
	if marker, size, ok := mdFenceAt(line, indent); ok {
		tokens := []Token{{Start: indent, End: indent + size, Kind: KindKeyword}}

		// 여는 표시 뒤의 낱말은 그 안이 어느 언어인지다. 색을 주고, 그 언어의 시작 문맥을
		// 들고 간다. 모르는 언어면 nil 이라 안쪽에 색이 없다(ADR-0040).
		var inner State
		if info := bytes.TrimSpace(line[indent+size:]); len(info) > 0 {
			at := indent + size + bytes.Index(line[indent+size:], info)
			tokens = append(tokens, Token{Start: at, End: at + len(info), Kind: KindType})
			inner = mdInfoLanguage(info)
		}

		return tokens, mdFence{marker: marker, size: size, inner: inner}
	}

	// 제목은 줄 하나가 통째로 제목이다. 안의 코드 스팬까지 가르지 않는다 — 제목 줄에서
	// 굵기가 끊기면 어디까지가 제목인지 흐려진다.
	if end := mdHeadingEnd(line, indent); end > indent {
		return []Token{{Start: 0, End: len(line), Kind: KindHeading}}, mdNormal{}
	}

	// 인용문도 줄 단위다. 인용은 곧 남의 말이라 주석과 같은 자리에 둔다.
	if indent < len(line) && line[indent] == '>' {
		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, mdNormal{}
	}

	// 참조 링크의 정의(`[ref]: 주소`) 다. 줄 하나가 통째로 정의라 안을 더 훑지 않는다.
	if from, to, ok := mdLinkDefinition(line, indent); ok {
		return []Token{{Start: from, End: to, Kind: KindLink}}, mdNormal{}
	}

	// 표의 한 행이다. 이어지는 `|` 줄 중 첫 줄이 머리 행이다.
	//
	// **여기서 잰 indent 를 쓰지 않는다.** 표는 목록 안에서 깊이 들여쓰이고 tab 으로도
	// 들여쓰이는데, 위의 indent 는 코드펜스의 자라 셋에서 끊기고 tab 을 세지 않는다.
	if at, ok := mdTableRowAt(line); ok {
		return mdLexTableRow(line, at, !s.afterTableRow), mdNormal{afterTableRow: true}
	}

	return mdLexInline(line), mdNormal{}
}

func (s mdFence) Lex(line []byte) ([]Token, State) {
	// **닫는 표시를 안쪽 언어보다 먼저 본다.** 이 순서가 아니면 안쪽에서 열린 raw string 이나
	// 블록 주석이 닫는 펜스를 먹어서 펜스가 영영 닫히지 않는다 — 그때부터 문서 나머지가
	// 코드로 그려진다(ADR-0040).
	//
	// 닫는 표시는 같은 글자로, 여는 것보다 짧지 않게, 그 뒤에 아무것도 없어야 한다.
	indent := mdIndentOf(line)

	run := 0
	for indent+run < len(line) && line[indent+run] == s.marker {
		run++
	}

	if indent <= mdMaxFenceIndent && run >= s.size &&
		len(bytes.TrimSpace(line[indent+run:])) < 1 {
		// 안쪽 문맥은 버린다. 펜스가 닫히면 안쪽에서 열려 있던 것도 같이 끝난다.
		return []Token{{Start: indent, End: indent + run, Kind: KindKeyword}}, mdNormal{}
	}

	// 언어를 모르면 색이 없다. 통째로 문자열 색을 주면 코드블록이 서른 줄씩 한 색으로 덮여
	// 안 넣는 것보다 나쁘다.
	if s.inner == nil {
		return nil, s
	}

	// 줄을 통째로 맡긴다. 잘라낸 것이 없으니 자리를 되돌릴 것도 없다.
	//
	// 여는 펜스의 들여쓰기를 벗기지 않는다. CommonMark 는 벗기는데, 그러려면 칸이 하나 더
	// 붙어 모든 캐시 줄의 상태 사슬에 실린다. 우리 lexer 중 앞 공백을 보는 것은 makefile 의
	// 조리법 판정뿐이라 얻는 것이 거의 없다(docs/tasks.md).
	tokens, inner := s.inner.Lex(line)

	return tokens, mdFence{marker: s.marker, size: s.size, inner: inner}
}

// mdInfoLanguage 는 info string 이 가리키는 언어다.
//
// 첫 낱말만 본다(CommonMark) — ```go title=x 처럼 뒤에 무엇이 붙는 것이 있다.
// 색은 지금처럼 info string 전체에 붙는다. 그건 위임과 무관한 시각 결정이다(docs/tasks.md).
func mdInfoLanguage(info []byte) State {
	if cut := bytes.IndexAny(info, " \t"); cut >= 0 {
		info = info[:cut]
	}

	return languageByName(string(info))
}

// mdFenceAt 은 그 자리에서 코드펜스가 열리는지다.
func mdFenceAt(line []byte, indent int) (marker byte, size int, ok bool) {
	if indent > mdMaxFenceIndent || indent >= len(line) {
		return 0, 0, false
	}

	marker = line[indent]
	if marker != '`' && marker != '~' {
		return 0, 0, false
	}

	for indent+size < len(line) && line[indent+size] == marker {
		size++
	}

	return marker, size, size >= 3
}

// mdHeadingEnd 는 `#` 이 몇 개인지다. 제목이 아니면 indent 를 그대로 돌려준다.
//
// `#` 뒤에는 빈 칸이 오거나 줄이 끝나야 한다. 그러지 않으면 `#hashtag` 가 제목이 된다.
func mdHeadingEnd(line []byte, indent int) int {
	at := indent
	for at < len(line) && line[at] == '#' && at-indent < 6 {
		at++
	}

	if at == indent {
		return indent
	}
	if at < len(line) && line[at] != ' ' {
		return indent
	}

	return at
}

// mdTableIndent 는 표를 찾을 때 건너뛰는 앞 빈 칸이다.
//
// **깊이에 상한이 없고 tab 도 센다.** 코드펜스·제목과 다른 자다 — 그쪽은 넉 칸부터 들여쓴
// 코드블록이라 CommonMark 가 셋으로 막는데(mdMaxFenceIndent), 우리는 들여쓴 코드블록을
// 아예 보지 않기로 했으므로(ADR-0105) 표와 다툴 읽기가 없다.
//
// 목록 안의 표는 목록만큼 들어가 있고, 그 들여쓰기는 넉 칸을 쉽게 넘는다. tab 을 세는 것도
// 같은 까닭이다 — 이 저장소의 목록이 tab 으로 들여쓰여 있다.
func mdTableIndent(line []byte) int {
	at := 0
	for at < len(line) && (line[at] == ' ' || line[at] == '\t') {
		at++
	}

	return at
}

// mdTableRowAt 은 그 줄이 표의 한 행인지와, 그렇다면 `|` 가 시작하는 자리다.
//
// **`|` 로 시작하는 줄만 본다.** GFM 은 바깥 `|` 를 생략해도 표로 읽지만(`a | b`), 그러면
// 산문의 `a | b` 와 갈리지 않는다. 시작 표시를 요구하면 줄 하나만 보고 정할 수 있어서
// 표의 어느 자리인지를 문맥에 들고 다닐 필요가 없다.
//
// `|` 가 하나 더 있어야 한다. 칸을 가르는 것이 표라, 하나뿐이면 표가 아니다.
func mdTableRowAt(line []byte) (int, bool) {
	indent := mdTableIndent(line)
	if indent >= len(line) || line[indent] != '|' {
		return 0, false
	}

	return indent, bytes.IndexByte(line[indent+1:], '|') >= 0
}

// mdTableDelimiterAt 은 그 줄이 표의 구분줄(`|---|:--:|`) 인지다. 통째로 표시라 안을 훑지 않는다.
func mdTableDelimiterAt(line []byte, indent int) bool {
	if indent >= len(line) {
		return false
	}

	dash := false
	for _, c := range line[indent:] {
		switch c {
		case '-':
			dash = true
		case '|', ':', ' ', '\t':
		default:
			return false
		}
	}

	return dash
}

// mdTablePipes 는 칸을 가르는 `|` 의 자리다. inline 은 그 줄을 훑은 결과다.
//
// **인라인 토큰 사이만 본다.** 코드 스팬 안의 `|`(“ `a | b` “) 는 칸을 가르지 않는다.
// 앞에 `\` 가 붙은 것도 아니다 — GFM 이 칸 안에 `|` 를 적는 길로 둔 것이다.
//
// 이 답을 강조와 표 정리(core 의 `표 맞추기`) 가 같이 쓴다. 갈라 두면 화면에서 칸으로
// 보이던 자리와 실제로 잘리는 자리가 달라진다.
func mdTablePipes(line []byte, inline []Token) []int {
	pipes := []int{}

	add := func(from, to int) {
		for at := from; at < to; at++ {
			if line[at] == '|' && (at == 0 || line[at-1] != '\\') {
				pipes = append(pipes, at)
			}
		}
	}

	at := 0
	for _, token := range inline {
		add(at, token.Start)
		at = max(at, token.End)
	}

	add(at, len(line))

	return pipes
}

// mdLexTableRow 는 표의 한 행이다. head 면 머리 행이라 칸의 글을 굵게 그린다.
//
// head 를 받는 것은 그것만이 이 줄에서 알 수 없는 사실이기 때문이다(mdNormal 의 afterTableRow).
//
// 칸 안의 글은 여느 줄과 같이 훑는다. 코드 스팬·굵게·링크가 표 안에서도 그대로 들어야 한다.
func mdLexTableRow(line []byte, indent int, head bool) []Token {
	// 들여쓰기는 표시가 아니다. `|` 부터가 표다.
	if mdTableDelimiterAt(line, indent) {
		return []Token{{Start: indent, End: len(line), Kind: KindKeyword}}
	}

	inline := mdLexInline(line)
	pipes := mdTablePipes(line, inline)

	tokens := make([]Token, 0, len(inline)+2*len(pipes)+1)

	// 앞의 빈 칸은 칸 글이 아니라 들여쓰기다. 머리 행에서 그것까지 굵게 하지 않는다.
	at, next := indent, 0

	// `|` 와 인라인 토큰을 자리 차례로 섞는다. 둘 다 앞에서 뒤로 정렬되어 있다.
	for _, pipe := range pipes {
		for next < len(inline) && inline[next].Start < pipe {
			tokens = mdAppendCellText(tokens, at, inline[next].Start, head)
			tokens = append(tokens, inline[next])
			at = inline[next].End
			next++
		}

		tokens = mdAppendCellText(tokens, at, pipe, head)
		tokens = append(tokens, Token{Start: pipe, End: pipe + 1, Kind: KindKeyword})
		at = pipe + 1
	}

	for ; next < len(inline); next++ {
		tokens = mdAppendCellText(tokens, at, inline[next].Start, head)
		tokens = append(tokens, inline[next])
		at = inline[next].End
	}

	return mdAppendCellText(tokens, at, len(line), head)
}

// mdAppendCellText 는 칸 안의 맨 글을 담는다. 머리 행에서만 갈래가 붙는다.
func mdAppendCellText(tokens []Token, from, to int, head bool) []Token {
	if !head || from >= to {
		return tokens
	}

	return append(tokens, Token{Start: from, End: to, Kind: KindStrong})
}

// MarkdownTableCells 는 표 한 행의 칸 글이다. 표의 행이 아니면 ok 가 false 다.
//
// state 는 그 줄을 **시작한** 문맥이다(core 의 syntaxStateAt). markdown 의 보통 문맥이
// 아니면 표가 아니다 — 다른 언어의 파일도, 코드펜스 안의 `|` 줄도 여기서 갈린다. 코드
// 예시 안의 표를 고쳐 쓰면 그것은 남의 글을 바꾸는 일이다.
//
// 칸 글은 앞뒤 빈 칸을 뗀 것이다. 바깥 `|` 밖은 담지 않는다.
func MarkdownTableCells(state State, line []byte) (cells []string, delimiter, ok bool) {
	if _, markdown := state.(mdNormal); !markdown {
		return nil, false, false
	}

	indent, ok := mdTableRowAt(line)
	if !ok {
		return nil, false, false
	}

	pipes := mdTablePipes(line, mdLexInline(line))

	cells = []string{}
	for i := 0; i+1 < len(pipes); i++ {
		cells = append(cells, string(bytes.TrimSpace(line[pipes[i]+1:pipes[i+1]])))
	}

	// 바깥 `|` 를 닫지 않은 행(`| a | b`) 의 마지막 칸이다.
	if tail := bytes.TrimSpace(line[pipes[len(pipes)-1]+1:]); len(tail) > 0 {
		cells = append(cells, string(tail))
	}

	return cells, mdTableDelimiterAt(line, indent), true
}

// mdLinkDefinition 은 참조 링크를 정의하는 줄(`[ref]: 주소 "제목"`) 의 주소 자리다.
//
// 주소에만 색을 준다. `[ref]` 는 사람이 본문에서 다시 부를 이름이고 주소는 기계가 읽는
// 것이라, 인라인 링크에서 가른 자리와 같다(mdLexInline).
func mdLinkDefinition(line []byte, indent int) (from, to int, ok bool) {
	if indent > mdMaxFenceIndent || indent >= len(line) || line[indent] != '[' {
		return 0, 0, false
	}

	closeAt := bytes.IndexByte(line[indent:], ']')
	if closeAt < 2 || indent+closeAt+1 >= len(line) || line[indent+closeAt+1] != ':' {
		return 0, 0, false
	}

	rest := line[indent+closeAt+2:]

	dest := bytes.TrimLeft(rest, " \t")
	if len(dest) < 1 {
		return 0, 0, false
	}

	from = len(line) - len(dest)

	to = from
	for to < len(line) && line[to] != ' ' && line[to] != '\t' {
		to++
	}

	return from, to, true
}

// mdLexInline 은 줄 안의 것들을 훑는다.
//
// 앞에서 뒤로 한 번만 지나간다. 코드 스팬이 가장 세다 — 그 안의 `*` 는 강조가 아니다.
//
// 링크는 주소에만 색을 준다. `[글]` 의 글은 읽는 사람에게 보일 문장이라 본문과 같이 두고,
// 주소는 기계가 읽는 것이라 갈라 놓는다. vim 의 markdown 문법도 같은 자리를 칠한다.
// 목록 표시(`-` `1.`) 와 굵은 제목선(`===`) 은 일부러 두고 본다: 이 저장소의 markdown 은
// 거의 전부 목록이라, 표시마다 색이 붙으면 글이 점으로 뒤덮인다.
func mdLexInline(line []byte) []Token {
	tokens := []Token{}

	for at := 0; at < len(line); {
		switch line[at] {
		case '`':
			if end, ok := mdCodeSpanEnd(line, at); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				at = end

				continue
			}
		case '[':
			if from, to, end, ok := mdLinkDest(line, at); ok {
				// `[글][]` 는 칠할 이름이 없다. 자리만 지나간다.
				if to > from {
					tokens = append(tokens, Token{Start: from, End: to, Kind: KindLink})
				}

				at = end

				continue
			}
		case '<':
			if end, ok := mdAutolinkEnd(line, at); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindLink})
				at = end

				continue
			}
		case '*', '_':
			if end, run, ok := mdEmphasisEnd(line, at); ok {
				// 표시 하나는 기울임, 둘은 굵기다. 쓴 사람이 고른 표시를 화면이 따라간다.
				kind := KindEmphasis
				if run > 1 {
					kind = KindStrong
				}

				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
				at = end

				continue
			}
		case '~':
			// **표시가 둘일 때만 받는다.** `~` 하나는 홈 경로(`~/.config`) 와 범위(`1~2`) 에
			// 홀로 쓰이는 글자라, 열어 주면 그 뒤 아무 `~` 까지가 통째로 그어진다.
			if end, run, ok := mdEmphasisEnd(line, at); ok && run == 2 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindStrikethrough})
				at = end

				continue
			}
		}

		at++
	}

	return tokens
}

// mdCodeSpanEnd 는 코드 스팬이 끝나는 자리다. 여는 것과 같은 수의 백틱으로 닫힌다.
func mdCodeSpanEnd(line []byte, at int) (int, bool) {
	open := 0
	for at+open < len(line) && line[at+open] == '`' {
		open++
	}

	for i := at + open; i < len(line); {
		if line[i] != '`' {
			i++

			continue
		}

		run := 0
		for i+run < len(line) && line[i+run] == '`' {
			run++
		}
		if run == open {
			return i + run, true
		}

		i += run
	}

	return 0, false
}

// mdLinkDest 는 `[글](주소)` 의 주소 자리다. 글은 그냥 글이라 색을 주지 않는다.
//
// 참조 링크(`[글][ref]`) 의 이름도 여기다. 그 이름이 주소가 적힌 자리를 가리키므로 주소와
// 같은 갈래로 본다. `[글][]` 처럼 이름이 비면 칠할 것이 없어 from 과 to 가 같다.
//
// **`[ref]` 하나만 적는 꼴(shortcut) 은 받지 않는다.** 그냥 대괄호로 묶은 글과 생김새가
// 같아서, 받으면 이 저장소의 할 일 표시(`- [ ]` `- [x]`) 가 전부 링크가 된다.
func mdLinkDest(line []byte, at int) (from, to, end int, ok bool) {
	closeAt := bytes.IndexByte(line[at:], ']')
	if closeAt < 0 || at+closeAt+1 >= len(line) {
		return 0, 0, 0, false
	}

	switch line[at+closeAt+1] {
	case '(':
		from = at + closeAt + 2

		paren := bytes.IndexByte(line[from:], ')')
		if paren < 0 {
			return 0, 0, 0, false
		}

		to = from + paren
		if from >= to {
			return 0, 0, 0, false
		}

		return from, to, to + 1, true

	case '[':
		from = at + closeAt + 2

		bracket := bytes.IndexByte(line[from:], ']')
		if bracket < 0 {
			return 0, 0, 0, false
		}

		to = from + bracket

		return from, to, to + 1, true
	}

	return 0, 0, 0, false
}

// mdAutolinkEnd 는 `<http://...>` 가 끝나는 자리다. 꺾쇠 안에 빈 칸이 있으면 링크가 아니다.
func mdAutolinkEnd(line []byte, at int) (int, bool) {
	closeAt := bytes.IndexByte(line[at:], '>')
	if closeAt < 2 {
		return 0, false
	}

	inner := line[at+1 : at+closeAt]
	if bytes.IndexByte(inner, ' ') >= 0 || bytes.IndexByte(inner, ':') < 0 {
		return 0, false
	}

	return at + closeAt + 1, true
}

// mdEmphasisEnd 는 강조가 끝나는 자리와 표시의 개수다. 개수가 기울임(1) 과 굵기(2) 를 가른다.
//
// 취소선(`~~`) 도 이것을 쓴다. 표시 글자만 다르고 찾는 법이 같아서다. 몇 개를 받을지는
// 부르는 쪽이 정한다 — `~` 는 둘만이다(mdLexInline).
//
// 여는 표시 뒤와 닫는 표시 앞에 빈 칸이 없어야 한다.
//
// 그 규칙이 없으면 곱셈 기호나 snake_case 의 밑줄이 강조를 연다. CommonMark 의 규칙보다
// 훨씬 얕지만, 틀렸을 때 피해가 그 줄에서 멈춘다 — 강조는 줄을 넘지 않는다.
func mdEmphasisEnd(line []byte, at int) (end, run int, ok bool) {
	marker := line[at]

	// 밑줄은 낱말 안에서 강조를 열지 않는다. 그러지 않으면 snake_case 가 강조가 된다 —
	// 이 저장소의 markdown 은 코드 이름을 그대로 적는 자리가 많아서 바로 드러난다.
	// 별표는 낱말 안에서도 연다(CommonMark 와 같다).
	if marker == '_' && at > 0 && mdIsWord(line[at-1]) {
		return 0, 0, false
	}

	open := 0
	for at+open < len(line) && line[at+open] == marker && open < 2 {
		open++
	}

	body := at + open
	if body >= len(line) || line[body] == ' ' {
		return 0, 0, false
	}

	for i := body; i+open <= len(line); i++ {
		if line[i] != marker {
			continue
		}

		closing := 0
		for i+closing < len(line) && line[i+closing] == marker {
			closing++
		}

		closes := closing == open && line[i-1] != ' '
		if closes && marker == '_' && i+closing < len(line) && mdIsWord(line[i+closing]) {
			closes = false
		}
		if !closes {
			i += closing - 1

			continue
		}

		return i + closing, open, true
	}

	return 0, 0, false
}

// mdIsWord 는 낱말을 이루는 byte 인지다. 한글처럼 여러 byte 인 글자는 이어지는 byte 가
// 전부 0x80 이상이라 하나만 봐도 낱말 안이라는 것을 알 수 있다.
func mdIsWord(b byte) bool {
	switch {
	case b >= '0' && b <= '9', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z':
		return true
	case b >= 0x80:
		return true
	}

	return false
}

// mdIndent 는 markdown 의 들여쓰기 규칙이다.
//
// **열한 중 유일하게 공백이 아닌 글자를 낸다.** 목록 안에서 줄을 바꾸면 다음 줄도 그 목록의
// 항목이라, 자리만 맞추고 표시를 손으로 다시 치게 하면 목록을 쓰는 내내 그 일이 되풀이된다.
type mdIndent struct{}

// mdListMark 는 줄 앞의 목록 표시다. 표시가 없으면 빈 것이다.
//
// number 가 0 이 아니면 번호 매긴 목록이고, 그때 mark 는 번호 뒤의 구두점(`.` 또는 `)`) 이다.
type mdListMark struct {
	mark    byte
	number  int
	content bool // 표시 뒤에 내용이 있는지. 빈 항목이면 거짓이다
}

func (mdIndent) Next(line []byte, _ []Token) (int, []byte) {
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}

	mark, ok := mdParseListMark(line[indent:])
	if !ok {
		return 0, nil
	}

	// 빈 항목에서 Enter 를 치면 목록이 끝난다. 표시를 하나 더 내면 빈 항목만 쌓인다.
	if !mark.content {
		return 0, nil
	}

	if mark.number > 0 {
		return 0, fmt.Appendf(nil, "%d%c ", mark.number+1, mark.mark)
	}

	return 0, []byte{mark.mark, ' '}
}

// Close 는 언제나 0 이다. markdown 에 블록을 닫는 표시가 없다.
func (mdIndent) Close(_ []byte) int { return 0 }

// Reindents 는 거짓이다. 목록의 깊이와 네 칸 들여쓴 코드 블록은 글쓴이가 정한 것이라
// 앞 줄에서 되짚을 수 없다 — 되짚으려 들면 중첩 목록이 평평해진다.
func (mdIndent) Reindents() bool { return false }

// TabIndentsLine 은 목록 줄에서 참이다. 항목 가운데에 커서를 두고 tab 을 쳐도 그 항목이
// 통째로 한 단계 깊어진다 — 목록을 쓰다 「이건 하위 항목이다」 싶을 때의 손이다.
//
// 빈 항목(`- ` 만 있는 줄) 도 참이다. 표시를 치자마자 tab 으로 깊이를 잡는 것이 가장 흔하다.
func (mdIndent) TabIndentsLine(line []byte) bool {
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}

	_, ok := mdParseListMark(line[indent:])

	return ok
}

// Unit 은 space 두 칸이다. 목록 규칙이 level 을 내지 않아서 쓰이는 일이 없다.
func (mdIndent) Unit() []byte { return []byte("  ") }

// mdParseListMark 는 들여쓰기를 지난 자리에서 목록 표시를 읽는다.
//
// 표시 뒤에 공백이 반드시 와야 한다. `-단어` 는 목록이 아니라 그냥 글이다. 인용문(`>`) 도
// 여기서 같이 본다 — 다음 줄로 이어지는 것이 목록과 같다.
func mdParseListMark(rest []byte) (mdListMark, bool) {
	if len(rest) < 1 {
		return mdListMark{}, false
	}

	switch rest[0] {
	case '-', '*', '+', '>':
		if len(rest) < 2 || rest[1] != ' ' {
			return mdListMark{}, false
		}

		return mdListMark{
			mark:    rest[0],
			content: len(bytes.TrimSpace(rest[2:])) > 0,
		}, true
	}

	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits < 1 || digits+1 >= len(rest) {
		return mdListMark{}, false
	}
	if rest[digits] != '.' && rest[digits] != ')' {
		return mdListMark{}, false
	}
	if rest[digits+1] != ' ' {
		return mdListMark{}, false
	}

	number, err := strconv.Atoi(string(rest[:digits]))
	if err != nil {
		return mdListMark{}, false
	}

	return mdListMark{
		mark:    rest[digits],
		number:  number,
		content: len(bytes.TrimSpace(rest[digits+2:])) > 0,
	}, true
}

// mdOutline 은 markdown 의 뼈대 규칙이다. 아래 글을 거느리는 것은 제목이다.
//
// **들여쓰기를 보지 않는다.** markdown 의 들여쓰기는 목록의 깊이와 네 칸 코드블록이라
// 글쓴이가 정한 것이고 문서의 뼈대가 아니다 — mdIndent.Reindents 가 거짓인 것과 같은 이유다.
type mdOutline struct{}

// mdHeadingDepth 는 제목의 깊이다. `#` 하나가 0 이고 여섯이 5 다. 제목이 아니면 거짓이다.
//
// **줄보다 토큰을 먼저 본다.** 줄만 보면 코드펜스 안의 `# 주석` 이 제목이 된다 — ```py 안에서
// 그것은 python 주석이다. mdFence.Lex 는 KindHeading 을 내보내지 않으므로(위의 Lex) 그 갈래의
// 토큰이 있는지가 곧 「여기는 펜스 밖이다」다. 펜스를 다시 세는 것이 아니라 이미 센 것을 읽는
// 것이라 값이 0 이다(ADR-0040).
//
// **0 부터 세는 것이 중요하다.** 위로 훑기는 깊이 0 에서 멈추는데, `#` 을 1 로 세면 h1 이 없는
// 문서(이 저장소의 `docs/tasks.md` 가 그렇다) 에서 멈출 자리가 없어진다.
//
// 아직 훑지 않은 줄은 토큰이 비어서 제목이 아닌 것이 된다. 그때 모자라는 것은 머리줄 하나이고
// **틀린 머리줄을 그리지는 않는다** — 어느 쪽으로 틀릴지가 정해져 있는 것이 이 순서의 값이다.
func mdHeadingDepth(line []byte, tokens []Token) (int, bool) {
	if len(tokens) != 1 || tokens[0].Kind != KindHeading {
		return 0, false
	}

	// 제목 토큰은 줄 전체라 자리를 알려주지 않는다. 깊이는 줄에서 다시 센다.
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}

	end := mdHeadingEnd(line, indent)
	if end <= indent {
		return 0, false
	}

	return end - indent - 1, true
}

// Depth 는 제목이면 그 깊이이고, 제목이 아닌 줄은 어느 제목보다도 깊다.
//
// 본문이 가장 깊어야 위로 훑기가 가장 가까운 제목부터 차례로 거둔다.
func (mdOutline) Depth(line []byte, tokens []Token) int {
	depth, ok := mdHeadingDepth(line, tokens)
	if !ok {
		return OutlineDeep
	}

	return depth
}

func (mdOutline) Heads(line []byte, tokens []Token) bool {
	_, ok := mdHeadingDepth(line, tokens)

	return ok
}
