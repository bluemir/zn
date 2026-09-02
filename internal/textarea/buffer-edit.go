package textarea

import (
	"bytes"
)

// 줄을 갈아끼우는 자리다. **replaceLines 가 `buf.lines` 를 바꾸는 유일한 함수**이고,
// 되돌리기 구간(beginEdit…endEdit) 과 문법 캐시가 거기 하나에 걸린다 (ADR-0001, ADR-0033).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-edit.go 다 (ADR-0121).

// squeezeInnerSpaces 는 들여쓰기와 줄 끝을 그대로 두고 가운데의 이어진 공백·tab 을
// 빈 칸 하나로 줄인 줄이다. 줄일 것이 없으면 받은 줄을 그대로 준다(ADR-0001).
func squeezeInnerSpaces(line []byte) []byte {
	indent := len(line) - len(bytes.TrimLeft(line, " \t"))
	body := len(trimLineEnd(line))

	if indent >= body {
		// 빈 줄이거나 공백뿐인 줄이다. 줄일 가운데가 없다.
		return line
	}

	squeezed := make([]byte, 0, len(line))
	squeezed = append(squeezed, line[:indent]...)

	space := false
	for _, c := range line[indent:body] {
		if c == ' ' || c == '\t' {
			space = true

			continue
		}

		if space {
			squeezed = append(squeezed, ' ')
			space = false
		}

		squeezed = append(squeezed, c)
	}

	squeezed = append(squeezed, line[body:]...)

	if len(squeezed) == len(line) {
		return line
	}

	return squeezed
}

// trimLineEnd 는 줄 끝의 공백과 tab 을 뗀 부분이다.
// 자르기만 하므로 새로 할당하지 않는다(ADR-0001).
func trimLineEnd(line []byte) []byte {
	End := len(line)
	for End > 0 && (line[End-1] == ' ' || line[End-1] == '\t') {
		End--
	}

	return line[:End]
}

// replaceLines 는 lines 의 [at, at+count) 를 with 로 갈아끼우고 원래 있던 줄들을 돌려준다.
//
// 반드시 새 slice 를 만들어 담는다. buf.lines 를 제자리에서 append 하면 옛 줄을 가리키던
// undo 기록이 덮여서 되돌릴 수 없게 된다.
func (buf *Buffer) ReplaceLines(at, count int, with [][]byte) [][]byte {
	old := buf.Lines[at : at+count]

	next := make([][]byte, 0, len(buf.Lines)-count+len(with))
	next = append(next, buf.Lines[:at]...)
	next = append(next, with...)
	next = append(next, buf.Lines[at+count:]...)
	buf.Lines = next

	// 문법 캐시도 같은 자리를 같은 수로 갈아끼운다. 줄과 index 가 나란해야 아래쪽에 담아둔
	// 것을 그대로 쓸 수 있다(syntax.go).
	//
	// 이 함수가 lines 를 갈아끼우는 유일한 자리라 여기 한 줄이면 된다. 편집 경로가 늘어도
	// 제자리 수정은 undo 를 깨므로(위 주석) 이곳을 지나지 않을 수 없다.
	buf.syntax.replace(at, count, len(with))

	return old
}

// growEdit 는 편집으로 줄 수가 바뀐 만큼 열린 구간의 범위를 넓히거나 좁힌다.
func (buf *Buffer) growEdit(delta int) {
	if !buf.editing {
		return
	}
	buf.undo[len(buf.undo)-1].count += delta
}

// endEdit 는 열린 구간을 닫는다. 다음 편집은 새 항목이 된다.
// Esc 와 insert mode 의 커서 이동이 구간을 끊는다. vim 과 같다.
func (buf *Buffer) EndEdit() {
	buf.editing = false
}

// resumeEdit 는 방금 닫은 구간을 다시 연다. 이어지는 타이핑이 그 구간에 들어가서
// 지운 것과 새로 친 글자가 한 번의 `u` 로 함께 돌아간다.
//
// `c` 가 쓴다 — 지우기와 insert 가 한 동작이라 되돌리기도 하나여야 한다. vim 과 같다.
// 되돌릴 것이 하나도 없으면(지운 것이 없었으면) 열 구간도 없다.
func (buf *Buffer) resumeEdit() {
	buf.editing = len(buf.undo) > 0
}
