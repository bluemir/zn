package core

// 바꾸는 것들이다. `c`·`cc` 다. 지우고 insert 로 들어가는 것이라 지우기와 나란한데,
// `cc` 만 줄을 없애지 않고 들여쓰기를 남긴다 — vim 과 같다 (ADR-0017).

// changeByMotion 은 motion 이 가리키는 범위를 바꾼다. `c` 뒤에 붙은 키가 motion 이다.
//
// 지우고 insert mode 로 들어가는 것이 `d` 와 다른 전부라 범위 계산도 register 도 같이 쓴다.
// 다만 두 자리에서 갈린다(ADR-0013, ADR-0017).
//
//   - 줄 단위(`cc` `cj` `cG`) 는 줄을 없애지 않고 들여쓰기만 남긴 채 비운다
//   - `cw` 는 단어 뒤 공백을 남긴다. vim 의 예외다
//
// 모르는 motion 이면 false 다. 그래야 `c` 뒤에 손이 미끄러진 키가 insert mode 로 끌고 가지 않는다.
// 바꿀 것이 없어도(빈 줄에서 친 `cw`) true 다 — vim 처럼 그 자리에서 넣기 시작한다.
func (buf *Buffer) changeByMotion(m motion, count, width int) (register, bool) {
	area, ok := m.span(*buf, count, width)
	if !ok {
		return register{}, false
	}

	return buf.changeRange(area, width)
}

// changeRange 는 잡아 둔 범위를 바꾼다. 되돌리기 구간을 열어 둔 채 나오므로 이어 친 글자가
// 같은 `u` 로 함께 돌아간다(ADR-0033).
// motion 이 잡은 것도 visual 이 고른 것도 여기로 온다(ADR-0037).
func (buf *Buffer) changeRange(area motionRange, width int) (register, bool) {
	if area.linewise {
		removed := buf.changeLines(area.startLine, area.endLine, width)
		buf.resumeEdit()

		return removed, true
	}

	removed, changed := buf.deleteText(area.startLine, area.startCol, area.endLine, area.endCol, width)
	if !changed {
		// 바꿀 것이 없었다. 편집이 없었으니 되돌리기 구간도 열지 않는다.
		return register{}, true
	}

	// 지운 자리에서 이어 친다. deleteText 가 normal mode 규칙으로 당겨 둔 커서를 되돌린다 —
	// `c$` 는 줄 끝 다음 칸에서 시작해야 하고 그 자리는 insert mode 에만 있다.
	buf.cursorLine, buf.cursorCol = area.startLine, area.startCol
	buf.updateDesiredCol(width)
	buf.resumeEdit()

	return removed, true
}

// wordEndToChange 는 `cw` 가 바꿀 끝 자리로 간다. 커서가 선 글자까지 넣은 자리다.
//
// 첫 걸음만 지금 단어의 끝에서 멈춘다 — 이미 단어의 마지막 글자 위면 그 글자 하나가 전부다.
// `e` 를 그대로 쓰면 거기서 다음 단어의 끝까지 먹는다. 나머지 걸음은 `e` 와 같아서
// `c2w` 는 다음 단어의 끝까지다. vim 이 첫 걸음에만 예외를 두는 것과 같다.
func (buf *Buffer) wordEndToChange(n int, kind wordKind, width int) {
	if !buf.atWordEnd(kind) {
		buf.wordEnd(kind)
	}

	buf.moveWordEnd(n-1, kind, width)
	buf.includeCursorCluster()
}

// atWordEnd 는 커서가 단어의 마지막 글자 위인지다. 공백 위면 끝낼 단어가 없어서 false 다.
func (buf Buffer) atWordEnd(kind wordKind) bool {
	class := buf.classAt(buf.cursorLine, buf.cursorCol, kind)
	if class == classBlank {
		return false
	}

	line, col, ok := buf.nextPos(buf.cursorLine, buf.cursorCol)

	return !ok || line != buf.cursorLine || buf.classAt(line, col, kind) != class
}

// changeLines 는 [from, to] 줄을 들여쓰기만 남기고 비운다.
//
// **줄을 없애지 않는다.** `dd` 는 줄이 사라지지만 `cc` 는 그 자리에서 다시 치는 것이라
// 빈 줄 하나가 남아야 한다. 여러 줄이면 그것들이 한 줄로 합쳐진다. vim 과 같다.
//
// 남기는 들여쓰기는 첫 줄의 것이다. 들여쓴 코드에서 `cc` 를 칠 때마다 tab 을 다시 치지 않는다.
// autoindent 가 아직 없어서 `o` `O` 는 들여쓰기를 이어받지 않는데, 이쪽은 새 줄을 만드는 것이
// 아니라 있던 줄을 비우는 것이라 원래 들여쓰기가 그 줄의 것이다.
func (buf *Buffer) changeLines(from, to, width int) register {
	count := to - from + 1
	indent := leadingBlank(buf.lines[from])

	buf.endEdit()
	buf.beginEdit(from, count)
	removed := buf.replaceLines(from, count, [][]byte{indent})
	buf.growEdit(1 - count)
	buf.endEdit()

	// 들여쓰기 다음 칸에서 이어 친다.
	buf.cursorLine, buf.cursorCol = from, len(indent)
	buf.updateDesiredCol(width)

	return register{lines: removed, linewise: true}
}
