package syntax

import (
	"bytes"
)

// tomlNormal 은 TOML 의 보통 문맥이다. depth 는 줄을 넘어 열려 있는 `[`·`{` 의 겹 수다.
//
// **겹 수가 있어야 표 머리를 가른다.** 줄 앞의 `[` 는 맨 바깥에서는 표 머리(`[package]`) 이고
// 여러 줄 배열 안에서는 안쪽 배열(`[1, 2],`) 이다. 글자로는 같아서 열려 있는 겹을 들고 간다.
type tomlNormal struct {
	depth int
}

// tomlMultiline 은 세 겹 따옴표(큰따옴표 셋이나 작은따옴표 셋) 로 열린 여러 줄 문자열 안이다. quote 는 연 따옴표이고 depth 는
// 그 문자열을 연 자리의 겹 수다. 닫힌 뒤 그 겹으로 돌아간다.
type tomlMultiline struct {
	quote byte
	depth int
}

func (tomlNormal) Indent() Indent    { return tomlIndent{} }
func (tomlMultiline) Indent() Indent { return tomlIndent{} }

func (s tomlNormal) Lex(line []byte) ([]Token, State) {
	return tomlLex(line, 0, s.depth)
}

func (s tomlMultiline) Lex(line []byte) ([]Token, State) {
	end, closed := tomlMultilineEnd(line, 0, s.quote)
	if !closed {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tokens := []Token{{Start: 0, End: end, Kind: KindString}}
	tail, next := tomlLex(line, end, s.depth)

	return append(tokens, tail...), next
}

// tomlLex 는 줄의 from 부터 훑는다. depth 는 그 자리에서 열려 있는 겹 수다.
func tomlLex(line []byte, from, depth int) ([]Token, State) {
	tokens := []Token{}
	lineStart := true

	for at := from; at < len(line); {
		c := line[at]

		if c == ' ' || c == '\t' || c == '\r' {
			at++

			continue
		}

		first := lineStart && from == 0
		lineStart = false

		switch {
		case c == '#':
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), tomlNormal{depth: depth}

		case c == '[' && first && depth == 0:
			// 맨 바깥 줄 앞의 `[` 는 표 머리다. `[[bin]]` 도 같다. 머리 안의 따옴표 이름
			// (`["a.b"]`) 에 든 `]` 에서 닫지 않는다.
			end := tomlHeaderEnd(line, at)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindHeading})
			at = end

			continue
		}

		// 키는 `=` 앞의 이름이다. 뒤를 보고 가르므로 자리를 따로 셀 일이 없다 — 인라인 표
		// (`{ a = 1 }`) 안의 키도, 여러 줄 배열 안의 문자열(`"a",`) 이 키가 아닌 것도 같은 규칙이다.
		if end, ok := tomlKeyEnd(line, at); ok {
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindKey})
			at = end

			continue
		}

		switch {
		case c == '"' || c == '\'':
			if at+2 < len(line) && line[at+1] == c && line[at+2] == c {
				end, closed := tomlMultilineEnd(line, at+3, c)
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				if !closed {
					return tokens, tomlMultiline{quote: c, depth: depth}
				}

				at = end

				continue
			}

			// 한 겹 따옴표는 줄을 넘지 못한다. 닫히지 않아도 문맥을 바꾸지 않는다.
			end := tomlStringEnd(line, at)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			at = end

			continue

		case c == '[' || c == '{':
			depth++

		case c == ']' || c == '}':
			depth = max(depth-1, 0)

		case c == ',' || c == '=':

		default:
			end := tomlWordEnd(line, at)
			if kind := tomlValueKind(line[at:end]); kind != KindPlain {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			}

			at = end

			continue
		}

		at++
	}

	return tokens, tomlNormal{depth: depth}
}

// tomlHeaderEnd 는 at 에서 연 표 머리가 닫히는 자리다. 닫히지 않으면 주석 앞 또는 줄 끝이다.
func tomlHeaderEnd(line []byte, at int) int {
	for i := at + 1; i < len(line); i++ {
		switch line[i] {
		case '"', '\'':
			i = tomlStringEnd(line, i) - 1
		case ']':
			if i+1 < len(line) && line[i+1] == ']' {
				return i + 2
			}

			return i + 1
		case '#':
			// 닫지 않고 주석이 왔다. 주석은 주석 색이어야 해서 그 앞 빈 칸까지에서 끊는다.
			return at + len(bytes.TrimRight(line[at:i], " \t"))
		}
	}

	return len(line)
}

// tomlKeyEnd 는 at 에서 키가 시작하면 그 끝이다. 뒤에 `=` 가 와야 키다.
//
// 키는 맨이름(`name`·`serde-json`) 이나 따옴표 이름(`"a.b"`) 이고 `.` 으로 잇는다
// (`package.name`·`dependencies."my crate"`). 점 앞뒤의 빈 칸도 문법이 받는다.
func tomlKeyEnd(line []byte, at int) (int, bool) {
	end := at

	for {
		switch {
		case end < len(line) && (line[end] == '"' || line[end] == '\''):
			// 닫히지 않은 따옴표는 키가 아니다. 닫혔으면 마지막 글자가 연 따옴표다.
			close := tomlStringEnd(line, end)
			if close < end+2 || line[close-1] != line[end] {
				return 0, false
			}

			end = close

		case end < len(line) && tomlBareKeyByte(line[end]):
			for end < len(line) && tomlBareKeyByte(line[end]) {
				end++
			}

		default:
			return 0, false
		}

		after := tomlSkipBlank(line, end)
		if after < len(line) && line[after] == '.' {
			end = tomlSkipBlank(line, after+1)

			continue
		}

		if after < len(line) && line[after] == '=' {
			return end, true
		}

		return 0, false
	}
}

// tomlBareKeyByte 는 따옴표 없는 키에 들 수 있는 글자다. `A-Za-z0-9_-` 다.
func tomlBareKeyByte(b byte) bool {
	return jsDigit(b) || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || b == '-'
}

func tomlSkipBlank(line []byte, at int) int {
	for at < len(line) && (line[at] == ' ' || line[at] == '\t') {
		at++
	}

	return at
}

// tomlStringEnd 는 at 의 따옴표로 연 한 줄 문자열이 닫히는 자리다. 닫히지 않으면 줄 끝이다.
//
// `"` 는 `\` 로 escape 하고 `'` 는 literal 이라 escape 가 없다. `'C:\path'` 의 `\` 는 글자다.
func tomlStringEnd(line []byte, at int) int {
	quote := line[at]

	for i := at + 1; i < len(line); i++ {
		switch {
		case line[i] == '\\' && quote == '"':
			i++
		case line[i] == quote:
			return i + 1
		}
	}

	return len(line)
}

// tomlMultilineEnd 는 from 부터 보아 세 겹 따옴표가 닫히는 자리다. 닫히지 않으면 줄 끝과 false 다.
//
// **닫는 자리 앞에 따옴표가 둘까지 더 붙을 수 있다.** `""""a""""` 는 `"a"` 다(TOML 1.0). 그래서
// 세 겹을 만나면 뒤로 붙은 따옴표를 둘까지 더 먹는다.
func tomlMultilineEnd(line []byte, from int, quote byte) (int, bool) {
	for i := from; i < len(line); i++ {
		if line[i] == '\\' && quote == '"' {
			i++

			continue
		}

		if i+2 >= len(line) || line[i] != quote || line[i+1] != quote || line[i+2] != quote {
			continue
		}

		end := i + 3
		for extra := 0; extra < 2 && end < len(line) && line[end] == quote; extra++ {
			end++
		}

		return end, true
	}

	return len(line), false
}

// tomlWordEnd 는 따옴표 없는 값 하나가 끝나는 자리다. 빈 칸·`,`·괄호·주석에서 끝난다.
func tomlWordEnd(line []byte, at int) int {
	end := at
	for end < len(line) {
		switch line[end] {
		case ' ', '\t', '\r', ',', '[', ']', '{', '}', '#', '=':
			return end
		}

		end++
	}

	return end
}

// tomlValueKind 는 따옴표 없는 값의 갈래다.
//
// 수·날짜·시각을 한 갈래(KindNumber) 로 둔다. `1_000`·`0xff`·`3.14`·`1e6`·`inf`·`nan` 과
// `1979-05-27T07:32:00Z`·`07:32:00` 이다. 날짜와 시각 사이에 빈 칸을 둔 꼴(`1979-05-27 07:32:00`)
// 은 두 낱말로 갈리지만 둘 다 숫자로 시작해서 같은 색이 된다.
func tomlValueKind(word []byte) Kind {
	switch string(word) {
	case "true", "false":
		return KindConstant
	case "inf", "+inf", "-inf", "nan", "+nan", "-nan":
		return KindNumber
	}

	rest := word
	if len(rest) > 0 && (rest[0] == '+' || rest[0] == '-') {
		rest = rest[1:]
	}

	if len(rest) > 0 && jsDigit(rest[0]) {
		return KindNumber
	}

	return KindPlain
}

// tomlIndent 는 TOML 의 들여쓰기 규칙이다. 여러 줄 배열과 인라인 표의 괄호만 센다.
//
// 표 머리(`[package]`) 는 같은 줄에서 닫혀서 단계를 열지 않는다. 표 아래 키를 들여쓰지
// 않는 것이 Cargo.toml·pyproject.toml 의 흔한 꼴이다.
type tomlIndent struct{}

func (tomlIndent) Next(line []byte, tokens []Token) (int, []byte) {
	if bracketOpens(codeBytes(line, tokens), "[{", "]}") {
		return 1, nil
	}

	return 0, nil
}

func (tomlIndent) Close(head []byte) int {
	if closesBracket(head, "]}") {
		return 1
	}

	return 0
}

func (tomlIndent) Reindents() bool { return true }

func (tomlIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 space 네 칸이다. Cargo 와 pyproject 의 여러 줄 배열이 흔히 그렇게 쓴다.
func (tomlIndent) Unit() []byte { return []byte("    ") }
