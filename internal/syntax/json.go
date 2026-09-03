package syntax

import "bytes"

// jsonNormal 은 json 의 보통 문맥이다.
//
// **json 계열 넷을 한 lexer 로 훑는다** — `.json`·`.jsonc`·`.json5`·`.hjson` (ADR-0055).
// 넷이 같은 모양에 허용하는 것만 다르고 hjson 이 그중 가장 넓어서, 나누면 거의 같은 lexer 가
// 넷이 되고 그 사이의 경계를 색으로 지키게 된다. **강조는 검사가 아니다** — `.json` 에 적은
// 주석도 주석 색이 입고, 문법이 틀렸다는 것은 그 파일을 읽는 프로그램이 말한다.
type jsonNormal struct{}

// jsonComment 는 `/*` 로 열린 주석 안이다.
type jsonComment struct{}

// jsonQuoted 는 hjson 의 `”'` 로 열린 여러 줄 문자열 안이다.
type jsonQuoted struct{}

func (jsonNormal) Indent() Indent  { return jsonIndent{} }
func (jsonComment) Indent() Indent { return jsonIndent{} }
func (jsonQuoted) Indent() Indent  { return jsonIndent{} }

func (s jsonNormal) Lex(line []byte) ([]Token, State) {
	return jsonLex(line)
}

func (s jsonComment) Lex(line []byte) ([]Token, State) {
	end := bytes.Index(line, []byte("*/"))
	if end < 0 {
		// 빈 줄에 토큰을 내지 않는다. 빈 토큰은 없다(Token 설명).
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, s
	}

	tail, next := lexTail(jsonNormal{}, line, end+2)

	return append([]Token{{Start: 0, End: end + 2, Kind: KindComment}}, tail...), next
}

func (s jsonQuoted) Lex(line []byte) ([]Token, State) {
	end := bytes.Index(line, []byte("'''"))
	if end < 0 {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(jsonNormal{}, line, end+3)

	return append([]Token{{Start: 0, End: end + 3, Kind: KindString}}, tail...), next
}

// jsonWords 는 값으로 쓰이는 낱말이다.
//
// `Infinity`·`NaN` 은 json5 의 것이다. 키로 쓰인 같은 낱말은 아래에서 먼저 걸러지므로
// `{ "null": 1 }` 의 `null` 은 키다.
var jsonWords = map[string]Kind{
	"true": KindConstant, "false": KindConstant, "null": KindConstant,
	"Infinity": KindConstant, "NaN": KindConstant,
}

// jsonLex 는 줄 하나를 훑는다.
//
// 따옴표 없는 값(hjson 의 `key: 한글 그대로`) 은 토큰을 내지 않는다. 어디서 끝나는지가
// 줄 끝이라 색을 입히면 줄 뒤쪽이 통째로 문자열이 되는데, 그것이 정말 문자열인지는 그 파일을
// 읽는 프로그램이 정한다. 비워 두면 아무 색도 없어서 틀릴 여지가 없다.
func jsonLex(line []byte) ([]Token, State) {
	tokens := []Token{}

	for at := 0; at < len(line); {
		switch {
		// `#` 는 hjson, `//` 는 jsonc·json5·hjson 이다.
		case line[at] == '#', line[at] == '/' && at+1 < len(line) && line[at+1] == '/':
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), jsonNormal{}

		case line[at] == '/' && at+1 < len(line) && line[at+1] == '*':
			if end := bytes.Index(line[at+2:], []byte("*/")); end >= 0 {
				to := at + 2 + end + 2
				tokens = append(tokens, Token{Start: at, End: to, Kind: KindComment})
				at = to

				continue
			}

			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), jsonComment{}

		// hjson 의 여러 줄 문자열이다. 한 겹 따옴표보다 먼저 봐야 `'''` 이 빈 문자열 뒤에
		// 따옴표 하나가 남은 것으로 읽히지 않는다.
		case bytes.HasPrefix(line[at:], []byte("'''")):
			if end := bytes.Index(line[at+3:], []byte("'''")); end >= 0 {
				to := at + 3 + end + 3
				tokens = append(tokens, Token{Start: at, End: to, Kind: KindString})
				at = to

				continue
			}

			return append(tokens, Token{Start: at, End: len(line), Kind: KindString}), jsonQuoted{}

		case line[at] == '"', line[at] == '\'':
			end, ok := quotedEnd(line, at, len(line))
			if !ok {
				// 한 겹 따옴표는 줄을 넘지 않는다. 닫히지 않은 것은 치는 도중이라 보고
				// 줄 끝까지 문자열로 둔다 — 다음 줄까지 물들이지는 않는다.
				return append(tokens, Token{Start: at, End: len(line), Kind: KindString}), jsonNormal{}
			}

			kind := KindString
			if jsonBeforeColon(line, end) {
				kind = KindKey
			}

			tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			at = end

			continue

		// 숫자가 이름보다 먼저다. 숫자 글자도 이름 글자라서 순서가 뒤집히면 `1.5` 가 이름이 된다.
		case jsonDigit(line[at]),
			line[at] == '-' && at+1 < len(line) && jsonDigit(line[at+1]):
			end := jsonNumberEnd(line, at)

			// 숫자 뒤에 이름 글자가 붙어 있으면 따옴표 없는 값이다(`1abc`). 숫자가 아니다.
			if end < len(line) && isIdentByte(line[end]) {
				at = identEnd(line, end, len(line))

				continue
			}

			tokens = append(tokens, Token{Start: at, End: end, Kind: KindNumber})
			at = end

			continue

		case isIdentByte(line[at]):
			end := identEnd(line, at, len(line))

			switch {
			case jsonBeforeColon(line, end):
				// 따옴표 없는 키다. hjson·json5 가 이것을 쓴다.
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindKey})
			default:
				if kind, ok := jsonWords[string(line[at:end])]; ok {
					tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
				}
			}

			at = end

			continue
		}

		at++
	}

	return tokens, jsonNormal{}
}

// jsonBeforeColon 은 그 자리 뒤에 `:` 가 오는지다. 오면 방금 끝난 것은 값이 아니라 키다.
//
// 빈 칸만 건너뛴다. `:` 가 다음 줄에 있는 키(줄을 넘겨 적은 것) 는 문자열로 그려진다 —
// 이 lexer 는 줄 하나만 보고, 줄을 넘겨서까지 키를 찾으려면 문맥이 하나 더 늘어난다.
func jsonBeforeColon(line []byte, at int) bool {
	for ; at < len(line); at++ {
		switch line[at] {
		case ' ', '\t':
			continue
		case ':':
			return true
		}

		return false
	}

	return false
}

// jsonNumberEnd 는 숫자가 끝나는 자리다.
//
// json 의 `-1.5e+10` 과 json5 의 `0x1f`·`.5` 를 받는다. 자릿수를 세지 않는다 — 값이 맞는지는
// 그 파일을 읽는 프로그램이 보고, 여기는 어디까지가 숫자인지만 정한다.
func jsonNumberEnd(line []byte, at int) int {
	end := at
	if end < len(line) && (line[end] == '-' || line[end] == '+') {
		end++
	}

	// `0x1f` 는 json5 다. 여기서 끝내야 뒤의 `f` 가 이름으로 새지 않는다.
	if end+1 < len(line) && line[end] == '0' && (line[end+1] == 'x' || line[end+1] == 'X') {
		hex := end + 2
		for hex < len(line) && jsonHexDigit(line[hex]) {
			hex++
		}
		if hex > end+2 {
			return hex
		}
	}

	digits := end
	for digits < len(line) && (jsonDigit(line[digits]) || line[digits] == '.') {
		digits++
	}
	if digits == end {
		return at
	}
	end = digits

	// 지수부는 숫자가 하나라도 있어야 딸려 온다. `1e` 는 `1` 까지다.
	if end < len(line) && (line[end] == 'e' || line[end] == 'E') {
		exp := end + 1
		if exp < len(line) && (line[exp] == '-' || line[exp] == '+') {
			exp++
		}

		to := exp
		for to < len(line) && jsonDigit(line[to]) {
			to++
		}
		if to > exp {
			end = to
		}
	}

	return end
}

func jsonDigit(b byte) bool { return b >= '0' && b <= '9' }

func jsonHexDigit(b byte) bool {
	switch {
	case jsonDigit(b), b >= 'a' && b <= 'f', b >= 'A' && b <= 'F':
		return true
	}

	return false
}

// jsonIndent 는 json 계열의 들여쓰기 규칙이다. 여는 괄호가 블록을 연다.
//
// braceIndent 를 나눠 쓰지 않는다. 그것은 줄 끝의 `:` 를 switch 이름표로도 보는데(go·js 의
// `case 1:`) json 에는 switch 가 없어서, `case` 라는 키 하나가 이름표로 잡혀 그 아래가
// 들여쓰인다. 모양이 같아 보이는 것과 규칙이 같은 것은 다르다.
type jsonIndent struct{}

func (jsonIndent) Next(line []byte, tokens []Token) (int, []byte) {
	if bracketOpens(codeBytes(line, tokens), braceOpen, braceClose) {
		return 1, nil
	}

	return 0, nil
}

func (jsonIndent) Close(head []byte) int {
	if closesBracket(head, braceClose) {
		return 1
	}

	return 0
}

func (jsonIndent) Reindents() bool { return true }

func (jsonIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 space 두 칸이다. `npm`·`go mod` 가 내는 json 이 그렇고, 설정 파일은 깊어서 좁은
// 쪽이 읽힌다.
func (jsonIndent) Unit() []byte { return []byte("  ") }
