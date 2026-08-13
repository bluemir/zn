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

	assert.Equal(t, 0, cursorLineOf(t, m), "오른쪽 버튼은 아무 일도 하지 않는다")
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
	_, spans := m.tabline(m.textWidth())
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

	_, spans := m.tabline(m.textWidth())
	require.Len(t, spans, 3)

	for i, span := range spans {
		var model tea.Model = m
		model, _ = model.Update(click(span[1]-1, 0)) // 그 tab 의 마지막 칸

		normal, ok := model.(viewEditorNormal)
		require.True(t, ok)
		assert.Equal(t, i, normal.active, "%d 번째 tab 의 끝 칸", i+1)
	}
}

func TestClickTablineFillerDoesNothing(t *testing.T) {
	m := newTestEditor("abc\n", 60, 5)
	m.buffers = append(m.buffers, newBuffer("second.txt", []byte("xyz\n")))

	_, spans := m.tabline(m.textWidth())

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
	assert.Equal(t, "w", command.input, "치던 명령이 그대로다")
	assert.Equal(t, 0, command.buffers[command.active].cursorLine, "커서도 그대로다")
}

func TestClickIgnoredWhileTypingSearch(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 40, 5)
	left := contentLeftOf(t, m)

	m = send(m, "/", "d")
	m, _ = m.Update(click(left+1, tablineHeight))

	search, ok := m.(viewEditorSearch)
	require.True(t, ok, "검색에 남는다")
	assert.Equal(t, "d", search.input)
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
