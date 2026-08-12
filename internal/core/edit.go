package core

import (
	"bytes"
)

// replaceLines 는 lines 의 [at, at+count) 를 with 로 갈아끼우고 원래 있던 줄들을 돌려준다.
//
// 반드시 새 slice 를 만들어 담는다. buf.lines 를 제자리에서 append 하면 옛 줄을 가리키던
// undo 기록이 덮여서 되돌릴 수 없게 된다.
func (buf *Buffer) replaceLines(at, count int, with [][]byte) [][]byte {
	old := buf.lines[at : at+count]

	next := make([][]byte, 0, len(buf.lines)-count+len(with))
	next = append(next, buf.lines[:at]...)
	next = append(next, with...)
	next = append(next, buf.lines[at+count:]...)
	buf.lines = next

	return old
}

// beginEdit 는 바꾸기 직전에 부른다. [at, at+count) 를 건드릴 참이라고 알린다.
//
// 열린 구간이 있으면 거기에 합치고, 없으면 새로 연다.
// 열린 구간의 범위를 벗어나면 범위를 넓힌다. 밖의 줄은 이 구간에서 아직 안 건드린 줄이라
// 지금 내용이 곧 구간 시작 시점의 내용이다. 줄 시작에서 Backspace 로 앞 줄과 합칠 때 이 경로를 탄다.
func (buf *Buffer) beginEdit(at, count int) {
	// 새 편집이 생기면 되돌아갈 앞날은 사라진다.
	buf.redo = nil
	buf.dirty = true

	if !buf.editing {
		buf.undo = append(buf.undo, edit{
			at:         at,
			before:     append([][]byte(nil), buf.lines[at:at+count]...),
			count:      count,
			cursorLine: buf.cursorLine,
			cursorCol:  buf.cursorCol,
		})
		buf.editing = true

		return
	}

	open := &buf.undo[len(buf.undo)-1]

	if at < open.at {
		head := append([][]byte(nil), buf.lines[at:open.at]...)
		open.before = append(head, open.before...)
		open.count += open.at - at
		open.at = at
	}
	if end := at + count; end > open.at+open.count {
		tail := buf.lines[open.at+open.count : end]
		open.before = append(open.before, tail...)
		open.count = end - open.at
	}
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
func (buf *Buffer) endEdit() {
	buf.editing = false
}

// insert 는 커서 자리에 text 를 넣는다. 모든 입력이 지나는 단 하나의 경로다.
//
// 타이핑은 insert(글자), Enter 는 insert("\n"), Tab 은 insert("\t"),
// 여러 줄 붙여넣기는 insert(붙여넣은 것) 이다. 줄바꿈이 들어오면 줄이 갈린다.
//
// backing buffer 는 건드리지 않는다. 바뀐 줄만 새로 만들어 갈아끼운다(ADR-0001).
func (buf *Buffer) insert(text []byte, width int) {
	if len(text) == 0 {
		return
	}

	line := buf.lines[buf.cursorLine]
	head, tail := line[:buf.cursorCol], line[buf.cursorCol:]
	pieces := bytes.Split(text, []byte{'\n'})

	buf.beginEdit(buf.cursorLine, 1)

	if len(pieces) == 1 {
		merged := make([]byte, 0, len(line)+len(text))
		merged = append(merged, head...)
		merged = append(merged, text...)
		merged = append(merged, tail...)

		buf.replaceLines(buf.cursorLine, 1, [][]byte{merged})
		buf.cursorCol += len(text)
		buf.updateDesiredCol(width)

		return
	}

	next := make([][]byte, 0, len(pieces))

	first := make([]byte, 0, len(head)+len(pieces[0]))
	first = append(first, head...)
	first = append(first, pieces[0]...)
	next = append(next, first)

	for _, piece := range pieces[1 : len(pieces)-1] {
		next = append(next, piece)
	}

	last := pieces[len(pieces)-1]
	joined := make([]byte, 0, len(last)+len(tail))
	joined = append(joined, last...)
	joined = append(joined, tail...)
	next = append(next, joined)

	buf.replaceLines(buf.cursorLine, 1, next)
	buf.growEdit(len(next) - 1)

	buf.cursorLine += len(next) - 1
	buf.cursorCol = len(last)
	buf.updateDesiredCol(width)
}

// deleteBackward 는 커서 앞 글자를 지운다. 줄 시작이면 앞 줄과 합친다.
func (buf *Buffer) deleteBackward(width int) {
	if buf.cursorCol > 0 {
		line := buf.lines[buf.cursorLine]
		from := buf.prevOffset(buf.cursorCol, width)

		// 한글 3 byte, 이모지 18 byte 도 한 번에 지운다. prevOffset 이 글자 경계를 준다.
		rest := make([]byte, 0, len(line)-(buf.cursorCol-from))
		rest = append(rest, line[:from]...)
		rest = append(rest, line[buf.cursorCol:]...)

		buf.beginEdit(buf.cursorLine, 1)
		buf.replaceLines(buf.cursorLine, 1, [][]byte{rest})
		buf.cursorCol = from
		buf.updateDesiredCol(width)

		return
	}

	if buf.cursorLine == 0 {
		return
	}

	prev := buf.lines[buf.cursorLine-1]
	line := buf.lines[buf.cursorLine]

	joined := make([]byte, 0, len(prev)+len(line))
	joined = append(joined, prev...)
	joined = append(joined, line...)

	// 두 줄을 건드리므로 열린 구간이 있으면 범위가 넓어진다.
	buf.beginEdit(buf.cursorLine-1, 2)
	buf.replaceLines(buf.cursorLine-1, 2, [][]byte{joined})
	buf.growEdit(-1)

	buf.cursorLine--
	buf.cursorCol = len(prev)
	buf.updateDesiredCol(width)
}

// applyUndo 는 마지막 변경을 되돌린다. 되돌릴 것이 없으면 false 다.
func (buf *Buffer) applyUndo(width int) bool {
	buf.endEdit()

	if len(buf.undo) == 0 {
		return false
	}

	last := buf.undo[len(buf.undo)-1]
	buf.undo = buf.undo[:len(buf.undo)-1]
	buf.redo = append(buf.redo, buf.revert(last, width))

	return true
}

// applyRedo 는 되돌린 것을 다시 적용한다.
func (buf *Buffer) applyRedo(width int) bool {
	buf.endEdit()

	if len(buf.redo) == 0 {
		return false
	}

	last := buf.redo[len(buf.redo)-1]
	buf.redo = buf.redo[:len(buf.redo)-1]
	buf.undo = append(buf.undo, buf.revert(last, width))

	return true
}

// revert 는 e 를 적용하고 반대 방향으로 되돌릴 edit 을 돌려준다.
func (buf *Buffer) revert(e edit, width int) edit {
	inverse := edit{
		at:         e.at,
		before:     append([][]byte(nil), buf.lines[e.at:e.at+e.count]...),
		count:      len(e.before),
		cursorLine: buf.cursorLine,
		cursorCol:  buf.cursorCol,
	}

	buf.replaceLines(e.at, e.count, e.before)

	buf.cursorLine = min(e.cursorLine, len(buf.lines)-1)
	buf.cursorCol = min(e.cursorCol, len(buf.lines[buf.cursorLine]))
	buf.updateDesiredCol(width)

	return inverse
}
