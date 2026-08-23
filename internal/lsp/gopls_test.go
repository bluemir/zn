package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
