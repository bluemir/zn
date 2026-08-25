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

	// 문법 캐시도 같은 자리를 같은 수로 갈아끼운다. 줄과 index 가 나란해야 아래쪽에 담아둔
	// 것을 그대로 쓸 수 있다(syntax.go).
	//
	// 이 함수가 lines 를 갈아끼우는 유일한 자리라 여기 한 줄이면 된다. 편집 경로가 늘어도
	// 제자리 수정은 undo 를 깨므로(위 주석) 이곳을 지나지 않을 수 없다.
	buf.syntax.replace(at, count, len(with))

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

// resumeEdit 는 방금 닫은 구간을 다시 연다. 이어지는 타이핑이 그 구간에 들어가서
// 지운 것과 새로 친 글자가 한 번의 `u` 로 함께 돌아간다.
//
// `c` 가 쓴다 — 지우기와 insert 가 한 동작이라 되돌리기도 하나여야 한다. vim 과 같다.
// 되돌릴 것이 하나도 없으면(지운 것이 없었으면) 열 구간도 없다.
func (buf *Buffer) resumeEdit() {
	buf.editing = len(buf.undo) > 0
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

// insertLines 는 at 자리에 줄들을 끼운다. at 이 줄 수와 같으면 마지막 줄 뒤다.
// 줄 단위 붙여넣기가 쓴다. 커서는 건드리지 않는다 — 부르는 쪽이 정한다.
//
// 끼우는 자리 옆의 줄 하나를 붙잡고 replaceLines 로 갈아끼운다. 되돌리기 구간은 건드린 줄을
// 담아야 열리는데(beginEdit), 새로 끼우는 줄은 아직 없는 줄이라 붙잡을 것이 없다.
func (buf *Buffer) insertLines(at int, lines [][]byte) {
	anchor := min(at, len(buf.lines)-1)

	next := make([][]byte, 0, len(lines)+1)
	if at > anchor {
		// 마지막 줄 뒤다. 앞 줄을 붙잡고 그 뒤에 잇는다.
		next = append(next, buf.lines[anchor])
		next = append(next, lines...)
	} else {
		next = append(next, lines...)
		next = append(next, buf.lines[anchor])
	}

	buf.beginEdit(anchor, 1)
	buf.replaceLines(anchor, 1, next)
	buf.growEdit(len(lines))
}

// openLineBelow 는 지금 줄 아래에 줄을 만들고 커서를 그 줄로 옮긴다. `o` 가 쓴다.
//
// 줄 끝으로 가서 줄바꿈을 넣는 것과 같다. insert 를 그대로 쓰므로 되돌리기 구간도 거기서 열린다 —
// `o` 로 만든 줄과 이어서 친 글자가 한 번의 `u` 로 같이 사라진다. vim 과 같다.
//
// 새 줄은 이 파일의 규칙이 정한 들여쓰기를 받는다(indent.go). `cc` 가 들여쓰기를 남기는 것과
// 손이 같아졌다 — 둘 다 「새로 칠 줄」이다.
func (buf *Buffer) openLineBelow(width int) {
	buf.cursorCol = len(buf.lines[buf.cursorLine])
	buf.insertNewLine(width)
}

// openLineAbove 는 지금 줄 위에 줄을 만들고 커서를 그 줄로 옮긴다. `O` 가 쓴다.
//
// 들여쓰기는 **윗 줄** 에서 가져온다. 지금 줄이 아니다 — 새 줄이 들어가는 자리가 윗 줄 다음이라
// `o` 를 윗 줄에서 친 것과 같은 자리다. 첫 줄 위에는 가져올 곳이 없어서 빈 줄이다.
func (buf *Buffer) openLineAbove(width int) {
	indent := []byte(nil)
	if buf.cursorLine > 0 {
		above := buf.lines[buf.cursorLine-1]
		indent = buf.indentForNewLine(buf.cursorLine-1, above)
	}

	buf.cursorCol = 0
	buf.insert(concat(indent, []byte{'\n'}), width)

	// 줄 맨 앞에서 가르면 원래 내용이 아래로 밀리고 커서가 그것을 따라간다.
	// 새로 생긴 줄은 그 위이므로 한 줄 되돌아온다.
	buf.cursorLine--
	buf.cursorCol = len(indent)
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

// deleteForward 는 커서 자리 글자를 지운다. 줄 끝이면 다음 줄을 끌어올려 붙인다.
// insert mode 의 `delete` 다 — deleteBackward 의 거울이다.
//
// normal 에는 걸지 않았다. 그 자리에는 `x` 가 이미 있고 register 에 담는 것까지 정해져 있다.
func (buf *Buffer) deleteForward(width int) {
	line := buf.lines[buf.cursorLine]

	if buf.cursorCol < len(line) {
		// 한글 3 byte, 이모지 18 byte 도 한 번에 지운다. clusterSize 가 글자 경계를 준다.
		to := buf.cursorCol + clusterSize(line, buf.cursorCol)

		rest := make([]byte, 0, len(line)-(to-buf.cursorCol))
		rest = append(rest, line[:buf.cursorCol]...)
		rest = append(rest, line[to:]...)

		buf.beginEdit(buf.cursorLine, 1)
		buf.replaceLines(buf.cursorLine, 1, [][]byte{rest})
		buf.updateDesiredCol(width)

		return
	}

	// 줄 끝이다. 마지막 줄이면 끌어올 것이 없다.
	if buf.cursorLine == len(buf.lines)-1 {
		return
	}

	next := buf.lines[buf.cursorLine+1]

	joined := make([]byte, 0, len(line)+len(next))
	joined = append(joined, line...)
	joined = append(joined, next...)

	// 두 줄을 건드리므로 열린 구간이 있으면 범위가 넓어진다. deleteBackward 와 같다.
	buf.beginEdit(buf.cursorLine, 2)
	buf.replaceLines(buf.cursorLine, 2, [][]byte{joined})
	buf.growEdit(-1)

	// 커서는 제자리다 — 이은 자리가 곧 커서 자리다.
	buf.updateDesiredCol(width)
}

// trimTrailingSpace 는 모든 줄 끝의 공백과 tab 을 지운다. 지운 줄 수를 돌려준다.
//
// 지우는 것은 `' '` 와 `'\t'` 뿐이다. 유니코드 공백(NBSP 등) 은 건드리지 않는다 —
// 눈에 보이지 않는 글자가 조용히 사라지는 것이 더 나쁘고, 일부러 넣는 문서가 있다.
//
// 바뀌는 줄 전체를 한 번에 갈아끼운다. 줄마다 beginEdit 를 부르면 두 번째부터 열린 구간을
// 넓히면서 **이미 잘린 지금 내용** 을 되돌릴 내용으로 담아서, `u` 를 눌러도 원본이 돌아오지 않는다.
func (buf *Buffer) trimTrailingSpace(width int) int {
	// 바꿀 것이 없는데 beginEdit 를 부르면 dirty 가 서고 redo 가 날아간다. 먼저 훑기만 한다.
	first, last, count := -1, -1, 0
	for i, line := range buf.lines {
		if len(line) == len(trimLineEnd(line)) {
			continue
		}

		if first < 0 {
			first = i
		}
		last = i
		count++
	}
	if count == 0 {
		return 0
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
	buf.endEdit()
	buf.beginEdit(first, last-first+1)

	// 사이에 낀 안 바뀐 줄은 원본 그대로 담는다. 범위를 파일 전체로 넓히지 않으려는 것뿐이다.
	next := make([][]byte, 0, last-first+1)
	for _, line := range buf.lines[first : last+1] {
		next = append(next, trimLineEnd(line))
	}

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	buf.replaceLines(first, last-first+1, next)

	// 커서가 잘려나간 자리에 서 있었으면 줄 끝으로 당긴다.
	// beginEdit 가 이미 원래 자리를 기록했으므로 `u` 로 되돌리면 거기로 돌아간다.
	buf.cursorCol = min(buf.cursorCol, len(buf.lines[buf.cursorLine]))
	buf.updateDesiredCol(width)

	buf.endEdit()

	return count
}

// replaceAll 은 파일 전체를 next 로 갈아끼운다. 저장할 때 포매터가 낸 글이 이 길로 온다
// (save-hook.go).
//
// **한 번의 `u` 로 통째로 돌아간다.** 포매터가 고친 것은 한 동작이라 되돌리기도 하나여야 하고,
// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다 — trimTrailingSpace 와 같은
// 자리다. 줄 수가 달라지므로 growEdit 으로 열린 구간을 지금 줄 수에 맞춘다.
//
// 커서는 줄 번호를 지킨다. 포매터는 들여쓰기를 고치고 import 를 옮기지 줄을 뒤섞지 않아서,
// 보던 자리가 대개 그 자리에 있다. 파일이 짧아졌으면 범위 안으로 끌어온다 — 다시 읽기가
// 커서를 이어받는 것과 같은 태도다(Reload).
func (buf *Buffer) replaceAll(next [][]byte, width int) {
	before := len(buf.lines)

	buf.endEdit()
	buf.beginEdit(0, before)

	buf.replaceLines(0, before, next)
	buf.growEdit(len(next) - before)

	buf.cursorLine = min(buf.cursorLine, len(buf.lines)-1)
	buf.cursorCol = min(buf.cursorCol, len(buf.lines[buf.cursorLine]))
	buf.updateDesiredCol(width)

	buf.endEdit()
}

// trimLineEnd 는 줄 끝의 공백과 tab 을 뗀 부분이다.
// 자르기만 하므로 새로 할당하지 않는다(ADR-0001).
func trimLineEnd(line []byte) []byte {
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}

	return line[:end]
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
