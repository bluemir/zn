package core

import (
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// openGitRepo 는 dir 이 든 저장소를 열고, 작업 트리의 뿌리와 그 뿌리 기준 dir 의 위치를 준다.
//
// dir 부터 위로 올라가며 `.git` 을 찾는다 — `git` 명령을 그 디렉터리에서 부르던 것과 같은
// 기준이다. 저장소가 아니면 false 고, 부르는 쪽은 표시를 하지 않는다.
//
// bare 저장소도 false 다. 작업 트리가 없으면 무시 규칙도 dirty 도 볼 것이 없다.
//
// rel 은 뿌리 기준 상대 경로이고 뿌리 자신은 빈 문자열이다. 무시 규칙과 index 의 경로가 모두
// 뿌리 기준이라 이것 없이는 견줄 수 없다. 뿌리와 rel 을 여기서 함께 주는 것은 그 둘을 맞추는
// 일이 틀리기 쉬워서다 — 부르는 쪽마다 다시 세면 그 실수가 세 군데로 흩어진다.
func openGitRepo(dir string) (*git.Repository, string, string, bool) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, "", "", false
	}

	if !gitHeadLooksValid(repo) {
		return nil, "", "", false
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, "", "", false
	}

	root := worktree.Filesystem.Root()

	// 뿌리는 절대 경로다. 부르는 쪽은 `.` 처럼 상대 경로를 주므로(readGitStatus 가 그렇다)
	// 먼저 절대 경로로 맞춰야 상대 위치를 셀 수 있다.
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", "", false
	}

	// go-git 은 뿌리에서 symlink 를 풀어서 준다. 부르는 쪽이 준 경로는 풀리지 않은 채일 수
	// 있어서(macOS 의 `/tmp` 가 `/private/tmp` 다) 그대로 견주면 상대 경로가 엉뚱하게 나온다.
	// 같은 자리를 가리키는 두 이름을 한쪽으로 모아 놓고 센다.
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		resolved = absolute
	}

	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return nil, "", "", false
	}

	rel = filepath.ToSlash(rel)
	if rel == "." {
		rel = ""
	}

	// 뿌리 밖이다. 저장소가 아는 것이 없으므로 저장소가 아닌 것과 같이 다룬다.
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, "", "", false
	}

	return repo, root, rel, true
}

// gitHeadLooksValid 는 `.git/HEAD` 가 저장소의 것처럼 보이는지 본다.
//
// go-git 은 진짜 git 보다 관대하다. `.git/HEAD` 에 아무 글자나 들어 있어도 저장소로 열고,
// `Head()` 는 오류 대신 전부 0 인 해시를 준다 — 그대로 두면 statusBar 에 `0000000` 이 찍힌다.
// git 은 같은 디렉터리를 `fatal: not a git repository` 로 물린다.
//
// 갓 `git init` 한 저장소는 HEAD 가 아직 없는 branch 를 가리키는 상징 참조다. 그것은 저장소가
// 맞으므로(`git ls-files` 도 그때 동작한다) 상징 참조는 그대로 통과시키고, 해시로 적힌 것만
// 0 이 아닌지 본다.
func gitHeadLooksValid(repo *git.Repository) bool {
	head, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return false
	}

	if head.Type() == plumbing.SymbolicReference {
		return true
	}

	return !head.Hash().IsZero()
}
