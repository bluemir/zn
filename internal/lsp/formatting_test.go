package lsp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 줄을 넘는 편집을 뒤에서 앞으로 얹는다. 표본은 rust-analyzer 가 실제로 준 답이다.
func TestApplyTextEditsAcrossLines(t *testing.T) {
	in := []byte("async fn g(){ let y=2; }\nfn main(){}\n\n")
	edits := []TextEdit{
		{Range: Range{Start: Position{Line: 0, Character: 12}, End: Position{Line: 0, Character: 13}}, NewText: " {\n   "},
		{Range: Range{Start: Position{Line: 0, Character: 19}, End: Position{Line: 0, Character: 20}}, NewText: " = "},
		{Range: Range{Start: Position{Line: 0, Character: 22}, End: Position{Line: 0, Character: 23}}, NewText: "\n"},
		{Range: Range{Start: Position{Line: 1, Character: 9}, End: Position{Line: 1, Character: 9}}, NewText: " "},
		{Range: Range{Start: Position{Line: 2, Character: 0}, End: Position{Line: 3, Character: 0}}, NewText: ""},
	}

	assert.Equal(t, "async fn g() {\n    let y = 2;\n}\nfn main() {}\n", string(applyTextEdits(in, edits)))
}

// 열은 UTF-16 이다. 한글 뒤의 자리가 byte 로 옮아야 한다.
func TestApplyTextEditsCountsUTF16(t *testing.T) {
	in := []byte("fn 인사(){}\n")
	edits := []TextEdit{
		{Range: Range{Start: Position{Line: 0, Character: 7}, End: Position{Line: 0, Character: 7}}, NewText: " "},
	}

	assert.Equal(t, "fn 인사() {}\n", string(applyTextEdits(in, edits)))
}

// 글 밖을 가리키면 글 끝으로 당긴다. 서버가 보던 판과 어긋났을 때 panic 하지 않는다.
func TestApplyTextEditsClampsOutside(t *testing.T) {
	in := []byte("a\n")
	edits := []TextEdit{
		{Range: Range{Start: Position{Line: 9, Character: 0}, End: Position{Line: 9, Character: 0}}, NewText: "b"},
		{Range: Range{Start: Position{Line: 0, Character: 99}, End: Position{Line: 0, Character: 99}}, NewText: "!"},
	}

	assert.Equal(t, "a!\nb", string(applyTextEdits(in, edits)))
}
