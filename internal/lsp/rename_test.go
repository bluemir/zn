package lsp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gopls 는 documentChanges 로 답한다. changes 로 오는 판도 읽는다.
func TestParseWorkspaceEditShapes(t *testing.T) {
	fromChanges, err := parseWorkspaceEdit([]byte(`{"documentChanges":[
		{"textDocument":{"uri":"file:///w/a.go","version":0},
		 "edits":[{"newText":"B","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":3}}}]}]}`))
	require.NoError(t, err)
	require.Len(t, fromChanges, 1)
	assert.Equal(t, "/w/a.go", fromChanges[0].Path)
	assert.Equal(t, "B", fromChanges[0].Edits[0].NewText)

	old, err := parseWorkspaceEdit([]byte(`{"changes":{"file:///w/b.go":[
		{"newText":"B","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}]}}`))
	require.NoError(t, err)
	require.Len(t, old, 1)
	assert.Equal(t, "/w/b.go", old[0].Path)

	none, err := parseWorkspaceEdit([]byte(`null`))
	require.NoError(t, err)
	assert.Empty(t, none)
}

// 파일을 만들거나 지우는 항목은 edits 가 없다. 파일 자체를 옮기는 것은 우리가 받지 않는다.
func TestParseWorkspaceEditSkipsFileOperations(t *testing.T) {
	files, err := parseWorkspaceEdit([]byte(`{"documentChanges":[
		{"kind":"create","uri":"file:///w/new.go"},
		{"textDocument":{"uri":"file:///w/a.go","version":0},"edits":[]}]}`))
	require.NoError(t, err)

	assert.Empty(t, files)
}

// 여기서는 진짜 gopls 에게 묻는다. **묻기만 한다** — 고치는 것은 core 의 일이라 이 시험은
// 저장소의 파일을 하나도 건드리지 않는다(ADR-0067).
func TestGoplsRename(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/tip.go")
	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	line, col := -1, -1
	for i, l := range lines {
		if idx := strings.Index(string(l), "func fitTip("); idx >= 0 {
			line, col = i, idx+len("func ")

			break
		}
	}
	require.GreaterOrEqual(t, line, 0, "`fitTip` 선언을 찾지 못했다")

	files, err := client.Rename(path, Position{Line: line, Character: col}, "pickTip")
	require.NoError(t, err)

	// 선언한 파일만이 아니라 부르는 파일까지 온다. 그것이 이 기능의 전부다.
	require.Greater(t, len(files), 1, "쓰는 자리가 여러 파일에 있다")

	paths := make([]string, 0, len(files))
	for _, file := range files {
		assert.NotEmpty(t, file.Edits, "빈 편집은 걸러진다")
		paths = append(paths, filepath.Base(file.Path))
	}

	assert.Contains(t, paths, "tip.go")
	assert.Contains(t, paths, "render-empty-screen.go", "부르는 쪽도 같이 온다")
}
