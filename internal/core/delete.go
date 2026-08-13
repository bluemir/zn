package core

// register 는 지운 내용이다. vim 의 무명 register 에 해당한다.
//
// 아직 읽는 곳이 없다. 붙여넣기(`p`) 를 넣을 때 여기서 꺼낸다.
// 줄 단위로 지웠는지를 같이 들고 있어야 붙여넣기가 줄로 넣을지 글자로 넣을지 정할 수 있다.
type register struct {
	lines    [][]byte // 줄 단위면 지운 줄들, 글자 단위면 지운 조각을 줄로 끊은 것
	linewise bool
}

// deleteByMotion 은 motion 이 가리키는 범위를 지운다. `d` 뒤에 붙은 키가 motion 이다.
//
// 모르는 motion 이거나 지울 것이 없으면 아무것도 하지 않고 false 다. 그래야 `d` 뒤에
// 손이 미끄러진 키가 dirty 를 세우거나 되돌릴 앞날(redo) 을 날리지 않는다.
func (buf *Buffer) deleteByMotion(motion string, count, width int) (register, bool) {
	if line, ok := buf.lineMotionTarget(motion, count); ok {
		return buf.deleteLines(min(buf.cursorLine, line), max(buf.cursorLine, line), width), true
	}

	line, col, ok := buf.charMotionTarget(motion, count, width)
	if !ok {
		return register{}, false
	}

	// 뒤로 가는 motion 은 커서가 범위의 끝이다.
	if line < buf.cursorLine || (line == buf.cursorLine && col < buf.cursorCol) {
		return buf.deleteText(line, col, buf.cursorLine, buf.cursorCol, width)
	}

	return buf.deleteText(buf.cursorLine, buf.cursorCol, line, col, width)
}

// lineMotionTarget 은 줄 단위로 지우는 motion 이 닿는 줄이다.
//
// `dd` `dj` `dk` `dG` `dgg` 다. 커서 칸과 상관없이 줄 전체가 지워진다. vim 과 같다.
func (buf Buffer) lineMotionTarget(motion string, count int) (int, bool) {
	last := len(buf.lines) - 1
	n := max(count, 1)

	switch motion {
	case "d":
		// `dd` 는 커서 줄부터 n 줄이다. 줄이 모자라면 있는 만큼만 지운다.
		return min(buf.cursorLine+n-1, last), true
	case "j":
		// 이미 마지막 줄이면 갈 곳이 없어서 아무 일도 하지 않는다.
		// 줄이 모자라기만 한 것은 파일 끝까지 지운다. vim 과 같다.
		if buf.cursorLine == last {
			return 0, false
		}

		return min(buf.cursorLine+n, last), true
	case "k":
		if buf.cursorLine == 0 {
			return 0, false
		}

		return max(buf.cursorLine-n, 0), true
	case "G":
		// 숫자는 되풀이가 아니라 줄 번호다. 없으면 마지막 줄이다. 이동 키와 같다.
		if count > 0 {
			return min(count-1, last), true
		}

		return last, true
	case "g g":
		return min(n-1, last), true
	}

	return 0, false
}

// charMotionTarget 은 글자 단위로 지우는 motion 이 닿는 자리다. 지울 범위의 끝은 이 자리 앞까지다.
//
// motion 을 Buffer 복사본 위에서 실제로 실행해서 구한다. 이동 코드가 하나뿐이라 `w` 가 가는
// 자리와 `dw` 가 지우는 끝이 어긋날 수 없다. Buffer 는 slice header 뭉치라 복사가 싸고
// 이동은 lines 를 건드리지 않는다.
//
// `e` `E` 는 커서가 단어의 마지막 글자에 서므로 한 글자 더 나아간 자리를 준다.
// 그 글자까지 지워야 단어가 통째로 사라진다. vim 의 inclusive motion 이다.
func (buf Buffer) charMotionTarget(motion string, count, width int) (line, col int, ok bool) {
	n := max(count, 1)

	switch motion {
	case "h", "left":
		buf.moveLeft(n, width)
	case "l", "right":
		buf.moveRight(n, width)
	case "0":
		buf.moveLineStart(width)
	case "^":
		buf.moveLineFirstNonBlank(width)
	case "$":
		buf.moveLineEnd(n, width)
	case "b":
		buf.moveWordBackward(n, smallWord, width)
	case "B":
		buf.moveWordBackward(n, bigWord, width)
	case "e":
		buf.moveWordEnd(n, smallWord, width)
		buf.includeCursorCluster()
	case "E":
		buf.moveWordEnd(n, bigWord, width)
		buf.includeCursorCluster()
	case "w":
		buf.wordForwardToDelete(n, smallWord, width)
	case "W":
		buf.wordForwardToDelete(n, bigWord, width)
	default:
		return 0, 0, false
	}

	return buf.cursorLine, buf.cursorCol, true
}

// includeCursorCluster 는 커서가 선 글자까지 범위에 넣는다. inclusive motion 이 쓴다.
func (buf *Buffer) includeCursorCluster() {
	line := buf.lines[buf.cursorLine]
	if buf.cursorCol >= len(line) {
		return
	}

	buf.cursorCol += clusterSize(line, buf.cursorCol)
}

// wordForwardToDelete 는 `dw` 가 지울 끝 자리로 간다. 마지막 한 걸음은 줄을 넘지 않는다.
//
// vim 은 operator 와 함께 쓰인 `w` 를 줄 끝에서 멈춘다. 줄의 마지막 단어에서 `dw` 를 쳐도
// 다음 줄이 끌려 올라오지 않는다 — 줄을 없애려면 `dd` 가 있고, `dw` 로 줄이 합쳐지는 것은
// 단어 하나를 지우려던 손에는 사고다.
//
// 빈 줄에서는 그 줄 자체가 지울 것이라 다음 줄 시작까지 간다. 이것도 vim 과 같다.
// 중간 걸음은 줄을 넘어도 된다 — `2dw` 는 다음 줄의 단어까지 지운다.
func (buf *Buffer) wordForwardToDelete(n int, kind wordKind, width int) {
	buf.moveWordForward(n-1, kind, width)

	line := buf.cursorLine
	if len(buf.lines[line]) == 0 {
		if line+1 < len(buf.lines) {
			buf.cursorLine, buf.cursorCol = line+1, 0
		}

		return
	}

	buf.wordForward(kind)
	if buf.cursorLine != line {
		buf.cursorLine, buf.cursorCol = line, len(buf.lines[line])
	}
}

// deleteText 는 (startLine, startCol) 부터 (endLine, endCol) 앞까지 지운다.
// 지울 것이 없으면 아무것도 하지 않고 false 다.
func (buf *Buffer) deleteText(startLine, startCol, endLine, endCol, width int) (register, bool) {
	if startLine == endLine && startCol == endCol {
		return register{}, false
	}

	count := endLine - startLine + 1
	removed := make([][]byte, 0, count)
	if count == 1 {
		removed = append(removed, buf.lines[startLine][startCol:endCol])
	} else {
		removed = append(removed, buf.lines[startLine][startCol:])
		removed = append(removed, buf.lines[startLine+1:endLine]...)
		removed = append(removed, buf.lines[endLine][:endCol])
	}

	head, tail := buf.lines[startLine][:startCol], buf.lines[endLine][endCol:]
	joined := make([]byte, 0, len(head)+len(tail))
	joined = append(joined, head...)
	joined = append(joined, tail...)

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 지우기는 언제나 제 구간이다.
	buf.endEdit()
	buf.beginEdit(startLine, count)
	buf.replaceLines(startLine, count, [][]byte{joined})
	buf.growEdit(1 - count)
	buf.endEdit()

	// 지운 자리가 곧 커서 자리다. 줄 끝을 지웠으면 마지막 글자 위로 당겨진다.
	buf.cursorLine, buf.cursorCol = startLine, startCol
	buf.clampToNormal(width)
	buf.updateDesiredCol(width)

	return register{lines: removed}, true
}

// deleteLines 는 [from, to] 줄을 통째로 지운다.
//
// 파일의 모든 줄을 지우면 빈 줄 하나를 남긴다. lines 는 비어 있을 수 없다 —
// 커서가 설 줄이 없으면 그리는 쪽과 이동하는 쪽이 모두 무너진다.
func (buf *Buffer) deleteLines(from, to, width int) register {
	count := to - from + 1

	with := [][]byte(nil)
	if count == len(buf.lines) {
		with = [][]byte{{}}
	}

	buf.endEdit()
	buf.beginEdit(from, count)
	removed := buf.replaceLines(from, count, with)
	buf.growEdit(len(with) - count)
	buf.endEdit()

	// 지운 자리를 메운 줄로 간다. 마지막 줄을 지웠으면 그 앞 줄이다.
	// 칸은 첫 비공백이다 — 지운 줄의 칸을 지키는 것보다 들여쓴 코드에서 손이 덜 간다. vim 과 같다.
	buf.cursorLine = min(from, len(buf.lines)-1)
	buf.moveLineFirstNonBlank(width)
	buf.clampToNormal(width)

	return register{lines: removed, linewise: true}
}
