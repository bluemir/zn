package core

import (
	"regexp"
)

// buffer 안에서 패턴을 찾는 것들이다. 패턴을 짓고 화면에 칠하는 것은 search.go 에 있다 (ADR-0042).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-search.go 다 (ADR-0121).

// isWordClass 는 `*` 가 잡는 부류인지다. 공백과 문장부호 위에서는 단어를 찾지 않는다.
func isWordClass(class charClass) bool {
	return class == classWord || class == classCJK
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
