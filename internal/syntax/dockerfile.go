package syntax

import (
	"bytes"
	"strings"
)

// dockerNormal 은 Dockerfile 의 보통 문맥이다. 줄 앞이 명령 이름이다.
type dockerNormal struct{}

// dockerContinued 는 앞 줄이 `\` 로 이어진 자리다. 줄 앞에 명령 이름이 없다.
type dockerContinued struct{}

func (dockerNormal) Indent() Indent    { return dockerIndent{} }
func (dockerContinued) Indent() Indent { return dockerIndent{} }

func (s dockerNormal) Lex(line []byte) ([]Token, State) {
	limit, comment := dockerCommentAt(line)
	if limit == 0 {
		return comment, dockerNext(line, s)
	}

	at := 0
	for at < limit && line[at] == ' ' {
		at++
	}

	tokens := []Token{}

	// 명령 이름은 대소문자를 가리지 않는다. `from` 도 `FROM` 도 같은 명령이다.
	word := at
	for word < limit && line[word] != ' ' {
		word++
	}
	if word > at && dockerInstructions[strings.ToUpper(string(line[at:word]))] {
		tokens = append(tokens, Token{Start: at, End: word, Kind: KindKeyword})
		at = word
	}

	tokens = append(tokens, dockerLexBody(line, at, limit)...)

	return append(tokens, comment...), dockerNext(line, s)
}

func (s dockerContinued) Lex(line []byte) ([]Token, State) {
	limit, comment := dockerCommentAt(line)

	tokens := dockerLexBody(line, 0, limit)

	return append(tokens, comment...), dockerNext(line, s)
}

// dockerNext 는 다음 줄의 문맥이다. 줄이 `\` 로 끝나면 다음 줄이 이 줄의 뒷부분이다.
func dockerNext(line []byte, current State) State {
	if bytes.HasSuffix(bytes.TrimRight(line, " "), []byte{'\\'}) {
		return dockerContinued{}
	}

	if _, ok := current.(dockerContinued); ok {
		return dockerNormal{}
	}

	return current
}

// dockerCommentAt 은 주석이 시작하는 자리와 그 토큰이다.
// 주석이 없으면 limit 이 줄 길이다.
//
// `# syntax=docker/dockerfile:1` 같은 지시자도 그냥 주석이다. 다른 도구에게도 주석이고
// 파일에 한 번 나오므로, 그것만 세 조각으로 가르지 않는다(docs/tasks.md).
func dockerCommentAt(line []byte) (int, []Token) {
	at := 0
	for at < len(line) && line[at] == ' ' {
		at++
	}

	if at < len(line) && line[at] == '#' {
		return 0, []Token{{Start: 0, End: len(line), Kind: KindComment}}
	}

	return len(line), nil
}

// dockerLexBody 는 명령 이름 뒤를 훑는다.
//
// 본문은 대개 shell 이라 색을 입히지 않는다 — shell lexer 가 없다. 어느 shell 에서나 같은
// 뜻인 것만 집는다: 값을 가리키는 `${...}`·`$VAR`, `--flag` 의 이름, 따옴표 안.
func dockerLexBody(line []byte, from, limit int) []Token {
	tokens := []Token{}

	for at := from; at < limit; {
		switch {
		case line[at] == '$':
			// `$$` 는 `$` 한 글자다. 둘을 같이 건너뛰지 않으면 뒤의 `$` 가 참조로 읽힌다.
			if at+1 < limit && line[at+1] == '$' {
				at += 2

				continue
			}

			if end, ok := shellExpansionEnd(line, at, limit); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindVariable})
				at = end

				continue
			}
		case line[at] == '"', line[at] == '\'':
			if end, ok := quotedEnd(line, at, limit); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				at = end

				continue
			}
		case line[at] == '-' && at+1 < limit && line[at+1] == '-' && dockerFlagStart(line, at):
			end := at + 2
			for end < limit && line[end] != '=' && line[end] != ' ' {
				end++
			}
			if end > at+2 {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindVariable})
				at = end

				continue
			}
		case dockerStageWord(line, at, limit):
			tokens = append(tokens, Token{Start: at, End: at + 2, Kind: KindKeyword})
			at += 2

			continue
		}

		at++
	}

	return tokens
}

// dockerFlagStart 는 `--` 가 낱말의 시작인지다. 줄 가운데의 `a--b` 를 flag 로 보지 않는다.
func dockerFlagStart(line []byte, at int) bool {
	return at == 0 || line[at-1] == ' '
}

// dockerStageWord 는 그 자리가 `AS`(단계 이름 붙이기) 인지다.
func dockerStageWord(line []byte, at, limit int) bool {
	if at+2 > limit || (at > 0 && line[at-1] != ' ') {
		return false
	}
	if at+2 < limit && line[at+2] != ' ' {
		return false
	}

	return strings.ToUpper(string(line[at:at+2])) == "AS"
}

// dockerInstructions 는 Dockerfile 명령 전부다. 대문자로 적고 견줄 때 올려 맞춘다.
var dockerInstructions = map[string]bool{
	"ADD": true, "ARG": true, "CMD": true, "COPY": true, "ENTRYPOINT": true,
	"ENV": true, "EXPOSE": true, "FROM": true, "HEALTHCHECK": true, "LABEL": true,
	"MAINTAINER": true, "ONBUILD": true, "RUN": true, "SHELL": true, "STOPSIGNAL": true,
	"USER": true, "VOLUME": true, "WORKDIR": true,
}

// dockerIndent 는 dockerfile 의 들여쓰기 규칙이다. 블록이 없고 `\` 로 이어지는 줄만 있다.
type dockerIndent struct{}

func (dockerIndent) Next(line []byte, _ []Token) (int, []byte) {
	continues := bytes.HasSuffix(bytes.TrimRight(line, " \t"), []byte{'\\'})
	indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')

	switch {
	case continues && !indented:
		return 1, nil // 이어짐이 시작한다
	case !continues && indented:
		return -1, nil // 이어짐이 끝났다
	}

	return 0, nil
}

// Close 는 언제나 0 이다.
func (dockerIndent) Close(_ []byte) int { return 0 }

func (dockerIndent) Reindents() bool { return true }

func (dockerIndent) TabIndentsLine([]byte) bool { return false }

// Unit 은 space 네 칸이다. 이어지는 줄을 눈에 띄게 물리는 것이 dockerfile 의 흔한 모양이다.
func (dockerIndent) Unit() []byte { return []byte("    ") }
