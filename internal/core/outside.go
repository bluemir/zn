package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// fileCooldown 은 파일 검사가 끝난 뒤 다음 검사까지 쉬는 시간이다.
//
// git 갱신과 같은 5 초이고 재는 방식도 같다 — 주기가 아니라 cooldown 이다(ADR-0043).
const fileCooldown = 5 * time.Second

// fileJobName 은 파일 검사 작업의 이름이자 신원이다. 같은 이름은 한 번에 하나만 돈다(job.go).
const fileJobName = "파일 검사"

// fileTickMsg 는 보고 있는 파일을 다시 맞춰 볼 때가 되었다는 것이다.
//
// git tick 과 나눠 두었다. cooldown 을 따로 정할 수 있고, 무엇을 위한 tick 인지가 이름에 남는다.
// 이 msg 를 받지 않는 mode 가 있으면 그 mode 에서 고리가 끊기므로, 모든 mode 가 공용
// 처리(handleJob) 로 넘긴다 (ADR-0038, ADR-0044).
type fileTickMsg time.Time

// scheduleFileTick 은 cooldown 을 재고 다음 검사를 예약한다.
//
// 예약된 tick 은 늘 하나다. 이유는 git 쪽과 같다 — 포커스 복귀로 검사가 한 번 더 도는 일이
// 있고, 그것이 끝날 때도 이 자리를 지난다(ADR-0043).
func (e *editor) scheduleFileTick() tea.Cmd {
	if e.fileTickScheduled {
		return nil
	}

	e.fileTickScheduled = true

	return tea.Tick(fileCooldown, func(t time.Time) tea.Msg {
		return fileTickMsg(t)
	})
}

// outsideResult 는 검사가 알아낸 것이다. 값이 드는 일은 전부 여기까지 끝나 있다.
type outsideResult struct {
	change outsideChange

	// size·mtime 은 검사가 본 파일의 크기와 mtime 이다. 다음 검사의 앞잡이가 된다.
	size  int64
	mtime time.Time

	// next 는 갈아끼울 내용이다. 내용이 달라졌을 때만 채운다 — 만드는 것이 파일 크기만큼
	// 드는 일이라 이 자리(백그라운드) 에서 해 둔다.
	next *viewport

	err error
}

// outsideTarget 은 맞춰 볼 파일 하나와 그때의 기준이다.
//
// 시작하는 자리에서 떠 둔다. 결과가 돌아올 때는 tab 이 닫히거나 바뀌어 있을 수 있어서
// 검사하는 쪽이 buffer 를 들고 있으면 안 된다(ADR-0044).
type outsideTarget struct {
	path  string
	seen  []byte
	size  int64
	mtime time.Time
}

// outsideTargets 는 열려 있는 buffer 전부를 맞춰 볼 거리로 만든다.
//
// **활성 buffer 하나가 아니라 전부다.** 보고 있지 않은 tab 의 파일이 밖에서 바뀌면 그
// 낡은 내용이 tab 을 옮길 때까지 남고, 언어 서버에는 그 낡은 overlay 가 계속 실려 간다 —
// ADR-0092 가 「알리지 않는 것보다 나쁜 자리」로 적어 둔 것이 그것이다(ADR-0093).
//
// 값 걱정은 앞잡이가 답한다. 유휴 상태의 검사 하나는 Lstat 하나라(ADR-0044) tab 이 열
// 개여도 tick 마다 Lstat 열 번이다.
func (e *editor) outsideTargets() []outsideTarget {
	targets := make([]outsideTarget, 0, len(e.buffers))

	for i := range e.buffers {
		targets = append(targets, outsideTargetOf(&e.buffers[i]))
	}

	return targets
}

// outsideTargetOf 는 buffer 하나에서 기준을 뜬다.
func outsideTargetOf(buf *viewport) outsideTarget {
	return outsideTarget{
		path:  buf.path,
		seen:  buf.disk.hash,
		size:  buf.disk.size,
		mtime: buf.disk.mtime,
	}
}

// startOutsideCheck 는 열려 있는 파일들을 백그라운드에서 맞춰 보는 작업을 시작한다.
//
// 읽기와 해시와 새 Buffer 만들기가 모두 이 작업 안에서 끝난다. `Update` 에는 아무 I/O 도
// 남지 않는다 — 예전에는 이것이 `Update` 안에서 동기로 돌아, 100MB 파일에서 5 초마다 편집기가
// 42ms(바뀐 것을 만나면 122ms) 멈췄다(ADR-0044).
//
// **인자 없는 작업 하나가 전부를 본다.** 파일마다 따로 시작하면 그것들이 「이미 도는 중」이라
// 건너뛰어질 때 `jobDoneMsg` 가 하나도 오지 않고 cooldown 고리가 조용히 멈춘다. tab 이 없어도
// 작업이 도는 것과 같은 까닭이다 — 시작하지 않으면 끝나지도 않는다(ADR-0044, ADR-0064).
func (e *editor) startOutsideCheck() tea.Cmd {
	targets := e.outsideTargets()

	return e.startJob(fileJobName, nil, func(ctx context.Context) <-chan jobProgress {
		return checkOutsideFiles(ctx, targets)
	})
}

// startOutsideCheckFor 는 파일 하나만 맞춰 본다. 감시기가 부르는 자리다(watch.go).
//
// **이름은 주기 검사와 같고 인자로 갈린다**(ADR-0075). 같은 이름이라 이 작업이 끝나는 것도
// cooldown 을 잇는 자리를 그대로 지나고, 인자가 달라서 주기 검사와 서로를 막지 않는다.
// 같은 파일에 이벤트가 잇달아 와도 신원이 같아 두 번 돌지 않는다.
func (e *editor) startOutsideCheckFor(path string) tea.Cmd {
	buf := e.bufferByPath(path)
	if buf == nil {
		return nil
	}

	target := outsideTargetOf(buf)

	return e.startJob(fileJobName, []string{path}, func(ctx context.Context) <-chan jobProgress {
		return checkOutsideFiles(ctx, []outsideTarget{target})
	})
}

// checkOutsideFiles 는 거리들을 차례로 맞춰 보고 하나씩 알린다. 작업 goroutine 이 도는 자리다.
//
// 거리가 없으면 아무것도 보내지 않고 채널을 닫는다. 그것으로 `jobDoneMsg` 가 오므로
// tab 이 하나도 없어도 cooldown 고리는 이어진다.
func checkOutsideFiles(ctx context.Context, targets []outsideTarget) <-chan jobProgress {
	ch := make(chan jobProgress)

	go func() {
		defer close(ch)

		for i, target := range targets {
			result := checkOutsideFile(target.path, target.seen, target.size, target.mtime)

			if err := ctx.Err(); err != nil {
				ch <- jobProgress{err: err}

				return
			}

			progress := jobProgress{
				done:    i + 1,
				total:   len(targets),
				summary: outsideSummary(result),
				apply:   func(e *editor) { e.applyOutsideResult(target.path, target.seen, result) },
			}

			// 알아낸 것이 없는 파일은 요약을 덮지 않는다. 여럿을 볼 때 마지막 하나가
			// 「그대로」라고 해서 앞에서 찾은 것이 `:jobs` 에서 지워지면 안 된다
			// (updateJob 은 빈 요약을 흘려보낸다). 첫 조각만 바닥을 깔아 둔다.
			if result.change == outsideSame && i > 0 {
				progress.summary = ""
			}

			select {
			case ch <- progress:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// checkOutsideFile 은 파일 하나를 맞춰 본다. 작업 goroutine 이 부르는 자리다.
//
// 이름 없는 buffer 는 맞춰 볼 파일이 없으므로 그대로인 것으로 본다. 그래도 작업은 돈다 —
// 시작하지 않으면 끝나지도 않아서 cooldown 고리가 그 자리에서 멈춘다(ADR-0044).
func checkOutsideFile(path string, seen []byte, size int64, mtime time.Time) outsideResult {
	if path == "" {
		return outsideResult{change: outsideSame}
	}

	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// 열 때도 없던 파일이면 달라진 것이 없다.
		if seen == nil {
			return outsideResult{change: outsideSame}
		}

		return outsideResult{change: outsideRemoved}
	case err != nil:
		return outsideResult{err: errors.Wrapf(err, "cannot read %s", path)}
	}

	// 앞잡이가 맞으면 읽지 않는다. 유휴 상태에서 대부분이 이 길로 끝난다.
	//
	// seen 이 nil 인 것은 열 때 파일이 없었다는 뜻이라 견줄 기준이 아직 없다. 그때 앞잡이만
	// 보고 「그대로」라고 답하면, 없던 파일이 생긴 것을 한 번 알린 뒤 다음 검사에서 마커가
	// 조용히 지워진다 — buffer 는 여전히 비어 있는데 어긋난 것이 없다고 말하는 셈이다.
	if seen != nil && !mtime.IsZero() && info.Size() == size && info.ModTime().Equal(mtime) {
		return outsideResult{change: outsideSame, size: size, mtime: mtime}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return outsideResult{err: errors.Wrapf(err, "cannot read %s", path)}
	}

	result := outsideResult{size: info.Size(), mtime: info.ModTime()}

	if seen == nil {
		result.change = outsideCreated

		return result
	}

	sum := sha256.Sum256(data)
	if bytes.Equal(sum[:], seen) {
		result.change = outsideSame

		return result
	}

	result.change = outsideModified

	// 갈아끼울 내용을 여기서 만들어 둔다. 줄 나누기와 해시가 파일 크기만큼 드는 일이라
	// `Update` 로 넘기면 그만큼 화면이 멈춘다.
	next := newBuffer(path, data)
	next.disk.size = result.size
	next.disk.mtime = result.mtime
	result.next = &next

	return result
}

// applyOutsideResult 는 알아낸 것을 buffer 에 넣는다. `Update` 안에서 불린다(job.go).
//
// 잃을 것이 없으면 그 자리에서 갈아끼운다 — 잃을 것이 없다는 것은 `dirty` 가 아니라는 뜻이다.
// 손에 저장하지 않은 변경이 있으면 읽지 않고 마커만 붙인다. 그때 무엇을 남길지는 사람이 정할
// 일이라 `:e` 의 확인창이 그대로 맡는다 (ADR-0016, ADR-0038).
//
// 내용이 달라진 경우만 자동으로 읽는다. 없던 파일이 생긴 것은 `:tabnew` 로 새로 쓰려던 자리일
// 수 있어서 손으로 정한다.
//
// mode 는 보지 않는다. 이 함수가 불릴 때 어느 mode 인지 알 수 없고(ADR-0002 에서 mode 는 model
// 타입이다), 알 필요도 없다 — 조건은 `dirty` 하나다(ADR-0044).
func (e *editor) applyOutsideResult(path string, seen []byte, result outsideResult) {
	if result.err != nil {
		// 읽지 못한 것은 상태가 아니라 사고다. 마커로 남기지 않고 그 자리에서만 알린다.
		e.notifyError(result.err)

		return
	}

	buf := e.bufferByPath(path)

	// 검사하는 동안 그 tab 이 닫혔거나, 저장·다시 읽기로 기준이 달라졌다. 지금 넣으면
	// 낡은 것을 넣는 셈이라 물러난다 — 다음 tick 이 새 기준으로 다시 본다.
	if buf == nil || !bytes.Equal(buf.disk.hash, seen) {
		return
	}

	// 앞잡이는 판정과 무관하게 갱신한다. 다음 검사가 읽지 않고 끝나는 것이 이 값이다.
	buf.disk.size = result.size
	buf.disk.mtime = result.mtime

	if result.change != outsideModified || buf.dirty || result.next == nil {
		if message := markOutsideChange(buf, result.change); message != "" {
			e.notify(message)
		}

		return
	}

	*buf = buf.adopt(*result.next)

	// 커서 칸은 유지하지만 그 자리가 새 내용에서는 줄 끝 다음일 수 있다. 팔레트의
	// 「파일 다시 읽기」와 같은 뒷마무리다(palette.go).
	//
	// 보고 있는 tab 이 아니면 화면을 건드릴 것이 없다. 그 tab 으로 옮겨갈 때 scrollToCursor 가 돈다.
	if buf == e.activeBuffer() {
		buf.clampToNormal(e.contentWidth())
		e.scrollToCursor()
	}

	e.notify("파일이 밖에서 바뀌어 다시 읽었습니다")
}

// outsideSummary 는 `:jobs` 에 남기는 한 줄이다. 무엇을 보고 무엇을 알아냈는지가 보인다.
func outsideSummary(result outsideResult) string {
	switch {
	case result.err != nil:
		return "읽지 못함"
	case result.change == outsideModified:
		return "밖에서 바뀜"
	case result.change == outsideRemoved:
		return "밖에서 사라짐"
	case result.change == outsideCreated:
		return "밖에서 생김"
	}

	return "그대로"
}

// markOutsideChange 는 판정을 buffer 에 적고 알릴 문구를 준다.
//
// 깨끗했던 것이 달라지는 순간에만 알린다. 이미 `[!]` 가 붙어 있는데 또 알리면, 밖에서 계속
// 바뀌는 파일을 띄워둔 동안 statusBar 아래 줄이 그 알림에 계속 덮인다. 알림은 발견을 알리는
// 것이고 지금 상태를 들고 있는 것은 마커다 (ADR-0031).
//
// 반대로 밖의 변경이 되돌아가 다시 같아지면 마커도 조용히 사라진다. 알리지 않는다 —
// 볼 것이 없어졌다는 알림은 읽는 사람이 할 일이 없다.
//
// 활성 buffer 가 아니라 buffer 를 받는다. 검사가 백그라운드로 내려가서, 결과가 돌아올 때는
// 보고 있는 tab 이 검사한 tab 이 아닐 수 있다(ADR-0044).
func markOutsideChange(buf *viewport, change outsideChange) string {
	was := buf.disk.outside
	buf.disk.outside = change

	if change == outsideSame || was != outsideSame {
		return ""
	}

	return outsideChangeMessage(change)
}

// outsideChangeMessage 는 바깥 변경을 발견했을 때 알릴 문구다.
//
// 저장할 때의 문구(checkNotChangedOutside) 와 판정은 같고 다음 걸음이 다르다. 여기서는 아직
// 아무것도 쓰려 하지 않았으므로 덮어쓰는 길이 아니라 가져오는 길을 알린다. 사라진 파일은
// 가져올 것이 없어서 사실만 알린다 — 손에 든 것이 마지막 사본이다 (ADR-0016, ADR-0023).
func outsideChangeMessage(change outsideChange) string {
	switch change {
	case outsideRemoved:
		return "파일이 밖에서 사라졌습니다"
	case outsideCreated:
		return "파일이 밖에서 새로 생겼습니다. 다시 읽으려면 `:e` 입니다"
	case outsideModified:
		return "파일이 밖에서 바뀌었습니다. 다시 읽으려면 `:e` 입니다"
	}

	return ""
}

// outsideChange 는 파일이 읽은(또는 마지막으로 쓴) 시점과 어떻게 달라졌는지다.
//
// 알릴 문구는 부르는 쪽이 만든다. 같은 사실에 붙는 다음 걸음이 자리마다 다르다 —
// 저장이 막힌 자리는 빠져나가는 길(`:w!`) 을 알려야 하고, 셸에서 돌아온 자리는
// 가져오는 길(`:e`) 을 알린다 (ADR-0015, ADR-0023).
type outsideChange int

const (
	outsideSame     outsideChange = iota // 읽은 시점과 같다
	outsideModified                      // 내용이 달라졌다
	outsideCreated                       // 없던 파일이 생겼다
	outsideRemoved                       // 있던 파일이 사라졌다
)
