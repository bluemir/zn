package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ansiStrip 은 색을 뺀다. 테두리를 글자로 견주는 시험이 쓴다.
func ansiStrip(s string) string { return ansi.Strip(s) }

// 아랫 테두리 오른쪽에 「몇 번째/전부」가 얹힌다. 목록 행은 하나도 쓰지 않는다(ADR-0079).
func TestCountBorderPutsCountOnTheRight(t *testing.T) {
	row := renderCountBorder(boxUnicode, 40, 30, 3540)

	require.Equal(t, 40, widthOf(row), "테두리 폭은 그대로다")
	assert.True(t, strings.HasPrefix(row, boxUnicode.bottomLeft), "왼쪽 모서리")
	assert.True(t, strings.HasSuffix(row, boxUnicode.bottomRight), "오른쪽 모서리")
	assert.Contains(t, row, "30/3,540", "세 자리마다 쉼표를 넣는다")

	// 오른쪽에 붙되 모서리에 닿지 않는다 — 닿으면 테두리의 일부처럼 읽힌다.
	assert.True(t, strings.HasSuffix(row, boxUnicode.horizontal+boxUnicode.bottomRight),
		"모서리 앞에 테두리 한 칸이 남는다: %q", row)

	// 왼쪽은 테두리로 찬다.
	assert.Contains(t, row, boxUnicode.bottomLeft+boxUnicode.horizontal)
}

// 담긴 것이 없으면 셀 것도 없다. 테두리만 그린다.
func TestCountBorderIsPlainWhenEmpty(t *testing.T) {
	plain := boxUnicode.bottomLeft + strings.Repeat(boxUnicode.horizontal, 38) + boxUnicode.bottomRight

	assert.Equal(t, plain, renderCountBorder(boxUnicode, 40, 1, 0))
	assert.Equal(t, plain, renderCountBorder(boxUnicode, 40, 0, -1))
}

// 자리에 안 들어가면 테두리만 그린다. 반쯤 잘린 숫자는 숫자로 읽히지 않는다.
func TestCountBorderIsPlainWhenTooNarrow(t *testing.T) {
	narrow := renderCountBorder(boxUnicode, 8, 1234, 5678)

	require.Equal(t, 8, widthOf(narrow))
	assert.NotContains(t, narrow, "1,234")
	assert.NotContains(t, narrow, "…", "자르지 않고 버린다")
}

// 딱 들어가는 폭에서는 얹고, 폭이 그대로다.
func TestCountBorderKeepsWidthAtTheEdge(t *testing.T) {
	// ` 9/99 ` 는 여섯 칸이다. 모서리 둘을 빼고 여섯 칸이 남는 폭이 최소다.
	for width := 8; width <= 12; width++ {
		row := renderCountBorder(boxUnicode, width, 9, 99)

		assert.Equal(t, width, widthOf(row), "폭 %d", width)
	}

	assert.Contains(t, renderCountBorder(boxUnicode, 8, 9, 99), "9/99")
}

// 세 판이 같은 자를 쓴다. 판마다 다른 규칙을 만들지 않는다(ADR-0079).
func TestCountBorderSharedByThreePanes(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	// **줄을 다 다르게 한다.** 같은 자리는 하나로 접히므로(ADR-0074) 되풀이하면 개수가 안 는다.
	for i := range 20 {
		e.recordVisit(jumpPlace{path: paths[0], line: i})
	}

	logs, _ := jumplogsMode(e)
	require.Len(t, logs.(viewJumplogs).logs.places, 20)
	assert.Contains(t, lastLine(ansiStrip(logs.(viewJumplogs).renderDrawer())), "1/20")

	// jumplist 는 방문 기록과 다른 것이라 `recordJump` 로 찬다(view-jumps_test.go).
	jumped, paths2 := newJumpsView(t)
	require.NotEmpty(t, paths2)
	require.NotEmpty(t, jumped.jumps.places)

	bottom := lastLine(ansiStrip(jumped.renderDrawer()))
	assert.Contains(t, bottom, "/"+formatCount(len(jumped.jumps.places)))
}
