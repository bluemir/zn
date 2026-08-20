package core

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// fileInterval 은 보고 있는 파일을 다시 맞춰 보는 주기다.
//
// git 표시와 같은 5 초다(ADR-0030). 다른 창에서 고치고 편집기로 눈을 돌리는 사이에 값이 맞는
// 정도이면서, 유휴 상태에 파일 하나를 분당 열두 번 읽는 자리다 (ADR-0038).
const fileInterval = 5 * time.Second

// fileTickMsg 는 보고 있는 파일을 다시 맞춰 볼 때가 되었다는 것이다.
//
// git tick 과 나눠 두었다. 주기를 따로 정할 수 있고, 무엇을 위한 tick 인지가 이름에 남는다.
// 대신 이 msg 를 받지 않는 mode 가 있으면 그 mode 에서 고리가 끊기므로, 공용 처리(handleJob)
// 가 다시 예약하는 것을 모든 mode 가 지나가게 두었다 (ADR-0038).
type fileTickMsg time.Time

// tickFile 은 다음 tick 을 예약한다.
//
// 예약은 tick 을 받은 자리에서 한 번씩만 한다. 보는 mode(normal·트리) 는 스스로 예약하고
// 나머지 mode 는 handleJob 이 대신 예약한다 — 한 msg 를 한 자리가 받으므로 고리가 갈라지지
// 않는다. 여러 곳에서 걸면 주기가 반으로, 다시 반으로 줄어든다(git tick 과 같다).
func tickFile() tea.Cmd {
	return tea.Tick(fileInterval, func(t time.Time) tea.Msg {
		return fileTickMsg(t)
	})
}

// noteOutsideChange 는 보고 있는 파일을 맞춰 보고 그 결과를 buffer 에 적는다.
// 알릴 문구를 돌려주고, 알릴 것이 없으면 빈 문자열이다.
//
// 적기만 하고 가져오지는 않는다. 가져오는 자리는 reloadOutsideChange 다 (ADR-0038).
func (e *editor) noteOutsideChange() string {
	change, err := e.activeBuffer().checkOutside()
	if err != nil {
		// 읽지 못한 것은 상태가 아니라 사고다. 마커로 남기지 않고 그 자리에서만 알린다.
		return errors.Cause(err).Error()
	}

	return e.markOutsideChange(change)
}

// reloadOutsideChange 는 보고 있는 파일을 맞춰 보고, 잃을 것이 없으면 그 자리에서 다시 읽는다.
// 알릴 문구를 돌려주고, 알릴 것이 없으면 빈 문자열이다 (ADR-0038).
//
// 잃을 것이 없다는 것은 `dirty` 가 아니라는 뜻이다. 손에 저장하지 않은 변경이 있으면 읽지 않고
// 마커만 붙인다 — 그때 무엇을 남길지는 사람이 정할 일이라 `:e` 의 확인창이 그대로 맡는다
// (ADR-0016).
//
// 내용이 달라진 경우(outsideModified) 만 자동으로 읽는다. 없던 파일이 생긴 것은 `:tabnew` 로
// 새로 쓰려던 자리일 수 있어서 손으로 정한다.
func (e *editor) reloadOutsideChange() string {
	buf := e.activeBuffer()

	change, err := buf.checkOutside()
	if err != nil {
		return errors.Cause(err).Error()
	}

	if change != outsideModified || buf.dirty {
		return e.markOutsideChange(change)
	}

	if err := buf.Reload(); err != nil {
		return errors.Cause(err).Error()
	}

	// 커서 칸은 유지하지만 그 자리가 새 내용에서는 줄 끝 다음일 수 있다. 팔레트의
	// 「파일 다시 읽기」와 같은 뒷마무리다(palette.go).
	buf.clampToNormal(e.contentWidth())
	e.scrollToCursor()

	// 읽고 나면 어긋난 것이 없다. Reload 가 buffer 를 갈아끼우므로 마커도 같이 사라진다.
	return "파일이 밖에서 바뀌어 다시 읽었습니다"
}

// markOutsideChange 는 판정을 buffer 에 적고 알릴 문구를 준다.
//
// 깨끗했던 것이 달라지는 순간에만 알린다. 이미 `[!]` 가 붙어 있는데 또 알리면, 밖에서 계속
// 바뀌는 파일을 띄워둔 동안 statusBar 아래 줄이 그 알림에 계속 덮인다. 알림은 발견을 알리는
// 것이고 지금 상태를 들고 있는 것은 마커다 (ADR-0031).
//
// 반대로 밖의 변경이 되돌아가 다시 같아지면 마커도 조용히 사라진다. 알리지 않는다 —
// 볼 것이 없어졌다는 알림은 읽는 사람이 할 일이 없다.
func (e *editor) markOutsideChange(change outsideChange) string {
	buf := e.activeBuffer()

	was := buf.outside
	buf.outside = change

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
