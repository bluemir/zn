package core

import (
	"unicode/utf8"
)

// 글자를 덮어쓰는 것들이다. vim 의 `r` 다. 지우고 넣는 것이 아니라 그 자리를 갈아끼운다.

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
	// 결합 문자가 붙은 글자와 이모지는 여러 rune 이라 lines.Size 로 센다.
	text := []byte(key)
	if len(text) == 0 || glyphSize(text, 0) != len(text) {
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
func (buf *viewport) replaceChar(text []byte, count, width int) bool {
	n := max(count, 1)

	line := buf.lines[buf.cursor.Line]
	end, ok := clusterEnd(line, buf.cursor.Col, n)
	if !ok {
		return false
	}

	next := make([]byte, 0, len(line)-(end-buf.cursor.Col)+len(text)*n)
	next = append(next, line[:buf.cursor.Col]...)
	for range n {
		next = append(next, text...)
	}
	next = append(next, line[end:]...)

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 바꿔 넣기는 언제나 제 구간이다.
	buf.endEdit()
	buf.beginEdit(buf.cursor.Line, 1)
	buf.replaceLines(buf.cursor.Line, 1, [][]byte{next})
	buf.endEdit()

	// 커서는 마지막으로 바꾼 글자 위다. vim 과 같다.
	buf.cursor.Col += (n - 1) * len(text)
	buf.updateDesiredCol(width)

	return true
}

// replaceWithNewline 은 커서부터 count 글자를 지우고 그 자리에서 줄을 가른다. vim 의 `r<Enter>` 다.
//
// 새 줄은 이 파일의 규칙이 정한 들여쓰기를 받는다. `o` 와 같다(indent.go).
func (buf *viewport) replaceWithNewline(count, width int) bool {
	line := buf.lines[buf.cursor.Line]

	end, ok := clusterEnd(line, buf.cursor.Col, max(count, 1))
	if !ok {
		return false
	}

	// 자르기 전에 정한다. 자른 뒤의 앞 줄은 커서 앞까지라 줄 끝의 여는 괄호가 사라질 수 있다.
	indent := buf.indentForNewLine(buf.cursor.Line, line[:buf.cursor.Col])
	below := concat(indent, line[end:])

	buf.endEdit()
	buf.beginEdit(buf.cursor.Line, 1)
	buf.replaceLines(buf.cursor.Line, 1, [][]byte{line[:buf.cursor.Col], below})
	buf.growEdit(1)
	buf.endEdit()

	buf.cursor.Line++
	buf.cursor.Col = len(indent)
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

		col += glyphSize(line, col)
	}

	return col, true
}
