package core

import (
	tea "charm.land/bubbletea/v2"
)

// region 은 화면 좌표가 어느 영역인지다.
//
// mouse 는 키와 달리 "무엇을 눌렀는가" 가 좌표로만 오므로, 좌표를 뜻으로 바꾸는 단계가
// 먼저 있어야 한다. 키의 상태 기계(ADR-0006) 가 키 나열을 동작으로 바꾸는 것과 같은 자리다.
type region int

const (
	// regionNone 은 눌러도 할 일이 없는 자리다. statusBar 와 화면 밖이다.
	regionNone region = iota
	regionSidebar
	regionTabline

	// regionText 는 편집 영역이다. 줄번호 칸도 여기에 든다.
	regionText
)

// regionAt 은 화면 좌표가 어느 영역인지 돌려준다.
//
// 그리는 쪽(screenRows) 과 판별하는 쪽이 어긋나면 한 칸 옆을 누른 것이 되므로
// 수치를 새로 두지 않고 render 가 쓰는 geometry 를 그대로 쓴다.
func (e editor) regionAt(x, y int) region {
	if x < 0 || y < 0 || x >= e.width || y >= e.height {
		return regionNone
	}

	// sidebar 를 tabline 보다 먼저 본다. sidebar 는 화면 맨 윗줄부터 시작하고
	// tabline 은 그 오른쪽에서만 그려진다(ADR-0005).
	//
	// open 이 아니라 sidebarVisible 을 본다 — 좁은 화면에서는 켜져 있어도 그리지 않는다.
	if e.sidebarVisible() && x < sidebarWidth && y < e.sidebarHeight() {
		return regionSidebar
	}

	switch {
	case y < tablineHeight:
		return regionTabline
	case y < tablineHeight+e.textHeight():
		return regionText
	default:
		return regionNone
	}
}

// clickText 는 편집 영역 좌표로 커서를 옮긴다. 그 자리에 글자가 없으면 아무것도 하지 않는다.
//
// scrollTo 는 부르지 않는다. 이미 보이는 자리를 눌렀으니 화면이 움직일 이유가 없고,
// 부르면 wrap 된 줄 안에 스크롤해 둔 상태에서 화면이 튄다.
func (e *editor) clickText(x, y int) {
	buf := e.activeBuffer()

	line, col, ok := buf.positionAt(x-e.contentLeft(), y-tablineHeight, e.contentWidth(), e.textHeight())
	if !ok {
		return
	}

	buf.moveTo(line, col, e.contentWidth())
}

// clickSidebar 는 sidebar 좌표의 항목을 고르고 연다.
//
// 고르기와 열기를 클릭 한 번으로 같이 한다. double-click 을 알 수 없고
// (bubbletea 의 Mouse 에 누른 횟수가 없다) 트리에서는 이것이 오히려 `enter` 와 같아서 자연스럽다.
func (e *editor) clickSidebar(y int) (tea.Model, tea.Cmd) {
	if !e.sidebar.selectRow(y, e.sidebarHeight()) {
		return viewSidebar{editor: e}, nil
	}

	// enter 가 디렉터리 토글과 파일 열기를 다 한다. 파일을 열면 포커스도 편집 영역으로 보낸다.
	return viewSidebar{editor: e}.enter()
}

// clickTabline 은 tabline 좌표의 tab 으로 옮겨간다.
// 구분선과 오른쪽 빈 칸처럼 tab 이 없는 칸이면 아무것도 하지 않는다.
//
// 양끝의 가려짐 표시를 누르면 보고 있는 tab 은 그대로 두고 그 방향으로 한 칸 민다(ADR-0029).
// 지금 편집하는 것을 놓지 않고 가려진 쪽에 무엇이 있는지 훑을 수 있어야 한다.
func (e *editor) clickTabline(x int) tea.Cmd {
	row := e.tabline(e.textWidth())

	col := x - e.sidebarLeft()
	switch {
	case inSpan(row.left, col):
		e.tabScroll = max(e.tabScroll-1, 0)

		return nil
	case inSpan(row.right, col):
		e.tabScroll = min(e.tabScroll+1, len(e.buffers)-1)

		return nil
	}

	index := -1
	for i, span := range row.tabs {
		if inSpan(span, col) {
			index = i

			break
		}
	}
	if index < 0 {
		return nil
	}

	e.active = index

	// 그 buffer 는 이 창 크기를 본 적이 없을 수 있다. gt 와 같은 처리다.
	e.activeBuffer().scrollTo(e.contentWidth(), e.textHeight())

	return e.revealInSidebar(e.activeBuffer().path)
}

// wheelRows 는 휠 한 번에 굴리는 화면 행 수다.
// 한 행은 너무 느리고 화면 절반은 어지럽다. vim 의 `scroll` 은 ctrl+d 용이라 휠에는 과하다.
const wheelRows = 3

// wheel 은 휠을 먹는다. 포커스가 아니라 포인터가 얹힌 영역이 굴러간다 —
// 편집 영역을 보면서 트리만 굴려볼 수 있어야 한다.
//
// 버튼을 누르지 않은 이동은 알려주지 않지만(MouseModeCellMotion) 휠 이벤트가 그 순간의
// 좌표를 실어 오므로 그것으로 충분하다. AllMotion 으로 올리면 이벤트가 계속 쏟아진다.
func (e *editor) wheel(mouse tea.Mouse) {
	rows := wheelRows

	switch mouse.Button {
	case tea.MouseWheelUp:
		rows = -rows
	case tea.MouseWheelDown:
	default:
		// 좌우 휠은 쓰지 않는다. 가로 스크롤이 없다(ADR-0001).
		return
	}

	switch e.regionAt(mouse.X, mouse.Y) {
	case regionSidebar:
		e.sidebar.scrollBy(rows, e.sidebarHeight())
	case regionText:
		e.activeBuffer().scrollBy(rows, e.contentWidth(), e.textHeight())
	}
}

// click 은 normal mode 에서 왼쪽 버튼을 먹는다.
func (m viewEditorNormal) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	switch m.regionAt(mouse.X, mouse.Y) {
	case regionSidebar:
		return m.clickSidebar(mouse.Y)
	case regionTabline:
		// 옮겨간 tab 의 파일 자리를 트리가 아직 안 읽었으면 읽는 작업이 시작된다(ADR-0032).
		return m, m.clickTabline(mouse.X)
	case regionText:
		m.clickText(mouse.X, mouse.Y)

		// normal 의 커서는 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
		m.activeBuffer().clampToNormal(m.contentWidth())
	}

	return m, nil
}

// click 은 insert mode 에서 왼쪽 버튼을 먹는다.
// 커서만 옮기고 insert 에 머문다 — 눌러서 자리를 잡고 이어 치는 것이 mouse 를 쓰는 이유다.
func (m viewEditorInsert) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	switch m.regionAt(mouse.X, mouse.Y) {
	case regionSidebar:
		return m.clickSidebar(mouse.Y)
	case regionTabline:
		m.activeBuffer().endEdit()

		return m, m.clickTabline(mouse.X)
	case regionText:
		// 커서를 옮기면 undo 구간이 끊긴다. 화살표 이동과 같다. vim 과 같다.
		m.activeBuffer().endEdit()
		m.clickText(mouse.X, mouse.Y)
	}

	return m, nil
}

// click 은 sidebar 에 포커스가 있을 때 왼쪽 버튼을 먹는다.
func (m viewSidebar) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	switch m.regionAt(mouse.X, mouse.Y) {
	case regionSidebar:
		return m.clickSidebar(mouse.Y)
	case regionTabline:
		reveal := m.clickTabline(mouse.X)

		// 편집 영역을 누른 것이므로 포커스도 그리로 간다.
		model, cmd := normalMode(m.editor)

		return model, tea.Batch(cmd, reveal)
	case regionText:
		m.clickText(mouse.X, mouse.Y)
		m.activeBuffer().clampToNormal(m.contentWidth())

		return normalMode(m.editor)
	}

	return m, nil
}
