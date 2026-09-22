package core

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// treeRowOf 는 이름이 name 인 항목의 화면 행이다. 우클릭 좌표를 만들 때 쓴다.
func treeRowOf(t *testing.T, s sidebar, name string) int {
	t.Helper()

	for i, row := range s.rows() {
		if row.node.name == name {
			return i - s.top
		}
	}

	t.Fatalf("%q 를 찾지 못했다", name)

	return -1
}

// labelsOf 는 메뉴에 선 항목 이름이다.
func labelsOf(menu viewSidebarMenu) []string {
	out := []string{}
	for _, item := range menu.items() {
		out = append(out, item.label)
	}

	return out
}

// itemPos 는 메뉴의 at 번째 항목을 누를 좌표다.
func itemPos(menu viewSidebarMenu, at int) (x, y int) {
	return menu.left, menu.top + menuHeader + at
}

// 파일을 우클릭하면 그 파일의 메뉴가 뜬다. 무엇에 대한 메뉴인지가 상자 첫 줄에 있다.
func TestSidebarRightClickOpensMenuForRow(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))

	menu, ok := model.(viewSidebarMenu)
	require.True(t, ok, "메뉴가 뜬다")
	assert.Equal(t, filepath.Join(m.sidebar.root, "README.md"), menu.path)
	assert.Equal(t, []string{"새 파일", "새 이름", "지우기"}, labelsOf(menu))
	assert.Contains(t, menu.View().Content, "README.md")
}

// 우클릭한 행이 곧 고른 행이다. 메뉴를 닫고 트리로 가면 그 자리에서 이어 하게 된다.
func TestSidebarRightClickSelectsRow(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "main.go")

	model, _ := m.Update(rightClickAt(2, row))

	menu := model.(viewSidebarMenu)
	require.NotNil(t, menu.sidebar.selectedNode())
	assert.Equal(t, "main.go", menu.sidebar.selectedNode().name)
}

// 디렉터리는 이름 뒤에 `/` 가 붙어 파일과 갈린다.
func TestSidebarRightClickOnDirMarksIt(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "docs")

	model, _ := m.Update(rightClickAt(2, row))

	menu := model.(viewSidebarMenu)
	assert.True(t, menu.isDir)
	assert.Equal(t, "docs/", menu.title())
}

// 뿌리는 지울 수도 이름을 바꿀 수도 없다. 항목이 「새 파일」 하나다.
func TestSidebarRightClickOnRootShowsCreateOnly(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, filepath.Base(m.sidebar.root))

	model, _ := m.Update(rightClickAt(2, row))

	menu := model.(viewSidebarMenu)
	assert.True(t, menu.isRoot)
	assert.Equal(t, []string{"새 파일"}, labelsOf(menu))

	// relLabel 이 뿌리에는 이미 `/` 를 붙여 준다. 그것을 보지 않으면 `zn//` 가 된다.
	assert.Equal(t, filepath.Base(m.sidebar.root)+"/", menu.title())
}

// 항목이 없는 아래쪽도 뿌리 메뉴다. 빈 곳에도 「거기에 만든다」는 뜻이 있다.
func TestSidebarRightClickBelowRowsUsesRoot(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	below := len(m.sidebar.rows()) + 1
	require.Less(t, below, m.sidebarHeight(), "트리 아래에 빈 행이 남아 있다")

	model, _ := m.Update(rightClickAt(2, below))

	menu, ok := model.(viewSidebarMenu)
	require.True(t, ok)
	assert.Equal(t, m.sidebar.root, menu.path)
	assert.Equal(t, []string{"새 파일"}, labelsOf(menu))
}

// 구분선은 끌어서 폭을 바꾸는 한 칸이다. 거기서 메뉴가 뜨면 잡으려다 누른 것이 화면을 덮는다.
func TestSidebarRightClickOnDividerDoesNothing(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model, _ := m.Update(rightClickAt(m.sidebarDividerCol(), 2))

	assert.IsType(t, viewEditorNormal{}, model, "메뉴가 뜨지 않는다")
}

// `… 읽는 중` 은 파일이 아니라 안내다. 짚을 자리가 없으므로 알리고 포커스는 그대로 둔다.
func TestSidebarRightClickOnPlaceholderKeepsFocus(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	build := m.sidebar.rows()[treeRowOf(t, m.sidebar, "build")].node
	build.expanded, build.loading = true, true

	row := treeRowOf(t, m.sidebar, "… 읽는 중")
	model, _ := m.Update(rightClickAt(2, row))

	require.IsType(t, viewEditorNormal{}, model, "메뉴가 뜨지 않고 포커스도 그대로다")
	assert.Contains(t, barOf(t, model)[1], "읽는 중입니다")
}

// 상자 바깥을 누르면 닫히기만 한다. 그 자리의 동작까지 하면 닫으려던 손이 파일을 연다.
func TestSidebarMenuOutsideClickOnlyCloses(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)

	back, _ := menu.Update(click(menu.left+menu.boxWidth()+1, menu.top))

	require.IsType(t, viewEditorNormal{}, back, "누르기 전 화면으로 돌아간다")
	assert.Len(t, back.(viewEditorNormal).buffers, 1, "파일이 열리지 않았다")
}

// `esc` 는 닫는 길이다. 우클릭 직전이 insert 였으면 insert 로 돌아간다.
func TestSidebarMenuEscapeReturnsToParent(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	insert, _ := insertMode(m.editor)
	model, _ := insert.Update(rightClickAt(2, row))
	require.IsType(t, viewSidebarMenu{}, model)

	back, _ := model.Update(key("esc"))

	assert.IsType(t, viewEditorInsert{}, back, "치던 화면으로 돌아간다")
}

// 「새 파일」은 메뉴가 섰던 자리에서 이름을 받는다.
func TestSidebarMenuCreateOpensBoxAtMenu(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "docs")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)

	next, _ := menu.Update(click(itemPos(menu, 0)))

	input, ok := next.(viewSidebarInput)
	require.True(t, ok, "이름 상자로 간다")
	assert.Equal(t, "새 파일", input.prompt)
	assert.Empty(t, input.input.text, "빈 칸에서 시작한다")
	assert.Contains(t, input.View().Content, "새 파일: ")
}

// 「새 이름」은 지금 경로가 채워진 채 뜬다. 한 글자만 고치는 것이 이 동작의 거의 전부다.
func TestSidebarMenuRenameStartsFilled(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)

	next, _ := menu.Update(click(itemPos(menu, 1)))

	input, ok := next.(viewSidebarInput)
	require.True(t, ok)
	assert.Equal(t, "새 이름", input.prompt)
	assert.Equal(t, "README.md", input.input.text)
}

// 메뉴의 지우기는 깨끗한 파일도 묻는다. `md` 와 갈리는 자리다(ADR-0057, ADR-0060).
func TestSidebarMenuDeleteAlwaysAsks(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)

	next, _ := menu.Update(click(itemPos(menu, 2)))

	confirm, ok := next.(viewSidebarDelete)
	require.True(t, ok, "확인창이 뜬다")
	assert.Equal(t, filepath.Join(m.sidebar.root, "README.md"), confirm.path)
	assert.Contains(t, confirm.View().Content, "지우면 되돌릴 수 없습니다.")
}

// 상자는 화면 밖으로 나가지 않는다. 한 칸이라도 나가면 캔버스가 커져 화면이 밀린다(ADR-0011).
func TestSidebarMenuBoxStaysOnScreen(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	// 트리 오른쪽 끝, 아래로는 자리가 없는 행에서 누른다.
	model, _ := m.Update(rightClickAt(m.sidebar.width-1, m.sidebarHeight()-1))
	menu := model.(viewSidebarMenu)

	assert.LessOrEqual(t, menu.left+menu.boxWidth(), m.width, "오른쪽으로 나가지 않는다")
	assert.LessOrEqual(t, menu.top+len(menu.items())+menuFrame, m.height-statusBarHeight, "statusBar 를 덮지 않는다")

	rows := strings.Split(menu.View().Content, "\n")
	assert.Len(t, rows, m.height, "화면 높이가 그대로다")
	for i, line := range rows {
		assert.LessOrEqual(t, textarea.WidthOf(ansi.Strip(line)), m.width, "행 %d 가 화면보다 넓다", i)
	}
}

// 파일을 하나도 열지 않은 첫 화면에도 트리는 있다. 닫을 tab 이 없을 뿐이다(ADR-0064).
func TestSidebarRightClickWorksOnEmptyScreen(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	m.editor.buffers = nil

	empty := viewEditorEmpty{editor: m.editor}
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := empty.Update(rightClickAt(2, row))

	menu, ok := model.(viewSidebarMenu)
	require.True(t, ok, "빈 화면에서도 메뉴가 뜬다")
	assert.Equal(t, filepath.Join(m.sidebar.root, "README.md"), menu.path)
}

// 굴려도 메뉴는 그대로다. 트리가 밀리면 메뉴가 가리키는 행이 화면과 어긋난다.
func TestSidebarMenuIgnoresWheel(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)

	next, _ := menu.Update(wheel(2, 2, false))

	assert.IsType(t, viewSidebarMenu{}, next, "메뉴에 머문다")
	assert.Equal(t, menu.sidebar.top, next.(viewSidebarMenu).sidebar.top, "트리가 굴러가지 않는다")
}

// moveTo 는 마우스가 움직인 자리다. hover 를 확인할 때 쓴다.
func moveTo(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseNone}
}

// 포인터가 얹힌 항목을 따라간다. 열릴 때는 어느 항목 위에도 없다.
func TestSidebarMenuTracksHover(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)
	require.Equal(t, -1, menu.hover, "상자가 누른 칸 아래에 서므로 처음에는 얹힌 것이 없다")

	for at := range menu.items() {
		x, y := itemPos(menu, at)
		next, _ := menu.Update(moveTo(x, y))
		assert.Equal(t, at, next.(viewSidebarMenu).hover, "%d 번째 항목 위다", at)
	}
}

// 상자 밖과 테두리·이름 줄에서는 얹힌 것이 없다.
func TestSidebarMenuHoverLeavesBox(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)

	x, y := itemPos(menu, 0)
	menu = must(menu.Update(moveTo(x, y))).(viewSidebarMenu)
	require.Equal(t, 0, menu.hover)

	for name, at := range map[string][2]int{
		"이름 줄":   {x, menu.top + 1},
		"아래 테두리": {x, menu.top + menuHeader + len(menu.items())},
		"상자 오른쪽": {menu.left + menu.boxWidth(), y},
		"상자 왼쪽":  {menu.left - 1, y},
	} {
		next, _ := menu.Update(moveTo(at[0], at[1]))
		assert.Equal(t, -1, next.(viewSidebarMenu).hover, "%s 는 항목이 아니다", name)
	}
}

// 얹힌 항목만 바탕이 칠해진다. 고른 것이 아니라 「지금 이 위에 있다」라서 반전은 쓰지 않는다.
func TestSidebarMenuPaintsHoveredItem(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	row := treeRowOf(t, m.sidebar, "README.md")

	model, _ := m.Update(rightClickAt(2, row))
	menu := model.(viewSidebarMenu)
	require.NotContains(t, menu.renderBox(), "\x1b[", "얹힌 것이 없으면 아무 줄도 칠하지 않는다")

	x, y := itemPos(menu, 1)
	painted := must(menu.Update(moveTo(x, y))).(viewSidebarMenu).renderBox()

	lines := strings.Split(painted, "\n")
	require.Len(t, lines, len(menu.items())+menuFrame)

	for at, line := range lines {
		want := at == menuHeader+1
		assert.Equal(t, want, strings.Contains(line, "\x1b["), "행 %d: %q", at, line)
	}

	// 칠해도 폭은 그대로다. 한 칸이라도 늘면 상자가 화면 밖으로 나간다.
	for at, line := range lines {
		assert.Equal(t, menu.boxWidth(), textarea.WidthOf(ansi.Strip(line)), "행 %d", at)
	}
}

// hover 를 받으려면 mouse 를 AllMotion 으로 올려야 한다. 메뉴가 떠 있는 동안만이다.
func TestSidebarMenuRaisesMouseModeWhileOpen(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	require.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode)

	row := treeRowOf(t, m.sidebar, "README.md")
	model, _ := m.Update(rightClickAt(2, row))

	assert.Equal(t, tea.MouseModeAllMotion, model.View().MouseMode, "메뉴는 움직임까지 받는다")

	back, _ := model.Update(key("esc"))
	assert.Equal(t, tea.MouseModeCellMotion, back.View().MouseMode, "닫으면 되돌아간다")

	// 이름 상자는 고를 항목이 없다. 올려 둘 까닭이 없다.
	next, _ := model.(viewSidebarMenu).Update(click(itemPos(model.(viewSidebarMenu), 0)))
	assert.Equal(t, tea.MouseModeCellMotion, next.View().MouseMode)
}

// tabline 우클릭은 그대로 tab 을 닫는다. 메뉴가 그 길을 가로채지 않는다(ADR-0060).
func TestRightClickOnTablineStillClosesTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "main.go")
	model = sendSync(t, model, "enter")

	view, ok := model.(viewEditorNormal)
	require.True(t, ok)
	require.Len(t, view.buffers, 2, "tab 이 둘이다")

	closed, _ := view.Update(rightClickAt(view.sidebarLeft()+1, 0))

	assert.Len(t, closed.(viewEditorNormal).buffers, 1, "누른 tab 이 닫힌다")
}
