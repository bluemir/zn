package core

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/textarea"
	"github.com/cockroachdb/errors"
)

// `:[범위]s/찾을 것/바꿀 글/flag` 를 뜯는 자리다.
//
// 빌려 쓰는 것이 둘이다 — 패턴 문법은 `/` 검색과 같은 RE2 이고(ADR-0010), 범위는 명령줄이
// 이미 뜯어 둔 것을 그대로 받는다(ADR-0046). 이 파일이 새로 정하는 것은 구분자 규칙과
// 바꿀 글의 문법 둘뿐이다 (ADR-0084).

// substitution 은 뜯어놓은 `:s` 한 줄이다.
type substitution struct {
	// input 은 친 패턴 그대로다. 못 찾았다고 알릴 때 이것을 보여준다 — 검색의 searchState 와
	// 같은 자리이고, 실제로 그리로 실려 간다.
	input string

	pattern *regexp.Regexp

	// replacement 는 Go 정규식의 `${1}` 문법으로 옮겨 둔 바꿀 글이다. 사람이 치는 것은
	// vim 의 `&`·`\1` 이고 옮기는 자리는 parseReplacement 하나다.
	replacement []byte

	all     bool // `g` — 한 줄의 모든 자리
	confirm bool // `c` — 한 자리씩 물어본다
}

// parseSubstitute 는 이름 뒤에 통째로 온 한 줄을 뜯는다. `/a/b/g` 가 온다.
//
// **첫 글자가 구분자다.** vim 처럼 아무 글자나 쓸 수 있다 — 경로를 바꿀 때 `/` 를 막지
// 않아도 되는 것이 그 값이다(`:%s#internal/core#internal/ui#`).
//
// **마지막 구분자만 생략할 수 있다.** 그 뒤에는 flag 밖에 없어서 없는 것과 비어 있는 것이
// 같다. 다른 생략은 없다 — `:%s/a` 는 바꿀 글을 대지 않은 것이고, 빈 패턴을 마지막 검색으로
// 채워 주지도 않는다.
func parseSubstitute(input string) (substitution, error) {
	rest := strings.TrimLeft(input, " \t")
	if rest == "" {
		return substitution{}, errors.New("바꿀 것을 대지 않았습니다")
	}

	delimiter, size := utf8.DecodeRuneInString(rest)
	if !isSubstituteDelimiter(delimiter) {
		return substitution{}, errors.Newf("구분자로 쓸 수 없습니다: %c", delimiter)
	}

	pattern, rest, closed := cutUnescaped(rest[size:], delimiter)
	if !closed {
		return substitution{}, errors.New("바꿀 글이 없습니다")
	}
	if pattern == "" {
		return substitution{}, errors.New("찾을 것이 없습니다")
	}

	// 닫히지 않았으면 남은 것이 통째로 바꿀 글이다. 그것이 허용하는 유일한 생략이다.
	replacement, flags, _ := cutUnescaped(rest, delimiter)

	sub := substitution{input: pattern, all: strings.ContainsRune(flags, 'g'), confirm: strings.ContainsRune(flags, 'c')}

	// 제 것을 걷어내고 남은 것은 패턴에 얹을 flag 다 — `i` 가 그것이다. 모르는 글자를
	// 걸러내는 문구는 `/` 검색과 같은 자리에서 나온다(search.go).
	compiled, err := compilePattern(pattern, strings.Map(dropSubstituteFlag, flags))
	if err != nil {
		return substitution{}, err
	}
	sub.pattern = compiled

	text, err := parseReplacement(replacement)
	if err != nil {
		return substitution{}, err
	}
	sub.replacement = text

	return sub, nil
}

// dropSubstituteFlag 는 `:s` 가 제 것으로 먹는 flag 를 지운다. 남은 것이 패턴의 flag 다.
func dropSubstituteFlag(flag rune) rune {
	switch flag {
	case 'g', 'c':
		return -1
	default:
		return flag
	}
}

// isSubstituteDelimiter 는 `:s` 의 구분자로 쓸 수 있는 글자다.
//
// 영문자·숫자는 뺀다 — 이름을 이어갈 수 있는 글자라 그것이 오면 이름이 아직 안 끝난 것이다
// (`:substitute` 의 가운데 글자들이 그 자리다). 공백도 뺀다: `:s /a/b/` 처럼 띄어 쓴 것을
// 받아야 하고, 띄어쓰기가 구분자가 되면 그 길이 막힌다. `\` 는 막는 글자, `"` 는 명령줄의
// 따옴표라 둘 다 뺀다. vim 도 이 넷을 뺀다.
//
// ASCII 만이다. 한글이 구분자가 되는 자리를 만들 값이 없다.
func isSubstituteDelimiter(ch rune) bool {
	if ch > unicode.MaxASCII {
		return false
	}

	switch ch {
	case '\\', '"', ' ', '\t':
		return false
	}

	return !unicode.IsLetter(ch) && !unicode.IsDigit(ch)
}

// cutUnescaped 는 `\` 로 막히지 않은 첫 구분자에서 자른다.
//
// 막혔는지는 앞에서부터 세어야 안다 — `\\/` 의 `/` 는 막히지 않은 것이다. 검색의
// splitSearchFlags 와 같은 손이고, 구분자는 ASCII 한 글자라 자를 자리도 한 byte 뒤다.
func cutUnescaped(text string, delimiter rune) (before, after string, found bool) {
	escaped := false

	for i, ch := range text {
		switch {
		case escaped:
			escaped = false
		case ch == '\\':
			escaped = true
		case ch == delimiter:
			return text[:i], text[i+1:], true
		}
	}

	return text, "", false
}

// parseReplacement 는 vim 식 바꿀 글을 Go 정규식이 읽는 문법으로 옮긴다.
//
//	&        잡은 것 전체      → ${0}
//	\0 … \9  잡은 것           → ${0} … ${9}
//	\&       `&` 한 글자
//	\t       tab. 명령줄에서 tab 키로는 낼 수 없어서 이 길이 유일하다
//	\x       그 밖의 글자는 그대로다. 구분자를 넣는 `\/` 도 이 길로 온다
//	$        사람이 친 `$` 는 글자다. Go 쪽 문법이라 `$$` 로 막아 둔다
//
// byte 로 훑어도 된다. 뜻을 갖는 글자가 다 ASCII 이고 UTF-8 의 이어지는 byte 는 ASCII 와
// 겹치지 않아서, 한글은 default 로 그대로 실려 나간다.
func parseReplacement(text string) ([]byte, error) {
	out := make([]byte, 0, len(text))

	for i := 0; i < len(text); i++ {
		switch ch := text[i]; ch {
		case '$':
			out = append(out, '$', '$')
		case '&':
			out = append(out, "${0}"...)
		case '\\':
			if i+1 >= len(text) {
				// 명령줄 tokenizer 가 내는 것과 같은 문구다. 같은 실수라 같은 말이어야 한다.
				return nil, errors.New("`\\` 뒤에 글자가 없습니다")
			}
			i++

			switch next := text[i]; {
			case next >= '0' && next <= '9':
				out = append(out, "${"...)
				out = append(out, next, '}')
			case next == 't':
				out = append(out, '\t')
			case next == '$':
				out = append(out, '$', '$')
			default:
				out = append(out, next)
			}
		default:
			out = append(out, ch)
		}
	}

	return out, nil
}

// matchesIn 은 그 줄의 [from, to) 안에서 바꿀 자리들이다. `g` 가 없으면 첫 자리 하나다.
//
// 줄 전체에서 한 번에 찾는다. 앞에서부터 자르며 이어 찾으면 `^` 가 자른 자리마다 다시
// 맞아서, `:s/^/> /g` 가 줄 앞이 아니라 글자마다 붙는다.
//
// **구간이 좁아도 찾는 것은 줄 전체다.** 구간을 잘라서 돌리면 앵커가 자른 자리에 붙는다 —
// 위와 같은 실수다. 찾은 뒤에 구간 밖을 걸러 내므로 `^` 와 `$` 는 줄의 끝에 맞는다(ADR-0089).
//
// **거르는 것이 `g` 보다 먼저다.** 먼저 하나로 줄이면 구간 밖의 첫 자리가 남아서
// `g` 없는 `:s` 가 아무 일도 하지 않는다.
//
// 구간에 걸친 자리는 버린다. 반만 고른 낱말을 바꾸면 고른 밖이 딸려 나간다.
func (s substitution) matchesIn(line []byte, from, to int) [][]int {
	all := s.pattern.FindAllSubmatchIndex(line, -1)

	found := make([][]int, 0, len(all))
	for _, match := range all {
		if match[0] >= from && match[1] <= to {
			found = append(found, match)
		}
	}

	if !s.all && len(found) > 1 {
		return found[:1]
	}

	return found
}

// expand 는 잡은 것을 바꿀 글에 넣어 그 자리에 들어갈 글자를 만든다.
func (s substitution) expand(line []byte, match []int) []byte {
	return s.pattern.Expand(nil, s.replacement, line, match)
}

// applyRange 는 한 줄의 [from, to) 안에서 바꿀 자리를 다 바꾼다. 바꾼 줄과 바꾼 자리 수를 준다.
//
// 구간 밖은 그대로 실려 나간다. 줄 단위 범위는 부르는 쪽이 `[0, 줄끝]` 을 주므로 이 함수
// 하나가 두 갈래를 다 답한다(buffer-selection.go 의 selectionOn, ADR-0089).
//
// 바꿀 것이 없으면 들어온 줄을 그대로 돌려준다 — 새 slice 를 만들지 않아서, 안 바뀐 줄이
// 되돌리기 기록에 옛 줄 그대로 남는다.
func (s substitution) applyRange(line []byte, from, to int) ([]byte, int) {
	matches := s.matchesIn(line, from, to)
	if len(matches) == 0 {
		return line, 0
	}

	next := make([]byte, 0, len(line))
	at := 0

	for _, match := range matches {
		next = append(next, line[at:match[0]]...)
		next = s.pattern.Expand(next, s.replacement, line, match)
		at = match[1]
	}

	return append(next, line[at:]...), len(matches)
}

// substituteMessage 는 몇 곳을 몇 줄에서 바꿨는지다. vim 의 `3 substitutions on 2 lines` 다.
//
// 하나여도 그대로 적는다. 복사 알림이 한 줄짜리도 알리는 것과 같은 손이다(ADR-0017).
func substituteMessage(changes, lines int) string {
	return fmt.Sprintf("%d 곳을 %d 줄에서 바꿨습니다", changes, lines)
}

// substituteIn 은 area 의 줄들을 바꾼 뒤 창에 얹는다. 바꾼 자리 수·줄 수와 마지막으로 바꾼
// 줄을 준다.
//
// **줄마다 볼 구간을 SelectionOn 이 잘라 준다.** 줄 단위 범위에는 `[0, 줄끝]` 을 주고 글자로
// 고른 범위에는 그 글자 구간을 주므로, 길이 갈리지 않는다. 그래서 `V` 로 고른 것은 줄 전체가
// 바뀌고 `v` 로 고른 것은 고른 글자만 바뀐다(ADR-0089).
//
// **하나도 안 바뀌었으면 창을 건드리지 않는다.** 건드리면 dirty 가 서고 되돌아갈 앞날(redo)
// 이 날아간다 — 못 찾은 `:s` 가 파일을 건드린 것이 된다(ADR-0083).
//
// **무엇으로 바꿀지는 여기가 안다.** 창은 갈아끼우기만 한다(Viewport.ReplaceRun, ADR-0128).
func substituteIn(buf *textarea.Viewport, sub substitution, area scheme.MotionRange) (changes, lines, last int) {
	from, to := area.Start.Line, area.End.Line

	next := make([][]byte, 0, to-from+1)

	for at := from; at <= to; at++ {
		span, _, ok := buf.SelectionOn(area, at)
		if !ok {
			next = append(next, buf.Lines[at])

			continue
		}

		line, found := sub.applyRange(buf.Lines[at], span[0], span[1])
		next = append(next, line)

		if found > 0 {
			changes, lines, last = changes+found, lines+1, at
		}
	}

	if changes == 0 {
		return 0, 0, 0
	}

	buf.ReplaceRun(from, next)

	return changes, lines, last
}
