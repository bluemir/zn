package lsp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitLines 는 테스트가 쓰는 줄 나누기다. core 의 Buffer 와 같은 모양으로 만든다 —
// 줄 내용에 줄바꿈이 없고 마지막 빈 줄은 없다.
func splitLines(s string) [][]byte {
	out := [][]byte{}
	for _, line := range strings.Split(s, "\n") {
		out = append(out, []byte(line))
	}

	return out
}

// applyChange 는 서버가 하는 일을 흉내낸다. 우리가 만든 변경을 옛 글에 얹어 새 글을 만든다.
//
// 이것으로 재는 것이 요점이다. 범위를 한 줄 어긋나게 잡아도 "변경을 만들었다" 는 것만 보면
// 통과하지만, 얹어 보면 글이 달라져서 걸린다 — 서버 안에서 조용히 어긋나던 것이 여기서 터진다.
func applyChange(old [][]byte, change contentChange) string {
	if change.Range == nil {
		return change.Text
	}

	text := text(old)
	start := offsetOf(old, change.Range.Start)
	end := offsetOf(old, change.Range.End)

	return text[:start] + change.Text + text[end:]
}

// offsetOf 는 자리 하나를 글 전체의 byte offset 으로 바꾼다.
func offsetOf(lines [][]byte, at Position) int {
	offset := 0
	for i := 0; i < at.Line && i < len(lines); i++ {
		offset += len(lines[i]) + 1 // 줄바꿈 한 칸
	}

	if at.Line < len(lines) {
		offset += ByteColumn(lines[at.Line], at.Character)
	}

	return offset
}

// diff 가 만든 변경을 얹으면 반드시 지금 글이 나와야 한다. 이것이 증분 동기화의 계약 전부다.
func TestDiffApplies(t *testing.T) {
	tests := []struct {
		name string
		old  string
		now  string
	}{
		{name: "가운데 한 줄을 고친다", old: "a\nb\nc", now: "a\nB\nc"},
		{name: "첫 줄을 고친다", old: "a\nb\nc", now: "A\nb\nc"},
		{name: "마지막 줄을 고친다", old: "a\nb\nc", now: "a\nb\nC"},
		{name: "가운데에 줄을 넣는다", old: "a\nb\nc", now: "a\nx\nb\nc"},
		{name: "맨 앞에 줄을 넣는다", old: "a\nb\nc", now: "x\na\nb\nc"},
		{name: "맨 뒤에 줄을 붙인다", old: "a\nb\nc", now: "a\nb\nc\nx"},
		{name: "맨 뒤에 여러 줄을 붙인다", old: "a\nb\nc", now: "a\nb\nc\nx\ny\nz"},
		{name: "가운데 줄을 지운다", old: "a\nb\nc", now: "a\nc"},
		{name: "첫 줄을 지운다", old: "a\nb\nc", now: "b\nc"},
		{name: "마지막 줄을 지운다", old: "a\nb\nc", now: "a\nb"},
		{name: "여러 줄을 한꺼번에 지운다", old: "a\nb\nc\nd\ne", now: "a\ne"},
		{name: "전부 갈아치운다", old: "a\nb\nc", now: "x\ny"},
		{name: "한 줄만 있던 것을 고친다", old: "a", now: "b"},
		{name: "한 줄에서 여러 줄이 된다", old: "a", now: "a\nb\nc"},
		{name: "여러 줄에서 한 줄이 된다", old: "a\nb\nc", now: "a"},
		{name: "빈 줄을 넣는다", old: "a\nb", now: "a\n\nb"},
		{name: "줄 하나를 통째로 비운다", old: "a\nb\nc", now: "a\n\nc"},
		{name: "한글이 든 줄을 고친다", old: "가나다\nb", now: "가나다라\nb"},
		{name: "한글 줄이 마지막일 때 고친다", old: "a\n가나다", now: "a\n가나다라마"},
		{name: "이모지가 든 마지막 줄", old: "a\n🙂", now: "a\n🙂🙂"},
		{name: "앞뒤가 같고 가운데만 다르다", old: "a\nb\nc\nd", now: "a\nX\nY\nd"},
		{name: "같은 줄이 되풀이되는 가운데를 고친다", old: "x\nx\nx\nx", now: "x\nx\ny\nx"},
		{name: "빈 파일에 글을 넣는다", old: "", now: "a"},
		{name: "글을 다 지운다", old: "a\nb", now: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old, now := splitLines(tt.old), splitLines(tt.now)

			change, changed := diff(old, now)
			require.True(t, changed, "달라졌는데 변경이 없다고 했다")

			assert.Equal(t, text(now), applyChange(old, change))
		})
	}
}

// 달라진 것이 없으면 아무것도 보내지 않는다. tick 이 주기적으로 도는데 그때마다 보내면
// 서버가 같은 글을 되풀이해 받는다.
func TestDiffNoChange(t *testing.T) {
	lines := splitLines("a\nb\nc")

	_, changed := diff(lines, copyLines(lines))
	assert.False(t, changed)

	// 사본이 아니라 같은 것을 넣어도 같다.
	_, changed = diff(lines, lines)
	assert.False(t, changed)
}

// 보내는 것이 실제로 「바뀐 줄만」 이어야 증분이라 할 수 있다. 천 줄 중 한 줄을 고쳤으면
// 그 한 줄만 나간다.
func TestDiffSendsOnlyChangedLines(t *testing.T) {
	old := make([][]byte, 0, 1000)
	for i := range 1000 {
		old = append(old, []byte(strings.Repeat("x", 40)+string(rune('a'+i%26))))
	}

	now := copyLines(old)
	now[500] = []byte("고친 줄")

	change, changed := diff(old, now)
	require.True(t, changed)
	require.NotNil(t, change.Range)

	assert.Equal(t, 500, change.Range.Start.Line)
	assert.Equal(t, 501, change.Range.End.Line)
	assert.Equal(t, "고친 줄\n", change.Text)
}

// text 는 파일 전문이다. 줄끝은 늘 LF 이고 마지막에도 하나 붙는다.
func TestText(t *testing.T) {
	assert.Equal(t, "a\nb\n", text(splitLines("a\nb")))
	assert.Equal(t, "\n", text(splitLines("")))
	assert.Equal(t, "가나다\n", text(splitLines("가나다")))
}

// copyLines 는 겉껍데기를 새로 만든다. 그러지 않으면 다음 편집이 사본까지 바꿔서 차이가 사라진다.
func TestCopyLinesIsIndependent(t *testing.T) {
	original := splitLines("a\nb\nc")
	copied := copyLines(original)

	// 편집은 줄을 갈아끼운다(core/edit.go 의 replaceLines). 그것을 흉내낸다.
	original[1] = []byte("B")

	change, changed := diff(copied, original)
	require.True(t, changed)
	assert.Equal(t, "B\n", change.Text)
}
