package core

import (
	"unicode/utf8"
)

// replacementText 는 `r` 뒤에 붙은 키가 파일에 넣을 글자면 그것을 준다.
//
// 방향키나 `ctrl` 조합처럼 글자가 아닌 키는 false 다. 그 키로는 아무 일도 하지 않고 무른다 —
// `esc` 로 무르는 것과 같은 길이라 잘못 누른 `r` 을 되돌릴 수 있다. vim 과 같다.
//
// 키 이름은 글자면 글자 그대로 오지만(`a` `한` `?`) 빈 칸과 tab 은 이름으로 온다.
func replacementText(key string) ([]byte, bool) {
	switch key {
	case "space":
		return []byte(" "), true
	case "tab":
		return []byte("\t"), true
	}

	// 글자 하나여야 한다. `up` `esc` `ctrl+a` 같은 이름은 여기서 걸린다.
	// 결합 문자가 붙은 글자와 이모지는 여러 rune 이라 clusterSize 로 센다.
	text := []byte(key)
	if len(text) == 0 || clusterSize(text, 0) != len(text) {
		return nil, false
	}

	if char, _ := utf8.DecodeRune(text); char == utf8.RuneError {
		return nil, false
	}

	return text, true
}

// replaceChar 는 커서부터 count 글자를 text 로 하나씩 바꾼다. vim 의 `r` 이다.
//
// 줄에 남은 글자가 count 보다 적으면 아무것도 바꾸지 않는다. vim 과 같다 — `3r` 은 세 글자를
// 바꾸겠다는 뜻이라, 두 글자만 바꿔주면 친 것과 다른 일이 일어난다.
func (buf *Buffer) replaceChar(text []byte, count, width int) bool {
	n := max(count, 1)

	line := buf.lines[buf.cursorLine]
	end, ok := clusterEnd(line, buf.cursorCol, n)
	if !ok {
		return false
	}

	next := make([]byte, 0, len(line)-(end-buf.cursorCol)+len(text)*n)
	next = append(next, line[:buf.cursorCol]...)
	for range n {
		next = append(next, text...)
	}
	next = append(next, line[end:]...)

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 바꿔 넣기는 언제나 제 구간이다.
	buf.endEdit()
	buf.beginEdit(buf.cursorLine, 1)
	buf.replaceLines(buf.cursorLine, 1, [][]byte{next})
	buf.endEdit()

	// 커서는 마지막으로 바꾼 글자 위다. vim 과 같다.
	buf.cursorCol += (n - 1) * len(text)
	buf.updateDesiredCol(width)

	return true
}

// replaceWithNewline 은 커서부터 count 글자를 지우고 그 자리에서 줄을 가른다. vim 의 `r<Enter>` 다.
//
// 커서는 새로 생긴 아래 줄의 첫 칸이다. 들여쓰기는 이어받지 않는다. `o` 와 같다.
func (buf *Buffer) replaceWithNewline(count, width int) bool {
	line := buf.lines[buf.cursorLine]

	end, ok := clusterEnd(line, buf.cursorCol, max(count, 1))
	if !ok {
		return false
	}

	buf.endEdit()
	buf.beginEdit(buf.cursorLine, 1)
	buf.replaceLines(buf.cursorLine, 1, [][]byte{line[:buf.cursorCol], line[end:]})
	buf.growEdit(1)
	buf.endEdit()

	buf.cursorLine++
	buf.cursorCol = 0
	buf.updateDesiredCol(width)

	return true
}

// clusterEnd 는 col 에서 글자 n 개 뒤의 offset 이다. 줄에 그만큼 남아 있지 않으면 false 다.
//
// byte 가 아니라 글자로 센다. 한글 한 글자는 3 byte 이고 이모지는 더 길다.
func clusterEnd(line []byte, col, n int) (int, bool) {
	for range n {
		if col >= len(line) {
			return 0, false
		}

		col += clusterSize(line, col)
	}

	return col, true
}
