package lsp

import (
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// 서버에게 「이 자리가 무엇인가」를 묻는 자리다(ADR-0103).
//
// 우리 lexer 는 생김새로 어림잡는다 — 식별자 둘이 붙으면 뒤가 type 이라는 식이다. 서버는
// type 검사까지 마친 답을 준다. 그래서 generic 인자와 결과 type 처럼 앞뒤 토큰만으로는
// 갈리지 않는 자리가 여기서 답이 된다.

// SemanticToken 은 서버가 준 토큰 하나다.
//
// **Start 와 Length 는 UTF-16 코드 단위다**(protocol.go 의 Position). 받는 쪽이 그 줄의
// byte 자리로 바꾼다 — 줄의 내용을 아는 것은 편집기 쪽이다.
//
// Type 과 Modifiers 는 서버가 알린 이름 그대로다. 번호로 두지 않는 것은 그 번호가 서버가
// 그때 알린 표(legend) 안에서만 뜻이 있어서다. 이름으로 바꿔 두면 표를 들고 다닐 필요가 없다.
type SemanticToken struct {
	Line      int
	Start     int
	Length    int
	Type      string
	Modifiers []string
}

// semanticLegend 는 서버가 악수에서 알린 이름표다. 토큰의 번호를 이름으로 되돌리는 데 쓴다.
//
// 서버가 semantic token 을 주지 않으면 비어 있고, 그때 SemanticTokens 는 아무것도 묻지 않는다.
type semanticLegend struct {
	types     []string
	modifiers []string
}

// SemanticTokens 는 [fromLine, toLine) 줄의 토큰을 묻는다.
//
// **파일 전체를 묻지 않는다.** gopls v0.23.0 은 10 만 byte 가 넘는 파일에 `full` 로 물으면
// 오류가 아니라 **빈 답**을 준다(84 KB 는 14402 개, 103 KB 는 0 개였다). 큰 파일에서 조용히
// 색이 사라지는 길이라 창 단위로만 묻는다. 잰 값으로 300 KB 파일의 60 줄이 1 ms 다(ADR-0103).
func (c *Client) SemanticTokens(path string, fromLine, toLine int) ([]SemanticToken, error) {
	if len(c.semantic.types) == 0 {
		return nil, nil
	}

	result, err := c.conn.call("textDocument/semanticTokens/range", map[string]any{
		"textDocument": textDocumentIdentifier{URI: fileURI(path)},
		"range": Range{
			Start: Position{Line: fromLine, Character: 0},
			End:   Position{Line: toLine, Character: 0},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "문법 토큰을 묻지 못했다")
	}

	return c.semantic.parse(result)
}

// parse 는 응답을 토큰 목록으로 푼다.
//
// 규격이 정한 담는 법이 독특하다. 토큰 하나가 정수 다섯이고(줄 차이, 열 차이, 길이, 갈래,
// 수식어), **자리는 앞 토큰에서 얼마나 떨어졌는지**로 적힌다. 줄이 달라지면 열 차이는 줄
// 머리부터 세고, 같은 줄이면 앞 토큰의 열에 더한다.
func (legend semanticLegend) parse(result json.RawMessage) ([]SemanticToken, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	var answer struct {
		Data []int `json:"data"`
	}
	if err := json.Unmarshal(result, &answer); err != nil {
		return nil, errors.Wrap(err, "문법 토큰 응답을 읽지 못했다")
	}

	tokens := make([]SemanticToken, 0, len(answer.Data)/5)
	line, start := 0, 0

	for at := 0; at+4 < len(answer.Data); at += 5 {
		deltaLine, deltaStart := answer.Data[at], answer.Data[at+1]

		line += deltaLine
		if deltaLine == 0 {
			start += deltaStart
		} else {
			start = deltaStart
		}

		// 모르는 번호는 버린다. 서버가 알린 표에 없는 갈래를 낼 이유는 없지만, 낸다면
		// 그것은 우리가 이름을 모르는 것이라 갈래를 매길 수 없다.
		kind, ok := legend.name(answer.Data[at+3])
		if !ok {
			continue
		}

		tokens = append(tokens, SemanticToken{
			Line:      line,
			Start:     start,
			Length:    answer.Data[at+2],
			Type:      kind,
			Modifiers: legend.modifierNames(answer.Data[at+4]),
		})
	}

	return tokens, nil
}

// name 은 갈래 번호의 이름이다.
func (legend semanticLegend) name(index int) (string, bool) {
	if index < 0 || index >= len(legend.types) {
		return "", false
	}

	return legend.types[index], true
}

// modifierNames 는 수식어 비트들의 이름이다. n 번 비트가 표의 n 번째 이름이다.
//
// 하나도 없는 것이 흔해서(gopls 의 예약어·연산자가 그렇다) 그때는 아무것도 만들지 않는다.
func (legend semanticLegend) modifierNames(bits int) []string {
	if bits == 0 {
		return nil
	}

	names := []string{}
	for i, name := range legend.modifiers {
		if bits&(1<<i) != 0 {
			names = append(names, name)
		}
	}

	return names
}

// readSemanticLegend 는 악수 응답에서 이름표를 꺼낸다. 서버가 알리지 않으면 빈 표다.
func readSemanticLegend(result json.RawMessage) semanticLegend {
	var answer struct {
		Capabilities struct {
			SemanticTokensProvider struct {
				Legend struct {
					TokenTypes     []string `json:"tokenTypes"`
					TokenModifiers []string `json:"tokenModifiers"`
				} `json:"legend"`
			} `json:"semanticTokensProvider"`
		} `json:"capabilities"`
	}

	if err := json.Unmarshal(result, &answer); err != nil {
		return semanticLegend{}
	}

	legend := answer.Capabilities.SemanticTokensProvider.Legend

	return semanticLegend{types: legend.TokenTypes, modifiers: legend.TokenModifiers}
}
