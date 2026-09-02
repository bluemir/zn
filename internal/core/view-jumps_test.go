package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"
)

// newJumpsView 는 이력 셋이 쌓인 판이다.
func newJumpsView(t *testing.T) (viewJumps, []string) {
	t.Helper()

	e, paths := jumpEditor(t)

	e.active = 0
	for i, path := range paths {
		e.activeBuffer().MoveTo(scheme.Cursor{Line: i * 2})
		e.recordJump()
		require.NoError(t, gotoFile(e, path, i*2+1))
	}

	model, _ := jumpsMode(e)

	here, ok := model.(viewJumps)
	require.True(t, ok, "판이 열려야 한다")

	return here, paths
}

// 판은 편집 영역의 행을 가져간다. `GOTO` 판과 같다(ADR-0069).
func TestJumpsTakesDrawerRows(t *testing.T) {
	m, _ := newJumpsView(t)

	require.Len(t, m.jumps.places, 3)
	assert.Equal(t, 3, m.jumpsRows())
	assert.Equal(t, 5, m.drawerHeight)
	assert.Equal(t, m.textAndDrawerHeight()-5, m.textHeight())
}

// 이력이 없어도 열린다. 「없다」를 보여주는 것이 아무 일도 안 나는 것보다 낫다.
func TestJumpsOpensWhenEmpty(t *testing.T) {
	e, _ := jumpEditor(t)

	model, _ := jumpsMode(e)
	m, ok := model.(viewJumps)
	require.True(t, ok)

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "되돌아간 자리가 없습니다")
	assert.Contains(t, content, "되돌아간 자리 0 개")
}

// j/k 로 고른 자리가 움직이고 양끝에서 멈춘다.
func TestJumpsMoves(t *testing.T) {
	m, _ := newJumpsView(t)

	// 이력 끝에 서 있으므로 맨 아래에서 시작한다.
	require.Equal(t, 2, m.selected)

	next, _ := m.press("k")
	m = next.(viewJumps)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("g")
	m = next.(viewJumps)
	assert.Equal(t, 0, m.selected)

	// 위 끝에서 멈춘다.
	next, _ = m.press("k")
	m = next.(viewJumps)
	assert.Equal(t, 0, m.selected)

	next, _ = m.press("G")
	m = next.(viewJumps)
	assert.Equal(t, 2, m.selected)
}

// 한글 상태로 쳐도 움직인다. 입력줄이 없는 판이다(ADR-0008).
func TestJumpsMovesInHangul(t *testing.T) {
	m, _ := newJumpsView(t)

	next, _ := m.press("ㅏ") // k
	m = next.(viewJumps)
	assert.Equal(t, 1, m.selected)
}

// **`j`/`k` 가 커서를 실제로 그 자리로 옮겨 보여준다.** 확정 전이라 이력의 지금 자리는
// 그대로다(ADR-0071).
func TestJumpsPreviewsWhileMoving(t *testing.T) {
	m, _ := newJumpsView(t)

	at := m.jumps.at
	require.Equal(t, 2, m.selected, "이력 끝에 서 있으면 맨 아래에서 시작한다")

	next, _ := m.press("k")
	m = next.(viewJumps)

	assert.Equal(t, 1, m.selected)
	assert.Equal(t, m.jumps.places[1].line, m.activeBuffer().Cursor.Line, "그 자리를 보여준다")
	assert.Equal(t, m.jumps.places[1].path, m.activeBuffer().Path)
	assert.Equal(t, at, m.jumps.at, "확정 전이라 지금 자리는 그대로다")

	// 맨 위까지 둘러본다.
	next, _ = m.press("g")
	m = next.(viewJumps)
	assert.Equal(t, 0, m.selected)
	assert.Equal(t, m.jumps.places[0].line, m.activeBuffer().Cursor.Line)

	// 위 끝에서 멈추면 커서도 그대로다.
	next, _ = m.press("k")
	m = next.(viewJumps)
	assert.Equal(t, 0, m.selected)
	assert.Equal(t, m.jumps.places[0].line, m.activeBuffer().Cursor.Line)
	assert.Equal(t, at, m.jumps.at, "끝까지 둘러봐도 지금 자리는 그대로다")
}

// enter 는 **판을 닫으며 확정한다.** 지금 자리가 고른 자리로 옮겨간다(ADR-0071).
func TestJumpsConfirmsAndCloses(t *testing.T) {
	m, paths := newJumpsView(t)

	next, _ := m.press("g") // 맨 위(가장 오래된 자리) 로 둘러본다
	m = next.(viewJumps)

	before := len(m.jumps.places)

	model, _ := m.press("enter")
	require.IsType(t, viewEditorNormal{}, model, "판이 닫혀야 한다")
	assert.Zero(t, model.(viewEditorNormal).drawerHeight)

	assert.Equal(t, paths[0], m.activeBuffer().Path)
	assert.Equal(t, m.jumps.places[0].line, m.activeBuffer().Cursor.Line)
	assert.Equal(t, 0, m.jumps.at, "지금 자리가 고른 자리로 옮겨간다")
	assert.Equal(t, before, len(m.jumps.places), "되짚는 이동은 이력에 담기지 않는다")

	// `ctrl+i` 가 거기서부터 이어진다.
	m.jumpForward()
	assert.Equal(t, 1, m.jumps.at)
}

// 열자마자 친 enter 도 확정이다. 둘러보지 않았어도 그 자리로 간다.
func TestJumpsConfirmsWithoutBrowsing(t *testing.T) {
	m, _ := newJumpsView(t)

	model, _ := m.press("enter")
	require.IsType(t, viewEditorNormal{}, model)

	assert.Equal(t, 2, m.jumps.at)
	assert.Equal(t, m.jumps.places[2].line, m.activeBuffer().Cursor.Line)
}

// **q 와 esc 는 취소다.** 둘러본 것이 없던 일이 되어 판을 열기 전 자리로 되돌아간다(ADR-0071).
func TestJumpsCancelRestoresOrigin(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newJumpsView(t)

			room := m.textAndDrawerHeight()
			originPath := m.activeBuffer().Path
			originLine := m.activeBuffer().Cursor.Line
			at := m.jumps.at

			// 둘러본다. 커서가 실제로 움직인다.
			next, _ := m.press("g")
			m = next.(viewJumps)
			require.NotEqual(t, originLine, m.activeBuffer().Cursor.Line)

			model, _ := m.press(key)

			require.IsType(t, viewEditorNormal{}, model, "판이 닫혀야 한다")
			assert.Zero(t, model.(viewEditorNormal).drawerHeight)
			assert.Equal(t, room, model.(viewEditorNormal).textHeight())

			assert.Equal(t, originPath, m.activeBuffer().Path, "열기 전 파일로 돌아온다")
			assert.Equal(t, originLine, m.activeBuffer().Cursor.Line, "열기 전 줄로 돌아온다")
			assert.Equal(t, at, m.jumps.at, "확정한 것이 없으니 이력도 그대로다")
		})
	}
}

// 둘러보느라 연 tab 은 확정한 것만 남는다. 취소하면 하나도 남지 않는다(ADR-0071).
func TestJumpsClosesPreviewTabs(t *testing.T) {
	t.Run("취소하면 다 닫는다", func(t *testing.T) {
		m, paths := jumpsViewWithClosedTabs(t)

		require.Len(t, m.buffers, 1, "한 파일만 열어 두고 시작한다")

		next, _ := m.press("g") // 닫혀 있는 파일을 둘러본다
		m = next.(viewJumps)
		require.Len(t, m.buffers, 2, "미리보기가 tab 을 열었다")

		m.press("esc")

		require.Len(t, m.buffers, 1, "둘러보며 연 tab 이 닫혔다")
		assert.Equal(t, paths[2], m.activeBuffer().Path)
	})

	t.Run("확정한 것만 남는다", func(t *testing.T) {
		m, paths := jumpsViewWithClosedTabs(t)

		// 이력은 first.go 둘과 second.go 하나다. 둘 다 닫혀 있다.
		require.Equal(t, paths[0], m.jumps.places[0].path)
		require.Equal(t, paths[1], m.jumps.places[2].path)

		next, _ := m.press("g") // first.go 를 연다
		m = next.(viewJumps)

		next, _ = m.press("G") // second.go 도 연다
		m = next.(viewJumps)
		require.Len(t, m.buffers, 3, "둘 다 열렸다")

		m.press("enter")

		require.Len(t, m.buffers, 2, "확정한 것과 원래 열려 있던 것만 남는다")
		assert.Equal(t, paths[1], m.activeBuffer().Path, "확정한 파일에 서 있다")
	})
}

// jumpsViewWithClosedTabs 는 이력의 파일 중 둘이 닫혀 있는 판이다.
// 미리보기가 여는 tab 을 재려면 닫혀 있는 파일이 있어야 한다.
func jumpsViewWithClosedTabs(t *testing.T) (viewJumps, []string) {
	t.Helper()

	e, paths := jumpEditor(t)

	e.active = 0
	for i, path := range paths {
		e.activeBuffer().MoveTo(scheme.Cursor{Line: i * 2})
		e.recordJump()
		require.NoError(t, gotoFile(e, path, i*2+1))
	}

	// 마지막 파일만 남기고 닫는다.
	for len(e.buffers) > 1 {
		if e.buffers[0].Path == paths[2] {
			e.closeTabAt(1)

			continue
		}

		e.closeTabAt(0)
	}
	require.Equal(t, paths[2], e.activeBuffer().Path)

	model, _ := jumpsMode(e)

	return model.(viewJumps), paths
}

// 지금 자리는 `●` 로, 고른 줄은 `▸` 로 드러난다. 둘은 겹칠 수 있어서 칸을 나눠 가진다.
//
// 반전만으로 나타내지 않는 것이 요점이다 — 화면을 글자로 떠서 보는 길이 있다.
func TestJumpsMarksCurrentAndSelected(t *testing.T) {
	m, _ := newJumpsView(t)

	// 이력 끝에 서 있으면 `●` 가 어디에도 없다. 되짚어 들어오지 않은 상태다.
	//
	// 판만 본다 — hint 줄에는 `● 지금` 이라는 범례가 늘 있다.
	require.Equal(t, 3, m.jumps.at)
	drawer := ansi.Strip(m.renderDrawer())
	assert.NotContains(t, drawer, "●")
	assert.Contains(t, drawer, "▸")

	// 한 번 되돌아가면 그 줄에 `●` 가 선다.
	m.jumpBack()
	next, _ := m.press("k")
	m = next.(viewJumps)

	rows := strings.Split(ansi.Strip(m.renderDrawer()), "\n")
	marked := 0
	for _, row := range rows {
		if strings.Contains(row, "●") {
			marked++
		}
	}
	assert.Equal(t, 1, marked, "지금 자리는 하나다")
}

// 판의 모든 행이 정확히 편집 영역 폭이다. 한 행이라도 넘치면 그 아래가 통째로 밀린다.
func TestJumpsDrawerWidth(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120} {
		m, _ := newJumpsView(t)
		m.width = width

		for i, row := range strings.Split(m.renderDrawer(), "\n") {
			assert.Equal(t, m.textWidth(), widthOf(row), "폭 %d 의 %d 행: %q", width, i, row)
		}
	}
}
