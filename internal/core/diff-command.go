package core

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// `:diff` 가 들어오는 자리다(ADR-0140 §3).
//
// 문이 다섯인데 넷이 여기로 오고(인자 없음·하나·둘) 나머지 하나는 `COMMIT` 의 `enter` 다
// (view-commit.go). 정하는 것은 「견줄 두 쪽이 무엇인가」뿐이고, 읽는 것은 전부 판이
// 발행하는 Cmd 안에서 돈다(diff-source.go).

// runDiff 는 `:diff` 와 팔레트의 「파일 비교」다.
func runDiff(e *editor, args []string) (tea.Model, tea.Cmd) {
	request, err := diffRequestFor(e, args)
	if err != nil {
		return normalModeError(e, err)
	}

	return diffMode(nil, e, request)
}

// runDiffPalette 는 팔레트에서 부르는 자리다. 인자 없는 `:diff` 와 같은 길이다.
//
// 고른 범위를 보지 않는다. 견주는 것이 파일 둘이라 고른 줄이 가리킬 것이 없다
// (ADR-0111 이 실어 보내는 것을 여기서는 쓰지 않는다).
func runDiffPalette(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return runDiff(e, nil)
}

// diffRequestFor 는 친 인자로 견줄 두 쪽을 정한다.
//
//	:diff              HEAD 의 이 파일과 지금 buffer
//	:diff <인자>        그 인자와 지금 buffer. 파일이 있으면 경로이고 아니면 커밋이다
//	:diff <A> <B>      A 파일과 B 파일
//
// **인자가 둘이면 둘 다 경로다.** 커밋과 경로를 섞으려면 `--` 로 가르는 규칙이 필요해지는데
// (`:diff master -- path`), 그 손이 필요해질 때 본다. 규칙을 하나로 두는 편이 지금 값이 크다.
func diffRequestFor(e *editor, args []string) (diffRequest, error) {
	if len(args) > 2 {
		return diffRequest{}, errors.New("견줄 것은 둘까지입니다")
	}

	if len(args) == 2 {
		return diffRequest{left: diffFileTarget(args[0]), right: diffFileTarget(args[1])}, nil
	}

	// 인자가 하나 이하면 오른쪽은 지금 편집 중인 내용이다. 저장하지 않은 것까지 든다.
	right, err := diffBufferTarget(e)
	if err != nil {
		return diffRequest{}, err
	}

	if len(args) == 0 {
		return diffRequest{
			left:  diffTarget{kind: diffTargetCommit, rev: "HEAD", path: right.path, label: "HEAD"},
			right: right,
		}, nil
	}

	return diffRequest{
		left:  diffTarget{kind: diffTargetAuto, arg: args[0], path: right.path},
		right: right,
	}, nil
}

// diffBufferTarget 은 지금 편집 중인 내용을 오른쪽 쪽으로 만든다.
//
// **줄을 여기서 떠 간다.** 읽는 일은 Cmd 안에서 도는데 그 사이에도 이 buffer 는 살아 있다.
// 판이 열려 있는 동안 고칠 수는 없지만(읽기만 하는 판이다) 바깥 변경을 따라 다시 읽는 길이
// 열려 있어서(ADR-0038), 그때 갈아끼워질 것을 지금 값으로 든다.
func diffBufferTarget(e *editor) (diffTarget, error) {
	if !e.hasTab() {
		return diffTarget{}, errors.New("볼 파일이 없습니다")
	}

	path := e.activePath()
	if path == "" {
		return diffTarget{}, errors.New("이름 없는 파일입니다")
	}

	buf := e.activeBuffer()

	lines := make([][]byte, buf.LineCount())
	for i := range lines {
		lines[i] = buf.Line(i)
	}

	return diffTarget{kind: diffTargetBuffer, path: path, lines: lines, label: "지금"}, nil
}

// diffFileTarget 은 경로 하나를 한 쪽으로 만든다.
//
// `~` 를 여기서 펴지 않는다. 명령줄이 인자를 넘기기 전에 이미 폈다 — 어느 명령에서 되고 어느
// 명령에서 안 되는지를 세는 자리를 만들지 않으려는 것이다(view-editor-command.go, ADR-0088).
func diffFileTarget(arg string) diffTarget {
	return diffTarget{kind: diffTargetFile, path: arg, label: shortenPath(arg)}
}

// diffCommitRequest 는 `COMMIT` 의 `enter` 가 쓰는 것이다. 그 커밋의 부모와 그 커밋을 견준다.
//
// **경로는 작업 트리 기준으로 맞춰 넘긴다.** `enter` 로 뛰는 곳이 작업본이라, 저장소 뿌리
// 기준 이름이 아니라 지금 열 수 있는 경로여야 한다(ADR-0140 §5).
//
// **왼쪽과 오른쪽의 경로가 갈릴 수 있다.** 옮긴 파일이 그렇다. 한 이름으로 묶으면 옮긴 것이
// 통째로 지우고 새로 만든 것으로 보인다.
//
// **뿌리 커밋은 왼쪽이 빈 쪽이다.** 견줄 앞이 없어서 모든 줄이 넣은 줄로 그려진다.
// `readCommitDetail` 이 빈 tree 와 견줘 전부 `A` 로 내는 것과 같은 답이다(git-commit.go).
func diffCommitRequest(detail commitDetail, commit graphCommit, file commitFile, root string) diffRequest {
	from := file.path
	if file.from != "" {
		from = file.from
	}

	left := diffTarget{
		kind:  diffTargetCommit,
		rev:   detail.parentHash,
		path:  filepath.Join(root, from),
		label: firstOr(detail.parents, "없음"),
	}

	if detail.parentHash == "" {
		left = diffTarget{kind: diffTargetNone, path: filepath.Join(root, from), label: "없음"}
	}

	return diffRequest{
		left: left,
		right: diffTarget{
			kind:  diffTargetCommit,
			rev:   commit.hash.String(),
			path:  filepath.Join(root, file.path),
			label: commit.short,
		},
	}
}

// firstOr 는 목록의 첫 값이다. 비어 있으면 대신 쓸 말이다.
func firstOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}

	return values[0]
}
