package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTreeFixture 는 트리 시험용 디렉터리를 만든다.
//
//	root/
//	  .git/          (감춰져야 한다)
//	  .gitignore     (숨김 파일이지만 보여야 한다)
//	  build/
//	    out
//	  docs/
//	    spec.md
//	  main.go
//	  README.md
func newTreeFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{".git", "build", "docs"} {
		require.NoError(t, os.Mkdir(filepath.Join(root, dir), 0755))
	}
	for _, file := range []string{".gitignore", "main.go", "README.md", "build/out", "docs/spec.md", ".git/HEAD"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte("x\n"), 0644))
	}

	return root
}

// names 는 행 목록을 "깊이:이름" 으로 펴서 비교하기 쉽게 만든다.
func names(rows []treeRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, string(rune('0'+row.depth))+":"+row.node.name)
	}

	return out
}

func TestSidebarOpensWithRootExpanded(t *testing.T) {
	root := newTreeFixture(t)

	s := openSidebar(root)

	require.True(t, s.open)
	require.NotNil(t, s.tree)
	assert.Equal(t, filepath.Base(root), s.tree.name)
	assert.True(t, s.tree.expanded, "뿌리는 펼친 채로 시작한다")
}

// 디렉터리가 먼저, 그 안은 이름순이다. os.ReadDir 이 이미 이름순으로 준다.
func TestSidebarSortsDirsFirst(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	assert.Equal(t, []string{
		"1:build", "1:docs",
		"1:.gitignore", "1:README.md", "1:main.go",
	}, names(s.rows())[1:], "디렉터리 먼저, 그 안은 이름순")
}

// .git 은 감추고 나머지 숨김 파일은 보인다.
func TestSidebarHidesOnlyDotGit(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	shown := names(s.rows())
	assert.NotContains(t, shown, "1:.git")
	assert.Contains(t, shown, "1:.gitignore", "숨김 파일이어도 고치는 파일은 보인다")
}

// worktree·submodule 에서는 .git 이 디렉터리가 아니라 파일이다. 종류를 보지 않고 이름으로 거른다.
func TestSidebarHidesDotGitFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x\n"), 0644))

	s := openSidebar(root)

	assert.Equal(t, []string{"1:main.go"}, names(s.rows())[1:])
}

func TestSidebarExpandAndCollapse(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	docs := s.rows()[2].node
	require.Equal(t, "docs", docs.name)
	require.False(t, docs.expanded)

	docs.toggle()

	assert.Contains(t, names(s.rows()), "2:spec.md", "펼치면 자식이 목록에 들어온다")

	docs.toggle()

	assert.NotContains(t, names(s.rows()), "2:spec.md")
}

// 접었다 펴는 것이 곧 새로고침이다. watcher 없이 이걸로 충분하다.
func TestSidebarRereadsOnExpand(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebar(root)

	docs := s.rows()[2].node
	docs.toggle()
	require.Contains(t, names(s.rows()), "2:spec.md")

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "new.md"), []byte("x\n"), 0644))
	docs.toggle()
	docs.toggle()

	assert.Contains(t, names(s.rows()), "2:new.md", "다시 펼치면 새 파일이 보인다")
}

// symlink 는 따라가지 않는다. 디렉터리를 가리켜도 잎이다.
func TestSidebarDoesNotFollowSymlink(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")))

	s := openSidebar(root)

	var link *treeNode
	for _, row := range s.rows() {
		if row.node.name == "link" {
			link = row.node
		}
	}
	require.NotNil(t, link)

	assert.True(t, link.symlink)
	assert.False(t, link.isDir, "디렉터리를 가리켜도 잎으로 둔다")

	link.toggle()
	assert.False(t, link.expanded, "펼쳐지지 않는다")
}

// 끊어진 symlink 도 죽지 않아야 한다.
func TestSidebarHandlesBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(root, "nope"), filepath.Join(root, "dangling")))

	assert.NotPanics(t, func() {
		s := openSidebar(root)
		assert.Len(t, s.rows(), 2)
	})
}

// 권한이 없는 디렉터리는 빈 것으로 보인다. 펼침 표시가 있으므로 정말 빈 것과 구분된다.
func TestSidebarUnreadableDirIsEmpty(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(locked, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "secret"), []byte("x\n"), 0644))
	require.NoError(t, os.Chmod(locked, 0000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })

	s := openSidebar(root)
	node := s.rows()[1].node
	require.Equal(t, "locked", node.name)

	assert.NotPanics(t, func() { node.toggle() })
	assert.True(t, node.expanded)
	assert.Empty(t, node.children)
}

func TestSidebarEmptyDir(t *testing.T) {
	s := openSidebar(t.TempDir())

	assert.Len(t, s.rows(), 1, "뿌리 한 줄만")
	assert.Equal(t, s.tree, s.selectedNode())
}

func TestSidebarSelectedNode(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	assert.Equal(t, s.tree, s.selectedNode(), "처음에는 뿌리")

	s.selected = 3
	require.NotNil(t, s.selectedNode())
	assert.Equal(t, ".gitignore", s.selectedNode().name)

	s.selected = 999
	assert.Nil(t, s.selectedNode(), "범위를 벗어나면 nil")
}

// git 이 무시하는 항목은 표시된다. 규칙은 git 에게 묻는다.
func TestSidebarMarksGitIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root := newTreeFixture(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n"), 0644))
	require.NoError(t, exec.Command("git", "-C", root, "init").Run())

	s := openSidebar(root)

	byName := map[string]*treeNode{}
	for _, row := range s.rows() {
		byName[row.node.name] = row.node
	}

	assert.True(t, byName["build"].ignored, "gitignore 된 디렉터리")
	assert.False(t, byName["main.go"].ignored)
	assert.False(t, byName["README.md"].ignored)
}

// 저장소가 아니면 아무것도 흐리게 하지 않는다. 흐린 것이 없을 뿐 틀리지 않는다.
func TestSidebarNoRepoMarksNothing(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	for _, row := range s.rows() {
		assert.False(t, row.node.ignored, "%s", row.node.name)
	}
}
