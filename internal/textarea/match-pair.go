package textarea

import (
	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/syntax"
)

// 짝을 찾는 자리다. `%` 가 이것 하나를 부른다 (ADR-0132).
//
// 짝은 둘이다. 괄호 셋(`()` `[]` `{}`) 은 모든 파일에서, tag 쌍(`<div>`↔`</div>`) 은 tag 가
// 짝을 이루는 언어에서만 본다(syntax 의 PairsTags).
//
// **글자만 보고 센다.** 문자열이나 주석 안의 괄호도 짝으로 센다 — `x := "{"` 의 `{` 가
// 그렇다. 거르려면 커서에서 짝까지의 모든 줄을 문법으로 훑어야 하는데, 지금 문법 훑기는
// 화면까지만 게으르게 한다(buffer-syntax.go 의 LexSyntaxTo). vim 의 기본과 같은 자리다.

// pairSpan 은 짝 한쪽이 차지한 자리다. 괄호는 글자 하나이고 tag 는 `<` 부터 `>` 까지다.
//
// End 는 제외다. MotionRange 와 같은 규칙이다.
type pairSpan struct {
	start, end scheme.Cursor
}

// matchBrackets 는 `%` 가 오가는 괄호 셋이다. vim 의 `matchpairs` 기본값과 같다.
//
// `<`·`>` 는 없다. go 의 제네릭이나 C++ 템플릿에 쓸모가 있지만 `a < b` 의 비교 연산자와
// 구별할 수 없어서, 짝이 아닌 자리로 데려간다.
var matchBrackets = []struct{ open, close byte }{
	{'(', ')'},
	{'[', ']'},
	{'{', '}'},
}

// MatchPair 는 커서가 든 짝의 반대쪽이다. 짝이 없으면 false 다.
//
// target 은 `%` 가 커서를 놓을 자리다. **짝의 먼 쪽 끝이다** — 앞으로 가면 마지막 글자,
// 뒤로 가면 첫 글자다. 괄호는 한 글자라 둘이 같고 tag 에서만 갈린다.
//
// 그렇게 두는 것은 `v%` 때문이다. 고른 범위는 커서가 선 글자까지라, 앞으로 가는 `%` 가
// `</div>` 의 `<` 에 서면 닫는 tag 가 반만 잡혀서 `/div>` 가 남는다. 먼 쪽 끝에 서면
// 괄호에서든 tag 에서든 `v%d` 와 `d%` 가 같은 것을 지운다.
//
// area 는 operator 가 잡을 범위이고 **양쪽을 다 담는다**. `d%` 가 여는 괄호와 닫는 괄호를
// 함께 지운다 — vim 의 inclusive motion 이다. 글자 단위라 줄을 넘어도 줄이 통째로 사라지지
// 않는다.
//
// 두 값을 돌려주는 것은 부르는 쪽이 이동과 범위 둘로 갈리기 때문이다(motion.go 의 move·span).
// target 을 area 에서 만들어 낼 수 없다 — 어느 끝이 짝인지를 area 가 말하지 않는다.
func (buf Buffer) MatchPair(at scheme.Cursor) (target scheme.Cursor, area scheme.MotionRange, ok bool) {
	here, there, found := buf.pairAt(at)
	if !found {
		return scheme.Cursor{}, scheme.MotionRange{}, false
	}

	// 뒤로 가는 짝이면 there 가 앞이다.
	if before(there.start, here.start) {
		return there.start, scheme.MotionRange{Start: there.start, End: here.end}, true
	}

	last := scheme.Cursor{Line: there.end.Line, Col: there.end.Col - 1}

	return last, scheme.MotionRange{Start: here.start, End: there.end}, true
}

// pairAt 은 커서가 든 쪽과 그 짝이다.
//
// **괄호가 먼저다.** `<div onclick="f()">` 의 `(` 위에서는 tag 가 아니라 그 괄호를 본다 —
// 커서가 선 글자 자체가 짝이면 그것을 제쳐 두고 둘레를 볼 이유가 없다.
func (buf Buffer) pairAt(at scheme.Cursor) (here, there pairSpan, ok bool) {
	if here, there, ok := buf.bracketAt(at); ok {
		return here, there, true
	}

	if buf.Language.PairsTags() {
		return buf.tagPairAt(at)
	}

	return pairSpan{}, pairSpan{}, false
}

// ── 괄호 ──

// bracketAt 은 커서가 선 괄호와 그 짝이다.
//
// **커서가 괄호 글자 위에 있을 때만 일한다.** vim 은 그 줄에서 오른쪽으로 첫 괄호를 찾아
// 주는데, 그러면 `%` 가 어디로 갈지 알려면 커서 오른쪽 줄 끝까지를 눈으로 훑어야 한다.
func (buf Buffer) bracketAt(at scheme.Cursor) (here, there pairSpan, ok bool) {
	line := buf.Line(at.Line)
	if at.Col < 0 || at.Col >= len(line) {
		return pairSpan{}, pairSpan{}, false
	}

	here = pairSpan{start: at, end: scheme.Cursor{Line: at.Line, Col: at.Col + 1}}

	for _, pair := range matchBrackets {
		switch line[at.Col] {
		case pair.open:
			found, ok := buf.scanBracket(at, pair.open, pair.close, scanForward)
			if !ok {
				return pairSpan{}, pairSpan{}, false
			}

			return here, found, true
		case pair.close:
			found, ok := buf.scanBracket(at, pair.close, pair.open, scanBackward)
			if !ok {
				return pairSpan{}, pairSpan{}, false
			}

			return here, found, true
		}
	}

	return pairSpan{}, pairSpan{}, false
}

// scanBracket 은 from 에 선 괄호의 짝을 찾는다. same 를 만나면 겹이 깊어지고 other 를 만나면
// 얕아진다. 파일 끝까지 가도 겹이 0 이 되지 않으면 false 다.
func (buf Buffer) scanBracket(from scheme.Cursor, same, other byte, dir scanDirection) (pairSpan, bool) {
	depth := 1

	for at, ok := buf.stepByte(from, dir); ok; at, ok = buf.stepByte(at, dir) {
		switch buf.Line(at.Line)[at.Col] {
		case same:
			depth++
		case other:
			depth--
			if depth == 0 {
				return pairSpan{start: at, end: scheme.Cursor{Line: at.Line, Col: at.Col + 1}}, true
			}
		}
	}

	return pairSpan{}, false
}

// ── tag ──

// htmlTagSpan 은 줄에서 읽어 낸 tag 하나다.
type htmlTagSpan struct {
	span    pairSpan
	name    string
	closing bool // `</div>` 인가
	selfEnd bool // `<img />` 처럼 스스로 닫았는가
}

// pairs 는 짝을 가지는 tag 인지다. void tag(`<br>`) 와 스스로 닫은 tag 는 짝이 없다.
func (tag htmlTagSpan) pairs() bool {
	return !tag.selfEnd && !syntax.HTMLVoidTag(tag.name)
}

// tagPairAt 은 커서가 든 tag 와 그 짝이다.
//
// **`<` 부터 `>` 까지 안 아무 곳이면 그 tag 다.** `<di|v class="a">` 에서도 `</div>` 로 간다.
// tag 는 이름을 보고 치는 것이라 양끝 글자 위에 서 있으라고 하면 손이 한 번 더 움직인다.
func (buf Buffer) tagPairAt(at scheme.Cursor) (here, there pairSpan, ok bool) {
	tag, ok := buf.tagAround(at)
	if !ok || !tag.pairs() {
		return pairSpan{}, pairSpan{}, false
	}

	dir := scanForward
	if tag.closing {
		dir = scanBackward
	}

	found, ok := buf.scanTag(tag, dir)
	if !ok {
		return pairSpan{}, pairSpan{}, false
	}

	return tag.span, found.span, true
}

// scanTag 는 같은 이름의 짝 tag 를 찾는다. 같은 이름이 또 열리면 겹이 깊어진다.
//
// **주석 안의 tag 도 센다.** `<!-- <div> -->` 가 겹으로 잡힌다 — 어느 자리가 주석 안인지는
// 파일 처음부터 훑어야 알고, 문자열 안의 괄호를 세는 것과 같은 자리다(이 파일 머리글).
func (buf Buffer) scanTag(from htmlTagSpan, dir scanDirection) (htmlTagSpan, bool) {
	depth := 1

	at := from.span.start
	if dir == scanForward {
		at = scheme.Cursor{Line: from.span.end.Line, Col: from.span.end.Col - 1}
	}

	for {
		next, ok := buf.nextTag(at, dir)
		if !ok {
			return htmlTagSpan{}, false
		}

		at = next.span.start
		if dir == scanForward {
			at = scheme.Cursor{Line: next.span.end.Line, Col: next.span.end.Col - 1}
		}

		if next.name != from.name || !next.pairs() {
			continue
		}

		if next.closing == from.closing {
			depth++

			continue
		}

		depth--
		if depth == 0 {
			return next, true
		}
	}
}

// nextTag 는 from 다음(또는 앞) 의 tag 다. 짝을 이루지 않는 것(`<!DOCTYPE>` `<?xml?>`) 은
// 건너뛴다.
func (buf Buffer) nextTag(from scheme.Cursor, dir scanDirection) (htmlTagSpan, bool) {
	for at, ok := buf.stepByte(from, dir); ok; at, ok = buf.stepByte(at, dir) {
		if buf.Line(at.Line)[at.Col] != '<' {
			continue
		}

		tag, ok := buf.tagFrom(at)
		if !ok {
			continue
		}

		return tag, true
	}

	return htmlTagSpan{}, false
}

// tagAround 는 커서를 덮는 tag 다. tag 밖이면 false 다.
//
// 뒤로 가며 `<` 를 찾는데, 그 전에 `>` 를 만나면 tag 밖이다 — 앞선 tag 가 이미 닫혀 있다는
// 뜻이다. 커서가 선 글자 자체가 `>` 인 것은 예외다. 그 자리는 tag 의 마지막 글자다.
func (buf Buffer) tagAround(at scheme.Cursor) (htmlTagSpan, bool) {
	line := buf.Line(at.Line)
	if at.Col < 0 || at.Col >= len(line) {
		return htmlTagSpan{}, false
	}

	open := at
	for buf.Line(open.Line)[open.Col] != '<' {
		if open != at && buf.Line(open.Line)[open.Col] == '>' {
			return htmlTagSpan{}, false
		}

		prev, ok := buf.stepByte(open, scanBackward)
		if !ok {
			return htmlTagSpan{}, false
		}

		open = prev
	}

	tag, ok := buf.tagFrom(open)
	if !ok {
		return htmlTagSpan{}, false
	}

	// 찾은 tag 가 커서를 덮는지 본다. `<br> 글 >` 처럼 tag 가 커서 앞에서 이미 닫혔으면
	// 커서는 그 tag 안이 아니다.
	if before(tag.span.end, scheme.Cursor{Line: at.Line, Col: at.Col + 1}) {
		return htmlTagSpan{}, false
	}

	return tag, true
}

// tagFrom 은 `<` 자리에서 시작하는 tag 다. tag 가 아니거나 `>` 로 닫히지 않으면 false 다.
func (buf Buffer) tagFrom(open scheme.Cursor) (htmlTagSpan, bool) {
	name, _, closing, ok := syntax.HTMLTagAt(buf.Line(open.Line), open.Col)
	if !ok {
		return htmlTagSpan{}, false
	}

	closeAt, selfEnd, ok := buf.tagEnd(open)
	if !ok {
		return htmlTagSpan{}, false
	}

	span := pairSpan{start: open, end: scheme.Cursor{Line: closeAt.Line, Col: closeAt.Col + 1}}

	return htmlTagSpan{span: span, name: name, closing: closing, selfEnd: selfEnd}, true
}

// tagEnd 는 tag 를 닫는 `>` 자리와 그것이 `/>` 였는지다.
//
// 따옴표 안은 건너뛴다. `<div title="a > b">` 의 `>` 로 끝나지 않게 한다. 줄을 넘어 찾는다 —
// 속성을 줄마다 적은 tag 가 흔하다.
func (buf Buffer) tagEnd(open scheme.Cursor) (scheme.Cursor, bool, bool) {
	quote := byte(0)
	slash := false

	for at, ok := buf.stepByte(open, scanForward); ok; at, ok = buf.stepByte(at, scanForward) {
		ch := buf.Line(at.Line)[at.Col]

		if quote != 0 {
			if ch == quote {
				quote = 0
			}

			continue
		}

		switch ch {
		case '"', '\'':
			quote = ch
		case '<':
			// 닫히지 않은 채 다음 tag 가 열렸다. 그 앞은 tag 가 아니다.
			return scheme.Cursor{}, false, false
		case '>':
			return at, slash, true
		}

		slash = ch == '/'
	}

	return scheme.Cursor{}, false, false
}

// ── 자리 옮기기 ──

// scanDirection 은 훑는 쪽이다.
type scanDirection int

const (
	scanForward scanDirection = iota
	scanBackward
)

// stepByte 는 at 의 다음(또는 앞) byte 자리다. 줄을 넘어가고, 빈 줄은 건너뛴다.
// 파일 끝(또는 처음) 이면 false 다.
//
// **글자가 아니라 byte 다.** 짝으로 세는 글자가 전부 ASCII 라 UTF-8 이어지는 byte 는 어느
// 짝과도 같지 않다. 여기서 grapheme cluster 를 세면 파일 전체를 짝 하나 찾자고 훑게 된다.
func (buf Buffer) stepByte(at scheme.Cursor, dir scanDirection) (scheme.Cursor, bool) {
	if dir == scanBackward {
		if at.Col > 0 {
			return scheme.Cursor{Line: at.Line, Col: at.Col - 1}, true
		}

		for line := at.Line - 1; line >= 0; line-- {
			if length := len(buf.Line(line)); length > 0 {
				return scheme.Cursor{Line: line, Col: length - 1}, true
			}
		}

		return scheme.Cursor{}, false
	}

	if at.Col+1 < len(buf.Line(at.Line)) {
		return scheme.Cursor{Line: at.Line, Col: at.Col + 1}, true
	}

	for line := at.Line + 1; line < buf.LineCount(); line++ {
		if len(buf.Line(line)) > 0 {
			return scheme.Cursor{Line: line}, true
		}
	}

	return scheme.Cursor{}, false
}

// before 는 a 가 b 보다 앞인지다.
func before(a, b scheme.Cursor) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Col < b.Col)
}
