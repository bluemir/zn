package textarea

import (
	"bytes"
	"slices"
)

// 줄을 갈아끼우는 자리다. **replaceLines 가 `buf.lines` 를 바꾸는 유일한 함수**이고,
// 되돌리기 구간(beginEdit…endEdit) 과 문법 캐시가 거기 하나에 걸린다 (ADR-0001, ADR-0033).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-edit.go 에 있다 (ADR-0121).

// beginEdit 는 바꾸기 직전에 부른다. [at, at+count) 를 건드릴 참이라고 알린다.
//
// 열린 구간이 있으면 거기에 합치고, 없으면 새로 연다.
// 열린 구간의 범위를 벗어나면 범위를 넓힌다. 밖의 줄은 이 구간에서 아직 안 건드린 줄이라
// 지금 내용이 곧 구간 시작 시점의 내용이다. 줄 시작에서 Backspace 로 앞 줄과 합칠 때 이 경로를 탄다.
func (viewport *Viewport) BeginEdit(at, count int) {
	// 새 편집이 생기면 되돌아갈 앞날은 사라진다.
	viewport.redo = nil
	viewport.Dirty = true

	if !viewport.editing {
		viewport.undo = append(viewport.undo, edit{
			at:     at,
			before: append([][]byte(nil), viewport.Lines[at:at+count]...),
			count:  count,
			Cursor: viewport.Cursor,
		})
		viewport.editing = true

		return
	}

	open := &viewport.undo[len(viewport.undo)-1]

	if at < open.at {
		head := append([][]byte(nil), viewport.Lines[at:open.at]...)
		open.before = append(head, open.before...)
		open.count += open.at - at
		open.at = at
	}
	if End := at + count; End > open.at+open.count {
		tail := viewport.Lines[open.at+open.count : End]
		open.before = append(open.before, tail...)
		open.count = End - open.at
	}
}

// insert 는 커서 자리에 text 를 넣는다. 모든 입력이 지나는 단 하나의 경로다.
//
// 타이핑은 insert(글자), Enter 는 insert("\n"), Tab 은 insert("\t"),
// 여러 줄 붙여넣기는 insert(붙여넣은 것) 이다. 줄바꿈이 들어오면 줄이 갈린다.
//
// backing buffer 는 건드리지 않는다. 바뀐 줄만 새로 만들어 갈아끼운다(ADR-0001).
func (viewport *Viewport) Insert(text []byte) {
	if len(text) == 0 {
		return
	}

	Line := viewport.Lines[viewport.Cursor.Line]
	head, tail := Line[:viewport.Cursor.Col], Line[viewport.Cursor.Col:]
	pieces := bytes.Split(text, []byte{'\n'})

	viewport.BeginEdit(viewport.Cursor.Line, 1)

	if len(pieces) == 1 {
		merged := make([]byte, 0, len(Line)+len(text))
		merged = append(merged, head...)
		merged = append(merged, text...)
		merged = append(merged, tail...)

		viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{merged})
		viewport.Cursor.Col += len(text)
		viewport.UpdateDesiredCol()

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

	viewport.ReplaceLines(viewport.Cursor.Line, 1, next)
	viewport.growEdit(len(next) - 1)

	viewport.Cursor.Line += len(next) - 1
	viewport.Cursor.Col = len(last)
	viewport.UpdateDesiredCol()
}

// insertLines 는 at 자리에 줄들을 끼운다. at 이 줄 수와 같으면 마지막 줄 뒤다.
// 줄 단위 붙여넣기가 쓴다. 커서는 건드리지 않는다 — 부르는 쪽이 정한다.
//
// 끼우는 자리 옆의 줄 하나를 붙잡고 replaceLines 로 갈아끼운다. 되돌리기 구간은 건드린 줄을
// 담아야 열리는데(beginEdit), 새로 끼우는 줄은 아직 없는 줄이라 붙잡을 것이 없다.
func (viewport *Viewport) insertLines(at int, Lines [][]byte) {
	anchor := min(at, len(viewport.Lines)-1)

	next := make([][]byte, 0, len(Lines)+1)
	if at > anchor {
		// 마지막 줄 뒤다. 앞 줄을 붙잡고 그 뒤에 잇는다.
		next = append(next, viewport.Lines[anchor])
		next = append(next, Lines...)
	} else {
		next = append(next, Lines...)
		next = append(next, viewport.Lines[anchor])
	}

	viewport.BeginEdit(anchor, 1)
	viewport.ReplaceLines(anchor, 1, next)
	viewport.growEdit(len(Lines))
}

// openLineBelow 는 지금 줄 아래에 줄을 만들고 커서를 그 줄로 옮긴다. `o` 가 쓴다.
//
// 줄 끝으로 가서 줄바꿈을 넣는 것과 같다. insert 를 그대로 쓰므로 되돌리기 구간도 거기서 열린다 —
// `o` 로 만든 줄과 이어서 친 글자가 한 번의 `u` 로 같이 사라진다. vim 과 같다.
//
// 새 줄은 이 파일의 규칙이 정한 들여쓰기를 받는다(indent.go). `cc` 가 들여쓰기를 남기는 것과
// 손이 같아졌다 — 둘 다 「새로 칠 줄」이다.
func (viewport *Viewport) OpenLineBelow() {
	viewport.Cursor.Col = len(viewport.Lines[viewport.Cursor.Line])
	viewport.InsertNewLine()
}

// openLineAbove 는 지금 줄 위에 줄을 만들고 커서를 그 줄로 옮긴다. `O` 가 쓴다.
//
// 들여쓰기는 **윗 줄** 에서 가져온다. 지금 줄이 아니다 — 새 줄이 들어가는 자리가 윗 줄 다음이라
// `o` 를 윗 줄에서 친 것과 같은 자리다. 첫 줄 위에는 가져올 곳이 없어서 빈 줄이다.
func (viewport *Viewport) OpenLineAbove() {
	indent := []byte(nil)
	if viewport.Cursor.Line > 0 {
		above := viewport.Lines[viewport.Cursor.Line-1]
		indent = viewport.indentForNewLine(viewport.Cursor.Line-1, above)
	}

	viewport.Cursor.Col = 0
	viewport.Insert(concat(indent, []byte{'\n'}))

	// 줄 맨 앞에서 가르면 원래 내용이 아래로 밀리고 커서가 그것을 따라간다.
	// 새로 생긴 줄은 그 위이므로 한 줄 되돌아온다.
	viewport.Cursor.Line--
	viewport.Cursor.Col = len(indent)
	viewport.UpdateDesiredCol()
}

// deleteBackward 는 커서 앞 글자를 지운다. 줄 시작이면 앞 줄과 합친다.
func (viewport *Viewport) DeleteBackward() {
	if viewport.Cursor.Col > 0 {
		Line := viewport.Lines[viewport.Cursor.Line]
		from := viewport.prevOffset(viewport.Cursor.Col)

		// 한글 3 byte, 이모지 18 byte 도 한 번에 지운다. prevOffset 이 글자 경계를 준다.
		rest := make([]byte, 0, len(Line)-(viewport.Cursor.Col-from))
		rest = append(rest, Line[:from]...)
		rest = append(rest, Line[viewport.Cursor.Col:]...)

		viewport.BeginEdit(viewport.Cursor.Line, 1)
		viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{rest})
		viewport.Cursor.Col = from
		viewport.UpdateDesiredCol()

		return
	}

	if viewport.Cursor.Line == 0 {
		return
	}

	prev := viewport.Lines[viewport.Cursor.Line-1]
	Line := viewport.Lines[viewport.Cursor.Line]

	joined := make([]byte, 0, len(prev)+len(Line))
	joined = append(joined, prev...)
	joined = append(joined, Line...)

	// 두 줄을 건드리므로 열린 구간이 있으면 범위가 넓어진다.
	viewport.BeginEdit(viewport.Cursor.Line-1, 2)
	viewport.ReplaceLines(viewport.Cursor.Line-1, 2, [][]byte{joined})
	viewport.growEdit(-1)

	viewport.Cursor.Line--
	viewport.Cursor.Col = len(prev)
	viewport.UpdateDesiredCol()
}

// deleteForward 는 커서 자리 글자를 지운다. 줄 끝이면 다음 줄을 끌어올려 붙인다.
// insert mode 의 `delete` 다 — deleteBackward 의 거울이다.
//
// normal 에는 걸지 않았다. 그 자리에는 `x` 가 이미 있고 register 에 담는 것까지 정해져 있다.
func (viewport *Viewport) DeleteForward() {
	Line := viewport.Lines[viewport.Cursor.Line]

	if viewport.Cursor.Col < len(Line) {
		// 한글 3 byte, 이모지 18 byte 도 한 번에 지운다. lines.Size 가 글자 경계를 준다.
		to := viewport.Cursor.Col + GlyphSize(Line, viewport.Cursor.Col)

		rest := make([]byte, 0, len(Line)-(to-viewport.Cursor.Col))
		rest = append(rest, Line[:viewport.Cursor.Col]...)
		rest = append(rest, Line[to:]...)

		viewport.BeginEdit(viewport.Cursor.Line, 1)
		viewport.ReplaceLines(viewport.Cursor.Line, 1, [][]byte{rest})
		viewport.UpdateDesiredCol()

		return
	}

	// 줄 끝이다. 마지막 줄이면 끌어올 것이 없다.
	if viewport.Cursor.Line == len(viewport.Lines)-1 {
		return
	}

	next := viewport.Lines[viewport.Cursor.Line+1]

	joined := make([]byte, 0, len(Line)+len(next))
	joined = append(joined, Line...)
	joined = append(joined, next...)

	// 두 줄을 건드리므로 열린 구간이 있으면 범위가 넓어진다. deleteBackward 와 같다.
	viewport.BeginEdit(viewport.Cursor.Line, 2)
	viewport.ReplaceLines(viewport.Cursor.Line, 2, [][]byte{joined})
	viewport.growEdit(-1)

	// 커서는 제자리다 — 이은 자리가 곧 커서 자리다.
	viewport.UpdateDesiredCol()
}

// trimTrailingSpace 는 `[from, to)` 줄의 끝에 붙은 공백과 tab 을 지운다. 지운 줄 수를 준다.
//
// 지우는 것은 `' '` 와 `'\t'` 뿐이다. 유니코드 공백(NBSP 등) 은 건드리지 않는다 —
// 눈에 보이지 않는 글자가 조용히 사라지는 것이 더 나쁘고, 일부러 넣는 문서가 있다.
//
// 부르는 쪽이 구간을 댄다. 저장 hook 은 파일 전체를 대고(buffer-save.go), 팔레트는 visual
// 에서 고른 범위가 있으면 그것을 댄다(ADR-0111).
//
// 바뀌는 줄 전체를 한 번에 갈아끼운다. 줄마다 beginEdit 를 부르면 두 번째부터 열린 구간을
// 넓히면서 **이미 잘린 지금 내용** 을 되돌릴 내용으로 담아서, `u` 를 눌러도 원본이 돌아오지 않는다.
func (viewport *Viewport) TrimTrailingSpace(from, to int) int {
	// 바꿀 것이 없는데 beginEdit 를 부르면 dirty 가 서고 redo 가 날아간다. 먼저 훑기만 한다.
	first, last, count := -1, -1, 0
	for i := from; i < to; i++ {
		Line := viewport.Lines[i]
		if len(Line) == len(trimLineEnd(Line)) {
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
	viewport.EndEdit()
	viewport.BeginEdit(first, last-first+1)

	// 사이에 낀 안 바뀐 줄은 원본 그대로 담는다. 범위를 파일 전체로 넓히지 않으려는 것뿐이다.
	next := make([][]byte, 0, last-first+1)
	for _, Line := range viewport.Lines[first : last+1] {
		next = append(next, trimLineEnd(Line))
	}

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	viewport.ReplaceLines(first, last-first+1, next)

	// 커서가 잘려나간 자리에 서 있었으면 줄 끝으로 당긴다.
	// beginEdit 가 이미 원래 자리를 기록했으므로 `u` 로 되돌리면 거기로 돌아간다.
	viewport.Cursor.Col = min(viewport.Cursor.Col, len(viewport.Lines[viewport.Cursor.Line]))
	viewport.UpdateDesiredCol()

	viewport.EndEdit()

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
func (viewport *Viewport) ReplaceAll(next [][]byte) {
	before := len(viewport.Lines)

	viewport.EndEdit()
	viewport.BeginEdit(0, before)

	viewport.ReplaceLines(0, before, next)
	viewport.growEdit(len(next) - before)

	viewport.Cursor.Line = min(viewport.Cursor.Line, len(viewport.Lines)-1)
	viewport.Cursor.Col = min(viewport.Cursor.Col, len(viewport.Lines[viewport.Cursor.Line]))
	viewport.UpdateDesiredCol()

	viewport.EndEdit()
}

// sortLines 는 `[from, to)` 줄을 오름차순으로 다시 늘어놓는다. 자리가 바뀐 줄 수를 준다.
//
// **byte 순이다.** UTF-8 의 byte 순은 코드포인트 순과 같아서 한글 음절은 가나다 순으로
// 선다(음절이 유니코드에서 이어져 있다). 사전 순(locale) 을 쓰지 않는 것은 그것이 판마다
// 다른 답을 내기 때문이고, 「설정 없이 컴파일된다」는 이 편집기의 태도와도 맞지 않는다.
//
// **안정 정렬이다.** 같은 줄끼리는 원래 차례를 지킨다. 눈에 보이지 않는 자리이지만, 정렬을
// 두 번 돌렸을 때 결과가 달라지지 않게 한다.
//
// 이미 정렬되어 있으면 아무것도 하지 않는다. 바꿀 것이 없는데 beginEdit 를 부르면 dirty 가
// 서고 redo 가 날아간다(trimTrailingSpace 와 같은 자리다).
func (viewport *Viewport) SortLines(from, to int) int {
	next := make([][]byte, to-from)
	copy(next, viewport.Lines[from:to])

	slices.SortStableFunc(next, bytes.Compare)

	moved := 0
	for i := range next {
		if !bytes.Equal(next[i], viewport.Lines[from+i]) {
			moved++
		}
	}

	if moved == 0 {
		return 0
	}

	// **고른 구간을 통째로 갈아끼운다.** 줄이 자리를 바꾸는 일이라 「바뀐 구간」이 구간 전체다.
	// 앞의 타이핑에 섞이면 `u` 한 번에 남의 편집까지 딸려온다(trimTrailingSpace 와 같은 자리다).
	viewport.EndEdit()
	viewport.BeginEdit(from, to-from)

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	viewport.ReplaceLines(from, to-from, next)

	// 커서 줄의 내용이 바뀌었으므로 줄 밖에 서 있을 수 있다.
	viewport.Cursor.Col = min(viewport.Cursor.Col, len(viewport.Lines[viewport.Cursor.Line]))
	viewport.UpdateDesiredCol()

	viewport.EndEdit()

	return moved
}

// squeezeSpaces 는 `[from, to)` 줄 가운데의 이어진 공백을 한 칸으로 줄인다. 고친 줄 수를 준다.
//
// **들여쓰기는 건드리지 않는다.** 줄 앞의 공백은 이 편집기가 뜻으로 다루는 것이라
// (`.editorconfig`·들여쓰기 마커·autoindent) 줄이면 코드가 깨진다.
//
// **줄 끝도 건드리지 않는다.** 그것은 「줄 끝 공백 지우기」의 몫이다. 한 명령이 두 가지를
// 하면 무엇이 내 줄을 고쳤는지 알기 어려워진다(ADR-0011 의 「한 기능에 진입점 하나」).
func (viewport *Viewport) SqueezeSpaces(from, to int) int {
	first, last, count := -1, -1, 0

	// 파일 전체 길이로 잡아 자리를 줄 번호와 맞춘다. 구간 밖은 nil 인 채로 두고 쓰지 않는다 —
	// first·last 가 줄 번호라 자리를 옮겨 셈하면 잘라내는 자리에서 어긋나기 쉽다.
	next := make([][]byte, len(viewport.Lines))
	for i := from; i < to; i++ {
		Line := viewport.Lines[i]

		next[i] = squeezeInnerSpaces(Line)
		if len(next[i]) == len(Line) {
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

	viewport.EndEdit()
	viewport.BeginEdit(first, last-first+1)

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	viewport.ReplaceLines(first, last-first+1, next[first:last+1])

	// 커서가 줄어든 자리 뒤에 서 있었으면 줄 끝으로 당긴다.
	viewport.Cursor.Col = min(viewport.Cursor.Col, len(viewport.Lines[viewport.Cursor.Line]))
	viewport.UpdateDesiredCol()

	viewport.EndEdit()

	return count
}

// applyUndo 는 마지막 변경을 되돌린다. 되돌릴 것이 없으면 false 다.
func (viewport *Viewport) ApplyUndo() bool {
	viewport.EndEdit()

	if len(viewport.undo) == 0 {
		return false
	}

	last := viewport.undo[len(viewport.undo)-1]
	viewport.undo = viewport.undo[:len(viewport.undo)-1]
	viewport.redo = append(viewport.redo, viewport.revert(last))

	return true
}

// applyRedo 는 되돌린 것을 다시 적용한다.
func (viewport *Viewport) ApplyRedo() bool {
	viewport.EndEdit()

	if len(viewport.redo) == 0 {
		return false
	}

	last := viewport.redo[len(viewport.redo)-1]
	viewport.redo = viewport.redo[:len(viewport.redo)-1]
	viewport.undo = append(viewport.undo, viewport.revert(last))

	return true
}

// revert 는 e 를 적용하고 반대 방향으로 되돌릴 edit 을 돌려준다.
func (viewport *Viewport) revert(e edit) edit {
	inverse := edit{
		at:     e.at,
		before: append([][]byte(nil), viewport.Lines[e.at:e.at+e.count]...),
		count:  len(e.before),
		Cursor: viewport.Cursor,
	}

	viewport.ReplaceLines(e.at, e.count, e.before)

	viewport.Cursor.Line = min(e.Cursor.Line, len(viewport.Lines)-1)
	viewport.Cursor.Col = min(e.Cursor.Col, len(viewport.Lines[viewport.Cursor.Line]))
	viewport.UpdateDesiredCol()

	return inverse
}
