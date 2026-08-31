package core

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// 「타이핑이 멎었다」를 알리는 tick 이다.
//
// 두 가지가 이 tick 에 매달려 있다. 언어 서버와 맞추는 것(ADR-0051) 과 git 줄 마커를 다시
// 내는 것(ADR-0094) 이다. 둘 다 키마다 하기에는 값이 크고, 손이 멈춘 다음에 하면 되는 일이다.
//
// **예약 상태를 늘리지 않았다.** 편집 뒤에 미뤄 두는 일이 하나 더 생겼을 때 tick 을 하나 더
// 두면 같은 250ms 를 재는 타이머가 둘이 된다. 이름을 lsp 에서 편집으로 넓힌 것이 그 답이다
// (ADR-0043 이 예약을 하나로 묶어 둔 것과 같은 자리다).

// editIdleDelay 는 마지막 키에서 이만큼 조용해지면 미뤄 둔 것을 한다.
//
// 타이핑마다 보내지 않는 이유는 보내는 값이 아까워서가 아니라(잰 값으로 34.6KB 파일이
// 750µs 다) 화면을 그리는 goroutine 이 남의 프로세스와 발을 맞출 이유가 없어서다.
// 정확성은 이 시간에 매달려 있지 않다 — 묻는 순간에 전문으로 한 번 맞춘다(ADR-0051).
const editIdleDelay = 250 * time.Millisecond

// editTickMsg 는 타이핑이 멎었다는 것이다.
type editTickMsg time.Time

// scheduleEditTick 은 그때를 예약한다. 이미 걸어둔 것이 있으면 그것을 쓴다.
//
// git 갱신과 같은 손이다(ADR-0043). 걸어둔 것이 하나라는 규칙이 타이핑을 모아 주는 자리이기도
// 하다 — 키를 열 번 쳐도 예약은 하나이고, 그 하나가 250ms 뒤에 지금 상태를 한 번 본다.
//
// 할 일이 없으면 걸지 않는다. 서버도 없고 견줄 HEAD 원본도 없으면 250ms 뒤에 할 것이 없다.
func (e *editor) scheduleEditTick() tea.Cmd {
	if e.editTickScheduled {
		return nil
	}

	if !e.anyServerRunning() && !e.hasGitBase() {
		return nil
	}

	e.editTickScheduled = true

	return tea.Tick(editIdleDelay, func(t time.Time) tea.Msg {
		return editTickMsg(t)
	})
}

// hasGitBase 는 지금 보고 있는 파일에 견줄 HEAD 원본이 있는지다.
func (e editor) hasGitBase() bool {
	return e.hasTab() && len(e.buffers[e.active].gitBase) > 0
}

// refreshActiveGitLines 는 보고 있는 파일의 줄 마커를 다시 낸다.
//
// **보고 있는 것 하나만 한다.** 다른 tab 의 내용은 편집으로 바뀌지 않고, 통째로 갈리는
// 자리(Reload) 는 원본까지 같이 비우므로 다음 git 갱신이 그쪽을 맡는다(applyGitSnapshot).
func (e *editor) refreshActiveGitLines() {
	if !e.hasTab() {
		return
	}

	e.activeBuffer().refreshGitLines()
}
