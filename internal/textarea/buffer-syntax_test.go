package textarea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluemir/zn/internal/syntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 언어 서버가 준 토큰을 줄에 얹는 자리를 보는 시험이다(buffer-syntax.go).
//
// **editor 를 지나지 않는다.** 옮기는 일(UTF-16 열을 byte 로, 토큰 이름을 색 갈래로) 은
// 그쪽이 하고(core/semantic.go) 여기는 이미 옮겨진 것을 받아 얹는 자리다 (ADR-0125, ADR-0129).

// semanticTestBuffer 는 강조를 한 번 훑어 둔 Go 파일이다.
func semanticTestBuffer(t *testing.T, lines ...string) *Viewport {
	t.Helper()

	path := filepath.Join(t.TempDir(), "main.go")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)
	buf.LexSyntaxTo(len(buf.Lines) - 1)

	return &buf
}

// tokenAt 은 그 줄에 얹힌 서버 토큰이다. 없으면 nil 이다.
func tokenAt(buf *Viewport, line int) []syntax.Token {
	return buf.syntax.Lines[line].semantic
}

// serverToken 은 서버가 준 토큰이 옮겨진 뒤의 꼴이다. 열은 이미 byte 다.
func serverToken(line, start, end int, kind syntax.Kind) SemanticToken {
	return SemanticToken{Line: line, Token: syntax.Token{Start: start, End: end, Kind: kind}}
}

// 다음 답에서 빠진 줄은 lexer 에게 돌아간다. 낡은 답이 그 줄에 굳으면 안 된다.
func TestSetSemanticTokensClearsRange(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	first := []SemanticToken{
		serverToken(0, 0, 7, syntax.KindKeyword),
		serverToken(2, 0, 3, syntax.KindKeyword),
	}
	buf.SetSemanticTokens(0, 3, first)
	require.NotNil(t, tokenAt(buf, 2))

	// 두 번째 답에는 3 번째 줄이 없다. 문법이 깨진 동안 이렇게 온다.
	buf.SetSemanticTokens(0, 3, first[:1])

	assert.Nil(t, tokenAt(buf, 2))
	assert.NotNil(t, tokenAt(buf, 0))
}

// 편집한 줄의 답은 그 자리에서 버려진다. 손대지 않은 줄은 그대로 남는다.
func TestEditDropsSemanticOnChangedLineOnly(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	buf.SetSemanticTokens(0, 3, []SemanticToken{
		serverToken(0, 0, 7, syntax.KindKeyword),
		serverToken(2, 0, 3, syntax.KindKeyword),
	})

	buf.ReplaceLines(2, 1, [][]byte{[]byte("var x = 33")})

	assert.Nil(t, tokenAt(buf, 2), "고친 줄은 lexer 로 돌아간다")
	assert.NotNil(t, tokenAt(buf, 0), "손대지 않은 줄은 그대로다")
}

// 창 밖이나 파일 밖을 가리키는 토큰은 버린다. 서버가 보던 판과 어긋난 답이 그렇다.
func TestSetSemanticTokensIgnoresOutOfRange(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	buf.SetSemanticTokens(0, 2, []SemanticToken{
		serverToken(2, 0, 3, syntax.KindKeyword),  // 창 밖이다
		serverToken(99, 0, 3, syntax.KindKeyword), // 파일 밖이다
	})

	for i := range buf.Lines {
		assert.Nil(t, tokenAt(buf, i), "줄 %d", i)
	}
}
