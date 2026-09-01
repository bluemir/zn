package core

// 지우는 것들이다. `d`·`x`·`dd` 다. 지운 것은 register 로 나간다 (ADR-0017).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-delete.go 다 (ADR-0121).

// textBetween 은 (startLine, startCol) 부터 (endLine, endCol) 앞까지를 줄로 끊은 것이다.
// 지우기와 복사가 같은 모양으로 register 를 채운다.
//
// 줄은 제자리에서 바뀌지 않으므로(ADR-0001) 잘라낸 조각을 그대로 들고 있어도 된다.
func (buf Buffer) textBetween(startLine, startCol, endLine, endCol int) [][]byte {
	count := endLine - startLine + 1

	text := make([][]byte, 0, count)
	if count == 1 {
		return append(text, buf.lines[startLine][startCol:endCol])
	}

	text = append(text, buf.lines[startLine][startCol:])
	text = append(text, buf.lines[startLine+1:endLine]...)

	return append(text, buf.lines[endLine][:endCol])
}
