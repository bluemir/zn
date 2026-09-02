package textarea

// `:s` 가 파일을 건드리는 자리다. 무엇을 바꿀지는 substitution 이 정하고(substitute.go)
// 여기는 그것을 buffer 에 얹는다 (ADR-0084).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-substitute.go 다 (ADR-0121).

// spliceLine 은 line 줄의 [from, to) 를 with 로 갈아끼운다. `:s///c` 가 한 자리씩 바꾸는 문이다.
//
// **되돌리기 구간을 열지 않는다.** 물어보며 바꾼 것 전부가 한 구간에 담겨야 해서, 여는 것도
// 닫는 것도 한 판을 아는 쪽이 한다(view-editor-substitute.go).
func (buf *Buffer) SpliceLine(Line, from, to int, with []byte) {
	old := buf.Lines[Line]

	next := make([]byte, 0, len(old)-(to-from)+len(with))
	next = append(next, old[:from]...)
	next = append(next, with...)
	next = append(next, old[to:]...)

	buf.ReplaceLines(Line, 1, [][]byte{next})
}
