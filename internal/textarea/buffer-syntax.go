package textarea

import "github.com/bluemir/zn/internal/syntax"

// 문법 토큰을 채우는 것들이다. 담아 두는 그릇(syntaxCache) 은 syntax.go 에 있다 (ADR-0039).

// lexSyntaxTo 는 lastLine 까지의 토큰을 채운다. 그리는 쪽이 화면 맨 아래 줄을 알려준다.
//
// 줄 하나를 훑으려면 그 앞 줄을 끝낸 문맥이 필요해서, 담아둔 것이 없으면 위에서부터 내려온다.
func (buf *Buffer) LexSyntaxTo(lastLine int) {
	// 언어는 이름에서 이미 골라 두었다(buffer.go). 여기서는 표의 첫 칸을 꺼내 담을 뿐이다.
	// 모르는 언어면 nil 이고 훑을 것이 없다.
	buf.syntax.Start = buf.Language.State()
	if buf.syntax.Start == nil {
		return
	}

	// 줄 수와 어긋났으면 담아둔 것을 믿을 수 없다. 통째로 다시 훑는다. replaceLines 가 같이
	// 맞춰 주므로 여기 걸리는 것은 lines 를 다른 길로 바꾼 경우뿐이다 — 느려질 뿐 틀리지 않는다.
	if len(buf.syntax.Lines) != len(buf.Lines) {
		buf.syntax.Lines = make([]syntaxLine, len(buf.Lines))
		buf.syntax.valid, buf.syntax.Filled, buf.syntax.changedEnd = 0, 0, 0
	}

	// 화면 아래까지 이미 차 있으면 할 일이 없다.
	if buf.syntax.valid > lastLine {
		return
	}

	state := buf.syntax.Start
	if buf.syntax.valid > 0 {
		state = buf.syntax.Lines[buf.syntax.valid-1].after
	}

	for i := buf.syntax.valid; i < len(buf.Lines); i++ {
		before := buf.syntax.Lines[i].after
		tokens, after := state.Lex(buf.Lines[i])

		// 서버가 얹어둔 답(semantic) 은 건드리지 않는다. 그것을 비우는 자리는 그 줄의 글이
		// 갈리는 한 자리다(syntaxCache.replace).
		buf.syntax.Lines[i].tokens = tokens
		buf.syntax.Lines[i].after = after
		state = after

		// 고친 줄을 다 지난 뒤에, 이 줄을 끝낸 문맥이 전과 같으면 아래 줄들은 앞과 같은 문맥에서
		// 시작한다 — 앞서 훑어 둔 만큼은 담아둔 것이 그대로 맞다. 그래서 차 있던 자리까지
		// 되돌린다. **파일 끝까지가 아니다** — 훑지 않은 줄을 맞다고 하면 그 줄이 색 없이 그려진다.
		//
		// 아직 안 훑은 줄은 before 가 nil 이라 같아지지 않으므로, 처음 훑는 동안에는 걸리지 않는다.
		if i+1 >= buf.syntax.changedEnd && after == before {
			buf.syntax.valid = max(buf.syntax.Filled, i+1)
			buf.syntax.changedEnd = 0

			return
		}

		// 화면 아래는 지금 볼 사람이 없다. 굴러 내려갈 때 이어서 훑는다.
		// changedEnd 는 그대로 둔다 — 화면 밖에 고친 줄이 남아 있을 수 있다.
		if i >= lastLine {
			buf.syntax.valid = i + 1
			buf.syntax.Filled = max(buf.syntax.Filled, buf.syntax.valid)

			return
		}
	}

	buf.syntax.valid, buf.syntax.Filled, buf.syntax.changedEnd = len(buf.Lines), len(buf.Lines), 0
}

// syntaxStateAt 은 그 줄을 **시작한** 문맥이다. 아직 훑지 않은 줄이면 nil 이다.
//
// 담아 두는 것은 줄을 끝낸 문맥이라(syntaxLine.after) 앞 줄의 것을 꺼낸다. 첫 줄은 파일이
// 시작하는 문맥이다.
func (buf Buffer) syntaxStateAt(line int) syntax.State {
	if line <= 0 {
		return buf.syntax.Start
	}

	if line-1 >= len(buf.syntax.Lines) || line-1 >= buf.syntax.valid {
		return nil
	}

	return buf.syntax.Lines[line-1].after
}

// indentRuleAt 은 그 줄이 따르는 들여쓰기 규칙이다.
//
// **파일 이름이 아니라 문맥이 고른다.** markdown 코드펜스 안과 html 의 `<script>`·`<style>`
// 안은 안쪽 언어의 규칙을 받는다(syntax.State 의 Indent).
//
// 아직 훑지 않은 줄이면 파일 언어의 규칙이다. 부르는 자리가 전부 커서 줄이나 그 앞이고
// 먼저 lexSyntaxTo 를 지나므로 실제로 담아둔 것이 없는 때는 파일을 막 연 순간뿐이다.
func (buf Buffer) indentRuleAt(line int) syntax.Indent {
	if state := buf.syntaxStateAt(line); state != nil {
		return state.Indent()
	}

	return buf.Language.Indent()
}

// syntaxTokens 는 그 줄에 담아둔 토큰이다.
// 강조하지 않는 파일이거나 아직 훑지 않은 줄이면 nil 이다.
func (buf Buffer) SyntaxTokens(line int) []syntax.Token {
	// valid 이후는 아직 훑지 않은 줄이다. 화면 밖이라 그릴 사람이 없다.
	if line < 0 || line >= len(buf.syntax.Lines) || line >= buf.syntax.valid {
		return nil
	}

	// 언어 서버가 말한 줄은 서버가 정한다. 생김새로 어림잡은 답보다 type 검사를 마친 답이
	// 낫고, 서버가 없거나 아직 말하지 않은 줄에는 lexer 의 답이 남아 있다(ADR-0103).
	if semantic := buf.syntax.Lines[line].semantic; semantic != nil {
		return semantic
	}

	return buf.syntax.Lines[line].tokens
}

// setSemanticTokens 는 [from, to) 의 서버 답을 갈아끼운다. 열은 이미 byte 로 옮겨져 있다.
//
// **구간을 먼저 비운다.** 서버가 아무 말도 하지 않은 줄은 lexer 에게 돌려주어야 한다.
// 문법이 깨진 동안 서버의 답에서 빠지는 줄이 그렇다 — 낡은 답을 남겨 두면 그 줄만 지나간
// 글의 색으로 굳는다.
//
// 물을 때와 글이 달라졌는지는 부르는 쪽이 이미 본다(applySemanticTokens).
func (buf *Buffer) SetSemanticTokens(from, to int, tokens []SemanticToken) {
	from = max(0, from)
	to = min(len(buf.syntax.Lines), to)

	for line := from; line < to; line++ {
		buf.syntax.Lines[line].semantic = nil
	}

	for _, item := range tokens {
		if item.Line < from || item.Line >= to {
			continue
		}

		buf.syntax.Lines[item.Line].semantic = append(buf.syntax.Lines[item.Line].semantic, item.Token)
	}
}

// semanticToken 은 서버가 말한 색 한 조각이다. **열이 이미 byte 로 옮겨져 있다.**
//
// 창이 드는 자료라 `lsp.SemanticToken` 을 그대로 쓰지 않는다. 창은 어느 줄 어디에 무슨 색이
// 붙는지만 알면 되고, 그것을 언어 서버가 말했다는 것도 그 열이 UTF-16 이었다는 것도 알
// 필요가 없다 (ADR-0125).
type SemanticToken struct {
	Line  int
	Token syntax.Token
}
