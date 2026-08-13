package core

import (
	"strconv"
)

// normalKey 는 키 하나 또는 여럿이 모여 완성된 normal mode 명령이다.
//
// 접두 키가 붙은 것은 `g t` 처럼 공백으로 이어 붙인 한 이름이다.
// 그래야 실행하는 쪽이 몇 개를 눌러 만든 이름인지 알 필요 없이 평평한 switch 로 받는다.
type normalKey struct {
	name  string // 완성된 이름. 빈 값이면 아직 다음 키를 기다리는 중이다
	count int    // 앞에 붙은 숫자. 없으면 0
}

// normalState 는 normal mode 가 키를 받아가며 옮겨 다니는 상태다.
//
// 글자를 먹어 토큰을 뱉는 tokenizerState 와 같은 모양이다(command-parser.go).
// 명령이 완성되지 않았으면 빈 이름을 준다.
//
// 숫자 접두, `g` 같은 접두 키, 앞으로 들어올 operator-pending(`d` `c` `y`) 과
// 인자를 한 글자 더 받는 키(`f` `t`) 가 모두 "다음 키를 기다리는 상태" 다.
// 필드를 하나씩 늘리는 대신 상태를 하나씩 늘린다(ADR-0006).
type normalState interface {
	press(key string) (normalKey, normalState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// maxCount 는 count 가 커지는 한계다. 넘으면 더 치는 자리를 버린다.
// 줄 수보다 훨씬 크면 어차피 양끝에서 멈추므로, 숫자가 int 를 넘치지 않게 막기만 하면 된다.
const maxCount = 1_000_000

// normalStart 는 아무것도 먹지 않은 처음이다.
type normalStart struct{}

func (s normalStart) press(key string) (normalKey, normalState) {
	// `0` 은 count 의 첫 자리가 될 수 없다. vim 에서 줄 시작으로 가는 키라 자리를 비워둔다.
	if n, ok := keyDigit(key); ok && n > 0 {
		return normalKey{}, normalCount{count: n}
	}

	switch key {
	case "g", "ctrl+w":
		// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 showcmd 만 바뀐다.
		return normalKey{}, normalPending{prefix: key}
	case "d":
		// 뒤에 motion 이 붙어서 지울 범위를 정한다.
		return normalKey{}, normalOperator{op: key}
	}

	return normalKey{name: key}, normalStart{}
}

func (s normalStart) showcmd() string { return "" }

// normalCount 는 숫자를 모으는 중이다.
type normalCount struct {
	count int
}

func (s normalCount) press(key string) (normalKey, normalState) {
	if n, ok := keyDigit(key); ok {
		if s.count >= maxCount {
			return normalKey{}, s
		}

		return normalKey{}, normalCount{count: s.count*10 + n}
	}

	switch key {
	case "g":
		// 접두 키는 숫자를 들고 다음 키를 기다린다. `10gg` 는 10 번째 줄이다.
		return normalKey{}, normalPending{prefix: key, count: s.count}
	case "d":
		// operator 도 숫자를 들고 간다. `3dd` 는 세 줄이다.
		return normalKey{}, normalOperator{op: key, count: s.count}
	case "h", "j", "k", "l", "w", "W", "e", "E", "b", "B", "$", "G", "n", "N", "*", "#", "x":
		return normalKey{name: key, count: s.count}, normalStart{}
	}

	// 숫자를 쓰지 않는 키다. 모으던 숫자를 버리고 그 키만 친 것으로 본다.
	return normalStart{}.press(key)
}

func (s normalCount) showcmd() string { return strconv.Itoa(s.count) }

// normalPending 은 `g` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 이름이 완성된다. 짝이 없는 조합은 실행하는 쪽이 모르는 이름이라
// 아무 일도 하지 않는다. `esc` 와 `ctrl+c` 도 여기로 와서 버려진다 — 잘못 누른 접두 키를
// 무르는 것이라 vim 과 같고, `g` 뒤에 손이 미끄러져서 편집기가 꺼지는 일도 없다.
type normalPending struct {
	prefix string
	count  int // 접두 키 앞에 숫자가 있었으면 그것도 같이 들고 간다
}

func (s normalPending) press(key string) (normalKey, normalState) {
	return normalKey{name: s.prefix + " " + key, count: s.count}, normalStart{}
}

func (s normalPending) showcmd() string {
	return countString(s.count) + s.prefix
}

// normalOperator 는 `d` 처럼 뒤에 motion 이 붙어 범위를 정하는 키를 먹은 뒤다.
// vim 의 operator-pending 이다.
//
// motion 쪽 키는 inner 가 처음부터 다시 먹는다. 숫자(`d3w`)와 접두 키(`dgg`)가 이동 키에서와
// 똑같이 동작해야 하는데 그 규칙은 이미 다른 상태들에 있다. 여기서 다시 쓰면 두 벌이 되어 갈린다.
//
// 완성된 이름은 접두 키와 같은 모양으로 잇는다 — `d w`, `d g g`. 실행하는 쪽은 이름 하나로 받는다.
type normalOperator struct {
	op    string
	count int // operator 앞에 붙은 숫자

	// inner 는 motion 쪽 상태다. zero value(nil) 는 아직 아무것도 안 먹은 처음이다.
	inner normalState
}

func (s normalOperator) press(key string) (normalKey, normalState) {
	inner := s.motionState()

	// operator 를 두 번 치면 줄 단위다(`dd`). inner 에 넘기면 operator 를 하나 더 여는 셈이 된다.
	// 그 앞에 모아둔 숫자는 줄 수다 — `d3d` 는 `3dd` 와 같다.
	if key == s.op {
		return normalKey{name: s.op + " " + s.op, count: operatorCount(s.count, motionCount(inner))}, normalStart{}
	}

	motion, next := inner.press(key)
	if motion.name == "" {
		return normalKey{}, normalOperator{op: s.op, count: s.count, inner: next}
	}

	return normalKey{name: s.op + " " + motion.name, count: operatorCount(s.count, motion.count)}, normalStart{}
}

func (s normalOperator) showcmd() string {
	return countString(s.count) + s.op + s.motionState().showcmd()
}

func (s normalOperator) motionState() normalState {
	if s.inner == nil {
		return normalStart{}
	}

	return s.inner
}

// motionCount 는 motion 자리에 모아둔 숫자다. 숫자를 모으는 중이 아니면 없는 것이다.
func motionCount(state normalState) int {
	count, ok := state.(normalCount)
	if !ok {
		return 0
	}

	return count.count
}

// operatorCount 는 operator 앞뒤의 두 숫자를 하나로 합친다. vim 처럼 곱한다 — `3d2w` 는 여섯 단어다.
//
// 둘 다 없으면 0 이다. `dG` 처럼 숫자가 되풀이가 아니라 줄 번호인 motion 이 "숫자 없음" 을
// 알아야 해서, 없는 자리를 1 로 메워 넘기지 않는다.
func operatorCount(operator, motion int) int {
	if operator == 0 && motion == 0 {
		return 0
	}

	return min(max(operator, 1)*max(motion, 1), maxCount)
}

// countString 은 showcmd 에 붙일 숫자다. 숫자가 없으면 빈 값이다.
func countString(count int) string {
	if count == 0 {
		return ""
	}

	return strconv.Itoa(count)
}

// keyDigit 은 키가 숫자 하나면 그 값을 준다.
func keyDigit(key string) (int, bool) {
	if len(key) != 1 || key[0] < '0' || key[0] > '9' {
		return 0, false
	}

	return int(key[0] - '0'), true
}
