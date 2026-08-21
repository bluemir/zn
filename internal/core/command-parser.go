package core

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// command 는 명령줄에 친 한 줄을 뜯어놓은 것이다.
//
// 이름 뒤에 붙은 `!` 는 이름의 일부가 아니라 따로 뗀다. vim 에서 그 `!` 는 "묻지 말고 해라"
// 라는 같은 뜻을 어느 명령에나 붙이는 것이라, 이름에 붙여두면 `q` 와 `q!` 처럼 같은 명령이
// 둘로 갈라진다.
//
// **맨 앞의 `!` 는 뜻이 다르다.** 그것은 강제가 아니라 이름 그 자체이고(`:!ls`), 뒤에 오는
// 것은 인자가 아니라 셸에 통째로 넘길 한 줄이다. 위치가 뜻을 가르는 자리라 상태 기계가 안다.
type command struct {
	name  string   // w, q, tabnew, ! ...
	force bool     // 이름 뒤에 붙은 `!`
	args  []string // 파일 이름 등. `:!` 는 뒤를 뜯지 않은 한 줄이 통째로 args[0] 이다
}

// parseCommand 는 `:` 뒤에 친 것을 command 로 만든다.
//
// 상태 기계 둘을 잇는다 — 글자에서 토큰을 끊는 것(tokenizerState) 과 토큰에서 command 를
// 만드는 것(commandState) 이다. 다 먹은 뒤 문자열을 다시 뜯어보는 자리는 없다.
func parseCommand(input string) (command, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return command{}, err
	}

	var state commandState = commandName{}
	for _, token := range tokens {
		state = state.consume(token)
	}

	return state.end(), nil
}

// commandState 는 토큰에서 command 를 만들며 옮겨 다니는 상태다.
//
// 자리마다 뜻이 갈리는 토큰이 있어서 "몇 번째 토큰인가" 를 세지 않고 상태로 안다. 맨 앞의 `!`
// 가 그 첫 자리이고, 범위 문법(`:1,5d`) 은 commandName 앞에 상태가 하나 더 붙는 자리가 된다.
//
// end 는 입력이 끝났음을 알린다. 오류를 내지 않는다 — 세 상태 모두 끝날 수 있는 자리이고,
// 닫히지 않은 따옴표처럼 끝나면 안 되는 것은 글자 단위 쪽이 이미 막았다.
type commandState interface {
	consume(token string) commandState
	end() command
}

// commandName 은 첫 토큰을 기다리는 자리다. 이름과 force 를 가르는 것도 여기다.
type commandName struct{}

func (s commandName) consume(token string) commandState {
	// 맨 앞의 `!` 는 그 자체가 이름이고 뒤가 셸 줄이다. 이름에 붙는 `!` 와 뜻이 다르다.
	if token == "!" {
		return commandShell{}
	}

	// `!` 하나는 위에서 갈렸으므로 여기 오는 `!` 는 늘 이름 뒤에 붙은 것이다.
	if name, ok := strings.CutSuffix(token, "!"); ok {
		return commandArgs{cmd: command{name: name, force: true}}
	}

	return commandArgs{cmd: command{name: token}}
}

// `:` 만 치고 enter 를 누른 자리다.
func (s commandName) end() command { return command{} }

// commandShell 은 `:!` 뒤를 기다리는 자리다. 오는 토큰 하나가 뜯지 않은 셸 줄이다.
type commandShell struct{}

func (s commandShell) consume(token string) commandState {
	return commandArgs{cmd: command{name: "!", args: []string{token}}}
}

// `:!` 만 쳤다. 이름은 있고 넘길 것이 없다.
func (s commandShell) end() command { return command{name: "!"} }

// commandArgs 는 나머지 토큰을 인자로 모으는 자리다.
type commandArgs struct {
	cmd command
}

func (s commandArgs) consume(token string) commandState {
	s.cmd.args = append(s.cmd.args, token)

	return s
}

func (s commandArgs) end() command { return s.cmd }

// tokenize 는 글자를 하나씩 먹으면서 토큰으로 끊는다.
//
// 공백으로 끊는 것이 기본이고, 따옴표와 `\` 로 공백이 든 파일 이름을 쓸 수 있다.
// `:!ls -la` 처럼 뒤를 통째로 넘기는 것은 tokenRest 가 한다.
//
// 첫 상태가 tokenPlain 이 아니라 tokenHead 인 것이 이 함수가 하는 유일한 판단이다 —
// 위치에 따라 뜻이 갈리는 글자(맨 앞의 `!`) 가 있고, 몇 번째인지를 아는 자리가 여기뿐이다.
func tokenize(input string) ([]string, error) {
	tokens := []string{}

	var state tokenizerState = tokenHead{}
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

// tokenHead 는 줄 맨 앞이다. `!` 가 "뒤를 통째로" 라는 뜻을 갖는 자리는 여기뿐이다 —
// `:w !foo` 의 `!` 는 파일 이름이라 뜻이 없다.
type tokenHead struct{}

func (s tokenHead) consume(ch rune) (string, tokenizerState) {
	switch ch {
	case ' ', '\t':
		// 앞의 공백을 지나도 아직 맨 앞이다. `: !ls` 도 셸 명령이다.
		return "", s
	case '!':
		// 이름은 `!` 하나로 끝나고 뒤는 통째로 한 토큰이다.
		return "!", tokenRest{}
	default:
		// 맨 앞이 아니게 되었다. 그 글자부터는 여느 자리와 같다.
		return tokenPlain{}.consume(ch)
	}
}

func (s tokenHead) end() (string, error) {
	return "", nil
}

// tokenRest 는 뒤를 통째로 넘기는 자리다. 공백도 따옴표도 `\` 도 전부 글자다 —
// 그것을 읽는 것은 셸이다.
type tokenRest struct {
	buf []rune
}

func (s tokenRest) consume(ch rune) (string, tokenizerState) {
	return "", tokenRest{buf: append(s.buf, ch)}
}

func (s tokenRest) end() (string, error) {
	return string(s.buf), nil
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
