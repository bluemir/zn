package textarea

import "github.com/bluemir/zn/internal/scheme"

// 지우는 것들이다. `d`·`x`·`dd` 다. 지운 것은 register 로 나간다 (ADR-0017).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-delete.go 에 있다 (ADR-0121).

// deleteRange 는 잡아 둔 범위를 지운다. 지울 것이 없으면 false 다.
//
// **범위를 잡는 것은 부르는 쪽이다.** motion 이 잡은 것(`dw`) 도 visual 이 고른 것(`d`) 도
// 여기로 온다 — 둘이 범위를 얻는 길만 다르고 그다음은 같다(action.go, ADR-0037).
func (buf *Viewport) DeleteRange(area scheme.MotionRange) (TextBlock, bool) {
	if area.Linewise {
		return buf.deleteLines(area.Start.Line, area.End.Line), true
	}

	return buf.deleteText(area.Start, area.End)
}

// includeCursorCluster 는 커서가 선 글자까지 범위에 넣는다. inclusive motion 이 쓴다.
func (buf *Viewport) IncludeCursorCluster() {
	Line := buf.Lines[buf.Cursor.Line]
	if buf.Cursor.Col >= len(Line) {
		return
	}

	buf.Cursor.Col += GlyphSize(Line, buf.Cursor.Col)
}

// wordForwardToDelete 는 `dw` 가 지울 끝 자리로 간다. 마지막 한 걸음은 줄을 넘지 않는다.
//
// atWordEnd 는 커서가 단어의 마지막 글자 위인지다. 공백 위면 끝낼 단어가 없어서 false 다.
//
// `cw` 가 첫 걸음을 어디서 멈출지 이것으로 가른다. 그 판단은 motion.go 가 한다 — 여기는
// 「지금 자리가 단어 끝인가」만 답한다(ADR-0100).
// deleteText 는 (start.line, start.col) 부터 (end.line, end.col) 앞까지 지운다.
// 지울 것이 없으면 아무것도 하지 않고 false 다.
func (buf *Viewport) deleteText(Start, End scheme.Cursor) (TextBlock, bool) {
	if Start.Line == End.Line && Start.Col == End.Col {
		return TextBlock{}, false
	}

	count := End.Line - Start.Line + 1
	removed := buf.textBetween(Start, End)

	head, tail := buf.Lines[Start.Line][:Start.Col], buf.Lines[End.Line][End.Col:]
	joined := make([]byte, 0, len(head)+len(tail))
	joined = append(joined, head...)
	joined = append(joined, tail...)

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. 지우기는 언제나 제 구간이다.
	buf.EndEdit()
	buf.BeginEdit(Start.Line, count)
	buf.ReplaceLines(Start.Line, count, [][]byte{joined})
	buf.growEdit(1 - count)
	buf.EndEdit()

	// 지운 자리가 곧 커서 자리다. 줄 끝을 지웠으면 마지막 글자 위로 당겨진다.
	buf.Cursor.Line, buf.Cursor.Col = Start.Line, Start.Col
	buf.ClampToNormal()
	buf.UpdateDesiredCol()

	return TextBlock{Lines: removed}, true
}

// deleteLines 는 [from, to] 줄을 통째로 지운다.
//
// 파일의 모든 줄을 지우면 빈 줄 하나를 남긴다. lines 는 비어 있을 수 없다 —
// 커서가 설 줄이 없으면 그리는 쪽과 이동하는 쪽이 모두 무너진다.
func (buf *Viewport) deleteLines(from, to int) TextBlock {
	count := to - from + 1

	with := [][]byte(nil)
	if count == len(buf.Lines) {
		with = [][]byte{{}}
	}

	buf.EndEdit()
	buf.BeginEdit(from, count)
	removed := buf.ReplaceLines(from, count, with)
	buf.growEdit(len(with) - count)
	buf.EndEdit()

	// 지운 자리를 메운 줄로 간다. 마지막 줄을 지웠으면 그 앞 줄이다.
	// 칸은 첫 비공백이다 — 지운 줄의 칸을 지키는 것보다 들여쓴 코드에서 손이 덜 간다. vim 과 같다.
	buf.Cursor.Line = min(from, len(buf.Lines)-1)
	buf.MoveLineFirstNonBlank()
	buf.ClampToNormal()

	return TextBlock{Lines: removed, Linewise: true}
}
