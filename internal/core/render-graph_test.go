package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drawGraph 는 줄들의 그래프 칸만 두 줄씩 이어 붙인다. 눈으로 읽는 그림이 곧 기대값이다.
func drawGraph(chars boxSet, rows []graphRow) string {
	lines := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		lanes := graphLanes(row)

		lines = append(lines,
			renderGraphMark(chars, row, lanes),
			renderGraphLink(chars, row, lanes),
		)
	}

	return strings.Join(lines, "\n")
}

// 갈래가 갈리고(merge) 합쳐지는(뿌리) 자리를 통째로 그려 본다.
//
// 위에서부터 merge, topic 쪽, master 쪽, 뿌리다. merge 가 둘째 열을 열고 그 열이 뿌리에서
// 첫 열로 모여든다.
func TestRenderGraphBranchAndMerge(t *testing.T) {
	rows := walkAll(t, newGraphFixture(t), 100)
	require.Len(t, rows, 4)

	assert.Equal(t, strings.Join([]string{
		"●   ",
		"│─┐ ",
		"│ ● ",
		"│ │ ",
		"● │ ",
		"│ │ ",
		"●─┘ ",
		"    ",
	}, "\n"), drawGraph(boxUnicode, rows))
}

// 물러선 터미널에서는 같은 그림이 ASCII 로 나온다. 열 폭이 같아야 줄이 맞는다(ADR-0028).
func TestRenderGraphASCII(t *testing.T) {
	rows := walkAll(t, newGraphFixture(t), 100)

	assert.Equal(t, strings.Join([]string{
		"*   ",
		"|-+ ",
		"| * ",
		"| | ",
		"* | ",
		"| | ",
		"*-+ ",
		"    ",
	}, "\n"), drawGraph(boxASCII, rows))
}

// 갈래가 하나뿐이면 칸도 하나다. 대부분의 저장소가 대부분의 자리에서 이 모습이다.
func TestRenderGraphSingleLane(t *testing.T) {
	rows := walkAll(t, newGitFixture(t), 100)
	require.Len(t, rows, 1)

	assert.Equal(t, 1, graphLanes(rows[0]))
	assert.Equal(t, "● \n  ", drawGraph(boxUnicode, rows))
}

// 폭은 커밋마다 잰다. 갈래가 없는 구간이 merge 때문에 밀리지 않는다.
func TestGraphLanes(t *testing.T) {
	rows := walkAll(t, newGraphFixture(t), 100)

	assert.Equal(t, 2, graphLanes(rows[0]), "merge 는 자기 줄에서 이미 둘째 열을 연다")
	assert.Equal(t, 2, graphLanes(rows[3]), "뿌리는 갈래 둘을 받아들인다")
	assert.Equal(t, 1, graphLanes(walkAll(t, newGitFixture(t), 100)[0]), "갈래가 하나면 한 칸이다")
}
