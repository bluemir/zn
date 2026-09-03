package syntax

import (
	"bytes"
	"slices"
	"strings"
)

// pyNormal 은 Python 의 보통 문맥이다.
type pyNormal struct{}

// pyTripleQuote 는 세 겹 따옴표 문자열 안이다. 여는 따옴표 글자를 든다 —
// `"""` 는 `”'` 로 닫히지 않는다.
type pyTripleQuote struct {
	quote byte
}

func (pyNormal) Indent() Indent      { return pyIndent{} }
func (pyTripleQuote) Indent() Indent { return pyIndent{} }

func (s pyNormal) Lex(line []byte) ([]Token, State) {
	tokens := []Token{}

	for at := 0; at < len(line); {
		switch {
		case line[at] == '#':
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), s

		case line[at] == '@' && pyDecoratorAt(line, at):
			end := identEnd(line, at+1, len(line))
			for end < len(line) && line[end] == '.' {
				end = identEnd(line, end+1, len(line))
			}
			if end > at+1 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindFunction})
				at = end

				continue
			}

		case line[at] == '"', line[at] == '\'':
			token, next, end := pyLexString(line, at, at)
			tokens = append(tokens, token)
			if next != nil {
				return tokens, next
			}
			at = end

			continue

		case isIdentByte(line[at]) && !pyDigit(line[at]):
			end := identEnd(line, at, len(line))
			word := string(line[at:end])

			// 이름 앞에 따옴표 접두가 붙는 문자열이다 — `r'\d+'` `f"{x}"` `b"..."`.
			if end < len(line) && (line[end] == '"' || line[end] == '\'') && pyStringPrefix(word) {
				token, next, tail := pyLexString(line, end, at)
				tokens = append(tokens, token)
				if next != nil {
					return tokens, next
				}
				at = tail

				continue
			}

			if kind, ok := pyKeywords[word]; ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			} else if kind, ok := pyNamedAfter(tokens, line, at); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			}

			at = end

			continue

		case pyDigit(line[at]):
			end := pyNumberEnd(line, at)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindNumber})
			at = end

			continue
		}

		at++
	}

	return tokens, s
}

func (s pyTripleQuote) Lex(line []byte) ([]Token, State) {
	closing := bytes.Repeat([]byte{s.quote}, 3)

	end := bytes.Index(line, closing)
	if end < 0 {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(pyNormal{}, line, end+3)

	return append([]Token{{Start: 0, End: end + 3, Kind: KindString}}, tail...), next
}

// pyLexString 은 문자열 하나를 훑는다. quote 는 따옴표 자리이고 from 은 접두를 포함한 시작이다.
//
// 세 겹 따옴표가 그 줄에서 닫히지 않으면 next 가 그 문맥이다.
// f-string 안의 `{...}` 는 가르지 않는다 — 통째로 문자열이다(docs/tasks.md).
func pyLexString(line []byte, quote, from int) (Token, State, int) {
	mark := line[quote]

	if quote+2 < len(line) && line[quote+1] == mark && line[quote+2] == mark {
		closing := bytes.Repeat([]byte{mark}, 3)

		if end := bytes.Index(line[quote+3:], closing); end >= 0 {
			to := quote + 3 + end + 3

			return Token{Start: from, End: to, Kind: KindString}, nil, to
		}

		return Token{Start: from, End: len(line), Kind: KindString}, pyTripleQuote{quote: mark}, len(line)
	}

	// 한 겹 따옴표는 줄을 넘지 못한다. 닫히지 않아도 문맥을 바꾸지 않는다.
	if end, ok := quotedEnd(line, quote, len(line)); ok {
		return Token{Start: from, End: end, Kind: KindString}, nil, end
	}

	return Token{Start: from, End: len(line), Kind: KindString}, nil, len(line)
}

// pyNamedAfter 는 앞의 예약어가 이 이름의 갈래를 정하는지다.
// `def 이름` 은 부르는 이름이고 `class 이름` 은 type 이다.
func pyNamedAfter(tokens []Token, line []byte, at int) (Kind, bool) {
	if len(tokens) < 1 {
		return KindPlain, false
	}

	last := tokens[len(tokens)-1]
	if last.Kind != KindKeyword || last.End >= at {
		return KindPlain, false
	}

	// 사이에 빈 칸만 있어야 한다. `def 이름` 은 붙어 있고 `def a, 이름` 은 아니다.
	if len(bytes.Trim(line[last.End:at], " ")) > 0 {
		return KindPlain, false
	}

	switch string(line[last.Start:last.End]) {
	case "def":
		return KindFunction, true
	case "class":
		return KindType, true
	}

	return KindPlain, false
}

// pyDecoratorAt 은 `@` 가 줄 앞(들여쓰기 뒤) 에 있는지다. 그 자리만 장식자다.
func pyDecoratorAt(line []byte, at int) bool {
	return len(bytes.TrimLeft(line[:at], " \t")) < 1
}

// pyStringPrefix 는 따옴표 앞에 붙을 수 있는 접두인지다. `r` `b` `f` `u` 와 그 조합이다.
func pyStringPrefix(word string) bool {
	if len(word) > 2 {
		return false
	}

	for _, c := range strings.ToLower(word) {
		if !strings.ContainsRune("rbfu", c) {
			return false
		}
	}

	return len(word) > 0
}

func pyDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// pyNumberEnd 는 숫자가 끝나는 자리다. `0x1f` `1_000` `1e9` `3j` 를 받는다.
func pyNumberEnd(line []byte, at int) int {
	end := at
	for end < len(line) {
		c := line[end]
		switch {
		case pyDigit(c), c == '_', c == '.':
			end++
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F', c == 'x', c == 'X',
			c == 'o', c == 'O', c == 'b', c == 'B', c == 'j', c == 'J':
			end++
		case c == '+' || c == '-':
			// 지수 뒤의 부호만 숫자에 든다. `1e-9` 는 하나고 `1-9` 는 셋이다.
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

// pyKeywords 는 예약어와 그 갈래다. `True` 는 낱말 표에 있지만 값이라 갈래가 다르다.
//
// `match`·`case` 는 넣지 않았다 — soft keyword 라 `match = 1` 처럼 예약어가 아닌 자리가
// 흔하고, 그것까지 색이 든다.
var pyKeywords = map[string]Kind{
	"and": KindKeyword, "as": KindKeyword, "assert": KindKeyword, "async": KindKeyword,
	"await": KindKeyword, "break": KindKeyword, "class": KindKeyword, "continue": KindKeyword,
	"def": KindKeyword, "del": KindKeyword, "elif": KindKeyword, "else": KindKeyword,
	"except": KindKeyword, "finally": KindKeyword, "for": KindKeyword, "from": KindKeyword,
	"global": KindKeyword, "if": KindKeyword, "import": KindKeyword, "in": KindKeyword,
	"is": KindKeyword, "lambda": KindKeyword, "nonlocal": KindKeyword, "not": KindKeyword,
	"or": KindKeyword, "pass": KindKeyword, "raise": KindKeyword, "return": KindKeyword,
	"try": KindKeyword, "while": KindKeyword, "with": KindKeyword, "yield": KindKeyword,

	"True": KindConstant, "False": KindConstant, "None": KindConstant,
}

// pyIndent 는 python 의 들여쓰기 규칙이다. 블록을 여는 것은 줄 끝의 `:` 다.
type pyIndent struct{}

// pyDedentKeywords 는 앞 블록을 닫고 새로 여는 낱말이다. 그 줄 자체가 한 단계 나온다.
var pyDedentKeywords = []string{"else", "elif", "except", "finally", "case"}

// pyBlockEnders 는 이 낱말로 시작한 줄 다음이 한 단계 나오는 것이다. 흐름이 거기서 끊긴다.
var pyBlockEnders = []string{"return", "pass", "break", "continue", "raise"}

func (pyIndent) Next(line []byte, tokens []Token) (int, []byte) {
	code := codeBytes(line, tokens)
	trimmed := bytes.TrimRight(code, " \t")

	if bytes.HasSuffix(trimmed, []byte{':'}) || bracketOpens(code, braceOpen, braceClose) {
		return 1, nil
	}

	// `return` 뒤에 같은 블록이 이어지는 일은 드물다. vim 도 여기서 내어쓴다.
	if slices.Contains(pyBlockEnders, string(firstWord(code))) {
		return -1, nil
	}

	return 0, nil
}

func (pyIndent) Close(head []byte) int {
	if closesBracket(head, braceClose) || startsWord(head, pyDedentKeywords...) {
		return 1
	}

	return 0
}

func (pyIndent) Reindents() bool { return true }

func (pyIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 PEP 8 이 정한 space 네 칸이다.
func (pyIndent) Unit() []byte { return []byte("    ") }
