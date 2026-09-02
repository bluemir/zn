package core

// 자동완성이 고른 것을 buffer 에 넣는다.

// insertCompletion 은 커서 줄의 [start, end) 를 고른 글로 갈아끼우고 커서를 그 뒤에 둔다.
//
// **자리는 이미 byte 로 옮겨져 있다**(editor 의 applyCompletion). 서버가 주는 범위는 UTF-16
// 열이라 이 줄의 글자를 봐야 byte 로 옮길 수 있는데, 그 옮김을 여기서 하면 창이 언어 서버를
// 알게 된다. 창은 「이 줄의 이 자리를 이 글로 바꾼다」만 받는다 (ADR-0125).
//
// **되돌리기 구간을 닫지 않는다.** insert 에서 친 글자와 한 구간에 있어야 `u` 한 번으로
// 「자동완성으로 넣은 것까지」 돌아간다. 닫는 것은 insert 를 나갈 때다(ADR-0033, ADR-0066).
func (buf *viewport) insertCompletion(start, end int, text []byte) {
	line := buf.cursor.Line

	// 서버가 보던 판과 지금 판이 어긋났으면 범위가 줄 밖을 가리킬 수 있다.
	start = min(max(start, 0), len(buf.lines[line]))
	end = min(max(end, start), len(buf.lines[line]))

	next := make([]byte, 0, len(buf.lines[line])-(end-start)+len(text))
	next = append(next, buf.lines[line][:start]...)
	next = append(next, text...)
	next = append(next, buf.lines[line][end:]...)

	buf.beginEdit(line, 1)
	buf.replaceLines(line, 1, [][]byte{next})

	buf.cursor.Col = start + len(text)
	buf.updateDesiredCol()
}
