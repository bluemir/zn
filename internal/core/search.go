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
		return nil, errors.Wrap(err, "잘못된 패턴")
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

// find 는 (fromLine, fromCol) 에서 방향대로 가장 가까운 매칭을 찾는다.
// 파일 끝에 닿으면 반대쪽 끝에서 이어 보고, 한 바퀴를 다 돌면 ok 가 false 다.
func (buf Buffer) find(pattern *regexp.Regexp, direction searchDirection, fromLine, fromCol int) (searchResult, bool) {
	if direction == searchBackward {
		return buf.findBackward(pattern, fromLine, fromCol)
	}

	return buf.findForward(pattern, fromLine, fromCol)
}

// findForward 는 커서 다음 자리부터 아래로 찾는다.
//
// 줄을 한 바퀴 돌되 시작 줄은 두 번 본다. 처음에는 커서 뒤쪽만, 감싼 뒤에는 커서 앞쪽만이다.
// 그래야 시작 줄 안에서 커서보다 앞에 있는 매칭도 빠지지 않는다.
func (buf Buffer) findForward(pattern *regexp.Regexp, fromLine, fromCol int) (searchResult, bool) {
	for i := 0; i <= len(buf.lines); i++ {
		line := (fromLine + i) % len(buf.lines)

		for _, match := range pattern.FindAllIndex(buf.lines[line], -1) {
			// 커서 자리의 매칭에 다시 걸리면 `n` 이 제자리걸음을 한다.
			if i == 0 && match[0] <= fromCol {
				continue
			}
			if i == len(buf.lines) && match[0] > fromCol {
				continue
			}

			return searchResult{line: line, col: match[0], wrapped: fromLine+i >= len(buf.lines)}, true
		}
	}

	return searchResult{}, false
}

// findBackward 는 커서 앞자리부터 위로 찾는다. 한 줄 안에서는 뒤에 있는 매칭이 먼저다.
func (buf Buffer) findBackward(pattern *regexp.Regexp, fromLine, fromCol int) (searchResult, bool) {
	for i := 0; i <= len(buf.lines); i++ {
		// i 는 줄 수까지만 커지므로 한 번 더해주면 음수가 나오지 않는다.
		line := (fromLine - i + len(buf.lines)) % len(buf.lines)

		matches := pattern.FindAllIndex(buf.lines[line], -1)
		for j := len(matches) - 1; j >= 0; j-- {
			match := matches[j]

			if i == 0 && match[0] >= fromCol {
				continue
			}
			if i == len(buf.lines) && match[0] < fromCol {
				continue
			}

			return searchResult{line: line, col: match[0], wrapped: fromLine-i < 0}, true
		}
	}

	return searchResult{}, false
}

// moveTo 는 커서를 그 자리로 옮긴다. 검색이 찾은 자리로 뛸 때 쓴다.
func (buf *Buffer) moveTo(line, col, width int) {
	buf.cursorLine = min(max(line, 0), len(buf.lines)-1)
	buf.cursorCol = min(max(col, 0), len(buf.lines[buf.cursorLine]))
	buf.updateDesiredCol(width)
}

// wordUnderCursor 는 `*` `#` 가 찾을 단어와 그 단어가 시작하는 자리다.
//
// 커서가 단어 위가 아니면 그 줄에서 오른쪽으로 첫 단어를 찾는다. 줄에 단어가 없으면 false 다.
// vim 과 같다 — 들여쓰기 위에서 눌러도 그 줄의 첫 낱말을 찾아준다.
func (buf Buffer) wordUnderCursor() (string, int, bool) {
	line := buf.lines[buf.cursorLine]

	col := buf.cursorCol
	for col < len(line) && !isWordClass(clusterClass(line, col, smallWord)) {
		col += clusterSize(line, col)
	}
	if col >= len(line) {
		return "", 0, false
	}

	// 단어는 한 부류가 이어지는 구간이다. `한글abc` 가 두 단어인 것도 여기서 따라온다.
	class := clusterClass(line, col, smallWord)

	start := col
	for start > 0 {
		prev := prevClusterStart(line, 0, start)
		if clusterClass(line, prev, smallWord) != class {
			break
		}

		start = prev
	}

	end := col
	for end < len(line) && clusterClass(line, end, smallWord) == class {
		end += clusterSize(line, end)
	}

	return string(line[start:end]), start, true
}

// isWordClass 는 `*` 가 잡는 부류인지다. 공백과 문장부호 위에서는 단어를 찾지 않는다.
func isWordClass(class charClass) bool {
	return class == classWord || class == classCJK
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
