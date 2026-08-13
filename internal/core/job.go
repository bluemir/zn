package core

import (
	"fmt"
	"strconv"
	"strings"

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
}

// job 은 statusBar 에 진행을 찍기 위한 것이다. 채널은 여기 두지 않는다 — msg 가 들고 다닌다.
//
// name 은 statusBar 에 찍히는 이름이자 신원이다. 같은 이름은 한 번에 하나만 돈다 —
// 두 번째 요청은 새로 시작하지 않고 돌고 있는 것에 붙는다.
type job struct {
	name        string
	done, total int
}

// jobProgressMsg 는 다음 조각을 받을 채널을 같이 들고 다닌다.
//
// 목록에서 찾지 않으므로 editor 복사본이 어긋나도 이어받는 고리가 끊기지 않는다.
// 확인창처럼 editor 를 아예 들고 있지 않은 화면도 고리를 이어줄 수 있다.
type jobProgressMsg struct {
	name string
	ch   <-chan jobProgress

	jobProgress
}

// jobDoneMsg 는 채널이 닫혔다는 것이다. 작업이 끝났다.
type jobDoneMsg struct {
	name string
}

// waitJob 은 채널에서 조각 하나를 받아 msg 로 바꾸는 Cmd 다.
//
// 받을 때마다 다시 발행해야 다음 조각이 온다. handleJob 이 그것을 한다.
func waitJob(name string, ch <-chan jobProgress) tea.Cmd {
	return func() tea.Msg {
		progress, ok := <-ch
		if !ok {
			return jobDoneMsg{name: name}
		}

		return jobProgressMsg{name: name, ch: ch, jobProgress: progress}
	}
}

// startJob 은 작업을 시작한다. 같은 이름이 이미 돌고 있으면 시작하지 않고 nil 을 준다.
//
// 채널이 아니라 채널을 만드는 함수를 받는다. 채널을 먼저 만들면 이미 돌고 있을 때
// 갈 곳 없는 goroutine 이 하나 뜬다.
func (e *editor) startJob(name string, start func() <-chan jobProgress) tea.Cmd {
	if e.jobRunning(name) {
		return nil
	}

	e.putJob(job{name: name})

	return waitJob(name, start())
}

// handleJob 은 mode 가 공유하는 작업 msg 처리다. 다음 조각을 받을 Cmd 를 준다.
//
// mode 마다 `case jobProgressMsg, jobDoneMsg:` 한 자리를 두고 여기로 넘긴다. 하는 일은 여기 하나로
// 모여 있고(ADR-0002 가 "늘어나면 공용 처리로 뺀다" 고 적어둔 자리다) mode 쪽에는 어떤 msg 를
// 받는지가 남는다. default 에 숨기면 그 mode 가 작업 msg 를 받는다는 것이 보이지 않는다.
//
// 결과가 무엇인지는 여기서 알지 못한다 — 이름으로 가르는 곳이 없다.
func (e *editor) handleJob(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case jobProgressMsg:
		e.putJob(job{name: msg.name, done: msg.done, total: msg.total})

		if msg.apply != nil {
			msg.apply(e)
		}

		return waitJob(msg.name, msg.ch)
	case jobDoneMsg:
		e.removeJob(msg.name)

		return nil
	default:
		return nil
	}
}

// jobRunning 은 그 이름의 작업이 돌고 있는지다.
func (e editor) jobRunning(name string) bool {
	for _, running := range e.jobs {
		if running.name == name {
			return true
		}
	}

	return false
}

// putJob 은 진행을 갱신한다. 목록에 없으면 맨 뒤에 붙는다 —
// 확인창을 거치며 목록이 어긋난 복사본으로 돌아왔어도 다음 조각 하나로 다시 맞는다.
//
// buffers 와 같은 이유로 목록을 새로 할당한다(closeTab 참고).
func (e *editor) putJob(next job) {
	jobs := make([]job, len(e.jobs), len(e.jobs)+1)
	copy(jobs, e.jobs)

	for i := range jobs {
		if jobs[i].name == next.name {
			jobs[i] = next
			e.jobs = jobs

			return
		}
	}

	e.jobs = append(jobs, next)
}

// removeJob 은 끝난 작업을 목록에서 뺀다.
func (e *editor) removeJob(name string) {
	jobs := make([]job, 0, len(e.jobs))
	for _, running := range e.jobs {
		if running.name == name {
			continue
		}

		jobs = append(jobs, running)
	}

	e.jobs = jobs
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

// drawBar 는 막대다. 전체 대비 얼마나 왔는지를 예순 단계로 나눈다.
func drawBar(done, total int) string {
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

// jobBar 는 맨 앞 작업의 막대다. 전체를 모르면 그리지 않는다 — 반쯤 찬 막대가 거짓말이 된다.
func (e editor) jobBar() string {
	if len(e.jobs) == 0 || e.jobs[0].total <= 0 {
		return ""
	}

	return drawBar(e.jobs[0].done, e.jobs[0].total)
}

// jobText 는 statusBar 에 붙는 진행 표시다. 도는 것이 없으면 빈 문자열이다.
//
// 목록 맨 앞, 곧 가장 먼저 시작한 것을 찍는다. 끝날 때까지 가리키는 것이 바뀌지 않아야
// 눈이 따라갈 수 있다. 나머지는 개수로만 알린다.
//
// bar 는 막대다. 오른쪽에 붙일 칸이 모자라면 부르는 쪽이 빈 문자열을 준다.
func (e editor) jobText(bar string) string {
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
