package syntax

import (
	"unicode/utf8"
)

// rustNormal 은 Rust 의 보통 문맥이다.
type rustNormal struct{}

// rustBlockComment 는 `/*` 로 열린 주석 안이다. depth 는 열린 겹 수다.
//
// **Rust 의 블록 주석은 겹친다.** `/* 바깥 /* 안 */ 아직 주석 */` 이다. js·Go 처럼 첫 `*/` 에서
// 닫으면 그 뒤의 「아직 주석」이 코드로 읽히고, 그 줄의 따옴표가 아래 파일 전체를 문자열로 연다.
type rustBlockComment struct {
	depth int
}

// rustString 은 `"` 로 열린 문자열 안이다. `b"`·`c"` 도 여기다.
//
// **Rust 의 문자열은 줄을 넘는다.** Go·js 의 `"` 와 다르다. 줄 끝의 `\` 는 다음 줄 앞 공백을
// 먹는 이음이지만 강조로는 「아직 문자열」이라는 같은 답이다.
type rustString struct{}

// rustRawString 은 `r"`·`r#"` 로 열린 raw 문자열 안이다. hashes 는 연 `#` 수다.
//
// 닫는 것은 `"` 뒤에 같은 수의 `#` 다. `r#"안의 "따옴표""#` 처럼 그보다 적은 `#` 는 닫지 않는다.
type rustRawString struct {
	hashes int
}

func (rustNormal) Indent() Indent       { return rustIndent{} }
func (rustBlockComment) Indent() Indent { return rustIndent{} }
func (rustString) Indent() Indent       { return rustIndent{} }
func (rustRawString) Indent() Indent    { return rustIndent{} }

func (s rustNormal) Lex(line []byte) ([]Token, State) {
	tokens := []Token{}

	for at := 0; at < len(line); {
		c := line[at]

		switch {
		case c == '/' && at+1 < len(line) && line[at+1] == '/':
			// `///`·`//!` 문서 주석도 주석이다.
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}), s

		case c == '/' && at+1 < len(line) && line[at+1] == '*':
			end, depth := rustBlockCommentEnd(line, at+2, 1)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindComment})
			if depth > 0 {
				return tokens, rustBlockComment{depth: depth}
			}

			at = end

			continue

		case c == '"':
			end, closed := rustStringEnd(line, at+1)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			if !closed {
				return tokens, rustString{}
			}

			at = end

			continue

		case c == '\'':
			if end, kind := rustQuoteEnd(line, at); end > at {
				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
				at = end

				continue
			}

		case c == '#' && rustAttributeAt(line, at):
			// `#[derive(..)]`·`#![allow(..)]` 의 이름까지다. python 의 `@decorator` 와 같은 자리라
			// 같은 갈래다(pyDecoratorAt). 괄호 안은 보통 코드로 훑는다 — `Debug` 는 trait 이름이다.
			name := at + 2
			if line[at+1] == '!' {
				name++
			}

			end := rustPathEnd(line, name)
			if end > name {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindFunction})
				at = end

				continue
			}

		case jsDigit(c):
			end := rustNumberEnd(line, at)
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindNumber})
			at = end

			continue

		case isIdentByte(c):
			// 앞붙이가 붙은 문자열(`b"`·`r#"`·`br"`·`c"`) 과 raw 이름(`r#type`) 이 이 자리에서 갈린다.
			if end, next, ok := rustPrefixed(line, at); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				if next != nil {
					return tokens, next
				}

				at = end

				continue
			}

			start := at
			if c == 'r' && at+2 < len(line) && line[at+1] == '#' && isIdentByte(line[at+2]) {
				at += 2
			}

			end := identEnd(line, at, len(line))
			kind, end := rustWordKind(line, tokens, start, at, end)
			if kind != KindPlain {
				tokens = append(tokens, Token{Start: start, End: end, Kind: kind})
			}

			at = end

			continue
		}

		at++
	}

	return tokens, s
}

func (s rustBlockComment) Lex(line []byte) ([]Token, State) {
	end, depth := rustBlockCommentEnd(line, 0, s.depth)
	if depth > 0 {
		if len(line) < 1 {
			return nil, rustBlockComment{depth: depth}
		}

		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, rustBlockComment{depth: depth}
	}

	tail, next := lexTail(rustNormal{}, line, end)

	return append([]Token{{Start: 0, End: end, Kind: KindComment}}, tail...), next
}

func (s rustString) Lex(line []byte) ([]Token, State) {
	end, closed := rustStringEnd(line, 0)
	if !closed {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(rustNormal{}, line, end)

	return append([]Token{{Start: 0, End: end, Kind: KindString}}, tail...), next
}

func (s rustRawString) Lex(line []byte) ([]Token, State) {
	end, closed := rustRawStringEnd(line, 0, s.hashes)
	if !closed {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(rustNormal{}, line, end)

	return append([]Token{{Start: 0, End: end, Kind: KindString}}, tail...), next
}

// rustBlockCommentEnd 는 from 부터 겹을 세어 주석이 닫히는 자리와 남은 겹 수다.
// 닫히지 않으면 줄 끝과 0 보다 큰 겹 수다.
func rustBlockCommentEnd(line []byte, from, depth int) (int, int) {
	for at := from; at+1 < len(line); at++ {
		switch {
		case line[at] == '/' && line[at+1] == '*':
			depth++
			at++
		case line[at] == '*' && line[at+1] == '/':
			depth--
			at++
			if depth == 0 {
				return at + 1, 0
			}
		}
	}

	return len(line), depth
}

// rustStringEnd 는 from 부터 보아 `"` 가 닫히는 자리다. 닫히지 않으면 줄 끝과 false 다.
func rustStringEnd(line []byte, from int) (int, bool) {
	for at := from; at < len(line); at++ {
		switch line[at] {
		case '\\':
			at++
		case '"':
			return at + 1, true
		}
	}

	return len(line), false
}

// rustRawStringEnd 는 from 부터 보아 `"` 와 `#` hashes 개가 닫히는 자리다. raw 라 escape 가 없다.
func rustRawStringEnd(line []byte, from, hashes int) (int, bool) {
	for at := from; at < len(line); at++ {
		if line[at] != '"' {
			continue
		}

		count := 0
		for at+1+count < len(line) && line[at+1+count] == '#' && count < hashes {
			count++
		}

		if count == hashes {
			return at + 1 + hashes, true
		}
	}

	return len(line), false
}

// rustPrefixed 는 at 에서 앞붙이가 붙은 문자열이 열리는지 본다. 열리면 그 끝과, 줄을 넘으면
// 다음 줄의 문맥이다. 한 줄에서 닫히면 next 는 nil 이다.
//
// `b"..."`·`c"..."` 는 보통 문자열이고 `r"`·`br"`·`cr"` 는 raw 다. `b'x'` 는 byte 문자다.
func rustPrefixed(line []byte, at int) (end int, next State, ok bool) {
	rest := line[at:]

	prefix := 0
	switch {
	case len(rest) >= 2 && (rest[0] == 'b' || rest[0] == 'c') && rest[1] == 'r':
		prefix = 2
	case len(rest) >= 1 && (rest[0] == 'b' || rest[0] == 'c' || rest[0] == 'r'):
		prefix = 1
	default:
		return 0, nil, false
	}

	// 앞붙이가 낱말의 처음이어야 한다. `abr"` 의 `br` 은 앞붙이가 아니다 — 부르는 쪽이 낱말의
	// 처음에서만 부르므로 여기서는 at 이 그 자리다.
	raw := rest[prefix-1] == 'r'
	from := at + prefix

	if raw {
		hashes := 0
		for from+hashes < len(line) && line[from+hashes] == '#' {
			hashes++
		}

		if from+hashes >= len(line) || line[from+hashes] != '"' {
			return 0, nil, false
		}

		end, closed := rustRawStringEnd(line, from+hashes+1, hashes)
		if !closed {
			return end, rustRawString{hashes: hashes}, true
		}

		return end, nil, true
	}

	if from >= len(line) {
		return 0, nil, false
	}

	switch {
	case line[from] == '"':
		end, closed := rustStringEnd(line, from+1)
		if !closed {
			return end, rustString{}, true
		}

		return end, nil, true

	case line[from] == '\'' && rest[0] == 'b':
		if end, kind := rustQuoteEnd(line, from); end > from && kind == KindString {
			return end, nil, true
		}
	}

	return 0, nil, false
}

// rustQuoteEnd 는 at 의 `'` 가 문자 리터럴인지 lifetime 인지 보고 그 끝과 갈래다. 둘 다
// 아니면 at 이다.
//
// **글자 하나 뒤에 `'` 가 오면 문자다.** `'a'` 는 문자이고 `'a` 는 lifetime 이다. 그 글자가
// 한글이면 세 byte 라 byte 하나 뒤를 보면 틀린다. `'\n'`·`'\u{1F600}'` 처럼 `\` 로 열면 늘
// 문자다. 문자는 줄을 넘지 못해서 닫히지 않아도 문맥을 바꾸지 않는다.
func rustQuoteEnd(line []byte, at int) (int, Kind) {
	if at+1 >= len(line) {
		return at, KindPlain
	}

	if line[at+1] == '\\' {
		for i := at + 2; i < len(line); i++ {
			if line[i] == '\'' && i > at+2 {
				return i + 1, KindString
			}
		}

		return len(line), KindString
	}

	_, size := utf8.DecodeRune(line[at+1:])
	if at+1+size < len(line) && line[at+1+size] == '\'' {
		return at + 2 + size, KindString
	}

	if isIdentByte(line[at+1]) {
		return identEnd(line, at+1, len(line)), KindVariable
	}

	return at, KindPlain
}

// rustAttributeAt 는 at 의 `#` 가 attribute 를 여는지다. `#[` 와 `#![` 다.
func rustAttributeAt(line []byte, at int) bool {
	if at+1 < len(line) && line[at+1] == '[' {
		return true
	}

	return at+2 < len(line) && line[at+1] == '!' && line[at+2] == '['
}

// rustPathEnd 는 `serde::rename` 처럼 `::` 로 이은 이름이 끝나는 자리다.
func rustPathEnd(line []byte, at int) int {
	end := identEnd(line, at, len(line))
	for end > at && end+2 < len(line) && line[end] == ':' && line[end+1] == ':' && isIdentByte(line[end+2]) {
		end = identEnd(line, end+2, len(line))
	}

	return end
}

// rustNumberEnd 는 숫자가 끝나는 자리다. `0x1f` `1_000u32` `2.5f64` `1e-9` 를 받는다.
//
// **`.` 은 뒤에 숫자가 올 때만 먹는다.** `0..10` 의 범위와 `1.max(2)` 의 메서드가 그 자리다.
// 붙는 type(`u32`) 은 낱말 글자라 그대로 이어서 먹는다.
func rustNumberEnd(line []byte, at int) int {
	hex := at+1 < len(line) && line[at] == '0' && (line[at+1] == 'x' || line[at+1] == 'X')

	end := at
	for end < len(line) {
		c := line[end]
		switch {
		case c == '.':
			if hex || end+1 >= len(line) || !jsDigit(line[end+1]) {
				return end
			}
			end++
		case (c == '+' || c == '-') && !hex && end > at && (line[end-1] == 'e' || line[end-1] == 'E'):
			end++
		case isIdentByte(c) && c < 0x80:
			end++
		default:
			return end
		}
	}

	return end
}

// rustWordKind 는 낱말 하나의 갈래와, 그 토큰이 끝나는 자리다. start 는 토큰의 처음
// (`r#` 를 붙였으면 그 자리) 이고 [at, end) 가 이름이다.
//
// 끝이 이름 끝과 다를 수 있다. `println!` 은 `!` 까지가 한 토큰이다.
func rustWordKind(line []byte, tokens []Token, start, at, end int) (Kind, int) {
	word := string(line[at:end])

	// raw 이름(`r#type`) 은 예약어가 아니다. 그것이 그 문법의 뜻이다.
	if start == at {
		if kind, ok := rustKeywords[word]; ok {
			return kind, end
		}
	}

	// `println!(` 의 `!` 다. `x != y` 의 `!` 는 아니다.
	if end < len(line) && line[end] == '!' && (end+1 >= len(line) || line[end+1] != '=') {
		return KindFunction, end + 1
	}

	switch rustAfterKeyword(tokens, line, start) {
	case "fn":
		return KindFunction, end
	case "struct", "enum", "trait", "type", "union":
		return KindType, end
	}

	if end < len(line) && line[end] == '(' {
		// `Some(x)` 처럼 대문자로 시작하면 부르는 것이 아니라 tuple 꼴 variant·struct 다.
		if !rustUpper(line[at]) {
			return KindFunction, end
		}
	}

	if rustUpper(line[at]) {
		if rustScreaming(line[at:end]) {
			return KindConstant, end
		}

		// Rust 는 type·trait·variant 를 CamelCase 로 짓는다(컴파일러가 경고로 지킨다). 이름
		// 모양만 보고 type 으로 친다. variant 와 type 은 서버가 붙으면 갈린다.
		return KindType, end
	}

	return KindPlain, end
}

// rustAfterKeyword 는 바로 앞 토큰이 빈 칸만 두고 붙은 예약어면 그 낱말이다.
func rustAfterKeyword(tokens []Token, line []byte, at int) string {
	if len(tokens) < 1 {
		return ""
	}

	last := tokens[len(tokens)-1]
	if last.Kind != KindKeyword || last.End >= at {
		return ""
	}

	for _, b := range line[last.End:at] {
		if b != ' ' && b != '\t' {
			return ""
		}
	}

	return string(line[last.Start:last.End])
}

func rustUpper(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

// rustScreaming 은 `MAX_LEN` 처럼 대문자와 `_`·숫자로만 된 두 글자 넘는 이름인지다. Rust 의
// const·static 이름 짓기다. `T` 하나는 generic 인자라 뺀다.
func rustScreaming(word []byte) bool {
	if len(word) < 2 {
		return false
	}

	for _, b := range word {
		if !(rustUpper(b) || b == '_' || jsDigit(b)) {
			return false
		}
	}

	return true
}

// rustKeywords 는 예약어와 이름 모양으로 갈리지 않는 낱말들의 갈래다.
//
// 기본 type(`u32`·`str`) 은 소문자라 CamelCase 규칙에 걸리지 않아서 표에 둔다. `Some`·`None`·
// `Ok`·`Err` 는 prelude 의 variant 라 대문자여도 type 이 아니라 값이다.
var rustKeywords = map[string]Kind{
	"as": KindKeyword, "async": KindKeyword, "await": KindKeyword, "break": KindKeyword,
	"const": KindKeyword, "continue": KindKeyword, "crate": KindKeyword, "dyn": KindKeyword,
	"else": KindKeyword, "enum": KindKeyword, "extern": KindKeyword, "fn": KindKeyword,
	"for": KindKeyword, "if": KindKeyword, "impl": KindKeyword, "in": KindKeyword,
	"let": KindKeyword, "loop": KindKeyword, "match": KindKeyword, "mod": KindKeyword,
	"move": KindKeyword, "mut": KindKeyword, "pub": KindKeyword, "ref": KindKeyword,
	"return": KindKeyword, "self": KindKeyword, "static": KindKeyword, "struct": KindKeyword,
	"super": KindKeyword, "trait": KindKeyword, "type": KindKeyword, "unsafe": KindKeyword,
	"use": KindKeyword, "where": KindKeyword, "while": KindKeyword, "yield": KindKeyword,

	"Self": KindType,
	"bool": KindType, "char": KindType, "str": KindType,
	"i8": KindType, "i16": KindType, "i32": KindType, "i64": KindType, "i128": KindType, "isize": KindType,
	"u8": KindType, "u16": KindType, "u32": KindType, "u64": KindType, "u128": KindType, "usize": KindType,
	"f32": KindType, "f64": KindType,

	"true": KindConstant, "false": KindConstant,
	"Some": KindConstant, "None": KindConstant, "Ok": KindConstant, "Err": KindConstant,
}

// rustIndent 는 Rust 의 들여쓰기 규칙이다. 세 가지 괄호를 센다.
//
// **braceIndent 를 나눠 쓰지 않았다.** 그것은 `case`·`default` 로 시작하는 줄을 한 단계
// 내는데, Rust 에는 switch 이름표가 없고 `default: bool,` 같은 필드가 흔하다. nginx 가 따로
// 선 것과 같은 까닭이다(nginxIndent).
type rustIndent struct{}

func (rustIndent) Next(line []byte, tokens []Token) (int, []byte) {
	if bracketOpens(codeBytes(line, tokens), braceOpen, braceClose) {
		return 1, nil
	}

	return 0, nil
}

func (rustIndent) Close(head []byte) int {
	if closesBracket(head, braceClose) {
		return 1
	}

	return 0
}

func (rustIndent) Reindents() bool { return true }

func (rustIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 space 네 칸이다. rustfmt 의 기본값이다.
func (rustIndent) Unit() []byte { return []byte("    ") }
