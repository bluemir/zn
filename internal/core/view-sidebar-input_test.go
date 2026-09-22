package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openMenuItem 은 name 행을 우클릭해 at 번째 항목을 누른다.
// 트리는 따로 받는다 — 화면 model 마다 타입이 달라서 거기서 꺼내려면 타입 단언이 붙는다.
func openMenuItem(t *testing.T, m tea.Model, s sidebar, name string, at int) tea.Model {
	t.Helper()

	model, _ := m.Update(rightClickAt(2, treeRowOf(t, s, name)))

	menu, ok := model.(viewSidebarMenu)
	require.True(t, ok, "메뉴가 뜬다")

	next, _ := menu.Update(click(itemPos(menu, at)))

	return next
}

// typeBox 는 이름 상자에 한 글자씩 쳐 넣는다. 상자는 키를 동작이 아니라 글자로 먹는다.
func typeBox(t *testing.T, m tea.Model, name string) tea.Model {
	t.Helper()

	for _, c := range name {
		require.IsType(t, viewSidebarInput{}, m, "상자에 머문다")
		m = sendSync(t, m, string(c))
	}

	return m
}

// 상자에 이름을 치고 `enter` 하면 파일이 생기고 tab 으로 열린다.
func TestSidebarInputCreatesFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := openMenuItem(t, m, m.sidebar, "docs", 0)
	model = typeBox(t, model, "guide.md")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewEditorNormal{}, model, "만들자마자 쓰러 편집 영역으로 간다")

	data, err := os.ReadFile(filepath.Join(root, "docs", "guide.md"))
	require.NoError(t, err, "파일이 만들어져 있다")
	assert.Empty(t, data)
	assert.Len(t, model.(viewEditorNormal).buffers, 2, "tab 으로 열린다")
}

// `/` 로 끝나면 디렉터리다. 열 것이 없으므로 트리에 머문다. `mc` 와 같은 규칙이다.
func TestSidebarInputCreatesDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := openMenuItem(t, m, m.sidebar, "docs", 0)
	model = typeBox(t, model, "img/")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model, "디렉터리는 열 것이 없어 트리에 머문다")

	info, err := os.Stat(filepath.Join(root, "docs", "img"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// 파일을 골랐으면 그 파일이 있는 디렉터리에 만든다.
func TestSidebarInputCreatesBesideSelectedFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	// `docs` 를 펼쳐야 그 안의 파일이 트리에 선다.
	next, cmd := m.Update(click(2, treeRowOf(t, m.sidebar, "docs")))
	tree := settle(t, next, cmd).(viewSidebar)

	model := openMenuItem(t, tree, tree.sidebar, "spec.md", 0)
	model = typeBox(t, model, "note.md")
	model = sendSync(t, model, "enter")

	_, err := os.Stat(filepath.Join(root, "docs", "note.md"))
	assert.NoError(t, err, "고른 파일 옆에 만든다")
}

// `esc` 는 메뉴가 아니라 메뉴를 열기 전 화면으로 돌아간다. 그만두려고 두 번 누르지 않는다.
func TestSidebarInputEscapeReturnsToParent(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := openMenuItem(t, m, m.sidebar, "docs", 0)
	model = typeBox(t, model, "guide.md")
	model, _ = model.Update(key("esc"))

	assert.IsType(t, viewEditorNormal{}, model, "누르기 전 화면으로 돌아간다")

	_, err := os.Stat(filepath.Join(root, "docs", "guide.md"))
	assert.Error(t, err, "만들지 않았다")
}

// 이름을 다 지워도 상자에 남는다. 그만두는 것은 `esc` 하나다.
func TestSidebarInputStaysWhenEmptied(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := openMenuItem(t, m, m.sidebar, "README.md", 1)
	for range len("README.md") {
		model = sendSync(t, model, "backspace")
	}

	input, ok := model.(viewSidebarInput)
	require.True(t, ok, "상자에 머문다")
	assert.Empty(t, input.input.text)
}

// 클릭은 먹지 않는다. 치던 이름이 클릭 한 번에 조용히 사라지면 안 된다.
func TestSidebarInputIgnoresClick(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := openMenuItem(t, m, m.sidebar, "docs", 0)
	model = typeBox(t, model, "guide.md")
	model, _ = model.Update(click(2, 2))

	input, ok := model.(viewSidebarInput)
	require.True(t, ok, "상자에 머문다")
	assert.Equal(t, "guide.md", input.input.text, "치던 이름이 남는다")
}

// 새 이름은 뿌리 기준 경로를 고치는 것이다. 앞을 고치면 자리를 옮기는 것이 된다.
func TestSidebarInputRenamesAndMoves(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := openMenuItem(t, m, m.sidebar, "main.go", 1)
	for range len("main.go") {
		model = sendSync(t, model, "backspace")
	}
	model = typeBox(t, model, "docs/app.go")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model, "이름을 바꾸고 트리에 머문다")

	_, err := os.Stat(filepath.Join(root, "docs", "app.go"))
	require.NoError(t, err, "옮겨간 자리에 있다")

	_, err = os.Stat(filepath.Join(root, "main.go"))
	assert.Error(t, err, "옛 자리에는 없다")
}

// 이름이 비면 알리고 만들지 않는다. `mc` 와 같은 말을 한다.
func TestSidebarInputRefusesEmptyName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := openMenuItem(t, m, m.sidebar, "docs", 0)
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "이름이 없습니다")
}

// 상자도 화면 밖으로 나가지 않는다(ADR-0011).
func TestSidebarInputBoxStaysOnScreen(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model, _ := m.Update(rightClickAt(m.sidebar.width-1, m.sidebarHeight()-1))
	menu := model.(viewSidebarMenu)

	next, _ := menu.Update(click(itemPos(menu, 0)))

	input, ok := next.(viewSidebarInput)
	require.True(t, ok)
	assert.LessOrEqual(t, input.left+input.inner()+2, m.width, "오른쪽으로 나가지 않는다")
	assert.LessOrEqual(t, input.top+sidebarInputRows, m.height-statusBarHeight, "statusBar 를 덮지 않는다")
}
