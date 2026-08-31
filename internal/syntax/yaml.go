package syntax

import "bytes"

// yamlNormal 은 yaml 의 보통 문맥이다.
//
// yaml 은 json 과 갈래는 같아도(키·값) 모양이 아주 달라서 lexer 를 따로 둔다(ADR-0055) —
// 괄호가 아니라 들여쓰기가 구조이고, 따옴표 없는 값이 예외가 아니라 보통이다.
type yamlNormal struct{}

// yamlBlock 은 `|`·`>` 로 열린 블록 스칼라 안이다.
//
// indent 는 그 표시가 있던 줄의 들여쓴 칸 수다. 그보다 깊은 줄이 블록의 내용이고, 같거나 얕은
// 줄에서 블록이 끝난다 — yaml 은 블록의 끝을 표시하지 않고 들여쓰기로만 알린다. 빈 줄은
// 얕아 보여도 블록 안이다.
//
// 칸을 세는 것이라 tab 을 따로 보지 않는다. yaml 은 들여쓰기에 tab 을 쓸 수 없다.
type yamlBlock struct {
	indent int
}

func (yamlNormal) Indent() Indent { return yamlIndent{} }
func (yamlBlock) Indent() Indent  { return yamlIndent{} }

func (s yamlNormal) Lex(line []byte) ([]Token, State) {
	tokens := []Token{}
	at := yamlIndentEnd(line)

	// `---`·`...` 는 문서 경계다. 뒤에 무엇이 이어질 수 있어서 지나가고 계속 훑는다.
	if bytes.HasPrefix(line[at:], []byte("---")) || bytes.HasPrefix(line[at:], []byte("...")) {
		tokens = append(tokens, Token{Start: at, End: at + 3, Kind: KindKeyword})
		at = yamlSpaceEnd(line, at+3)
	}

	// 목록 표시를 지난다. `- - a` 처럼 한 줄에 겹칠 수 있다.
	for at < len(line) && line[at] == '-' && yamlBreaksWord(line, at+1) {
		at = yamlSpaceEnd(line, at+1)
	}

	// 키다. 뒤에 값이 이어지거나 그 자리에서 끝난다.
	if end, colon, ok := yamlKeyEnd(line, at); ok {
		tokens = append(tokens, Token{Start: at, End: end, Kind: KindKey})
		at = colon
	}

	value, next := yamlLexValue(line, at)

	return append(tokens, value...), next
}

func (s yamlBlock) Lex(line []byte) ([]Token, State) {
	// 빈 줄은 블록 안이지만 색을 입힐 글자가 없다. 빈 토큰은 없다(Token 설명).
	indent := yamlIndentEnd(line)
	if indent >= len(line) {
		return nil, s
	}

	if indent > s.indent {
		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	// 블록이 끝났다. 이 줄은 보통 줄로 다시 읽는다.
	return yamlNormal{}.Lex(line)
}

// yamlWords 는 값으로 쓰이는 낱말이다. yaml 1.1 의 `yes`·`no`·`on`·`off` 까지 든다 —
// 그것으로 적은 설정 파일이 아직 많다.
var yamlWords = map[string]Kind{
	"true": KindConstant, "false": KindConstant, "null": KindConstant,
	"yes": KindConstant, "no": KindConstant, "on": KindConstant, "off": KindConstant,
	"True": KindConstant, "False": KindConstant, "Null": KindConstant, "NULL": KindConstant,
	"TRUE": KindConstant, "FALSE": KindConstant,
}

// yamlLexValue 는 키 뒤(또는 키가 없는 줄의 처음) 부터 줄 끝까지를 훑는다.
//
// 따옴표 없는 값은 토큰을 내지 않는다. yaml 에서 그것은 대부분의 값이고, 어디서 끝나는지가
// 줄 끝이라 색을 입히면 값이 있는 줄마다 뒤쪽이 통째로 물든다. 낱말로 뜻이 정해진 것
// (`true`·`null`) 과 숫자만 골라 낸다.
func yamlLexValue(line []byte, from int) ([]Token, State) {
	tokens := []Token{}

	for at := from; at < len(line); {
		switch {
		// 주석은 줄 앞이나 빈 칸 뒤에서만 시작한다. `a#b` 는 값 안의 글자다.
		case line[at] == '#' && yamlWordStart(line, at):
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), yamlNormal{}

		case line[at] == '"', line[at] == '\'':
			end, ok := quotedEnd(line, at, len(line))
			if !ok {
				// yaml 의 따옴표는 줄을 넘을 수 있지만 문맥을 하나 더 두지 않았다. 흔하지
				// 않고, 넘어간 다음 줄은 따옴표 없는 값과 같이 색이 없어서 틀리지 않는다.
				return append(tokens, Token{Start: at, End: len(line), Kind: KindString}), yamlNormal{}
			}

			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			at = end

			continue

		// `&이름` 은 자리에 이름을 붙이는 것이고 `*이름` 은 그것을 가리키는 것이다. 가리키는
		// 자리라 키가 아니라 KindVariable 이다(syntax.go 의 KindKey 설명).
		case line[at] == '&' || line[at] == '*':
			end := identEnd(line, at+1, len(line))
			if end > at+1 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindVariable})
				at = end

				continue
			}

		// `!!str`·`!Foo` 는 값의 type 이다.
		case line[at] == '!':
			end := at + 1
			if end < len(line) && line[end] == '!' {
				end++
			}
			end = identEnd(line, end, len(line))
			if end > at+1 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindType})
				at = end

				continue
			}

		// 블록 스칼라를 여는 표시다. 뒤에 주석만 올 수 있고 값이 오면 표시가 아니다.
		case line[at] == '|' || line[at] == '>':
			end := yamlBlockMarkerEnd(line, at)
			if end > at {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindKeyword})

				// 뒤에 주석이 붙을 수 있다. 그것까지 읽고 블록으로 들어간다.
				tail, _ := yamlLexValue(line, end)

				return append(tokens, tail...), yamlBlock{indent: yamlIndentEnd(line)}
			}

		case yamlWordStart(line, at) &&
			(jsonDigit(line[at]) || line[at] == '-' && at+1 < len(line) && jsonDigit(line[at+1])):
			end := jsonNumberEnd(line, at)

			// 값이 숫자로 끝나야 숫자다. `2026-08-23` 은 여기서 걸린다 — 날짜를 숫자 셋으로
			// 쪼개 그리지 않는다.
			if !yamlEndsValue(line, end) {
				at = yamlPlainEnd(line, at)

				continue
			}

			tokens = append(tokens, Token{Start: at, End: end, Kind: KindNumber})
			at = end

			continue

		case isIdentByte(line[at]) && yamlWordStart(line, at):
			end := identEnd(line, at, len(line))

			// 뜻이 정해진 낱말은 **값 전체가 그것일 때만** 낱말이다. `no problem` 의 `no` 는
			// 값의 첫 조각이라 색이 없다.
			if kind, ok := yamlWords[string(line[at:end])]; ok && yamlEndsValue(line, end) {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
				at = end

				continue
			}

			// 따옴표 없는 값은 통째로 지난다. 낱말마다 다시 보면 그 안의 숫자가 값의 일부가
			// 아니라 숫자로 읽힌다(`desc: version 2`).
			at = yamlPlainEnd(line, at)

			continue
		}

		at++
	}

	return tokens, yamlNormal{}
}

// yamlKeyEnd 는 키가 끝나는 자리와 그 뒤 `:` 다음 자리다. 키가 없으면 ok 가 거짓이다.
//
// 키는 `:` 뒤에 빈 칸이나 줄 끝이 오는 것으로 알아본다. yaml 의 규칙이 그렇다 —
// `http://x` 가 키가 아닌 것이 이 규칙 때문이다.
//
// 흐름 모음(`{a: 1}`) 안의 키는 찾지 않는다. 줄 앞이 `{`·`[` 면 키 자리가 아니다.
func yamlKeyEnd(line []byte, from int) (int, int, bool) {
	if from >= len(line) {
		return 0, 0, false
	}
	if line[from] == '{' || line[from] == '[' || line[from] == '#' {
		return 0, 0, false
	}

	// 따옴표로 적은 키는 그 안에 `:` 가 있어도 통째로 이름이다.
	if line[from] == '"' || line[from] == '\'' {
		end, ok := quotedEnd(line, from, len(line))
		if !ok {
			return 0, 0, false
		}

		colon := yamlSpaceEnd(line, end)
		if colon < len(line) && line[colon] == ':' && yamlBreaksWord(line, colon+1) {
			return end, colon + 1, true
		}

		return 0, 0, false
	}

	for at := from; at < len(line); at++ {
		switch line[at] {
		case ':':
			if !yamlBreaksWord(line, at+1) {
				continue
			}
			// `키   :` 처럼 앞에 빈 칸이 있으면 이름에서 뗀다.
			end := len(bytes.TrimRight(line[from:at], " \t")) + from
			if end == from {
				return 0, 0, false
			}

			return end, at + 1, true
		case '#':
			// 주석이 먼저 나왔다. 이 줄에는 키가 없다.
			if yamlWordStart(line, at) {
				return 0, 0, false
			}
		}
	}

	return 0, 0, false
}

// yamlBlockMarkerEnd 는 블록 스칼라 표시가 끝나는 자리다. 표시가 아니면 at 을 그대로 준다.
//
// `|`·`>` 뒤에는 자르기(`-` `+`) 와 들여쓰기 숫자만 올 수 있고, 그다음은 빈 칸이나 주석이나
// 줄 끝이어야 한다. `a | b` 의 `|` 가 표시가 아닌 것이 이 규칙 때문이다.
func yamlBlockMarkerEnd(line []byte, at int) int {
	end := at + 1
	for end < len(line) && (line[end] == '-' || line[end] == '+' || jsonDigit(line[end])) {
		end++
	}

	rest := yamlSpaceEnd(line, end)
	if rest < len(line) && line[rest] != '#' {
		return at
	}

	return end
}

// yamlPlainEnd 는 따옴표 없는 값이 끝나는 자리다.
//
// **빈 칸에서 끊지 않는다** — `version 2` 는 낱말 둘이 아니라 값 하나다. 빈 칸 뒤의 주석과
// 흐름 모음의 구두점, 줄 끝에서 끝난다.
func yamlPlainEnd(line []byte, at int) int {
	for ; at < len(line); at++ {
		switch line[at] {
		case ',', ']', '}':
			return at
		case '#':
			if yamlWordStart(line, at) {
				return at
			}
		}
	}

	return at
}

// yamlEndsValue 는 그 자리에서 값이 끝나는지다.
//
// 뒤에 남은 것이 없거나, 흐름 모음의 구두점이거나, 빈 칸 뒤의 주석이면 끝난 것이다.
// 숫자와 뜻이 정해진 낱말은 값 전체가 그것일 때만 색을 입는다.
func yamlEndsValue(line []byte, at int) bool {
	switch {
	case at >= len(line):
		return true
	case line[at] == ',', line[at] == ']', line[at] == '}':
		return true
	case line[at] != ' ' && line[at] != '\t':
		return false
	}

	rest := yamlSpaceEnd(line, at)

	return rest >= len(line) || line[rest] == '#'
}

// yamlIndentEnd 는 줄 앞 빈 칸이 끝나는 자리다. 공백뿐인 줄이면 줄 길이다.
func yamlIndentEnd(line []byte) int {
	return yamlSpaceEnd(line, 0)
}

// yamlSpaceEnd 는 at 부터 빈 칸을 지난 자리다.
func yamlSpaceEnd(line []byte, at int) int {
	for at < len(line) && (line[at] == ' ' || line[at] == '\t') {
		at++
	}

	return at
}

// yamlBreaksWord 는 그 자리가 낱말의 끝인지다. 줄 끝과 빈 칸이 그렇다.
func yamlBreaksWord(line []byte, at int) bool {
	return at >= len(line) || line[at] == ' ' || line[at] == '\t'
}

// yamlWordStart 는 그 자리가 낱말의 시작인지다. 흐름 모음의 구두점 뒤도 시작이다.
func yamlWordStart(line []byte, at int) bool {
	if at == 0 {
		return true
	}

	switch line[at-1] {
	case ' ', '\t', ',', '[', '{':
		return true
	}

	return false
}

// yamlIndent 는 yaml 의 들여쓰기 규칙이다.
//
// **구조가 곧 들여쓰기다.** 그래서 여는 표시가 없는 언어인데도 「다음 줄이 들어간다」를 말할 수
// 있다 — 값 없는 키(`key:`) 아래가 그 키의 내용이다.
type yamlIndent struct{}

func (yamlIndent) Next(line []byte, tokens []Token) (int, []byte) {
	// **주석만 지운다.** codeBytes 는 문자열까지 지워서 `key: "값"` 이 `key:` 로 보이고,
	// 값이 있는 줄이 값 없는 키로 읽힌다.
	code := bytes.TrimRight(yamlWithoutComment(line, tokens), " \t")
	if len(code) < 1 {
		return 0, nil
	}

	// 블록 스칼라 표시(`|` `>-` `|2`) 뒤는 그 내용이 들어간다.
	if yamlOpensBlock(code) {
		return 1, nil
	}

	// 값 없는 키 아래가 그 키의 내용이다.
	if code[len(code)-1] == ':' {
		return 1, nil
	}

	// 목록 표시만 있는 줄 아래는 그 항목의 내용이다. 끝의 `-` 만으로 보면 `key: 값-` 도 걸린다.
	if bytes.Equal(bytes.TrimLeft(code, " \t"), []byte("-")) {
		return 1, nil
	}

	// 흐름 모음은 괄호가 여닫는다. 문자열 안의 괄호를 세지 않도록 여기만 codeBytes 를 쓴다.
	if bracketOpens(codeBytes(line, tokens), "{[", "}]") {
		return 1, nil
	}

	return 0, nil
}

func (yamlIndent) Close(head []byte) int {
	// 블록을 닫는 표시가 흐름 모음의 괄호뿐이다. 나머지는 들여쓰기를 줄이는 것 자체가 닫는 것이다.
	if closesBracket(head, "}]") {
		return 1
	}

	return 0
}

// Reindents 는 거짓이다. markdown 과 같은 까닭이다 — yaml 에서 **들여쓰기가 곧 뜻**이라
// 규칙만으로 되짚을 수 없다. 되짚으려 들면 중첩된 map 이 평평해져 파일의 뜻이 바뀐다.
func (yamlIndent) Reindents() bool { return false }

// TabIndentsLine 은 거짓이다. yaml 은 들여쓰기에 tab 을 쓸 수 없어서 목록 줄에서도 tab 키로
// 항목을 옮기지 않는다 — markdown 과 갈리는 자리다.
func (yamlIndent) TabIndentsLine([]byte) bool { return false }

// Unit 은 space 두 칸이다. yaml 은 tab 을 들여쓰기로 쓸 수 없어서 관례가 아니라 규칙이다.
func (yamlIndent) Unit() []byte { return []byte("  ") }

// yamlOpensBlock 은 줄이 블록 스칼라 표시로 끝나는지다.
func yamlOpensBlock(code []byte) bool {
	end := len(code)
	for end > 0 && (code[end-1] == '-' || code[end-1] == '+' || jsonDigit(code[end-1])) {
		end--
	}

	return end > 0 && (code[end-1] == '|' || code[end-1] == '>')
}

// yamlWithoutComment 는 주석 자리를 빈 칸으로 지운 줄이다.
//
// **새로 할당한다.** 받는 줄은 Buffer 가 든 read-only data 의 subslice 다(ADR-0001).
func yamlWithoutComment(line []byte, tokens []Token) []byte {
	code := bytes.Clone(line)

	for _, token := range tokens {
		if token.Kind != KindComment {
			continue
		}

		for i := max(token.Start, 0); i < min(token.End, len(code)); i++ {
			code[i] = ' '
		}
	}

	return code
}
