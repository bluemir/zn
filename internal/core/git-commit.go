package core

import (
	"context"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitFile 은 커밋 하나가 건드린 파일이다. `git log --name-status` 의 한 줄이다.
type commitFile struct {
	action string // `A` 새로 만듦, `M` 고침, `D` 지움, `R` 옮김
	path   string
	from   string // 옮긴 것이면 이전 경로다. 아니면 빈 문자열
}

// label 은 목록에 적히는 한 줄이다. 옮긴 것은 어디서 왔는지까지 적는다.
func (f commitFile) label() string {
	if f.from == "" {
		return f.action + "  " + f.path
	}

	return f.action + "  " + f.from + " → " + f.path
}

// commitDetail 은 커밋 상세 화면이 보여주는 것이다.
//
// 짧은 해시와 ref 이름은 여기 없다. 목록이 이미 들고 있어서(graphCommit) 다시 읽지 않는다.
type commitDetail struct {
	author    object.Signature
	committer object.Signature
	message   string
	parents   []string // 짧은 해시. merge 면 둘 이상이다
	files     []commitFile
}

// readCommitDetail 은 커밋 하나의 전문과 건드린 파일 목록을 읽는다.
//
// **merge 커밋은 첫 부모와 견준다.** `git log --name-status` 는 merge 에서 아무것도 보여주지
// 않는데(`-m` 을 줘야 나온다) 그것보다는 「이 merge 로 무엇이 들어왔나」가 쓸모 있다.
// 갈래에서 이미 본 파일이 다시 서지만, 아무것도 안 서는 것보다 낫다.
//
// 뿌리 커밋은 빈 tree 와 견줘서 전부 `A` 다.
//
// 줄 수(`+12 -3`) 는 내지 않는다. blob 을 전부 풀어 diff 를 돌려야 나오는 값이고,
// 그 값이 필요해지는 것은 diff 화면과 함께다(docs/tasks.md).
func readCommitDetail(ctx context.Context, dir string, hash plumbing.Hash) (commitDetail, error) {
	repo, root, _, ok := openGitRepo(dir)
	if !ok {
		return commitDetail{}, errors.New("git 저장소가 아닙니다")
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		return commitDetail{}, errors.Wrapf(err, "커밋을 읽을 수 없습니다: %s", hash)
	}

	detail := commitDetail{
		author:    commit.Author,
		committer: commit.Committer,
		message:   strings.TrimRight(commit.Message, "\n"),
	}

	short := gitShortHashLen(gitPackedObjectCount(root))
	for _, parent := range commit.ParentHashes {
		text := parent.String()
		detail.parents = append(detail.parents, text[:min(short, len(text))])
	}

	files, err := commitFiles(ctx, repo, commit)
	if err != nil {
		return commitDetail{}, err
	}

	detail.files = files

	return detail, nil
}

// commitFiles 는 커밋이 건드린 파일 목록이다. 첫 부모와 견준다.
func commitFiles(ctx context.Context, repo *git.Repository, commit *object.Commit) ([]commitFile, error) {
	to, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	// 뿌리 커밋이면 견줄 앞이 없다. nil tree 가 빈 tree 다.
	var from *object.Tree
	if len(commit.ParentHashes) > 0 {
		parent, err := repo.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return nil, err
		}

		if from, err = parent.Tree(); err != nil {
			return nil, err
		}
	}

	// 옮긴 파일을 찾는다. git 도 2.9 부터 `--name-status` 에서 기본으로 찾는다.
	//
	// **라이브러리가 권하는 옵션 묶음을 그대로 쓴다.** `DiffTreeOptions{DetectRenames: true}`
	// 만 넘기면 닮은 정도의 문턱이 0 이 되어 **지운 파일과 새로 만든 파일이 무엇이든 짝지어진다** —
	// 내용이 전혀 다른 둘도 「옮긴 것」이 된다. 문턱 60 이 그 묶음 안에 들어 있다.
	changes, err := object.DiffTreeWithOptions(ctx, from, to, object.DefaultDiffTreeOptions)
	if err != nil {
		return nil, err
	}

	files := make([]commitFile, 0, len(changes))
	for _, change := range changes {
		files = append(files, describeChange(change))
	}

	// 경로순으로 세운다. tree 를 훑는 차례는 디렉터리마다 끊겨서 눈으로 훑기 어렵다.
	slices.SortFunc(files, func(a, b commitFile) int {
		return strings.Compare(a.path, b.path)
	})

	return files, nil
}

// describeChange 는 변경 하나를 갈래와 경로로 읽는다.
func describeChange(change *object.Change) commitFile {
	switch {
	case change.From.Name == "":
		return commitFile{action: "A", path: change.To.Name}
	case change.To.Name == "":
		return commitFile{action: "D", path: change.From.Name}
	case change.From.Name != change.To.Name:
		return commitFile{action: "R", path: change.To.Name, from: change.From.Name}
	default:
		return commitFile{action: "M", path: change.To.Name}
	}
}
