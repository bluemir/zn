package core

import "github.com/cockroachdb/errors"

// noteOutsideChange 는 보고 있는 파일을 맞춰 보고 그 결과를 buffer 에 적는다.
// 알릴 문구를 돌려주고, 알릴 것이 없으면 빈 문자열이다.
//
// 깨끗했던 것이 달라지는 순간에만 알린다. 이미 `[!]` 가 붙어 있는데 또 알리면, 밖에서 계속
// 바뀌는 파일을 띄워둔 동안 statusBar 아래 줄이 그 알림에 계속 덮인다. 알림은 발견을 알리는
// 것이고 지금 상태를 들고 있는 것은 마커다 (ADR-0031).
//
// 반대로 밖의 변경이 되돌아가 다시 같아지면 마커도 조용히 사라진다. 알리지 않는다 —
// 볼 것이 없어졌다는 알림은 읽는 사람이 할 일이 없다.
func (e *editor) noteOutsideChange() string {
	buf := e.activeBuffer()

	change, err := buf.checkOutside()
	if err != nil {
		// 읽지 못한 것은 상태가 아니라 사고다. 마커로 남기지 않고 그 자리에서만 알린다.
		return errors.Cause(err).Error()
	}

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
