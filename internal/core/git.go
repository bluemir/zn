package core

import (
	"os/exec"
	"strings"
)

// gitStatus 는 statusBar 오른쪽에 찍히는 저장소 상태다.
//
// 저장소가 아니거나 git 이 없으면 전부 빈 값이고 아무것도 그리지 않는다.
// sidebar 의 gitignore 표시와 같은 태도다 — 표시가 없을 뿐 틀리지는 않는다.
type gitStatus struct {
	branch string // detached HEAD 면 빈 문자열이다
	commit string // 짧은 해시. 이것이 비면 저장소가 아니거나 commit 이 하나도 없는 것이다
	dirty  bool
}

// readGitStatus 는 cwd 저장소의 상태를 읽는다.
//
// 활성 파일이 아니라 cwd 를 본다. sidebar 트리의 뿌리와 같은 기준이라 tab 을 오가도
// 표시가 흔들리지 않고, 지금 작업 중인 저장소 하나를 가리킨다.
//
// 프로세스를 세 번 띄우므로 화면을 그릴 때마다 부르면 안 된다.
// 편집기를 열 때, 파일을 열 때, 저장할 때만 부른다(ADR-0009).
func readGitStatus() gitStatus {
	// 저장소인지는 이 명령 하나로 갈린다. 저장소 밖이면 exit 128,
	// 저장소이지만 commit 이 하나도 없으면 exit 128 이다. 둘 다 찍을 것이 없다.
	commit, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return gitStatus{}
	}

	// detached HEAD 면 symbolic-ref 가 실패한다. branch 가 비는 것이 곧 detached 다.
	branch, _ := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD").Output()

	// untracked 파일도 dirty 로 센다. `git status` 가 clean 이라고 부르는 것과 같은 기준이다.
	changes, _ := exec.Command("git", "status", "--porcelain").Output()

	return gitStatus{
		branch: strings.TrimSpace(string(branch)),
		commit: strings.TrimSpace(string(commit)),
		dirty:  len(changes) > 0,
	}
}

// label 은 statusBar 에 찍히는 글자다. `master(a1b2c3d*)` 이고 `*` 가 dirty 다.
//
// detached HEAD 는 branch 자리가 비어서 해시만 찍는다. `(a1b2c3d)` 처럼 빈 괄호를 두면
// 이름을 못 읽은 것처럼 보이는데, 사실은 가리킬 branch 가 없는 상태다.
func (g gitStatus) label() string {
	if g.commit == "" {
		return ""
	}

	commit := g.commit
	if g.dirty {
		commit += "*"
	}

	if g.branch == "" {
		return commit
	}

	return g.branch + "(" + commit + ")"
}
