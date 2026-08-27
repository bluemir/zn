package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// jobProgress 는 백그라운드 작업이 채널로 보내는 한 조각이다.
//
// 실행기는 done/total 만 안다. 결과가 무엇이고 어디에 어떻게 쌓이는지는 작업이 정한다 —
// apply 가 그것이다. 작업이 자기 결과를 editor 의 자기 자리에 넣는 함수를 같이 실어 보낸다.
// 아직 알릴 결과가 없는 조각은 nil 이다.
//
// 실행기가 결과 모양을 정하면 소비자가 늘 때마다 깨진다. 여러 파일 검색의 결과는 파일·줄번호·
// 내용이라 파일 목록과 같은 모양이 아니고, 실행기가 소비자를 하나씩 알게 되는 것도 방향이 뒤집힌
// 것이다. interface 를 두지 않은 것은 지금 필요한 것이 "editor 를 고치는 함수 하나" 뿐이라서다.
//
// apply 는 goroutine 이 아니라 handleJob 안에서, 곧 Update 안에서만 불린다.
// 그래서 editor 를 그냥 고치면 된다 — 잠금도 복사도 필요 없다.
type jobProgress struct {
	done, total int // total 이 0 이면 전체를 아직 모른다
	apply       func(*editor)

	// summary 는 끝나며 남기는 한 줄이고 err 은 실패한 이유다. 마지막 조각만 채운다.
	// 무엇이 결과인지는 여기서도 작업이 정한다 — 실행기는 목록에 옮겨 적기만 한다.
	summary string
	err     error
}

// job 은 도는 작업 하나이자 끝난 작업 하나다. statusBar 와 `:jobs` 목록이 이것을 읽는다.
// 채널은 여기 두지 않는다 — msg 가 들고 다닌다.
//
// **신원은 name 과 args 를 합친 것이다.** 같은 신원은 한 번에 하나만 돈다 — 두 번째 요청은
// 새로 시작하지 않고 돌고 있는 것에 붙는다.
//
// name 은 하는 일의 갈래이고 args 는 그 일이 무엇을 대상으로 하는지다. 갈랐기 때문에
// 디렉터리 읽기가 경로마다 따로 돌면서도 목록에서는 이름 하나로 모인다. 이름에 경로를
// 이어 붙이던 때는 신원이 곧 화면 글자라, 같은 일을 하는 작업들이 목록에서 서로 남이었다
// (ADR-0075).
//
// args 가 빈 작업이 여섯이고(git 상태·파일 검사·gopls 설치·goimports 설치·파일 인덱싱·
// 유니코드 훑기) 그것들은 신원이 곧 이름이라 한 번에 하나만 돈다.
type job struct {
	name        string
	args        []string
	done, total int
	started     time.Time

	// cancel 은 이 작업의 ctx 를 끊는다. 끝난 작업은 nil 이다.
	cancel context.CancelFunc

	// 아래 셋은 끝난 뒤에만 찬다. 상태를 따로 두지 않는 것은 "끝난 목록에 있는가" 와
	// "err 이 무엇인가" 로 이미 갈리기 때문이다.
	finished time.Time
	summary  string // 작업이 남긴 한 줄. "151,933 개" 처럼 무엇을 했는지다
	err      error  // context.Canceled 면 취소, 그 밖이면 실패
}

// is 는 이 작업이 그 신원인지다. 이름과 인자가 모두 같아야 같은 작업이다.
func (j job) is(name string, args []string) bool {
	return j.name == name && slices.Equal(j.args, args)
}

// title 은 이름과 인자를 이은 한 줄이다.
//
// `:jobs` 목록은 이것을 쓰지 않는다 — 거기서는 이름이 부모 줄이고 인자가 자식 줄이라
// 둘이 이미 갈려 있다. 목록 바깥에서 작업 하나를 가리켜야 하는 자리, 곧 실패 알림이 쓴다.
// 인자를 떨구면 `디렉터리 읽기 실패` 가 어느 디렉터리인지 없이 알림 목록에 남는다.
func (j job) title() string {
	if len(j.args) == 0 {
		return j.name
	}

	return j.name + " " + strings.Join(j.args, " ")
}

// label 은 끝난 작업의 상태와 요약이다. `:jobs` 목록이 진행 막대 자리에 대신 넣는다.
func (j job) label() string {
	switch {
	case j.err == nil:
		return "끝남  " + j.summary
	case errors.Is(j.err, context.Canceled):
		return "취소됨"
	default:
		return "실패  " + j.err.Error()
	}
}

// elapsed 는 걸린 시간이다. 도는 중이면 지금까지, 끝났으면 끝날 때까지다.
func (j job) elapsed(now time.Time) time.Duration {
	if !j.finished.IsZero() {
		return j.finished.Sub(j.started)
	}

	return now.Sub(j.started)
}

// jobProgressMsg 는 다음 조각을 받을 채널을 같이 들고 다닌다.
//
// 받은 자리에서 바로 고리를 이을 수 있어서 목록을 뒤질 필요가 없다. 아직 목록에 없는
// 이름의 조각이 와도 마찬가지다.
type jobProgressMsg struct {
	name string
	args []string
	ch   <-chan jobProgress

	jobProgress
}

// jobDoneMsg 는 채널이 닫혔다는 것이다. 작업이 끝났다.
type jobDoneMsg struct {
	name string
	args []string
}

// waitJob 은 채널에서 조각 하나를 받아 msg 로 바꾸는 Cmd 다.
//
// 받을 때마다 다시 발행해야 다음 조각이 온다. handleJob 이 그것을 한다.
func waitJob(name string, args []string, ch <-chan jobProgress) tea.Cmd {
	return func() tea.Msg {
		progress, ok := <-ch
		if !ok {
			return jobDoneMsg{name: name, args: args}
		}

		return jobProgressMsg{name: name, args: args, ch: ch, jobProgress: progress}
	}
}

// startJob 은 작업을 시작한다. 같은 이름이 이미 돌고 있으면 시작하지 않고 nil 을 준다.
//
// **이름 규칙**: 작업을 여는 일만 하는 함수는 `start*` 다(`startJob`·`startTree`·`startGitRefresh`).
// 역할이 다른 자리는 그 역할로 이름 짓고(`tickGit`·`waitJob`·`continueReveal`), 하는 일이 따로
// 있으면서 곁들여 Cmd 가 나오는 함수는 이름을 건드리지 않고 왜 나오는지 doc 주석에 적는다
// (`nextTab`·`toggleNode`·`clickTabline`).
//
// 채널이 아니라 채널을 만드는 함수를 받는다. 채널을 먼저 만들면 이미 돌고 있을 때
// 갈 곳 없는 goroutine 이 하나 뜬다.
//
// 작업마다 ctx 를 나눠 준다. 취소는 그것을 끊는 것이고, 편집기를 끝내면 루트가 끊겨 전부 정리된다.
func (e *editor) startJob(name string, args []string, start func(context.Context) <-chan jobProgress) tea.Cmd {
	if e.jobRunning(name, args) {
		return nil
	}

	ctx, cancel := context.WithCancel(e.rootContext())
	e.putJob(job{name: name, args: args, started: time.Now(), cancel: cancel})

	return waitJob(name, args, start(ctx))
}

// rootContext 는 작업들이 갈라져 나오는 뿌리다.
//
// core.Run 이 받은 것을 editor 가 들고 있다. 테스트는 editor 를 직접 만들어서 비어 있으므로
// 그때는 Background 다 — 취소가 프로세스 종료까지 이어지지 않을 뿐이고 동작은 같다.
func (e editor) rootContext() context.Context {
	if e.ctx == nil {
		return context.Background()
	}

	return e.ctx
}

// cancelJob 은 그 작업에게 그만하라고 말한다. 목록에서 바로 빼지 않는다 —
// 언제 진짜 끝났는지는 작업이 알고, 채널을 닫으면 jobDoneMsg 가 와서 나머지는 같은 길로 흐른다.
//
// 취소했다는 것은 여기서 적는다. 작업이 알려주기를 기다리면, 조각을 보내다 끊긴 작업은
// 아무 말 없이 채널만 닫아서 목록에 「끝남」으로 남는다.
func (e *editor) cancelJob(name string, args []string) {
	for i := range e.jobs {
		if !e.jobs[i].is(name, args) || e.jobs[i].cancel == nil {
			continue
		}

		e.jobs[i].err = context.Canceled
		e.jobs[i].cancel()

		return
	}
}

// handleJob 은 mode 가 공유하는 작업 msg 처리다. 다음 조각을 받을 Cmd 를 준다.
//
// **model 을 돌려주면 mode 가 바뀐다.** 바꾸지 않으면 nil 이라 부르는 쪽이 지금 mode 를
// 그대로 쓴다 — 동작(action.run) 과 같은 규칙이다(ADR-0026). 정의 후보가 여럿이라 고르는
// 화면을 여는 자리 하나가 이것을 쓴다(ADR-0051).
//
// mode 마다 `case jobProgressMsg, jobDoneMsg, gitTickMsg:` 한 자리를 두고 여기로 넘긴다. 하는 일은
// 여기 하나로 모여 있고(ADR-0002 가 "늘어나면 공용 처리로 뺀다" 고 적어둔 자리다) mode 쪽에는 어떤
// msg 를 받는지가 남는다. default 에 숨기면 그 mode 가 작업 msg 를 받는다는 것이 보이지 않는다.
//
// 결과가 무엇인지는 대개 여기서 알지 못한다. 이름을 보는 곳은 한 자리뿐이다 — 주기 작업이
// 끝나면 cooldown 을 다시 걸어야 하고(ADR-0043, ADR-0044), 그 고리를 잇는 자리가 여기여야
// mode 를 오갈 때 갈라지지 않는다(ADR-0030, ADR-0038).
func (e *editor) handleJob(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case gitTickMsg:
		// 여기서는 다음 것을 예약하지 않는다. cooldown 은 갱신이 *끝난* 뒤부터 재는 것이라
		// 예약은 jobDoneMsg 자리에서 한다(ADR-0043).
		e.gitTickScheduled = false

		return nil, e.startGitRefresh()
	case fileTickMsg:
		// 검사는 작업이 한다. 어느 mode 에서 받았는지는 보지 않는다 — 읽을지 말지를 가르는
		// 것은 `dirty` 하나이고, 그 판정은 결과가 돌아온 뒤에 한다(ADR-0044).
		e.fileTickScheduled = false

		return nil, e.startOutsideCheck()
	case lspTickMsg:
		// 언어 서버와 맞출 때다. 다음 것을 여기서 예약하지 않는다 — 주기가 아니라 마지막 키에서
		// 재는 것이라, 예약은 키를 받는 자리가 한다(ADR-0051).
		e.lspTickScheduled = false

		return nil, e.syncGopls()
	case goplsReadyMsg:
		return nil, e.finishGopls(msg)
	case diagnosticsMsg:
		// 서버가 진단을 밀어 주었다. 우리가 물은 답이 아니라 서버가 자기 때에 보낸 것이라
		// 여기서 화면에 올리고 다음 것을 기다리는 고리를 다시 잇는다(diagnostics.go, ADR-0086).
		e.applyDiagnostics()

		if e.gopls == nil {
			// 서버를 내리는 중에 마지막 종이 울린 것이다. 기다릴 상대가 없다.
			return nil, nil
		}

		return nil, waitDiagnostics(e.gopls)
	case definitionMsg:
		// 정의를 물은 답이다. 후보가 하나면 그 자리로 뛰고(mode 그대로) 여럿이면 고르는
		// 화면을 연다 — mode 를 바꾸는 유일한 작업 결과다(ADR-0051).
		return e.finishDefinition(msg)
	case referencesMsg:
		// 사용처를 물은 답이다. 하나면 그 자리로 뛰고 여럿이면 고르는 화면을 연다 —
		// 정의와 같은 길이고 같은 화면이다(references.go, ADR-0068).
		return e.finishReferences(msg)
	case renameMsg:
		// 이름을 물은 답이다. 여기서 파일을 고치고 쓴다(rename.go, ADR-0067).
		// mode 는 바꾸지 않는다 — 어느 화면에서 답이 오든 그 화면 그대로다.
		e.finishRename(msg)

		return nil, nil
	case jobProgressMsg:
		e.updateJob(msg)

		if msg.apply != nil {
			msg.apply(e)
		}

		// 트리가 자식을 기다리며 멈춰 있었으면 여기서 다음 층으로 나아간다.
		// apply 는 `func(*editor)` 라 Cmd 를 낼 수 없어서 이 자리가 그것을 대신한다(ADR-0032).
		// 기다리는 것이 없으면 곧바로 nil 이라 다른 작업의 조각에는 얹히지 않는다.
		return nil, tea.Batch(waitJob(msg.name, msg.args, msg.ch), e.continueReveal())
	case jobDoneMsg:
		e.finishJob(msg.name, msg.args)

		// 주기 작업이 끝났으면 여기서부터 cooldown 을 잰다. 주기를 잇는 자리가 한 곳이라는
		// 규칙은 그대로고, 그 한 곳이 tick 받는 자리에서 작업 끝나는 자리로 옮겨온 것이다
		// (ADR-0030, ADR-0043, ADR-0044).
		switch msg.name {
		case gitJobName:
			return nil, e.scheduleGitTick()
		case fileJobName:
			return nil, e.scheduleFileTick()
		case goplsJobName:
			if !e.goplsFailed {
				return nil, e.startGoplsForOpenBuffers()
			}
		}

		return nil, nil
	default:
		return nil, nil
	}
}

// updateJob 은 진행을 목록에 옮겨 적는다. 마지막 조각의 summary·err 도 여기서 받아 둔다 —
// 채널이 닫히는 것은 그 다음이라 finishJob 은 이미 적힌 것을 쓴다.
func (e *editor) updateJob(msg jobProgressMsg) {
	for i := range e.jobs {
		if !e.jobs[i].is(msg.name, msg.args) {
			continue
		}

		e.jobs[i].done, e.jobs[i].total = msg.done, msg.total
		if msg.summary != "" {
			e.jobs[i].summary = msg.summary
		}
		if msg.err != nil {
			e.jobs[i].err = msg.err
		}

		return
	}

	// 목록에 없는 신원이면 끼워 넣는다. 시작한 자리를 지나온 msg 여도 여기서 자리를 잡는다.
	e.jobs = append(e.jobs, job{
		name:    msg.name,
		args:    msg.args,
		done:    msg.done,
		total:   msg.total,
		started: time.Now(),
		summary: msg.summary,
		err:     msg.err,
	})
}

// jobRunning 은 그 신원의 작업이 돌고 있는지다.
func (e editor) jobRunning(name string, args []string) bool {
	for _, running := range e.jobs {
		if running.is(name, args) {
			return true
		}
	}

	return false
}

// putJob 은 작업을 목록에 넣는다. 같은 신원이 있으면 갈아끼운다.
func (e *editor) putJob(next job) {
	for i := range e.jobs {
		if e.jobs[i].is(next.name, next.args) {
			e.jobs[i] = next

			return
		}
	}

	e.jobs = append(e.jobs, next)
}

// finishJob 은 끝난 작업을 도는 목록에서 끝난 목록으로 옮긴다.
//
// **찾는 것은 신원(이름+인자) 이고, 끝난 목록에서 미는 것은 이름이다.** 끝난 목록은 이름당
// 마지막 결과 하나이고, 개수 상한은 두지 않는다 — 이름은 코드에 있는 종류만큼만 있다. 끝난
// 순서대로 쌓으면 주기적으로 도는 git 갱신이 목록을 자기 이름으로 뒤덮어서, 조용히 실패한 다른
// 작업을 찾으라고 남겨둔 자리가 그것으로 다 찬다(ADR-0030).
//
// 이름에 경로를 이어 붙이던 때는 그 「이름은 종류만큼만」이 디렉터리 읽기에서 깨졌다 —
// 펼친 디렉터리마다 이름이 달라서 끝난 목록에 한 줄씩 쌓였다. 갈라 놓으니 그 가정이 되돌아온다.
// 지워지는 것은 어느 경로가 언제 끝났는지인데, 실패는 알림 목록에 통째로 남으므로 유실이 없다
// (ADR-0053, ADR-0075).
//
// 실패는 statusBar 아래 줄로도 알린다 — 목록을 열어 보기 전에는 아무 일도 없던 것처럼 보이기
// 때문이다. 취소는 알리지 않는다. 그만하라고 한 사람이 결과를 이미 안다.
func (e *editor) finishJob(name string, args []string) {
	for i, running := range e.jobs {
		if !running.is(name, args) {
			continue
		}

		running.cancel = nil
		running.finished = time.Now()

		e.jobs = slices.Delete(e.jobs, i, i+1)
		e.finished = append([]job{running}, slices.DeleteFunc(e.finished, func(old job) bool {
			return old.name == name
		})...)

		if running.err != nil && !errors.Is(running.err, context.Canceled) {
			e.notifyFailure(running.title() + " 실패: " + running.err.Error())
		}

		return
	}
}

// 막대 크기다. 칸 하나가 여섯 단계라 열 칸이면 예순 단계다.
const (
	jobBarCells = 10
	jobBarSteps = 6
)

// jobBarRunes 는 채운 정도별 글자다. 바닥선만 있는 것(0)부터 꽉 찬 것(6)까지 점이 하나씩 는다.
//
// 점자 한 칸은 2열×4행이고 바닥 줄(dot 7·8)을 늘 켜 두므로 남는 것이 여섯 점이다.
// 왼쪽 열을 아래에서 위로 채우고 오른쪽 열로 넘어간다. 빈 칸이 바닥선으로 남아 막대의 끝이 보인다.
//
// 점자(U+2800–U+28FF)는 East Asian Width 가 Neutral 이라 폭이 확실하다. 블록 문자(`█`)와
// 박스 그리기 문자는 Ambiguous 라 터미널이 두 칸으로 잡으면 statusBar 가 넘쳐 화면이 밀린다.
// ADR-0020 이 공백 마커로 `»` 를 고른 것과 같은 기준이다.
var jobBarRunes = []rune{'⣀', '⣄', '⣆', '⣇', '⣧', '⣷', '⣿'}

// renderBar 는 막대다. 전체 대비 얼마나 왔는지를 예순 단계로 나눈다.
func renderBar(done, total int) string {
	steps := 0
	if total > 0 {
		steps = done * jobBarCells * jobBarSteps / total
	}
	steps = min(max(steps, 0), jobBarCells*jobBarSteps)

	bar := make([]rune, 0, jobBarCells)
	for cell := range jobBarCells {
		bar = append(bar, jobBarRunes[min(max(steps-cell*jobBarSteps, 0), jobBarSteps)])
	}

	return string(bar)
}

// renderJobBar 는 맨 앞 작업의 막대다. 전체를 모르면 그리지 않는다 — 반쯤 찬 막대가 거짓말이 된다.
func (e editor) renderJobBar() string {
	if len(e.jobs) == 0 || e.jobs[0].total <= 0 {
		return ""
	}

	return renderBar(e.jobs[0].done, e.jobs[0].total)
}

// renderJobText 는 statusBar 에 붙는 진행 표시다. 도는 것이 없으면 빈 문자열이다.
//
// 목록 맨 앞, 곧 가장 먼저 시작한 것을 찍는다. 끝날 때까지 가리키는 것이 바뀌지 않아야
// 눈이 따라갈 수 있다. 나머지는 개수로만 알린다.
//
// bar 는 막대다. 오른쪽에 붙일 칸이 모자라면 부르는 쪽이 빈 문자열을 준다.
func (e editor) renderJobText(bar string) string {
	if len(e.jobs) == 0 {
		return ""
	}

	first := e.jobs[0]

	text := first.name
	if bar != "" {
		text += " " + bar
	}

	// 전체를 모르면 지금까지 한 개수가 유일한 단서다.
	if first.total > 0 {
		text += fmt.Sprintf(" %d%%", 100*first.done/first.total)
	} else {
		text += " " + formatCount(first.done)
	}

	if rest := len(e.jobs) - 1; rest > 0 {
		text += fmt.Sprintf(" (+%d)", rest)
	}

	return text
}

// formatCount 는 세 자리마다 쉼표를 넣는다. 수만 개가 지나가는 자리라 자릿수를 눈으로 세게 되면 안 된다.
func formatCount(n int) string {
	digits := strconv.Itoa(n)

	head := len(digits) % 3
	if head == 0 {
		head = 3
	}

	groups := []string{digits[:head]}
	for i := head; i < len(digits); i += 3 {
		groups = append(groups, digits[i:i+3])
	}

	return strings.Join(groups, ",")
}
