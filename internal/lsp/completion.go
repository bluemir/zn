package lsp

import (
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// CompletionItem 은 서버가 준 후보 하나다.
//
// **규격의 칸을 다 옮기지 않는다.** 여기 든 것은 그리는 데(Label·Detail) 와 넣는 데
// (TextEdit·InsertText) 와 줄 세우는 데(SortText) 쓰는 것뿐이다.
//
// `documentation` 은 일부러 받지 않는다. 한 후보에 수백 바이트인데 우리 목록은 한 줄짜리라
// 그릴 자리가 없다 — 재 보면 `rand.` 뒤 스물여덟 후보의 답이 11KB 이고 그 대부분이 그것이다.
type CompletionItem struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`

	// Kind 는 갈래다(함수 3, 필드 5, 변수 6…). 규격의 번호를 그대로 든다 —
	// 이름을 붙이는 것은 그리는 쪽의 일이고 그쪽이 core 에 있다(core/completion.go).
	Kind int `json:"kind"`

	SortText   string `json:"sortText"`
	InsertText string `json:"insertText"`

	// TextEdit 은 「이 범위를 이 글자로 바꿔라」다. 접두를 이미 친 자리에서 그 접두를 덮는
	// 범위로 온다(`rand.IntN` 에서 `IntN` 넉 자를 덮었다). 그래서 우리가 접두 길이를 세지
	// 않는다 — 무엇을 지우고 무엇을 넣을지는 서버가 이미 알고 있다.
	TextEdit *TextEdit `json:"textEdit"`
}

// TextEdit 은 범위 하나를 글자로 바꾸는 것이다.
type TextEdit struct {
	NewText string `json:"newText"`
	Range   Range  `json:"range"`
}

// Text 는 이 후보를 넣을 때 쓸 글자다.
//
// 순서는 규격이 정한 대로 TextEdit → InsertText → Label 이다. gopls 는 늘 TextEdit 을 주지만
// (잰 모든 경우에 그랬다) 남의 응답이라 우리 관측이 규격보다 좁을 이유가 없다.
func (item CompletionItem) Text() string {
	switch {
	case item.TextEdit != nil:
		return item.TextEdit.NewText
	case item.InsertText != "":
		return item.InsertText
	default:
		return item.Label
	}
}

// Completion 은 그 자리에서 이어 칠 수 있는 것을 묻는다.
//
// pos 의 열은 UTF-16 코드 단위다(protocol.go 의 Position). 부르는 쪽이 이미 바꿔서 준다.
//
// 잰 값으로 첫 요청이 19ms, 이어지는 요청은 1ms 아래다 — 서버가 그 자리의 답을 들고 있다.
func (c *Client) Completion(path string, pos Position) ([]CompletionItem, error) {
	result, err := c.conn.call("textDocument/completion", map[string]any{
		"textDocument": textDocumentIdentifier{URI: fileURI(path)},
		"position":     pos,
	})
	if err != nil {
		return nil, errors.Wrap(err, "완성 후보를 묻지 못했다")
	}

	return parseCompletion(result)
}

// parseCompletion 은 완성 응답을 읽는다.
//
// 규격이 `CompletionItem` 목록, `CompletionList`(목록에 `isIncomplete` 가 붙은 것), `null`
// 셋을 다 허용한다. gopls 는 잰 모든 경우에 `CompletionList` 로 답했다.
//
// **`isIncomplete` 는 버린다.** 「더 치면 다시 물어라」인데 우리는 어차피 글자마다 다시
// 묻는다(core/completion.go). 들고 있어 봐야 쓰는 자리가 없다.
func parseCompletion(result json.RawMessage) ([]CompletionItem, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	var list struct {
		Items []CompletionItem `json:"items"`
	}
	if err := json.Unmarshal(result, &list); err == nil && list.Items != nil {
		return list.Items, nil
	}

	var items []CompletionItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, errors.Wrap(err, "완성 응답을 읽지 못했다")
	}

	return items, nil
}
