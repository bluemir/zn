package core

import (
	"context"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
// 프로세스를 세 번 띄우므로 Update 안에서 부르면 안 된다. 백그라운드 작업이 부르고
// 결과만 msg 로 돌아온다(ADR-0030). ctx 는 그 작업의 것이라 취소하면 프로세스까지 끊긴다.
func readGitStatus(ctx context.Context) gitStatus {
	// 저장소인지는 이 명령 하나로 갈린다. 저장소 밖이면 exit 128,
	// 저장소이지만 commit 이 하나도 없으면 exit 128 이다. 둘 다 찍을 것이 없다.
	commit, err := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return gitStatus{}
	}

	// detached HEAD 면 symbolic-ref 가 실패한다. branch 가 비는 것이 곧 detached 다.
	branch, _ := exec.CommandContext(ctx, "git", "symbolic-ref", "--quiet", "--short", "HEAD").Output()

	// untracked 파일도 dirty 로 센다. `git status` 가 clean 이라고 부르는 것과 같은 기준이다.
	changes, _ := exec.CommandContext(ctx, "git", "status", "--porcelain").Output()

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

// gitInterval 은 git 표시를 다시 읽는 주기다.
//
// 5 초는 다른 터미널에서 `commit`·`checkout` 을 하고 편집기로 눈을 돌리는 사이에 값이 맞는
// 정도이면서, 유휴 상태에서 뜨는 프로세스가 분당 서른여섯 개로 그치는 자리다(ADR-0030).
const gitInterval = 5 * time.Second

// gitJobName 은 갱신 작업의 이름이자 신원이다.
//
// 같은 이름은 한 번에 하나만 돈다(job.go). 앞의 갱신이 아직 안 끝났으면 이번 tick 은
// 새로 시작하지 않고 지나간다 — 느린 저장소에서 갱신이 겹쳐 쌓이지 않는다.
const gitJobName = "git 상태"

// gitTickMsg 는 다시 읽을 때가 되었다는 것이다. 갱신 자체는 이것을 받고 시작하는 작업이 한다.
type gitTickMsg time.Time

// tickGit 은 다음 tick 을 예약한다.
//
// 예약은 tick 을 받은 자리에서 한 번씩만 한다(handleJob). 여러 곳에서 걸면 고리가 갈라져서
// 주기가 반으로, 다시 반으로 줄어든다.
func tickGit() tea.Cmd {
	return tea.Tick(gitInterval, func(t time.Time) tea.Msg {
		return gitTickMsg(t)
	})
}

// refreshGit 은 저장소 상태를 백그라운드에서 읽는 작업을 시작한다.
//
// 결과는 apply 가 editor 에 넣는다 — Update 안에서 불리므로 잠금이 필요 없다(job.go).
func (e *editor) refreshGit() tea.Cmd {
	return e.startJob(gitJobName, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress, 1)

		go func() {
			defer close(ch)

			status := readGitStatus(ctx)

			// 끊긴 작업이 읽어온 것은 "저장소가 아니다" 와 구별되지 않는다.
			// 그대로 넣으면 편집기를 끝내는 길에 표시가 사라진다.
			if err := ctx.Err(); err != nil {
				ch <- jobProgress{err: err}

				return
			}

			// 요약은 읽어온 표시 그대로다. `:jobs` 에서 언제 무엇으로 바뀌었는지가 보인다.
			summary := status.label()
			if summary == "" {
				summary = "저장소 아님"
			}

			ch <- jobProgress{
				done:    1,
				total:   1,
				summary: summary,
				apply:   func(e *editor) { e.git = status },
			}
		}()

		return ch
	})
}
