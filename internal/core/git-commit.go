package core

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bluemir/zn/internal/textarea"
)

// commitFile 은 커밋 하나가 건드린 파일이다. `git log --numstat` 의 한 줄이다.
type commitFile struct {
	action string // `A` 새로 만듦, `M` 고침, `D` 지움, `R` 옮김
	path   string
	from   string // 옮긴 것이면 이전 경로다. 아니면 빈 문자열

	// added·removed 는 넣은 줄과 들어낸 줄 수다. `git show --stat` 과 같은 셈이다.
	//
	// 고친 줄은 **양쪽에 한 번씩** 든다. 사람이 한 일은 하나인데 diff 는 지우기와 넣기 둘로
	// 적기 때문이고, git 도 그렇게 센다(ADR-0142).
	added   int
	removed int

	// binary 는 줄로 셀 수 없는 파일인지다. 그림과 폰트가 그렇다.
	binary bool
}

// label 은 목록 왼쪽에 적히는 글이다. 옮긴 것은 어디서 왔는지까지 적는다.
//
// 줄 수는 여기 없다. 오른쪽 끝에 맞춰 세우는 것이라 목록 전체의 자릿수를 알아야 하고,
// 그것은 그리는 쪽이 안다(view-commit.go).
func (f commitFile) label() string {
	if f.from == "" {
		return f.action + "  " + f.path
	}

	return f.action + "  " + f.from + " → " + f.path
}

// counts 는 오른쪽 끝에 세울 `+274  -12` 다. 자릿수는 목록에서 가장 긴 것에 맞춘다.
//
// 줄로 셀 수 없는 파일은 수 대신 그렇다고 적는다. 비워 두면 안 바뀐 것처럼 보인다.
// 그 줄은 수 칸에 맞추지 않는다. 오른쪽 끝에 서는 것은 그리는 쪽이 맞춘다.
func (f commitFile) counts(added, removed int) string {
	if f.binary {
		return "이진"
	}

	return fmt.Sprintf("%*s %*s", added, f.addedText(), removed, f.removedText())
}

// addedText·removedText 는 부호를 붙인 수다.
//
// **부호를 직접 쓴다.** `%+d` 는 0 에도 `+` 를 붙여서, 지운 것이 없는 파일이 `+0` 으로 적힌다.
func (f commitFile) addedText() string   { return "+" + formatCount(f.added) }
func (f commitFile) removedText() string { return "-" + formatCount(f.removed) }

// commitDetail 은 커밋 상세 화면이 보여주는 것이다.
//
// 짧은 해시와 ref 이름은 여기 없다. 목록이 이미 들고 있어서(graphCommit) 다시 읽지 않는다.
type commitDetail struct {
	author    object.Signature
	committer object.Signature
	message   string
	parents   []string // 짧은 해시. merge 면 둘 이상이다
	files     []commitFile

	// parentHash 는 **첫 부모의 온전한 해시**다. 뿌리 커밋이면 빈 문자열이다.
	//
	// 위의 parents 와 따로 둔다. 저쪽은 화면에 적는 글이라 짧게 줄인 것이고, 이것은 diff 판이
	// 견줄 쪽을 찾는 데 쓰는 이름이다. 줄인 해시를 되짚어 푸는 것보다 온전한 것을 들고 있는
	// 편이 싸고, 「무엇을 보이나」와 「무엇을 여나」가 한 값에 얹히지 않는다(ADR-0140 §3).
	parentHash string
}

// readCommitDetail 은 커밋 하나의 전문과 건드린 파일 목록을 읽는다.
//
// **merge 커밋은 첫 부모와 견준다.** `git log --name-status` 는 merge 에서 아무것도 보여주지
// 않는데(`-m` 을 줘야 나온다) 그것보다는 「이 merge 로 무엇이 들어왔나」가 쓸모 있다.
// 갈래에서 이미 본 파일이 다시 서지만, 아무것도 안 서는 것보다 낫다.
//
// 뿌리 커밋은 빈 tree 와 견줘서 전부 `A` 다.
//
// **줄 수까지 센다.** blob 을 전부 풀어 diff 를 돌리는 일이라, 재 보니 tree 만 견주던 것에서
// 커밋 하나에 평균 12ms 가 는다(ADR-0142). 이 함수가 통째로 Cmd 안에서 도므로 그동안 화면이
// 멈추지 않는다.
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

	// diff 판이 견줄 앞이다. merge 는 첫 부모와 견주므로 파일 목록과 같은 기준이 된다.
	if len(commit.ParentHashes) > 0 {
		detail.parentHash = commit.ParentHashes[0].String()
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
		file := describeChange(change)

		if err := countChangedLines(change, &file); err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	// 경로순으로 세운다. tree 를 훑는 차례는 디렉터리마다 끊겨서 눈으로 훑기 어렵다.
	slices.SortFunc(files, func(a, b commitFile) int {
		return strings.Compare(a.path, b.path)
	})

	return files, nil
}

// countChangedLines 는 그 변경의 넣은 줄과 들어낸 줄을 센다.
//
// **그리는 셈을 그대로 쓴다**(buildDiffRows). 세기만 하는 길을 따로 내 봤는데 값이 같았다 —
// 병목이 diff 가 아니라 blob 을 푸는 쪽이다. 그러면 줄을 세는 자리가 둘로 갈릴 까닭이 없다
// (ADR-0142).
//
// **읽을 수 없는 쪽은 빈 것으로 친다.** submodule 항목이 그렇다. 파일이 아니라 커밋 해시라
// 풀 blob 이 없고, git 도 그 줄에는 수를 적지 않는다.
func countChangedLines(change *object.Change, file *commitFile) error {
	from, to, err := change.Files()
	if err != nil {
		return err
	}

	before, fromBinary, err := commitFileLines(from)
	if err != nil {
		return err
	}

	after, toBinary, err := commitFileLines(to)
	if err != nil {
		return err
	}

	if fromBinary || toBinary {
		file.binary = true

		return nil
	}

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

	return nil
}

// commitFileLines 는 blob 의 줄들과 그것이 줄로 셀 수 없는 것인지다. 없는 쪽이면 빈 것이다.
//
// **앞 8,000 byte 에 NUL 이 있으면 이진이다.** git 이 쓰는 잣대와 같다. 통째로 훑지 않는
// 것은 큰 파일에서 그 훑기만으로 값이 들기 때문이다.
func commitFileLines(file *object.File) ([][]byte, bool, error) {
	if file == nil {
		return nil, false, nil
	}

	text, err := file.Contents()
	if err != nil {
		return nil, false, err
	}

	data := []byte(text)

	if bytes.IndexByte(data[:min(len(data), commitBinarySniff)], 0) >= 0 {
		return nil, true, nil
	}

	lines, _ := textarea.SplitLines(data)

	return lines, false, nil
}

// commitBinarySniff 는 이진인지 보려고 훑는 byte 수다. git 과 같이 8,000 이다.
const commitBinarySniff = 8000

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
