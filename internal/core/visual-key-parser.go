package core

import (
	"strconv"
	"unicode/utf8"
)

// visualState 는 visual mode 가 키를 받아가며 옮겨 다니는 상태다.
//
// normal mode 의 normalState 와 같은 모양이지만 표를 나눠 가진다(normal-key-parser.go).
// normalState 를 그대로 쓸 수 없는 것은 normalStart 가 `d`·`y`·`c`·`>`·`<`·`=` 를 operator 로
// 먹기 때문이다 — visual 에서 `d` 는 다음 키를 기다리지 않고 그 자리에서 끝난다. sidebarState 가
// 갈린 것과 같은 이유다(ADR-0006, ADR-0037).
//
// 기다리는 모양은 숫자 접두, 접두 키(`g`), register 이름 셋이라 상태가 넷이다. operator 가
// 없어서 normalState 의 partial 도, 글자 하나를 받는 normalReplace 도 여기에는 없다.
// 이름은 partial 대신 상태마다 `reg` 로 들고 다닌다 — 들 것이 그것 하나라서다(ADR-0058).
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
// 숫자를 보는 것은 반 화면 이동뿐이다. 나머지는 고른 범위가 이미 정해져 있어서 count 를 그냥
// 무시한다 — `3d` 가 `d` 와 같고 되풀이할 것이 없다.
//
// `:` `/` `gt` `ctrl+p` `ctrl+w` 는 여기 없다. 짝이 없는 조합이 아무 일도 하지 않는 것과 같다.
func visualAction(key string, count int) action {
	switch key {
	case "ctrl+c":
		return actionQuit{}
	case "ctrl+d":
		// 고른 범위가 커서를 따라 자란다. 이동 키를 친 것과 같다 — 화면 단위 이동이 motion 이
		// 아니라 홀로 서는 동작인 것은 화면 높이가 있어야 정해지기 때문이다(ADR-0062).
		return actionPage{direction: pageDown, span: pageHalf, count: count}
	case "ctrl+u":
		return actionPage{direction: pageUp, span: pageHalf, count: count}
	case "ctrl+f":
		return actionPage{direction: pageDown, span: pageFull, count: count}
	case "ctrl+b":
		return actionPage{direction: pageUp, span: pageFull, count: count}
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
	case ">":
		return actionVisualIndent{direction: indentRight}
	case "<":
		return actionVisualIndent{direction: indentLeft}
	case "=":
		return actionVisualReindent{}
	}

	return nil
}

// visualWithRegister 는 이름을 실은 동작이다. 이름을 받지 않는 동작이면 버린다.
//
// normal 쪽 `partial.built` 와 같은 규칙이고 통과하는 갈래만 다르다 — visual 에는 붙여넣기가
// 아직 없어서(docs/tasks.md) 담는 것 셋뿐이다. 숫자 이름에 담을 수 없는 것도 같다(ADR-0058).
func visualWithRegister(built action, reg string) action {
	if reg == "" {
		return built
	}
	if !registerWritable(reg) {
		return nil
	}

	switch a := built.(type) {
	case actionVisualDelete:
		a.reg = reg

		return a
	case actionVisualYank:
		a.reg = reg

		return a
	case actionVisualChange:
		a.reg = reg

		return a
	}

	return nil
}

// visualStart 는 다음 키를 동작의 시작으로 받는 상태다.
type visualStart struct {
	reg string // `"` 로 고른 register 이름. "" 면 무명이다
}

func (s visualStart) press(key string) ([]action, visualState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedVisual(s, keys)
	}
	key = keys[0]

	// `0` 은 count 의 첫 자리가 될 수 없다. 줄 시작으로 가는 키라 자리를 비워둔다.
	if n, ok := keyDigit(key); ok && n > 0 {
		return nil, visualCount{reg: s.reg, count: n}
	}

	if key == "g" {
		return nil, visualPending{reg: s.reg, prefix: key}
	}

	if key == `"` {
		// 뒤에 register 이름 한 개가 붙는다.
		return nil, visualRegister{}
	}

	if mo, ok := motionFor("", key); ok {
		// 이름을 실은 이동은 없다. normal 의 `"1w` 와 같이 아무 일도 하지 않는다 —
		// 범위를 눈으로 고른 뒤에 이름을 대는 것이라 그 사이에 이동이 낄 자리가 없다.
		if s.reg != "" {
			return nil, visualStart{}
		}

		return one(actionMove{motion: mo}), s
	}

	return one(visualWithRegister(visualAction(key, 0), s.reg)), visualStart{}
}

func (s visualStart) showcmd() string { return registerString(s.reg) }

// visualCount 는 숫자를 모으는 중이다.
type visualCount struct {
	reg   string
	count int
}

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

		return nil, visualCount{reg: s.reg, count: s.count*10 + n}
	}

	// 접두 키는 숫자를 들고 다음 키를 기다린다. `10gg` 는 10 번째 줄이다.
	if key == "g" {
		return nil, visualPending{reg: s.reg, prefix: key, count: s.count}
	}

	if key == `"` {
		return nil, visualRegister{count: s.count}
	}

	if mo, ok := motionFor("", key); ok {
		if s.reg != "" {
			return nil, visualStart{}
		}

		return one(actionMove{motion: mo, count: s.count}), visualStart{}
	}

	// 이동이 아닌 키다. 숫자를 보는 것은 반 화면 이동뿐이고 나머지는 그 키만 친 것과 같다 —
	// 동작들이 count 를 받지 않으므로 여기서 걸러 두지 않고 그냥 넘긴다(visualAction).
	return one(visualWithRegister(visualAction(key, s.count), s.reg)), visualStart{}
}

func (s visualCount) showcmd() string { return registerString(s.reg) + strconv.Itoa(s.count) }

// visualRegister 는 `"` 를 먹고 register 이름 한 개를 기다리는 상태다.
//
// normal 쪽 normalRegister 와 같은 규칙이다 — 글자 하나면 무엇이든 이름이고, 글자 하나가
// 아닌 키(`esc` `ctrl+c`) 는 무른다. 한글도 같이 되돌린다(ADR-0008, ADR-0058).
//
// **모아둔 숫자는 여기서 버린다.** 이름을 받는 동작들은 고른 범위가 이미 정해져 있어서
// count 를 쓰지 않는다 — `3d` 가 `d` 인 것과 같은 자리다(visualAction).
type visualRegister struct{ count int }

func (s visualRegister) press(key string) ([]action, visualState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return nil, visualStart{}
	}
	key = keys[0]

	if utf8.RuneCountInString(key) != 1 {
		return nil, visualStart{}
	}

	return nil, visualStart{reg: key}
}

func (s visualRegister) showcmd() string { return countString(s.count) + `"` }

// visualPending 은 `g` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 여기서 끝난다. 짝이 없는 조합은 아무 일도 하지 않는다 —
// `gt` 는 visual 이 받지 않으므로 여기서 버려진다.
type visualPending struct {
	reg    string
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
		// 이름을 실은 이동은 없다. visualStart 와 같은 자리다.
		if s.reg != "" {
			return nil, visualStart{}
		}

		return one(actionMove{motion: mo, count: s.count}), visualStart{}
	}

	return nil, visualStart{}
}

func (s visualPending) showcmd() string {
	return registerString(s.reg) + countString(s.count) + s.prefix
}
