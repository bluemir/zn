package lsp

import (
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// FileEdits 는 파일 하나에 걸린 편집이다. rename 의 답이 이것들로 온다.
//
// 서버가 주는 것은 URI 인데 여기서 경로로 풀어서 준다 — 받는 쪽(core) 이 다루는 것이 경로다.
type FileEdits struct {
	Path  string
	Edits []TextEdit
}

// Rename 은 그 자리의 이름을 바꾸면 어디가 어떻게 달라지는지 묻는다.
// **고치지는 않는다.** 파일에 손대는 것은 받는 쪽의 일이다.
//
// 잰 값으로 664ms 다. 서버가 저장소 전체에서 그 이름을 쓰는 자리를 찾아야 해서 정의 찾기
// (15ms) 보다 한참 걸린다 — 그래서 부르는 쪽이 답을 기다리지 않고 Cmd 로 낸다(ADR-0067).
//
// 바꿀 수 없는 자리(예약어, 다른 모듈의 이름) 면 서버가 오류로 답하고 그 문구가 그대로 온다.
func (c *Client) Rename(path string, pos Position, newName string) ([]FileEdits, error) {
	result, err := c.conn.call("textDocument/rename", map[string]any{
		"textDocument": textDocumentIdentifier{URI: fileURI(path)},
		"position":     pos,
		"newName":      newName,
	})
	if err != nil {
		return nil, errors.Wrap(err, "이름을 바꾸지 못했다")
	}

	return parseWorkspaceEdit(result)
}

// parseWorkspaceEdit 는 rename 의 답을 읽는다.
//
// 규격이 두 모양을 허용한다 — `documentChanges`(파일마다 판이 붙은 목록) 와 `changes`(URI 를
// 키로 하는 map). **gopls 는 documentChanges 로 답한다**(잰 모든 경우에 그랬다). 그래도 둘 다
// 읽는 것은 남의 응답이라 우리 관측이 규격보다 좁을 이유가 없어서다(parseLocations 와 같다).
//
// `documentChanges` 에는 파일을 만들거나 지우거나 이름을 바꾸는 항목도 올 수 있다. 그것들은
// `edits` 가 없어서 여기서 걸러진다 — 파일 자체를 옮기는 rename 은 우리가 받지 않는다.
func parseWorkspaceEdit(result json.RawMessage) ([]FileEdits, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	var edit struct {
		DocumentChanges []struct {
			TextDocument versionedTextDocumentIdentifier `json:"textDocument"`
			Edits        []TextEdit                      `json:"edits"`
		} `json:"documentChanges"`
		Changes map[string][]TextEdit `json:"changes"`
	}
	if err := json.Unmarshal(result, &edit); err != nil {
		return nil, errors.Wrap(err, "이름 바꾸기 응답을 읽지 못했다")
	}

	files := []FileEdits{}

	for _, change := range edit.DocumentChanges {
		if len(change.Edits) == 0 {
			continue
		}

		files = append(files, FileEdits{
			Path:  Location{URI: change.TextDocument.URI}.Path(),
			Edits: change.Edits,
		})
	}

	for uri, edits := range edit.Changes {
		if len(edits) == 0 {
			continue
		}

		files = append(files, FileEdits{Path: Location{URI: uri}.Path(), Edits: edits})
	}

	return files, nil
}
