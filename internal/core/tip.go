package core

import (
	"strings"

	"github.com/bluemir/zn/internal/assets"
)

// tipGap 은 아래 줄 왼쪽 글자와 tip 사이를 띄우는 칸이다.
//
// 한 칸이면 `1:1  (550 줄)` 에 붙은 말처럼 읽힌다. 위 줄이 git 앞을 두 칸 띄우는 것과
// 같은 이유다(render-status-bar.go 의 renderWithStatus).
const tipGap = 2

// renderWithTip 은 statusBar 아래 줄 오른쪽 끝을 채운다.
//
// **그 칸의 주인이 이 함수 하나다.** showcmd 가 먼저고, 보여줄 showcmd 가 없을 때만 tip 이다 —
// 접두 키를 치는 동안에는 그 키가 먹혔는지가 tip 보다 급하다(ADR-0061).
//
// 알림이 떠 있는 동안에는 tip 을 붙이지 않는다. 짧은 알림(`저장했습니다`) 은 자리가 남아서,
// 막지 않으면 알림과 배움이 한 줄에 같이 뜬다. 알림이 그 줄을 쓰고 다음 키에 걷히면 그때
// 새 문장이 드러나는 것이 이 기능의 리듬이다(notice.go 의 record).
//
// **색을 입히지 않는다.** 아래 줄은 truncateToWidth 를 지나는데 그것이 ANSI 를 모르는 byte
// 계산이라, 흐린 글씨로 만들면 escape 가 폭으로 세어져 줄이 어긋난다. 위 줄이 색을 쓸 수
// 있는 것은 다 지은 줄을 통째로 감싸기 때문이다.
func (e editor) renderWithTip(bottom, showcmd string) string {
	if showcmd != "" {
		return e.renderWithShowcmd(bottom, showcmd)
	}

	if e.notice != "" {
		return bottom
	}

	room := e.textWidth() - screenWidthOf(bottom)

	tip := fitTip(assets.Tips, e.tipIndex, room-tipGap)
	if tip == "" {
		return bottom
	}

	return bottom + strings.Repeat(" ", room-screenWidthOf(tip)) + tip
}

// fitTip 은 from 자리부터 한 바퀴 훑어 room 칸에 들어가는 첫 문장이다. 없으면 빈 문자열이다.
//
// **자르지 않고 고른다.** 뒤가 `…` 로 끊긴 tip 은 무엇을 누르라는 것인지 알려주지 못하면서
// 자리는 다 차지한다. 긴 문장은 넓은 화면에서 만나면 된다.
//
// 목록을 인자로 받는 것은 검사가 자기 짧은 목록으로 고르기 규칙만 보게 하려는 것이고,
// 문장 자체는 core 밖에 살기 때문이다(internal/assets 의 Tips). 데이터와 규칙을 가른 것이
// truncateToWidth·noticeText 와 같다 — 문장을 고치려고 편집기 코드를 열지 않는다.
func fitTip(list []string, from, room int) string {
	for i := range list {
		if tip := list[(from+i)%len(list)]; screenWidthOf(tip) <= room {
			return tip
		}
	}

	return ""
}
