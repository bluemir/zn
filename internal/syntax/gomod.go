package syntax

import "bytes"

// gomodNormal 은 go.mod·go.work 의 보통 문맥이다. 줄 앞이 지시자 자리다.
type gomodNormal struct{}

// gomodBlock 은 `require (` 처럼 괄호로 연 덩이 안이다. 줄 앞에 지시자가 없다.
//
// 문맥을 갈라 두는 것은 덩이 안의 첫 낱말이 지시자로 읽히지 않게 하기 위해서다. `tool (`
// 안의 `example.com/go` 나 `use (` 안의 `./go` 처럼 지시자와 같은 글자로 시작하는 줄이 있다.
type gomodBlock struct{}

func (gomodNormal) Indent() Indent { return tabBraceIndent }
func (gomodBlock) Indent() Indent  { return tabBraceIndent }

func (s gomodNormal) Lex(line []byte) ([]Token, State) {
	body, comment := gomodCommentAt(line)

	tokens := []Token{}

	at := gomodSkipSpace(line, 0, body)
	if word := gomodWordEnd(line, at, body); word > at && gomodDirectives[string(line[at:word])] {
		tokens = append(tokens, Token{Start: at, End: word, Kind: KindKeyword})
		at = word
	}

	tokens = append(tokens, gomodLexBody(line, at, body)...)

	return append(tokens, comment...), gomodNext(line, body, s)
}

func (s gomodBlock) Lex(line []byte) ([]Token, State) {
	body, comment := gomodCommentAt(line)

	tokens := gomodLexBody(line, 0, body)

	return append(tokens, comment...), gomodNext(line, body, s)
}

// gomodNext 는 다음 줄의 문맥이다. 괄호로 연 덩이가 여러 줄에 걸친다.
func gomodNext(line []byte, body int, current State) State {
	code := bytes.TrimRight(line[:body], " \t")

	if _, inBlock := current.(gomodBlock); inBlock {
		if head := bytes.TrimLeft(code, " \t"); len(head) > 0 && head[0] == ')' {
			return gomodNormal{}
		}

		return current
	}

	if bytes.HasSuffix(code, []byte{'('}) {
		return gomodBlock{}
	}

	return current
}

// gomodCommentAt 은 주석이 없는 앞부분의 끝과 주석 토큰이다.
//
// **문자열을 살피지 않는다.** go.mod 문법에는 따옴표로 감싼 경로가 있지만 `gofmt` 도
// `go mod tidy` 도 그것을 벗겨 내서, 사람이 손으로 적어 넣지 않는 한 파일에 남지 않는다.
// 블록 주석도 없다 — `//` 가 나오는 자리가 곧 주석이고, 모듈 경로에 `//` 가 연달아 오는
// 일은 없다.
func gomodCommentAt(line []byte) (int, []Token) {
	at := bytes.Index(line, []byte("//"))
	if at < 0 {
		return len(line), nil
	}

	return at, []Token{{Start: at, End: len(line), Kind: KindComment}}
}

// gomodLexBody 는 지시자 뒤를 훑는다. 낱말마다 모양을 보고 갈래를 고른다.
func gomodLexBody(line []byte, from, limit int) []Token {
	tokens := []Token{}

	for at := from; at < limit; {
		at = gomodSkipSpace(line, at, limit)
		if at >= limit {
			break
		}

		end := gomodWordEnd(line, at, limit)
		if end == at {
			at++ // 낱말을 가르는 글자다

			continue
		}

		tokens = append(tokens, gomodWordTokens(line, at, end)...)
		at = end
	}

	return tokens
}

// gomodWordTokens 는 낱말 하나의 토큰이다.
//
// `=>` 는 바꿔치기를 가리키는 표시라 지시자와 같은 갈래다. `이름=값`(`godebug`) 은 둘로
// 갈라 낸다. 나머지는 모양이 버전이면 숫자, 아니면 경로로 보아 키다.
func gomodWordTokens(line []byte, start, end int) []Token {
	word := line[start:end]

	if string(word) == "=>" {
		return []Token{{Start: start, End: end, Kind: KindKeyword}}
	}

	if equal := bytes.IndexByte(word, '='); equal > 0 {
		tokens := []Token{{Start: start, End: start + equal, Kind: KindKey}}
		if start+equal+1 < end {
			tokens = append(tokens, Token{
				Start: start + equal + 1,
				End:   end,
				Kind:  gomodWordKind(word[equal+1:]),
			})
		}

		return tokens
	}

	return []Token{{Start: start, End: end, Kind: gomodWordKind(word)}}
}

// gomodWordKind 는 낱말의 모양으로 고른 갈래다. 버전으로 읽히면 숫자, 아니면 키다.
//
// **지시자마다 「몇 번째 낱말이 버전인가」를 적어 두지 않는다.** 그 표는 지시자가 늘 때마다
// 같이 늘고, go.mod 와 go.work 가 지시자를 나눠 쓰고 있어 두 벌이 된다. 버전은 모양이
// 뚜렷해서 모양만으로 갈린다.
func gomodWordKind(word []byte) Kind {
	if gomodVersion(word) {
		return KindNumber
	}

	return KindKey
}

// gomodVersion 은 낱말이 버전으로 읽히는지다. 세 가지 모양이 있다.
//
//   - `v1.2.3` — 모듈 버전이다. 뒤에 `-rc1` 이나 `/go.mod` 가 붙어도 버전이다
//   - `go1.24.0` — `toolchain` 줄이다
//   - `1.24` — `go` 줄이다
//
// `v` 로 시작하는 모듈 경로(`v2.example.com/x`) 가 있으면 버전으로 읽힌다. 경로와 버전이
// 같은 자리에 오지 않으므로 화면에서 헷갈릴 일은 없고, 가르려면 낱말 하나가 아니라 줄의
// 짜임을 알아야 한다.
func gomodVersion(word []byte) bool {
	if len(word) > 1 && word[0] == 'v' && gomodDigit(word[1]) {
		return true
	}

	if len(word) > 2 && bytes.HasPrefix(word, []byte("go")) && gomodDigit(word[2]) {
		return true
	}

	return gomodNumber(word)
}

// gomodNumber 는 숫자와 점으로만 된 낱말인지다. `go 1.24` 의 뒷자리다.
//
// 첫 글자가 숫자인 것만으로는 모자라다 — `4d63.com/gochecknoglobals` 처럼 숫자로 시작하는
// 모듈 경로가 있다.
func gomodNumber(word []byte) bool {
	if len(word) == 0 || !gomodDigit(word[0]) {
		return false
	}

	for _, b := range word {
		if !gomodDigit(b) && b != '.' {
			return false
		}
	}

	return true
}

func gomodDigit(b byte) bool { return b >= '0' && b <= '9' }

func gomodSkipSpace(line []byte, at, limit int) int {
	for at < limit && (line[at] == ' ' || line[at] == '\t') {
		at++
	}

	return at
}

// gomodWordEnd 는 그 자리에서 시작하는 낱말이 끝나는 자리다. 낱말이 아니면 at 그대로다.
//
// 빈 칸 말고 `(` `)` `[` `]` `,` 도 낱말을 가른다. 덩이를 여는 괄호와 `retract [v1.0.0,
// v1.1.0]` 의 범위가 낱말에 붙어 있어서, 빈 칸만으로 가르면 `[v1.0.0,` 이 한 덩어리가 된다.
func gomodWordEnd(line []byte, at, limit int) int {
	end := at
	for end < limit && !gomodSeparator(line[end]) {
		end++
	}

	return end
}

func gomodSeparator(b byte) bool {
	switch b {
	case ' ', '\t', '(', ')', '[', ']', ',':
		return true
	}

	return false
}

// gomodDirectives 는 go.mod 와 go.work 의 지시자 전부다.
//
// 두 파일의 것을 한 표에 둔다. 문법이 같고 지시자 낱말만 갈리는데, 표를 나누면 문맥도 둘로
// 갈라야 하고 얻는 것은 go.work 에 `require` 를 적었을 때 색이 안 나오는 것뿐이다.
var gomodDirectives = map[string]bool{
	// go.mod
	"module": true, "require": true, "exclude": true, "retract": true, "tool": true,
	// go.work
	"use": true,
	// 둘 다
	"go": true, "toolchain": true, "godebug": true, "replace": true, "ignore": true,
}
