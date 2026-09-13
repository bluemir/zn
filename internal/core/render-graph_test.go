package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 칸을 글자로 옮기는 표다. 그림이 맞는지는 진짜 git 과 대는 시험이 본다(git-graph-draw_test.go).

// 갈래마다 글자가 하나씩 붙는다. 빠뜨리면 그 칸이 빈 칸으로 그려져 선이 끊긴다.
func TestGraphCharCoversEverySymbol(t *testing.T) {
	all := []graphSymbol{
		graphNode, graphVertical, graphSlashUp, graphSlashDown,
		graphUnderscore, graphDash, graphDot,
	}

	for _, cell := range all {
		assert.NotEqual(t, " ", graphChar(boxUnicode, cell), "%d 번 갈래에 글자가 없다", cell)
		assert.NotEqual(t, " ", graphChar(boxASCII, cell), "%d 번 갈래에 물러설 글자가 없다", cell)
	}

	assert.Equal(t, " ", graphChar(boxUnicode, graphBlank))
}

// 물러선 터미널의 글자가 곧 git 이 쓰는 글자다. 전부 Narrow 라 열이 밀리지 않는다(ADR-0028).
func TestGraphCharASCIIMatchesGit(t *testing.T) {
	assert.Equal(t, "*", graphChar(boxASCII, graphNode))
	assert.Equal(t, "|", graphChar(boxASCII, graphVertical))
	assert.Equal(t, "/", graphChar(boxASCII, graphSlashUp))
	assert.Equal(t, "\\", graphChar(boxASCII, graphSlashDown))
}

// 행을 폭에 맞춰 채운다. 행마다 길이가 다르면 뒤에 붙는 커밋 이름이 들쭉날쭉해진다.
func TestRenderGraphLinePadsToWidth(t *testing.T) {
	line := []graphSymbol{graphNode, graphBlank}

	got := renderGraphLine(boxUnicode, line, 6)

	assert.Equal(t, boxUnicode.dot+strings.Repeat(" ", 5), got)
}

// 폭은 커밋마다 잰다. 갈래가 없는 구간이 merge 때문에 밀리지 않는다.
func TestGraphWidthOf(t *testing.T) {
	rows := walkAll(t, newGraphFixture(t), 100)
	require.Len(t, rows, 4)

	for _, row := range rows {
		width := graphWidthOf(row)

		for _, line := range row.graph {
			assert.LessOrEqual(t, len(line), width, "어느 행도 잰 폭을 넘지 않는다")
		}
	}

	// 열 하나가 글자 한 칸과 그 오른쪽 사이 칸을 쓴다. 갈래가 하나면 둘이다.
	assert.Equal(t, 2, graphWidthOf(walkAll(t, newGitFixture(t), 100)[0]), "갈래가 하나면 한 열이다")
}
