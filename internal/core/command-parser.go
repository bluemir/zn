package core

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// command 는 명령줄에 친 한 줄을 뜯어놓은 것이다.
//
// `!` 는 이름의 일부가 아니라 따로 뗀다. vim 에서 `!` 는 "묻지 말고 해라" 라는 같은 뜻을
// 어느 명령에나 붙이는 것이라, 이름에 붙여두면 `q` 와 `q!` 처럼 같은 명령이 둘로 갈라진다.
type command struct {
	name  string   // w, q, tabnew ...
	force bool     // 뒤에 붙은 `!`
	args  []string // 파일 이름 등
}

// parseCommand 는 `:` 뒤에 친 것을 command 로 만든다.
//
// 토큰에서 command 를 만드는 것은 "첫 토큰이 이름, 나머지가 인자" 로 끝나서 상태가 없다.
// 범위 문법(`:1,5d`) 처럼 앞자리가 늘어나면 그때 이 자리에 상태 기계가 들어온다.
func parseCommand(input string) (command, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return command{}, err
	}
	if len(tokens) == 0 {
		return command{}, nil
	}

	name := tokens[0]

	return command{
		name:  strings.TrimSuffix(name, "!"),
		force: strings.HasSuffix(name, "!"),
		args:  tokens[1:],
	}, nil
}

// tokenize 는 글자를 하나씩 먹으면서 토큰으로 끊는다.
//
// 공백으로 끊는 것이 기본이고, 따옴표와 `\` 로 공백이 든 파일 이름을 쓸 수 있다.
// 나중에 `:!ls -la` 처럼 뒤를 통째로 넘겨야 하는 명령이 생기면 여기에 상태를 하나 더한다.
func tokenize(input string) ([]string, error) {
	tokens := []string{}

	var state tokenizerState = tokenPlain{}
	for _, ch := range input {
		var token string
		token, state = state.consume(ch)

		if token != "" {
			tokens = append(tokens, token)
		}
	}

	// 입력이 끝나도 모으던 토큰이 남아 있다. 흘리지 않으면 마지막 인자가 사라진다.
	last, err := state.end()
	if err != nil {
		return nil, err
	}
	if last != "" {
		tokens = append(tokens, last)
	}

	return tokens, nil
}

// tokenizerState 는 tokenize 가 옮겨 다니는 상태다.
//
// consume 은 토큰 하나가 끝났으면 그것을 돌려준다. 아직 모으는 중이면 빈 문자열이다.
// end 는 입력이 끝났음을 알린다. 닫히지 않은 따옴표처럼 끝나면 안 되는 자리에서는 오류를 낸다.
type tokenizerState interface {
	consume(ch rune) (string, tokenizerState)
	end() (string, error)
}

// tokenPlain 은 따옴표 밖이다. 공백을 만나면 토큰이 끝난다.
type tokenPlain struct {
	buf []rune
}

func (s tokenPlain) consume(ch rune) (string, tokenizerState) {
	switch ch {
	case ' ', '\t':
		// 모으는 중이 아니면 토큰 사이의 공백이라 흘려보낸다.
		if len(s.buf) == 0 {
			return "", s
		}

		return string(s.buf), tokenPlain{}
	case '"':
		// 토큰 중간에서도 열 수 있다. `a"b c"` 는 `ab c` 한 토큰이다.
		return "", tokenQuoted{buf: s.buf}
	case '\\':
		return "", tokenEscaped{buf: s.buf}
	default:
		return "", tokenPlain{buf: append(s.buf, ch)}
	}
}

func (s tokenPlain) end() (string, error) {
	return string(s.buf), nil
}

// tokenQuoted 는 따옴표 안이다. 닫는 따옴표까지 전부 글자 그대로다.
//
// 안에서는 `\` 도 글자다. 그래야 `"C:\path\to"` 를 그대로 쓸 수 있다.
// 따옴표 자체를 넣어야 하면 따옴표 밖에서 `\"` 로 쓴다.
type tokenQuoted struct {
	buf []rune
}

func (s tokenQuoted) consume(ch rune) (string, tokenizerState) {
	if ch == '"' {
		// 토큰이 끝난 것이 아니라 따옴표가 끝난 것이다. `"a b"c` 는 `a bc` 한 토큰이다.
		return "", tokenPlain{buf: s.buf}
	}

	return "", tokenQuoted{buf: append(s.buf, ch)}
}

func (s tokenQuoted) end() (string, error) {
	return "", errors.New("따옴표가 닫히지 않았습니다")
}

// tokenEscaped 는 `\` 바로 뒤다. 다음 글자 하나를 뜻 없이 그대로 받는다.
type tokenEscaped struct {
	buf []rune
}

func (s tokenEscaped) consume(ch rune) (string, tokenizerState) {
	return "", tokenPlain{buf: append(s.buf, ch)}
}

func (s tokenEscaped) end() (string, error) {
	return "", errors.New("`\\` 뒤에 글자가 없습니다")
}
