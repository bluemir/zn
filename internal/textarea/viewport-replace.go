package textarea

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
func ReplacementText(key string) ([]byte, bool) {
	switch key {
	case "space":
		return []byte(" "), true
	case "tab":
		return []byte("\t"), true
	}

	// 글자 하나여야 한다. `up` `esc` `ctrl+a` 같은 이름은 여기서 걸린다.
	// 결합 문자가 붙은 글자와 이모지는 여러 rune 이라 lines.Size 로 센다.
	text := []byte(key)
	if len(text) == 0 || GlyphSize(text, 0) != len(text) {
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
func (viewport *Viewport) ReplaceChar(text []byte, count int) bool {
	n := max(count, 1)

	Line := viewport.Lines[viewport.Cursor.Line]
	End, ok := clusterEnd(Line, viewport.Cursor.Col, n)
	if !ok {
		return false
	}

	next := make([]byte, 0, len(Line)-(End-viewport.Cursor.Col)+len(text)*n)
	next = append(next, Line[:viewport.Cursor.Col]...)
	for range n {
		next = append(next, text...)
	}
	next = append(next, Line[End:]...)

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 바꿔 넣기는 언제나 제 구간이다.
	viewport.EndEdit()
	viewport.BeginEdit(viewport.Cursor.Line, 1)
	viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{next})
	viewport.EndEdit()

	// 커서는 마지막으로 바꾼 글자 위다. vim 과 같다.
	viewport.Cursor.Col += (n - 1) * len(text)
	viewport.UpdateDesiredCol()

	return true
}

// replaceWithNewline 은 커서부터 count 글자를 지우고 그 자리에서 줄을 가른다. vim 의 `r<Enter>` 다.
//
// 새 줄은 이 파일의 규칙이 정한 들여쓰기를 받는다. `o` 와 같다(indent.go).
func (viewport *Viewport) ReplaceWithNewline(count int) bool {
	Line := viewport.Lines[viewport.Cursor.Line]

	End, ok := clusterEnd(Line, viewport.Cursor.Col, max(count, 1))
	if !ok {
		return false
	}

	// 자르기 전에 정한다. 자른 뒤의 앞 줄은 커서 앞까지라 줄 끝의 여는 괄호가 사라질 수 있다.
	indent := viewport.indentForNewLine(viewport.Cursor.Line, Line[:viewport.Cursor.Col])
	below := concat(indent, Line[End:])

	viewport.EndEdit()
	viewport.BeginEdit(viewport.Cursor.Line, 1)
	viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{Line[:viewport.Cursor.Col], below})
	viewport.growEdit(1)
	viewport.EndEdit()

	viewport.Cursor.Line++
	viewport.Cursor.Col = len(indent)
	viewport.UpdateDesiredCol()

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

		col += GlyphSize(line, col)
	}

	return col, true
}
