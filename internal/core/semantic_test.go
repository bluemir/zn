package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
	"github.com/bluemir/zn/internal/syntax"
)

// 서버가 말한 갈래가 우리 갈래로 옮는다.
func TestSemanticKind(t *testing.T) {
	tests := []struct {
		name  string
		token lsp.SemanticToken
		want  syntax.Kind
	}{
		{name: "type", token: lsp.SemanticToken{Type: "type"}, want: syntax.KindType},
		{
			name:  "generic 인자도 type 이다",
			token: lsp.SemanticToken{Type: "typeParameter"},
			want:  syntax.KindType,
		},
		{
			name:  "package 이름도 type 색이다",
			token: lsp.SemanticToken{Type: "namespace"},
			want:  syntax.KindType,
		},
		{name: "함수", token: lsp.SemanticToken{Type: "function"}, want: syntax.KindFunction},
		{name: "메서드", token: lsp.SemanticToken{Type: "method"}, want: syntax.KindFunction},
		{name: "예약어", token: lsp.SemanticToken{Type: "keyword"}, want: syntax.KindKeyword},
		{name: "주석", token: lsp.SemanticToken{Type: "comment"}, want: syntax.KindComment},
		{name: "문자열", token: lsp.SemanticToken{Type: "string"}, want: syntax.KindString},
		{name: "숫자", token: lsp.SemanticToken{Type: "number"}, want: syntax.KindNumber},
		{
			name:  "선언하는 이름은 변수다",
			token: lsp.SemanticToken{Type: "variable", Modifiers: []string{"definition"}},
			want:  syntax.KindVariable,
		},
		{
			name:  "인자 이름도 그렇다",
			token: lsp.SemanticToken{Type: "parameter", Modifiers: []string{"definition"}},
			want:  syntax.KindVariable,
		},
		{
			name:  "struct 필드도 그렇다",
			token: lsp.SemanticToken{Type: "property", Modifiers: []string{"definition"}},
			want:  syntax.KindVariable,
		},
		{
			name:  "쓰는 자리는 칠하지 않는다",
			token: lsp.SemanticToken{Type: "variable"},
			want:  syntax.KindPlain,
		},
		{
			name:  "바뀌지 않는 이름은 상수다",
			token: lsp.SemanticToken{Type: "variable", Modifiers: []string{"readonly", "definition"}},
			want:  syntax.KindConstant,
		},
		{
			name:  "`nil` 처럼 미리 있는 것도 상수다",
			token: lsp.SemanticToken{Type: "variable", Modifiers: []string{"readonly", "defaultLibrary"}},
			want:  syntax.KindConstant,
		},
		{name: "연산자는 그리지 않는다", token: lsp.SemanticToken{Type: "operator"}, want: syntax.KindPlain},
		{name: "모르는 갈래도 그렇다", token: lsp.SemanticToken{Type: "지어낸것"}, want: syntax.KindPlain},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, semanticKind(test.token))
		})
	}
}

// 서버의 답이 줄에 얹히고, 그 줄은 lexer 대신 서버가 정한다.
func TestSetSemanticTokens(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var 이름 = 3")

	buf.setSemanticTokens(semanticTokensMsg{
		path:     buf.path,
		revision: buf.syntax.revision,
		from:     0,
		to:       3,
		tokens: []lsp.SemanticToken{
			{Line: 0, Start: 0, Length: 7, Type: "keyword"},
			{Line: 0, Start: 8, Length: 4, Type: "namespace"},
			// 한글은 UTF-16 으로 한 글자가 하나, byte 로는 셋이다.
			{Line: 2, Start: 4, Length: 2, Type: "variable", Modifiers: []string{"definition"}},
			{Line: 2, Start: 9, Length: 1, Type: "number"},
		},
	})

	assert.Equal(t, []syntax.Token{
		{Start: 0, End: 7, Kind: syntax.KindKeyword},
		{Start: 8, End: 12, Kind: syntax.KindType},
	}, buf.syntaxTokens(0))

	assert.Equal(t, []syntax.Token{
		{Start: 4, End: 10, Kind: syntax.KindVariable},
		{Start: 13, End: 14, Kind: syntax.KindNumber},
	}, buf.syntaxTokens(2), "한글 줄의 열이 byte 로 바뀌어야 한다")
}

// 물을 때와 글이 달라졌으면 버린다. 줄이 밀린 뒤에 얹으면 엉뚱한 자리에 색이 붙는다.
func TestSetSemanticTokensDropsStaleAnswer(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	asked := buf.syntax.revision

	// 답을 기다리는 사이에 위에 한 줄이 끼었다.
	buf.replaceLines(0, 0, [][]byte{[]byte("// 끼운 줄")})

	buf.setSemanticTokens(semanticTokensMsg{
		path:     buf.path,
		revision: asked,
		from:     0,
		to:       3,
		tokens:   []lsp.SemanticToken{{Line: 0, Start: 0, Length: 7, Type: "keyword"}},
	})

	assert.Nil(t, buf.syntax.lines[0].semantic, "낡은 답이 얹혔다")
}

// 다음 답에서 빠진 줄은 lexer 에게 돌아간다. 낡은 답이 그 줄에 굳으면 안 된다.
func TestSetSemanticTokensClearsRange(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	first := semanticTokensMsg{
		path:     buf.path,
		revision: buf.syntax.revision,
		from:     0,
		to:       3,
		tokens: []lsp.SemanticToken{
			{Line: 0, Start: 0, Length: 7, Type: "keyword"},
			{Line: 2, Start: 0, Length: 3, Type: "keyword"},
		},
	}
	buf.setSemanticTokens(first)
	require.NotNil(t, buf.syntax.lines[2].semantic)

	// 두 번째 답에는 3 번째 줄이 없다. 문법이 깨진 동안 이렇게 온다.
	second := first
	second.tokens = first.tokens[:1]
	buf.setSemanticTokens(second)

	assert.Nil(t, buf.syntax.lines[2].semantic)
	assert.NotNil(t, buf.syntax.lines[0].semantic)
}

// 편집한 줄의 답은 그 자리에서 버려진다. 손대지 않은 줄은 그대로 남는다.
func TestEditDropsSemanticOnChangedLineOnly(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	buf.setSemanticTokens(semanticTokensMsg{
		path:     buf.path,
		revision: buf.syntax.revision,
		from:     0,
		to:       3,
		tokens: []lsp.SemanticToken{
			{Line: 0, Start: 0, Length: 7, Type: "keyword"},
			{Line: 2, Start: 0, Length: 3, Type: "keyword"},
		},
	})

	buf.replaceLines(2, 1, [][]byte{[]byte("var x = 33")})

	assert.Nil(t, buf.syntax.lines[2].semantic, "고친 줄은 lexer 로 돌아간다")
	assert.NotNil(t, buf.syntax.lines[0].semantic, "손대지 않은 줄은 그대로다")
}

// 창 밖이나 파일 밖을 가리키는 토큰은 버린다. 서버가 보던 판과 어긋난 답이 그렇다.
func TestSetSemanticTokensIgnoresOutOfRange(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	buf.setSemanticTokens(semanticTokensMsg{
		path:     buf.path,
		revision: buf.syntax.revision,
		from:     0,
		to:       2,
		tokens: []lsp.SemanticToken{
			{Line: 2, Start: 0, Length: 3, Type: "keyword"},   // 창 밖이다
			{Line: 99, Start: 0, Length: 3, Type: "keyword"},  // 파일 밖이다
			{Line: 0, Start: 200, Length: 3, Type: "keyword"}, // 줄 끝 너머다
		},
	})

	for i := range buf.syntax.lines {
		assert.Nil(t, buf.syntax.lines[i].semantic, "줄 %d", i)
	}
}

// 서버가 없으면 묻지 않는다. Go 파일이 아닌 것도 그렇다.
func TestStartSemanticTokensWithoutServer(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("main.go")}, width: 80, height: 20}

	assert.Nil(t, e.startSemanticTokens())

	assert.Nil(t, (&editor{}).startSemanticTokens(), "볼 파일이 없으면 커서도 없다")
}

// 답은 물어본 파일의 buffer 로 간다. 답을 기다리는 사이에 tab 이 바뀌었을 수 있다.
func TestApplySemanticTokensPicksBufferByPath(t *testing.T) {
	first := semanticTestBuffer(t, "package main", "", "var x = 3")
	second := semanticTestBuffer(t, "package main", "", "var y = 4")

	e := &editor{buffers: []Buffer{*first, *second}, active: 0, width: 80, height: 20}

	e.applySemanticTokens(semanticTokensMsg{
		path:     second.path,
		revision: second.syntax.revision,
		from:     0,
		to:       3,
		tokens:   []lsp.SemanticToken{{Line: 0, Start: 0, Length: 7, Type: "keyword"}},
	})

	assert.Nil(t, e.buffers[0].syntax.lines[0].semantic)
	assert.NotNil(t, e.buffers[1].syntax.lines[0].semantic)
}

// 다시 읽기도 갈린 것으로 센다. 새 Buffer 라 세던 값이 0 으로 돌아가는 자리다.
func TestReloadKeepsSemanticRevisionMoving(t *testing.T) {
	buf := semanticTestBuffer(t, "package main", "", "var x = 3")

	asked := buf.syntax.revision

	require.NoError(t, os.WriteFile(buf.path, []byte("package main\n\nvar y = 4\n"), 0644))
	require.NoError(t, buf.Reload())

	buf.lexSyntaxTo(len(buf.lines) - 1)
	buf.setSemanticTokens(semanticTokensMsg{
		path:     buf.path,
		revision: asked,
		from:     0,
		to:       3,
		tokens:   []lsp.SemanticToken{{Line: 0, Start: 0, Length: 7, Type: "keyword"}},
	})

	assert.Nil(t, buf.syntax.lines[0].semantic, "다시 읽기 전에 물어둔 답이 얹혔다")
}

// 진짜 gopls 에게 물어 화면까지 닿는 길을 한 번에 본다.
//
// **이 시험만이 묻는 자리(startSemanticTokens) 를 지난다.** 창을 어디로 잡는지, 묻기 전에
// 맞추는지가 여기서 걸린다. 나머지 시험은 답이 있다고 치고 얹는 자리만 본다.
//
// 고른 글은 lexer 가 앞뒤 토큰만으로는 갈라내지 못하는 자리들이다 — generic 인자와 결과
// type 과 인자 이름이다(ADR-0103).
func TestSemanticTokensFromGopls(t *testing.T) {
	if testing.Short() {
		t.Skip("gopls 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module probe\n\ngo 1.24\n"), 0644))

	path := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(
		"package main\n"+
			"\n"+
			"type List[T any] struct {\n"+
			"\thandler func(ev int) error\n"+
			"}\n"+
			"\n"+
			"func (l *List[T]) Add(item T) (int, error) { return 0, nil }\n"), 0644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client, err := lsp.Start(ctx, dir, lsp.ServerFor("main.go"))
	if err != nil {
		t.Skip("gopls 가 없다: " + err.Error())
	}

	t.Cleanup(client.Shutdown)

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{buffers: []Buffer{buf}, width: 100, height: 30}
	e.serverState(lsp.ServerFor("main.go")).client = client
	require.NoError(t, client.Open(path, e.activeBuffer().lines))
	e.activeBuffer().lexSyntaxTo(len(e.activeBuffer().lines) - 1)

	cmd := e.startSemanticTokens()
	require.NotNil(t, cmd)

	msg, ok := cmd().(semanticTokensMsg)
	require.True(t, ok, "답이 오지 않았다")

	e.applySemanticTokens(msg)

	active := e.activeBuffer()
	kinds := map[string]syntax.Kind{}
	for line := range active.lines {
		for _, token := range active.syntaxTokens(line) {
			kinds[string(active.lines[line][token.Start:token.End])] = token.Kind
		}
	}

	assert.Equal(t, syntax.KindType, kinds["T"], "generic 인자다")
	assert.Equal(t, syntax.KindType, kinds["error"], "결과 type 이다")
	assert.Equal(t, syntax.KindVariable, kinds["handler"], "struct 필드 이름이다")
	assert.Equal(t, syntax.KindVariable, kinds["ev"], "인자 이름이다")
	assert.Equal(t, syntax.KindFunction, kinds["Add"], "메서드 이름이다")
	assert.Equal(t, syntax.KindConstant, kinds["nil"])
}

// semanticTestBuffer 는 그 줄들을 담은 buffer 다. 문법 캐시는 한 번 훑어서 채워 둔다.
func semanticTestBuffer(t *testing.T, lines ...string) *Buffer {
	t.Helper()

	path := filepath.Join(t.TempDir(), "main.go")

	text := ""
	for i, line := range lines {
		if i > 0 {
			text += "\n"
		}

		text += line
	}

	require.NoError(t, os.WriteFile(path, []byte(text+"\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.lexSyntaxTo(len(buf.lines) - 1)

	return &buf
}
