package textarea

import (
	"regexp"

	"github.com/bluemir/zn/internal/scheme"
)

// buffer 안에서 패턴을 찾는 것들이다. 패턴을 짓고 화면에 칠하는 것은 search.go 에 있다 (ADR-0042).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-search.go 다 (ADR-0121).

// isWordClass 는 `*` 가 잡는 부류인지다. 공백과 문장부호 위에서는 단어를 찾지 않는다.
func isWordClass(class charClass) bool {
	return class == classWord || class == classCJK
}

// Find 는 from 에서 방향대로 가장 가까운 매칭을 찾는다.
// 파일 끝에 닿으면 반대쪽 끝에서 이어 보고, 한 바퀴를 다 돌면 ok 가 false 다.
//
// **방향은 인자로 받는다.** 부르는 두 자리가 다 그것을 변수로 들고 있다 — `/` `?` 는
// viewEditorSearch 의 필드에서, `n` `N` 은 담아 둔 searchState 에서 온다. 밖으로
// FindForward·FindBackward 둘을 내면 갈래를 여기서 지우는 대신 부르는 두 자리에 심는다.
func (buf Buffer) Find(pattern *regexp.Regexp, direction SearchDirection, from scheme.Cursor) (SearchResult, bool) {
	if direction == SearchBackward {
		return buf.findBackward(pattern, from)
	}

	return buf.findForward(pattern, from)
}

// findForward 는 커서 다음 자리부터 아래로 찾는다.
//
// 줄을 한 바퀴 돌되 시작 줄은 두 번 본다. 처음에는 커서 뒤쪽만, 감싼 뒤에는 커서 앞쪽만이다.
// 그래야 시작 줄 안에서 커서보다 앞에 있는 매칭도 빠지지 않는다.
func (buf Buffer) findForward(pattern *regexp.Regexp, from scheme.Cursor) (SearchResult, bool) {
	for i := 0; i <= len(buf.Lines); i++ {
		line := (from.Line + i) % len(buf.Lines)

		for _, match := range pattern.FindAllIndex(buf.Lines[line], -1) {
			// 커서 자리의 매칭에 다시 걸리면 `n` 이 제자리걸음을 한다.
			if i == 0 && match[0] <= from.Col {
				continue
			}
			if i == len(buf.Lines) && match[0] > from.Col {
				continue
			}

			return SearchResult{
				Cursor:  scheme.Cursor{Line: line, Col: match[0]},
				Wrapped: from.Line+i >= len(buf.Lines),
			}, true
		}
	}

	return SearchResult{}, false
}

// findBackward 는 커서 앞자리부터 위로 찾는다. 한 줄 안에서는 뒤에 있는 매칭이 먼저다.
func (buf Buffer) findBackward(pattern *regexp.Regexp, from scheme.Cursor) (SearchResult, bool) {
	for i := 0; i <= len(buf.Lines); i++ {
		// i 는 줄 수까지만 커지므로 한 번 더해주면 음수가 나오지 않는다.
		line := (from.Line - i + len(buf.Lines)) % len(buf.Lines)

		matches := pattern.FindAllIndex(buf.Lines[line], -1)
		for j := len(matches) - 1; j >= 0; j-- {
			match := matches[j]

			if i == 0 && match[0] >= from.Col {
				continue
			}
			if i == len(buf.Lines) && match[0] < from.Col {
				continue
			}

			return SearchResult{
				Cursor:  scheme.Cursor{Line: line, Col: match[0]},
				Wrapped: from.Line-i < 0,
			}, true
		}
	}

	return SearchResult{}, false
}

// SearchDirection 은 검색이 훑는 방향이다. `/` 가 아래로, `?` 가 위로다.
type SearchDirection int

const (
	SearchForward SearchDirection = iota
	SearchBackward
)

// SearchResult 는 찾은 자리다.
//
// **자리를 Cursor 로 든다.** 찾은 것을 그대로 다음 검색의 시작으로 넘길 수 있어야 한다 —
// `3n` 은 결과를 다시 시작 자리로 삼아 세 번 돈다. Line·Col 을 낱개로 들면 부르는 쪽이
// 그 짝을 손으로 굴리게 된다.
type SearchResult struct {
	scheme.Cursor

	// Wrapped 는 파일 끝이나 처음을 지나 감쌌는지다. vim 처럼 아래 줄에 알린다 —
	// 알리지 않으면 커서가 왜 뒤로 갔는지 알 수 없다.
	Wrapped bool
}

// reverse 는 반대 방향이다. `N` 이 마지막 검색을 거꾸로 되풀이할 때 쓴다.
func (d SearchDirection) Reverse() SearchDirection {
	if d == SearchBackward {
		return SearchForward
	}

	return SearchBackward
}
