package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// tipClick 은 지금 아래 줄에 선 tip 을 누른 좌표다. 서 있지 않으면 시험을 멈춘다.
func tipClick(t *testing.T, m viewEditorNormal) tea.MouseClickMsg {
	t.Helper()

	span := m.tipSpan(m.tipBottom(), m.keyState().showcmd())
	require.NotEqual(t, [2]int{}, span, "tip 이 서 있어야 누를 수 있다")

	return click(span[0], m.height-1)
}

// 아래 줄의 tip 을 누르면 그 문장에서 목록이 열린다.
func TestTipClickOpensListAtThatTip(t *testing.T) {
	m := newTestEditor("hello\nworld\n", 120, 10)

	// 시험은 core.Run 을 지나지 않아서 tipIndex 가 0 이다 — 선 문장이 목록의 첫 것이다(ADR-0061).
	tip := m.pickTip(m.tipBottom(), "")
	require.Equal(t, assets.Tips[0], tip)

	model, _ := m.Update(tipClick(t, m))

	tips, ok := model.(viewTips)
	require.True(t, ok, "tip 목록이 열린다: %T", model)
	assert.Equal(t, 0, tips.selected, "누른 그 문장에 선다")
}

// 누른 자리가 tip 의 끝 칸이어도 열린다. 칸 범위 안이면 어디든 같다.
func TestTipClickTakesWholeSpan(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)

	span := m.tipSpan(m.tipBottom(), "")
	model, _ := m.Update(click(span[1]-1, m.height-1))

	assert.IsType(t, viewTips{}, model)
}

// tip 오른쪽 끝 다음 칸은 tip 이 아니다.
func TestTipClickStopsAtSpanEnd(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)

	span := m.tipSpan(m.tipBottom(), "")
	model, _ := m.Update(click(span[1], m.height-1))

	assert.IsType(t, viewEditorNormal{}, model, "빈 칸을 누른 것이다")
}

// 아래 줄 왼쪽(커서 자리) 은 tip 이 아니다. statusBar 는 눌러도 아무 일이 없다(ADR-0012).
func TestTipClickIgnoresRestOfStatusBar(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)

	model, _ := m.Update(click(0, m.height-1))

	assert.IsType(t, viewEditorNormal{}, model)
}

// 알림이 떠 있는 동안에는 tip 이 서지 않는다. 그 자리를 눌러도 아무 일이 없다.
func TestTipClickDoesNothingWhileNoticeShows(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)
	span := m.tipSpan(m.tipBottom(), "")
	require.NotEqual(t, [2]int{}, span)

	m.notify("저장했습니다")
	require.Equal(t, [2]int{}, m.tipSpan(m.tipBottom(), ""), "알림이 그 줄을 쓴다")

	model, _ := m.Update(click(span[0], m.height-1))

	assert.IsType(t, viewEditorNormal{}, model)
}

// 접두 키를 기다리는 동안에도 그 칸은 showcmd 것이다.
func TestTipClickDoesNothingWhileShowcmd(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)

	model := send(tea.Model(m), "g")
	pending, ok := model.(viewEditorNormal)
	require.True(t, ok)
	require.NotEmpty(t, pending.keyState().showcmd(), "`g` 를 기다리는 중이다")

	assert.Equal(t, [2]int{}, pending.tipSpan(pending.tipBottom(), pending.keyState().showcmd()))
}

// 아래 줄이 아니면 tip 이 아니다. 같은 x 라도 statusBar 위 줄은 지나간다.
func TestTipClickOnlyOnBottomRow(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)

	span := m.tipSpan(m.tipBottom(), "")
	model, _ := m.Update(click(span[0], m.height-2))

	assert.IsType(t, viewEditorNormal{}, model)
}

// insert 에서 눌러도 열린다. 치던 자리는 tip 목록을 닫으면 normal 이다.
func TestTipClickWorksInInsert(t *testing.T) {
	m := newTestEditor("hello\n", 120, 10)

	insert, _ := insertMode(m.editor)
	view := insert.(viewEditorInsert)

	span := view.tipSpan(view.tipBottom(), "")
	require.NotEqual(t, [2]int{}, span)

	model, _ := insert.Update(click(span[0], view.height-1))

	assert.IsType(t, viewTips{}, model)
}

// 트리에 포커스가 있어도 같다. 아래 줄은 화면을 가로지른다.
func TestTipClickWorksInTree(t *testing.T) {
	m := newTreeEditor(t, 120, 10)

	tree := viewSidebar{editor: m.editor}
	span := tree.tipSpan(tree.tipBottom(), "")
	require.NotEqual(t, [2]int{}, span)

	model, _ := tree.Update(click(span[0], tree.height-1))

	assert.IsType(t, viewTips{}, model)
}

// 그리는 쪽과 재는 쪽이 같은 문장을 본다. 폭을 좁히면 둘 다 같이 사라진다.
func TestTipSpanFollowsWhatIsDrawn(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120, 200} {
		m := newTestEditor("hello\n", width, 10)

		bottom := m.tipBottom()
		tip := m.pickTip(bottom, "")
		span := m.tipSpan(bottom, "")

		if tip == "" {
			assert.Equal(t, [2]int{}, span, "폭 %d: 서지 않으면 칸도 없다", width)

			continue
		}

		assert.Contains(t, tipLinesOf(m.View())[m.height-1], tip, "폭 %d: 그린 줄에 그 문장이 있다", width)
		assert.Equal(t, m.sidebarLeft()+m.textWidth(), span[1], "폭 %d: 오른쪽 끝에 붙는다", width)
	}
}
