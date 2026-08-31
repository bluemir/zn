package lsp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 시험이 쓰는 이름표다. gopls v0.23.0 이 알린 표의 앞부분과 같다.
var testLegend = semanticLegend{
	types:     []string{"namespace", "type", "typeParameter", "parameter", "property", "variable"},
	modifiers: []string{"definition", "readonly", "defaultLibrary"},
}

// 자리가 앞 토큰에서 얼마나 떨어졌는지로 적혀 온다. 줄이 바뀌면 열은 줄 머리부터 다시 센다.
func TestSemanticTokensParse(t *testing.T) {
	// 줄0 열0 길이4 type, 줄0 열9 길이1 typeParameter, 줄2 열4 길이5 property(선언)
	data := []int{
		0, 0, 4, 1, 0,
		0, 9, 1, 2, 0,
		2, 4, 5, 4, 1 << 0,
	}

	tokens, err := testLegend.parse(marshal(t, data))
	require.NoError(t, err)

	assert.Equal(t, []SemanticToken{
		{Line: 0, Start: 0, Length: 4, Type: "type"},
		{Line: 0, Start: 9, Length: 1, Type: "typeParameter"},
		{Line: 2, Start: 4, Length: 5, Type: "property", Modifiers: []string{"definition"}},
	}, tokens)
}

// 수식어는 비트다. n 번 비트가 표의 n 번째 이름이다.
func TestSemanticTokensModifiers(t *testing.T) {
	data := []int{0, 0, 3, 5, 1<<1 | 1<<2}

	tokens, err := testLegend.parse(marshal(t, data))
	require.NoError(t, err)

	require.Len(t, tokens, 1)
	assert.Equal(t, []string{"readonly", "defaultLibrary"}, tokens[0].Modifiers)
}

// 표에 없는 번호는 이름을 모르니 갈래를 매길 수 없다. 그 하나만 버리고 나머지는 읽는다.
//
// **자리 셈은 버린 토큰도 지난다.** 뒤 토큰의 자리가 그것에서부터 매겨지기 때문이다.
func TestSemanticTokensSkipsUnknownType(t *testing.T) {
	data := []int{
		0, 0, 4, 99, 0,
		0, 6, 2, 1, 0,
	}

	tokens, err := testLegend.parse(marshal(t, data))
	require.NoError(t, err)

	assert.Equal(t, []SemanticToken{{Line: 0, Start: 6, Length: 2, Type: "type"}}, tokens)
}

// 답이 비었거나 null 이면 토큰이 없다. 오류가 아니다 — 말할 것이 없는 파일이 있다.
func TestSemanticTokensEmptyAnswer(t *testing.T) {
	for _, raw := range []string{"null", `{"data":[]}`, ""} {
		tokens, err := testLegend.parse(json.RawMessage(raw))

		require.NoError(t, err)
		assert.Empty(t, tokens)
	}
}

// 이름표는 악수 응답에서 읽는다. 서버가 알리지 않으면 빈 표이고, 그러면 묻지 않는다.
func TestReadSemanticLegend(t *testing.T) {
	answer := `{"capabilities":{"semanticTokensProvider":{"legend":{
		"tokenTypes":["namespace","type"],"tokenModifiers":["definition"]}}}}`

	legend := readSemanticLegend(json.RawMessage(answer))

	assert.Equal(t, []string{"namespace", "type"}, legend.types)
	assert.Equal(t, []string{"definition"}, legend.modifiers)

	assert.Empty(t, readSemanticLegend(json.RawMessage(`{"capabilities":{}}`)).types)
	assert.Empty(t, readSemanticLegend(json.RawMessage(`깨진 것`)).types)
}

// 이름표가 없는 서버에게는 묻지 않는다. 물어도 읽을 수 없는 답이 온다.
func TestSemanticTokensWithoutLegend(t *testing.T) {
	client := &Client{}

	tokens, err := client.SemanticTokens("/없는/파일.go", 0, 10)

	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func marshal(t *testing.T, data []int) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(map[string]any{"data": data})
	require.NoError(t, err)

	return raw
}
