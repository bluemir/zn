package core

// sticky 머리줄은 화면 맨 위 몇 행을 감싸는 제목·정의로 덮는다(ADR-0049).
//
// 이 파일은 「무엇을 붙이나」와 「어떻게 그리나」를 같이 든다. 그리기만 하는 파일이 아니라
// render- 접두를 붙이지 않는다 — tabline.go·sidebar.go 와 같은 자리다(ADR-0036).

// stickyMaxRows 는 머리줄이 먹을 수 있는 최대 행 수다. 편집 영역의 절반이다.
//
// **설정이 아니라 자물쇠다.** 이 편집기에는 설정이 없고(README) 머리줄은 늘 켜져 있다.
// 그래서 깊게 중첩된 코드에서 감싸는 것이 열 겹일 때 화면이 통째로 머리줄이 되어 정작
// 보려던 줄이 남지 않는 일을 막을 자리가 여기밖에 없다. 절반은 「본문이 머리줄보다 적어지지
// 않는다」는 뜻이다.
//
// 40 행 화면에서 20 겹이 필요하므로 실제로 걸릴 일은 거의 없다. 그래서 정책이 아니라 자물쇠다.
func stickyMaxRows(height int) int {
	return height / 2
}

// renderStickyRow 는 화면 위에 붙는 머리줄 한 행이다.
//
// 본문 행과 **같은 모양**으로 그린다. 새 색도 구분선도 넣지 않는다 — 줄번호 칸이 그대로 있어서
// 번호가 뚝 끊기는 것(`3` 다음에 `47`) 으로 머리줄임이 읽힌다. 상대번호는 그 제목까지 가는
// 실제 `k` 수라 그대로 쓸모가 있다.
//
// **한 행에서 끊는다.** 편집 영역보다 긴 머리줄은 본문에서 여러 행으로 접히지만 여기서는 첫
// 행까지만 그린다 — 두 행이 되면 그 아래가 통째로 밀린다. 자르는 자리는 wrapOffsets 가 준다.
// 두 칸 글자와 결합 문자를 가운데서 가르지 않는 규칙이 이미 거기 있어서 자르는 코드를 새로
// 쓰지 않는다.
//
// 검색 매칭과 고른 범위는 칠하지 않는다. 이 줄은 화면 밖에 있는 줄이고, 본문에 같이 보이지도
// 않는 자리에서만 강조하면 어느 쪽이 지금 자리인지 흐려진다.
func (e editor) renderStickyRow(buf *Buffer, line int) string {
	text := buf.lines[line]
	width := e.contentWidth()

	end := len(text)
	if offsets := wrapOffsets(text, width); len(offsets) > 1 {
		end = offsets[1]
	}

	row := screenRow{line: line, start: 0, end: end}

	return e.renderLineNumber(buf.cursorLine, row) +
		renderRow(text, row, width, rowHighlight{
			cursorCol: -1,
			tokens:    buf.syntaxTokens(line),
		})
}

// stickyRows 는 실제로 그릴 머리줄들이다. 그린 행 수보다 많으면 바깥쪽부터 버린다.
//
// 파일 끝에서 화면이 다 안 차는 자리를 위한 것이다. 보통은 scrollTo 가 머리줄 수만큼 top 을
// 올려 두어 rows 가 화면을 채우므로 자를 것이 없다.
func (e editor) stickyRows(buf *Buffer, height, rows int) []int {
	sticky := buf.stickyAt(buf.top, height)
	if over := len(sticky) - rows; over > 0 {
		sticky = sticky[over:]
	}

	return sticky
}
