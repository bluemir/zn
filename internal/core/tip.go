package core

import (
	"math/rand/v2"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/assets"

	"github.com/bluemir/zn/internal/textarea"
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
// **흐린 글씨다(styleTip).** 눈에 덜 띄어야 tip 이지, 커서 위치와 같은 밝기로 서면 읽던 것을
// 끊는다. 그러려고 아래 줄을 자르는 truncateToWidth 가 escape 를 지나치게 되었다 —
// 색이 아니라 그 byte 계산이 이 자리를 오래 막고 있었다(ADR-0061 §6).
func (e editor) renderWithTip(bottom, showcmd string) string {
	if showcmd != "" {
		return e.renderWithShowcmd(bottom, showcmd)
	}

	tip := e.pickTip(bottom, showcmd)
	if tip == "" {
		return bottom
	}

	room := e.textWidth() - textarea.WidthOf(bottom)

	return bottom + strings.Repeat(" ", room-textarea.WidthOf(tip)) + styleTip.Render(tip)
}

// pickTip 은 지금 아래 줄에 설 문장이다. 설 자리가 없으면 빈 문자열이다.
//
// **그리는 쪽과 누른 자리를 재는 쪽이 이 하나를 지난다.** 고르는 규칙을 두 벌 두면 눌렀을 때
// 화면에 없는 문장이 열린다 — tabline 이 그리면서 칸 범위를 같이 내주는 것과 같은 자리다
// (ADR-0012, ADR-0147).
func (e editor) pickTip(bottom, showcmd string) string {
	// 접두 키를 치는 동안에는 그 칸이 showcmd 것이고, 알림이 떠 있는 동안에는 알림 것이다.
	if showcmd != "" || e.notice != "" {
		return ""
	}

	return fitTip(assets.Tips, e.tipIndex, e.textWidth()-textarea.WidthOf(bottom)-tipGap)
}

// tipSpan 은 그 문장이 차지하는 화면 칸이다. 서 있지 않으면 빈 칸이라 어느 x 도 들지 않는다.
//
// 오른쪽 끝에 붙으므로 끝이 편집 영역의 오른쪽 끝이다. sidebar 를 더하는 것은 아래 줄이
// 화면 끝에서 끝까지 이어져도 이 글은 편집 영역 자리에서 시작하기 때문이다.
func (e editor) tipSpan(bottom, showcmd string) [2]int {
	tip := e.pickTip(bottom, showcmd)
	if tip == "" {
		return [2]int{}
	}

	end := e.sidebarLeft() + e.textWidth()

	return [2]int{end - textarea.WidthOf(tip), end}
}

// clickTip 은 아래 줄의 tip 을 누른 것이면 그 문장에서 목록을 연다. 아니면 nil 이다.
//
// **`regionAt` 에 영역을 더하지 않았다.** 그 함수는 화면을 크게 가르는 자리이고 tip 은 아래
// 줄 오른쪽에 있다 없다 하는 글이라, 영역으로 세우면 「지금 tip 이 서 있는가」를 거기서 또
// 물어야 한다. 트리 메뉴가 자기 상자를 직접 재는 것과 같은 손이다(ADR-0146, ADR-0147).
//
// bottom·showcmd 는 그 화면이 아래 줄에 그린 것 그대로다. 그리는 자리와 같은 값을 넘겨야
// 같은 문장이 나온다.
func (e *editor) clickTip(mouse tea.Mouse, bottom, showcmd string) (tea.Model, tea.Cmd) {
	if mouse.Y != e.height-1 || !inSpan(e.tipSpan(bottom, showcmd), mouse.X) {
		return nil, nil
	}

	// 누른 그 문장에서 목록이 열린다. 스쳐 지나가는 것을 붙잡으려고 누른 것이라,
	// 맨 위에서 열리면 방금 본 문장을 목록에서 다시 찾아야 한다.
	at := slices.Index(assets.Tips, e.pickTip(bottom, showcmd))
	if at < 0 {
		return nil, nil
	}

	return tipsMode(e, at)
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
		if tip := list[(from+i)%len(list)]; textarea.WidthOf(tip) <= room {
			return tip
		}
	}

	return ""
}

// tipStride 는 다음 문장으로 건너뛸 최대 칸이다.
//
// 1 이면 목록 순서 그대로 돌고, 목록 길이만큼이면 아무 데나 뛰는 것과 같다. 스물은 그 사이다 —
// 순서가 눈에 띄게 흩어지면서, 뛰는 폭이 목록보다 훨씬 작아서 아주 오래 안 나오는 문장이
// 생기지 않는다.
const tipStride = 20

// nextTip 은 다음 문장으로 넘어간다. 알림이 날 때마다 한 번이다(notice.go 의 record).
//
// **한 칸씩이 아니라 한두 자리에서 스무 자리까지 건너뛴다.** 순서대로 돌면 편집기를 열어 둔
// 동안 목록의 차례가 그대로 드러나서 다음에 무엇이 올지 보인다. 무작위로 흩는 쪽인데,
// 목록을 한 바퀴 안에 다 보여준다는 약속은 하지 않는다 — tip 은 보조 수단이라 어떤 문장이
// 두 번 나오고 어떤 것이 늦게 나오는 것이 값으로 치이지 않는다.
//
// **적어도 한 칸은 간다.** 그래서 같은 문장이 잇달아 두 번 서지 않는다. 씨앗을 따로 심지
// 않는 것은 math/rand/v2 의 전역 난수가 프로세스마다 저절로 갈리기 때문이다.
func (e *editor) nextTip() {
	e.tipIndex += 1 + rand.IntN(tipStride)
}
