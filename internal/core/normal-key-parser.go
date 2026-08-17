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
//
// 숫자 접두, `g` 같은 접두 키, operator-pending(`d` `c` `y`) 과 인자를 한 글자 더 받는 키(`r`)
// 가 모두 "다음 키를 기다리는 상태" 다. 필드를 하나씩 늘리는 대신 상태를 하나씩 늘린다(ADR-0006).
//
// 계약은 이 둘뿐이다. 풀린 키 하나를 먹이는 자리는 밖으로 내지 않는다 — 그것이 계약에 있으면
// `state.???("한")` 처럼 풀리지 않은 키를 먹여 조용히 모르는 명령이 되는 길이 생긴다.
type normalState interface {
	// press 는 키 하나를 먹여 완성된 명령들을 준다.
	//
	// **한글로 온 키를 그대로 받는다.** 두벌식 자리의 영문 키로 푸는 것이 이 안에서 일어나므로
	// 키 하나가 명령 여럿이 될 수 있다 — `ㅘ` 는 `h` `k` 라 왼쪽·위 두 번이다(ADR-0008).
	// 아직 명령이 되지 않았으면 빈 목록이다.
	//
	// **푸는지 마는지는 상태마다 다르다.** 명령을 기다리는 상태는 풀고, 글자를 기다리는
	// 상태(normalReplace) 는 그대로 받는다 — `r` 뒤의 한 키를 풀면 `한` 이 `g` `k` `s` 가
	// 되어 한글을 넣을 수 없다(ADR-0018).
	press(key string) ([]normalKey, normalState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// pressExpanded 는 풀린 키들을 차례로 먹여 완성된 명령들을 모은다.
//
// 되먹이는 것은 press 다. 조각은 전부 ASCII 라 각자의 press 첫머리에서 곧바로 빠져나오므로
// (expandHangul 의 주석) 여기서 다시 풀리지 않고 재귀가 한 겹에서 끝난다.
func pressExpanded(state normalState, keys []string) ([]normalKey, normalState) {
	commands := make([]normalKey, 0, len(keys))
	for _, k := range keys {
		next, state2 := state.press(k)
		state = state2

		commands = append(commands, next...)
	}

	return commands, state
}

// maxCount 는 count 가 커지는 한계다. 넘으면 더 치는 자리를 버린다.
// 줄 수보다 훨씬 크면 어차피 양끝에서 멈추므로, 숫자가 int 를 넘치지 않게 막기만 하면 된다.
const maxCount = 1_000_000

// one 은 명령 하나짜리 목록이다. 이름이 비면(아직 기다리는 중) 빈 목록이다.
func one(command normalKey, next normalState) ([]normalKey, normalState) {
	if command.name == "" {
		return nil, next
	}

	return []normalKey{command}, next
}

// normalStart 는 아무것도 먹지 않은 처음이다.
type normalStart struct{}

func (s normalStart) press(key string) ([]normalKey, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	// 자모 하나는 영문 키 하나로 바뀐다 — `ㅁ` 이 `a` 다. 개수가 같아도 글자가 다르다.
	key = keys[0]

	// `0` 은 count 의 첫 자리가 될 수 없다. vim 에서 줄 시작으로 가는 키라 자리를 비워둔다.
	if n, ok := keyDigit(key); ok && n > 0 {
		return nil, normalCount{count: n}
	}

	switch key {
	case "g", "ctrl+w":
		// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 showcmd 만 바뀐다.
		return nil, normalPending{prefix: key}
	case "d", "y", "c":
		// 뒤에 motion 이 붙어서 지우거나 복사하거나 바꿀 범위를 정한다.
		return nil, normalOperator{op: key}
	case "r":
		// 뒤에 바꿔 넣을 글자 한 개가 붙는다.
		return nil, normalReplace{}
	}

	return one(normalKey{name: key}, normalStart{})
}

func (s normalStart) showcmd() string { return "" }

// normalCount 는 숫자를 모으는 중이다.
type normalCount struct {
	count int
}

func (s normalCount) press(key string) ([]normalKey, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	key = keys[0]

	if n, ok := keyDigit(key); ok {
		if s.count >= maxCount {
			return nil, s
		}

		return nil, normalCount{count: s.count*10 + n}
	}

	switch key {
	case "g":
		// 접두 키는 숫자를 들고 다음 키를 기다린다. `10gg` 는 10 번째 줄이다.
		return nil, normalPending{prefix: key, count: s.count}
	case "d", "y", "c":
		// operator 도 숫자를 들고 간다. `3dd` 는 세 줄이다.
		return nil, normalOperator{op: key, count: s.count}
	case "r":
		// `3rx` 는 세 글자를 바꾼다.
		return nil, normalReplace{count: s.count}
	case "h", "j", "k", "l", "w", "W", "e", "E", "b", "B", "$", "G", "n", "N", "*", "#", "x", "p", "P":
		return one(normalKey{name: key, count: s.count}, normalStart{})
	}

	// 숫자를 쓰지 않는 키다. 모으던 숫자를 버리고 그 키만 친 것으로 본다.
	// 이미 풀린 키라 normalStart 의 press 도 첫머리에서 그대로 빠져나온다.
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

func (s normalPending) press(key string) ([]normalKey, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	key = keys[0]

	return one(normalKey{name: s.prefix + " " + key, count: s.count}, normalStart{})
}

func (s normalPending) showcmd() string {
	return countString(s.count) + s.prefix
}

// normalReplace 는 `r` 을 먹고 바꿔 넣을 글자 한 개를 기다리는 상태다.
//
// 다음 키는 명령이 아니라 파일에 들어갈 글자다. 그래서 접두 키(normalPending) 와 따로 있다 —
// 한글 되돌림(ADR-0008) 을 이 한 키만 건너뛰어야 `r한` 이 한글을 넣는다(ADR-0018).
// 이름은 접두 키와 같은 모양으로 잇는다(`r x`). 글자가 아닌 키는 실행하는 쪽이 무른다.
type normalReplace struct {
	count int
}

// 풀지 않는 상태는 이것 하나뿐이다. 그래서 다른 상태들이 첫머리에 두는 expandHangul 이 없다.
func (s normalReplace) press(key string) ([]normalKey, normalState) {
	return one(normalKey{name: "r " + key, count: s.count}, normalStart{})
}

func (s normalReplace) showcmd() string {
	return countString(s.count) + "r"
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

// motion 을 기다리는 중이라 푼다. `dr` 처럼 inner 가 글자를 기다리게 된 자리는 보지 않는다 —
// `d r <글자>` 는 어차피 모르는 이름이라 아무 일도 하지 않는다. `f`·`t` 를 넣어 operator 뒤에
// 인자를 받는 것이 뜻을 가지면 그때 inner 에 묻는 자리가 생긴다(docs/tasks.md).
func (s normalOperator) press(key string) ([]normalKey, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	key = keys[0]

	inner := s.motionState()

	// operator 를 두 번 치면 줄 단위다(`dd`). inner 에 넘기면 operator 를 하나 더 여는 셈이 된다.
	// 그 앞에 모아둔 숫자는 줄 수다 — `d3d` 는 `3dd` 와 같다.
	if key == s.op {
		return one(normalKey{name: s.op + " " + s.op, count: operatorCount(s.count, motionCount(inner))}, normalStart{})
	}

	// 이미 풀린 키라 inner 의 press 도 첫머리에서 그대로 빠져나와 명령이 하나 이하다.
	motions, next := inner.press(key)
	if len(motions) == 0 {
		return nil, normalOperator{op: s.op, count: s.count, inner: next}
	}

	motion := motions[0]

	return one(normalKey{name: s.op + " " + motion.name, count: operatorCount(s.count, motion.count)}, normalStart{})
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
