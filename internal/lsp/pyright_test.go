package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 여기서는 진짜 pyright 에게 묻는다. gopls 쪽과 같은 자리이고 같은 까닭이다 — 흉내로는
// 악수에서 무엇이 오는지, 열을 어떻게 세는지가 잡히지 않는다(ADR-0051, ADR-0107).
//
// pyright 가 없으면 건너뛴다. `-short` 로도 건너뛴다.

// startPythonForTest 는 python 파일 하나가 든 임시 자리에 pyright 를 띄운다.
//
// **저장소를 새로 만든다.** zn 자신의 뿌리에 띄우면 python 파일이 없어서 잴 것이 없고,
// pyright 가 Go 저장소 전체를 훑는 값만 든다.
func startPythonForTest(t *testing.T) (*Client, string) {
	t.Helper()

	if testing.Short() {
		t.Skip("pyright 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	root := t.TempDir()

	server := serverNamed("pyright")
	require.NotNil(t, server, "표에 pyright 줄이 있어야 한다")

	if _, err := server.find(root); err != nil {
		t.Skip("pyright 가 없다: " + err.Error())
	}

	path := filepath.Join(root, "app.py")
	require.NoError(t, os.WriteFile(path, []byte(
		"class Greeter:\n"+
			"    def __init__(self, name: str) -> None:\n"+
			"        self.name = name\n"+
			"\n"+
			"    def greet(self) -> str:\n"+
			"        return f\"안녕 {self.name}\"\n"+
			"\n"+
			"\n"+
			"greeter = Greeter(\"세상\")\n"+
			"print(greeter.greet())\n"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client, err := Start(ctx, root, server)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	return client, path
}

// `Greeter("세상")` 에서 물으면 그 class 를 선언한 자리로 간다.
//
// **한글이 든 파일이라는 것이 요점이다.** 열을 UTF-16 으로 세지 않으면 여기서 어긋난다.
func TestPyrightDefinition(t *testing.T) {
	client, path := startPythonForTest(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	use := findLine(t, lines, "greeter = Greeter(")
	column := UTF16Column(lines[use], indexIn(lines[use], "Greeter("))

	locations, err := client.Definition(path, Position{Line: use, Character: column})
	require.NoError(t, err)
	require.Len(t, locations, 1)

	assert.Equal(t, path, locations[0].Path())
	assert.Equal(t, 0, locations[0].Range.Start.Line, "`class Greeter` 줄이다")
}

// 사용처는 선언을 빼고 온다. Go 쪽과 같은 규칙이라 갈래를 하나만 두었다(lsp/references.go).
func TestPyrightReferences(t *testing.T) {
	client, path := startPythonForTest(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	declaration := findLine(t, lines, "class Greeter:")
	column := UTF16Column(lines[declaration], indexIn(lines[declaration], "Greeter"))

	locations, err := client.References(path, Position{Line: declaration, Character: column})
	require.NoError(t, err)
	assert.NotEmpty(t, locations, "쓰는 곳이 있다")
}

// 이름을 바꾸면 그 파일의 자리들이 온다. pyright 는 `documentChanges` 로 답한다(잰 값이다).
func TestPyrightRename(t *testing.T) {
	client, path := startPythonForTest(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	declaration := findLine(t, lines, "class Greeter:")
	column := UTF16Column(lines[declaration], indexIn(lines[declaration], "Greeter"))

	files, err := client.Rename(path, Position{Line: declaration, Character: column}, "Welcomer")
	require.NoError(t, err)
	require.Len(t, files, 1)

	assert.Equal(t, path, files[0].Path)
	assert.NotEmpty(t, files[0].Edits)
}

// 저장하지 않은 글로 답한다. 증분(`didChange`) 을 받는다는 것이 여기서 걸린다 —
// 재보니 pyright 의 `textDocumentSync` 가 2(증분) 다.
func TestPyrightAnswersFromUnsavedEdit(t *testing.T) {
	client, path := startPythonForTest(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	// 디스크에는 없는 함수를 buffer 에만 더한다.
	edited := append([][]byte{}, lines...)
	edited = append(edited, []byte(""), []byte("def 인사(): pass"), []byte("결과 = 인사()"))

	require.NoError(t, client.Sync(path, edited))

	// 부르는 줄을 구별되게 적는다. `인사()` 로 찾으면 `def 인사(): pass` 가 먼저 걸려서
	// 선언 자리에서 선언을 묻는 셈이 된다.
	call := findLine(t, edited, "결과 = 인사()")
	column := UTF16Column(edited[call], indexIn(edited[call], "인사"))

	locations, err := client.Definition(path, Position{Line: call, Character: column})
	require.NoError(t, err)
	require.NotEmpty(t, locations, "저장하지 않은 글에서 찾아야 한다")

	assert.Equal(t, call-1, locations[0].Range.Start.Line, "방금 더한 `def` 줄이다")
}

// 진단은 **빈 능력 그대로** 온다. gopls 와 같아서 능력을 채우지 않는다(ADR-0086, ADR-0107).
func TestPyrightPushesDiagnostics(t *testing.T) {
	if testing.Short() {
		t.Skip("pyright 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	root := t.TempDir()

	server := serverNamed("pyright")
	require.NotNil(t, server)

	if _, err := server.find(root); err != nil {
		t.Skip("pyright 가 없다: " + err.Error())
	}

	path := filepath.Join(root, "broken.py")
	require.NoError(t, os.WriteFile(path, []byte("print(없는이름)\n"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client, err := Start(ctx, root, server)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	require.NoError(t, client.Open(path, readLines(t, path)))

	got := waitForDiagnostics(t, client, path)
	require.NotEmpty(t, got)
	assert.Equal(t, SeverityError, got[0].Severity)
}

// **문법 토큰을 주지 않는다.** 재보니 악수 응답에 `semanticTokensProvider` 가 없다 —
// 능력을 알려도, `semanticTokens` 설정을 실어도 없었다.
//
// 이것을 시험으로 못 박아 두는 까닭은 **python 의 색이 우리 lexer 의 것**이라는 사실이
// 여기에 매달려 있어서다. 어느 판에서 토큰이 오기 시작하면 색을 내는 자리가 조용히 갈리고,
// 그때는 그 판을 보고 정할 일이다(ADR-0103, ADR-0107).
func TestPyrightHasNoSemanticTokens(t *testing.T) {
	client, path := startPythonForTest(t)

	require.Empty(t, client.semantic.types, "이름표가 오지 않는다")

	require.NoError(t, client.Open(path, readLines(t, path)))

	// 이름표가 비면 묻지 않고 곧바로 물러난다. 오류가 아니라 빈 답이다.
	tokens, err := client.SemanticTokens(path, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, tokens)
}
