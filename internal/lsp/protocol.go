package lsp

import (
	"net/url"
	"strings"
)

// 여기 있는 것은 우리가 실제로 주고받는 자리뿐이다. LSP 규격 전체를 옮기지 않는다 —
// 쓰지 않는 필드는 서버가 보내도 버려지고, 늘어날 때 그 자리만 더한다.

// Position 은 파일 안의 한 자리다.
//
// **Character 는 UTF-16 코드 단위다.** byte 가 아니다. gopls v0.23.0 은 positionEncoding
// 협상을 받지 않아(빈 값으로 답한다) 규격 기본값인 UTF-16 으로 세야 한다 — byte 열로 물으면
// `column is beyond end of line` 로 거절당한다(ADR-0051 에서 잰 값이다).
//
// 그래서 이 자리를 만드는 쪽이 반드시 변환을 거친다. 이 패키지가 스스로 만드는 Position 은
// 줄 머리(Character 0) 와 줄 끝뿐이라 변환이 한 군데(utf16Len) 로 모인다.
type Position struct {
	Line      int `json:"line"`      // 0 부터 센다
	Character int `json:"character"` // UTF-16 코드 단위
}

// Range 는 두 자리 사이다. End 는 포함하지 않는다.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location 은 어느 파일의 어느 범위다. 정의로 뛰는 답이 이 목록으로 온다.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Path 는 Location 의 파일 경로다. `file://` 을 떼고 %XX 를 되돌린다.
//
// 되돌리지 못하면 적힌 그대로를 준다 — 열다 실패해서 "파일을 열 수 없다" 가 되는 것이,
// 조용히 빈 경로가 되어 아무 일도 안 나는 것보다 낫다.
func (l Location) Path() string {
	rest, ok := strings.CutPrefix(l.URI, "file://")
	if !ok {
		return l.URI
	}

	// 호스트 자리(`file://host/path`) 는 없다고 본다. 우리가 여는 것은 늘 이 기계의 파일이다.
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		return rest
	}

	return decoded
}

// fileURI 는 경로를 `file://` URI 로 만든다. 한글이 든 경로는 %XX 로 감싸진다.
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// textDocumentIdentifier 는 "어느 파일" 이다.
type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

// versionedTextDocumentIdentifier 는 didChange 가 쓰는, 판이 붙은 파일 이름이다.
//
// 판은 파일마다 1 부터 오르기만 한다. 서버는 이것으로 우리가 보낸 순서를 안다.
type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// textDocumentItem 은 didOpen 이 보내는 파일 전문이다.
type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// contentChange 는 didChange 의 한 조각이다.
//
// Range 가 nil 이면 Text 가 파일 전문이다 — 규격의 "전문 교체" 다. gopls 는 증분(change: 2)
// 을 광고하지만 전문 교체도 받아준다(ADR-0051 에서 재 보았다). 요청 직전에 이것으로
// 한 번 맞춘다.
type contentChange struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// utf16Len 은 줄 하나의 UTF-16 코드 단위 수다. 줄 끝 자리를 만들 때 쓴다.
//
// BMP 밖의 글자(이모지 등) 는 두 단위다. rune 수로 세면 그 줄에서 어긋난다.
func utf16Len(line []byte) int {
	n := 0
	for _, r := range string(line) {
		n++
		if r > 0xFFFF {
			n++
		}
	}

	return n
}
