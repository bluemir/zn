package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitDiffLines 는 시험이 읽기 좋게 줄을 늘어놓는다.
func splitDiffLines(text string) [][]byte {
	if text == "" {
		return nil
	}

	parts := strings.Split(text, "\n")

	lines := make([][]byte, len(parts))
	for i := range parts {
		lines[i] = []byte(parts[i])
	}

	return lines
}

// kinds 는 조각 안 자리들의 갈래다. 시험이 견주기 좋은 모양이다.
func kinds(hunk diffHunk) []diffRowKind {
	out := make([]diffRowKind, 0, len(hunk.rows))
	for _, row := range hunk.rows {
		out = append(out, row.kind)
	}

	return out
}

// 같으면 조각이 없다. 빈 판을 띄우지 않는 근거가 이 답이다.
func TestDiffNoChange(t *testing.T) {
	lines := splitDiffLines("a\nb\nc")

	assert.Empty(t, buildDiffHunks(lines, lines))
}

// 한 줄을 고치면 그 자리가 diffRowChanged 다. 지우기와 넣기 둘로 갈리지 않는다.
func TestDiffChangedLine(t *testing.T) {
	hunks := buildDiffHunks(
		splitDiffLines("a\nb\nc"),
		splitDiffLines("a\nB\nc"),
	)

	require.Len(t, hunks, 1)
	assert.Equal(t, []diffRowKind{diffRowSame, diffRowChanged, diffRowSame}, kinds(hunks[0]))

	changed := hunks[0].rows[1]
	assert.Equal(t, 1, changed.left.line)
	assert.Equal(t, 1, changed.right.line)
}

// 넣기만 한 줄은 왼쪽이 비어 있다. 그 자리가 side-by-side 의 filler 다.
func TestDiffAddedLine(t *testing.T) {
	hunks := buildDiffHunks(
		splitDiffLines("a\nc"),
		splitDiffLines("a\nb\nc"),
	)

	require.Len(t, hunks, 1)
	assert.Equal(t, []diffRowKind{diffRowSame, diffRowAdded, diffRowSame}, kinds(hunks[0]))
	assert.False(t, hunks[0].rows[1].left.exists())
	assert.True(t, hunks[0].rows[1].right.exists())
}

// 지운 줄은 오른쪽이 비어 있다. 마커 칸과 달리 **그 줄이 그 자리에 그려진다**(ADR-0140 §6).
func TestDiffRemovedLine(t *testing.T) {
	hunks := buildDiffHunks(
		splitDiffLines("a\nb\nc"),
		splitDiffLines("a\nc"),
	)

	require.Len(t, hunks, 1)
	assert.Equal(t, []diffRowKind{diffRowSame, diffRowRemoved, diffRowSame}, kinds(hunks[0]))
	assert.True(t, hunks[0].rows[1].left.exists())
	assert.False(t, hunks[0].rows[1].right.exists())
}

// 견줄 앞이 없으면 전부 넣은 줄이다. 뿌리 커밋과 git 밖의 새 파일이 이 자리다.
func TestDiffAgainstNothing(t *testing.T) {
	hunks := buildDiffHunks(nil, splitDiffLines("a\nb"))

	require.Len(t, hunks, 1)
	assert.Equal(t, []diffRowKind{diffRowAdded, diffRowAdded}, kinds(hunks[0]))
}

// 문맥은 앞뒤 셋이다. 그 바깥은 조각에 들지 않는다.
func TestDiffContextIsThree(t *testing.T) {
	before := splitDiffLines("1\n2\n3\n4\n5\n6\n7\n8\n9")
	after := splitDiffLines("1\n2\n3\n4\nX\n6\n7\n8\n9")

	hunks := buildDiffHunks(before, after)

	require.Len(t, hunks, 1)
	assert.Len(t, hunks[0].rows, diffContext*2+1)
	assert.Equal(t, 2, hunks[0].leftStart, "머리줄의 시작 줄은 1 부터다")
	assert.Equal(t, 7, hunks[0].leftCount)
}

// 가까운 변경 둘은 한 조각이다. 겹친 문맥이 같은 줄을 두 번 그리게 두지 않는다.
func TestDiffMergesNearbyHunks(t *testing.T) {
	before := splitDiffLines("1\n2\n3\n4\n5\n6\n7")
	after := splitDiffLines("X\n2\n3\n4\n5\n6\nY")

	hunks := buildDiffHunks(before, after)

	require.Len(t, hunks, 1, "문맥이 겹치면 이어 붙인다")
	assert.Len(t, hunks[0].rows, 7)
}

// 먼 변경 둘은 조각 둘이다.
func TestDiffSplitsFarHunks(t *testing.T) {
	before := splitDiffLines(strings.Repeat("x\n", 30) + "end")
	after := splitDiffLines("A\n" + strings.Repeat("x\n", 28) + "B\nend")

	assert.Len(t, buildDiffHunks(before, after), 2)
}

// 조각 머리줄은 `git diff` 와 같은 모양이다.
func TestDiffHunkHeader(t *testing.T) {
	hunks := buildDiffHunks(
		splitDiffLines("a\nb\nc"),
		splitDiffLines("a\nB\nc"),
	)

	require.Len(t, hunks, 1)
	assert.Equal(t, "@@ -1,3 +1,3 @@", hunks[0].header())
}

// `]c` 는 조각의 첫 **바뀐** 줄로 간다. 앞 문맥 셋을 지나게 두지 않는다(ADR-0095 §4).
func TestDiffHunkFirstChanged(t *testing.T) {
	hunks := buildDiffHunks(
		splitDiffLines("1\n2\n3\n4\n5\n6\n7"),
		splitDiffLines("1\n2\n3\nX\n5\n6\n7"),
	)

	require.Len(t, hunks, 1)
	assert.Equal(t, diffContext, hunks[0].firstChanged())
}

// 고친 줄 안에서 달라진 글자만 구간이 된다(ADR-0140 §7).
func TestDiffSpansMarkChangedBytes(t *testing.T) {
	left, right := diffSpansOf([]byte("const timeout = 100"), []byte("const timeout = 200"))

	require.Len(t, left, 1)
	require.Len(t, right, 1)
	assert.Equal(t, "1", string([]byte("const timeout = 100")[left[0].start:left[0].end]))
	assert.Equal(t, "2", string([]byte("const timeout = 200")[right[0].start:right[0].end]))
}

// 통째로 다른 줄에는 구간을 두지 않는다. 줄 바탕이 이미 말하는 것을 또 칠하지 않는다.
func TestDiffSpansSkipWhollyDifferentLines(t *testing.T) {
	left, right := diffSpansOf([]byte("aaaa"), []byte("bbbb"))

	assert.Nil(t, left)
	assert.Nil(t, right)
}

// 아주 긴 줄은 글자 diff 를 건너뛴다. 한 줄로 민 파일이 이 자리를 터뜨린다.
func TestDiffSpansSkipLongLines(t *testing.T) {
	long := []byte(strings.Repeat("a", diffSpanMaxBytes+1))

	left, right := diffSpansOf(long, append(long, 'b'))

	assert.Nil(t, left)
	assert.Nil(t, right)
}

// 구간 안인지 묻는 것이 맞아야 색이 어긋나지 않는다.
func TestInDiffSpans(t *testing.T) {
	spans := []diffSpan{{start: 2, end: 4}, {start: 7, end: 8}}

	assert.False(t, inDiffSpans(spans, 1))
	assert.True(t, inDiffSpans(spans, 2))
	assert.True(t, inDiffSpans(spans, 3))
	assert.False(t, inDiffSpans(spans, 4))
	assert.True(t, inDiffSpans(spans, 7))
	assert.False(t, inDiffSpans(spans, 9))
}

// 문법은 파일 이름이 고른다. 모르는 이름이면 색이 없다.
func TestLexDiffLines(t *testing.T) {
	tokens := lexDiffLines("a.go", splitDiffLines("package main"))
	require.Len(t, tokens, 1)
	assert.NotEmpty(t, tokens[0], "go 는 훑는다")

	assert.Nil(t, lexDiffLines("a.unknown", splitDiffLines("package main")))
}
