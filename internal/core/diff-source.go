package core

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/bluemir/zn/internal/textarea"
)

// 견줄 두 쪽을 어디서 읽을지 정하고 실제로 읽는 자리다(ADR-0140 §3).
//
// 읽는 일은 전부 Cmd 안에서 돈다. blob 을 푸는 것은 I/O 이고 문법을 통째로 훑는 것도 값이
// 드는 일이라, Update 안에서 할 일이 아니다(ADR-0140 §9).

// diffTargetKind 는 한 쪽을 어디서 읽는지다.
type diffTargetKind byte

const (
	// diffTargetBuffer 는 지금 편집 중인 내용이다. 저장하지 않은 것까지 든다.
	diffTargetBuffer diffTargetKind = iota

	// diffTargetFile 은 디스크의 파일이다.
	diffTargetFile

	// diffTargetCommit 은 그 커밋에 든 그 파일이다.
	diffTargetCommit

	// diffTargetAuto 는 아직 안 푼 `:diff <인자>` 하나다. 파일이 있으면 파일, 아니면 커밋이다.
	diffTargetAuto

	// diffTargetNone 은 빈 쪽이다. 뿌리 커밋의 부모 자리가 이것이다.
	diffTargetNone
)

// diffTarget 은 견주는 한 쪽을 어디서 읽을지다.
type diffTarget struct {
	kind diffTargetKind

	// path 는 읽을 파일이다. 커밋 쪽이면 그 커밋 **안에서 찾을** 이름이다.
	path string

	// rev 는 커밋 이름이다. `HEAD`·`master`·`HEAD~3` 처럼 아직 해시가 아니다.
	rev string

	// arg 는 아직 파일인지 커밋인지 안 가른 인자다. diffTargetAuto 뿐이다.
	arg string

	// lines 는 이미 손에 든 글이다. diffTargetBuffer 가 명령을 친 자리에서 떠 온다.
	lines [][]byte

	// label 은 제목줄에 적을 이름이다. auto 는 읽는 자리에서 정해진다.
	label string
}

// diffRequest 는 무엇과 무엇을 견줄지다.
//
// 왼쪽이 기준(먼저 있던 것) 이고 오른쪽이 지금이다. `enter` 가 여는 것도 오른쪽 파일이다.
type diffRequest struct {
	left, right diffTarget
}

// diffMsg 는 읽어 온 두 쪽과 견준 결과다.
type diffMsg struct {
	left, right diffSide
	hunks       []diffHunk

	// note 는 인자를 어느 쪽으로 풀었는지다. 헷갈릴 자리가 아니었으면 빈 문자열이다.
	note string

	err error
}

// readDiff 는 두 쪽을 읽고 견주는 Cmd 다.
//
// **읽는 것도 견주는 것도 문법을 훑는 것도 전부 여기 안이다.** 화면은 다 된 답만 받는다.
func readDiff(request diffRequest) tea.Cmd {
	return func() tea.Msg {
		left, note, err := readDiffSide(request.left)
		if err != nil {
			return diffMsg{err: err}
		}

		right, rightNote, err := readDiffSide(request.right)
		if err != nil {
			return diffMsg{err: err}
		}

		// 알림줄은 한 줄이라 하나만 선다. 인자를 푼 쪽은 어차피 한쪽뿐이다.
		if note == "" {
			note = rightNote
		}

		return diffMsg{
			left:  left,
			right: right,
			hunks: buildDiffHunks(left.lines, right.lines),
			note:  note,
		}
	}
}

// readDiffSide 는 한 쪽을 읽는다. 두 번째 값은 인자를 어느 쪽으로 풀었는지다.
func readDiffSide(target diffTarget) (diffSide, string, error) {
	switch target.kind {
	case diffTargetBuffer:
		return diffSide{
			label:  target.label,
			path:   target.path,
			lines:  target.lines,
			tokens: lexDiffLines(target.path, target.lines),
		}, "", nil
	case diffTargetFile:
		side, err := readDiffFile(target.path, target.label)

		return side, "", err
	case diffTargetCommit:
		side, err := readDiffCommit(target.rev, target.path, target.label)

		return side, "", err
	case diffTargetAuto:
		return readDiffAuto(target)
	case diffTargetNone:
		return diffSide{label: target.label, path: target.path}, "", nil
	default:
		return diffSide{}, "", errors.New("견줄 것을 모르겠습니다")
	}
}

// readDiffAuto 는 인자 하나를 푼다. **파일이 있으면 경로이고 아니면 커밋이다**(ADR-0140 §3).
//
// git 이 같은 차례로 푼다. `master` 는 둘 다일 수 있는데, 둘 다 맞으면 파일이 이기고 어느
// 쪽으로 풀었는지를 알림줄에 적는다. 조용히 고르면 `:diff master` 가 왜 엉뚱한 것을
// 보여주는지 알 길이 없다.
func readDiffAuto(target diffTarget) (diffSide, string, error) {
	file := isDiffFile(target.arg)
	rev := isDiffRevision(target.arg)

	switch {
	case file && rev:
		side, err := readDiffFile(target.arg, shortenPath(target.arg))

		return side, "`" + target.arg + "` 를 파일로 읽었습니다. 커밋 이름이기도 합니다", err
	case file:
		side, err := readDiffFile(target.arg, shortenPath(target.arg))

		return side, "", err
	case rev:
		// 커밋으로 풀렸으면 볼 파일은 **지금 보고 있는 것**이다. 인자는 기준만 옮긴다.
		side, err := readDiffCommit(target.arg, target.path, target.arg)

		return side, "", err
	default:
		return diffSide{}, "", errors.Errorf("파일도 커밋도 아닙니다: %s", target.arg)
	}
}

// readDiffFile 은 디스크의 파일을 읽는다.
func readDiffFile(path, label string) (diffSide, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return diffSide{}, errors.Wrapf(err, "읽을 수 없습니다: %s", shortenPath(path))
	}

	lines, _ := textarea.SplitLines(data)

	return diffSide{
		label:  label,
		path:   path,
		lines:  lines,
		tokens: lexDiffLines(path, lines),
	}, nil
}

// readDiffCommit 은 그 커밋에 든 그 파일을 읽는다.
//
// **커밋에 없는 파일은 빈 쪽이다.** 새로 만든 파일이 그렇고, 그러면 모든 줄이 넣은 줄로
// 그려진다. 오류로 물리면 「이 파일은 아직 git 밖이다」를 볼 길이 없어진다.
func readDiffCommit(rev, path, label string) (diffSide, error) {
	repo, root, _, ok := openGitRepo(".")
	if !ok {
		return diffSide{}, errors.New("git 저장소가 아닙니다")
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return diffSide{}, errors.Wrapf(err, "커밋을 찾을 수 없습니다: %s", rev)
	}

	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return diffSide{}, errors.Wrapf(err, "커밋을 읽을 수 없습니다: %s", rev)
	}

	// 제목줄에 적는 이름은 부른 그대로이되, 그것이 이름이 아니라 해시였으면 짧게 줄인다.
	short := hash.String()[:min(gitShortHashLen(gitPackedObjectCount(root)), len(hash.String()))]
	if label == "" {
		label = short
	}

	rel, ok := gitRelPath(root, path)
	if !ok {
		return diffSide{}, errors.Errorf("저장소 밖의 파일입니다: %s", shortenPath(path))
	}

	side := diffSide{label: label, path: path}

	file, err := commit.File(rel)
	if err != nil {
		// 그 커밋에는 없던 파일이다. 견줄 원본이 없다는 것도 답이다.
		return side, nil
	}

	contents, err := file.Contents()
	if err != nil {
		return diffSide{}, errors.Wrapf(err, "읽을 수 없습니다: %s", rel)
	}

	side.lines, _ = textarea.SplitLines([]byte(contents))
	side.tokens = lexDiffLines(path, side.lines)

	return side, nil
}

// isDiffFile 은 그 이름이 읽을 수 있는 파일인지다. 디렉터리는 아니다.
func isDiffFile(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}

// isDiffRevision 은 그 이름이 이 저장소의 커밋으로 풀리는지다.
func isDiffRevision(rev string) bool {
	repo, _, _, ok := openGitRepo(".")
	if !ok {
		return false
	}

	_, err := repo.ResolveRevision(plumbing.Revision(rev))

	return err == nil
}
