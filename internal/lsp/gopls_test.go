package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 여기서는 진짜 gopls 에게 묻는다. 흉내로는 잡히지 않는 것들이 있다 —
// 악수에서 무엇을 요구하는지, 저장하지 않은 글로 답하는지, 열을 어떻게 세는지다(ADR-0051).
//
// gopls 가 없으면 건너뛴다. `-short` 로도 건너뛴다 — 첫 요청이 모듈을 훑느라 1 초 남짓 걸린다.
func startForTest(t *testing.T) (*Client, string) {
	t.Helper()

	if testing.Short() {
		t.Skip("gopls 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	if _, err := findGopls(); err != nil {
		t.Skip("gopls 가 없다: " + err.Error())
	}

	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client, err := Start(ctx, root)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	return client, root
}

// openBuffers 를 부르는 자리에서 물으면 그것을 선언한 자리로 간다.
func TestGoplsDefinition(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	call := findLine(t, lines, "buffers, err := openBuffers(files)")
	column := UTF16Column(lines[call], indexIn(lines[call], "openBuffers"))

	locations, err := client.Definition(path, Position{Line: call, Character: column})
	require.NoError(t, err)
	require.Len(t, locations, 1, "잰 모든 경우에 하나였다(ADR-0051)")

	assert.Equal(t, path, locations[0].Path())

	target := locations[0].Range.Start.Line
	assert.Contains(t, string(lines[target]), "func openBuffers")
}

// 저장하지 않은 글로 답한다. 이것이 되지 않으면 고치는 중에는 정의가 한 줄씩 어긋난다.
func TestGoplsDefinitionUsesUnsavedText(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	// 맨 위에 세 줄을 끼운다. 파일은 건드리지 않는다 — 우리 사본만 바뀐다.
	shifted := append([][]byte{
		[]byte("// 끼운 줄 1"),
		[]byte("// 끼운 줄 2"),
		[]byte("// 끼운 줄 3"),
	}, lines...)

	// 증분으로 보낸다. 평소 타이핑이 지나는 길이다.
	require.NoError(t, client.Sync(path, shifted))

	call := findLine(t, shifted, "buffers, err := openBuffers(files)")
	column := UTF16Column(shifted[call], indexIn(shifted[call], "openBuffers"))

	locations, err := client.Definition(path, Position{Line: call, Character: column})
	require.NoError(t, err)
	require.Len(t, locations, 1)

	// 정의도 세 줄 밀려 있어야 한다. 서버가 디스크를 봤으면 밀리지 않는다.
	target := locations[0].Range.Start.Line
	assert.Contains(t, string(shifted[target]), "func openBuffers")
	assert.Equal(t, findLine(t, shifted, "func openBuffers"), target)
}

// 저장하지 않고 새로 만든 함수도 찾는다. 증분으로 보낸 것이 서버 안에서 온전한 글이 되는지를 본다.
func TestGoplsDefinitionForNewFunction(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	// 파일 끝에 함수를 더하고 그것을 부르는 줄도 더한다.
	grown := append(copyLines(lines),
		[]byte(""),
		[]byte("func 방금만든함수() int { return 1 }"),
		[]byte(""),
		[]byte("var _ = 방금만든함수"),
	)
	require.NoError(t, client.Sync(path, grown))

	call := findLine(t, grown, "var _ = 방금만든함수")
	column := UTF16Column(grown[call], indexIn(grown[call], "방금만든함수"))

	locations, err := client.Definition(path, Position{Line: call, Character: column})
	require.NoError(t, err)
	require.Len(t, locations, 1)

	assert.Equal(t, findLine(t, grown, "func 방금만든함수"), locations[0].Range.Start.Line)
}

// 표준 라이브러리와 의존 모듈로도 뛴다. 그 파일들은 module cache 에 있어서 읽기 전용이다.
func TestGoplsDefinitionLeavesTheModule(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	tests := []struct {
		name     string
		line     string
		symbol   string
		contains string
	}{
		{name: "표준 라이브러리", line: "key, err := filepath.Abs(file)", symbol: "Abs", contains: "path/filepath"},
		{name: "의존 모듈", line: "if _, err := tea.NewProgram(", symbol: "NewProgram", contains: "bubbletea"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := findLine(t, lines, tt.line)
			column := UTF16Column(lines[at], indexIn(lines[at], tt.symbol))

			locations, err := client.Definition(path, Position{Line: at, Character: column})
			require.NoError(t, err)
			require.Len(t, locations, 1)

			assert.Contains(t, locations[0].Path(), tt.contains)
		})
	}
}

// 전문으로 맞추는 길도 실제로 먹는지 본다. 묻기 직전에 이것을 한 번 보낸다(ADR-0051).
func TestGoplsSyncFull(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	shifted := append([][]byte{[]byte("// 끼운 줄")}, lines...)
	require.NoError(t, client.SyncFull(path, shifted))

	call := findLine(t, shifted, "buffers, err := openBuffers(files)")
	column := UTF16Column(shifted[call], indexIn(shifted[call], "openBuffers"))

	locations, err := client.Definition(path, Position{Line: call, Character: column})
	require.NoError(t, err)
	require.Len(t, locations, 1)

	assert.Equal(t, findLine(t, shifted, "func openBuffers"), locations[0].Range.Start.Line)
}

// 정의가 없는 자리(주석 안) 는 빈 목록이다. 오류가 아니다.
func TestGoplsDefinitionInComment(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	at := findLine(t, lines, "// git 표시는 여기서 읽지 않는다")

	locations, err := client.Definition(path, Position{Line: at, Character: 10})
	require.NoError(t, err)
	assert.Empty(t, locations)
}

// 닫은 파일은 서버도 잊는다. tab 을 닫은 뒤 그 파일의 저장 안 된 편집이 남아 있으면 안 된다.
// 사용처를 물으면 쓰는 자리들이 오고 **선언 자리는 오지 않는다**.
//
// `includeDeclaration: false` 를 gopls 가 실제로 지키는지 여기서 잰다. 지키지 않으면
// 「사용처를 찾지 못했습니다」가 거짓말이 된다 — 아무도 안 쓰는 이름이 1 개로 오기 때문이다
// (ADR-0068).
func TestGoplsReferencesExcludesDeclaration(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	decl := findLine(t, lines, "func openBuffers(")
	column := UTF16Column(lines[decl], indexIn(lines[decl], "openBuffers"))

	locations, err := client.References(path, Position{Line: decl, Character: column})
	require.NoError(t, err)
	require.NotEmpty(t, locations, "부르는 자리가 있는 함수다")

	for _, at := range locations {
		if at.Path() == path {
			assert.NotEqual(t, decl, at.Range.Start.Line, "선언 자리는 목록에 없어야 한다")
		}
	}

	// 부르는 자리가 목록에 있다. 같은 파일 안의 그것이다.
	call := findLine(t, lines, "buffers, err := openBuffers(files)")
	found := false
	for _, at := range locations {
		if at.Path() == path && at.Range.Start.Line == call {
			found = true
		}
	}
	assert.True(t, found, "부르는 자리가 목록에 있어야 한다")
}

// 쓰는 자리에서 물어도 선언에서 물은 것과 같은 목록이 온다.
// 「커서를 어디에 두어야 하나」를 사용자가 고민하지 않아도 된다는 뜻이다.
func TestGoplsReferencesFromUseSite(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))

	decl := findLine(t, lines, "func openBuffers(")
	fromDecl, err := client.References(path, Position{
		Line:      decl,
		Character: UTF16Column(lines[decl], indexIn(lines[decl], "openBuffers")),
	})
	require.NoError(t, err)

	call := findLine(t, lines, "buffers, err := openBuffers(files)")
	fromCall, err := client.References(path, Position{
		Line:      call,
		Character: UTF16Column(lines[call], indexIn(lines[call], "openBuffers")),
	})
	require.NoError(t, err)

	assert.Equal(t, fromDecl, fromCall)
}

// 아무도 쓰지 않는 이름은 빈 목록이다. 화면의 「사용처를 찾지 못했습니다」가 이 자리다.
func TestGoplsReferencesForUnusedName(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/zz_unused_for_test.go")
	lines := splitLines("package core\n\nfunc nobodyCallsThisEver() {}")

	require.NoError(t, client.Open(path, lines))
	t.Cleanup(func() { _ = client.Close(path) })

	locations, err := client.References(path, Position{
		Line:      2,
		Character: UTF16Column(lines[2], indexIn(lines[2], "nobodyCallsThisEver")),
	})
	require.NoError(t, err)
	assert.Empty(t, locations)
}

func TestGoplsCloseForgets(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)

	require.NoError(t, client.Open(path, lines))
	assert.True(t, client.Tracks(path))

	require.NoError(t, client.Close(path))
	assert.False(t, client.Tracks(path))

	// 목록으로 닫는 길도 같다.
	require.NoError(t, client.Open(path, lines))
	client.CloseAllExcept(map[string]bool{})
	assert.False(t, client.Tracks(path))
}

func readLines(t *testing.T, path string) [][]byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	// core 의 Buffer 와 같은 모양으로 만든다 — 마지막 줄끝은 줄이 아니다.
	text := strings.TrimSuffix(string(data), "\n")

	return splitLines(text)
}

// findLine 은 그 글이 든 첫 줄이다. 없으면 시험을 세운다 — 파일이 바뀌어 자리를 잃은 것이다.
func findLine(t *testing.T, lines [][]byte, needle string) int {
	t.Helper()

	for i, line := range lines {
		if strings.Contains(string(line), needle) {
			return i
		}
	}

	t.Fatalf("%q 가 든 줄이 없다. 파일이 바뀌었으면 시험도 고쳐야 한다", needle)

	return -1
}

func indexIn(line []byte, needle string) int {
	return strings.Index(string(line), needle)
}

// 진단은 우리가 묻지 않아도 온다. 빈 capabilities 그대로다(ADR-0086).
//
// **임시 모듈에서 잰다.** 이 저장소를 뿌리로 쓰면 진단이 없어야 정상이라(빌드가 되는 코드다)
// 「오지 않는 것」과 「깨끗한 것」을 가를 수 없다.
func TestGoplsPublishesDiagnostics(t *testing.T) {
	client, path := startForTestInBrokenModule(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	got := waitForDiagnostics(t, client, path)

	messages := make([]string, 0, len(got))
	for _, item := range got {
		assert.Equal(t, SeverityError, item.Severity, "컴파일 오류는 severity 1 이다")
		messages = append(messages, item.Message)
	}

	assert.Contains(t, strings.Join(messages, "\n"), "undefined: undefinedThing")

	// 고치면 빈 목록이 와서 담아 둔 것이 사라진다.
	fixed := splitLines("package broken\n\nfunc main() {}\n")
	require.NoError(t, client.SyncFull(path, fixed))

	deadline := time.Now().Add(10 * time.Second)
	for len(client.Diagnostics(path)) > 0 && time.Now().Before(deadline) {
		select {
		case <-client.DiagnosticsChanged():
		case <-time.After(500 * time.Millisecond):
		}
	}

	assert.Empty(t, client.Diagnostics(path), "고친 파일은 빈 목록으로 지워진다")
}

// startForTestInBrokenModule 은 오류가 있는 파일 하나만 든 임시 모듈에 서버를 띄운다.
func startForTestInBrokenModule(t *testing.T) (*Client, string) {
	t.Helper()

	if testing.Short() {
		t.Skip("gopls 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	if _, err := findGopls(); err != nil {
		t.Skip("gopls 가 없다: " + err.Error())
	}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module broken\n\ngo 1.22\n"), 0o644))

	path := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package broken\n\nfunc main() {\n\tprintln(undefinedThing)\n}\n"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client, err := Start(ctx, root)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	return client, path
}

// waitForDiagnostics 는 그 파일의 진단이 올 때까지 기다린다. 종을 받아 담아 둔 것을 읽는다.
//
// 잰 값으로 didOpen 뒤 250ms 이고 모듈 첫 적재까지 합쳐 650ms 다(ADR-0086). 넉넉히 기다린다.
func waitForDiagnostics(t *testing.T, client *Client, path string) []Diagnostic {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if got := client.Diagnostics(path); len(got) > 0 {
			return got
		}

		select {
		case <-client.DiagnosticsChanged():
		case <-time.After(500 * time.Millisecond):
		}
	}

	t.Fatal("진단이 오지 않았다")

	return nil
}
