package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changesOf 는 그 저장소의 변경 표다.
func changesOf(t *testing.T, root string) gitChanges {
	t.Helper()

	repo, repoRoot, _, ok := openGitRepo(root)
	require.True(t, ok, "저장소를 열지 못했다")

	head, err := repo.Head()
	require.NoError(t, err)

	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)

	return gitFileChanges(t.Context(), repo, repoRoot, commit.TreeHash)
}

// 고친 파일과 새 파일이 다른 갈래로 담긴다. 무시된 것은 담기지 않는다.
func TestGitFileChangesKinds(t *testing.T) {
	root := newGitFixture(t)

	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	write(t, root, "new.go", "package main\n")

	changes := changesOf(t, root)

	assert.Equal(t, gitChangeModified, changes.at(filepath.Join(root, "main.go")))
	assert.Equal(t, gitChangeUntracked, changes.at(filepath.Join(root, "new.go")))
	assert.Equal(t, gitChangeNone, changes.at(filepath.Join(root, "internal/edit.go")), "안 고친 파일")
	assert.Equal(t, gitChangeNone, changes.at(filepath.Join(root, "debug.log")), "무시된 파일")
}

// `git add` 를 해도 표시가 남는다. 기준이 index 가 아니라 HEAD 다(ADR-0094 §1).
func TestGitFileChangesStayAfterAdd(t *testing.T) {
	root := newGitFixture(t)

	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	runGit(t, root, "add", "main.go")

	assert.Equal(t, gitChangeModified, changesOf(t, root).at(filepath.Join(root, "main.go")))
}

// 접혀 있어도 보이도록 위 디렉터리마다 표시가 선다. 뿌리에는 서지 않는다.
func TestGitFileChangesFillsParents(t *testing.T) {
	root := newGitFixture(t)

	write(t, root, "internal/edit.go", "package internal\n\nvar x = 1\n")

	changes := changesOf(t, root)

	assert.Equal(t, gitChangeModified, changes.at(filepath.Join(root, "internal")), "위 디렉터리")
	assert.Equal(t, gitChangeNone, changes.at(root), "뿌리는 늘 서 있게 되므로 두지 않는다")
}

// 안에 고친 파일과 새 파일이 섞여 있으면 디렉터리는 고친 쪽으로 적힌다.
func TestGitFileChangesParentPrefersModified(t *testing.T) {
	root := newGitFixture(t)

	write(t, root, "internal/edit.go", "package internal\n\nvar x = 1\n")
	write(t, root, "internal/new.go", "package internal\n")

	assert.Equal(t, gitChangeModified, changesOf(t, root).at(filepath.Join(root, "internal")))
}

// 지운 파일은 트리에 행이 없다. 표시는 위 디렉터리에 남는다.
func TestGitFileChangesDeletedFile(t *testing.T) {
	root := newGitFixture(t)

	require.NoError(t, os.Remove(filepath.Join(root, "internal/edit.go")))

	changes := changesOf(t, root)

	assert.Equal(t, gitChangeModified, changes.at(filepath.Join(root, "internal/edit.go")))
	assert.Equal(t, gitChangeModified, changes.at(filepath.Join(root, "internal")))
}

// 깨끗한 저장소는 표가 빈다. `*` 판정도 이것으로 답한다(ADR-0094 §2).
func TestGitFileChangesCleanIsEmpty(t *testing.T) {
	root := newGitFixture(t)

	// fixture 는 무시되는 파일만 두고 온다. git 도 이 상태를 clean 이라고 부른다.
	assert.Empty(t, changesOf(t, root))
	assert.False(t, dirtyOf(t, root))
}

// 트리 오른쪽 끝에 마커가 선다. 행 폭은 그대로다.
func TestGitTreeMarkers(t *testing.T) {
	root := newGitFixture(t)

	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	write(t, root, "new.go", "package main\n")

	sidebar := openSidebarSync(t, root)
	cells := sidebar.renderCells(10, "", changesOf(t, root), boxUnicode)

	marks := map[string]string{}
	for _, cell := range cells {
		plain := ansi.Strip(cell)
		if name := strings.TrimSpace(plain[:labelWidth]); name != "" {
			marks[name] = plain[labelWidth : labelWidth+1]
		}

		assert.Equal(t, sidebarWidth, screenColAt([]byte(plain), len(plain), defaultTabWidth), "행 폭은 그대로다: %q", plain)
	}

	assert.Equal(t, markerGitTreeModified, marks["main.go"], "고친 파일")
	assert.Equal(t, markerGitTreeUntracked, marks["new.go"], "추적하지 않는 새 파일")
	assert.Equal(t, " ", marks["▸ internal/"], "안 고친 디렉터리")
	assert.Equal(t, " ", marks["▸ ignored/"], "무시된 디렉터리")
}

// index 의 TREE 캐시가 깨져 있어도 깨끗한 저장소는 깨끗하다.
//
// **이 자리에서 go-git 의 `index.Merged` 에 물렸다**(docs/issues/0002). 캐시가 성하면 그 앞에서
// 끝나서 드러나지 않는다 — 캐시를 깨 두는 이 시험이 그 길을 지난다.
func TestGitFileChangesWithoutTreeCache(t *testing.T) {
	root := newGitFixture(t)

	// 고쳐서 add 하면 캐시가 깨진다. 되돌려 다시 add 하면 index 는 HEAD 와 같아지는데
	// 깨진 캐시는 그대로 남는다. git 도 이 상태를 clean 이라고 부른다.
	write(t, root, "main.go", "package main\n\nvar x = 1\n")
	runGit(t, root, "add", "main.go")
	write(t, root, "main.go", "package main\n")
	runGit(t, root, "add", "main.go")

	require.Empty(t, strings.TrimSpace(runGit(t, root, "status", "--short")), "git 은 깨끗하다고 본다")

	assert.Empty(t, changesOf(t, root))
}

// 표 전체를 `git status` 와 견준다. 갈래까지 같아야 한다.
func TestGitFileChangesMatchesGitStatus(t *testing.T) {
	root := newGitFixture(t)

	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	write(t, root, "internal/new.go", "package internal\n")
	write(t, root, "staged.go", "package main\n")
	runGit(t, root, "add", "staged.go")
	require.NoError(t, os.Remove(filepath.Join(root, "internal/edit.go")))

	changes := changesOf(t, root)

	// `git status --short` 의 두 글자 중 하나라도 `?` 면 추적 안 하는 것이다.
	want := map[string]gitChange{}
	for _, line := range strings.Split(runGit(t, root, "status", "--short"), "\n") {
		if len(line) < 4 {
			continue
		}

		kind := gitChangeModified
		if strings.HasPrefix(line, "??") {
			kind = gitChangeUntracked
		}

		want[filepath.Join(root, strings.TrimSpace(line[2:]))] = kind
	}

	require.NotEmpty(t, want)

	for path, kind := range want {
		assert.Equal(t, kind, changes.at(path), "%s", path)
	}

	// 우리 표에는 디렉터리도 들어 있다. 파일만 골라 개수를 견준다.
	files := 0
	for path := range changes {
		if info, err := os.Lstat(path); err != nil || !info.IsDir() {
			files++
		}
	}

	assert.Equal(t, len(want), files, "git 이 세는 것보다 많지도 적지도 않다")
}
