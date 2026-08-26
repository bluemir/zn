package core

import (
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
)

// searchDirection 은 검색이 훑는 방향이다. `/` 가 아래로, `?` 가 위로다.
type searchDirection int

const (
	searchForward searchDirection = iota
	searchBackward
)

// reverse 는 반대 방향이다. `N` 이 마지막 검색을 거꾸로 되풀이할 때 쓴다.
func (d searchDirection) reverse() searchDirection {
	if d == searchBackward {
		return searchForward
	}

	return searchBackward
}

// searchState 는 마지막 검색이다.
//
// mode 와 tab 을 넘어 남는다. `n` 은 검색을 끝내고 한참 뒤에 눌리고, 다른 tab 으로 옮겨서도
// 같은 것을 찾는 것이 vim 의 동작이다. 그래서 Buffer 가 아니라 editor 가 들고 있다.
type searchState struct {
	// input 은 친 그대로다. 못 찾았다고 알릴 때 이것을 보여준다.
	input string

	pattern   *regexp.Regexp
	direction searchDirection

	// highlight 는 매칭을 화면에 칠할지다. 검색할 때마다 켜지고 `:noh` 로 끈다. vim 의 hlsearch 다.
	highlight bool
}

// parseSearchPattern 은 명령줄에 친 것을 정규식으로 만든다.
//
// 패턴은 Go 의 정규식(RE2) 이다. 끝이 `/글자들` 이면 그 글자들은 flag 다 — `/abcd/i` 는
// 대소문자를 무시하고 `abcd` 를 찾는다. 마지막 `/` 뒤가 비어 있거나 영문자가 아닌 것이
// 섞여 있으면 flag 가 아니라 패턴의 일부다. `http://` 를 그대로 칠 수 있어야 하기 때문이다.
//
// 패턴 안에 flag 로 읽힐 `/` 를 넣어야 하면 `\/` 로 막는다. 그 `\/` 는 정규식에서도
// `/` 한 글자를 뜻하므로 뗄 필요 없이 그대로 넘긴다.
func parseSearchPattern(input string) (*regexp.Regexp, error) {
	pattern, flags := splitSearchFlags(input)

	// flag 는 정규식 앞에 붙이는 Go 의 문법으로 옮긴다.
	prefix := ""
	for _, flag := range flags {
		switch flag {
		case 'i':
			prefix += "(?i)"
		default:
			return nil, errors.Newf("모르는 flag: %c", flag)
		}
	}

	compiled, err := regexp.Compile(prefix + pattern)
	if err != nil {
		// 감싸지(Wrap) 않고 문구를 짓는다. 바로 위의 `모르는 flag` 와 같은 손이다.
		//
		// 감싸면 `errors.Cause` 가 앞말을 벗겨 낸다 — 알림을 세우는 자리가 그것을 부르기
		// 때문이다(notice.go 의 notifyError). 감싸기는 「어느 파일을 쓰다 났는가」처럼
		// **사람에게 보일 것이 아닌** 맥락을 붙일 때 쓰고, 사람에게 보일 문구는 이렇게 짓는다
		// (ADR-0053).
		return nil, errors.Newf("잘못된 패턴: %v", err)
	}

	return compiled, nil
}

// splitSearchFlags 는 뒤에 붙은 flag 를 뗀다.
//
// `\` 로 막힌 `/` 는 구분자가 아니다. 막혔는지는 앞에서부터 세어야 알 수 있어서
// (`\\/` 는 막히지 않은 것이다) 뒤에서 찾지 않고 끝까지 훑는다.
func splitSearchFlags(input string) (pattern, flags string) {
	last := -1
	escaped := false

	for i, ch := range input {
		switch {
		case escaped:
			escaped = false
		case ch == '\\':
			escaped = true
		case ch == '/':
			last = i
		}
	}

	// 구분자가 없거나 뒤가 비어 있으면 전부 패턴이다.
	if last < 0 || last == len(input)-1 {
		return input, ""
	}

	rest := input[last+1:]
	for _, ch := range rest {
		if ch > unicode.MaxASCII || !unicode.IsLetter(ch) {
			return input, ""
		}
	}

	return input[:last], rest
}

// searchMatches 는 그 줄에서 강조할 자리들이다. 강조가 꺼져 있으면 비어 있다.
func (e editor) searchMatches(line []byte) [][]int {
	if !e.search.highlight || e.search.pattern == nil {
		return nil
	}

	return e.search.pattern.FindAllIndex(line, -1)
}

// searchResult 는 찾은 자리다.
type searchResult struct {
	line, col int

	// wrapped 는 파일 끝이나 처음을 지나 감쌌는지다. vim 처럼 아래 줄에 알린다 —
	// 알리지 않으면 커서가 왜 뒤로 갔는지 알 수 없다.
	wrapped bool
}

// wordSearchPattern 은 `*` `#` 가 쓰는 패턴이다. 단어 전체와 맞는 자리만 찾는다.
//
// `\b` 는 ASCII 만 아는 경계라 한글 단어의 양끝에는 붙이지 않는다. 붙이면 앞뒤가 ASCII
// 단어 글자여야 한다는 뜻이 되어 아무것도 찾지 못한다.
func wordSearchPattern(word string) string {
	pattern := regexp.QuoteMeta(word)

	if first, _ := utf8.DecodeRuneInString(word); isASCIIWord(first) {
		pattern = `\b` + pattern
	}
	if last, _ := utf8.DecodeLastRuneInString(word); isASCIIWord(last) {
		pattern += `\b`
	}

	return pattern
}

// isASCIIWord 는 Go 의 `\b` 가 단어 글자로 세는 글자인지다.
func isASCIIWord(r rune) bool {
	return r < utf8.RuneSelf && (r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r))
}
