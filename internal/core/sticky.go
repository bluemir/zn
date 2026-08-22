package core

import (
	"slices"

	"github.com/bluemir/zn/internal/syntax"
)

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

// stickyAt 은 line 을 화면 맨 위로 그릴 때 그 위에 붙는 머리줄들이다.
// 바깥쪽이 앞이고 line 자신은 들지 않는다. 강조하지 않는 파일이면 nil 이다.
//
// **기준은 커서가 아니라 화면 맨 윗줄이다.** 그래서 위로 밀려 안 보이게 된 것만 붙는다 —
// 화면에 이미 있는 제목이 위에 한 번 더 그려지지 않고, 파일 첫 화면에서는 아무것도 안 붙어
// 자리를 안 먹는다. VSCode 와 같다.
//
// **한 프레임에 여러 번 불린다.** 그리는 쪽이 부르고, scrollTo 가 수렴하는 동안 화면 높이의
// 절반만큼 더 부른다. 그래서 위로 훑다가 깊이 0 에서 멈춘다 — 파일을 처음부터 다시 읽지 않는다.
func (buf *Buffer) stickyAt(line, height int) []int {
	// 맨 윗줄이면 위에 붙일 것이 없다. 본문이 두 행보다 낮으면 머리줄에 내줄 자리가 없다.
	if line < 1 || line >= len(buf.lines) || height < 2 {
		return nil
	}

	// **여기서 캐시를 채운다.** scrollTo 는 Update 에서 돌고 토큰은 View 에서 채워진다.
	// 채우지 않으면 두 쪽의 답이 한 프레임 어긋나서 커서가 머리줄 아래에 그려진다.
	// receiver 가 포인터인 이유가 이것뿐이다 — editorView 와 같은 손이다(ADR-0039).
	//
	// 위로만 훑으므로 line 까지면 넉넉하다. lexSyntaxTo 는 언제나 0 번 줄부터 내려온다.
	buf.lexSyntaxTo(line)

	// 강조하지 않는 파일이다. 뼈대 규칙도 없다 — 둘 다 표의 같은 줄에서 오므로 같이 있고
	// 같이 없다(TestOutlineForEveryLanguage 가 지킨다).
	if buf.syntax.start == nil {
		return nil
	}

	rule := syntax.OutlineFor(buf.path)
	if rule == nil {
		return nil
	}

	// limit 은 「여기보다 얕아야 바깥이다」다. 지나온 줄 중 가장 얕은 깊이다.
	//
	// **머리줄만이 아니라 지나는 줄마다 이것을 낮춘다.** 이미 닫힌 블록을 걸러내는 것이
	// 그것이다. 머리줄만 보고 낮추면 이런 자리가 틀린다(ADR-0049):
	//
	//	func alpha() {
	//	    if a {
	//	        x()
	//	    }
	//	    s := "text" +
	//	        "more"      <- 여기서 위를 보면 닫힌 `if a {` 가 붙는다
	//
	// 닫는 줄(`}`) 이 그 블록과 같은 깊이라, 지나면서 한계를 낮추면 그 위의 여는 줄이 저절로
	// 걸러진다. 파일 전체의 괄호를 세지 않고 닫힘을 아는 것이 이 한 줄이다.
	limit := rule.Depth(buf.lines[line], buf.syntaxTokens(line))

	heads := []int{}
	for at := line - 1; at >= 0 && limit > 0; at-- {
		depth := rule.Depth(buf.lines[at], buf.syntaxTokens(at))
		if depth >= limit {
			continue
		}

		limit = depth

		// 한계를 낮춘 줄에만 묻는다. 머리줄은 반드시 더 얕으므로 나머지는 물어볼 필요가 없고,
		// 이 물음이 줄을 복제하는 규칙(makeOutline) 이 있어서 값이 싸지 않다.
		if rule.Heads(buf.lines[at], buf.syntaxTokens(at)) {
			heads = append(heads, at)
		}
	}

	if len(heads) < 1 {
		return nil
	}

	// 위로 훑었으므로 안쪽부터 담겼다. 바깥쪽이 앞이다.
	slices.Reverse(heads)

	// 넘치면 **바깥쪽부터 버린다.** 가장 안쪽이 방금 화면 위로 사라진 것이라 잃으면 가장
	// 아깝고, 가장 바깥쪽(`# 제목`, `package`) 은 파일을 열 때부터 알고 있는 것이다.
	if over := len(heads) - stickyMaxRows(height); over > 0 {
		heads = heads[over:]
	}

	return heads
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
