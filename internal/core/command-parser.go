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

	// lines 는 이름 앞에 붙은 줄 범위다(`:1,5d` 의 `1,5`). 치지 않았으면 zero 이고,
	// 그때 무엇을 대신 쓸지는 명령이 정한다 — `:d` 는 커서 줄이다.
	lines lineRange
}

// parseCommand 는 `:` 뒤에 친 것을 command 로 만든다.
//
// 상태 기계 둘을 잇는다 — 글자에서 토큰을 끊는 것(tokenizerState) 과 토큰에서 command 를
// 만드는 것(commandState) 이다. 토큰은 자기가 무엇으로 끊긴 것인지를 들고 다니므로(tokenKind)
// 뒤쪽 기계가 문자열을 보고 갈래를 되짚는 자리가 없다.
//
// currentPath 는 인자 자리의 `%` 를 바꿔 놓을 경로다. 이름 없는 tab 이면 빈 문자열이고,
// 그때 `%` 를 치면 오류다(ADR-0114).
func parseCommand(input string, currentPath string) (command, error) {
	tokens, err := tokenize(input, currentPath)
	if err != nil {
		return command{}, err
	}

	var state commandState = commandRange{}
	for _, t := range tokens {
		state = state.consume(t)
	}

	return state.end()
}

// commandState 는 토큰에서 command 를 만들며 옮겨 다니는 상태다.
//
// 자리마다 뜻이 갈리는 토큰이 있어서 "몇 번째 토큰인가" 를 세지 않고 상태로 안다.
// 이름 앞의 범위와 맨 앞의 `!` 가 그런 자리다.
//
// end 는 입력이 끝났음을 알린다. 뜯다 막힌 상태(commandFailed) 가 그 오류를 여기서 낸다.
type commandState interface {
	consume(t token) commandState
	end() (command, error)
}

// commandRange 는 이름 앞의 범위 자리다. 명령줄의 첫 상태다.
//
// 범위 토큰이 아니면 그 토큰부터가 이름이라 곧바로 넘긴다 — 범위는 있어도 되고 없어도 된다.
type commandRange struct{}

func (s commandRange) consume(t token) commandState {
	if t.kind != tokenKindRange {
		return commandName{}.consume(t)
	}

	lines, err := parseLineRange(t.text)
	if err != nil {
		return commandFailed{err: err}
	}

	return commandName{lines: lines}
}

// `:` 만 치고 enter 를 누른 자리다.
func (s commandRange) end() (command, error) { return command{}, nil }

// commandName 은 이름 토큰을 기다리는 자리다. 이름과 force 를 가르는 것도 여기다.
type commandName struct {
	lines lineRange
}

func (s commandName) consume(t token) commandState {
	// 이름 자리의 `!` 는 이름이 `!` 이고 뒤가 셸 줄이다. 이름에 붙는 `!`(force) 와 뜻이
	// 다른데, 그 갈림은 글자 단위 기계가 이미 했고 여기는 갈래를 읽을 뿐이다.
	if t.kind == tokenKindShell {
		cmd := command{name: "!", lines: s.lines}

		// `:!` 만 쳤으면 넘길 것이 없다. 빈 줄을 셸에 넘기지 않는다.
		if t.text != "" {
			cmd.args = []string{t.text}
		}

		return commandArgs{cmd: cmd}
	}

	if name, ok := strings.CutSuffix(t.text, "!"); ok {
		return commandArgs{cmd: command{name: name, force: true, lines: s.lines}}
	}

	return commandArgs{cmd: command{name: t.text, lines: s.lines}}
}

// 범위만 치고 이름이 없다. `:5` 로 그 줄로 가는 자리다.
func (s commandName) end() (command, error) { return command{lines: s.lines}, nil }

// commandArgs 는 나머지 토큰을 인자로 모으는 자리다.
type commandArgs struct {
	cmd command
}

func (s commandArgs) consume(t token) commandState {
	// 뒤를 통째로 받는 자리가 **공백뿐이면** 인자가 아니다. `:grep ` 도 `:grep   ` 도
	// `:grep` 과 같아야 한다 — 그것을 인자로 세면 「패턴을 댔다」가 되어 물어보는 길로 못 가고,
	// 공백 두 칸을 찾는 정규식이 되어 온 저장소가 걸린다. 오타일 가능성이 훨씬 크다.
	//
	// 공백을 정말 찾아야 하면 `\s` 나 `[ ]` 로 쓴다. 이름 자리의 `!` 가 빈 경우를
	// commandName 에서 이미 가르고, 여기는 그것을 공백까지로 넓힌 것이다(ADR-0045, ADR-0077).
	if t.kind == tokenKindShell && strings.TrimSpace(t.text) == "" {
		return s
	}

	s.cmd.args = append(s.cmd.args, t.text)

	return s
}

func (s commandArgs) end() (command, error) { return s.cmd, nil }

// commandFailed 는 뜯다 막힌 자리다. 남은 토큰을 먹고 끝에서 그 오류를 낸다.
//
// 막힌 자리에서 바로 오류를 돌려주지 않는 것은 consume 이 상태만 주기 때문이다.
// 남은 토큰을 먹어도 결과가 달라지지 않으니 흐름을 끊을 이유도 없다.
type commandFailed struct {
	err error
}

func (s commandFailed) consume(token) commandState { return s }

func (s commandFailed) end() (command, error) { return command{}, s.err }

// tokenKind 는 토큰이 무엇으로 끊긴 것인지다. 글자 단위 기계가 정하고 토큰 단위 기계가 읽는다.
//
// 갈래를 토큰에 실어 보내면 뒤쪽 기계가 문자열을 보고 갈래를 되짚지 않는다 — 어디서 끊겼는지는
// 끊은 쪽만 알 수 있다. `:1,5d` 의 `1,5` 와 `:w 1,5` 의 `1,5` 는 글자가 같고 뜻이 다르다.
type tokenKind int

const (
	tokenKindWord  tokenKind = iota // 여느 토큰. 이름이거나 인자다
	tokenKindRange                  // 이름 앞의 줄 범위(`1,5`, `%`, `.,+3`)
	tokenKindShell                  // 이름 자리의 `!` 뒤를 뜯지 않은 한 줄
)

// token 은 끊어놓은 한 조각이다.
type token struct {
	text string
	kind tokenKind
}

// tokenize 는 글자를 하나씩 먹으면서 토큰으로 끊는다.
//
// 공백으로 끊는 것이 기본이고, 따옴표와 `\` 로 공백이 든 파일 이름을 쓸 수 있다.
// 이름 앞의 줄 범위는 tokenRange 가, `:!ls -la` 의 뒤쪽은 tokenRest 가 통째로 끊는다.
//
// 첫 상태가 tokenPlain 이 아니라 tokenHead 인 것이 이 함수가 하는 유일한 판단이다 —
// 위치에 따라 뜻이 갈리는 글자(맨 앞의 `!` 와 범위) 가 있고, 몇 번째인지를 아는 자리가 여기뿐이다.
//
// currentPath 는 인자 자리를 지나는 상태들이 이어 들고 다닌다. 들고 있는 상태만 `%` 를 편다 —
// 셸 줄과 정규식은 받지 않으므로 그쪽에서는 `%` 가 글자로 남는다(ADR-0114).
func tokenize(input string, currentPath string) ([]token, error) {
	tokens := []token{}

	var state tokenizerState = tokenHead{currentPath: currentPath}
	for _, ch := range input {
		var t token
		t, state = state.consume(ch)

		if keepToken(t) {
			tokens = append(tokens, t)
		}
	}

	// 입력이 끝나도 모으던 토큰이 남아 있다. 흘리지 않으면 마지막 인자가 사라진다.
	last, err := state.end()
	if err != nil {
		return nil, err
	}
	if keepToken(last) {
		tokens = append(tokens, last)
	}

	return tokens, nil
}

// keepToken 은 모아둘 토큰인지다.
//
// 아직 모으는 중인 상태는 빈 토큰을 주므로 그것을 걸러낸다. 셸 줄만 예외다 —
// 비어 있다는 것도 결과여서(`:!` 만 친 것) 여기서 버리면 명령이 이름을 잃는다.
func keepToken(t token) bool {
	return t.text != "" || t.kind == tokenKindShell
}

// tokenizerState 는 tokenize 가 옮겨 다니는 상태다.
//
// consume 은 토큰 하나가 끝났으면 그것을 돌려준다. 아직 모으는 중이면 빈 토큰이다.
// end 는 입력이 끝났음을 알린다. 닫히지 않은 따옴표처럼 끝나면 안 되는 자리에서는 오류를 낸다.
type tokenizerState interface {
	consume(ch rune) (token, tokenizerState)
	end() (token, error)
}

// tokenHead 는 줄 맨 앞이다. 위치가 곧 뜻인 글자가 둘 있고, 그 갈림이 여기서 끝난다.
//
// `!` 는 "뒤를 통째로 셸에" 이고 — `:w !foo` 의 `!` 는 파일 이름이라 뜻이 없다 —
// 숫자·`.`·`$`·`%` 는 줄 범위다. `:e 1,5` 의 `1,5` 는 파일 이름이다.
type tokenHead struct {
	currentPath string
}

func (s tokenHead) consume(ch rune) (token, tokenizerState) {
	switch {
	case ch == ' ' || ch == '\t':
		// 앞의 공백을 지나도 아직 맨 앞이다. `: !ls` 도 `: 1,5d` 도 그대로 먹는다.
		return token{}, s
	case ch == '!':
		// 셸 줄은 currentPath 를 받지 않는다. `:!` 의 `%` 를 펴는 자리는 runShell 하나다 —
		// 팔레트의 `!` 는 이 기계를 지나지 않고 거기로 곧장 온다(ADR-0114).
		return token{}, tokenRest{}
	case isRangeChar(ch):
		return token{}, tokenRange{buf: []rune{ch}, currentPath: s.currentPath}
	default:
		// 맨 앞이 아니게 되었다. 그 글자부터가 이름 자리다.
		return tokenName{currentPath: s.currentPath}.consume(ch)
	}
}

func (s tokenHead) end() (token, error) {
	return token{}, nil
}

// isRangeChar 는 줄 범위를 **여는** 글자다. 주소(`42` `.` `$` `%` `'`)·자리 옮김(`+3` `-2`)·쉼표다.
//
// 이 글자로 시작하는 명령 이름은 없다. 그래서 맨 앞에서 이것을 만나면 범위로 읽어도
// 이름과 부딪히지 않는다. `'` 도 그렇다 — `:'` 로 시작하는 명령은 없다(ADR-0089).
//
// `<` 와 `>` 는 여기 없다. 고른 범위(`'<` `'>`) 의 뒷글자라 `'` 뒤에서만 오고, 여기 넣으면
// `:>` 가 「범위를 알 수 없습니다」로 걸려서 「알 수 없는 명령」이라고 말할 자리를 잃는다.
func isRangeChar(ch rune) bool {
	switch ch {
	case '.', '$', '%', ',', '+', '-', '\'':
		return true
	default:
		return ch >= '0' && ch <= '9'
	}
}

// tokenRange 는 이름 앞의 줄 범위 자리다. 범위 글자를 다 모아 한 토큰으로 준다.
//
// 안쪽 문법(주소 둘과 자리 옮김) 은 뜯지 않는다. 그것을 읽는 자리는 parseLineRange 하나다.
type tokenRange struct {
	buf         []rune
	currentPath string
}

func (s tokenRange) consume(ch rune) (token, tokenizerState) {
	// `<` `>` 는 범위를 열지는 못하지만 안쪽에서는 범위 글자다. `'<` `'>` 의 뒷글자라
	// `'` 뒤에만 오고, 그것을 가리는 것은 안쪽 문법을 읽는 parseLineAddress 의 일이다.
	if isRangeChar(ch) || ch == '<' || ch == '>' {
		return token{}, tokenRange{buf: append(s.buf, ch), currentPath: s.currentPath}
	}

	// 범위가 끝났고 이 글자부터가 이름 자리다. `!` 가 뜻을 갖는 것은 줄 맨 앞이 아니라
	// **이름 자리** 라서, 범위 뒤에 붙은 것도 셸이다(`:1,5!sort`).
	if ch == '!' {
		return token{text: string(s.buf), kind: tokenKindRange}, tokenRest{}
	}

	// 이름 자리로 넘긴다 — 빈 buf 로 시작하는 tokenName 은 글자 하나에 토큰을 내지 않으므로
	// 그쪽 토큰은 늘 비어 있다.
	_, next := tokenName{currentPath: s.currentPath}.consume(ch)

	return token{text: string(s.buf), kind: tokenKindRange}, next
}

func (s tokenRange) end() (token, error) {
	return token{text: string(s.buf), kind: tokenKindRange}, nil
}

// tokenRest 는 뒤를 통째로 넘기는 자리다. 공백도 따옴표도 `\` 도 전부 글자다 —
// 그것을 읽는 것은 셸이다.
type tokenRest struct {
	buf []rune
}

func (s tokenRest) consume(ch rune) (token, tokenizerState) {
	return token{}, tokenRest{buf: append(s.buf, ch)}
}

func (s tokenRest) end() (token, error) {
	return token{text: string(s.buf), kind: tokenKindShell}, nil
}

// tokenName 은 이름 토큰을 모으는 자리다. tokenPlain 과 같이 끊되, **이름이 끝났을 때 그
// 이름이 뒤를 통째로 받는 것이면 tokenRest 로 넘긴다.**
//
// ADR-0045 는 「뜻이 갈리는 것은 자리뿐」으로 두었다. `!` 는 자리로 알 수 있어서 그것으로
// 되었는데 정규식 패턴은 그렇지 않다 — `:e pat` 의 pat 은 파일 이름이라 뜯어야 하고
// `:grep pat` 의 pat 은 통째여야 한다. 자리가 같고 이름만 다르다.
//
// **`\` 가 그 갈림을 강제한다.** tokenEscaped 가 `\d` 를 `d` 로 만들어 버려서, 뜯은 뒤에
// 다시 이으면 `\d+` 가 `d+` 가 된다 — 정규식이 조용히 다른 뜻이 된다(ADR-0077).
type tokenName struct {
	buf         []rune
	currentPath string
}

func (s tokenName) consume(ch rune) (token, tokenizerState) {
	switch ch {
	case ' ', '\t':
		// 모으는 중이 아니면 이름 앞의 공백이라 흘려보낸다.
		if len(s.buf) == 0 {
			return token{}, s
		}

		// 통째로 넘기는 인자는 정규식이라 `%` 를 펴지 않는다. currentPath 를 주지 않는 것이
		// 곧 그 규칙이다 — `:s/50%/60%/` 의 `%` 는 글자다(ADR-0114).
		if takesRawArgument(string(s.buf)) {
			return token{text: string(s.buf)}, tokenRest{}
		}

		return token{text: string(s.buf)}, tokenPlain{currentPath: s.currentPath}
	case '"', '\\':
		// 이름에는 따옴표도 `\` 도 없다. 이름 자리가 아니었던 것으로 보고 여느 자리로 넘긴다.
		return tokenPlain{buf: s.buf, currentPath: s.currentPath}.consume(ch)
	default:
		// `:s` 는 이름과 인자 사이에 공백이 없다. **이름 다음 글자가 곧 구분자다**
		// (`:%s/a/b/g`). 공백을 기다리면 그 줄이 통째로 한 토큰이 되고, 그때 `\/` 가
		// tokenEscaped 를 지나 `/` 가 되어 정규식이 조용히 다른 뜻이 된다 — `:grep` 을
		// 통째로 넘기게 만든 것과 같은 까닭이다(ADR-0077, ADR-0084).
		if takesDelimitedArgument(string(s.buf)) && isSubstituteDelimiter(ch) {
			return token{text: string(s.buf)}, tokenRest{buf: []rune{ch}}
		}

		return token{}, tokenName{buf: append(s.buf, ch), currentPath: s.currentPath}
	}
}

// takesDelimitedArgument 는 이름 **다음 글자**가 구분자인 명령이다.
//
// `:s` 와 그 줄임말, 그리고 여러 파일 치환인 `:replace` 다. force 표시(`!`) 를 떼고 보지
// 않는다 — `:s!a!b!` 의 그 `!` 는 강제가 아니라 구분자다.
//
// `:replace` 가 같은 문법을 쓰는 것이 요점이다. 뜯는 자리가 `parseSubstitute` 하나라
// 구분자 규칙과 바꿀 글의 문법이 한 파일 치환과 갈릴 자리가 없다(ADR-0084, ADR-0097).
func takesDelimitedArgument(name string) bool {
	switch name {
	case "s", "substitute", "replace":
		return true
	default:
		return false
	}
}

func (s tokenName) end() (token, error) {
	return token{text: string(s.buf)}, nil
}

// takesRawArgument 는 이름 뒤를 뜯지 않고 통째로 넘기는 명령이다.
//
// 정규식을 받는 것들이다. `\` 가 그 갈림을 강제한다 — 뜯은 뒤에 다시 이으면 `\d+` 가
// `d+` 가 된다(ADR-0077). force 표시(`!`) 는 여기서 떼고 본다 — `:grep!` 도 패턴을
// 통째로 받아야 한다.
//
// `:s` 는 이름과 붙여 쓰는 것이 보통이라 그쪽 갈림은 takesDelimitedArgument 가 하고,
// 여기 있는 것은 띄어 쓴 `:s /a/b/` 를 받기 위해서다.
func takesRawArgument(name string) bool {
	switch strings.TrimSuffix(name, "!") {
	case "grep", "s", "substitute", "replace":
		return true
	default:
		return false
	}
}

// tokenPlain 은 따옴표 밖이다. 공백을 만나면 토큰이 끝난다.
type tokenPlain struct {
	buf         []rune
	currentPath string
}

func (s tokenPlain) consume(ch rune) (token, tokenizerState) {
	switch ch {
	case ' ', '\t':
		// 모으는 중이 아니면 토큰 사이의 공백이라 흘려보낸다.
		if len(s.buf) == 0 {
			return token{}, s
		}

		return token{text: string(s.buf)}, tokenPlain{currentPath: s.currentPath}
	case '"':
		// 토큰 중간에서도 열 수 있다. `a"b c"` 는 `ab c` 한 토큰이다.
		return token{}, tokenQuoted{buf: s.buf, currentPath: s.currentPath}
	case '\\':
		return token{}, tokenEscaped{buf: s.buf, currentPath: s.currentPath}
	case '%':
		// 인자 자리의 `%` 는 지금 보고 있는 파일이다. 글자 그대로 쓰려면 `\%` 로 친다 —
		// tokenEscaped 가 다음 글자를 뜻 없이 넣으므로 그쪽은 저절로 글자가 된다(ADR-0114).
		if s.currentPath == "" {
			return token{}, tokenFailed{err: errNoCurrentFile}
		}

		return token{}, tokenPlain{buf: append(s.buf, []rune(s.currentPath)...), currentPath: s.currentPath}
	default:
		return token{}, tokenPlain{buf: append(s.buf, ch), currentPath: s.currentPath}
	}
}

func (s tokenPlain) end() (token, error) {
	return token{text: string(s.buf)}, nil
}

// errNoCurrentFile 은 펼 파일이 없는데 `%` 를 친 것이다. 이름 없는 tab 이거나 tab 이 없다.
//
// 조용히 빈 문자열로 펴지 않는다. `:!rm %` 가 `rm ` 이 되어 아무 일도 없었던 것처럼 보이거나,
// 더 나쁘게는 뒤에 오는 것을 지운다. vim 도 여기서 명령을 세운다(E499).
var errNoCurrentFile = errors.New("이름 없는 파일이라 `%` 를 펼 수 없습니다")

// tokenQuoted 는 따옴표 안이다. 닫는 따옴표까지 전부 글자 그대로다.
//
// 안에서는 `\` 도 글자다. 그래야 `"C:\path\to"` 를 그대로 쓸 수 있다.
// 따옴표 자체를 넣어야 하면 따옴표 밖에서 `\"` 로 쓴다.
type tokenQuoted struct {
	buf         []rune
	currentPath string
}

func (s tokenQuoted) consume(ch rune) (token, tokenizerState) {
	if ch == '"' {
		// 토큰이 끝난 것이 아니라 따옴표가 끝난 것이다. `"a b"c` 는 `a bc` 한 토큰이다.
		return token{}, tokenPlain{buf: s.buf, currentPath: s.currentPath}
	}

	// 따옴표는 공백이 든 파일 이름을 쓰려고 여는 것이지 확장을 막는 울이 아니다.
	// `:e "%"` 도 지금 파일이다. 막는 것은 `\%` 하나다(ADR-0114).
	if ch == '%' {
		if s.currentPath == "" {
			return token{}, tokenFailed{err: errNoCurrentFile}
		}

		return token{}, tokenQuoted{buf: append(s.buf, []rune(s.currentPath)...), currentPath: s.currentPath}
	}

	return token{}, tokenQuoted{buf: append(s.buf, ch), currentPath: s.currentPath}
}

func (s tokenQuoted) end() (token, error) {
	return token{}, errors.New("따옴표가 닫히지 않았습니다")
}

// tokenEscaped 는 `\` 바로 뒤다. 다음 글자 하나를 뜻 없이 그대로 받는다.
//
// `\%` 가 글자 `%` 가 되는 것도 여기다. 따로 가리지 않는다 — 「다음 글자는 뜻이 없다」에
// 이미 들어 있다(ADR-0114).
type tokenEscaped struct {
	buf         []rune
	currentPath string
}

func (s tokenEscaped) consume(ch rune) (token, tokenizerState) {
	return token{}, tokenPlain{buf: append(s.buf, ch), currentPath: s.currentPath}
}

func (s tokenEscaped) end() (token, error) {
	return token{}, errors.New("`\\` 뒤에 글자가 없습니다")
}

// tokenFailed 는 글자를 먹다 막힌 자리다. 남은 글자를 먹고 끝에서 그 오류를 낸다.
//
// consume 이 오류를 돌려주지 않으므로 상태로 들고 간다. 토큰 단위 기계의 commandFailed 와
// 같은 결이다 — 막힌 뒤에 무엇을 더 먹어도 결과가 달라지지 않는다.
type tokenFailed struct {
	err error
}

func (s tokenFailed) consume(rune) (token, tokenizerState) { return token{}, s }

func (s tokenFailed) end() (token, error) { return token{}, s.err }
