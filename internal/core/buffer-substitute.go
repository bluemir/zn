package core

// `:s` 가 파일을 건드리는 자리다. 무엇을 바꿀지는 substitution 이 정하고(substitute.go)
// 여기는 그것을 buffer 에 얹는다 (ADR-0084).

// substitute 는 [from, to] 를 한 번에 바꾼다. 바꾼 자리 수·줄 수와 마지막으로 바꾼 줄을 준다.
//
// **되돌리기는 한 구간이다.** 손이 한 번 친 것은 한 번의 `u` 로 돌아가야 한다. `:1,5d` 가
// 지운 줄들을 한 묶음으로 되돌리는 것과 같은 자리다(ADR-0046).
//
// **하나도 안 바뀌었으면 구간을 열지 않는다.** 열면 dirty 가 서고 되돌아갈 앞날(redo) 이
// 날아간다 — 못 찾은 `:s` 가 파일을 건드린 것이 된다. `~` 가 한글 위에서 파일을 그대로
// 두는 것과 같은 까닭이다(ADR-0083).
func (buf *Buffer) substitute(sub substitution, from, to int) (changes, lines, last int) {
	next := make([][]byte, 0, to-from+1)

	for at := from; at <= to; at++ {
		line, found := sub.applyLine(buf.lines[at])
		next = append(next, line)

		if found > 0 {
			changes, lines, last = changes+found, lines+1, at
		}
	}

	if changes == 0 {
		return 0, 0, 0
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 치환은 언제나 제 구간이다.
	buf.endEdit()
	buf.beginEdit(from, len(next))
	buf.replaceLines(from, len(next), next)
	buf.endEdit()

	return changes, lines, last
}

// spliceLine 은 line 줄의 [from, to) 를 with 로 갈아끼운다. `:s///c` 가 한 자리씩 바꾸는 문이다.
//
// **되돌리기 구간을 열지 않는다.** 물어보며 바꾼 것 전부가 한 구간에 담겨야 해서, 여는 것도
// 닫는 것도 한 판을 아는 쪽이 한다(view-editor-substitute.go).
func (buf *Buffer) spliceLine(line, from, to int, with []byte) {
	old := buf.lines[line]

	next := make([]byte, 0, len(old)-(to-from)+len(with))
	next = append(next, old[:from]...)
	next = append(next, with...)
	next = append(next, old[to:]...)

	buf.replaceLines(line, 1, [][]byte{next})
}
