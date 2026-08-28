package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// HEAD 가 움직인 사이에 달라진 Go 파일을 골라내는 자리다(ADR-0092).

// gitCommitFixture 는 commit 둘이 든 저장소를 만들고 두 해시를 준다.
//
// go-git 으로 짓지 않고 `git` 을 부른다. 여기서 재려는 것은 우리가 tree 차이를 읽는 법이고,
// 그 기준은 git 이 실제로 만든 저장소여야 한다.
func gitCommitFixture(t *testing.T) (root, first, second string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root = t.TempDir()

	run := func(args ...string) string {
		t.Helper()

		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)

		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))

		return string(out)
	}

	write := func(name, body string) {
		t.Helper()

		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0644))
	}

	run("init", "-q", ".")

	write("keep.go", "package p\n\nfunc Keep() {}\n")
	write("gone.go", "package p\n\nfunc Gone() {}\n")
	write("notes.md", "첫 글\n")
	run("add", "-A")
	run("commit", "-qm", "첫째")

	first = run("rev-parse", "HEAD")

	// 하나는 고치고, 하나는 지우고, 하나는 만든다. Go 가 아닌 것도 하나 고친다.
	write("keep.go", "package p\n\nfunc Keep() int { return 1 }\n")
	require.NoError(t, os.Remove(filepath.Join(root, "gone.go")))
	write("deep/new.go", "package deep\n\nfunc New() {}\n")
	write("notes.md", "둘째 글\n")
	run("add", "-A")
	run("commit", "-qm", "둘째")

	second = run("rev-parse", "HEAD")

	return root, trimHash(first), trimHash(second)
}

// trimHash 는 `git rev-parse` 가 붙이는 줄바꿈을 떼어낸다.
func trimHash(out string) string {
	for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == '\r') {
		out = out[:len(out)-1]
	}

	return out
}

// 고친 것·지운 것·만든 것이 각각의 갈래로 온다. Go 가 아닌 것은 빠진다.
func TestGitTreeChanges(t *testing.T) {
	root, first, second := gitCommitFixture(t)

	before, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(before) })

	changes := gitTreeChanges(t.Context(), first, second)

	// **뿌리를 symlink 를 풀어서 견준다.** go-git 이 풀어서 주고 macOS 의 `/var` 는
	// `/private/var` 다 — openGitRepo 가 같은 자리에 적어 둔 이야기다.
	resolved, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	got := map[string]lsp.FileChangeKind{}
	for _, change := range changes {
		rel, err := filepath.Rel(resolved, change.Path)
		require.NoError(t, err)

		got[filepath.ToSlash(rel)] = change.Kind
	}

	assert.Equal(t, map[string]lsp.FileChangeKind{
		"keep.go":     lsp.FileChanged,
		"gone.go":     lsp.FileDeleted,
		"deep/new.go": lsp.FileCreated,
	}, got, "Go 파일만, 갈래를 맞춰서 온다")

	assert.NotContains(t, got, "notes.md", "gopls 가 볼 것이 아니다")

	// 경로는 절대 경로다. URI 로 감싸는 쪽이 그것을 요구한다(goplsPath 와 같은 기준이다).
	for _, change := range changes {
		assert.True(t, filepath.IsAbs(change.Path), "절대 경로여야 한다: %s", change.Path)
	}
}

// 같은 commit 이거나 한쪽이 비면 볼 것이 없다.
func TestGitTreeChangesNothingToCompare(t *testing.T) {
	root, first, _ := gitCommitFixture(t)

	before, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(before) })

	assert.Empty(t, gitTreeChanges(t.Context(), first, first), "같은 commit 이다")
	assert.Nil(t, gitTreeChanges(t.Context(), "", first))
	assert.Nil(t, gitTreeChanges(t.Context(), first, ""), "없는 해시는 commit 을 못 찾는다")
}

// **알릴 곳이 없으면 아무 일도 하지 않는다.** 서버가 없거나 HEAD 가 그대로인 때다.
func TestGoplsWatchChangesGuards(t *testing.T) {
	// nil client 로 불러도 터지지 않는다. gopls 가 안 떠 있는 것이 흔한 상태다.
	goplsWatchChanges(t.Context(), nil, "aaa", "bbb")
	goplsWatchChanges(t.Context(), nil, "", "bbb")
	goplsWatchChanges(t.Context(), nil, "aaa", "aaa")
}

// 저장소가 아니면 볼 것이 없다.
func TestGitTreeChangesOutsideRepo(t *testing.T) {
	before, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(before) })

	assert.Nil(t, gitTreeChanges(t.Context(), "aaa", "bbb"))
}

// HEAD 를 자르지 않은 해시로 든다. 짧은 해시는 저장소 크기에 따라 길이가 달라져서
// 견주는 자로 쓸 수 없다(gitShortHashLen).
func TestGitStatusKeepsFullHead(t *testing.T) {
	root, _, second := gitCommitFixture(t)

	before, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(before) })

	status := readGitStatus(t.Context(), nil).status

	assert.Equal(t, second, status.head, "자르지 않은 해시다")
	assert.Len(t, status.commit, gitMinShortHashLen, "찍는 것은 짧은 해시다")
	assert.True(t, len(status.head) > len(status.commit))
}
