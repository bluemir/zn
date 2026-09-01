package core

import (
	"regexp"
)

// buffer 안에서 패턴을 찾는 것들이다. 패턴을 짓고 화면에 칠하는 것은 search.go 에 있다 (ADR-0042).

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
	for col < len(line) && !isWordClass(glyphClass(line, col)) {
		col += glyphSize(line, col)
	}
	if col >= len(line) {
		return "", 0, false
	}

	// 단어는 한 부류가 이어지는 구간이다. `한글abc` 가 두 단어인 것도 여기서 따라온다.
	class := glyphClass(line, col)

	start := col
	for start > 0 {
		prev := prevGlyphStart(line, 0, start)
		if glyphClass(line, prev) != class {
			break
		}

		start = prev
	}

	end := col
	for end < len(line) && glyphClass(line, end) == class {
		end += glyphSize(line, end)
	}

	return string(line[start:end]), start, true
}

// isWordClass 는 `*` 가 잡는 부류인지다. 공백과 문장부호 위에서는 단어를 찾지 않는다.
func isWordClass(class charClass) bool {
	return class == classWord || class == classCJK
}
