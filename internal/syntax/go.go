package syntax

import (
	"bytes"
	"go/scanner"
	"go/token"
	"strings"
)

// goNormal 은 Go 의 보통 문맥이다. 여러 줄에 걸친 것 안이 아니다.
type goNormal struct{}

// goRawString 은 백틱으로 열린 문자열 안이다.
type goRawString struct{}

// goBlockComment 는 `/*` 로 열린 주석 안이다.
type goBlockComment struct{}

// 셋 다 같은 규칙이다. raw string 과 주석 안에서 Enter 를 쳐도 앞 줄의 들여쓰기를 잇는 것이
// 맞고, tabBraceIndent 가 그 답을 준다.
func (goNormal) Indent() Indent       { return tabBraceIndent }
func (goRawString) Indent() Indent    { return tabBraceIndent }
func (goBlockComment) Indent() Indent { return tabBraceIndent }

// Go 에서 줄을 넘는 것은 이 둘뿐이다. `"` 로 여는 문자열은 줄을 넘지 못한다.
//
// 두 문맥의 몸이 거의 같은데도 언어마다 따로 두는 이유가 있다. 나가는 문맥이 곧 「다음 줄이
// 어느 언어의 무엇인가」라서, js 와 주석 상태를 함께 쓰면 다음 줄을 어느 lexer 가 읽어야
// 하는지 알 수 없다. 이 되풀이는 정리하면 안 된다(syntax.go 의 State).

// goToken 은 go/scanner 가 준 토큰 하나다.
//
// 한 줄을 통째로 모아 두고 갈래를 매긴다. 다음 토큰을 봐야 정해지는 것(`foo(` 의 `foo`) 과
// 앞 토큰을 봐야 정해지는 것(`type Foo` 의 `Foo`) 이 있고, 줄 끝 문맥도 마지막 토큰이 정한다.
type goToken struct {
	offset int
	tok    token.Token
	lit    string
}

func (s goNormal) Lex(line []byte) ([]Token, State) {
	scanned := goScan(line)

	tokens := make([]Token, 0, len(scanned))
	for i := range scanned {
		kind := goKind(scanned, i)
		if kind == KindPlain {
			continue
		}

		// 갈래가 붙는 토큰은 모두 lit 이 채워져 있다 — 연산자·괄호는 lit 이 비어 있고 갈래도
		// 없어서 위에서 걸러진다. 그래서 길이를 lit 에서 얻어도 된다(go/scanner 의 Scan 문서).
		tokens = append(tokens, Token{
			Start: scanned[i].offset,
			End:   scanned[i].offset + len(scanned[i].lit),
			Kind:  kind,
		})
	}

	return tokens, goNext(scanned)
}

func (s goRawString) Lex(line []byte) ([]Token, State) {
	end := bytes.IndexByte(line, '`')
	if end < 0 {
		if len(line) == 0 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
	}

	tail, next := lexTail(goNormal{}, line, end+1)

	return append([]Token{{Start: 0, End: end + 1, Kind: KindString}}, tail...), next
}

func (s goBlockComment) Lex(line []byte) ([]Token, State) {
	end := bytes.Index(line, []byte("*/"))
	if end < 0 {
		if len(line) == 0 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, s
	}

	tail, next := lexTail(goNormal{}, line, end+2)

	return append([]Token{{Start: 0, End: end + 2, Kind: KindComment}}, tail...), next
}

// goScan 은 줄 하나를 훑는다. 자리는 줄 안의 byte offset 이다.
//
// **오류 handler 를 달지 않는다.** 닫히지 않은 것은 오류 문구가 아니라 마지막 토큰의
// 생김새로 알아낸다(goNext). 문구(`raw string literal not terminated`) 는 Go 판마다 바뀔 수
// 있는 남의 문장이고, 생김새(백틱으로 열렸는데 닫히지 않았다) 는 언어 문법이 정한 것이다.
// 게다가 문구로 가리려면 줄을 넘지 않는 `string literal not terminated` 와 넘는
// `raw string literal not terminated` 를 substring 관계로 갈라야 한다.
func goScan(line []byte) []goToken {
	fset := token.NewFileSet()
	file := fset.AddFile("", 1, len(line))

	var s scanner.Scanner
	s.Init(file, line, nil, scanner.ScanComments)

	scanned := []goToken{}
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return scanned
		}

		// 줄 끝에 저절로 끼는 `;` 다(auto-semicolon). 소스에 없는 글자라 자리가 줄 끝이거나
		// 그 뒤이고, 그대로 두면 없는 byte 에 색을 입힌다. 소스에 있는 `;` 는 lit 이 ";" 다.
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		scanned = append(scanned, goToken{offset: int(pos) - file.Base(), tok: tok, lit: lit})
	}
}

// goKind 는 토큰 하나의 갈래다. 강조하지 않을 자리는 KindPlain 이라 토큰이 되지 않는다.
func goKind(scanned []goToken, i int) Kind {
	item := scanned[i]

	switch {
	case item.tok == token.COMMENT:
		return KindComment
	case item.tok.IsKeyword():
		return KindKeyword
	case item.tok == token.STRING, item.tok == token.CHAR:
		return KindString
	case item.tok == token.INT, item.tok == token.FLOAT, item.tok == token.IMAG:
		return KindNumber
	case item.tok != token.IDENT:
		return KindPlain
	}

	// 미리 선언된 이름은 scanner 에게는 그냥 식별자다. `int` 가 type 으로, `nil` 이 값으로
	// 보이려면 이름 표가 있어야 한다. `int(x)` 는 부르는 자리처럼 보여도 type 이라 표가 먼저다.
	if kind, ok := goPredeclared[item.lit]; ok {
		return kind
	}

	// `type` 바로 뒤는 새 type 의 이름이다.
	if i > 0 && scanned[i-1].tok == token.TYPE {
		return KindType
	}

	if goTypePosition(scanned, i) {
		return KindType
	}

	// 바로 뒤가 `(` 면 부르는 자리다. `errors.New(` 의 `New` 도, `func Lex(` 의 `Lex` 도 여기다.
	if i+1 < len(scanned) && scanned[i+1].tok == token.LPAREN {
		return KindFunction
	}

	return KindPlain
}

// goTypePosition 은 그 이름이 type 을 적는 자리인지다.
//
// Go 에서 **식별자 둘이 공백만 두고 붙으면 뒤쪽은 type 자리다.** 표현식 사이에는 연산자나
// 구두점이 반드시 끼기 때문에 그 밖의 경우가 없다. `const aa BB = "ss"` 의 `BB`, `var x MyType`,
// struct 필드, 함수 인자가 전부 이 하나로 잡힌다. `package main`·`return x` 처럼 붙어 보이는
// 것은 앞이 예약어라 IDENT 가 아니다.
//
// `*Buffer` 와 `[]Token` 도 type 자리인데 앞에 `*`·`]` 가 끼어 있어서 따로 본다.
func goTypePosition(scanned []goToken, i int) bool {
	if i < 1 {
		return false
	}

	switch prev := scanned[i-1]; prev.tok {
	case token.IDENT:
		return true

	case token.MUL:
		// 곱셈과 갈라야 한다. gofmt 는 포인터의 `*` 를 이름에 붙이고(`*Buffer`) 곱셈의 `*` 는
		// 양쪽에 빈 칸을 둔다(`a * b`). 붙어 있는지로 가른다.
		return scanned[i].offset == prev.offset+1

	case token.RBRACK:
		// `[]Token` `[5]Foo` `map[string]Foo` 의 원소 type 이다.
		return true

	case token.PERIOD:
		// `syntax.State` 처럼 package 이름이 붙은 type 이다. 앞쪽이 type 자리면 뒤쪽도 그렇다.
		// 이것이 없으면 package 이름에만 색이 붙어 정작 type 이름이 빠진다.
		return i >= 2 && scanned[i-2].tok == token.IDENT && goTypePosition(scanned, i-2)
	}

	return false
}

// goNext 는 다음 줄의 문맥이다.
//
// scanner 는 닫히지 않은 것을 만나면 줄 끝까지 통째로 그 토큰으로 잡아 준다. 그래서 마지막
// 토큰이 열려 있는지만 보면 된다. `"` 로 여는 문자열은 Go 에서 줄을 넘지 못하므로 닫히지
// 않아도 문맥을 바꾸지 않는다 — 여기서 바꾸면 그 아래 파일 전체가 문자열 색이 된다.
func goNext(scanned []goToken) State {
	if len(scanned) < 1 {
		return goNormal{}
	}

	last := scanned[len(scanned)-1]
	switch {
	// 닫힌 raw string 은 백틱으로 끝나고 길이가 2 이상이다. 백틱 하나가 여는 조건과 닫는
	// 조건을 동시에 만족해서 길이를 같이 본다.
	case last.tok == token.STRING && strings.HasPrefix(last.lit, "`") &&
		!(len(last.lit) >= 2 && strings.HasSuffix(last.lit, "`")):
		return goRawString{}

	// 닫힌 block comment 는 `*/` 로 끝나고 길이가 4 이상이다. `/*/` 는 아직 열려 있는데 끝
	// 두 글자가 `*/` 라 길이를 보지 않으면 닫힌 것으로 읽힌다.
	case last.tok == token.COMMENT && strings.HasPrefix(last.lit, "/*") &&
		!(len(last.lit) >= 4 && strings.HasSuffix(last.lit, "*/")):
		return goBlockComment{}
	}

	return goNormal{}
}

// goPredeclared 는 미리 선언된 이름들이다. scanner 에게는 예약어가 아니라 식별자라 표가 필요하다.
//
// 가릴 수 있는 이름이지만(`func len(...)`) 가리는 코드는 드물어서 이름만 보고 갈래를 매긴다.
var goPredeclared = map[string]Kind{
	"any": KindType, "bool": KindType, "byte": KindType, "comparable": KindType,
	"complex64": KindType, "complex128": KindType, "error": KindType, "float32": KindType,
	"float64": KindType, "int": KindType, "int8": KindType, "int16": KindType,
	"int32": KindType, "int64": KindType, "rune": KindType, "string": KindType,
	"uint": KindType, "uint8": KindType, "uint16": KindType, "uint32": KindType,
	"uint64": KindType, "uintptr": KindType,

	"true": KindConstant, "false": KindConstant, "nil": KindConstant, "iota": KindConstant,

	"append": KindFunction, "cap": KindFunction, "clear": KindFunction, "close": KindFunction,
	"complex": KindFunction, "copy": KindFunction, "delete": KindFunction, "imag": KindFunction,
	"len": KindFunction, "make": KindFunction, "max": KindFunction, "min": KindFunction,
	"new": KindFunction, "panic": KindFunction, "print": KindFunction, "println": KindFunction,
	"real": KindFunction, "recover": KindFunction,
}
