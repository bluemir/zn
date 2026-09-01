package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// click, wheel 은 key 와 같은 자리다 — 화면 좌표 하나를 메시지로 만든다.
func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// rightClickAt 은 오른쪽 버튼으로 누른 좌표다. tabline 에서만 뜻이 있다(ADR-0060).
func rightClickAt(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight}
}

func wheel(x, y int, up bool) tea.MouseWheelMsg {
	button := tea.MouseWheelDown
	if up {
		button = tea.MouseWheelUp
	}

	return tea.MouseWheelMsg{X: x, Y: y, Button: button}
}

// contentLeftOf 는 편집 내용이 시작하는 화면 칸이다. 클릭 좌표를 만들 때 쓴다.
func contentLeftOf(t *testing.T, m tea.Model) int {
	t.Helper()

	e, ok := m.(interface{ contentLeft() int })
	require.True(t, ok, "편집 화면이 아니다: %T", m)

	return e.contentLeft()
}

func TestPositionAt(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		width   int
		x, y    int
		line    int
		col     int
		missing bool
	}{
		{name: "첫 줄 첫 칸", data: "abc\ndef\n", width: wide, x: 0, y: 0, line: 0, col: 0},
		{name: "둘째 줄 셋째 칸", data: "abc\ndef\n", width: wide, x: 2, y: 1, line: 1, col: 2},
		{
			name: "줄번호 칸을 누르면 줄 시작", data: "abc\ndef\n", width: wide,
			x: -3, y: 1, line: 1, col: 0,
		},
		{
			// tab stop 은 화면 행의 시작에서 센다. 줄을 통째로 넘기면 여기가 틀린다.
			name: "tab 으로 들여쓴 줄", data: "\tabc\n", width: wide,
			x: 4, y: 0, line: 0, col: 1,
		},
		{
			name: "tab 자리 안을 누르면 tab 시작", data: "\tabc\n", width: wide,
			x: 2, y: 0, line: 0, col: 0,
		},
		{
			// 두 칸짜리 글자의 둘째 칸은 그 글자의 시작으로 맞춘다.
			name: "두 칸짜리 글자의 오른쪽 칸", data: "한글\n", width: wide,
			x: 1, y: 0, line: 0, col: 0,
		},
		{
			name: "두 칸짜리 글자 다음 글자", data: "한글\n", width: wide,
			x: 2, y: 0, line: 0, col: 3,
		},
		{
			// 폭 4 에서 "abcdefgh" 는 행 둘이다. 둘째 행 첫 칸은 offset 4 다.
			name: "wrap 된 줄의 이어지는 행", data: "abcdefgh\n", width: 4,
			x: 0, y: 1, line: 0, col: 4,
		},
		{
			name: "wrap 된 줄 다음 논리 줄", data: "abcdefgh\nxy\n", width: 4,
			x: 1, y: 2, line: 1, col: 1,
		},
		{
			name: "줄 끝을 넘겨 누르면 줄 끝", data: "abc\n", width: wide,
			x: 40, y: 0, line: 0, col: 3,
		},
		{
			// 마지막 줄 아래는 아무것도 없는 자리다. 끌어당기지 않는다.
			name: "마지막 줄 아래 빈 자리", data: "abc\n", width: wide,
			x: 0, y: 5, missing: true,
		},
		{name: "화면 위쪽 밖", data: "abc\n", width: wide, x: 0, y: -1, missing: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.data))

			line, col, ok := buf.positionAt(test.x, test.y, test.width, 10)
			if test.missing {
				assert.False(t, ok)
				return
			}

			require.True(t, ok)
			assert.Equal(t, test.line, line, "줄")
			assert.Equal(t, test.col, col, "offset")
		})
	}
}

func TestRegionAt(t *testing.T) {
	tests := []struct {
		name string
		tree bool
		x, y int
		want region
	}{
		{name: "tabline", x: 5, y: 0, want: regionTabline},
		{name: "편집 영역", x: 5, y: 1, want: regionText},
		{name: "줄번호 칸도 편집 영역", x: 0, y: 2, want: regionText},
		{name: "statusBar 위 줄", x: 5, y: 6, want: regionNone},
		{name: "statusBar 아래 줄", x: 5, y: 7, want: regionNone},
		{name: "화면 오른쪽 밖", x: 100, y: 1, want: regionNone},
		{name: "화면 아래 밖", x: 5, y: 100, want: regionNone},
		{name: "음수 좌표", x: -1, y: 1, want: regionNone},

		// sidebar 는 화면 맨 윗줄부터라 그 폭 안에서는 y==0 도 tabline 이 아니다.
		{name: "sidebar 맨 윗줄", tree: true, x: 5, y: 0, want: regionSidebar},
		{name: "sidebar 구분선 칸", tree: true, x: 31, y: 2, want: regionSidebar},
		{name: "sidebar 오른쪽은 tabline", tree: true, x: 32, y: 0, want: regionTabline},
		{name: "sidebar 오른쪽은 편집 영역", tree: true, x: 32, y: 1, want: regionText},
		{name: "sidebar 아래는 statusBar", tree: true, x: 5, y: 6, want: regionNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTestEditor("abc\ndef\n", 60, 5)
			if test.tree {
				m = newTreeEditor(t, 60, 5)
			}

			assert.Equal(t, test.want, m.regionAt(test.x, test.y))
		})
	}
}

// sidebar 가 켜져 있어도 화면이 좁으면 그리지 않는다. open 만 보면 왼쪽 32 칸을 트리로 착각한다.
func TestRegionAtIgnoresHiddenSidebar(t *testing.T) {
	m := newTreeEditor(t, 60, 5)
	require.Equal(t, regionSidebar, m.regionAt(5, 1))

	m.width = 40 // 32 + 20 미만
	require.False(t, m.sidebarVisible())
	require.True(t, m.sidebar.open, "사용자 의도는 그대로다")

	assert.Equal(t, regionText, m.regionAt(5, 1), "sidebar 가 안 보이면 그 자리는 편집 영역이다")
}

func TestClickMovesCursor(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m, _ = m.Update(click(left+2, tablineHeight+1))

	assert.Equal(t, 1, cursorLineOf(t, m))
	assert.Equal(t, 2, cursorColOf(t, m))
	assert.IsType(t, viewEditorNormal{}, m, "normal 에 남는다")
}

// normal 의 커서는 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
func TestClickClampsToNormal(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)
	left := contentLeftOf(t, m)

	m, _ = m.Update(click(left+30, tablineHeight))

	assert.Equal(t, 2, cursorColOf(t, m), "마지막 글자 위")
}

func TestClickBelowLastLineDoesNothing(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m, _ = m.Update(click(left+1, tablineHeight+4))

	assert.Equal(t, 0, cursorLineOf(t, m))
	assert.Equal(t, 0, cursorColOf(t, m))
}

func TestClickOnStatusBarDoesNothing(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)

	m, _ = m.Update(click(5, tablineHeight+5)) // statusBar 첫 줄

	assert.Equal(t, 0, cursorLineOf(t, m))
	assert.Equal(t, 0, cursorColOf(t, m))
}

func TestNonLeftButtonIsIgnored(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m, _ = m.Update(tea.MouseClickMsg{X: left + 2, Y: tablineHeight + 1, Button: tea.MouseRight})

	assert.Equal(t, 0, cursorLineOf(t, m), "편집 영역의 오른쪽 버튼은 아무 일도 하지 않는다")
}

// 누르던 접두 키를 클릭이 조용히 삼키면 안 된다.
func TestClickKeepsPendingPrefix(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m = send(m, "g")
	m, _ = m.Update(click(left+1, tablineHeight))

	normal, ok := m.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, "g", normal.keyState().showcmd(), "g 를 계속 기다린다")
}

func TestClickInInsertStaysInInsert(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m = send(m, "i")
	m, _ = m.Update(click(left+3, tablineHeight+1))

	assert.IsType(t, viewEditorInsert{}, m)
	assert.Equal(t, 1, cursorLineOf(t, m))
	assert.Equal(t, 3, cursorColOf(t, m), "insert 는 줄 끝 다음 칸에 설 수 있다")
}

// 커서를 옮기면 undo 구간이 끊긴다. 화살표 이동과 같다.
func TestClickInInsertBreaksUndoRun(t *testing.T) {
	var m tea.Model = newTestEditor("ab\ncd\n", 40, 5)
	left := contentLeftOf(t, m)

	m = send(m, "i", "X")
	m, _ = m.Update(click(left+1, tablineHeight+1))
	m = send(m, "Y", "esc")

	require.Equal(t, []string{"Xab", "cYd"}, linesOf(bufferOf(t, m)))

	m = send(m, "u")
	assert.Equal(t, []string{"Xab", "cd"}, linesOf(bufferOf(t, m)), "클릭 뒤에 친 것만 되돌아간다")

	m = send(m, "u")
	assert.Equal(t, []string{"ab", "cd"}, linesOf(bufferOf(t, m)))
}

func TestClickSidebarTakesFocus(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 60, 8)

	// 트리 첫 행은 root 다. 디렉터리라 펼치기/접기이고 포커스가 트리에 남는다.
	m, _ = m.Update(click(3, 0))

	assert.IsType(t, viewSidebar{}, m, "포커스가 트리로 간다")
}

func TestClickSidebarTogglesDirectory(t *testing.T) {
	m := newTreeEditor(t, 60, 8)
	require.Greater(t, len(m.sidebar.rows()), 1, "root 가 펼쳐진 상태로 시작한다")

	var model tea.Model = m
	model, _ = model.Update(click(3, 0)) // root 를 접는다

	tree, ok := model.(viewSidebar)
	require.True(t, ok)
	assert.Len(t, tree.sidebar.rows(), 1, "접히면 root 한 행만 남는다")
	assert.Equal(t, 0, tree.sidebar.selected, "누른 행이 골라진다")
}

func TestClickSidebarOpensFile(t *testing.T) {
	m := newTreeEditor(t, 60, 8)

	// root 아래는 build/, docs/, .gitignore, README.md, main.go 순서다.
	rows := m.sidebar.rows()
	index := -1
	for i, row := range rows {
		if row.node.name == "README.md" {
			index = i
		}
	}
	require.GreaterOrEqual(t, index, 0)

	var model tea.Model = m
	model, _ = model.Update(click(3, index-m.sidebar.top))

	assert.IsType(t, viewEditorNormal{}, model, "파일을 열면 포커스가 편집 영역으로 온다")
	assert.Equal(t, "README.md", bufferOf(t, model).path[len(bufferOf(t, model).path)-9:])
}

func TestClickBelowTreeDoesNothing(t *testing.T) {
	m := newTreeEditor(t, 60, 8)
	rows := len(m.sidebar.rows())
	require.Less(t, rows, m.sidebarHeight(), "트리가 화면보다 짧아야 빈 행이 있다")

	var model tea.Model = m
	model, _ = model.Update(click(3, m.sidebarHeight()-1))

	tree, ok := model.(viewSidebar)
	require.True(t, ok)
	assert.Equal(t, m.sidebar.selected, tree.sidebar.selected, "고른 항목이 그대로다")
	assert.Len(t, tree.sidebar.rows(), rows, "펼치거나 접지 않는다")
}

func TestClickTextFromSidebarTakesFocus(t *testing.T) {
	m := newTreeEditor(t, 60, 8)

	var model tea.Model = m
	model = send(model, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)

	left := contentLeftOf(t, model)
	model, _ = model.Update(click(left+1, tablineHeight+1))

	assert.IsType(t, viewEditorNormal{}, model, "포커스가 편집 영역으로 온다")
	assert.Equal(t, 1, cursorLineOf(t, model))
	assert.Equal(t, 0, cursorColOf(t, model), "둘째 줄은 한 글자라 마지막 글자 위")
}

func TestClickTablineSwitchesTab(t *testing.T) {
	m := newTestEditor("abc\n", 60, 5)
	m.buffers = append(m.buffers, newBuffer("second.txt", []byte("xyz\n")))
	require.Equal(t, 0, m.active)

	// 두 번째 tab 이 그려진 칸을 tabline 이 알려준 대로 누른다.
	spans := m.renderTabline(m.textWidth()).tabs
	require.Len(t, spans, 2)

	var model tea.Model = m
	model, _ = model.Update(click(spans[1][0], 0))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 1, normal.active)
}

// dirty 표시 `+` 로 칸이 밀려도 클릭이 옆 tab 으로 가지 않는다.
func TestClickTablineWithDirtyTab(t *testing.T) {
	m := newTestEditor("abc\n", 60, 5)
	m.buffers[0].dirty = true
	m.buffers = append(m.buffers, newBuffer("second.txt", []byte("xyz\n")))
	m.buffers = append(m.buffers, newBuffer("third.txt", []byte("xyz\n")))

	spans := m.renderTabline(m.textWidth()).tabs
	require.Len(t, spans, 3)

	for i, span := range spans {
		var model tea.Model = m
		model, _ = model.Update(click(span[1]-1, 0)) // 그 tab 의 마지막 칸

		normal, ok := model.(viewEditorNormal)
		require.True(t, ok)
		assert.Equal(t, i, normal.active, "%d 번째 tab 의 끝 칸", i+1)
	}
}

// 가려짐 표시를 누르면 보고 있는 tab 은 그대로 두고 그 방향으로 민다.
// 편집하던 파일을 놓지 않고 가려진 쪽에 무엇이 있는지 훑을 수 있어야 한다(ADR-0029).
func TestClickTablineHiddenCountScrolls(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.width = 30
	require.Equal(t, " 1 a.txt │ 2 b.txt │.......│3>", tablineOf(t, m.View()))

	var model tea.Model = m
	model, _ = model.Update(click(m.renderTabline(m.textWidth()).right[0], 0))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 0, normal.active, "보고 있는 tab 은 그대로다")
	assert.Equal(t, "<1│ 2 b.txt │ 3 c.txt │....│2>", tablineOf(t, normal.View()))

	model, _ = model.Update(click(normal.renderTabline(normal.textWidth()).left[0], 0))

	normal, ok = model.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 0, normal.active)
	assert.Equal(t, " 1 a.txt │ 2 b.txt │.......│3>", tablineOf(t, normal.View()), "왼쪽 표시는 도로 당긴다")
}

// 밀어둔 채로 tab 을 옮기면 활성 tab 을 따라 다시 맞는다.
func TestTabSwitchResetsManualTablineScroll(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.width = 30

	var model tea.Model = m
	model, _ = model.Update(click(m.renderTabline(m.textWidth()).right[0], 0))
	require.Equal(t, "<1│ 2 b.txt │ 3 c.txt │....│2>", tablineOf(t, model.(viewEditorNormal).View()))

	model = send(model, "g", "T")

	assert.Equal(t, 4, activeOf(t, model), "마지막 tab 으로 둘러 간다")
	assert.Equal(t, "<3│ 4 d.txt │ 5 e.txt", tablineOf(t, model.(viewEditorNormal).View()))
}

func TestClickTablineFillerDoesNothing(t *testing.T) {
	m := newTestEditor("abc\n", 60, 5)
	m.buffers = append(m.buffers, newBuffer("second.txt", []byte("xyz\n")))

	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model, _ = model.Update(click(spans[len(spans)-1][1]+1, 0)) // 마지막 tab 오른쪽 빈 칸

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 0, normal.active, "tab 이 없는 칸은 아무 일도 하지 않는다")
}

func TestClickIgnoredWhileTypingCommand(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m = send(m, ":", "w")
	m, _ = m.Update(click(left+1, tablineHeight+1))

	command, ok := m.(viewEditorCommand)
	require.True(t, ok, "명령줄에 남는다")
	assert.Equal(t, "w", command.input.text, "치던 명령이 그대로다")
	assert.Equal(t, 0, command.buffers[command.active].cursorLine, "커서도 그대로다")
}

func TestClickIgnoredWhileTypingSearch(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m = send(m, "/", "d")
	m, _ = m.Update(click(left+1, tablineHeight))

	search, ok := m.(viewEditorSearch)
	require.True(t, ok, "검색에 남는다")
	assert.Equal(t, "d", search.input.text)
}

// 긴 파일을 만들어 스크롤할 자리를 둔다.
func longBuffer(lines int) string {
	out := ""
	for i := range lines {
		out += string(rune('a'+i%26)) + "\n"
	}
	return out
}

func TestWheelScrollsWithoutMovingCursor(t *testing.T) {
	var m tea.Model = newTestEditor(longBuffer(50), 40, 10)
	left := contentLeftOf(t, m)

	m, _ = m.Update(wheel(left+1, tablineHeight+1, false))

	buf := bufferOf(t, m)
	assert.Equal(t, wheelRows, buf.top, "화면이 굴러간다")
	assert.Equal(t, wheelRows, buf.cursorLine, "화면 맨 윗줄로 끌려온다")
}

func TestWheelKeepsCursorWhileVisible(t *testing.T) {
	m := newTestEditor(longBuffer(50), 40, 10)
	m.buffers[0].cursorLine = 8
	m.buffers[0].top = 0

	var model tea.Model = m
	model, _ = model.Update(wheel(contentLeftOf(t, model)+1, tablineHeight+1, false))

	buf := bufferOf(t, model)
	assert.Equal(t, wheelRows, buf.top)
	assert.Equal(t, 8, buf.cursorLine, "아직 화면 안이라 그대로다")
}

// 위로 굴리면 커서가 화면 아래로 벗어난다. 아래쪽 끝 행으로 데려온다.
func TestWheelUpPullsCursorToBottom(t *testing.T) {
	m := newTestEditor(longBuffer(50), 40, 10)
	m.buffers[0].top = 20
	m.buffers[0].cursorLine = 29 // 화면 맨 아랫줄

	var model tea.Model = m
	model, _ = model.Update(wheel(contentLeftOf(t, model)+1, tablineHeight+1, true))

	buf := bufferOf(t, model)
	require.Equal(t, 20-wheelRows, buf.top)
	assert.Equal(t, buf.top+9, buf.cursorLine, "화면 맨 아랫줄로 끌려온다")
}

func TestWheelUpAtTopDoesNothing(t *testing.T) {
	var m tea.Model = newTestEditor(longBuffer(50), 40, 10)

	m, _ = m.Update(wheel(contentLeftOf(t, m)+1, tablineHeight+1, true))

	assert.Equal(t, 0, bufferOf(t, m).top)
}

func TestWheelStopsAtLastLine(t *testing.T) {
	var m tea.Model = newTestEditor(longBuffer(20), 40, 5)
	left := contentLeftOf(t, m)

	for range 50 {
		m, _ = m.Update(wheel(left+1, tablineHeight+1, false))
	}

	buf := bufferOf(t, m)
	assert.Equal(t, len(buf.lines)-1, buf.top, "마지막 줄이 맨 위에서 멈춘다")
}

func TestWheelMovesTopRowInsideWrappedLine(t *testing.T) {
	// 폭보다 훨씬 긴 줄 하나. 화면 행이 여럿이라 topRow 가 움직여야 한다.
	long := ""
	for range 200 {
		long += "x"
	}

	var m tea.Model = newTestEditor(long+"\n", 40, 5)
	m, _ = m.Update(wheel(contentLeftOf(t, m)+1, tablineHeight+1, false))

	buf := bufferOf(t, m)
	assert.Equal(t, 0, buf.top, "같은 논리 줄 안이다")
	assert.Equal(t, wheelRows, buf.topRow)
}

func TestWheelOnSidebarScrollsTree(t *testing.T) {
	m := newTreeEditor(t, 60, 3) // 트리보다 낮은 화면이라 굴릴 자리가 있다
	require.Greater(t, len(m.sidebar.rows()), m.sidebarHeight())

	before := m.sidebar.selected

	var model tea.Model = m
	model, _ = model.Update(wheel(3, 1, false))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok, "포커스는 편집 영역에 남는다")
	assert.Equal(t, wheelRows, normal.sidebar.top, "트리가 굴러간다")
	assert.Equal(t, before, normal.sidebar.selected, "고른 항목은 그대로다")
	assert.Equal(t, 0, normal.buffers[normal.active].top, "편집 영역은 굴러가지 않는다")
}

func TestWheelOnTextDoesNotScrollTree(t *testing.T) {
	m := newTreeEditor(t, 60, 3)
	m.buffers[0] = newBuffer("main.go", []byte(longBuffer(50)))

	var model tea.Model = m
	model, _ = model.Update(wheel(contentLeftOf(t, model)+1, tablineHeight+1, false))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 0, normal.sidebar.top, "트리는 그대로다")
	assert.Equal(t, wheelRows, normal.buffers[normal.active].top)
}

// 고른 항목이 화면 밖으로 나가면 커서를 놓을 자리가 없다. ADR-0005 의 포커스 표시가 잠시 없다.
func TestWheelCanHideSidebarCursor(t *testing.T) {
	m := newTreeEditor(t, 60, 3)
	require.Greater(t, len(m.sidebar.rows()), m.sidebarHeight())

	var model tea.Model = m
	model = send(model, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)
	require.NotNil(t, model.View().Cursor, "포커스가 트리에 있으면 커서가 보인다")

	for range 5 {
		model, _ = model.Update(wheel(3, 1, false))
	}

	assert.Nil(t, model.View().Cursor, "고른 항목이 화면 밖이면 커서가 없다")
}

func TestWheelWorksWhileTypingCommand(t *testing.T) {
	var m tea.Model = newTestEditor(longBuffer(50), 40, 10)
	left := contentLeftOf(t, m)

	m = send(m, ":")
	m, _ = m.Update(wheel(left+1, tablineHeight+1, false))

	command, ok := m.(viewEditorCommand)
	require.True(t, ok, "명령줄에 남는다")
	assert.Equal(t, wheelRows, command.buffers[command.active].top)
}

// drag 는 버튼을 누른 채 움직이는 것이다. click 과 같이 화면 좌표 하나를 메시지로 만든다.
func drag(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// 클릭만 하면 커서 이동이고, 끌기 시작하면 그 자리를 anchor 로 삼아 visual 로 들어간다.
// vim 과 같다 (ADR-0012, ADR-0037).
func TestDragStartsVisual(t *testing.T) {
	m := newTestEditor("foo bar\nbaz qux", 80, 20)
	left := contentLeftOf(t, m)

	after, _ := m.Update(click(left+1, tablineHeight))
	assert.IsType(t, viewEditorNormal{}, after, "클릭만으로는 visual 이 아니다")

	after, _ = after.Update(drag(left+4, tablineHeight))

	visual, ok := after.(viewEditorVisual)
	require.True(t, ok, "끌기 시작하면 visual 이다")

	buf := visual.activeBuffer()
	assert.False(t, buf.selection.linewise, "드래그는 글자 단위다")
	assert.Equal(t, 1, buf.selection.col, "누른 자리가 anchor 다")
	assert.Equal(t, 4, buf.cursorCol, "끌린 자리가 커서다")

	// 뗄 때는 보지 않는다. visual 에 머문다.
	after, _ = after.Update(tea.MouseReleaseMsg{X: left + 4, Y: tablineHeight, Button: tea.MouseLeft})
	assert.IsType(t, viewEditorVisual{}, after)
}

// 편집 영역 밖으로 끌면 좌표를 안으로 당겨서 읽는다. 포커스는 옮기지 않는다.
func TestDragOutsideTextClampsInside(t *testing.T) {
	m := newTestEditor("foo bar\nbaz qux", 80, 20)
	left := contentLeftOf(t, m)

	after, _ := m.Update(click(left+4, tablineHeight+1))
	after, _ = after.Update(drag(0, tablineHeight+1))

	visual, ok := after.(viewEditorVisual)
	require.True(t, ok, "sidebar 쪽으로 끌어도 포커스는 그대로다")
	assert.Equal(t, 0, visual.activeBuffer().cursorCol, "줄 시작까지 골랐다")
}

// 화면 아래로 끌면 그 방향으로 한 행 굴린다.
//
// 여는 것은 편집 영역 안의 움직임이다. 밖에서 시작한 드래그는 받지 않아서 tabline 을 누르고
// 끌어도 범위가 열리지 않는다 — 일단 열린 뒤에는 밖으로 나가도 따라간다.
func TestDragBelowTextScrolls(t *testing.T) {
	m := newTestEditor("one\ntwo\nthree\nfour\nfive", 80, 2)
	left := contentLeftOf(t, m)

	after, _ := m.Update(click(left, tablineHeight))
	after, _ = after.Update(drag(left, tablineHeight+1))
	require.IsType(t, viewEditorVisual{}, after)

	after, _ = after.Update(drag(left, tablineHeight+2))

	visual, ok := after.(viewEditorVisual)
	require.True(t, ok)
	assert.Equal(t, 1, visual.activeBuffer().top, "한 행 굴러갔다")
	assert.Equal(t, 2, visual.activeBuffer().cursorLine, "끌린 쪽 끝 행이 커서다")
}

// visual 에서 클릭하면 고른 것을 놓고 그 자리가 새 시작이 된다.
func TestClickLeavesVisual(t *testing.T) {
	m := newTestEditor("foo bar\nbaz qux", 80, 20)
	left := contentLeftOf(t, m)

	after := send(m, "v", "l", "l")
	require.IsType(t, viewEditorVisual{}, after)

	after, _ = after.Update(click(left+2, tablineHeight+1))

	require.IsType(t, viewEditorNormal{}, after)
	buf := bufferOf(t, after)
	assert.False(t, buf.selection.active, "고른 범위를 놓는다")
	assert.Equal(t, 1, buf.cursorLine)
	assert.Equal(t, 2, buf.cursorCol)
}

// statusBar 는 눌러도 아무 일이 없다. visual 도 그대로다.
func TestClickStatusBarKeepsVisual(t *testing.T) {
	after := send(newTestEditor("foo bar", 80, 20), "v", "l")

	after, _ = after.Update(click(0, tablineHeight+20))

	assert.IsType(t, viewEditorVisual{}, after)
	assert.True(t, bufferOf(t, after).selection.active)
}

// 우클릭은 누른 자리의 tab 을 닫는다. 보고 있던 파일은 그대로 본다(ADR-0060).
func TestRightClickTablineClosesTab(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt")
	m.active = 1
	require.Equal(t, " 1 a.txt │ 2 b.txt │ 3 c.txt", tablineOf(t, m.View()))

	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model, _ = model.Update(rightClickAt(spans[2][0], 0))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok, "normal 에 머문다: %T", model)
	require.Len(t, normal.buffers, 2)
	assert.Equal(t, 1, normal.active)
	assert.Equal(t, "b.txt", normal.activeBuffer().path, "보고 있던 파일이 그대로다")
	assert.Equal(t, " 1 a.txt │ 2 b.txt", tablineOf(t, normal.View()))
}

// 보던 tab 의 왼쪽을 닫으면 번호만 당겨진다. 보는 파일이 바뀌면 안 된다.
func TestRightClickTablineOnLeftKeepsViewedFile(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt")
	m.active = 2

	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model, _ = model.Update(rightClickAt(spans[0][0], 0))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, 1, normal.active, "번호가 하나 당겨진다")
	assert.Equal(t, "c.txt", normal.activeBuffer().path)
	assert.Equal(t, " 1 b.txt │ 2 c.txt", tablineOf(t, normal.View()))
}

// 보고 있는 tab 을 우클릭하면 그 자리에 드러나는 파일을 본다. `:q` 와 같다.
func TestRightClickTablineClosesViewedTab(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt")
	m.active = 1

	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model, _ = model.Update(rightClickAt(spans[1][0], 0))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok)
	require.Len(t, normal.buffers, 2)
	assert.Equal(t, 1, normal.active)
	assert.Equal(t, "c.txt", normal.activeBuffer().path, "닫은 자리에 드러난 파일이다")
}

// tab 이 하나뿐이어도 닫는다. 닫으면 빈 화면이 남고 편집기는 끝나지 않는다(ADR-0064).
func TestRightClickLastTabShowsEmptyScreen(t *testing.T) {
	m := newTabsEditor("a.txt")

	var model tea.Model = m
	model, cmd := model.Update(rightClickAt(m.renderTabline(m.textWidth()).tabs[0][0], 0))

	empty, ok := model.(viewEditorEmpty)
	require.True(t, ok, "종료하지 않고 빈 화면이 된다: %T", model)
	assert.Empty(t, empty.buffers)
	assert.Nil(t, cmd, "드러날 파일이 없어서 트리를 데려가지 않는다")
}

// tab 이 없는 칸은 우클릭도 아무 일이 없다. 가려짐 표시는 밀지도 않는다.
func TestRightClickTablineNonTabColumns(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.width = 30
	require.Equal(t, " 1 a.txt │ 2 b.txt │.......│3>", tablineOf(t, m.View()))

	row := m.renderTabline(m.textWidth())

	for name, x := range map[string]int{
		"구분선":       row.tabs[0][1],
		"잘린 tab 자리": row.tabs[1][1] + 2,
		"가려짐 표시":    row.right[0],
	} {
		var model tea.Model = m
		model, _ = model.Update(rightClickAt(x, 0))

		normal, ok := model.(viewEditorNormal)
		require.True(t, ok)
		assert.Len(t, normal.buffers, 5, name)
		assert.Equal(t, " 1 a.txt │ 2 b.txt │.......│3>", tablineOf(t, normal.View()), name)
	}
}

// 보고 있지 않은 tab 이어도 저장하지 않은 변경이 있으면 묻는다. 물음에 파일 이름이 들어간다.
func TestRightClickDirtyTabAsks(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")
	m.buffers[1].dirty = true

	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model, _ = model.Update(rightClickAt(spans[1][0], 0))

	confirm, ok := model.(viewConfirmDiscard)
	require.True(t, ok, "확인창이 뜬다: %T", model)
	assert.Equal(t, "b.txt tab 을 닫으시겠습니까?", confirm.question)

	// No 로 나가면 tab 이 그대로 남는다.
	cancelled, _ := confirm.press("n")
	cancelled, _ = cancelled.(viewConfirmDiscard).press("enter")
	assert.Len(t, cancelled.(viewEditorNormal).buffers, 2, "취소하면 그대로다")

	closed, _ := confirm.press("enter")
	normal, ok := closed.(viewEditorNormal)
	require.True(t, ok)
	require.Len(t, normal.buffers, 1)
	assert.Equal(t, "a.txt", normal.activeBuffer().path)
}

// insert 로 치던 중에 옆 tab 을 닫아도 계속 친다. 고치던 buffer 는 그대로다.
func TestRightClickOtherTabKeepsInsert(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")
	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model = send(model, "i")
	require.IsType(t, viewEditorInsert{}, model)

	model, _ = model.Update(rightClickAt(spans[1][0], 0))

	insert, ok := model.(viewEditorInsert)
	require.True(t, ok, "insert 에 머문다: %T", model)
	require.Len(t, insert.buffers, 1)
	assert.Equal(t, "a.txt", insert.activeBuffer().path)
}

// 고치던 tab 을 닫았으면 insert 에 남을 수 없다. 남으면 다른 파일을 그 mode 로 고친다.
func TestRightClickViewedTabLeavesInsert(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")
	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model = send(model, "i")
	model, _ = model.Update(rightClickAt(spans[0][0], 0))

	normal, ok := model.(viewEditorNormal)
	require.True(t, ok, "normal 로 나간다: %T", model)
	assert.Equal(t, "b.txt", normal.activeBuffer().path)
}

// visual 로 고른 것이 남은 buffer 를 보고 있으면 범위도 mode 도 그대로다.
func TestRightClickOtherTabKeepsVisual(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")
	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = m
	model = send(model, "v", "l")
	require.IsType(t, viewEditorVisual{}, model)

	model, _ = model.Update(rightClickAt(spans[1][0], 0))

	visual, ok := model.(viewEditorVisual)
	require.True(t, ok, "visual 에 머문다: %T", model)
	require.Len(t, visual.buffers, 1)
	assert.True(t, visual.activeBuffer().selection.active, "고른 범위가 그대로다")
}

// 트리에 포커스를 두고 남의 tab 을 닫아도 포커스는 트리에 남는다. 보는 파일이 그대로다.
func TestRightClickOtherTabKeepsTreeFocus(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")
	spans := m.renderTabline(m.textWidth()).tabs

	var model tea.Model = viewSidebar{editor: m.editor}
	model, _ = model.Update(rightClickAt(spans[1][0], 0))

	tree, ok := model.(viewSidebar)
	require.True(t, ok, "포커스가 트리에 남는다: %T", model)
	require.Len(t, tree.buffers, 1)
	assert.Equal(t, "a.txt", tree.activeBuffer().path)
}
