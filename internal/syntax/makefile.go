package syntax

import (
	"bytes"
)

// makeNormal 은 Makefile 의 보통 문맥이다.
//
// 조리법 줄(tab 으로 시작하는 줄) 도 여기서 다룬다 — 「tab 으로 시작한다」는 줄 자체에
// 적혀 있어서 앞 줄을 알 필요가 없다. 문맥이 필요한 것은 `\` 로 이어진 줄뿐이다.
type makeNormal struct{}

// makeContinued 는 앞 줄이 `\` 로 이어진 자리다. 줄 앞이 대상 이름도 변수 이름도 아니다.
type makeContinued struct{}

func (makeNormal) Indent() Indent    { return makeIndent{} }
func (makeContinued) Indent() Indent { return makeIndent{} }

func (s makeNormal) Lex(line []byte) ([]Token, State) {
	limit, comment := makeCommentAt(line)
	if limit == 0 {
		return comment, makeNext(line, s)
	}

	tokens := makeLexHead(line, limit)
	tokens = append(tokens, makeLexExpansions(line, headEnd(tokens), limit)...)

	return append(tokens, comment...), makeNext(line, s)
}

func (s makeContinued) Lex(line []byte) ([]Token, State) {
	limit, comment := makeCommentAt(line)

	tokens := makeLexExpansions(line, 0, limit)

	return append(tokens, comment...), makeNext(line, s)
}

// headEnd 는 줄 앞에서 이미 갈래를 매긴 자리 다음이다. 값 참조는 그 뒤부터 찾는다.
func headEnd(tokens []Token) int {
	if len(tokens) < 1 {
		return 0
	}

	return tokens[len(tokens)-1].End
}

// makeNext 는 다음 줄의 문맥이다. 줄이 `\` 로 끝나면 다음 줄이 이 줄의 뒷부분이다.
func makeNext(line []byte, current State) State {
	if bytes.HasSuffix(line, []byte{'\\'}) {
		return makeContinued{}
	}

	if _, ok := current.(makeContinued); ok {
		return makeNormal{}
	}

	return current
}

// makeCommentAt 은 주석이 시작하는 자리와 그 토큰이다.
//
// make 의 주석은 줄 아무 자리에서나 시작해서 줄 끝까지다. 이 저장소의 Makefile 이 쓰는
// `## 도움 문구` 도 그래서 대상 이름 뒤에 붙는다.
func makeCommentAt(line []byte) (int, []Token) {
	for at := 0; at < len(line); at++ {
		if line[at] != '#' {
			continue
		}
		if at > 0 && line[at-1] == '\\' {
			continue
		}

		return at, []Token{{Start: at, End: len(line), Kind: KindComment}}
	}

	return len(line), nil
}

// makeLexHead 는 줄 앞이 무엇인지 가른다 — 지시자, 변수 이름, 대상 이름 중 하나다.
//
// 조리법 줄은 앞이 shell 이라 아무것도 아니다.
func makeLexHead(line []byte, limit int) []Token {
	if limit > 0 && line[0] == '\t' {
		return nil
	}

	at := 0
	for at < limit && line[at] == ' ' {
		at++
	}

	// 이름은 붙이기 기호 앞에서 끝난다. `?` `+` 를 빼먹으면 `VERSION?=` 의 이름이 `VERSION?` 이 된다.
	word := at
	for word < limit && bytes.IndexByte([]byte(" :=?+"), line[word]) < 0 {
		word++
	}

	if word > at && makeDirectives[string(line[at:word])] {
		return []Token{{Start: at, End: word, Kind: KindKeyword}}
	}

	// 변수 붙이기(`NAME =` `NAME :=` `NAME ?=` `NAME +=` `NAME ::=`) 는 이름이 값을 가리킨다.
	// 대상(`build: dep`) 은 부르는 이름이라 갈래가 다르다 — `make build` 로 부른다.
	if word > at && makeAssigns(line, word, limit) {
		return []Token{{Start: at, End: word, Kind: KindVariable}}
	}

	if word < limit && line[word] == ':' && word > at {
		return []Token{{Start: at, End: word, Kind: KindFunction}}
	}

	return nil
}

// makeAssigns 는 그 자리부터 붙이기 기호가 오는지다. 빈 칸을 건너뛰고 본다.
func makeAssigns(line []byte, from, limit int) bool {
	at := from
	for at < limit && line[at] == ' ' {
		at++
	}

	if at >= limit {
		return false
	}

	switch line[at] {
	case '=':
		return true
	case ':', '?', '+':
		// `:=` `::=` `?=` `+=` 다. `:` 하나는 대상이라 `=` 가 뒤따르는지로 갈린다.
		for at < limit && (line[at] == ':' || line[at] == '?' || line[at] == '+') {
			at++
		}

		return at < limit && line[at] == '='
	}

	return false
}

// makeLexExpansions 는 값 참조를 훑는다.
//
// 조리법 안은 shell 이라 나머지에 색을 입히지 않는다 — shell lexer 가 없다.
func makeLexExpansions(line []byte, from, limit int) []Token {
	tokens := []Token{}

	for at := from; at < limit; {
		if line[at] != '$' {
			at++

			continue
		}

		// `$$` 는 `$` 한 글자를 내는 것이다. 둘을 같이 건너뛰어야 뒤의 `$` 가 다시 참조로 읽히지
		// 않는다 — `$$2` 가 `$2` 로 보이면 shell 인자에 색이 붙는다.
		if at+1 < limit && line[at+1] == '$' {
			at += 2

			continue
		}

		// `$@` `$<` `$^` `$?` `$*` 는 make 만 쓰는 한 글자 이름이다.
		if at+1 < limit && bytes.IndexByte([]byte("@<^?*"), line[at+1]) >= 0 {
			tokens = append(tokens, Token{Start: at, End: at + 2, Kind: KindVariable})
			at += 2

			continue
		}

		end, ok := shellExpansionEnd(line, at, limit)
		if !ok {
			at++

			continue
		}

		tokens = append(tokens, Token{Start: at, End: end, Kind: KindVariable})
		at = end
	}

	return tokens
}

// makeDirectives 는 make 자신에게 하는 말이다. 대상도 변수도 아니다.
var makeDirectives = map[string]bool{
	"include": true, "-include": true, "sinclude": true,
	"ifeq": true, "ifneq": true, "ifdef": true, "ifndef": true,
	"else": true, "endif": true, "define": true, "endef": true,
	"export": true, "unexport": true, "override": true, "vpath": true,
}

// makeIndent 는 makefile 의 들여쓰기 규칙이다.
//
// **한 단계가 언제나 tab 인 유일한 언어다.** GNU make 는 조리법 줄을 tab 으로만 알아본다.
// `.editorconfig` 가 space 라고 적어 두었어도 여기서는 따를 수 없다. level 이 아니라 prefix 로
// tab 을 내보내면 한 단계를 정하는 셈을 아예 거치지 않아서, 예외를 core 가 알 필요가 없다.
type makeIndent struct{}

func (makeIndent) Next(line []byte, tokens []Token) (int, []byte) {
	if makeIsTarget(codeBytes(line, tokens)) {
		return 0, []byte{'\t'}
	}

	return 0, nil
}

// makeIsTarget 은 이 줄 다음에 조리법이 올 수 있는 대상 줄인지다.
//
// 대상 줄은 들여쓰기 없이 시작해서 `:` 이 나오는 줄이다. 가려낼 것이 둘 있다 —
// 변수 대입(`VAR := x`, `VAR ::= x`) 은 `:` 뒤가 `=` 이고, `.PHONY` 같은 특수 대상은
// 조리법을 갖지 않는다.
func makeIsTarget(code []byte) bool {
	if len(code) < 1 || code[0] == ' ' || code[0] == '\t' || code[0] == '.' {
		return false
	}

	at := bytes.IndexByte(code, ':')
	if at < 0 {
		return false
	}

	rest := bytes.TrimLeft(code[at+1:], ":")

	return len(rest) < 1 || rest[0] != '='
}

// Close 는 언제나 0 이다. makefile 에 블록을 닫는 표시가 없다.
func (makeIndent) Close(_ []byte) int { return 0 }

func (makeIndent) Reindents() bool { return true }

// TabIndentsLine 은 거짓이다. 조리법 줄 안의 tab 은 셸에 그대로 가는 글자다.
func (makeIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

func (makeIndent) Unit() []byte { return []byte{'\t'} }

// makeOutline 은 makefile 의 뼈대 규칙이다. 조리법을 거느리는 것은 대상 줄이다.
//
// **들여쓰기 규칙에 묻지 않는다.** makeIndent.Next 는 단계를 내지 않고 tab 을 prefix 로 낸다 —
// 한 단계가 언제나 tab 이라 그렇게 둔 것이고(위의 makeIndent), 그래서 「다음 줄이 들어간다」로는
// 대상 줄이 하나도 안 걸린다. 조리법이 서른 줄이 되는 것이 흔해서 대상 줄이 가장 아쉬운 자리다.
type makeOutline struct{}

func (makeOutline) Depth(line []byte, tokens []Token) int { return indentDepth(line, tokens) }

func (makeOutline) Heads(line []byte, tokens []Token) bool {
	return makeIsTarget(codeBytes(line, tokens))
}
