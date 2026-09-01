package core

import (
	"slices"
)

// 화면을 보는 것들이다. `top`·`topRow` 를 옮기거나 화면 좌표와 오가고, 하나같이 height 를 받는다 —
// Buffer 메서드 110 개 중 높이를 아는 것이 여기 든 열뿐이다.
//
// **화면 분할이 들어오면 커서·top·selection 과 함께 Buffer 밖으로 나갈 짐이 이 파일이다**
// (Buffer 주석, ADR-0080 뒤의 docs/tasks.md).
//
// 여기서 세는 단위는 논리 줄이 아니라 **화면 행**이다. `top` 이 논리 줄 index 이고 `topRow` 가
// 그 줄의 몇 번째 행부터 그리는지라, 둘이 짝이어야 자리 하나가 정해진다.
// 두 낱말의 뜻은 buffer-move.go 머리글에 있다.
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-screen.go 다 (ADR-0121).

// retreatRows 는 (line,row) 에서 화면 행 n 개 위로 올라간 위치를 돌려준다.
func (buf Buffer) retreatRows(line, row, n, width int) (int, int) {
	for range n {
		switch {
		case row > 0:
			row--
		case line > 0:
			line--
			row = len(wrapOffsets(buf.lines[line], width, buf.tabWidth())) - 1
		default:
			return 0, 0
		}
	}
	return line, row
}

// advanceRows 는 (line,row) 에서 화면 행 n 개 아래로 내려간 위치를 돌려준다.
// 파일 끝을 넘으면 마지막 줄의 마지막 행에서 멈춘다. retreatRows 의 반대 방향이다.
func (buf Buffer) advanceRows(line, row, n, width int) (int, int) {
	for range n {
		last := len(wrapOffsets(buf.lines[line], width, buf.tabWidth())) - 1

		switch {
		case row < last:
			row++
		case line < len(buf.lines)-1:
			line++
			row = 0
		default:
			return line, row
		}
	}
	return line, row
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

	// 뼈대 규칙이 없는 언어다. 강조도 같이 없다 — 둘 다 표의 같은 줄에서 오므로 같이 있고
	// 같이 없다(TestOutlineForEveryLanguage 가 지킨다). 표를 통째로 들고 있으니 물음도
	// 하나로 줄었다 — 전에는 「강조하는가」와 「뼈대가 있는가」를 따로 물었다 (ADR-0080).
	rule := buf.language.Outline()
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
