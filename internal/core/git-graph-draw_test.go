package core

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// git 이 내는 그래프와 **글자 단위로** 댄다(ADR-0141).
//
// **이 시험이 이 기능의 기준선이다.** 옮겨 온 것이 git 의 알고리즘이므로 「git 과 같은가」가
// 곧 맞는지다. 눈으로 읽는 그림을 기대값으로 적어 두면 틀린 그림을 굳히게 된다.

// graphASCII 는 칸을 git 이 쓰는 글자로 바꾼다.
func graphASCII(line []graphSymbol) string {
	out := make([]byte, len(line))
	for i, cell := range line {
		switch cell {
		case graphNode:
			out[i] = '*'
		case graphVertical:
			out[i] = '|'
		case graphSlashUp:
			out[i] = '/'
		case graphSlashDown:
			out[i] = '\\'
		case graphUnderscore:
			out[i] = '_'
		case graphDash:
			out[i] = '-'
		case graphDot:
			out[i] = '.'
		default:
			out[i] = ' '
		}
	}

	return strings.TrimRight(string(out), " ")
}

// gitGraphLines 는 진짜 git 이 낸 그래프 칸만 뽑는다.
//
// `--format=%H` 라 커밋 행은 그래프 뒤에 해시가 붙고 나머지 행은 그래프뿐이다.
func gitGraphLines(t *testing.T, root string, order ...string) []string {
	t.Helper()

	out := runGit(t, root, append([]string{"log", "--graph", "--all", "--format=%H"}, order...)...)

	var lines []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if at := strings.IndexAny(line, "0123456789abcdef"); at >= 0 && len(line[at:]) == 40 {
			line = line[:at]
		}

		lines = append(lines, strings.TrimRight(line, " "))
	}

	return lines
}

// drawnGraphLines 는 우리가 낸 그래프 칸이다. 차례는 훑기가 정한 그대로다.
func drawnGraphLines(t *testing.T, root string) []string {
	t.Helper()

	rows := walkAll(t, root, 100)

	var lines []string
	for _, row := range rows {
		for _, line := range row.graph {
			lines = append(lines, graphASCII(line))
		}
	}

	return lines
}

// assertSameGraph 는 git 과 우리 그림을 나란히 놓고 댄다.
func assertSameGraph(t *testing.T, root string, order ...string) {
	t.Helper()

	want, got := gitGraphLines(t, root, order...), drawnGraphLines(t, root)

	if assert.Equal(t, want, got) {
		return
	}

	// 다르면 나란히 찍는다. 어느 행에서 갈렸는지가 한눈에 보여야 고칠 수 있다.
	for i := 0; i < max(len(want), len(got)); i++ {
		var left, right string
		if i < len(want) {
			left = want[i]
		}
		if i < len(got) {
			right = got[i]
		}

		mark := "  "
		if left != right {
			mark = "≠ "
		}

		t.Logf("%s%-16q %q", mark, left, right)
	}
}

// newDrawFixture 는 시험용 저장소를 짓는다. build 가 git 명령을 부른다.
func newDrawFixture(t *testing.T, build func(root string, commit, merge func(...string))) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")

	clock := 0
	when := func() string {
		clock++

		return time.Date(2026, 1, 1, clock, 0, 0, 0, time.FixedZone("KST", 9*3600)).Format(time.RFC3339)
	}

	// **바퀴마다 내용이 달라야 한다.** 같은 글을 두 번 쓰면 git 이 「고친 것이 없다」로 물린다.
	commit := func(args ...string) {
		name := args[0]
		write(t, root, name+".txt", fmt.Sprintf("%s %d\n", name, clock))
		runGit(t, root, "add", "-A")
		runGitAt(t, root, when(), "commit", "-m", name)
	}

	merge := func(args ...string) {
		runGitAt(t, root, when(), append([]string{"merge", "--no-ff", "-m", "merge " + strings.Join(args, " ")}, args...)...)
	}

	build(root, commit, merge)

	return root
}

// 갈래 하나가 갈렸다 합쳐지는 가장 흔한 모양이다.
func TestGraphMatchesGitSimpleMerge(t *testing.T) {
	root := newDrawFixture(t, func(root string, commit, merge func(...string)) {
		commit("a")
		commit("b")
		runGit(t, root, "checkout", "-b", "topic", "HEAD~1")
		commit("c")
		runGit(t, root, "checkout", "master")
		merge("topic")
	})

	assertSameGraph(t, root, "--date-order")
}

// 갈래가 master 를 끌어왔다가 다시 합쳐진다. backmerge 다.
func TestGraphMatchesGitBackmerge(t *testing.T) {
	root := newDrawFixture(t, func(root string, commit, merge func(...string)) {
		commit("a")
		commit("b")
		runGit(t, root, "checkout", "-b", "feature", "HEAD~1")
		commit("c")
		merge("master")
		commit("e")
		runGit(t, root, "checkout", "master")
		merge("feature")
	})

	assertSameGraph(t, root, "--date-order")
}

// 갈래 둘을 번갈아 합친다. 열이 셋까지 늘었다 줄어드는 자리다.
func TestGraphMatchesGitTwoBranches(t *testing.T) {
	root := newDrawFixture(t, func(root string, commit, merge func(...string)) {
		commit("base")
		runGit(t, root, "branch", "a1")
		runGit(t, root, "branch", "a2")

		for range 3 {
			runGit(t, root, "checkout", "a1")
			commit("a1")
			runGit(t, root, "checkout", "a2")
			commit("a2")
			runGit(t, root, "checkout", "master")
			merge("a1")
			merge("a2")
		}
	})

	assertSameGraph(t, root, "--date-order")
}

// 부모가 넷인 octopus 다. 옆으로 펴는 `-`·`.` 가 여기서만 나온다.
func TestGraphMatchesGitOctopus(t *testing.T) {
	root := newDrawFixture(t, func(root string, commit, merge func(...string)) {
		commit("base")

		for _, name := range []string{"x", "y", "z"} {
			runGit(t, root, "checkout", "-b", name, "master")
			commit(name)
		}

		runGit(t, root, "checkout", "master")
		commit("m")
		merge("x", "y", "z")
	})

	assertSameGraph(t, root, "--date-order")
}

// 갈래가 넷까지 함께 살아 있다. 당기는 행이 여럿 겹치는 자리다.
func TestGraphMatchesGitWideBranches(t *testing.T) {
	root := newDrawFixture(t, func(root string, commit, merge func(...string)) {
		commit("base")

		for _, name := range []string{"p", "q", "r", "s"} {
			runGit(t, root, "checkout", "-b", name, "master")
			commit(name)
			commit(name + "2")
		}

		runGit(t, root, "checkout", "master")
		merge("p")
		merge("q")
		merge("r")
		merge("s")
	})

	assertSameGraph(t, root, "--date-order")
}

// 이 저장소 자체다. 시험용으로 지은 것이 아니라 진짜 기록이다.
func TestGraphMatchesGitThisRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	assertSameGraph(t, ".", "--date-order")
}
