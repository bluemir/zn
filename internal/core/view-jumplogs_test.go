package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJumplogsView 는 같은 파일의 여러 줄이 기록된 판이다.
// count 는 기록 수이고, 스크롤을 재려면 보이는 행 수보다 많아야 한다.
func newJumplogsView(t *testing.T, count int) (viewJumplogs, string) {
	t.Helper()

	e, paths := jumpEditor(t)

	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	// 오래된 것부터 담는다. 기록은 최근이 앞이므로 결과는 뒤집힌 순서다.
	for i := range count {
		e.recordVisit(jumpPlace{path: paths[0], line: i % 6})
	}

	model, _ := jumplogsMode(e)

	here, ok := model.(viewJumplogs)
	require.True(t, ok, "판이 열려야 한다")

	return here, paths[0]
}

// 판은 편집 영역의 행을 가져간다. 다른 판들과 같다(ADR-0069).
func TestJumplogsTakesDrawerRows(t *testing.T) {
	m, _ := newJumplogsView(t, 3)

	require.Len(t, m.logs.places, 3)
	assert.Equal(t, 3, m.jumplogsRows())
	assert.Equal(t, 5, m.drawerHeight)
	assert.Equal(t, m.paneHeight()-5, m.textHeight())
}

// **담는 것과 보이는 것이 다르다.** 기록이 256 개라도 판은 열여섯 줄이고 `j`/`k` 가 훑는다.
func TestJumplogsScrollsThroughLongLog(t *testing.T) {
	e, paths := jumpEditor(t)

	// 열여섯 줄이 실제로 걸리려면 화면이 그만큼 높아야 한다 — 20 행에서는 남는 자리가
	// 열두 줄이라 상한이 아니라 화면이 먼저 막는다.
	e.height = 25

	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	// 줄을 다 다르게 해서 중복 거르기에 걸리지 않게 한다.
	for i := range 60 {
		e.recordVisit(jumpPlace{path: paths[0], line: i})
	}
	require.Len(t, e.logs.places, 60)

	model, _ := jumplogsMode(e)
	m := model.(viewJumplogs)

	height := m.jumplogsRows()
	require.Equal(t, jumplogsMaxRows, height, "최대 열여섯 줄까지만 보인다")
	require.Less(t, height, len(m.logs.places), "판보다 기록이 길어야 재는 뜻이 있다")

	// 화면 끝까지는 첫 행이 움직이지 않는다.
	for range height - 1 {
		next, _ := m.press("j")
		m = next.(viewJumplogs)
	}
	assert.Equal(t, 0, m.top)

	// 한 칸 더 내려가면 그만큼만 밀린다.
	next, _ := m.press("j")
	m = next.(viewJumplogs)
	assert.Equal(t, 1, m.top)
	assert.Equal(t, height, m.selected)

	// 맨 끝으로 가면 마지막 화면이 된다.
	next, _ = m.press("G")
	m = next.(viewJumplogs)
	assert.Equal(t, len(m.logs.places)-1, m.selected)
	assert.Equal(t, len(m.logs.places)-height, m.top)

	// 아랫 테두리가 몇 번째인지 알려 준다 — 목록만으로는 어디쯤인지 모른다(ADR-0079).
	assert.Contains(t, lastLine(ansi.Strip(m.renderDrawer())), "60/60")
	assert.NotContains(t, m.hint(), "60/60", "아래 줄에는 적지 않는다")
}

// **최근이 맨 위다.** 브라우저 방문 기록과 같고, 되돌아간 자리 판과는 방향이 반대다.
func TestJumplogsShowsNewestFirst(t *testing.T) {
	e, paths := jumpEditor(t)

	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	for _, line := range []int{0, 2, 4} {
		e.recordVisit(jumpPlace{path: paths[0], line: line})
	}

	model, _ := jumplogsMode(e)
	m := model.(viewJumplogs)

	rows := strings.Split(ansi.Strip(m.renderDrawer()), "\n")

	// 테두리 한 줄을 건너뛴 첫 목록 행이 가장 최근(줄 5) 이다.
	assert.Contains(t, rows[1], ":5")
	assert.Contains(t, rows[3], ":1")
}

// `j`/`k` 가 커서를 실제로 그 자리로 옮겨 보여준다(ADR-0073 와 같은 손이다).
func TestJumplogsPreviewsWhileMoving(t *testing.T) {
	m, path := newJumplogsView(t, 6)

	require.Equal(t, 0, m.selected)

	next, _ := m.press("j")
	m = next.(viewJumplogs)

	assert.Equal(t, 1, m.selected)
	assert.Equal(t, path, m.activeBuffer().path)
	assert.Equal(t, m.logs.places[1].line, m.activeBuffer().cursorLine, "그 자리를 보여준다")
}

// enter 는 판을 닫으며 확정한다. 떠난 자리는 되돌아오기 이력에 담긴다.
func TestJumplogsConfirmsAndCloses(t *testing.T) {
	m, path := newJumplogsView(t, 6)

	origin := m.activeBuffer().cursorLine

	next, _ := m.press("j")
	m = next.(viewJumplogs)
	next, _ = m.press("j")
	m = next.(viewJumplogs)

	target := m.logs.places[m.selected].line

	model, _ := m.press("enter")

	require.IsType(t, viewEditorNormal{}, model, "판이 닫혀야 한다")
	assert.Zero(t, model.(viewEditorNormal).drawerHeight)
	assert.Equal(t, path, m.activeBuffer().path)
	assert.Equal(t, target, m.activeBuffer().cursorLine)

	// `ctrl+o` 로 판을 열기 전 자리로 돌아온다.
	m.jumpBack()
	assert.Equal(t, origin, m.activeBuffer().cursorLine)
}

// **q 와 esc 는 취소다.** 판을 열기 전 자리로 돌아가고 이력도 늘지 않는다(ADR-0073).
func TestJumplogsCancelRestoresOrigin(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newJumplogsView(t, 6)

			pane := m.paneHeight()
			originLine := m.activeBuffer().cursorLine

			next, _ := m.press("j")
			m = next.(viewJumplogs)
			next, _ = m.press("j")
			m = next.(viewJumplogs)
			require.NotEqual(t, originLine, m.activeBuffer().cursorLine)

			model, _ := m.press(key)

			require.IsType(t, viewEditorNormal{}, model)
			assert.Zero(t, model.(viewEditorNormal).drawerHeight, "판이 닫혀야 한다")
			assert.Equal(t, pane, model.(viewEditorNormal).textHeight())
			assert.Equal(t, originLine, m.activeBuffer().cursorLine, "열기 전 줄로 돌아온다")
			assert.Empty(t, m.jumps.places, "아무 데도 안 갔으니 되돌아오기 이력도 비어 있다")
		})
	}
}

// 기록이 없어도 열린다. 「없다」를 보여주는 것이 아무 일도 안 나는 것보다 낫다.
func TestJumplogsOpensWhenEmpty(t *testing.T) {
	e, _ := jumpEditor(t)

	model, _ := jumplogsMode(e)
	m, ok := model.(viewJumplogs)
	require.True(t, ok)

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "방문한 자리가 없습니다")
	assert.Contains(t, content, "방문한 자리 0 개")
}

// 한글 상태로 쳐도 움직인다. 입력줄이 없는 판이다(ADR-0008).
func TestJumplogsMovesInHangul(t *testing.T) {
	m, _ := newJumplogsView(t, 6)

	next, _ := m.press("ㅓ") // j
	m = next.(viewJumplogs)
	assert.Equal(t, 1, m.selected)
}

// 판의 모든 행이 정확히 편집 영역 폭이다. 한 행이라도 넘치면 그 아래가 통째로 밀린다.
func TestJumplogsDrawerWidth(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120} {
		m, _ := newJumplogsView(t, 6)
		m.width = width

		for i, row := range strings.Split(m.renderDrawer(), "\n") {
			assert.Equal(t, m.textWidth(), ansi.StringWidth(row), "폭 %d 의 %d 행: %q", width, i, row)
		}
	}
}
