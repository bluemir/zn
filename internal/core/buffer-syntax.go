package core

import "github.com/bluemir/zn/internal/syntax"

// 문법 토큰을 채우는 것들이다. 담아 두는 그릇(syntaxCache) 은 syntax.go 에 있다 (ADR-0039).

// lexSyntaxTo 는 lastLine 까지의 토큰을 채운다. 그리는 쪽이 화면 맨 아래 줄을 알려준다.
//
// 줄 하나를 훑으려면 그 앞 줄을 끝낸 문맥이 필요해서, 담아둔 것이 없으면 위에서부터 내려온다.
func (buf *Buffer) lexSyntaxTo(lastLine int) {
	// 언어는 이름에서 이미 골라 두었다(buffer.go). 여기서는 표의 첫 칸을 꺼내 담을 뿐이다.
	// 모르는 언어면 nil 이고 훑을 것이 없다.
	buf.syntax.start = buf.language.State()
	if buf.syntax.start == nil {
		return
	}

	// 줄 수와 어긋났으면 담아둔 것을 믿을 수 없다. 통째로 다시 훑는다. replaceLines 가 같이
	// 맞춰 주므로 여기 걸리는 것은 lines 를 다른 길로 바꾼 경우뿐이다 — 느려질 뿐 틀리지 않는다.
	if len(buf.syntax.lines) != len(buf.lines) {
		buf.syntax.lines = make([]syntaxLine, len(buf.lines))
		buf.syntax.valid, buf.syntax.filled, buf.syntax.changedEnd = 0, 0, 0
	}

	// 화면 아래까지 이미 차 있으면 할 일이 없다.
	if buf.syntax.valid > lastLine {
		return
	}

	state := buf.syntax.start
	if buf.syntax.valid > 0 {
		state = buf.syntax.lines[buf.syntax.valid-1].after
	}

	for i := buf.syntax.valid; i < len(buf.lines); i++ {
		before := buf.syntax.lines[i].after
		tokens, after := state.Lex(buf.lines[i])

		buf.syntax.lines[i] = syntaxLine{tokens: tokens, after: after}
		state = after

		// 고친 줄을 다 지난 뒤에, 이 줄을 끝낸 문맥이 전과 같으면 아래 줄들은 앞과 같은 문맥에서
		// 시작한다 — 앞서 훑어 둔 만큼은 담아둔 것이 그대로 맞다. 그래서 차 있던 자리까지
		// 되돌린다. **파일 끝까지가 아니다** — 훑지 않은 줄을 맞다고 하면 그 줄이 색 없이 그려진다.
		//
		// 아직 안 훑은 줄은 before 가 nil 이라 같아지지 않으므로, 처음 훑는 동안에는 걸리지 않는다.
		if i+1 >= buf.syntax.changedEnd && after == before {
			buf.syntax.valid = max(buf.syntax.filled, i+1)
			buf.syntax.changedEnd = 0

			return
		}

		// 화면 아래는 지금 볼 사람이 없다. 굴러 내려갈 때 이어서 훑는다.
		// changedEnd 는 그대로 둔다 — 화면 밖에 고친 줄이 남아 있을 수 있다.
		if i >= lastLine {
			buf.syntax.valid = i + 1
			buf.syntax.filled = max(buf.syntax.filled, buf.syntax.valid)

			return
		}
	}

	buf.syntax.valid, buf.syntax.filled, buf.syntax.changedEnd = len(buf.lines), len(buf.lines), 0
}

// syntaxTokens 는 그 줄에 담아둔 토큰이다.
// 강조하지 않는 파일이거나 아직 훑지 않은 줄이면 nil 이다.
func (buf Buffer) syntaxTokens(line int) []syntax.Token {
	// valid 이후는 아직 훑지 않은 줄이다. 화면 밖이라 그릴 사람이 없다.
	if line < 0 || line >= len(buf.syntax.lines) || line >= buf.syntax.valid {
		return nil
	}

	return buf.syntax.lines[line].tokens
}
