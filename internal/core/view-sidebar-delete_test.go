package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deleteBoxOf 는 지우기 물음 창에 그려진 글이다. 색을 뺀 채로 준다.
func deleteBoxOf(t *testing.T, m tea.Model) string {
	t.Helper()

	v, ok := m.(viewSidebarDelete)
	require.True(t, ok, "지우기 물음 창이 아니다")

	return ansi.Strip(v.renderBox(v.boxWidth()))
}

// 깨끗한 파일은 `md` 두 키로 곧바로 지운다. 묻지 않는다(ADR-0057).
func TestSidebarDeleteRemovesFileWithoutAsking(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "m", "d")

	require.IsType(t, viewSidebar{}, model, "묻지 않고 트리에 머문다")

	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "지워졌다")

	v := model.(viewSidebar)
	assert.NotContains(t, names(v.sidebar.rows()), "1:README.md", "트리를 다시 읽어서 그 행이 없다")

	// 묻지 않으므로 이 알림이 무엇이 사라졌는지 말하는 유일한 자리다.
	assert.Contains(t, barOf(t, model)[1], "지웠습니다: README.md")
}

// 디렉터리는 묻는다. 안의 것까지 사라지는데 접혀 있으면 이름 한 줄만 보인다.
func TestSidebarDeleteAsksForDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "d")

	require.IsType(t, viewSidebarDelete{}, model, "디렉터리는 한 번 더 묻는다")

	box := deleteBoxOf(t, model)
	assert.Contains(t, box, "안의 것까지 모두 지웁니다.")
	assert.Contains(t, box, "docs/ 를 지울까요?")
	assert.NotContains(t, box, "저장하지 않은", "열어둔 tab 이 없으면 그 줄이 없다")

	_, err := os.Stat(filepath.Join(root, "docs"))
	assert.NoError(t, err, "묻는 동안에는 아직 그대로다")
}

// 창은 트리 위에 뜨고 트리를 가리지 않는다. 커서가 지울 행에 그대로 남아서 무엇을 지우는지
// 창과 트리 두 곳에서 맞춰 볼 수 있다(ADR-0054, ADR-0130).
func TestSidebarDeleteBoxKeepsCursorOnTreeRow(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	tree := selectTree(t, tea.Model(m), "docs")
	treeRow, ok := tree.(viewSidebar).sidebar.selectedRow(tree.(viewSidebar).sidebarHeight())
	require.True(t, ok)

	model := send(tree, "m", "d")
	require.IsType(t, viewSidebarDelete{}, model)

	view := model.View()
	require.NotNil(t, view.Cursor)
	assert.Equal(t, treeRow, view.Cursor.Position.Y, "커서는 지울 행 위에 그대로다")
	assert.Equal(t, 0, view.Cursor.Position.X)
}

// `y` 는 고르기만 한다. `enter` 까지 와야 지운다 — 잘못 누른 한 키가 실행이 되지 않는다.
func TestSidebarDeleteNeedsEnter(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "d")
	require.IsType(t, viewSidebarDelete{}, model)

	model = send(model, "y")

	require.IsType(t, viewSidebarDelete{}, model, "`y` 하나로는 창을 벗어나지 않는다")
	_, err := os.Stat(filepath.Join(root, "docs"))
	assert.NoError(t, err, "`y` 하나로는 지워지지 않는다")
}

// 무르는 길이 셋이다. `esc`·`ctrl+c` 는 곧바로 무르고 No 를 고르면 `enter` 가 무른다.
// `ctrl+c` 는 여기서 편집기를 끄지 않는다(ADR-0054).
func TestSidebarDeleteCancels(t *testing.T) {
	for _, keys := range [][]string{{"esc"}, {"ctrl+c"}, {"n", "enter"}} {
		m := newTreeEditor(t, 80, 10)
		root := m.sidebar.root

		model := selectTree(t, tea.Model(m), "docs")
		model = send(model, "m", "d")
		require.IsType(t, viewSidebarDelete{}, model)

		model = sendSync(t, model, keys...)

		assert.IsType(t, viewSidebar{}, model, "%v 는 취소다", keys)
		assert.Contains(t, barOf(t, model)[1], "지우지 않았습니다", "%v", keys)

		_, err := os.Stat(filepath.Join(root, "docs"))
		assert.NoError(t, err, "%v 로는 지워지지 않는다", keys)
	}
}

// 뜻이 없는 키는 아무 일도 하지 않는다. 창에 그대로 머문다.
func TestSidebarDeleteIgnoresUnknownKey(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "d")
	require.IsType(t, viewSidebarDelete{}, model)

	model = send(model, "j")

	assert.IsType(t, viewSidebarDelete{}, model, "창을 벗어나지 않는다")
}

// 디렉터리는 안의 것까지 통째로 지운다. 무엇을 잃는지가 이름보다 먼저 온다.
func TestSidebarDeleteRemovesDirRecursively(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "d")
	require.IsType(t, viewSidebarDelete{}, model)

	model = sendSync(t, model, "y", "enter")

	_, err := os.Stat(filepath.Join(root, "docs"))
	assert.Error(t, err, "안에 파일이 있어도 지워진다")

	v := model.(viewSidebar)
	assert.NotContains(t, names(v.sidebar.rows()), "1:docs", "트리에서도 없어진다")
}

// 열려 있는 파일도 지우고 그 tab 을 같이 닫는다. 열어둔 채로 두면 `:w` 한 번에 지운 것이
// 되살아난다(ADR-0130).
func TestSidebarDeleteClosesOpenTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	// tab 으로 열고 다시 트리로 돌아온다.
	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "enter")
	require.IsType(t, viewEditorNormal{}, model)
	require.Len(t, model.(viewEditorNormal).buffers, 2)

	model = selectTree(t, model, "README.md")
	model = sendSync(t, model, "m", "d")

	require.IsType(t, viewSidebar{}, model, "고친 것이 없으면 묻지 않는다")

	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "지워졌다")

	v := model.(viewSidebar)
	require.Len(t, v.buffers, 1, "그 tab 이 닫혔다")
	assert.Equal(t, "main.go", v.buffers[0].Path, "남은 것은 열어 두었던 다른 tab 이다")

	// tabline 에서 사라진 것을 화면만 보고는 지우기가 한 일인지 알 수 없다.
	assert.Contains(t, barOf(t, model)[1], "tab 1 개를 닫았습니다")
}

// 디렉터리는 그 아래 열린 tab 도 닫는다. 통째로 지우는 것은 안의 파일을 지우는 것이다.
func TestSidebarDeleteClosesTabsInsideDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = sendSync(t, model, "enter") // 펼친다. 자식은 바로 아래 행이다
	model = sendSync(t, send(model, "down"), "enter")
	require.IsType(t, viewEditorNormal{}, model, "docs/spec.md 를 열었다")

	// 연 파일 자리에서 한 행 위가 그 파일이 든 디렉터리다.
	model = send(model, "ctrl+w", "ctrl+w", "up")
	require.Equal(t, "docs", model.(viewSidebar).sidebar.selectedNode().name)

	model = send(model, "m", "d")
	require.IsType(t, viewSidebarDelete{}, model, "디렉터리는 그대로 묻는다")

	model = sendSync(t, model, "y", "enter")

	_, err := os.Stat(filepath.Join(root, "docs"))
	assert.Error(t, err, "안의 것까지 지워졌다")

	v := model.(viewSidebar)
	require.Len(t, v.buffers, 1, "안의 파일을 보던 tab 이 닫혔다")
	assert.Equal(t, "main.go", v.buffers[0].Path)
}

// 저장하지 않은 변경이 있으면 깨끗한 파일과 달리 묻는다. `md` 두 키가 편집하던 것을 조용히
// 버리지 않는다 — 묻지 않고 지우는 근거가 「잃을 것이 없다」였다(ADR-0130).
func TestSidebarDeleteAsksWhenTabIsDirty(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "enter")
	require.IsType(t, viewEditorNormal{}, model)
	model.(viewEditorNormal).activeBuffer().Insert([]byte("X"))

	model = selectTree(t, model, "README.md")
	confirm := send(model, "m", "d")
	require.IsType(t, viewSidebarDelete{}, confirm)

	box := deleteBoxOf(t, confirm)
	assert.Contains(t, box, "저장하지 않은 tab 1 개가 같이 닫힙니다.")
	assert.Contains(t, box, "README.md 를 지울까요?")
	assert.NotContains(t, box, "안의 것까지", "파일은 안이 없다")

	// 무르면 파일도 tab 도 그대로다. 돌아가는 곳은 트리다.
	cancelled := sendSync(t, confirm, "esc")
	require.IsType(t, viewSidebar{}, cancelled)
	assert.Len(t, cancelled.(viewSidebar).buffers, 2, "tab 이 그대로 있다")
	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.NoError(t, err, "파일도 그대로다")

	deleted := sendSync(t, confirm, "y", "enter")
	require.IsType(t, viewSidebar{}, deleted, "지운 뒤에도 트리에 머문다")
	assert.Len(t, deleted.(viewSidebar).buffers, 1, "그 tab 이 닫혔다")
	_, err = os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "지워졌다")
}

// 잃는 것이 둘이면 둘을 다 적는다. 한 줄에 담기지 않는 것이 이 물음을 statusBar 아래 줄에서
// 창으로 올린 까닭이다(ADR-0130).
func TestSidebarDeleteDirWithDirtyTabNamesBothLosses(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "docs")
	model = sendSync(t, model, "enter")
	model = sendSync(t, send(model, "down"), "enter")
	require.IsType(t, viewEditorNormal{}, model, "docs/spec.md 를 열었다")
	model.(viewEditorNormal).activeBuffer().Insert([]byte("X"))

	model = send(model, "ctrl+w", "ctrl+w", "up")
	require.Equal(t, "docs", model.(viewSidebar).sidebar.selectedNode().name)

	model = send(model, "m", "d")

	box := deleteBoxOf(t, model)
	assert.Contains(t, box, "안의 것까지 모두 지웁니다.")
	assert.Contains(t, box, "저장하지 않은 tab 1 개가 같이 닫힙니다.")
	assert.Contains(t, box, "docs/ 를 지울까요?")
}

// 뿌리는 트리 자체다. 지우면 트리가 가리킬 자리도 없어진다.
func TestSidebarDeleteRefusesRoot(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := send(tea.Model(m), "ctrl+w", "ctrl+w")
	require.Equal(t, root, model.(viewSidebar).sidebar.selectedNode().path, "처음 고른 자리는 뿌리다")

	model = send(model, "m", "d")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "뿌리는 지울 수 없습니다")

	_, err := os.Stat(root)
	assert.NoError(t, err)
}

// 짝이 없는 `m` 조합은 아무 일도 하지 않는다. `ctrl+w` 접두 키와 같은 규칙이다.
func TestSidebarUnknownFileKeyDoesNothing(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "README.md")
	before := model.(viewSidebar).sidebar.selected

	model = send(model, "m", "x")

	require.IsType(t, viewSidebar{}, model)
	assert.Equal(t, before, model.(viewSidebar).sidebar.selected, "고른 자리도 그대로다")
}

// 한글 입력 상태에서도 지운다. `ㅡㅇ` 이 `md` 이고 `ㅛ` 가 `y` 다(ADR-0008).
func TestSidebarDeleteTakesHangulKeys(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "ㅡ", "ㅇ")

	require.IsType(t, viewSidebarDelete{}, model, "ㅡㅇ 가 md 다")

	model = sendSync(t, model, "ㅛ", "enter")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "docs"))
	assert.Error(t, err, "ㅛ 가 y 라 Yes 를 고르고 enter 가 지운다")
}
