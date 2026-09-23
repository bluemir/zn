package core

import (
	"slices"
	"strings"
)

// 명령줄과 검색창에 친 것을 담고 위·아래로 꺼내 쓰는 자리다(ADR-0143).
//
// **담는 것과 훑는 것이 갈려 있다.** `history` 는 editor 에 세션 내내 남고,
// `historyBrowse` 는 줄을 치는 동안에만 있다가 mode 를 나가면 같이 사라진다.
// 명령줄과 검색창이 둘 다 이 한 벌을 쓴다. `inputLine` 이 여덟 곳의 입력을 한 벌로
// 모은 것과 같은 자리다(ADR-0076).

// historyLimit 은 한 갈래가 담는 최대 개수다. vim 의 `'history'` 기본값과 같다.
const historyLimit = 50

// history 는 한 갈래의 이력이다. 오래된 것이 앞, 최근에 친 것이 뒤다.
//
// 갈래마다 따로 둔다. `/` 를 치고 위를 눌렀을 때 `:w` 가 나오면 꺼내 쓸 수 없다(ADR-0143 §1).
type history struct {
	entries []string
}

// add 는 친 것을 맨 뒤에 담는다. 빈 줄은 담지 않는다.
//
// **같은 것을 다시 치면 앞의 것을 지우고 맨 뒤로 보낸다.** `:w` 를 열 번 쳐도 이력에
// 하나다. 연달아 친 것만 접으면 `:w` `:e a` `:w` 가 둘로 남아서 50 칸이 `:w` 로 채워진다.
//
// 결과는 보지 않는다. 오타로 틀린 명령도 담긴다. 고쳐서 다시 치려면 그것이 이력에 있어야 한다.
func (h *history) add(text string) {
	if text == "" {
		return
	}

	h.entries = slices.DeleteFunc(h.entries, func(entry string) bool { return entry == text })
	h.entries = append(h.entries, text)

	if len(h.entries) > historyLimit {
		h.entries = slices.Clone(h.entries[len(h.entries)-historyLimit:])
	}
}

// match 는 prefix 로 시작하는 것만 차례대로 준다. prefix 가 비면 전부다.
//
// 「거르는 경우」와 「전부 보는 경우」가 따로 있는 것이 아니라 같은 규칙의 양끝이다
// (ADR-0143 §2).
func (h history) match(prefix string) []string {
	if prefix == "" {
		return h.entries
	}

	matched := make([]string, 0, len(h.entries))
	for _, entry := range h.entries {
		if strings.HasPrefix(entry, prefix) {
			matched = append(matched, entry)
		}
	}

	return matched
}

// historyBrowse 는 이력을 훑는 중인 상태다. 명령줄과 검색창이 저마다 하나씩 든다.
//
// **훑기 시작할 때의 글을 붙들고 있다.** 그것이 거르는 접두이면서, 맨 아래까지 내려왔을 때
// 돌려줄 글이다. 둘이 같은 값이라 따로 두지 않는다.
type historyBrowse struct {
	// typed 는 훑기 전에 치고 있던 글이다. 거르는 접두이기도 하다.
	typed string

	// matches 는 typed 로 거른 것이다. 오래된 것이 앞이다.
	matches []string

	// at 은 matches 안의 자리다. `len(matches)` 면 이력이 아니라 typed 를 보이는 중이다.
	at int

	// browsing 은 훑는 중인지다. 아니면 다음 위·아래가 지금 글로 새로 시작한다.
	browsing bool
}

// move 는 훑는 자리를 delta 만큼 옮기고 그 자리의 글을 준다. 양끝에서 멈춘다.
//
// 훑는 중이 아니면 typed 로 거른 목록을 새로 만들고 그 끝에서 시작한다. 그래서 첫 위는
// 가장 최근 것이고, 거기서 아래로 끝까지 내려오면 치던 글이 돌아온다.
//
// 꺼낼 것이 없으면 두 번째 값이 false 다. 부르는 쪽은 아무 일도 하지 않는다.
func (b *historyBrowse) move(h history, typed string, delta int) (string, bool) {
	if !b.browsing {
		b.typed = typed
		b.matches = h.match(typed)
		b.at = len(b.matches)
		b.browsing = true
	}

	if len(b.matches) == 0 {
		return "", false
	}

	b.at = min(max(b.at+delta, 0), len(b.matches))

	if b.at == len(b.matches) {
		return b.typed, true
	}

	return b.matches[b.at], true
}

// historyDelta 는 키 이름을 훑는 방향으로 바꾼다. 위가 더 오래된 쪽이다.
//
// 키 이름을 그대로 받는 것은 `inputLine.move` 와 같은 까닭이다 — 부르는 두 곳이 다
// `msg.String()` 으로 갈리는 switch 안이라 거기서 두 case 를 한 줄로 묶을 수 있다.
func historyDelta(key string) int {
	if key == "up" {
		return -1
	}

	return 1
}

// stop 은 훑기를 놓는다. 글을 고치면 거를 접두가 달라지므로 다음 위·아래는 새로 시작한다.
//
// 커서만 옮기는 것은 부르지 않는다. 글이 그대로라 훑던 자리가 살아 있어야 한다(ADR-0143 §2).
func (b *historyBrowse) stop() {
	*b = historyBrowse{}
}
