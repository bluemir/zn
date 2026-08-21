package syntax

import (
	"bytes"
)

// jsNormal 은 JavaScript 의 보통 문맥이다.
type jsNormal struct{}

// jsBlockComment 는 `/*` 로 열린 주석 안이다.
type jsBlockComment struct{}

// jsTemplate 는 백틱 문자열 안이다.
type jsTemplate struct{}

func (s jsNormal) Lex(line []byte) ([]Token, State) {
	tokens := []Token{}

	for at := 0; at < len(line); {
		switch {
		case line[at] == '/' && at+1 < len(line) && line[at+1] == '/':
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), s

		case line[at] == '/' && at+1 < len(line) && line[at+1] == '*':
			if end := bytes.Index(line[at+2:], []byte("*/")); end >= 0 {
				to := at + 2 + end + 2
				tokens = append(tokens, Token{Start: at, End: to, Kind: KindComment})
				at = to

				continue
			}

			tokens = append(tokens, Token{Start: at, End: len(line), Kind: KindComment})

			return tokens, jsBlockComment{}

		case line[at] == '`':
			// 백틱 문자열은 줄을 넘는다. `${...}` 안까지 가르지는 않는다(docs/tasks.md).
			if end, ok := jsTemplateEnd(line, at+1); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				at = end

				continue
			}

			tokens = append(tokens, Token{Start: at, End: len(line), Kind: KindString})

			return tokens, jsTemplate{}

		case line[at] == '"', line[at] == '\'':
			// 한 겹 따옴표는 줄을 넘지 못한다. 닫히지 않아도 문맥을 바꾸지 않는다.
			end, ok := quotedEnd(line, at, len(line))
			if !ok {
				end = len(line)
			}
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			at = end

			continue

		case jsDigit(line[at]):
			end := jsNumberEnd(line, at)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindNumber})
			at = end

			continue

		case isIdentByte(line[at]):
			end := identEnd(line, at, len(line))
			word := string(line[at:end])

			kind := jsKeywords[word]
			if kind == KindPlain {
				kind = jsNamedAfter(tokens, line, at)
			}
			if kind == KindPlain && end < len(line) && line[end] == '(' {
				kind = KindFunction
			}

			if kind != KindPlain {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			}

			at = end

			continue
		}

		at++
	}

	return tokens, s
}

func (s jsBlockComment) Lex(line []byte) ([]Token, State) {
	end := bytes.Index(line, []byte("*/"))
	if end < 0 {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, s
	}

	tail, next := lexTail(jsNormal{}, line, end+2)

	return append([]Token{{Start: 0, End: end + 2, Kind: KindComment}}, tail...), next
}

func (s jsTemplate) Lex(line []byte) ([]Token, State) {
	end, ok := jsTemplateEnd(line, 0)
	if !ok {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(jsNormal{}, line, end)

	return append([]Token{{Start: 0, End: end, Kind: KindString}}, tail...), next
}

// jsTemplateEnd 는 백틱 문자열이 닫히는 자리다.
func jsTemplateEnd(line []byte, from int) (int, bool) {
	for at := from; at < len(line); at++ {
		switch line[at] {
		case '\\':
			at++
		case '`':
			return at + 1, true
		}
	}

	return 0, false
}

// jsNamedAfter 는 앞의 예약어가 이 이름의 갈래를 정하는지다.
// `class 이름`·`new 이름` 은 type 이고 `function 이름` 은 부르는 이름이다.
func jsNamedAfter(tokens []Token, line []byte, at int) Kind {
	if len(tokens) < 1 {
		return KindPlain
	}

	last := tokens[len(tokens)-1]
	if last.Kind != KindKeyword || last.End >= at {
		return KindPlain
	}
	if len(bytes.Trim(line[last.End:at], " ")) > 0 {
		return KindPlain
	}

	switch string(line[last.Start:last.End]) {
	case "class", "new":
		return KindType
	case "function":
		return KindFunction
	}

	return KindPlain
}

func jsDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// jsNumberEnd 는 숫자가 끝나는 자리다. `0x1f` `1_000n` `1e9` 를 받는다.
func jsNumberEnd(line []byte, at int) int {
	end := at
	for end < len(line) {
		c := line[end]
		switch {
		case jsDigit(c), c == '_', c == '.':
			end++
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F',
			c == 'x', c == 'X', c == 'o', c == 'O', c == 'n':
			end++
		case c == '+' || c == '-':
			if end > at && (line[end-1] == 'e' || line[end-1] == 'E') {
				end++

				continue
			}

			return end
		default:
			return end
		}
	}

	return end
}

// jsKeywords 는 예약어와 그 갈래다. 값인 낱말은 갈래가 다르다.
//
// **정규식 리터럴(`/ab+/`) 은 다루지 않는다.** `/` 가 나눗셈인지 정규식인지는 앞 토큰의 갈래를
// 봐야 갈리고, 반쯤 맞히면 정규식 안의 따옴표가 문자열을 연다. 안 하면 피해가 그 줄에서 멈춘다
// (docs/tasks.md).
var jsKeywords = map[string]Kind{
	"async": KindKeyword, "await": KindKeyword, "break": KindKeyword, "case": KindKeyword,
	"catch": KindKeyword, "class": KindKeyword, "const": KindKeyword, "continue": KindKeyword,
	"debugger": KindKeyword, "default": KindKeyword, "delete": KindKeyword, "do": KindKeyword,
	"else": KindKeyword, "export": KindKeyword, "extends": KindKeyword, "finally": KindKeyword,
	"for": KindKeyword, "function": KindKeyword, "if": KindKeyword, "import": KindKeyword,
	"in": KindKeyword, "instanceof": KindKeyword, "let": KindKeyword, "new": KindKeyword,
	"of": KindKeyword, "return": KindKeyword, "static": KindKeyword, "super": KindKeyword,
	"switch": KindKeyword, "this": KindKeyword, "throw": KindKeyword, "try": KindKeyword,
	"typeof": KindKeyword, "var": KindKeyword, "void": KindKeyword, "while": KindKeyword,
	"with": KindKeyword, "yield": KindKeyword,

	"true": KindConstant, "false": KindConstant, "null": KindConstant,
	"undefined": KindConstant, "NaN": KindConstant, "Infinity": KindConstant,
}
