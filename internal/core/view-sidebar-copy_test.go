package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typeCopyPath 는 복제할 경로를 받는 칸에 글자를 이어 친다.
func typeCopyPath(t *testing.T, m tea.Model, text string) tea.Model {
	t.Helper()

	for _, c := range text {
		require.IsType(t, viewSidebarCopy{}, m, "경로를 받는 화면에 머문다")
		m = sendSync(t, m, string(c))
	}

	return m
}

// copyAs 는 name 을 골라 `mc` 로 열고, 채워진 경로를 지운 뒤 typed 를 쳐서 복제한다.
func copyAs(t *testing.T, m tea.Model, name, typed string) tea.Model {
	t.Helper()

	model := selectTree(t, m, name)
	model = send(model, "m", "c")

	copying, ok := model.(viewSidebarCopy)
	require.True(t, ok, "복제할 경로를 받는 화면으로 간다")

	model = eraseAll(t, model, len(copying.input.text))
	model = typeCopyPath(t, model, typed)

	return sendSync(t, model, "enter")
}

// `mc` 는 지금 경로가 채워진 칸을 준다. 이름 바꾸기와 같이 뿌리 기준 상대 경로다.
func TestSidebarCopyPrefillsCurrentPath(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "docs")
	model = sendSync(t, model, "enter") // 펼친다
	model = send(model, "down")         // 그 안의 spec.md
	model = send(model, "m", "c")

	copying, ok := model.(viewSidebarCopy)
	require.True(t, ok, "복제할 경로를 받는 화면으로 간다")
	assert.Equal(t, "docs/spec.md", copying.input.text, "지금 경로가 채워져 있다")
	assert.Contains(t, barOf(t, model)[1], "복제: docs/spec.md")
}

// 파일을 복제하면 원본은 그대로이고 새 것이 같은 내용으로 생긴다. 고른 자리는 새 것이다.
func TestSidebarCopyCopiesFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := copyAs(t, tea.Model(m), "README.md", "GUIDE.md")

	require.IsType(t, viewSidebar{}, model, "복제한 뒤에도 트리에 머문다")

	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.NoError(t, err, "원본은 그대로다")

	data, err := os.ReadFile(filepath.Join(root, "GUIDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "x\n", string(data), "내용이 같다")

	v := model.(viewSidebar)
	assert.Contains(t, names(v.sidebar.rows()), "1:GUIDE.md")
	assert.Equal(t, filepath.Join(root, "GUIDE.md"), v.sidebar.selectedNode().path, "고른 자리가 새 것이다")
	assert.Contains(t, barOf(t, model)[1], "복제했습니다: README.md → GUIDE.md")
}

// 앞을 고치면 다른 자리에 복제한다. 없는 디렉터리는 같이 만들고 그 자리까지 펼쳐 고른다.
func TestSidebarCopyIntoAnotherDir(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := copyAs(t, tea.Model(m), "main.go", "cmd/zn/main.go")

	require.IsType(t, viewSidebar{}, model)

	_, err := os.Stat(filepath.Join(root, "main.go"))
	assert.NoError(t, err, "원본은 그대로다")
	_, err = os.Stat(filepath.Join(root, "cmd", "zn", "main.go"))
	assert.NoError(t, err, "없던 중간 디렉터리까지 만들어 복제한다")

	v := model.(viewSidebar)
	assert.Equal(t, filepath.Join(root, "cmd", "zn", "main.go"), v.sidebar.selectedNode().path)
}

// 디렉터리는 안의 것까지 통째로 복제한다. 안의 symlink 는 따라가지 않고 링크로 남는다.
func TestSidebarCopyCopiesDirWithLinks(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "deep"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "deep", "note.md"), []byte("deep\n"), 0644))
	require.NoError(t, os.Symlink("../README.md", filepath.Join(root, "docs", "readme")))

	model := copyAs(t, tea.Model(m), "docs", "documents")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "복제했습니다: docs/ → documents/")

	data, err := os.ReadFile(filepath.Join(root, "documents", "deep", "note.md"))
	require.NoError(t, err, "안의 안까지 딸려 온다")
	assert.Equal(t, "deep\n", string(data))

	link, err := os.Readlink(filepath.Join(root, "documents", "readme"))
	require.NoError(t, err, "링크는 링크로 복제한다")
	assert.Equal(t, "../README.md", link, "가리키는 것도 그대로다")

	_, err = os.Stat(filepath.Join(root, "docs", "spec.md"))
	assert.NoError(t, err, "원본은 그대로다")
}

// 고른 것이 symlink 면 링크를 복제한다. 가리키는 것을 따라가 내용을 베끼지 않는다.
func TestSidebarCopyCopiesLinkItself(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink("docs", filepath.Join(root, "link")))

	m := newTreeEditor(t, 80, 12)
	m.sidebar = openSidebarSync(t, root)
	m.sidebar.scrollTo(m.sidebarHeight())

	model := copyAs(t, tea.Model(m), "link", "link2")

	require.IsType(t, viewSidebar{}, model)

	link, err := os.Readlink(filepath.Join(root, "link2"))
	require.NoError(t, err, "링크가 생긴다")
	assert.Equal(t, "docs", link)
}

// 권한은 원본 그대로다. 실행 파일은 실행 파일로 남는다.
func TestSidebarCopyKeepsMode(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root
	require.NoError(t, os.Chmod(filepath.Join(root, "main.go"), 0755))

	copyAs(t, tea.Model(m), "main.go", "run.go")

	info, err := os.Stat(filepath.Join(root, "run.go"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

// 저장하지 않은 tab 이 있어도 복제하는 것은 디스크의 것이다. `cp` 와 같다.
func TestSidebarCopyUsesDiskContent(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "enter")
	model = send(model, "i", "n", "e", "w", "esc")
	require.True(t, model.(viewEditorNormal).buffers[1].Dirty, "고치고 저장하지 않았다")

	model = copyAs(t, model, "README.md", "COPY.md")

	data, err := os.ReadFile(filepath.Join(root, "COPY.md"))
	require.NoError(t, err)
	assert.Equal(t, "x\n", string(data), "고친 것은 따라가지 않는다")
}

// 이미 있는 이름 위로, 뿌리 밖으로, 자기 안으로는 복제하지 않는다. 그대로 둔 경로는 이미 있는 것이다.
func TestSidebarCopyRefusesImpossibleTargets(t *testing.T) {
	tests := []struct {
		name  string
		pick  string
		typed string
		want  string
	}{
		{name: "이미 있음", pick: "main.go", typed: "README.md", want: "이미 있습니다: README.md"},
		{name: "그대로", pick: "README.md", typed: "README.md", want: "이미 있습니다: README.md"},
		{name: "뿌리 밖", pick: "README.md", typed: "../escaped.md", want: "뿌리 밖으로는 복제할 수 없습니다"},
		{name: "자기 안", pick: "docs", typed: "docs/inner", want: "디렉터리를 자기 안으로 복제할 수 없습니다"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTreeEditor(t, 80, 10)
			root := m.sidebar.root
			before, err := os.ReadFile(filepath.Join(root, "README.md"))
			require.NoError(t, err)

			model := copyAs(t, tea.Model(m), test.pick, test.typed)

			require.IsType(t, viewSidebar{}, model)
			assert.Contains(t, barOf(t, model)[1], test.want)

			after, err := os.ReadFile(filepath.Join(root, "README.md"))
			require.NoError(t, err)
			assert.Equal(t, before, after, "덮어쓰지 않았다")
			_, err = os.Stat(filepath.Join(root, "docs", "inner"))
			assert.True(t, os.IsNotExist(err), "자기 안에 아무것도 만들지 않았다")
		})
	}
}

// 중간에 실패하면 만든 것을 지운다. 반쯤 찬 복제본이 원본처럼 남으면 안 된다.
func TestSidebarCopyRemovesPartialCopyOnFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root 는 읽기 권한을 무시한다")
	}

	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	locked := filepath.Join(root, "docs", "locked.md")
	require.NoError(t, os.WriteFile(locked, []byte("x\n"), 0000))

	model := copyAs(t, tea.Model(m), "docs", "documents")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "복제할 수 없습니다")

	_, err := os.Lstat(filepath.Join(root, "documents"))
	assert.True(t, os.IsNotExist(err), "만들던 것이 남지 않는다")
}

// 뿌리는 복제할 수 없다. 뿌리 안의 어느 자리든 뿌리 자기 안이다.
func TestSidebarCopyRefusesRoot(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := send(tea.Model(m), "ctrl+w", "ctrl+w")
	model = send(model, "m", "c")

	require.IsType(t, viewSidebar{}, model)
	assert.Contains(t, barOf(t, model)[1], "뿌리는 복제할 수 없습니다")
}

// `esc` 는 그만둔다. 다 지워도 화면에 남는다 — 채워져 있던 것을 지운 것이다.
func TestSidebarCopyCancels(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "m", "c")
	model = eraseAll(t, model, len("README.md"))

	require.IsType(t, viewSidebarCopy{}, model, "다 지워도 이 화면이다")

	model = send(model, "esc")

	require.IsType(t, viewSidebar{}, model)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Len(t, entries, 6, "아무것도 생기지 않았다")
}

// 한글 입력 상태에서도 `ㅡㅊ` 가 `mc` 다. 그 뒤에 치는 경로는 글자 그대로다(ADR-0008).
func TestSidebarCopyKeepsHangulName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "ㅡ", "ㅊ")

	require.IsType(t, viewSidebarCopy{}, model, "ㅡㅊ 가 mc 다")

	model = eraseAll(t, model, len("README.md"))
	model = typeCopyPath(t, model, "한글.txt")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "한글.txt"))
	assert.NoError(t, err, "친 그대로 생긴다")
}

// 메뉴의 「복제」 는 지금 경로가 채워진 상자를 띄운다. `enter` 가 `mc` 와 같은 일을 한다.
func TestSidebarMenuCopy(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := openMenuItem(t, m, m.sidebar, "README.md", 2)

	box, ok := model.(viewSidebarInput)
	require.True(t, ok, "상자가 뜬다")
	assert.Equal(t, "README.md", box.input.text, "지금 경로가 채워져 있다")

	model = sendSync(t, model, "backspace", "backspace") // `md` 를 지운다
	model = typeBox(t, model, "txt")
	model = sendSync(t, model, "enter")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "README.txt"))
	assert.NoError(t, err, "복제본이 생긴다")
	_, err = os.Stat(filepath.Join(root, "README.md"))
	assert.NoError(t, err, "원본은 그대로다")
}
