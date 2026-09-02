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
// 그리는 쪽(renderScreen) 과 판별하는 쪽이 어긋나면 한 칸 옆을 누른 것이 되므로
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

	// tab 이 없으면 누를 tab 도 본문도 없다. 빈 화면은 sidebar 만 받는다 — 여기 하나로
	// 막으면 clickTabline·clickText·wheel·드래그가 다 같이 조용해진다(ADR-0064).
	if !e.hasTab() {
		return regionNone
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
// 부르면 wrap 된 줄 안에 스크롤해 둔 상태에서 화면이 튄다. **머리줄만 예외다** — 아래를 보라.
func (e *editor) clickText(x, y int) {
	buf := e.activeBuffer()
	row := y - tablineHeight

	// 머리줄을 누르면 그 줄로 간다. VSCode 와 같다(ADR-0049).
	//
	// **무시하는 선택지가 없다.** 이 자리는 본문을 덮고 있어서 그대로 positionAt 에 넘기면
	// 가려진 줄로 커서가 간다 — 눌러서 보이는 글자와 커서가 어긋난다.
	//
	// 여기서는 화면을 옮긴다. 가려는 곳이 화면 밖이라 위의 「이미 보이는 자리」가 아니다.
	if sticky := buf.StickyAt(buf.Top.Line, e.textHeight()); row < len(sticky) {
		buf.MoveToLine(sticky[row])
		e.scrollToCursor()

		return
	}

	line, col, ok := buf.PositionAt(x-e.contentLeft(), row, e.textHeight())
	if !ok {
		return
	}

	buf.MoveTo(line, col)
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
// 구분선과 오른쪽 빈 칸, 잘린 tab 자리의 점처럼 tab 이 없는 칸이면 아무것도 하지 않는다.
//
// 양끝의 가려짐 표시를 누르면 보고 있는 tab 은 그대로 두고 그 방향으로 한 칸 민다(ADR-0029).
// 지금 편집하는 것을 놓지 않고 가려진 쪽에 무엇이 있는지 훑을 수 있어야 한다.
//
// 옮겨간 tab 자리를 트리가 아직 읽지 않았으면 읽는 작업이 시작되므로 Cmd 가 나온다. `gt` 와 같다.
func (e *editor) clickTabline(x int) tea.Cmd {
	row := e.renderTabline(e.textWidth())

	col := x - e.sidebarLeft()
	switch {
	case inSpan(row.left, col):
		e.tabScroll = max(e.tabScroll-1, 0)

		return nil
	case inSpan(row.right, col):
		e.tabScroll = min(e.tabScroll+1, len(e.buffers)-1)

		return nil
	}

	index := row.tabAt(col)
	if index < 0 {
		return nil
	}

	e.active = index

	// 그 buffer 는 이 창 크기를 본 적이 없을 수 있다. gt 와 같은 처리다.
	e.scrollToCursor()

	return e.revealInSidebar(e.activeBuffer().Path)
}

// rightClick 은 오른쪽 버튼을 먹는다. tabline 의 tab 을 닫고 다른 영역에서는 아무 일도 없다(ADR-0060).
//
// mode 마다 갈라 두지 않았다. 닫는 일은 지금 어느 mode 인지와 상관이 없고, mode 를 옮겨야
// 하는 경우(보고 있던 tab 을 닫았다) 는 닫는 쪽이 안다. 왼쪽 버튼이 mode 마다 다른 것은
// 커서·포커스·insert 유지 때문인데 여기에는 그런 것이 없다.
//
// parent 는 닫을 것이 없거나 확인창에서 취소했을 때 돌아갈 화면이다.
func rightClick(parent tea.Model, e *editor, mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if e.regionAt(mouse.X, mouse.Y) != regionTabline {
		return parent, nil
	}

	// 구분선과 오른쪽 빈 칸, 잘린 tab 자리의 점은 tab 이 아니다. 양끝의 가려짐 표시도
	// 여기서는 tab 이 아니라 아무 일도 하지 않는다 — 미는 것과 닫는 것을 한 버튼에 섞으면
	// 한 칸 잘못 눌렀을 때 잃는 것이 화면 이동으로 끝나지 않는다.
	index := e.renderTabline(e.textWidth()).tabAt(mouse.X - e.sidebarLeft())
	if index < 0 {
		return parent, nil
	}

	return closeTabAt(parent, e, index)
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
		e.activeBuffer().ScrollBy(rows, e.textHeight())
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
		m.activeBuffer().ClampToNormal()
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
		m.activeBuffer().EndEdit()

		return m, m.clickTabline(mouse.X)
	case regionText:
		// 커서를 옮기면 undo 구간이 끊긴다. 화살표 이동과 같다. vim 과 같다.
		m.activeBuffer().EndEdit()
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
		m.activeBuffer().ClampToNormal()

		return normalMode(m.editor)
	}

	return m, nil
}

// dragTo 는 끌린 자리로 커서를 옮긴다. anchor 는 그대로라 고른 범위가 그만큼 자란다.
//
// **편집 영역 밖으로 나간 좌표는 안으로 당겨서 읽는다.** sidebar 쪽으로 끌면 줄 시작이고
// statusBar 아래로 끌면 맨 아랫행이다. 끄는 중이라 「다른 영역을 눌렀다」가 아니므로
// 포커스는 옮기지 않는다.
//
// **위아래로 나갔으면 그 방향으로 한 행 굴린다.** motion 이벤트는 포인터가 실제로 움직일 때만
// 오므로(MouseModeCellMotion) 잡은 채 가만히 있으면 굴러가지 않는다. 타이머는 두지 않았다.
func (e *editor) dragTo(x, y int) {
	buf, height := e.activeBuffer(), e.textHeight()
	if height < 1 {
		return
	}

	// 머리줄이 덮은 자리는 편집 영역 위로 나간 것과 같이 다룬다 — 그리로 끌면 위로 굴려서
	// 가려진 줄을 드러낸다. 머리줄이 없으면 sticky 가 0 이라 예전과 같다(ADR-0049).
	sticky := len(buf.StickyAt(buf.Top.Line, height))

	row := y - tablineHeight
	switch {
	case row < sticky:
		buf.ScrollBy(-1, height)

		row = len(buf.StickyAt(buf.Top.Line, height))
	case row >= height:
		buf.ScrollBy(1, height)

		row = height - 1
	}

	line, col, ok := buf.PositionAt(x-e.contentLeft(), row, height)
	if !ok {
		// 마지막 줄 아래로 끌었다. 클릭은 그 자리에서 멈추지만(positionAt 주석) 끄는 중에는
		// 있는 데까지 따라가야 한다 — 범위가 손을 놓치면 어디까지 골랐는지 알 수 없다.
		rows := buf.VisibleRows(height)
		if len(rows) < 1 {
			return
		}

		last := rows[len(rows)-1]
		line, col = last.Line, last.End
	}

	buf.MoveTo(line, col)
	buf.ClampToNormal()
}

// dragTab 은 끌린 자리로 활성 tab 을 옮긴다(ADR-0090).
//
// 끄는 동안 순서가 실제로 바뀐다. 그래서 놓을 자리를 알리는 표시를 따로 그릴 것이 없다 —
// 다음 프레임의 tabline 이 그것이다.
//
// **tabline 밖은 아무 일도 하지 않는다.** 끌던 자리를 유지하고 포인터가 돌아오면 이어 옮긴다.
// 끌어내서 닫는 길을 만들지 않은 것은 그것이 tab 을 잃는 일이고 오른쪽 버튼이 이미 하기
// 때문이다(ADR-0060).
func (e *editor) dragTab(x, y int) {
	if y >= tablineHeight {
		return
	}

	row := e.renderTabline(e.textWidth())
	col := x - e.sidebarLeft()

	// **양끝의 가려짐 표시 위는 tab 이 아니다.** 누르면 화면을 미는 자리인데, 끌면서 밀기
	// 시작하면 손이 멈춰도 계속 흘러간다 — motion 은 포인터가 실제로 움직일 때만 오므로
	// 멈추게 하려면 타이머가 하나 생긴다(ADR-0029, dragTo 가 한 행씩만 굴리는 것과 같다).
	if inSpan(row.left, col) || inSpan(row.right, col) {
		return
	}

	e.moveTab(row.tabAt(col))
}

// click 은 visual mode 에서 왼쪽 버튼을 먹는다. 누른 자리가 새 시작이라 visual 이 끝난다.
func (m viewEditorVisual) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	region := m.regionAt(mouse.X, mouse.Y)
	if region == regionNone {
		// statusBar 와 화면 밖이다. 아무 일도 하지 않는다 — 다른 mode 와 같다.
		return m, nil
	}

	// tab 을 옮기기 전에 놓아야 한다. 옮기고 나면 놓을 Buffer 가 바뀌어서 고른 범위가
	// 보이지 않는 tab 에 남는다.
	m.activeBuffer().ClearSelection()

	switch region {
	case regionSidebar:
		return m.clickSidebar(mouse.Y)
	case regionTabline:
		reveal := m.clickTabline(mouse.X)

		next, cmd := normalMode(m.editor)

		return next, tea.Batch(cmd, reveal)
	}

	m.clickText(mouse.X, mouse.Y)
	m.activeBuffer().ClampToNormal()

	return normalMode(m.editor)
}
