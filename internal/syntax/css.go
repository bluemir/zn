package syntax

import (
	"bytes"
)

// cssNormal 은 선택자 자리다. 중괄호 밖이다.
type cssNormal struct{}

// cssBlock 은 선언 자리다. 중괄호 안이다.
//
// inValue 는 앞 줄이 `:` 뒤에서 끝났는지다. 여러 줄에 걸친 선언(`box-shadow:` 뒤로 값이 두 줄)
// 에서 이어지는 줄의 첫 낱말이 속성 이름이 아니라 값이라는 것을 이것으로 안다.
type cssBlock struct {
	inValue bool
}

// cssComment 는 `/*` 로 열린 주석 안이다.
//
// 돌아갈 자리를 든다. 주석은 선택자 자리에서도 선언 자리에서도 열리고, 닫힌 뒤에는 열렸던
// 자리로 돌아가야 한다. 상태를 자리마다 쪼개면 `*/` 찾기가 그만큼 적히므로 값으로 든다 —
// 함수 인자의 flag 가 아니라 「이미 정해진 것」이라 normal-key-parser.go 의 partial 과 같은 손이다.
type cssComment struct {
	back State
}

func (s cssNormal) Lex(line []byte) ([]Token, State) {
	return cssLex(line, false, false)
}

func (s cssBlock) Lex(line []byte) ([]Token, State) {
	return cssLex(line, true, s.inValue)
}

func (s cssComment) Lex(line []byte) ([]Token, State) {
	end := bytes.Index(line, []byte("*/"))
	if end < 0 {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, s
	}

	tail, next := lexTail(s.back, line, end+2)

	return append([]Token{{Start: 0, End: end + 2, Kind: KindComment}}, tail...), next
}

// cssStateFor 는 줄이 끝난 자리의 문맥이다.
func cssStateFor(inBlock, value bool) State {
	if inBlock {
		return cssBlock{inValue: value}
	}

	return cssNormal{}
}

// cssLex 는 줄 하나를 훑는다. inBlock 이면 선언 자리이고, value 면 `:` 뒤다.
//
// 선언 자리에서 `:` 는 속성과 값을 가르고 `;` 는 다시 속성 자리로 돌린다. 값 자리인지는 줄이
// 끝나도 이어져야 해서(여러 줄에 걸친 선언) 나가는 문맥이 그것을 들고 간다.
func cssLex(line []byte, inBlock, value bool) ([]Token, State) {
	tokens := []Token{}

	for at := 0; at < len(line); {
		switch {
		case line[at] == '/' && at+1 < len(line) && line[at+1] == '*':
			if end := bytes.Index(line[at+2:], []byte("*/")); end >= 0 {
				to := at + 2 + end + 2
				tokens = append(tokens, Token{Start: at, End: to, Kind: KindComment})
				at = to

				continue
			}

			tokens = append(tokens, Token{Start: at, End: len(line), Kind: KindComment})

			return tokens, cssComment{back: cssStateFor(inBlock, value)}

		case line[at] == '{':
			inBlock, value = true, false
			at++

			continue

		case line[at] == '}':
			inBlock, value = false, true
			at++

			continue

		case line[at] == ';':
			// 선언이 끝나면 다음 이름은 다시 속성이다.
			value = false
			at++

			continue

		case line[at] == ':' && inBlock:
			value = true
			at++

			continue

		case line[at] == '"', line[at] == '\'':
			if end, ok := quotedEnd(line, at, len(line)); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				at = end

				continue
			}

		case line[at] == '#':
			// 값 자리의 `#fff` 는 색이고 선택자 자리의 `#id` 는 이름이다. 자리가 갈라 준다.
			end := cssNameEnd(line, at+1)
			if end > at+1 {
				kind := KindType
				if value {
					kind = KindNumber
				}
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
				at = end

				continue
			}

		case line[at] == '@':
			end := cssNameEnd(line, at+1)
			if end > at+1 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindKeyword})
				at = end

				continue
			}

		case line[at] == '$', line[at] == '.':
			// `.class` 는 선택자다. `$` 는 css 가 아니지만 전처리기 파일이 섞여 들어온다.
			end := cssNameEnd(line, at+1)
			if end > at+1 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindType})
				at = end

				continue
			}

		case line[at] == '-' && at+1 < len(line) && line[at+1] == '-':
			end := cssNameEnd(line, at+2)
			if end > at+2 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindVariable})
				at = end

				continue
			}

		case line[at] == '!':
			end := cssNameEnd(line, at+1)
			if end > at+1 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindKeyword})
				at = end

				continue
			}

		case cssDigit(line[at]):
			end := cssNumberEnd(line, at)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindNumber})
			at = end

			continue

		case isIdentByte(line[at]):
			end := cssNameEnd(line, at)

			// 값 자리의 낱말은 정해진 값(`block` `inherit`) 이고, 속성 자리의 낱말은 속성
			// 이름이다. 선택자 자리의 낱말은 요소 이름이라 선택자와 같이 본다.
			kind := KindKeyword
			switch {
			case !inBlock:
				kind = KindType
			case value:
				kind = KindConstant
			}

			tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
			at = end

			continue
		}

		at++
	}

	return tokens, cssStateFor(inBlock, value)
}

// cssNameEnd 는 이름이 끝나는 자리다. css 는 이름에 `-` 가 들어간다(`min-width`, `--my-var`).
func cssNameEnd(line []byte, at int) int {
	for at < len(line) && (isIdentByte(line[at]) || line[at] == '-') {
		at++
	}

	return at
}

func cssDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// cssNumberEnd 는 숫자와 단위가 끝나는 자리다. `1px` `0.5em` `50%` 를 하나로 본다.
func cssNumberEnd(line []byte, at int) int {
	for at < len(line) && (cssDigit(line[at]) || line[at] == '.') {
		at++
	}

	// 단위는 숫자에 붙은 것이라 따로 색을 주지 않는다.
	if at < len(line) && line[at] == '%' {
		return at + 1
	}

	return cssNameEnd(line, at)
}
