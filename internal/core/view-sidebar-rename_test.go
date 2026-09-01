package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typePath 는 이름을 받는 칸에 글자를 이어 친다. 이름 바꾸기 화면도 키를 글자로 먹는다.
func typePath(t *testing.T, m tea.Model, text string) tea.Model {
	t.Helper()

	for _, c := range text {
		require.IsType(t, viewSidebarRename{}, m, "이름을 받는 화면에 머문다")
		m = sendSync(t, m, string(c))
	}

	return m
}

// backspace 는 문자 수만큼 보낸다.
func eraseAll(t *testing.T, m tea.Model, count int) tea.Model {
	t.Helper()

	for range count {
		m = sendSync(t, m, "backspace")
	}

	return m
}

// `mm` 은 지금 경로가 채워진 칸을 준다. 뿌리 기준 상대 경로다.
func TestSidebarRenamePrefillsCurrentPath(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "docs")
	model = sendSync(t, model, "enter") // 펼친다
	model = send(model, "down")         // 그 안의 spec.md
	model = send(model, "m", "m")

	rename, ok := model.(viewSidebarRename)
	require.True(t, ok, "이름을 고치는 화면으로 간다")
	assert.Equal(t, "docs/spec.md", rename.input.text, "지금 경로가 채워져 있다")
	assert.Contains(t, barOf(t, model)[1], "새 이름: docs/spec.md")
}

// 뒤만 고치면 제자리 이름 바꾸기다.
func TestSidebarRenameRenamesInPlace(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "m")
	model = eraseAll(t, model, len("README.md"))
	model = typePath(t, model, "GUIDE.md")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model, "이름을 바꾼 뒤에도 트리에 머문다")

	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "옛 이름은 없다")

	data, err := os.ReadFile(filepath.Join(root, "GUIDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "x\n", string(data), "내용은 그대로다")

	v := model.(viewSidebar)
	assert.Contains(t, names(v.sidebar.rows()), "1:GUIDE.md")
	assert.Equal(t, filepath.Join(root, "GUIDE.md"), v.sidebar.selectedNode().path, "고른 자리가 따라간다")
	assert.Contains(t, barOf(t, model)[1], "이름을 바꿨습니다: README.md → GUIDE.md")
}

// 앞을 고치면 자리를 옮기는 것이다. 없는 디렉터리는 같이 만든다.
func TestSidebarRenameMovesToAnotherDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "main.go")
	model = send(model, "m", "m")
	model = eraseAll(t, model, len("main.go"))
	model = typePath(t, model, "cmd/zn/main.go")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)

	_, err := os.Stat(filepath.Join(root, "main.go"))
	assert.Error(t, err, "옛 자리는 비었다")
	_, err = os.Stat(filepath.Join(root, "cmd", "zn", "main.go"))
	assert.NoError(t, err, "없던 중간 디렉터리까지 만들어 옮긴다")

	v := model.(viewSidebar)
	assert.Equal(t, filepath.Join(root, "cmd", "zn", "main.go"), v.sidebar.selectedNode().path,
		"옮겨간 자리까지 펼쳐서 고른다")
}

// 디렉터리도 이름을 바꾼다. 안의 것은 그대로 딸려 간다.
func TestSidebarRenameRenamesDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = send(model, "m", "m")
	model = eraseAll(t, model, len("docs"))
	model = typePath(t, model, "documents")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)

	_, err := os.Stat(filepath.Join(root, "documents", "spec.md"))
	assert.NoError(t, err, "안의 파일이 딸려 간다")
	assert.Contains(t, barOf(t, model)[1], "이름을 바꿨습니다: docs/ → documents/")
}

// 열려 있는 파일의 tab 이 새 이름을 따라간다. 지우기와 다른 자리다.
func TestSidebarRenameFollowsOpenTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "enter")
	require.IsType(t, viewEditorNormal{}, model, "tab 으로 열었다")

	model = selectTree(t, model, "README.md")
	model = send(model, "m", "m")
	model = eraseAll(t, model, len("README.md"))
	model = typePath(t, model, "docs/README.md")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)

	v := model.(viewSidebar)
	assert.Equal(t, filepath.Join(root, "docs", "README.md"), v.buffers[v.active].path,
		"보고 있는 buffer 의 경로가 새 이름이다")
	assert.Len(t, v.buffers, 2, "tab 수는 그대로다")
}

// 디렉터리를 옮기면 그 아래 열린 tab 도 같이 따라간다.
func TestSidebarRenameFollowsOpenTabUnderDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "docs")
	model = sendSync(t, model, "enter")
	model = sendSync(t, send(model, "down"), "enter") // docs/spec.md 를 연다
	require.IsType(t, viewEditorNormal{}, model)

	model = send(model, "ctrl+w", "ctrl+w", "up") // 그 위가 docs 다
	require.Equal(t, "docs", model.(viewSidebar).sidebar.selectedNode().name)

	model = send(model, "m", "m")
	model = eraseAll(t, model, len("docs"))
	model = typePath(t, model, "documents")
	model = sendSync(t, model, "enter")

	v, ok := model.(viewSidebar)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(root, "documents", "spec.md"), v.buffers[v.active].path,
		"디렉터리를 옮기면 안의 파일을 보던 tab 도 따라간다")
}

// 이미 있는 이름 위로는 옮기지 않는다. os.Rename 은 묻지 않고 덮어쓴다.
func TestSidebarRenameRefusesExistingName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "main.go")
	model = send(model, "m", "m")
	model = eraseAll(t, model, len("main.go"))
	model = typePath(t, model, "README.md")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "이미 있습니다: README.md")

	_, err := os.Stat(filepath.Join(root, "main.go"))
	assert.NoError(t, err, "옮기지 않았다")
}

// 뿌리 밖으로는 옮기지 않는다. 그대로 두는 것과 자기 안으로 옮기는 것도 막는다.
func TestSidebarRenameRefusesImpossibleTargets(t *testing.T) {
	tests := []struct {
		name  string
		pick  string
		typed string
		want  string
	}{
		{name: "뿌리 밖", pick: "README.md", typed: "../escaped.md", want: "뿌리 밖으로는 옮길 수 없습니다"},
		{name: "그대로", pick: "README.md", typed: "README.md", want: "이름이 그대로입니다"},
		{name: "자기 안", pick: "docs", typed: "docs/inner", want: "디렉터리를 자기 안으로 옮길 수 없습니다"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTreeEditor(t, 80, 10)
			root := m.sidebar.root

			model := selectTree(t, tea.Model(m), test.pick)
			model = send(model, "m", "m")

			rename := model.(viewSidebarRename)
			model = eraseAll(t, model, len(rename.input.text))
			model = typePath(t, model, test.typed)
			model = sendSync(t, model, "enter")

			require.IsType(t, viewSidebar{}, model)
			assert.Contains(t, barOf(t, model)[1], test.want)

			_, err := os.Stat(filepath.Join(root, test.pick))
			assert.NoError(t, err, "그대로 있다")
		})
	}
}

// `esc` 는 고치던 것을 그만둔다. 다 지워도 화면에 남는다 — 채워져 있던 것을 지운 것이라
// 그만두려는 뜻으로 읽을 수 없다.
func TestSidebarRenameCancels(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "m")
	model = eraseAll(t, model, len("README.md"))

	require.IsType(t, viewSidebarRename{}, model, "다 지워도 이 화면이다")
	assert.Equal(t, "", model.(viewSidebarRename).input.text)

	model = send(model, "esc")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.NoError(t, err, "그대로 있다")
}

// 뿌리는 이름을 바꿀 수 없다. 트리의 뿌리도 cwd 도 그 자리에 없어진다.
func TestSidebarRenameRefusesRoot(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := send(tea.Model(m), "ctrl+w", "ctrl+w")
	model = send(model, "m", "m")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "뿌리는 이름을 바꿀 수 없습니다")
}

// 경로도 글자다. 한글이 두벌식 자리의 영문 키로 되돌려지면 안 된다(ADR-0008).
func TestSidebarRenameKeepsHangulName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "ㅡ", "ㅡ")

	require.IsType(t, viewSidebarRename{}, model, "ㅡㅡ 가 mm 이다")

	model = eraseAll(t, model, len("README.md"))
	model = typePath(t, model, "한글.txt")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "한글.txt"))
	assert.NoError(t, err, "친 그대로 바뀐다")
}
