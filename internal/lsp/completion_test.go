package lsp

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 규격이 셋을 다 허용한다. gopls 는 잰 모든 경우에 목록 객체로 답했다.
func TestParseCompletionShapes(t *testing.T) {
	list, err := parseCompletion([]byte(`{"isIncomplete":true,"items":[{"label":"Contains"}]}`))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Contains", list[0].Label)

	array, err := parseCompletion([]byte(`[{"label":"Clone"}]`))
	require.NoError(t, err)
	require.Len(t, array, 1)
	assert.Equal(t, "Clone", array[0].Label)

	none, err := parseCompletion([]byte(`null`))
	require.NoError(t, err)
	assert.Empty(t, none)

	_, err = parseCompletion([]byte(`"?"`))
	assert.Error(t, err)
}

// 넣을 글자는 TextEdit → InsertText → Label 순이다.
func TestCompletionItemText(t *testing.T) {
	assert.Equal(t, "edit", CompletionItem{
		Label:      "label",
		InsertText: "insert",
		TextEdit:   &TextEdit{NewText: "edit"},
	}.Text())

	assert.Equal(t, "insert", CompletionItem{Label: "label", InsertText: "insert"}.Text())
	assert.Equal(t, "label", CompletionItem{Label: "label"}.Text())
}

// 여기서는 진짜 gopls 에게 묻는다. 빈 capabilities 로도 후보가 오는지와,
// **이미 친 접두를 TextEdit 범위가 덮는지**가 이 시험이 지키는 것이다(ADR-0066).
func TestGoplsCompletion(t *testing.T) {
	client, root := startForTest(t)

	path := filepath.Join(root, "internal/core/core.go")
	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	// 파일 끝에 접두를 친 줄을 붙이고 그 자리에서 묻는다.
	probe := append(append([][]byte{}, lines...), []byte("var _ = rand.IntN"))
	require.NoError(t, client.SyncFull(path, probe))

	items, err := client.Completion(path, Position{
		Line:      len(probe) - 1,
		Character: utf16Len(probe[len(probe)-1]),
	})
	require.NoError(t, err)
	require.NotEmpty(t, items, "빈 capabilities 로도 후보가 온다")

	first := items[0]
	assert.Equal(t, "IntN", first.Label)
	assert.NotEmpty(t, first.Detail, "곁들일 타입도 같이 온다")

	require.NotNil(t, first.TextEdit, "무엇을 지우고 넣을지 서버가 준다")
	assert.Equal(t, "IntN", first.Text())
	assert.Equal(t, len(probe)-1, first.TextEdit.Range.Start.Line)
	assert.Equal(t, len("var _ = rand."), first.TextEdit.Range.Start.Character,
		"이미 친 접두 `IntN` 을 범위가 덮는다")
	assert.Equal(t, len("var _ = rand.IntN"), first.TextEdit.Range.End.Character)
}
