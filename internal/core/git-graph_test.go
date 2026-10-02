package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGitAt 은 커밋 시각까지 정해 두고 git 을 부른다.
//
// `--date` 는 **쓴 때**만 바꾼다. 차례를 정하는 것은 커밋한 때라(addReady) 그쪽은 환경
// 변수로만 정할 수 있고, 그것을 안 정하면 네 커밋이 모두 같은 초에 몰려 차례가 들어온 순서로 갈린다.
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

// 그래프 행이 나오는지. 그림이 맞는지는 진짜 git 과 대는 시험이 본다(git-graph-draw_test.go).
func TestGraphWalkDrawsRows(t *testing.T) {
	rows := walkAll(t, newGraphFixture(t), 100)
	require.Len(t, rows, 4, "뿌리·b·c·merge 넷이다")

	merge := rows[0]

	assert.Len(t, merge.commit.parents, 2, "merge 는 부모가 둘이다")
	assert.GreaterOrEqual(t, len(merge.graph), 2, "커밋 하나가 적어도 두 행이다")
	assert.Contains(t, merge.graph[merge.commitLine], graphNode, "커밋이 선 행에 점이 있다")

	for _, row := range rows {
		assert.Less(t, row.commitLine, len(row.graph), "커밋 행이 담긴 것 안이다")
	}
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

// newSkewFixture 는 시계가 어긋난 저장소를 만든다. merge 가 제 부모보다 **이르게** 커밋됐다.
//
//	M  00:03  (master, HEAD)  merge
//	|\
//	| B  00:05  (topic)  시계가 빠른 자리에서 만든 커밋
//	|/
//	A  00:00
//
// 커밋한 때로만 줄을 세우면 B 가 M 위로 올라가서 합쳐진 갈래가 따로 열린다. topic 이
// 씨앗으로 먼저 큐에 들어가 있는 것이 그 길을 연다.
func newSkewFixture(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

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

	runGit(t, root, "checkout", "-b", "topic")
	commit("b.txt", "2026-01-01T00:05:00+09:00")

	runGit(t, root, "checkout", "master")
	runGitAt(t, root, "2026-01-01T00:03:00+09:00",
		"merge", "--no-ff", "-m", "merge topic", "topic")

	return root
}

// 시계를 믿지 않는다. 자식이 부모보다 이르게 커밋돼도 부모는 자식 아래다 — git 이 그렇다.
func TestGraphWalkIgnoresClockSkew(t *testing.T) {
	assertSameGraph(t, newSkewFixture(t), "--date-order")
}

// commit-graph 파일이 있으면 그 세대 번호로 같은 차례를 낸다.
func TestGraphWalkUsesCommitGraph(t *testing.T) {
	root := newSkewFixture(t)
	runGit(t, root, "commit-graph", "write", "--reachable")

	assertSameGraph(t, root, "--date-order")
}

// commit-graph 파일이 낡아도 맞다. 파일을 쓴 뒤에 생긴 커밋은 그 안에 없다.
func TestGraphWalkStaleCommitGraph(t *testing.T) {
	root := newSkewFixture(t)
	runGit(t, root, "commit-graph", "write", "--reachable")

	runGit(t, root, "checkout", "-b", "late", "topic")
	write(t, root, "c.txt", "c\n")
	runGit(t, root, "add", "-A")
	runGitAt(t, root, "2026-01-01T00:01:00+09:00", "commit", "-m", "c.txt")
	runGit(t, root, "checkout", "master")
	runGitAt(t, root, "2026-01-01T00:02:00+09:00",
		"merge", "--no-ff", "-m", "merge late", "late")

	assertSameGraph(t, root, "--date-order")
}

// newSameSecondFixture 는 모든 커밋이 같은 초에 만들어진 저장소다. 차례가 커밋한 때로는 하나도
// 갈리지 않고, 들어온 순서로만 갈린다.
//
// 갈래 넷을 만들어 둘은 merge 하고 둘은 남긴다. 합친 갈래의 끝에도 ref 가 남아 있어서 씨앗이
// 다른 씨앗의 조상이 된다. remote ref 와 `origin/HEAD` 도 씨앗의 차례에 끼운다.
func newSameSecondFixture(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")

	const when = "2026-01-01T00:00:00+09:00"

	commit := func(rel string) {
		write(t, root, rel, rel+"\n")
		runGit(t, root, "add", "-A")
		runGitAt(t, root, when, "commit", "-m", rel)
	}

	commit("base.txt")

	for _, name := range []string{"zeta", "alpha", "mid", "omega"} {
		runGit(t, root, "checkout", "-b", name, "master")
		commit(name + "-1.txt")
		commit(name + "-2.txt")
	}

	runGit(t, root, "checkout", "master")
	commit("main.txt")
	runGitAt(t, root, when, "merge", "--no-ff", "-m", "merge zeta", "zeta")
	runGitAt(t, root, when, "merge", "--no-ff", "-m", "merge alpha", "alpha")

	runGit(t, root, "branch", "same-as-mid", "mid")
	runGit(t, root, "update-ref", "refs/remotes/origin/omega", "omega")
	runGit(t, root, "update-ref", "refs/remotes/origin/master", "master~1")
	runGit(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")
	runGit(t, root, "tag", "v1", "alpha~1")

	return root
}

// 같은 초이면 git 처럼 먼저 차례에 든 것이 앞이다.
//
// 그림은 견주지 않는다. 이 저장소에서 git 은 열을 `* | | /` 로 당기고 우리는 `* | | |` 로 둔다.
// 차례가 아니라 그리는 쪽의 일이다(`docs/tasks.md`).
func TestGraphWalkSameSecondMatchesGit(t *testing.T) {
	root := newSameSecondFixture(t)

	assert.Equal(t, gitLogHashes(t, root, "--all", "--date-order"), hashesOf(walkAll(t, root, 100)))
}

// commit-graph 로 게으르게 세는 길도 같은 차례다. git 도 두 길이 같은 차례를 낸다.
func TestGraphWalkSameSecondWithCommitGraph(t *testing.T) {
	root := newSameSecondFixture(t)
	runGit(t, root, "commit-graph", "write", "--reachable")

	assert.Equal(t, gitLogHashes(t, root, "--all", "--date-order"), hashesOf(walkAll(t, root, 100)))
}
