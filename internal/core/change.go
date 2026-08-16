package core

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
func (buf *Buffer) changeByMotion(motion string, count, width int) (register, bool) {
	area, ok := buf.changeRange(motion, count, width)
	if !ok {
		return register{}, false
	}

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

// changeRange 는 `c` 가 바꿀 범위다. `cw` 의 예외만 빼면 `d` 와 같은 계산이다.
//
// **`cw` 는 `ce` 다.** 커서가 공백 아닌 글자 위면 단어 뒤 공백을 남기고 단어 끝까지만 바꾼다.
// 단어 하나를 갈아 끼우고 나면 뒷 공백이 그대로 있어야 하기 때문이다. vim 과 같다.
// 커서가 공백 위면 예외가 아니라 `dw` 처럼 공백을 건너뛴다 — 바꿀 것이 그 공백이다.
func (buf Buffer) changeRange(motion string, count, width int) (motionRange, bool) {
	kind, isWordMotion := changeWordKind(motion)
	if !isWordMotion {
		return buf.rangeByMotion(motion, count, width)
	}

	// 빈 줄에서는 바꿀 것이 없다. `dw` 는 그 줄을 지우고 다음 줄을 끌어올리지만(ADR-0013),
	// 빈 줄에 글을 쓰려고 `cw` 를 친 손에는 다음 줄이 딸려 올라오는 것이 사고다.
	// vim 도 여기서는 줄을 합치지 않는다 — exclusive 보정 규칙이 이 자리를 줄 단위로 돌린다.
	if len(buf.lines[buf.cursorLine]) == 0 {
		return motionRange{
			startLine: buf.cursorLine, endLine: buf.cursorLine,
			targetLine: buf.cursorLine,
		}, true
	}

	// 공백 위면 예외가 아니다. 바꿀 것이 그 공백이라 `dw` 와 같이 건너뛴다.
	if buf.classAt(buf.cursorLine, buf.cursorCol, kind) == classBlank {
		return buf.rangeByMotion(motion, count, width)
	}

	// 이동을 복사본 위에서 실제로 실행해서 끝 자리를 얻는다. charMotionTarget 과 같은 방식이다.
	moved := buf
	moved.wordEndToChange(max(count, 1), kind, width)

	return motionRange{
		startLine: buf.cursorLine, startCol: buf.cursorCol,
		endLine: moved.cursorLine, endCol: moved.cursorCol,
		targetLine: moved.cursorLine, targetCol: moved.cursorCol,
	}, true
}

// changeWordKind 는 motion 이 `cw` 예외에 걸리는 단어 이동인지다.
func changeWordKind(motion string) (wordKind, bool) {
	switch motion {
	case "w":
		return smallWord, true
	case "W":
		return bigWord, true
	}

	return smallWord, false
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

// leadingBlank 는 줄 앞의 공백과 tab 이다.
//
// 줄은 제자리에서 바뀌지 않으므로(ADR-0001) 잘라낸 조각을 그대로 새 줄로 써도 된다.
func leadingBlank(line []byte) []byte {
	col := 0
	for col < len(line) && (line[col] == ' ' || line[col] == '\t') {
		col++
	}

	return line[:col]
}
