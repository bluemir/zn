package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitNameStatus 는 진짜 git 이 내는 파일 목록이다. 시험의 기준선이다.
func gitNameStatus(t *testing.T, root, rev string) []string {
	t.Helper()

	out := runGit(t, root, "show", "--name-status", "--format=", "-M", rev)

	var lines []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		// git 은 옮긴 것에 닮은 정도를 붙여 `R100` 이라고 적는다. 우리는 갈래만 보인다.
		fields[0] = strings.TrimRight(fields[0], "0123456789")

		lines = append(lines, strings.Join(fields, " "))
	}

	return lines
}

// ourNameStatus 는 우리가 내는 것을 같은 모양으로 맞춘 것이다.
func ourNameStatus(t *testing.T, root, rev string) []string {
	t.Helper()

	hash := plumbing.NewHash(strings.TrimSpace(runGit(t, root, "rev-parse", rev)))

	detail, err := readCommitDetail(context.Background(), root, hash)
	require.NoError(t, err)

	var lines []string
	for _, file := range detail.files {
		if file.from == "" {
			lines = append(lines, file.action+" "+file.path)

			continue
		}

		lines = append(lines, file.action+" "+file.from+" "+file.path)
	}

	return lines
}

// 고치고 만들고 지운 커밋의 파일 목록을 git 과 견준다.
func TestCommitFilesMatchesGit(t *testing.T) {
	root := newGraphFixture(t)

	write(t, root, "a.txt", "고쳤다\n")
	write(t, root, "new.txt", "새것\n")
	require.NoError(t, os.Remove(filepath.Join(root, "b.txt")))
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "고치고 만들고 지운다")

	assert.Equal(t, gitNameStatus(t, root, "HEAD"), ourNameStatus(t, root, "HEAD"))
}

// 뿌리 커밋은 견줄 앞이 없다. 전부 새로 만든 것이다.
func TestCommitFilesOnRootCommit(t *testing.T) {
	root := newGraphFixture(t)

	assert.Equal(t, []string{"A a.txt"}, ourNameStatus(t, root, "HEAD~1^"))
}

// merge 는 첫 부모와 견준다. git 은 기본으로 아무것도 안 보이므로 그쪽과 견주지 않는다.
func TestCommitFilesOnMerge(t *testing.T) {
	root := newGraphFixture(t)

	assert.Empty(t, gitNameStatus(t, root, "HEAD"), "git 은 merge 에서 아무것도 안 보인다")
	assert.Equal(t, []string{"A c.txt"}, ourNameStatus(t, root, "HEAD"),
		"우리는 첫 부모와 견줘서 갈래가 가져온 것을 보인다")
}

// 옮긴 파일은 `R` 이고 어디서 왔는지도 남는다. git 도 기본으로 찾는다.
func TestCommitFilesDetectsRename(t *testing.T) {
	root := newGraphFixture(t)

	runGit(t, root, "mv", "a.txt", "moved.txt")
	runGit(t, root, "commit", "-m", "옮긴다")

	assert.Equal(t, []string{"R a.txt moved.txt"}, ourNameStatus(t, root, "HEAD"))
	assert.Equal(t, gitNameStatus(t, root, "HEAD"), ourNameStatus(t, root, "HEAD"))
}

// 전문과 부모 해시가 상세 화면이 쓸 모양으로 온다.
func TestReadCommitDetail(t *testing.T) {
	root := newGraphFixture(t)
	runGit(t, root, "commit", "--allow-empty", "-m", "제목\n\n본문 첫 줄\n본문 둘째 줄\n")

	hash := plumbing.NewHash(strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD")))

	detail, err := readCommitDetail(context.Background(), root, hash)
	require.NoError(t, err)

	assert.Equal(t, "제목\n\n본문 첫 줄\n본문 둘째 줄", detail.message)
	assert.Equal(t, "test", detail.author.Name)
	assert.Equal(t, "test@example.com", detail.author.Email)
	assert.Len(t, detail.parents, 1)
	assert.Empty(t, detail.files, "빈 커밋은 건드린 파일이 없다")
}

// merge 의 부모는 둘이다. 상세 화면이 `Merge:` 줄을 그것으로 적는다.
func TestReadCommitDetailOnMergeHasTwoParents(t *testing.T) {
	root := newGraphFixture(t)

	hash := plumbing.NewHash(strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD")))

	detail, err := readCommitDetail(context.Background(), root, hash)
	require.NoError(t, err)

	assert.Len(t, detail.parents, 2)
	assert.Equal(t, gitMinShortHashLen, len(detail.parents[0]))
}

// 옮긴 것은 목록에 어디서 왔는지까지 적힌다.
func TestCommitFileLabel(t *testing.T) {
	assert.Equal(t, "M  main.go", commitFile{action: "M", path: "main.go"}.label())
	assert.Equal(t, "R  old.go → new.go",
		commitFile{action: "R", path: "new.go", from: "old.go"}.label())
}

// 줄 수가 `git show --numstat` 과 같은지 진짜 git 에 대 본다(ADR-0142).
//
// **이 셈의 기준선이다.** 손으로 적은 기대값은 틀린 셈을 굳힌다. git 과 같은가가 곧 맞는지다.
func TestCommitFilesMatchGitNumstat(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root := newGraphFixture(t)

	for _, hash := range gitLogHashes(t, root, "--all") {
		detail, err := readCommitDetail(context.Background(), root, plumbing.NewHash(hash))
		require.NoError(t, err)

		want := gitNumstat(t, root, hash)

		got := map[string][2]int{}
		for _, file := range detail.files {
			got[file.path] = [2]int{file.added, file.removed}
		}

		assert.Equal(t, want, got, "커밋 %s", hash[:7])
	}
}

// gitNumstat 은 `git show --numstat` 이 낸 경로별 줄 수다.
func gitNumstat(t *testing.T, root, hash string) map[string][2]int {
	t.Helper()

	out := runGit(t, root, "show", "--numstat", "--format=", "-m", "--first-parent", hash)

	stat := map[string][2]int{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] == "-" {
			continue
		}

		added, err := strconv.Atoi(fields[0])
		require.NoError(t, err)
		removed, err := strconv.Atoi(fields[1])
		require.NoError(t, err)

		stat[fields[2]] = [2]int{added, removed}
	}

	return stat
}

// 고친 줄은 양쪽에 한 번씩 든다. 사람이 한 일은 하나인데 diff 는 둘로 적는다.
func TestCommitFileCountsChangedLineOnBothSides(t *testing.T) {
	var file commitFile

	before := splitDiffLines("a\nb\nc")
	after := splitDiffLines("a\nB\nc")

	for _, row := range buildDiffRows(before, after) {
		switch row.kind {
		case diffRowAdded:
			file.added++
		case diffRowRemoved:
			file.removed++
		case diffRowChanged:
			file.added, file.removed = file.added+1, file.removed+1
		}
	}

	assert.Equal(t, 1, file.added)
	assert.Equal(t, 1, file.removed)
}

// 부호를 직접 쓴다. `%+d` 는 0 에도 `+` 를 붙여서 지운 것이 없는 파일이 `+0` 으로 적힌다.
func TestCommitFileCountsSigns(t *testing.T) {
	file := commitFile{added: 274, removed: 0}

	assert.Equal(t, "+274", file.addedText())
	assert.Equal(t, "-0", file.removedText())
	assert.Equal(t, "+274    -0", file.counts(4, 5), "칸 넷과 다섯에 맞추고 사이에 한 칸")
}

// 줄로 셀 수 없는 파일은 수 대신 그렇다고 적는다. 비워 두면 안 바뀐 것처럼 보인다.
func TestCommitFileCountsBinary(t *testing.T) {
	assert.Equal(t, "이진", commitFile{binary: true}.counts(4, 4))
}

// 앞 8,000 byte 에 NUL 이 있으면 이진이다. git 이 쓰는 잣대와 같다.
func TestCommitBinarySniff(t *testing.T) {
	assert.Equal(t, 8000, commitBinarySniff)
}
