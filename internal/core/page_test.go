package core

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageFixture 는 화면보다 훨씬 긴 파일이다. 줄마다 글자가 달라서 칸을 지키는지도 볼 수 있다.
func pageFixture(lines int) string {
	rows := make([]string, lines)
	for i := range rows {
		rows[i] = fmt.Sprintf("line %02d abcdef", i)
	}

	return strings.Join(rows, "\n") + "\n"
}

// `ctrl+d` 는 커서와 화면을 같이 반 화면 내린다. 커서가 화면 안 같은 자리에 남는다.
func TestHalfPageDownMovesCursorAndScreenTogether(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "ctrl+d")

	buf := bufferOf(t, model)
	assert.Equal(t, 5, buf.cursorLine, "반 화면은 높이의 절반이다")
	assert.Equal(t, 5, buf.top, "화면도 같은 행 수만큼 내려간다")
}

// `ctrl+u` 는 그 반대다. 내려간 만큼 그대로 올라온다.
func TestHalfPageUpComesBack(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "ctrl+d", "ctrl+d", "ctrl+u")

	buf := bufferOf(t, model)
	assert.Equal(t, 5, buf.cursorLine)
	assert.Equal(t, 5, buf.top)
}

// 숫자는 되풀이다. `3ctrl+d` 는 반 화면 세 번이다.
func TestHalfPageCountRepeats(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "3", "ctrl+d")

	buf := bufferOf(t, model)
	assert.Equal(t, 15, buf.cursorLine)
	assert.Equal(t, 15, buf.top)
}

// 파일 끝을 지나서까지 굴리지 않는다. 마지막 줄이 화면 맨 아래에 오는 자리가 끝이고
// 커서만 마지막 줄로 간다.
func TestHalfPageDownStopsAtLastPage(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := tea.Model(m)
	for range 20 {
		model = send(model, "ctrl+d")
	}

	buf := bufferOf(t, model)
	assert.Equal(t, 39, buf.cursorLine, "커서는 마지막 줄이다")
	assert.Equal(t, 30, buf.top, "마지막 줄이 화면 맨 아래에 오는 자리에서 멈춘다")
}

// 파일 처음에서 `ctrl+u` 는 첫 줄에 선다. 위로도 넘어가지 않는다.
func TestHalfPageUpStopsAtFirstLine(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "ctrl+u")

	buf := bufferOf(t, model)
	assert.Equal(t, 0, buf.cursorLine)
	assert.Equal(t, 0, buf.top)
}

// 가로 칸은 `j`/`k` 와 같이 지킨다. vim 의 `startofline` 을 따르지 않는다(ADR-0062).
func TestHalfPageKeepsColumn(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "l", "l", "l", "ctrl+d")

	buf := bufferOf(t, model)
	assert.Equal(t, 5, buf.cursorLine)
	assert.Equal(t, 3, buf.cursorCol, "칸이 그대로다")
}

// 짧은 파일에서는 갈 곳이 없어 커서만 끝으로 간다. 화면은 그대로다.
func TestHalfPageInShortFile(t *testing.T) {
	m := newTestEditor("a\nb\nc\n", 80, 10)

	model := send(tea.Model(m), "ctrl+d")

	buf := bufferOf(t, model)
	assert.Equal(t, 2, buf.cursorLine, "마지막 줄이다")
	assert.Equal(t, 0, buf.top, "화면은 굴러가지 않는다")
}

// visual mode 에서는 고른 범위가 커서를 따라 자란다. 이동 키를 친 것과 같다.
func TestHalfPageGrowsVisualSelection(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "v", "ctrl+d")

	require.IsType(t, viewEditorVisual{}, model, "visual 에 머문다")

	buf := bufferOf(t, model)
	assert.Equal(t, 0, buf.selection.line, "anchor 는 그대로다")
	assert.Equal(t, 5, buf.cursorLine)
}

// visual 에서도 숫자는 되풀이다. 반 화면 이동만 숫자를 본다.
func TestHalfPageCountInVisual(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "v", "2", "ctrl+d")

	assert.Equal(t, 10, bufferOf(t, model).cursorLine)
}

// operator 뒤에는 오지 않는다. `d ctrl+d` 는 아무것도 지우지 않는다 — vim 과 같다.
func TestHalfPageIsNotAMotion(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "d", "ctrl+d")

	buf := bufferOf(t, model)
	assert.Equal(t, len(m.activeBuffer().lines), len(buf.lines), "줄이 그대로다")
	assert.Equal(t, 0, buf.cursorLine, "커서도 움직이지 않는다")
}

// 세는 것은 논리 줄이 아니라 화면 행이다. wrap 된 긴 줄은 그 안에서 여러 행을 지난다.
func TestHalfPageCountsScreenRows(t *testing.T) {
	m := newTestEditor("first\n"+strings.Repeat("x", 200)+"\nthird\nfourth\nfifth\nsixth\n", 80, 10)
	require.Equal(t, 3, len(wrapOffsets(m.activeBuffer().lines[1], m.contentWidth())), "긴 줄이 세 행이다")

	model := send(tea.Model(m), "ctrl+d")

	// 첫 줄에서 다섯 행 내려간다 — 긴 줄이 세 행을 먹으므로 넷째 줄이다.
	assert.Equal(t, 3, bufferOf(t, model).cursorLine, "논리 줄로 세면 여섯째 줄이 된다")
}

// `ctrl+f` 는 한 화면이다. 두 행을 겹쳐 두므로 높이에서 둘을 뺀 만큼 움직인다.
func TestFullPageDownLeavesTwoRowsOverlap(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "ctrl+f")

	buf := bufferOf(t, model)
	assert.Equal(t, 8, buf.top, "높이 10 에서 여덟 행 — 두 행이 겹친다")
	assert.Equal(t, 8, buf.cursorLine, "커서도 같이 간다")
}

// `ctrl+b` 는 그 반대다. 한 번 넘긴 것이 그대로 돌아온다.
func TestFullPageUpComesBack(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "ctrl+f", "ctrl+f", "ctrl+b")

	buf := bufferOf(t, model)
	assert.Equal(t, 8, buf.top)
	assert.Equal(t, 8, buf.cursorLine)
}

// 겹치는 두 행 덕분에 앞 화면 마지막 두 줄이 새 화면 맨 위에 다시 선다.
func TestFullPageShowsPreviousLastRows(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	before := strings.Split(textOf(t, tea.Model(m)), "\n")
	after := strings.Split(textOf(t, send(tea.Model(m), "ctrl+f")), "\n")

	assert.Equal(t, before[8], after[0], "앞 화면 아래에서 둘째 행이 새 화면 첫 행이다")
	assert.Equal(t, before[9], after[1])
}

// 숫자는 되풀이다. `2ctrl+f` 는 한 화면 두 번이다.
func TestFullPageCountRepeats(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "2", "ctrl+f")

	assert.Equal(t, 16, bufferOf(t, model).cursorLine)
}

// 한 화면도 마지막 줄이 화면 맨 아래에 오는 자리에서 멈춘다. 반 화면과 같은 한계다.
func TestFullPageDownStopsAtLastPage(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := tea.Model(m)
	for range 10 {
		model = send(model, "ctrl+f")
	}

	buf := bufferOf(t, model)
	assert.Equal(t, 39, buf.cursorLine)
	assert.Equal(t, 30, buf.top)
}

// visual 에서도 이동 키와 같이 범위를 늘린다.
func TestFullPageGrowsVisualSelection(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 10)

	model := send(tea.Model(m), "v", "ctrl+f")

	require.IsType(t, viewEditorVisual{}, model, "visual 에 머문다")
	assert.Equal(t, 8, bufferOf(t, model).cursorLine)
}

// 화면이 아주 낮으면 겹칠 자리가 없다. 그래도 한 행은 움직인다.
func TestFullPageInTinyWindow(t *testing.T) {
	m := newTestEditor(pageFixture(40), 80, 2)

	model := send(tea.Model(m), "ctrl+f")

	assert.Equal(t, 1, bufferOf(t, model).cursorLine, "높이 2 에서 한 행이다")
}

// 트리에서도 같은 키다. 고른 항목과 트리 화면이 같이 반 화면 내려간다.
func TestSidebarHalfPageMovesSelectionAndScreen(t *testing.T) {
	m := newTreeEditor(t, 80, 3)
	require.Equal(t, 4, m.sidebarHeight(), "반 화면이 두 행인 높이다")

	model := send(sendSync(t, tea.Model(m), "ctrl+w", "ctrl+w"), "ctrl+d")

	tree := model.(viewSidebar).sidebar
	assert.Equal(t, 2, tree.selected)
	assert.Equal(t, 2, tree.top, "화면도 두 행 내려간다")

	model = send(model, "ctrl+u")

	tree = model.(viewSidebar).sidebar
	assert.Equal(t, 0, tree.selected)
	assert.Equal(t, 0, tree.top)
}

// 트리도 마지막 항목이 화면 맨 아래에 오는 자리에서 멈춘다.
func TestSidebarHalfPageStopsAtLastPage(t *testing.T) {
	m := newTreeEditor(t, 80, 3)

	model := send(sendSync(t, tea.Model(m), "ctrl+w", "ctrl+w"), "ctrl+d", "ctrl+d", "ctrl+d")

	tree := model.(viewSidebar).sidebar
	rows := len(tree.rows())
	assert.Equal(t, rows-1, tree.selected, "고른 것은 마지막 항목이다")
	assert.Equal(t, rows-m.sidebarHeight(), tree.top)
}

// 트리의 `ctrl+f` 도 두 행을 겹친다. 높이 넷이면 두 행씩이라 반 화면과 같은 수인데,
// 그것은 이 높이에서만 그렇다.
func TestSidebarFullPageLeavesTwoRowsOverlap(t *testing.T) {
	m := newTreeEditor(t, 80, 8)
	require.Equal(t, 9, m.sidebarHeight())

	model := send(sendSync(t, tea.Model(m), "ctrl+w", "ctrl+w"), "ctrl+f")

	tree := model.(viewSidebar).sidebar
	rows := len(tree.rows())
	assert.Equal(t, min(7, rows-1), tree.selected, "높이 9 에서 일곱 행이다")

	model = send(model, "ctrl+b")

	tree = model.(viewSidebar).sidebar
	assert.Equal(t, 0, tree.selected, "돌아온다")
	assert.Equal(t, 0, tree.top)
}
