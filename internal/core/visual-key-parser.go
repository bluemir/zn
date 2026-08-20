package core

import (
	"strconv"
)

// visualState 는 visual mode 가 키를 받아가며 옮겨 다니는 상태다.
//
// normal mode 의 normalState 와 같은 모양이지만 표를 나눠 가진다(normal-key-parser.go).
// normalState 를 그대로 쓸 수 없는 것은 normalStart 가 `d`·`y`·`c` 를 operator 로 먹기
// 때문이다 — visual 에서 `d` 는 다음 키를 기다리지 않고 그 자리에서 끝난다. sidebarState 가
// 갈린 것과 같은 이유다(ADR-0006, ADR-0037).
//
// 기다리는 모양은 숫자 접두와 접두 키(`g`) 둘뿐이라 상태가 셋이다. operator 가 없어서
// normalState 의 partial 도, 글자 하나를 받는 normalReplace 도 여기에는 없다.
type visualState interface {
	// press 는 키 하나를 먹여 완성된 동작들을 준다. 한글로 온 키를 그대로 받는다.
	// 계약은 normalState 의 것과 같은 자리이고 이 둘뿐이다(normal-key-parser.go).
	press(key string) ([]action, visualState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// pressExpandedVisual 은 풀린 키들을 차례로 먹인다. normal 쪽 pressExpanded 와 같다.
func pressExpandedVisual(state visualState, keys []string) ([]action, visualState) {
	actions := make([]action, 0, len(keys))
	for _, k := range keys {
		next, state2 := state.press(k)
		state = state2

		actions = append(actions, next...)
	}

	return actions, state
}

// visualAction 는 visual mode 에서 이동이 아닌 키가 가리키는 동작이다. 없으면 nil 이다.
//
// 숫자를 쓰지 않는 동작은 count 를 그냥 무시한다. 고른 범위가 이미 정해져 있어서
// `3d` 가 `d` 와 같다 — 되풀이할 것이 없다.
//
// `:` `/` `gt` `ctrl+p` `ctrl+w` 는 여기 없다. 짝이 없는 조합이 아무 일도 하지 않는 것과 같다.
func visualAction(key string) action {
	switch key {
	case "ctrl+c":
		return actionQuit{}
	case "esc":
		return actionVisualLeave{}
	case "v":
		return actionVisualSwitch{}
	case "V":
		return actionVisualSwitch{linewise: true}
	case "d", "x":
		return actionVisualDelete{}
	case "y":
		return actionVisualYank{}
	case "c":
		return actionVisualChange{}
	}

	return nil
}

// visualStart 는 다음 키를 동작의 시작으로 받는 상태다.
type visualStart struct{}

func (s visualStart) press(key string) ([]action, visualState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedVisual(s, keys)
	}
	key = keys[0]

	// `0` 은 count 의 첫 자리가 될 수 없다. 줄 시작으로 가는 키라 자리를 비워둔다.
	if n, ok := keyDigit(key); ok && n > 0 {
		return nil, visualCount{count: n}
	}

	if key == "g" {
		return nil, visualPending{prefix: key}
	}

	if mo, ok := motionFor("", key); ok {
		return one(actionMove{motion: mo}), s
	}

	return one(visualAction(key)), s
}

func (visualStart) showcmd() string { return "" }

// visualCount 는 숫자를 모으는 중이다.
type visualCount struct{ count int }

func (s visualCount) press(key string) ([]action, visualState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedVisual(s, keys)
	}
	key = keys[0]

	if n, ok := keyDigit(key); ok {
		if s.count >= maxCount {
			return nil, s
		}

		return nil, visualCount{count: s.count*10 + n}
	}

	// 접두 키는 숫자를 들고 다음 키를 기다린다. `10gg` 는 10 번째 줄이다.
	if key == "g" {
		return nil, visualPending{prefix: key, count: s.count}
	}

	if mo, ok := motionFor("", key); ok {
		return one(actionMove{motion: mo, count: s.count}), visualStart{}
	}

	// 이동이 아닌 키다. 모으던 숫자를 버리고 그 키만 친 것으로 본다.
	// 이미 풀린 키라 visualStart 의 press 도 첫머리에서 그대로 빠져나온다.
	return visualStart{}.press(key)
}

func (s visualCount) showcmd() string { return strconv.Itoa(s.count) }

// visualPending 은 `g` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 여기서 끝난다. 짝이 없는 조합은 아무 일도 하지 않는다 —
// `gt` 는 visual 이 받지 않으므로 여기서 버려진다.
type visualPending struct {
	prefix string
	count  int
}

func (s visualPending) press(key string) ([]action, visualState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedVisual(s, keys)
	}
	key = keys[0]

	if mo, ok := prefixMotion(s.prefix, key); ok {
		return one(actionMove{motion: mo, count: s.count}), visualStart{}
	}

	return nil, visualStart{}
}

func (s visualPending) showcmd() string { return countString(s.count) + s.prefix }
