package core

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/go-git/go-git/v5/plumbing"
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

// gitShortHashLen 은 statusBar 에 찍는 commit 해시의 길이다.
//
// `git rev-parse --short` 는 저장소가 커지면 앞자리가 겹치지 않을 만큼 늘리지만, 여기는
// 늘리지 않는다. 늘리려면 객체를 훑어 겹치는지 봐야 하는데, 표시 하나를 위해 낼 값이 아니다.
// 일곱 자리는 git 의 기본값이라 `git log` 와 대개 같게 보인다(ADR-0042).
const gitShortHashLen = 7

// readGitStatus 는 cwd 저장소의 상태를 읽는다.
//
// 활성 파일이 아니라 cwd 를 본다. sidebar 트리의 뿌리와 같은 기준이라 tab 을 오가도
// 표시가 흔들리지 않고, 지금 작업 중인 저장소 하나를 가리킨다.
//
// 프로세스를 띄우지 않고 라이브러리로 읽는다(ADR-0042). 그래도 Update 안에서 부르면 안 된다 —
// 큰 저장소에서 dirty 판정이 백 밀리초를 넘는다. 백그라운드 작업이 부르고 결과만 msg 로
// 돌아온다(ADR-0030). ctx 는 그 작업의 것이라 취소하면 훑는 것이 멎는다.
func readGitStatus(ctx context.Context) gitStatus {
	repo, root, _, ok := openGitRepo(".")
	if !ok {
		return gitStatus{}
	}

	// 저장소이지만 commit 이 하나도 없으면 여기서 갈린다. 찍을 것이 없다.
	head, err := repo.Head()
	if err != nil {
		return gitStatus{}
	}

	// detached HEAD 면 이름이 `HEAD` 그대로다. branch 가 비는 것이 곧 detached 다.
	branch := ""
	if head.Name() != plumbing.HEAD {
		branch = head.Name().Short()
	}

	status := gitStatus{
		branch: branch,
		commit: head.Hash().String()[:gitShortHashLen],
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return status
	}

	status.dirty = gitDirty(ctx, repo, root, commit.TreeHash)

	return status
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

// gitCooldown 은 갱신이 끝난 뒤 다음 갱신까지 쉬는 시간이다.
//
// 주기가 아니라 cooldown 이다 — 5 초를 재는 출발선이 「지난 tick」이 아니라 「지난 갱신이
// 끝난 시각」이다. 그래서 갱신이 오래 걸리는 저장소에서도 쉬는 시간이 반드시 5 초 들어간다.
// 고정 주기였을 때는 갱신이 5 초를 넘으면 끝나는 대로 다음 것이 시작해 사실상 쉼 없이
// 돌았다(ADR-0030 이 감수한 것). cooldown 은 그 상태를 만들지 않는다(ADR-0043).
//
// 5 초는 다른 터미널에서 `commit`·`checkout` 을 하고 편집기로 눈을 돌리는 사이에 값이 맞는
// 정도다.
const gitCooldown = 5 * time.Second

// gitJobName 은 갱신 작업의 이름이자 신원이다.
//
// 같은 이름은 한 번에 하나만 돈다(job.go). cooldown 이라 tick 이 갱신과 겹치지는 않지만,
// `:w` 로 저장하는 길이 갱신을 한 번 더 내므로 그때 겹칠 수 있다 — 그것이 쌓이지 않게 한다.
const gitJobName = "git 상태"

// gitTickMsg 는 다시 읽을 때가 되었다는 것이다. 갱신 자체는 이것을 받고 시작하는 작업이 한다.
type gitTickMsg time.Time

// scheduleGitTick 은 cooldown 을 재고 다음 갱신을 예약한다.
//
// 예약된 tick 은 늘 하나다. `:w` 나 파일 열기로 갱신이 한 번 더 도는 일이 있고, 그것이
// 끝날 때도 이 자리를 지나는데 — 그때마다 새로 걸면 고리가 갈라져서 저장할 때마다 갱신이
// 두 배로 는다. 이미 걸어둔 것이 있으면 그것을 그대로 쓴다(ADR-0043).
func (e *editor) scheduleGitTick() tea.Cmd {
	if e.gitTickScheduled {
		return nil
	}

	e.gitTickScheduled = true

	return tea.Tick(gitCooldown, func(t time.Time) tea.Msg {
		return gitTickMsg(t)
	})
}

// startGitRefresh 는 저장소 상태를 백그라운드에서 읽는 작업을 시작한다.
//
// 결과는 apply 가 editor 에 넣는다 — Update 안에서 불리므로 잠금이 필요 없다(job.go).
func (e *editor) startGitRefresh() tea.Cmd {
	return e.startJob(gitJobName, nil, func(ctx context.Context) <-chan jobProgress {
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
