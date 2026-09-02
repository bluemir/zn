package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typeName 은 이름을 한 글자씩 쳐 넣는다. 이름을 받는 화면은 키를 동작이 아니라 글자로 먹는다.
func typeName(t *testing.T, m tea.Model, name string) tea.Model {
	t.Helper()

	for _, c := range name {
		require.IsType(t, viewSidebarCreate{}, m, "이름을 받는 화면에 머문다")
		m = sendSync(t, m, string(c))
	}

	return m
}

// `mc` 는 이름을 받는 화면으로 간다. 만들 자리는 고른 항목 기준이다.
func TestSidebarCreateAsksNameInSelectedDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "c")

	create, ok := model.(viewSidebarCreate)
	require.True(t, ok, "이름을 받는 화면으로 간다")
	assert.Equal(t, filepath.Join(root, "docs"), create.dir, "디렉터리를 골랐으면 그 안이다")
	assert.Contains(t, barOf(t, model)[1], "새 파일: docs/", "만들 자리가 아래 줄에 보인다")
}

// `ma` 도 `mc` 와 같은 화면으로 간다. NERDTree 손버릇으로 온 손을 돌려보내지 않는다.
func TestSidebarCreateAcceptsMaAsWell(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "a")

	create, ok := model.(viewSidebarCreate)
	require.True(t, ok, "이름을 받는 화면으로 간다")
	assert.Equal(t, filepath.Join(root, "docs"), create.dir)
}

// 파일을 골랐으면 그 파일이 있는 디렉터리다.
func TestSidebarCreateUsesParentDirOfSelectedFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")

	create, ok := model.(viewSidebarCreate)
	require.True(t, ok)
	assert.Equal(t, root, create.dir)
}

// 만든 파일은 tab 으로 열리고 포커스가 편집 영역으로 간다. 트리에도 그 이름이 선다.
func TestSidebarCreateMakesFileAndOpensTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "c")
	model = typeName(t, model, "guide.md")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewEditorNormal{}, model, "만들자마자 쓰러 편집 영역으로 간다")

	path := filepath.Join(root, "docs", "guide.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "파일이 만들어져 있다")
	assert.Empty(t, data, "빈 파일이다")

	v := model.(viewEditorNormal)
	assert.Equal(t, path, v.buffers[v.active].Path, "그 파일을 보고 있다")
	assert.Contains(t, names(v.sidebar.rows()), "2:guide.md", "트리도 다시 읽어서 그 이름이 선다")
	assert.Equal(t, path, v.sidebar.selectedNode().path, "고른 자리도 만든 파일이다")
}

// 이름에 `/` 가 있으면 없는 중간 디렉터리를 같이 만든다.
func TestSidebarCreateMakesMissingParentDirs(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = typeName(t, model, "a/b/c.go")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewEditorNormal{}, model)

	_, err := os.Stat(filepath.Join(root, "a", "b", "c.go"))
	assert.NoError(t, err, "중간 디렉터리까지 만들어져 있다")
}

// `/` 로 끝나면 디렉터리만 만들고 트리에 머문다. 열 것이 없다.
func TestSidebarCreateMakesDirAndStaysInTree(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = typeName(t, model, "runtime/")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model, "디렉터리는 열 것이 없어서 트리에 머문다")

	info, err := os.Stat(filepath.Join(root, "runtime"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	v := model.(viewSidebar)
	assert.Len(t, v.buffers, 1, "tab 은 늘지 않는다")
	assert.Contains(t, names(v.sidebar.rows()), "1:runtime", "트리에 선다")
	assert.Contains(t, barOf(t, model)[1], "만들었습니다: runtime/")
}

// 이미 있는 이름은 만들지 않는다. 덮어쓰면 그 파일 내용이 조용히 사라진다.
func TestSidebarCreateRefusesExistingName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = typeName(t, model, "README.md")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model, "만들지 못했으므로 트리에 남는다")
	assert.Contains(t, barOf(t, model)[1], "이미 있습니다: README.md")

	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "x\n", string(data), "있던 내용이 그대로다")
}

// 뿌리 밖은 트리에 보이지 않는 자리다. `../` 로 나가는 이름은 만들지 않는다.
func TestSidebarCreateRefusesOutsideRoot(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = typeName(t, model, "../escaped.go")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "뿌리 밖에는 만들 수 없습니다")

	_, err := os.Stat(filepath.Join(filepath.Dir(root), "escaped.go"))
	assert.Error(t, err, "만들어지지 않았다")
}

// 이름 없이 Enter 는 아무것도 만들지 않는다.
func TestSidebarCreateRefusesEmptyName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "이름이 없습니다")
}

// `esc` 는 만들던 것을 그만둔다. backspace 로 다 지워도 같다.
func TestSidebarCreateCancels(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = typeName(t, send(model, "m", "c"), "gone.go")
	model = send(model, "esc")

	require.IsType(t, viewSidebar{}, model, "트리로 돌아온다")

	_, err := os.Stat(filepath.Join(root, "gone.go"))
	assert.Error(t, err, "만들지 않았다")

	// backspace 로 이름을 다 지우면 명령줄처럼 그 화면에서 나간다.
	model = selectTree(t, tea.Model(m), "README.md")
	model = typeName(t, send(model, "m", "c"), "ab")
	model = send(model, "backspace", "backspace", "backspace")

	assert.IsType(t, viewSidebar{}, model)
}

// 이름은 동작이 아니라 글자다. 한글이 두벌식 자리의 영문 키로 되돌려지면 안 된다(ADR-0008).
func TestSidebarCreateKeepsHangulName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = typeName(t, model, "한글.txt")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewEditorNormal{}, model)

	_, err := os.Stat(filepath.Join(root, "한글.txt"))
	assert.NoError(t, err, "친 그대로 만들어진다")
}

// `mc` 는 트리의 `m` 접두 키다. 다음 키를 기다리는 동안 showcmd 에 그것이 보인다.
func TestSidebarShowsPendingFilePrefix(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := send(tea.Model(m), "ctrl+w", "ctrl+w", "m")

	require.IsType(t, viewSidebar{}, model, "아직 아무 동작도 완성되지 않았다")
	assert.True(t, strings.HasSuffix(strings.TrimRight(barOf(t, model)[1], " "), "m"), "showcmd 자리에 접두 키가 보인다")
}
