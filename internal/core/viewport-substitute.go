package core

// `:s` 가 파일을 건드리는 자리다. 무엇을 바꿀지는 substitution 이 정하고(substitute.go)
// 여기는 그것을 buffer 에 얹는다 (ADR-0084).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-substitute.go 에 있다 (ADR-0121).

// substitute 는 area 를 한 번에 바꾼다. 바꾼 자리 수·줄 수와 마지막으로 바꾼 줄을 준다.
//
// **줄마다 볼 구간을 selectionOn 이 잘라 준다.** 줄 단위 범위에는 `[0, 줄끝]` 을 주고 글자로
// 고른 범위에는 그 글자 구간을 주므로, 길이 갈리지 않는다. 그래서 `V` 로 고른 것은 줄 전체가
// 바뀌고 `v` 로 고른 것은 고른 글자만 바뀐다(ADR-0089).
//
// **되돌리기는 한 구간이다.** 손이 한 번 친 것은 한 번의 `u` 로 돌아가야 한다. `:1,5d` 가
// 지운 줄들을 한 묶음으로 되돌리는 것과 같은 자리다(ADR-0046).
//
// **하나도 안 바뀌었으면 구간을 열지 않는다.** 열면 dirty 가 서고 되돌아갈 앞날(redo) 이
// 날아간다 — 못 찾은 `:s` 가 파일을 건드린 것이 된다. `~` 가 한글 위에서 파일을 그대로
// 두는 것과 같은 까닭이다(ADR-0083).
func (buf *viewport) substitute(sub substitution, area MotionRange) (changes, lines, last int) {
	from, to := area.start.line, area.end.line

	next := make([][]byte, 0, to-from+1)

	for at := from; at <= to; at++ {
		span, _, ok := buf.selectionOn(area, at)
		if !ok {
			next = append(next, buf.lines[at])

			continue
		}

		line, found := sub.applyRange(buf.lines[at], span[0], span[1])
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
