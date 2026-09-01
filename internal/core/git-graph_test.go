package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGitAt 은 커밋 시각까지 정해 두고 git 을 부른다.
//
// `--date` 는 **쓴 때**만 바꾼다. 차례를 정하는 것은 커밋한 때라(graphAfter) 그쪽은 환경
// 변수로만 정할 수 있고, 그것을 안 정하면 네 커밋이 모두 같은 초에 몰려 차례가 해시로 갈린다.
func runGitAt(t *testing.T, dir, when string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+when,
		"GIT_COMMITTER_DATE="+when,
	)

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// newGraphFixture 는 갈래가 갈리고 합쳐지는 저장소를 만든다.
//
//	  D  00:03  (master, HEAD)  merge
//	 / \
//	B   C  00:01 / 00:02  (topic 이 C 다)
//	 \ /
//	  A  00:00  뿌리. tag v0.1 이 붙어 있다
//
// 커밋한 때를 1 분씩 벌려 둔다. 시각으로 차례가 갈려야 시험이 무엇을 재는지 흐려지지 않는다.
func newGraphFixture(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	// symlink 를 풀어 둔다. macOS 의 `/tmp` 가 `/private/tmp` 다(git-dirty_test.go).
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")

	commit := func(rel, when string) {
		write(t, root, rel, rel+"\n")
		runGit(t, root, "add", "-A")
		runGitAt(t, root, when, "commit", "-m", rel)
	}

	commit("a.txt", "2026-01-01T00:00:00+09:00")
	runGit(t, root, "tag", "v0.1")

	commit("b.txt", "2026-01-01T00:01:00+09:00")

	runGit(t, root, "checkout", "-b", "topic", "HEAD~1")
	commit("c.txt", "2026-01-01T00:02:00+09:00")

	runGit(t, root, "checkout", "master")
	runGitAt(t, root, "2026-01-01T00:03:00+09:00",
		"merge", "--no-ff", "-m", "merge topic", "topic")

	return root
}

// gitLogHashes 는 진짜 git 이 내는 차례다. 시험의 기준선이다.
func gitLogHashes(t *testing.T, root string, args ...string) []string {
	t.Helper()

	out := runGit(t, root, append([]string{"log", "--format=%H"}, args...)...)

	return strings.Fields(out)
}

// walkAll 은 훑기가 끝까지 내는 것 전부다.
func walkAll(t *testing.T, root string, chunk int) []graphRow {
	t.Helper()

	walk, err := newGraphWalk(root)
	require.NoError(t, err)

	var rows []graphRow
	for {
		next, more, err := walk.next(context.Background(), chunk)
		require.NoError(t, err)

		rows = append(rows, next...)

		if !more {
			return rows
		}
	}
}

func hashesOf(rows []graphRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.commit.hash.String())
	}

	return out
}

// 씨앗이 모든 ref 인지를 진짜 git 과 견준다. `--all` 이 이 기능의 범위다.
func TestGraphWalkCoversAllRefs(t *testing.T) {
	root := newGraphFixture(t)

	assert.Equal(t, gitLogHashes(t, root, "--all"), hashesOf(walkAll(t, root, 100)))
}

// 조각을 나눠 읽어도 한 번에 읽은 것과 같아야 한다. 화면이 조금씩 읽으므로(ADR-0115) 이것이 곧 정확성이다.
func TestGraphWalkChunksMatchWhole(t *testing.T) {
	root := newGraphFixture(t)

	whole := hashesOf(walkAll(t, root, 100))

	for _, chunk := range []int{1, 2, 3} {
		assert.Equal(t, whole, hashesOf(walkAll(t, root, chunk)), "%d 개씩 읽어도 같다", chunk)
	}
}

// 갈래가 갈리고 합쳐지는 자리의 레인이다.
func TestGraphWalkLanes(t *testing.T) {
	root := newGraphFixture(t)

	rows := walkAll(t, root, 100)
	require.Len(t, rows, 4, "뿌리·b·c·merge 넷이다")

	merge, c, b, first := rows[0], rows[1], rows[2], rows[3]

	assert.Len(t, merge.commit.parents, 2, "merge 는 부모가 둘이다")
	assert.Equal(t, 0, merge.lane, "merge 는 첫 열이다")
	assert.Len(t, merge.before, 1, "merge 앞에는 열이 하나뿐이다")
	assert.Len(t, merge.after, 2, "merge 가 열을 하나 연다")

	assert.Equal(t, 1, c.lane, "merge 의 둘째 부모가 둘째 열에 선다")
	assert.Equal(t, 0, b.lane, "첫 부모는 merge 의 열을 물려받는다")

	assert.Equal(t, 0, first.lane)
	assert.Equal(t, []plumbing.Hash{first.commit.hash, first.commit.hash}, first.before,
		"두 갈래가 뿌리를 기다린다")
	assert.Empty(t, first.after, "뿌리에서 열이 모두 닫힌다")
}

// ref 이름표가 git 의 `--decorate` 와 같은 자리에 붙는지.
func TestGraphWalkDecorates(t *testing.T) {
	root := newGraphFixture(t)

	rows := walkAll(t, root, 100)

	assert.Equal(t, []string{"HEAD → master"}, rows[0].commit.refs, "merge 가 master 다")
	assert.Equal(t, []string{"topic"}, rows[1].commit.refs)
	assert.Equal(t, []string{"tag: v0.1"}, rows[3].commit.refs, "뿌리에 tag 가 붙어 있다")
	assert.Empty(t, rows[2].commit.refs)
}

// 떨어진 HEAD(detached) 는 branch 이름에 얹히지 않고 혼자 선다.
func TestGraphWalkDetachedHead(t *testing.T) {
	root := newGraphFixture(t)
	runGit(t, root, "checkout", "--detach", "topic")

	rows := walkAll(t, root, 100)

	assert.Equal(t, []string{"master"}, rows[0].commit.refs, "merge 에서 HEAD 가 떨어졌다")
	assert.Equal(t, []string{"HEAD", "topic"}, rows[1].commit.refs)
}

// 짧은 해시와 제목을 목록이 그대로 쓴다.
func TestGraphWalkDescribes(t *testing.T) {
	root := newGraphFixture(t)

	rows := walkAll(t, root, 100)
	top := rows[0].commit

	assert.Equal(t, "merge topic", top.subject)
	assert.Equal(t, "test", top.author)
	assert.Equal(t, gitMinShortHashLen, len(top.short))
	assert.True(t, strings.HasPrefix(top.hash.String(), top.short))
}

// 저장소가 아니면 열지 않는다. 판이 그 오류를 아래 줄에 적는다.
func TestGraphWalkRefusesNonRepo(t *testing.T) {
	_, err := newGraphWalk(t.TempDir())

	assert.Error(t, err)
}
