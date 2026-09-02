package core

// 바꾸는 것들이다. `c`·`cc` 다. 지우고 insert 로 들어가는 것이라 지우기와 나란한데,
// `cc` 만 줄을 없애지 않고 들여쓰기를 남긴다 — vim 과 같다 (ADR-0017).

// changeRange 는 잡아 둔 범위를 바꾼다. 되돌리기 구간을 열어 둔 채 나오므로 이어 친 글자가
// 같은 `u` 로 함께 돌아간다(ADR-0033).
//
// **범위를 잡는 것은 부르는 쪽이다.** motion 이 잡은 것(`cw`) 도 visual 이 고른 것(`c`) 도
// 여기로 온다 — 둘이 범위를 얻는 길만 다르고 그다음은 같다(action.go, ADR-0037).
//
// 지우고 insert mode 로 들어가는 것이 `d` 와 다른 전부라 범위도 register 도 같이 쓴다.
// 다만 두 자리에서 갈린다(ADR-0013, ADR-0017).
//
//   - 줄 단위(`cc` `cj` `cG`) 는 줄을 없애지 않고 들여쓰기만 남긴 채 비운다
//   - `cw` 는 단어 뒤 공백을 남긴다. vim 의 예외다
//
// 바꿀 것이 없어도(빈 줄에서 친 `cw`) true 다 — vim 처럼 그 자리에서 넣기 시작한다.
func (buf *viewport) changeRange(area MotionRange, width int) (register, bool) {
	if area.linewise {
		removed := buf.changeLines(area.start.line, area.end.line, width)
		buf.resumeEdit()

		return removed, true
	}

	removed, changed := buf.deleteText(area.start, area.end, width)
	if !changed {
		// 바꿀 것이 없었다. 편집이 없었으니 되돌리기 구간도 열지 않는다.
		return register{}, true
	}

	// 지운 자리에서 이어 친다. deleteText 가 normal mode 규칙으로 당겨 둔 커서를 되돌린다 —
	// `c$` 는 줄 끝 다음 칸에서 시작해야 하고 그 자리는 insert mode 에만 있다.
	buf.cursor.line, buf.cursor.col = area.start.line, area.start.col
	buf.updateDesiredCol(width)
	buf.resumeEdit()

	return removed, true
}

// atWordEnd 는 커서가 단어의 마지막 글자 위인지다. 공백 위면 끝낼 단어가 없어서 false 다.
//
// `cw` 가 첫 걸음을 어디서 멈출지 이것으로 가른다. 그 판단은 motion.go 가 한다 — 여기는
// 「지금 자리가 단어 끝인가」만 답한다(ADR-0100).
func (buf viewport) atWordEnd(kind wordKind) bool {
	class := buf.classAt(buf.cursor.line, buf.cursor.col, kind)
	if class == classBlank {
		return false
	}

	line, col, ok := buf.nextPos(buf.cursor.line, buf.cursor.col)

	return !ok || line != buf.cursor.line || buf.classAt(line, col, kind) != class
}

// changeLines 는 [from, to] 줄을 들여쓰기만 남기고 비운다.
//
// **줄을 없애지 않는다.** `dd` 는 줄이 사라지지만 `cc` 는 그 자리에서 다시 치는 것이라
// 빈 줄 하나가 남아야 한다. 여러 줄이면 그것들이 한 줄로 합쳐진다. vim 과 같다.
//
// 남기는 들여쓰기는 첫 줄의 것이다. 들여쓴 코드에서 `cc` 를 칠 때마다 tab 을 다시 치지 않는다.
// autoindent 가 아직 없어서 `o` `O` 는 들여쓰기를 이어받지 않는데, 이쪽은 새 줄을 만드는 것이
// 아니라 있던 줄을 비우는 것이라 원래 들여쓰기가 그 줄의 것이다.
func (buf *viewport) changeLines(from, to, width int) register {
	count := to - from + 1
	indent := leadingBlank(buf.lines[from])

	buf.endEdit()
	buf.beginEdit(from, count)
	removed := buf.replaceLines(from, count, [][]byte{indent})
	buf.growEdit(1 - count)
	buf.endEdit()

	// 들여쓰기 다음 칸에서 이어 친다.
	buf.cursor.line, buf.cursor.col = from, len(indent)
	buf.updateDesiredCol(width)

	return register{lines: removed, linewise: true}
}
