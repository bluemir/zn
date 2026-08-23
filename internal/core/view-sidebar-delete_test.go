package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `md` 는 묻고, `y` 가 지운다. 지운 자리는 트리에서도 없어진다.
func TestSidebarDeleteAsksThenRemovesFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "d")

	require.IsType(t, viewSidebarDelete{}, model, "지우기 전에 한 번 더 묻는다")
	assert.Contains(t, barOf(t, model)[1], "지울까요? README.md (y/n)")

	_, err := os.Stat(filepath.Join(root, "README.md"))
	require.NoError(t, err, "묻는 동안에는 아직 그대로다")

	model = sendSync(t, model, "y")

	require.IsType(t, viewSidebar{}, model, "지운 뒤에도 트리에 머문다")
	_, err = os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "지워졌다")

	v := model.(viewSidebar)
	assert.NotContains(t, names(v.sidebar.rows()), "1:README.md", "트리를 다시 읽어서 그 행이 없다")
	assert.Contains(t, barOf(t, model)[1], "지웠습니다: README.md")
}

// `y` 가 아닌 키는 전부 취소다. 되돌릴 수 없는 일이라 오타가 실행이 되어서는 안 된다.
func TestSidebarDeleteCancelsOnAnyOtherKey(t *testing.T) {
	for _, k := range []string{"n", "esc", "ctrl+c", "j", "enter"} {
		m := newTreeEditor(t, 80, 10)
		root := m.sidebar.root

		model := selectTree(t, tea.Model(m), "README.md")
		model = send(model, "m", "d")
		require.IsType(t, viewSidebarDelete{}, model)

		model = sendSync(t, model, k)

		assert.IsType(t, viewSidebar{}, model, "%s 는 취소다", k)
		_, err := os.Stat(filepath.Join(root, "README.md"))
		assert.NoError(t, err, "%s 로는 지워지지 않는다", k)
	}
}

// 디렉터리는 안의 것까지 통째로 지운다. 묻는 문구도 파일과 다르다.
func TestSidebarDeleteRemovesDirRecursively(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "d")

	require.IsType(t, viewSidebarDelete{}, model)
	assert.Contains(t, barOf(t, model)[1], "안의 것까지 모두 지울까요? docs/ (y/n)")

	model = sendSync(t, model, "y")

	_, err := os.Stat(filepath.Join(root, "docs"))
	assert.Error(t, err, "안에 파일이 있어도 지워진다")

	v := model.(viewSidebar)
	assert.NotContains(t, names(v.sidebar.rows()), "1:docs", "트리에서도 없어진다")
}

// 열려 있는 파일은 지우지 않는다. buffer 와 디스크가 어긋난 채로 남기지 않는다.
func TestSidebarDeleteRefusesOpenFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	// tab 으로 열고 다시 트리로 돌아온다.
	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "enter")
	require.IsType(t, viewEditorNormal{}, model)

	model = selectTree(t, model, "README.md")
	model = send(model, "m", "d")

	require.IsType(t, viewSidebar{}, model, "묻지도 않는다")
	assert.Contains(t, barOf(t, model)[1], "tab 에 열려 있습니다")

	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.NoError(t, err, "그대로 있다")
}

// 디렉터리는 그 아래 열린 파일도 본다. 통째로 지우는 것은 안의 파일을 지우는 것이다.
func TestSidebarDeleteRefusesDirWithOpenFileInside(t *testing.T) {
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

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "tab 에 열려 있습니다")

	_, err := os.Stat(filepath.Join(root, "docs", "spec.md"))
	assert.NoError(t, err)
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

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "ㅡ", "ㅇ")

	require.IsType(t, viewSidebarDelete{}, model, "ㅡㅇ 가 md 다")

	model = sendSync(t, model, "ㅛ")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "ㅛ 가 y 라 지워진다")
}
