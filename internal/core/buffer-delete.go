package core

// 지우는 것들이다. `d`·`x`·`dd` 다. 지운 것은 register 로 나간다 (ADR-0017).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-delete.go 다 (ADR-0121).

// textBetween 은 (start.line, start.col) 부터 (end.line, end.col) 앞까지를 줄로 끊은 것이다.
// 지우기와 복사가 같은 모양으로 register 를 채운다.
//
// 줄은 제자리에서 바뀌지 않으므로(ADR-0001) 잘라낸 조각을 그대로 들고 있어도 된다.
func (buf Buffer) textBetween(start, end cursor) [][]byte {
	count := end.line - start.line + 1

	text := make([][]byte, 0, count)
	if count == 1 {
		return append(text, buf.lines[start.line][start.col:end.col])
	}

	text = append(text, buf.lines[start.line][start.col:])
	text = append(text, buf.lines[start.line+1:end.line]...)

	return append(text, buf.lines[end.line][:end.col])
}
