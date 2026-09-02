package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tab 을 끌어서 순서를 바꾸는 자리다(ADR-0090).

// tabNamesOf 는 지금 tab 차례를 이름으로 준다.
func tabNamesOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	v, ok := m.(viewEditorNormal)
	require.True(t, ok, "normal mode 가 아니다: %T", m)

	names := make([]string, 0, len(v.buffers))
	for i := range v.buffers {
		names = append(names, v.tabName(i))
	}

	return names
}

// tabSpanOf 는 index 번째 tab 이 그려진 첫 화면 칸이다.
func tabSpanOf(t *testing.T, m viewEditorNormal, index int) int {
	t.Helper()

	spans := m.renderTabline(m.textWidth()).tabs
	require.Greater(t, len(spans), index)

	return spans[index][0] + m.sidebarLeft()
}

// moveTab 은 활성 tab 을 그 자리로 옮기고 보던 파일을 그대로 본다.
func TestMoveTab(t *testing.T) {
	tests := []struct {
		name   string
		active int
		to     int
		want   []string
		still  string
	}{
		{name: "오른쪽으로", active: 0, to: 2, want: []string{"b.txt", "c.txt", "a.txt", "d.txt"}, still: "a.txt"},
		{name: "왼쪽으로", active: 3, to: 1, want: []string{"a.txt", "d.txt", "b.txt", "c.txt"}, still: "d.txt"},
		{name: "맨 끝으로", active: 1, to: 3, want: []string{"a.txt", "c.txt", "d.txt", "b.txt"}, still: "b.txt"},
		{name: "맨 앞으로", active: 2, to: 0, want: []string{"c.txt", "a.txt", "b.txt", "d.txt"}, still: "c.txt"},
		{name: "제자리면 그대로", active: 1, to: 1, want: []string{"a.txt", "b.txt", "c.txt", "d.txt"}, still: "b.txt"},
		{name: "없는 자리는 무시", active: 1, to: 9, want: []string{"a.txt", "b.txt", "c.txt", "d.txt"}, still: "b.txt"},
		{name: "tab 아닌 칸(-1) 은 무시", active: 1, to: -1, want: []string{"a.txt", "b.txt", "c.txt", "d.txt"}, still: "b.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt")
			m.active = tt.active

			m.moveTab(tt.to)

			assert.Equal(t, tt.want, tabNamesOf(t, m))
			assert.Equal(t, tt.still, m.tabName(m.active), "보던 파일은 그대로다")
		})
	}
}

// tabline 에서 끌면 순서가 바뀐다. 끄는 동안 계속 바뀌므로 놓는 이벤트를 기다리지 않는다.
func TestDragTabReorders(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt")

	var model tea.Model = m

	// 첫 tab 을 누른다. 누른 순간 그것이 활성이다.
	model, _ = model.Update(click(tabSpanOf(t, m, 0), 0))
	require.Equal(t, 0, activeOf(t, model))

	// 셋째 tab 자리로 끈다.
	model, _ = model.Update(drag(tabSpanOf(t, m, 2), 0))

	assert.IsType(t, viewEditorNormal{}, model, "visual 로 넘어가지 않는다")
	assert.Equal(t, []string{"b.txt", "c.txt", "a.txt"}, tabNamesOf(t, model))
	assert.Equal(t, "a.txt", model.(viewEditorNormal).tabName(activeOf(t, model)))
}

// 끄는 동안 여러 번 바뀐다. 한 칸씩 옮겨 가도 어긋나지 않는다.
func TestDragTabStepByStep(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt")

	var model tea.Model = m
	model, _ = model.Update(click(tabSpanOf(t, m, 0), 0))

	// 한 칸 오른쪽으로. 그 뒤 자리표가 바뀌므로 매번 다시 읽는다.
	next := model.(viewEditorNormal)
	model, _ = model.Update(drag(tabSpanOf(t, next, 1), 0))
	assert.Equal(t, []string{"b.txt", "a.txt", "c.txt"}, tabNamesOf(t, model))

	next = model.(viewEditorNormal)
	model, _ = model.Update(drag(tabSpanOf(t, next, 2), 0))
	assert.Equal(t, []string{"b.txt", "c.txt", "a.txt"}, tabNamesOf(t, model))

	// 도로 왼쪽으로 끌면 되돌아온다.
	next = model.(viewEditorNormal)
	model, _ = model.Update(drag(tabSpanOf(t, next, 0), 0))
	assert.Equal(t, []string{"a.txt", "b.txt", "c.txt"}, tabNamesOf(t, model))
}

// **tabline 밖으로 끌면 아무 일도 하지 않는다.** 끌던 자리를 유지하고 범위를 고르지도 않는다.
//
// tabline 은 한 행이고 편집 영역이 바로 아래 한 칸이라, 이 자리가 막히지 않으면 가로로 끄는
// 손이 늘 글을 고르기 시작한다(ADR-0090 §3).
func TestDragTabOutsideTablineDoesNothing(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt")

	var model tea.Model = m
	model, _ = model.Update(click(tabSpanOf(t, m, 0), 0))

	// 편집 영역으로 내려간다.
	model, _ = model.Update(drag(contentLeftOf(t, model)+2, tablineHeight+1))

	assert.IsType(t, viewEditorNormal{}, model, "visual 로 넘어가지 않는다")
	assert.False(t, bufferOf(t, model).Selection.Active, "범위를 고르지 않는다")
	assert.Equal(t, []string{"a.txt", "b.txt", "c.txt"}, tabNamesOf(t, model), "순서도 그대로다")

	// 포인터가 tabline 으로 돌아오면 이어서 옮긴다.
	next := model.(viewEditorNormal)
	model, _ = model.Update(drag(tabSpanOf(t, next, 1), 0))

	assert.Equal(t, []string{"b.txt", "a.txt", "c.txt"}, tabNamesOf(t, model))
}

// 편집 영역에서 시작한 드래그는 그대로 visual 이다. tab 은 움직이지 않는다.
func TestDragFromTextStillOpensVisual(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")

	var model tea.Model = m
	left := contentLeftOf(t, m)

	model, _ = model.Update(click(left+1, tablineHeight))
	model, _ = model.Update(drag(left+3, tablineHeight))

	require.IsType(t, viewEditorVisual{}, model, "끌기 시작하면 visual 이다")

	// 위로 끌어 올려도 tab 이 안 움직인다. visual 의 motion 처리는 편집 영역만 본다.
	model, _ = model.Update(drag(0, 0))

	v, ok := model.(viewEditorVisual)
	require.True(t, ok)
	assert.Equal(t, []string{"a.txt", "b.txt"}, []string{v.tabName(0), v.tabName(1)})
}

// 놓으면 끄는 상태가 지워진다. 그다음 편집 영역 드래그가 제대로 visual 을 연다.
func TestDragTabReleaseClearsState(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")

	var model tea.Model = m
	model, _ = model.Update(click(tabSpanOf(t, m, 0), 0))
	require.True(t, model.(viewEditorNormal).draggingTab)

	model, _ = model.Update(tea.MouseReleaseMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	assert.False(t, model.(viewEditorNormal).draggingTab)

	// 이제 편집 영역 드래그가 막히지 않는다.
	left := contentLeftOf(t, model)
	model, _ = model.Update(click(left+1, tablineHeight))
	model, _ = model.Update(drag(left+3, tablineHeight))

	assert.IsType(t, viewEditorVisual{}, model)
}

// 편집 영역을 누르면 끄는 상태가 서지 않는다.
func TestClickTextDoesNotArmTabDrag(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")

	var model tea.Model = m
	model, _ = model.Update(click(contentLeftOf(t, m)+1, tablineHeight))

	assert.False(t, model.(viewEditorNormal).draggingTab)
}

// **양끝의 가려짐 표시 위는 tab 이 아니다.** 누르면 화면을 미는 자리인데 끌면서 밀지 않는다 —
// 손이 멈춰도 계속 흘러가게 된다(ADR-0029, ADR-0090 §5).
func TestDragTabOverHiddenMarkerDoesNothing(t *testing.T) {
	// tab 을 많이 열어 tabline 이 넘치게 한다.
	m := newTabsEditor("a.txt", "bbbbbbbb.txt", "cccccccc.txt", "dddddddd.txt", "eeeeeeee.txt")
	m.active = 4
	m.scrollTabsTo()

	row := m.renderTabline(m.textWidth())
	require.Greater(t, row.left[1], row.left[0], "왼쪽 가려짐 표시가 있어야 하는 판이다")

	before := tabNamesOf(t, m)

	var model tea.Model = m
	model, _ = model.Update(click(tabSpanOf(t, m, m.active), 0))
	model, _ = model.Update(drag(row.left[0]+m.sidebarLeft(), 0))

	assert.Equal(t, before, tabNamesOf(t, model), "순서가 그대로다")
}

// tab 이 하나뿐이면 옮길 자리가 없다.
func TestDragTabWithSingleTab(t *testing.T) {
	m := newTabsEditor("a.txt")

	var model tea.Model = m
	model, _ = model.Update(click(tabSpanOf(t, m, 0), 0))
	model, _ = model.Update(drag(30, 0))

	assert.Equal(t, []string{"a.txt"}, tabNamesOf(t, model))
	assert.Equal(t, 0, activeOf(t, model))
}
