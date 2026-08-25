package lsp

import (
	"github.com/cockroachdb/errors"
)

// References 는 그 자리의 이름을 **쓰는 자리**들을 묻는다. 사용처로 가기의 답이다(ADR-0068).
//
// pos 의 열은 UTF-16 코드 단위다(protocol.go 의 Position). 부르는 쪽이 이미 바꿔서 준다.
//
// `includeDeclaration` 을 거짓으로 둔다 — 선언 자리는 정의로 가기(`\gd`) 가 따로 있어서
// 이 목록에 섞으면 갈래가 둘인 목록이 된다.
//
// rename 과 같은 일을 서버가 한다(저장소 전체에서 그 이름을 쓰는 자리 찾기) 라 걸리는 시간도
// 그쪽에 가깝다 — 정의 찾기(15ms) 가 아니라 수백 ms 다. 부르는 쪽이 답을 기다리지 않는다.
func (c *Client) References(path string, pos Position) ([]Location, error) {
	result, err := c.conn.call("textDocument/references", map[string]any{
		"textDocument": textDocumentIdentifier{URI: fileURI(path)},
		"position":     pos,
		"context":      map[string]any{"includeDeclaration": false},
	})
	if err != nil {
		return nil, errors.Wrap(err, "사용처를 묻지 못했다")
	}

	// 답의 모양이 정의와 같다 — Location 목록이다.
	return parseLocations(result)
}
