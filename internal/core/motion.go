package core

// motion 은 커서가 갈 자리를 정하는 것이다.
//
// 이동 키로 그냥 칠 수도 있고(`w`) operator 뒤에 붙어 범위를 정할 수도 있다(`dw`).
// 파서가 키에서 곧바로 이 type 을 만들고, 동작은 이름이 아니라 이것을 들고 다닌다 —
// 실행하는 쪽이 `"d w"` 같은 문자열을 다시 뜯지 않는다(ADR-0034).
//
// 메서드가 둘인 것은 이동과 범위가 갈리는 자리가 있어서다. `dw` 는 줄을 넘지 않고
// `de` 는 커서가 선 글자까지다(ADR-0013).
type motion interface {
	// span 은 operator 가 잡을 범위다. 잡을 것이 없으면 false 다.
	//
	// count 를 그대로 받는다. 0 이 "숫자 없음" 이고 그것을 어떻게 읽을지는 motion 마다 다르다 —
	// `3w` 는 되풀이이고 `3G` 는 줄 번호다.
	span(buf Buffer, count, width int) (motionRange, bool)
}

// moveMotion 은 이동 키로도 칠 수 있는 motion 이다. 거의 다 여기 든다.
//
// operator 뒤에서만 생기는 것(`dd` 의 「줄 전체」) 은 갈 자리가 없어서 span 만 갖는다.
type moveMotion interface {
	motion

	// move 는 커서를 옮긴다. 이동 키를 그냥 쳤을 때다.
	move(buf *Buffer, count, width int)
}

// charSpan 은 글자 단위 범위다. 커서 자리와 이동이 닿은 자리 사이이고 닿은 자리는 제외다.
//
// 이동을 복사본 위에서 실제로 실행해서 얻는다. 이동 코드가 하나뿐이라 `w` 가 가는 자리와
// `dw` 가 지우는 끝이 어긋날 수 없다. Buffer 는 slice header 뭉치라 복사가 싸고
// 이동은 lines 를 건드리지 않는다(ADR-0013).
func charSpan(buf Buffer, moved Buffer) (motionRange, bool) {
	line, col := moved.cursorLine, moved.cursorCol

	// 뒤로 가는 motion 은 커서가 범위의 끝이다.
	if line < buf.cursorLine || (line == buf.cursorLine && col < buf.cursorCol) {
		return motionRange{
			startLine: line, startCol: col,
			endLine: buf.cursorLine, endCol: buf.cursorCol,
			targetLine: line, targetCol: col,
		}, true
	}

	return motionRange{
		startLine: buf.cursorLine, startCol: buf.cursorCol,
		endLine: line, endCol: col,
		targetLine: line, targetCol: col,
	}, true
}

// lineSpan 은 줄 단위 범위다. 커서 줄과 닿은 줄 사이의 줄 전체이고 커서 칸과 상관없다.
//
// 칸은 복사가 쓴다. 이동을 실제로 실행해서 얻으므로 `k` 는 칸을 지키고 `gg` 는 첫 비공백으로
// 간다 — 이동 키를 직접 쳤을 때와 같은 자리다(ADR-0017).
func lineSpan(buf Buffer, moved Buffer) (motionRange, bool) {
	return motionRange{
		startLine:  min(buf.cursorLine, moved.cursorLine),
		endLine:    max(buf.cursorLine, moved.cursorLine),
		targetLine: moved.cursorLine,
		targetCol:  moved.cursorCol,
		linewise:   true,
	}, true
}

// moveOn 은 복사본 위에서 이동을 실행한 결과다. span 을 구하는 자리가 모두 이것으로 시작한다.
func moveOn(m moveMotion, buf Buffer, count, width int) Buffer {
	m.move(&buf, count, width)

	return buf
}

// ── 글자 단위로 잡히는 이동 ──

// motionLeft 는 `h` 와 `←` 다.
type motionLeft struct{}

func (motionLeft) move(buf *Buffer, count, width int) { buf.moveLeft(max(count, 1), width) }

func (m motionLeft) span(buf Buffer, count, width int) (motionRange, bool) {
	return charSpan(buf, moveOn(m, buf, count, width))
}

// motionRight 는 `l` 과 `→` 다. `x` 도 이것으로 한 글자를 잡는다.
type motionRight struct{}

func (motionRight) move(buf *Buffer, count, width int) { buf.moveRight(max(count, 1), width) }

func (m motionRight) span(buf Buffer, count, width int) (motionRange, bool) {
	return charSpan(buf, moveOn(m, buf, count, width))
}

// motionLineStart 는 `0` 이다. count 를 받지 않는다.
type motionLineStart struct{}

func (motionLineStart) move(buf *Buffer, count, width int) { buf.moveLineStart(width) }

func (m motionLineStart) span(buf Buffer, count, width int) (motionRange, bool) {
	return charSpan(buf, moveOn(m, buf, count, width))
}

// motionFirstNonBlank 는 `^` 다. 들여쓰기를 건너뛴 첫 글자로 간다.
type motionFirstNonBlank struct{}

func (motionFirstNonBlank) move(buf *Buffer, count, width int) { buf.moveLineFirstNonBlank(width) }

func (m motionFirstNonBlank) span(buf Buffer, count, width int) (motionRange, bool) {
	return charSpan(buf, moveOn(m, buf, count, width))
}

// motionLineEnd 는 `$` 다. count 는 되풀이가 아니라 줄 수다 — `3$` 는 두 줄 아래의 줄 끝이다.
type motionLineEnd struct{}

func (motionLineEnd) move(buf *Buffer, count, width int) { buf.moveLineEnd(max(count, 1), width) }

func (m motionLineEnd) span(buf Buffer, count, width int) (motionRange, bool) {
	return charSpan(buf, moveOn(m, buf, count, width))
}

// motionWordBack 은 `b` 와 `B` 다.
type motionWordBack struct{ kind wordKind }

func (m motionWordBack) move(buf *Buffer, count, width int) {
	buf.moveWordBackward(max(count, 1), m.kind, width)
}

func (m motionWordBack) span(buf Buffer, count, width int) (motionRange, bool) {
	return charSpan(buf, moveOn(m, buf, count, width))
}

// motionWordEnd 는 `e` 와 `E` 다.
//
// 커서가 단어의 마지막 글자에 서므로 범위는 한 글자 더 나아간다. 그 글자까지 지워야 단어가
// 통째로 사라진다. vim 의 inclusive motion 이다(ADR-0013).
type motionWordEnd struct{ kind wordKind }

func (m motionWordEnd) move(buf *Buffer, count, width int) {
	buf.moveWordEnd(max(count, 1), m.kind, width)
}

func (m motionWordEnd) span(buf Buffer, count, width int) (motionRange, bool) {
	moved := moveOn(m, buf, count, width)
	moved.includeCursorCluster()

	return charSpan(buf, moved)
}

// motionWordForward 는 `w` 와 `W` 다.
//
// operator 와 함께일 때 마지막 한 걸음이 줄을 넘지 않는다. 줄의 마지막 단어에서 `dw` 를 쳐도
// 다음 줄이 끌려 올라오지 않는다 — 줄을 없애려면 `dd` 가 있고, `dw` 로 줄이 합쳐지는 것은
// 단어 하나를 지우려던 손에는 사고다. vim 과 같다(ADR-0013).
type motionWordForward struct{ kind wordKind }

func (m motionWordForward) move(buf *Buffer, count, width int) {
	buf.moveWordForward(max(count, 1), m.kind, width)
}

func (m motionWordForward) span(buf Buffer, count, width int) (motionRange, bool) {
	moved := buf
	moved.wordForwardToDelete(max(count, 1), m.kind, width)

	return charSpan(buf, moved)
}

// ── 줄 단위로 잡히는 이동 ──

// motionLineDown 은 `j` 다. wrap 된 줄도 한 번에 건넌다.
type motionLineDown struct{}

func (motionLineDown) move(buf *Buffer, count, width int) { buf.moveDownLine(max(count, 1)) }

func (m motionLineDown) span(buf Buffer, count, width int) (motionRange, bool) {
	// 이미 마지막 줄이면 갈 곳이 없어서 아무 일도 하지 않는다.
	// 줄이 모자라기만 한 것은 파일 끝까지다. vim 과 같다.
	if buf.cursorLine == len(buf.lines)-1 {
		return motionRange{}, false
	}

	return lineSpan(buf, moveOn(m, buf, count, width))
}

// motionLineUp 은 `k` 다.
type motionLineUp struct{}

func (motionLineUp) move(buf *Buffer, count, width int) { buf.moveUpLine(max(count, 1)) }

func (m motionLineUp) span(buf Buffer, count, width int) (motionRange, bool) {
	if buf.cursorLine == 0 {
		return motionRange{}, false
	}

	return lineSpan(buf, moveOn(m, buf, count, width))
}

// motionToLastLine 은 `G` 다. 숫자는 되풀이가 아니라 줄 번호이고, 없으면 마지막 줄이다.
//
// 숫자가 없다는 것을 알아야 해서 count 를 1 로 메워 받지 않는다. 파서가 0 을 그대로 준다.
type motionToLastLine struct{}

func (motionToLastLine) move(buf *Buffer, count, width int) {
	line := len(buf.lines) - 1
	if count > 0 {
		line = count - 1
	}

	buf.moveToLine(line, width)
}

func (m motionToLastLine) span(buf Buffer, count, width int) (motionRange, bool) {
	return lineSpan(buf, moveOn(m, buf, count, width))
}

// motionToFirstLine 은 `gg` 다. 숫자가 있으면 그 줄, 없으면 첫 줄이다.
type motionToFirstLine struct{}

func (motionToFirstLine) move(buf *Buffer, count, width int) {
	buf.moveToLine(max(count, 1)-1, width)
}

func (m motionToFirstLine) span(buf Buffer, count, width int) (motionRange, bool) {
	return lineSpan(buf, moveOn(m, buf, count, width))
}

// ── operator 가 받지 않는 이동 ──

// motionRowUp, motionRowDown 은 `↑` 와 `↓` 다. 화면 행 단위라 줄 단위와 갈려서
// operator 뒤에는 받지 않는다 — `d↑` 를 어떻게 잡을지는 아직 정하지 않았다(ADR-0013).
//
// **홀로 쓸 때는 count 를 받는다.** `3↑` 가 세 행이다 — `3j` 가 세 줄인 것과 같다.
// operator 를 안 받는 것과는 다른 물음이고, ADR-0013 이 정한 것은 그쪽뿐이다 (ADR-0081).
type motionRowUp struct{}

func (motionRowUp) move(buf *Buffer, count, width int) { buf.moveUpRow(max(count, 1), width) }

func (motionRowUp) span(Buffer, int, int) (motionRange, bool) {
	return motionRange{}, false
}

type motionRowDown struct{}

func (motionRowDown) move(buf *Buffer, count, width int) { buf.moveDownRow(max(count, 1), width) }

func (motionRowDown) span(Buffer, int, int) (motionRange, bool) {
	return motionRange{}, false
}

// motionRowStart, motionRowEnd 는 `home` 과 `end` 다. 화면 행의 양끝이라 `0`·`$` 와 갈린다
// (buffer.go 의 moveRowStart).
//
// operator 뒤에는 받지 않는다 — `↑`·`↓` 와 같은 까닭이고 같은 자리에 산다. 잡을 범위를
// 정하려면 「화면 행 단위 범위」가 무엇인지부터 정해야 하는데, 그것이 아직 없다(ADR-0013).
type motionRowStart struct{}

func (motionRowStart) move(buf *Buffer, count, width int) { buf.moveRowStart(width) }

func (motionRowStart) span(Buffer, int, int) (motionRange, bool) {
	return motionRange{}, false
}

type motionRowEnd struct{}

func (motionRowEnd) move(buf *Buffer, count, width int) { buf.moveRowEnd(width) }

func (motionRowEnd) span(Buffer, int, int) (motionRange, bool) {
	return motionRange{}, false
}

// motionChangeWord 는 `c` 뒤에 온 `w`·`W` 다. **`cw` 는 `ce` 다.**
//
// 커서가 공백 아닌 글자 위면 단어 뒤 공백을 남기고 단어 끝까지만 바꾼다. 단어 하나를 갈아
// 끼우고 나면 뒷 공백이 그대로 있어야 하기 때문이다. vim 과 같다(ADR-0033).
//
// 파서가 operator 를 보고 이 type 을 고른다 — `dw` 는 motionWordForward 이고 `cw` 만 이것이다.
// operator 에 따라 motion 이 갈리는 자리는 지금 여기 하나뿐이다.
type motionChangeWord struct{ kind wordKind }

// 이동 키로는 쓰이지 않는다. `c` 뒤에서만 만들어지므로 그냥 `w` 로 간다.
func (m motionChangeWord) move(buf *Buffer, count, width int) {
	buf.moveWordForward(max(count, 1), m.kind, width)
}

func (m motionChangeWord) span(buf Buffer, count, width int) (motionRange, bool) {
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
	if buf.classAt(buf.cursorLine, buf.cursorCol, m.kind) == classBlank {
		return motionWordForward{kind: m.kind}.span(buf, count, width)
	}

	moved := buf
	moved.wordEndToChange(max(count, 1), m.kind, width)

	return charSpan(buf, moved)
}

// motionWholeLines 는 operator 를 두 번 쳤을 때의 「줄 전체」다(`dd` `yy` `cc`).
//
// 이동 키가 아니라서 move 가 없다 — operator 뒤에서만 생긴다. 커서 줄부터 count 줄이고
// 줄이 모자라면 있는 만큼이다. 커서는 움직이지 않는다.
type motionWholeLines struct{}

func (motionWholeLines) span(buf Buffer, count, width int) (motionRange, bool) {
	end := min(buf.cursorLine+max(count, 1)-1, len(buf.lines)-1)

	return motionRange{
		startLine: buf.cursorLine, endLine: end,
		targetLine: end, targetCol: buf.cursorCol,
		linewise: true,
	}, true
}

// motionWordObject 는 커서가 든 단어다. vim 의 `iw`·`aw`·`iW`·`aW` 다(ADR-0091).
//
// 이동 키가 아니라서 move 가 없다 — operator 뒤에서만 생긴다. motionWholeLines 와 같은 갈래다.
//
// **count 를 보지 않는다.** vim 의 `d2iw` 는 단어와 공백을 번갈아 세는데, 모르면 「단어 둘」로
// 읽힌다. 틀리게 읽히는 숫자를 받지 않는다(ADR-0091 §4).
type motionWordObject struct {
	kind   wordKind
	around bool // `aw` 인가. 단어 둘레의 공백까지 먹는다
}

func (m motionWordObject) span(buf Buffer, count, width int) (motionRange, bool) {
	line := buf.cursorLine
	text := buf.lines[line]

	// **공백 위에서는 잡지 않는다.** vim 은 공백 덩어리를 잡는데, 같은 키가 커서 한 칸에 따라
	// 「단어를 바꾼다」와 「공백을 지운다」로 갈리면 눌러 보고 아는 키가 된다(ADR-0091 §2).
	//
	// 빈 줄과 줄 끝도 여기서 같이 걸린다. classAt 이 줄 끝을 공백으로 보기 때문이다.
	class := buf.classAt(line, buf.cursorCol, m.kind)
	if class == classBlank {
		return motionRange{}, false
	}

	// 같은 부류가 이어지는 데까지 좌우로 넓힌다. **줄을 넘지 않는다** — 단어는 줄 안의 것이고
	// 줄 끝이 공백이라 저절로 멈춘다.
	start := buf.cursorCol
	for start > 0 {
		prev := prevClusterStart(text, 0, start)
		if buf.classAt(line, prev, m.kind) != class {
			break
		}

		start = prev
	}

	end := buf.cursorCol
	for end < len(text) && buf.classAt(line, end, m.kind) == class {
		end += clusterSize(text, end)
	}

	if m.around {
		start, end = buf.aroundWord(line, start, end, m.kind)
	}

	return motionRange{
		startLine: line, startCol: start,
		endLine: line, endCol: end,
		targetLine: line, targetCol: start,
	}, true
}

// aroundWord 는 `aw` 가 단어에 더 먹는 공백까지 넓힌 범위다.
//
// **뒤 공백을 먹고, 없으면 앞 공백을 먹는다.** vim 과 같다. 줄 가운데 낱말을 `daw` 로 지우면
// 공백이 하나만 남고, 줄 끝 낱말이면 앞 공백을 먹어서 줄 끝에 공백이 남지 않는다.
func (buf Buffer) aroundWord(line, start, end int, kind wordKind) (int, int) {
	text := buf.lines[line]

	after := end
	for after < len(text) && buf.classAt(line, after, kind) == classBlank {
		after += clusterSize(text, after)
	}

	if after > end {
		return start, after
	}

	for start > 0 {
		prev := prevClusterStart(text, 0, start)
		if buf.classAt(line, prev, kind) != classBlank {
			break
		}

		start = prev
	}

	return start, end
}

// motionRange 는 operator 가 motion 으로 잡은 범위다. `d` 와 `y` 가 같이 쓴다.
//
// 글자 단위면 (startLine, startCol) 부터 (endLine, endCol) **앞까지** 이고,
// 줄 단위면 [startLine, endLine] 줄 전체다.
//
// targetLine/targetCol 은 motion 이 커서를 둔 자리다. 앞으로 가는 motion 이면 범위의 끝,
// 뒤로 가는 motion 이면 범위의 시작이다. 지우기는 제 커서 규칙이 있어서 쓰지 않고 복사가 쓴다 —
// vim 의 `yk` 는 칸을 지키고 `ygg` 는 첫 비공백으로 가는데, 그 차이가 곧 motion 이 둔 자리다.
type motionRange struct {
	startLine, startCol   int
	endLine, endCol       int
	targetLine, targetCol int
	linewise              bool
}
