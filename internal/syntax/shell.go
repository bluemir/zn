package syntax

import "bytes"

// shNormal 은 shell 의 보통 문맥이다.
type shNormal struct{}

// shQuoted 는 따옴표 안이다. 여는 따옴표 글자를 든다.
//
// shell 의 따옴표는 **줄을 넘는다.** js·python 의 한 겹 따옴표와 다른 자리다.
//
// shell 이 줄을 넘는 것은 이것과 heredoc 둘인데 heredoc 은 넣지 않았다. `<<` 가
// here-string(`<<<`) 과 산술 shift(`$((a << 2))`) 와 헷갈려 가장 틀리기 쉬운 자리이고,
// Dockerfile 의 `RUN <<EOF` 와 같이 봐야 한다(docs/tasks.md).
type shQuoted struct {
	quote byte
}

func (shNormal) Indent() Indent { return shIndent{} }
func (shQuoted) Indent() Indent { return shIndent{} }

func (s shNormal) Lex(line []byte) ([]Token, State) {
	return shLex(line)
}

func (s shQuoted) Lex(line []byte) ([]Token, State) {
	end := shQuoteEnd(line, 0, s.quote)
	if end < 0 {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(shNormal{}, line, end)

	return append([]Token{{Start: 0, End: end, Kind: KindString}}, tail...), next
}

// shLex 는 줄 하나를 훑는다.
func shLex(line []byte) ([]Token, State) {
	tokens := []Token{}

	for at := 0; at < len(line); {
		switch {
		// 주석은 줄 앞이나 빈 칸 뒤에서만 시작한다. `${#var}`(길이) 와 `a#b`(그냥 글자) 는
		// 주석이 아니다.
		case line[at] == '#' && shWordStart(line, at):
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), shNormal{}

		case line[at] == '$':
			// `$$` 는 값이 아니라 프로세스 번호다. 둘을 같이 건너뛰지 않으면 뒤의 `$` 가
			// 다시 참조로 읽힌다 — make·docker 에서 이미 겪은 자리다.
			if at+1 < len(line) && line[at+1] == '$' {
				at += 2

				continue
			}

			if end, ok := shellExpansionEnd(line, at, len(line)); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindVariable})
				at = end

				continue
			}

		case line[at] == '"', line[at] == '\'':
			end := shQuoteEnd(line, at+1, line[at])
			if end < 0 {
				// 줄을 넘는다. 다음 줄도 따옴표 안이다.
				return append(tokens, Token{Start: at, End: len(line), Kind: KindString}),
					shQuoted{quote: line[at]}
			}

			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			at = end

			continue

		case isIdentByte(line[at]) && shWordStart(line, at):
			end := identEnd(line, at, len(line))

			if kind, ok := shKeywords[string(line[at:end])]; ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			}

			at = end

			continue
		}

		at++
	}

	return tokens, shNormal{}
}

// shQuoteEnd 는 따옴표가 닫히는 자리 다음이다. 닫히지 않으면 -1 이다.
//
// 홑따옴표 안에서는 `\` 가 escape 가 아니다. shell 에서 홑따옴표 안은 글자 그대로다 —
// `'a\'` 는 닫히지 않은 것이 아니라 `a\` 다.
func shQuoteEnd(line []byte, from int, quote byte) int {
	for at := from; at < len(line); at++ {
		if quote == '"' && line[at] == '\\' {
			at++

			continue
		}

		if line[at] == quote {
			return at + 1
		}
	}

	return -1
}

// shWordStart 는 그 자리가 낱말의 시작인지다.
//
// 예약어와 주석은 낱말의 시작에서만 뜻을 가진다. `--done` 의 `done` 이나 `a#b` 의 `#` 이
// 그렇지 않은 자리다.
func shWordStart(line []byte, at int) bool {
	if at == 0 {
		return true
	}

	switch line[at-1] {
	case ' ', '\t', ';', '|', '&', '(', ')', '{', '}', '`':
		return true
	}

	return false
}

// shKeywords 는 예약어와 그 갈래다.
//
// 명령 이름(`echo` `ls` `make`) 은 넣지 않는다. 끝이 없는 목록이고, 무엇이 명령인지는 PATH 가
// 정하는 것이라 소스만 보고는 알 수 없다.
var shKeywords = map[string]Kind{
	"if": KindKeyword, "then": KindKeyword, "else": KindKeyword, "elif": KindKeyword,
	"fi": KindKeyword, "for": KindKeyword, "while": KindKeyword, "until": KindKeyword,
	"do": KindKeyword, "done": KindKeyword, "case": KindKeyword, "esac": KindKeyword,
	"in": KindKeyword, "function": KindKeyword, "select": KindKeyword, "time": KindKeyword,
	"return": KindKeyword, "break": KindKeyword, "continue": KindKeyword, "exit": KindKeyword,
	"local": KindKeyword, "export": KindKeyword, "readonly": KindKeyword, "declare": KindKeyword,
	"unset": KindKeyword, "shift": KindKeyword, "eval": KindKeyword, "exec": KindKeyword,
	"source": KindKeyword, "trap": KindKeyword, "set": KindKeyword,

	"true": KindConstant, "false": KindConstant,
}

// shIndent 는 shell 의 들여쓰기 규칙이다. 블록을 여는 것은 줄 끝의 `then`·`do`·`in` 이다.
type shIndent struct{}

// shCloseKeywords 는 블록을 닫는 낱말이다. `else`·`elif` 는 닫고 다시 여는 자리라 둘 다다.
var shCloseKeywords = []string{"fi", "done", "esac", "else", "elif"}

func (shIndent) Next(line []byte, tokens []Token) (int, []byte) {
	code := codeBytes(line, tokens)
	trimmed := bytes.TrimRight(code, " \t;")

	for _, word := range []string{"then", "do", "in", "else"} {
		if bytes.HasSuffix(trimmed, []byte(word)) &&
			shWordStart(trimmed, len(trimmed)-len(word)) {
			return 1, nil
		}
	}

	if bracketOpens(code, "{(", ")}") {
		return 1, nil
	}

	// case 갈래(`a)`·`b|c)`·`*)`) 도 블록을 연다. 그 갈래의 몸통이 한 단계 들어가고 `;;` 가
	// 닫는다. 여는 낱말이 없는 자리라 위의 셈으로는 잡히지 않는다.
	//
	// **짝 없이 닫는 `)` 로 끝나는 줄**이 그것이다. `f()` 와 `x=$(cmd)` 는 이 줄에서 연 것을
	// 닫으므로 걸리지 않는다. 괄호로 감싸는 형태(`(a|b)`) 는 짝이 맞아 걸리지 않는데, 그것을
	// 잡으려면 `case` 안인지를 알아야 하고 규칙은 줄 하나만 본다.
	if bytes.HasSuffix(trimmed, []byte(")")) && bracketDepth(code, "(", ")") < 0 {
		return 1, nil
	}

	return 0, nil
}

func (shIndent) Close(head []byte) int {
	// `;;` 는 case 갈래의 끝이다. 낱말이 아니라 마침 글자를 기다릴 것이 없다.
	if bytes.HasPrefix(head, []byte(";;")) {
		return 1
	}

	if closesBracket(head, "})") || startsWord(head, shCloseKeywords...) {
		return 1
	}

	return 0
}

func (shIndent) Reindents() bool { return true }

func (shIndent) TabIndentsLine([]byte) bool { return false }

// Unit 은 space 두 칸이다. shell 소스에 굳은 관례가 없어서 좁은 쪽을 고른다.
func (shIndent) Unit() []byte { return []byte("  ") }
