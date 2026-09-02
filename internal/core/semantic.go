package core

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/lsp"
	"github.com/bluemir/zn/internal/syntax"
)

// 언어 서버가 준 문법 토큰을 화면에 얹는 자리다(ADR-0103).
//
// 서버 쪽 이야기는 internal/lsp 가 안다. 여기 있는 것은 「언제 무엇을 묻고, 받은 것을 어느
// 갈래로 그리는가」다.
//
// lexer 를 대신하지 않는다. 서버가 없거나 아직 답하지 않은 줄은 lexer 의 답이 그대로 남고,
// 서버가 말한 줄만 갈아끼운다(buffer-syntax.go 의 syntaxTokens).

// semanticMargin 은 화면 위아래로 더 묻는 줄 수다.
//
// 보이는 만큼만 물으면 한 줄만 굴려도 새로 보이는 줄이 답을 기다리는 동안 색이 튄다.
// 감싸는 머리줄(ADR-0049) 이 화면 위의 줄을 그리는 것도 이 여유가 받는다. 300 KB 파일에서
// 60 줄이 1 ms 라 이만큼 더 물어도 값이 눈에 띄지 않는다.
const semanticMargin = 100

// semanticTokensMsg 는 물어본 답이다. 어느 파일의 어느 구간인지를 같이 싣는다 —
// 답이 오는 사이에 tab 이 바뀌거나 화면이 굴러갔을 수 있다.
type semanticTokensMsg struct {
	path     string
	revision int
	from     int
	to       int
	tokens   []lsp.SemanticToken
}

// startSemanticTokens 는 지금 보고 있는 창의 토큰을 묻는다.
//
// 부르는 자리는 타이핑이 멎었을 때 하나다(job.go 의 editTickMsg). 키마다 묻지 않는 것은
// 서버와 맞추는 일이 이미 그 자리에 모여 있어서이고(ADR-0051), 이동 키도 그 tick 을
// 예약하므로 굴러간 창도 같은 자리에서 다시 묻는다.
func (e *editor) startSemanticTokens() tea.Cmd {
	if !e.hasTab() {
		return nil
	}

	buf := e.activeBuffer()

	server, path, ok := serverPath(buf.path)
	if !ok {
		return nil
	}

	client := e.clientOf(server)
	if client == nil {
		return nil
	}

	rows := buf.visibleRows(e.textHeight())
	if len(rows) < 1 {
		return nil
	}

	from := max(0, rows[0].line-semanticMargin)
	to := min(len(buf.lines), rows[len(rows)-1].line+1+semanticMargin)
	revision := buf.syntaxRevision()
	lines := buf.lines

	return func() tea.Msg {
		// 서버가 아직 이 파일을 모르면 묻지 않는다. 여는 것은 맞추는 자리의 몫이고
		// (syncServers) 그것이 끝나면 다음 tick 이 다시 묻는다.
		if !client.Tracks(path) {
			return nil
		}

		// 묻기 전에 맞춘다. **여기서 맞추는 것이 순서를 정한다** — 맞추는 일과 묻는 일이
		// 각자의 goroutine 이라, 이 한 줄이 없으면 서버가 아직 옛 글을 든 채로 답할 수 있고
		// 그 답은 지금 화면과 어긋난 자리에 색을 얹는다. 이미 맞았으면 아무것도 보내지
		// 않는다(lsp 의 Sync).
		_ = client.Sync(path, lines)

		tokens, err := client.SemanticTokens(path, from, to)
		if err != nil {
			// 조용히 물러난다. 색이 lexer 의 답으로 남을 뿐이고, 서버가 죽은 것은 맞추는
			// 자리가 알아채서 다시 띄운다(ADR-0092). 250ms 마다 알림을 쌓을 일이 아니다.
			return nil
		}

		return semanticTokensMsg{path: path, revision: revision, from: from, to: to, tokens: tokens}
	}
}

// semanticToken 은 서버가 말한 색 한 조각이다. **열이 이미 byte 로 옮겨져 있다.**
//
// 창이 드는 자료라 `lsp.SemanticToken` 을 그대로 쓰지 않는다. 창은 어느 줄 어디에 무슨 색이
// 붙는지만 알면 되고, 그것을 언어 서버가 말했다는 것도 그 열이 UTF-16 이었다는 것도 알
// 필요가 없다 (ADR-0125).
type semanticToken struct {
	line  int
	token syntax.Token
}

// applySemanticTokens 는 받은 답을 그 파일의 줄들에 얹는다.
//
// **여기서 창의 말로 옮긴다.** 서버의 열은 UTF-16 이라 byte 로 바꿔야 하는데(lsp/position.go)
// 그러려면 그 줄의 글자가 필요하고, 그 줄을 든 것이 창이다. 그래서 창을 손에 쥔 이 자리가
// 옮기기에 맞다 — 창 안에서 하면 창이 lsp 를 알게 된다 (ADR-0125).
func (e *editor) applySemanticTokens(msg semanticTokensMsg) {
	for i := range e.buffers {
		_, path, ok := serverPath(e.buffers[i].path)
		if !ok || path != msg.path {
			continue
		}

		buf := &e.buffers[i]

		// **물을 때와 글이 달라졌으면 버린다.** 답은 뒤늦게 오고 그 사이의 한 글자가 줄 자리를
		// 밀어 두었을 수 있다. 그때 얹으면 엉뚱한 줄에 색이 붙는다 — 버리면 lexer 의 답으로 한
		// 박자 남았다가 다음 tick 이 다시 묻는다(syntaxCache.revision).
		if buf.syntaxRevision() != msg.revision {
			return
		}

		tokens := make([]semanticToken, 0, len(msg.tokens))
		for _, token := range msg.tokens {
			if token.Line < 0 || token.Line >= len(buf.lines) {
				continue
			}

			kind := semanticKind(token)
			if kind == syntax.KindPlain {
				// 강조하지 않는 자리는 토큰을 내지 않는다(syntax.go 의 KindPlain). 연산자와
				// 이름표가 여기서 걸러진다.
				continue
			}

			// 서버의 열은 UTF-16 이라 byte 로 바꾼다(lsp/position.go).
			line := buf.lines[token.Line]
			start := lsp.ByteColumn(line, token.Start)
			end := lsp.ByteColumn(line, token.Start+token.Length)

			if start >= end {
				continue
			}

			tokens = append(tokens, semanticToken{
				line:  token.Line,
				token: syntax.Token{Start: start, End: end, Kind: kind},
			})
		}

		buf.setSemanticTokens(msg.from, msg.to, tokens)

		return
	}
}

// semanticKind 는 서버가 말한 갈래를 우리 갈래로 옮긴다. 그릴 것이 없으면 KindPlain 이다.
//
// 이름은 서버가 악수에서 알린 표의 것이다(lsp/semantic.go). LSP 규격이 정한 이름이라
// 서버가 달라도 같은 표로 읽힌다 — pyright 는 이 표를 쓰지 않는다(문법 토큰을 내지 않는다, ADR-0107).
func semanticKind(token lsp.SemanticToken) syntax.Kind {
	switch token.Type {
	case "type", "typeParameter", "namespace":
		// package 이름도 type 색이다. `syntax.State` 처럼 앞에 붙는 이름은 그 뒤의 type 과
		// 한 덩어리로 읽히고, `errors.New` 의 `errors` 도 같은 이름이라 같이 칠한다.
		return syntax.KindType

	case "function", "method", "macro":
		return syntax.KindFunction

	case "keyword":
		return syntax.KindKeyword

	case "comment":
		return syntax.KindComment

	case "string":
		return syntax.KindString

	case "number":
		return syntax.KindNumber

	case "variable", "parameter", "property":
		// 값이 바뀌지 않는 이름은 상수다. `nil`·`true`·`iota` 와 `const` 로 선언한 것이
		// 여기 든다 — lexer 가 이름 표로 잡던 것과 같은 갈래다(syntax/go.go).
		if slices.Contains(token.Modifiers, "readonly") {
			return syntax.KindConstant
		}

		// **이름을 붙이는 자리만 칠한다.** 그 이름을 쓰는 자리까지 칠하면 Go 소스의 거의 모든
		// 식별자가 한 색이 되어 색이 구조를 짚어 주지 못한다. 선언은 파일에 한 번뿐이라
		// 눈이 「여기서 생겼다」를 짚을 자국이 된다.
		if slices.Contains(token.Modifiers, "definition") {
			return syntax.KindVariable
		}
	}

	return syntax.KindPlain
}
