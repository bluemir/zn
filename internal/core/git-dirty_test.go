package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGit 은 fixture 를 세울 때 쓰는 git 이다.
//
// 판정 자체는 라이브러리가 하지만(ADR-0042), 시험의 기준선은 진짜 git 이어야 한다.
// 우리 판정과 git 의 판정을 견주는 것이 이 파일이 하는 일 전부다.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)

	return string(out)
}

// newGitFixture 는 commit 이 하나 있는 저장소를 만든다.
//
// `.gitignore` 에 `ignored/` 와 `node_modules` 가 들어 있고, 무시되는 자리에 파일이 미리 있다.
// 「무시된 것은 dirty 가 아니다」 는 거의 모든 경우에 같이 봐야 하는 조건이다.
func newGitFixture(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	// symlink 를 풀어 둔다. macOS 의 `/tmp` 가 `/private/tmp` 라, 풀지 않으면 변경 표의 키
	// (openGitRepo 가 푼 뿌리 기준이다) 와 시험이 견주는 경로가 어긋난다.
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")

	write(t, root, ".gitignore", "ignored/\nnode_modules\n*.log\n")
	write(t, root, "main.go", "package main\n")
	write(t, root, "internal/edit.go", "package internal\n")

	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")

	// 무시되는 자리에 파일을 둔다. 뿌리 직속과 중첩 둘 다다 — 중첩이 가지치기가 깨지는 자리다.
	write(t, root, "ignored/out", "x\n")
	write(t, root, "debug.log", "x\n")
	write(t, root, "packages/app/node_modules/lib/index.js", "x\n")

	return root
}

// write 는 fixture 파일 하나를 쓴다. 없는 디렉터리는 만든다.
func write(t *testing.T, root, rel, data string) {
	t.Helper()

	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(data), 0644))
}

// dirtyOf 는 우리 판정이다.
func dirtyOf(t *testing.T, root string) bool {
	t.Helper()

	repo, repoRoot, _, ok := openGitRepo(root)
	require.True(t, ok, "저장소를 열지 못했다")

	head, err := repo.Head()
	require.NoError(t, err)

	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)

	return gitDirty(t.Context(), repo, repoRoot, commit.TreeHash)
}

// dirtyByGit 은 진짜 git 의 판정이다. `git status` 가 한 줄이라도 내면 dirty 다(ADR-0009).
func dirtyByGit(t *testing.T, root string) bool {
	t.Helper()

	return strings.TrimSpace(runGit(t, root, "status", "--porcelain")) != ""
}

// dirty 판정이 `git status` 와 같은지 경우마다 본다.
//
// 기대값을 손으로 적지 않고 git 에게 묻는다. 손으로 적으면 「git 과 같은 기준」이라는
// 이 판정의 목표가 시험에서 빠진다(ADR-0009).
func TestGitDirtyMatchesGitStatus(t *testing.T) {
	cases := map[string]func(t *testing.T, root string){
		"갓 commit 한 그대로": func(*testing.T, string) {},

		"추적 파일을 고쳤다": func(t *testing.T, root string) {
			write(t, root, "main.go", "package main // 고쳤다\n")
		},

		"추적 파일을 지웠다": func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, "main.go")))
		},

		"새 파일을 add 만 했다": func(t *testing.T, root string) {
			write(t, root, "added.go", "package added\n")
			runGit(t, root, "add", "added.go")
		},

		"고친 것을 add 까지 했다": func(t *testing.T, root string) {
			write(t, root, "main.go", "package main // 고쳤다\n")
			runGit(t, root, "add", "main.go")
		},

		"추적 안 하는 파일이 있다": func(t *testing.T, root string) {
			write(t, root, "scratch.go", "package scratch\n")
		},

		"무시되는 파일만 늘었다": func(t *testing.T, root string) {
			write(t, root, "ignored/more", "x\n")
			write(t, root, "another.log", "x\n")
		},

		"중첩된 무시 디렉터리 안이 늘었다": func(t *testing.T, root string) {
			write(t, root, "packages/app/node_modules/lib/other.js", "x\n")
			write(t, root, "packages/web/node_modules/deep/a/b/c.js", "x\n")
		},

		"mtime 만 바뀌고 내용은 그대로다": func(t *testing.T, root string) {
			// 내용이 같으므로 git 은 clean 이라고 한다. metadata 만 보고 dirty 라 하면 틀린다.
			write(t, root, "main.go", "package main\n")
		},

		"실행 권한이 바뀌었다": func(t *testing.T, root string) {
			require.NoError(t, os.Chmod(filepath.Join(root, "main.go"), 0755))
		},

		"아래 디렉터리에 자기 .gitignore 가 있다": func(t *testing.T, root string) {
			write(t, root, "internal/.gitignore", "tmp/\n")
			runGit(t, root, "add", "internal/.gitignore")
			runGit(t, root, "commit", "-m", "nested ignore")
			write(t, root, "internal/tmp/x", "x\n")
		},

		"아래 .gitignore 가 위 규칙을 되살린다": func(t *testing.T, root string) {
			write(t, root, "internal/.gitignore", "!keep.log\n")
			runGit(t, root, "add", "internal/.gitignore")
			runGit(t, root, "commit", "-m", "negate")
			write(t, root, "internal/keep.log", "x\n")
		},

		"add 한 뒤 그 파일을 또 고쳤다": func(t *testing.T, root string) {
			write(t, root, "main.go", "package main // 한 번\n")
			runGit(t, root, "add", "main.go")
			write(t, root, "main.go", "package main // 두 번\n")
		},

		"빈 디렉터리만 늘었다": func(t *testing.T, root string) {
			// git 은 디렉터리를 추적하지 않으므로 clean 이다.
			require.NoError(t, os.MkdirAll(filepath.Join(root, "empty", "deep"), 0755))
		},

		"symlink 가 늘었다": func(t *testing.T, root string) {
			require.NoError(t, os.Symlink("main.go", filepath.Join(root, "link.go")))
		},
	}

	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			root := newGitFixture(t)

			setup(t, root)

			assert.Equal(t, dirtyByGit(t, root), dirtyOf(t, root))
		})
	}
}

// 저장소 뿌리가 아니라 그 아래에서 물어도 뿌리 기준으로 답한다.
// statusBar 는 cwd 저장소 하나를 가리키는 것이라(ADR-0009) 어느 자리에서 물어도 같아야 한다.
func TestGitDirtyFromSubdirectory(t *testing.T) {
	root := newGitFixture(t)
	write(t, root, "scratch.go", "package scratch\n")

	assert.True(t, dirtyOf(t, filepath.Join(root, "internal")))
}

// 무시된 디렉터리 안은 들어가지 않는다. 들어가면 node_modules 가 있는 저장소에서
// 판정 한 번이 수만 개 파일을 훑는다(ADR-0042).
func TestWalkGitFilesSkipsIgnoredDirectories(t *testing.T) {
	root := newGitFixture(t)

	seen := []string{}
	walkGitFiles(t.Context(), root, "", func(rel string) bool {
		seen = append(seen, rel)

		return true
	})

	assert.Contains(t, seen, "main.go")
	assert.Contains(t, seen, ".gitignore")

	for _, rel := range seen {
		assert.NotContains(t, rel, "node_modules", "중첩된 무시 디렉터리에 들어갔다")
		assert.NotContains(t, rel, "ignored/", "무시 디렉터리에 들어갔다")
		assert.NotContains(t, rel, ".log", "무시된 파일을 주었다")
	}
}

// `core.fileMode` 가 꺼져 있으면 실행 권한 차이는 변경이 아니다.
// 권한을 지키지 못하는 파일 시스템에서 저장소가 늘 dirty 로 보이면 표시가 굳는다.
func TestGitDirtyHonorsCoreFileMode(t *testing.T) {
	root := newGitFixture(t)
	runGit(t, root, "config", "core.fileMode", "false")

	require.NoError(t, os.Chmod(filepath.Join(root, "main.go"), 0755))

	assert.Equal(t, dirtyByGit(t, root), dirtyOf(t, root))
	assert.False(t, dirtyOf(t, root), "권한만 바뀌었는데 변경으로 봤다")
}

// 망가진 `.git` 은 저장소가 아니다.
//
// go-git 은 그런 디렉터리도 열고 `Head()` 로 전부 0 인 해시를 준다. 그대로 두면 statusBar 에
// `0000000` 이 찍힌다 — 진짜 git 은 같은 자리를 저장소가 아니라고 물린다(ADR-0042).
func TestOpenGitRepoRejectsBrokenHead(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0755))
	write(t, root, ".git/HEAD", "쓰레기\n")

	_, _, _, ok := openGitRepo(root)

	assert.False(t, ok)
}

// 갓 `git init` 한 저장소는 아직 commit 이 없어 HEAD 가 없는 branch 를 가리킨다.
// 그래도 저장소이므로 무시 규칙과 파일 목록은 그대로 동작해야 한다.
func TestOpenGitRepoAcceptsUnbornHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root := t.TempDir()
	runGit(t, root, "init")

	_, _, _, ok := openGitRepo(root)

	assert.True(t, ok)
}

// 저장소를 symlink 로 지나 열어도 뿌리 기준 상대 경로가 맞아야 한다.
// macOS 의 `/tmp` 가 `/private/tmp` 인 것이 늘 이 자리를 지난다 — 어긋나면 무시 규칙이
// 통째로 듣지 않는다(ADR-0042).
func TestOpenGitRepoResolvesSymlinkedPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	real := t.TempDir()
	runGit(t, real, "init")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "internal"), 0755))

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))

	_, _, rel, ok := openGitRepo(filepath.Join(link, "internal"))

	require.True(t, ok)
	assert.Equal(t, "internal", rel)
}

// `.` 처럼 상대 경로로 물어도 답해야 한다. `readGitStatus` 가 그렇게 부른다 —
// 절대 경로로 맞추지 않으면 뿌리와 견줄 수 없어서 저장소를 못 찾는다.
func TestOpenGitRepoAcceptsRelativePath(t *testing.T) {
	root := newGitFixture(t)

	restore, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(restore) })

	_, _, rel, ok := openGitRepo(".")

	require.True(t, ok, "상대 경로로는 저장소를 못 찾는다")
	assert.Equal(t, "", rel)
}
