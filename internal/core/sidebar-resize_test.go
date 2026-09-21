package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// 트리 구분선을 끌어서 폭을 바꾸는 자리다(ADR-0145).

// release 는 버튼을 놓는 것이다. 끄는 것이 거기서 끝난다.
func release(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// sidebarOf 는 화면 model 이 든 트리다. mode 마다 type 이 달라서(ADR-0002) 갈라 받는다.
func sidebarOf(t *testing.T, m tea.Model) sidebar {
	t.Helper()

	switch v := m.(type) {
	case viewEditorNormal:
		return v.sidebar
	case viewEditorInsert:
		return v.sidebar
	case viewEditorVisual:
		return v.sidebar
	case viewSidebar:
		return v.sidebar
	case viewEditorEmpty:
		return v.sidebar
	}

	require.Fail(t, "트리를 든 화면이 아니다", "%T", m)

	return sidebar{}
}

// 구분선 칸은 트리 본문과 다른 영역이다. 끌어서 폭을 바꾸는 자리다.
func TestSidebarEdgeIsItsOwnRegion(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	assert.Equal(t, sidebarDefaultWidth-2, m.sidebarDividerCol())
	assert.Equal(t, regionSidebarEdge, m.regionAt(m.sidebarDividerCol(), 0), "구분선")
	assert.Equal(t, regionSidebar, m.regionAt(m.sidebarDividerCol()-1, 0), "git 마커 칸은 트리다")
	assert.Equal(t, regionSidebar, m.regionAt(m.sidebarDividerCol()+1, 0), "구분선 뒤 빈 칸도 트리다")
	assert.Equal(t, regionTabline, m.regionAt(sidebarDefaultWidth, 0), "그 오른쪽은 편집 영역이다")
}

// 구분선을 눌러도 트리 항목이 골라지지 않는다. 그 자리는 잡는 자리다.
func TestSidebarEdgeClickSelectsNothing(t *testing.T) {
	m := newTreeEditor(t, 80, 6)
	m.sidebar.selected = 2

	model, _ := tea.Model(m).Update(click(m.sidebarDividerCol(), 3))

	require.IsType(t, viewEditorNormal{}, model, "mode 가 바뀌지 않는다")
	assert.Equal(t, 2, sidebarOf(t, model).selected, "고른 항목이 그대로다")
	assert.True(t, model.(viewEditorNormal).draggingSidebar, "끌 채비만 한다")
}

// 끌면 구분선이 끌린 칸으로 간다. 놓는 것을 기다리지 않고 끄는 동안 계속 바뀐다.
func TestSidebarDragResizes(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	var model tea.Model = m
	model, _ = model.Update(click(m.sidebarDividerCol(), 3))

	model, _ = model.Update(drag(40, 3))
	assert.Equal(t, 42, sidebarOf(t, model).width, "구분선이 40 칸에 서는 폭이다")

	model, _ = model.Update(drag(20, 3))
	assert.Equal(t, 22, sidebarOf(t, model).width, "도로 좁히는 것도 된다")

	model, _ = model.Update(release(20, 3))
	assert.False(t, model.(viewEditorNormal).draggingSidebar, "놓으면 끝난다")

	model, _ = model.Update(drag(60, 3))
	assert.Equal(t, 22, sidebarOf(t, model).width, "놓은 뒤 움직이는 것은 폭을 끌지 않는다")
}

// 구분선을 잡은 것이 아니면 폭이 끌리지 않는다.
func TestSidebarDragNeedsEdgePress(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	var model tea.Model = m
	model, _ = model.Update(click(2, 3)) // 트리 항목을 누른다

	model, _ = model.Update(drag(50, 3))

	assert.Equal(t, sidebarDefaultWidth, sidebarOf(t, model).width)
}

// 폭은 양쪽에서 멈춘다. 좁히는 쪽은 sidebarMinWidth, 넓히는 쪽은 편집 영역이 남는 자리다.
func TestSidebarResizeClamps(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	m.resizeSidebarTo(0)
	assert.Equal(t, sidebarMinWidth, m.sidebar.width, "끝까지 좁혀도 최소 폭에서 멈춘다")

	m.resizeSidebarTo(79)
	assert.Equal(t, 80-textarea.MinTextWidth, m.sidebar.width, "편집 영역이 MinTextWidth 만큼 남는다")
	assert.True(t, m.sidebarVisible(), "끝까지 넓혀도 트리가 사라지지 않는다")
	assert.Equal(t, textarea.MinTextWidth, m.textWidth())
}

// 폭을 바꾸면 그린 행도 그 폭이고, 편집 영역이 그만큼 옆으로 간다.
func TestSidebarResizeMovesEverything(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	m.resizeSidebarTo(18) // 폭 20

	require.Equal(t, 20, m.sidebar.width)
	assert.Equal(t, 20, m.sidebarLeft())
	assert.Equal(t, 60, m.textWidth())
	assert.Equal(t, 17, m.sidebar.labelWidth(), "이름 칸도 같이 준다")

	for i, cell := range m.sidebar.renderCells(6, "", nil, boxUnicode) {
		plain := ansi.Strip(cell)
		assert.Equal(t, 20, textarea.WidthOf(plain), "행 %d: %q", i, plain)
		assert.True(t, strings.HasSuffix(plain, "│ "), "행 %d: %q", i, plain)
	}

	// statusBar 위 줄의 mode 칸도 트리 폭에 맞는다(ADR-0005).
	top := ansi.Strip(m.renderStatusBar("NORMAL", "")[0])
	assert.Equal(t, 20, strings.Index(top, "main.go"), "경로는 편집 영역 왼쪽 끝에 맞는다")
}

// 끌어 둔 폭이 화면보다 넓어지면 감췄다가, 화면이 넓어지면 그 폭 그대로 돌아온다.
func TestSidebarKeepsWidthAcrossResize(t *testing.T) {
	m := newTreeEditor(t, 120, 6)

	m.resizeSidebarTo(58) // 폭 60
	require.Equal(t, 60, m.sidebar.width)

	m.width = 70
	assert.False(t, m.sidebarVisible(), "60 + 20 이 안 되면 감춘다")
	assert.Equal(t, 0, m.sidebarLeft())

	m.width = 120
	assert.True(t, m.sidebarVisible())
	assert.Equal(t, 60, m.sidebarLeft(), "끌어 둔 폭이 살아 있다")
}

// 트리에 포커스가 있을 때도 끌린다. 구분선은 어느 mode 에서나 같은 자리다.
func TestSidebarDragFromTreeFocus(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	model := sendSync(t, tea.Model(m), "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)

	tree := model.(viewSidebar)
	model, _ = model.Update(click(tree.sidebarDividerCol(), 2))
	model, _ = model.Update(drag(24, 2))

	require.IsType(t, viewSidebar{}, model, "트리에 머문다")
	assert.Equal(t, 26, sidebarOf(t, model).width)
}

// insert 에서도 끌린다. 글을 치다가 트리를 넓히는 손이 mode 를 나가지 않아야 한다.
func TestSidebarDragFromInsert(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	model, _ := tea.Model(m).Update(key("i"))
	require.IsType(t, viewEditorInsert{}, model)

	model, _ = model.Update(click(m.sidebarDividerCol(), 3))
	model, _ = model.Update(drag(24, 3))

	require.IsType(t, viewEditorInsert{}, model, "insert 에 머문다")
	assert.Equal(t, 26, sidebarOf(t, model).width)
}

// 파일이 하나도 안 열린 첫 화면에서도 끌린다. 볼 파일이 없어도 트리는 있다(ADR-0064).
func TestSidebarDragOnEmptyScreen(t *testing.T) {
	m := newEmptyEditor(80, 8)
	m.sidebar = openSidebarSync(t, newTreeFixture(t))

	var model tea.Model = m
	model, _ = model.Update(click(m.sidebarDividerCol(), 3))
	require.True(t, model.(viewEditorEmpty).draggingSidebar, "구분선을 잡았다")

	model, _ = model.Update(drag(24, 3))

	require.IsType(t, viewEditorEmpty{}, model, "빈 화면에 머문다")
	assert.Equal(t, 26, sidebarOf(t, model).width)
	assert.Equal(t, 26, model.(viewEditorEmpty).sidebarLeft())

	model, _ = model.Update(release(24, 3))
	assert.False(t, model.(viewEditorEmpty).draggingSidebar, "놓으면 끝난다")
}

// visual 에서도 끌린다. 고른 범위는 건드리지 않는다.
func TestSidebarDragFromVisualKeepsSelection(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	model := sendSync(t, tea.Model(m), "v", "l")
	require.IsType(t, viewEditorVisual{}, model)

	before := model.(viewEditorVisual).activeBuffer().Cursor

	model, _ = model.Update(click(m.sidebarDividerCol(), 3))
	model, _ = model.Update(drag(24, 3))

	require.IsType(t, viewEditorVisual{}, model, "visual 에 머문다")
	assert.Equal(t, 26, sidebarOf(t, model).width)
	assert.Equal(t, before, model.(viewEditorVisual).activeBuffer().Cursor, "커서가 그대로다")
}
