package core

import (
	"strconv"
)

// partial 은 짓는 중인 동작에서 이미 정해진 부분이다. 모든 상태가 이것을 들고 다닌다.
//
// **operator 를 중첩 상태로 들지 않는 이유가 여기 있다.** `d` 를 먹으면 여기에 적어 두고
// 상태는 처음(normalStart) 으로 돌아간다. 그러면 motion 자리의 키가 최상위와 똑같은 상태들을
// 그대로 지나가므로, 숫자 접두(`d3w`) 와 접두 키(`dgg`) 의 규칙이 한 벌로 유지된다(ADR-0013).
//
// "무엇을 기다리는가" 는 type 이 나타내고 "무엇이 정해졌는가" 는 이 값이 나타낸다.
type partial struct {
	op      string // 이미 먹은 operator(`d` `y` `c`). "" 면 없음
	opCount int    // 그 앞에 붙은 숫자
}

// resolve 는 키 하나로 동작을 짓는다. 지을 수 없으면 nil 이다.
//
// motion 이면 operator 를 얹고(`dw`) operator 가 없으면 이동이다(`w`).
// motion 이 아니면 홀로 서는 동작인데, 그것은 operator 뒤에 올 수 없다 — `di` 는 아무것도 아니다.
func (p partial) resolve(key string, count int) action {
	if mo, ok := motionFor(p.op, key); ok {
		return p.apply(mo, count)
	}

	if p.op != "" {
		return nil
	}

	return standaloneAction(key, count)
}

// apply 는 motion 에 operator 를 얹는다. operator 가 없으면 이동 동작이다.
func (p partial) apply(mo moveMotion, count int) action {
	if p.op == "" {
		return actionMove{motion: mo, count: count}
	}

	return p.operate(mo, count)
}

// operate 는 범위 하나에 operator 를 얹는다. `dd` 처럼 이동이 아닌 범위도 여기로 온다.
func (p partial) operate(mo motion, count int) action {
	n := operatorCount(p.opCount, count)

	switch p.op {
	case "d":
		return actionDelete{motion: mo, count: n}
	case "y":
		return actionYank{motion: mo, count: n}
	case "c":
		return actionChange{motion: mo, count: n}
	}

	return nil
}

// showcmd 는 이미 정해진 부분을 화면에 찍는 글자다. 상태가 자기 몫을 뒤에 잇는다.
func (p partial) showcmd() string {
	return countString(p.opCount) + p.op
}

// startOperator 는 operator 키를 먹었을 때의 다음 상태다. count 는 그 앞에 모아둔 숫자다.
//
// 이미 operator 가 있는데 다른 것을 또 치면(`dy`) 앞의 것을 무르고 새로 연다. vim 이 그렇다 —
// 잘못된 motion 에서 바로 무르고 다음 키를 동작으로 받는다(tmux 로 vim 9.1 확인).
// 무르는 것이므로 모아둔 숫자도 앞 동작의 것이라 같이 버린다.
func (p partial) startOperator(op string, count int) normalState {
	if p.op != "" {
		return normalStart{building: partial{op: op}}
	}

	return normalStart{building: partial{op: op, opCount: count}}
}

// motionFor 는 키가 가리키는 motion 이다. motion 이 아니면 false 다.
//
// **operator 에 따라 갈리는 자리가 하나 있다.** `cw` 는 `ce` 라서 `c` 뒤의 `w` 만 다른 type 이다
// (ADR-0033). 나머지는 operator 를 보지 않는다.
func motionFor(op, key string) (moveMotion, bool) {
	switch key {
	case "h", "left":
		return motionLeft{}, true
	case "l", "right":
		return motionRight{}, true
	case "0":
		return motionLineStart{}, true
	case "^":
		return motionFirstNonBlank{}, true
	case "$":
		return motionLineEnd{}, true
	case "b":
		return motionWordBack{kind: smallWord}, true
	case "B":
		return motionWordBack{kind: bigWord}, true
	case "e":
		return motionWordEnd{kind: smallWord}, true
	case "E":
		return motionWordEnd{kind: bigWord}, true
	case "w":
		if op == "c" {
			return motionChangeWord{kind: smallWord}, true
		}

		return motionWordForward{kind: smallWord}, true
	case "W":
		if op == "c" {
			return motionChangeWord{kind: bigWord}, true
		}

		return motionWordForward{kind: bigWord}, true
	case "j":
		return motionLineDown{}, true
	case "k":
		return motionLineUp{}, true
	case "G":
		return motionToLastLine{}, true
	case "up":
		return motionRowUp{}, true
	case "down":
		return motionRowDown{}, true
	}

	return nil, false
}

// standaloneAction 는 motion 이 아닌, 홀로 서는 동작이다. 없으면 nil 이다.
//
// 숫자를 쓰지 않는 동작은 count 를 그냥 무시한다 — `3i` 가 `i` 인 것이 그래서다.
// 예전에는 숫자를 받는 키 목록을 따로 두어 걸러야 했는데, 동작이 type 이 되면서 그 표가 없어졌다.
func standaloneAction(key string, count int) action {
	switch key {
	case "ctrl+c":
		return actionQuit{}
	case "ctrl+z":
		return actionSuspend{}
	case ":":
		return actionOpenCommandLine{}
	case "ctrl+p":
		return actionOpenPalette{}
	case "/":
		return actionSearch{direction: searchForward}
	case "?":
		return actionSearch{direction: searchBackward}
	case "n":
		return actionNextMatch{count: count}
	case "N":
		return actionPrevMatch{count: count}
	case "*":
		return actionSearchWord{direction: searchForward, count: count}
	case "#":
		return actionSearchWord{direction: searchBackward, count: count}
	case "i":
		return actionInsert{}
	case "a":
		return actionAppend{}
	case "o":
		return actionOpenBelow{}
	case "O":
		return actionOpenAbove{}
	case "x":
		// `dl` 과 같다. 줄 끝을 넘지 않으므로 다음 줄이 끌려 올라오지 않는다. vim 과 같다.
		return actionDelete{motion: motionRight{}, count: count}
	case "p":
		return actionPasteAfter{count: count}
	case "P":
		return actionPasteBefore{count: count}
	case "u":
		return actionUndo{}
	case "ctrl+r":
		return actionRedo{}
	}

	return nil
}

// prefixMotion 은 접두 키 조합이 가리키는 motion 이다. operator 뒤에도 올 수 있다(`dgg`).
func prefixMotion(prefix, key string) (moveMotion, bool) {
	if prefix == "g" && key == "g" {
		return motionToFirstLine{}, true
	}

	return nil, false
}

// prefixAction 는 접두 키 조합이 가리키는, motion 이 아닌 동작이다.
// operator 뒤에는 올 수 없다 — `dgt` 는 아무것도 아니다.
func prefixAction(prefix, key string) action {
	switch prefix {
	case "g":
		switch key {
		case "t":
			return actionNextTab{}
		case "T":
			return actionPrevTab{}
		}
	case "ctrl+w":
		switch key {
		case "ctrl+w", "w":
			return actionFocusTree{}
		}
	}

	return nil
}

// normalState 는 normal mode 가 키를 받아가며 옮겨 다니는 상태다.
//
// 글자를 먹어 토큰을 뱉는 tokenizerState 와 같은 모양이다(command-parser.go).
//
// type 은 "다음에 무엇을 기다리는가" 만 나타낸다 — 숫자를 모으는 중, 접두 키 뒤, 글자 하나 뒤.
// 이미 정해진 것(operator 와 그 앞 숫자) 은 type 이 아니라 building 이 든다(ADR-0006, ADR-0013).
//
// 계약은 이 둘뿐이다. 풀린 키 하나를 먹이는 자리는 밖으로 내지 않는다 — 그것이 계약에 있으면
// 풀리지 않은 키를 먹여 조용히 아무 일도 안 하게 되는 길이 생긴다.
type normalState interface {
	// press 는 키 하나를 먹여 완성된 동작들을 준다.
	//
	// **한글로 온 키를 그대로 받는다.** 두벌식 자리의 영문 키로 푸는 것이 이 안에서 일어나므로
	// 키 하나가 동작 여럿이 될 수 있다 — `ㅘ` 는 `h` `k` 라 왼쪽·위 두 번이다(ADR-0008).
	// 아직 동작이 되지 않았으면 빈 목록이다.
	//
	// **푸는지 마는지는 상태마다 다르다.** 동작을 기다리는 상태는 풀고, 글자를 기다리는
	// 상태(normalReplace) 는 그대로 받는다 — `r` 뒤의 한 키를 풀면 `한` 이 `g` `k` `s` 가
	// 되어 한글을 넣을 수 없다(ADR-0018).
	press(key string) ([]action, normalState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// pressExpanded 는 풀린 키들을 차례로 먹여 완성된 동작들을 모은다.
//
// 되먹이는 것은 press 다. 조각은 전부 ASCII 라 각자의 press 첫머리에서 곧바로 빠져나오므로
// (expandHangul 의 주석) 여기서 다시 풀리지 않고 재귀가 한 겹에서 끝난다.
func pressExpanded(state normalState, keys []string) ([]action, normalState) {
	actions := make([]action, 0, len(keys))
	for _, k := range keys {
		next, state2 := state.press(k)
		state = state2

		actions = append(actions, next...)
	}

	return actions, state
}

// maxCount 는 count 가 커지는 한계다. 넘으면 더 치는 자리를 버린다.
// 줄 수보다 훨씬 크면 어차피 양끝에서 멈추므로, 숫자가 int 를 넘치지 않게 막기만 하면 된다.
const maxCount = 1_000_000

// one 은 동작 하나짜리 목록이다. 지을 수 없었으면 빈 목록이다.
func one(built action) []action {
	if built == nil {
		return nil
	}

	return []action{built}
}

// normalStart 는 다음 키를 동작의 시작으로 받는 상태다.
//
// operator 를 먹은 직후도 여기다 — building 에 그것이 적혀 있고, 뒤따르는 motion 은
// 이동 키를 그냥 친 것과 똑같은 길을 지난다.
type normalStart struct {
	building partial
}

func (s normalStart) press(key string) ([]action, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	// 자모 하나는 영문 키 하나로 바뀐다 — `ㅁ` 이 `a` 다. 개수가 같아도 글자가 다르다.
	key = keys[0]

	// `0` 은 count 의 첫 자리가 될 수 없다. vim 에서 줄 시작으로 가는 키라 자리를 비워둔다.
	if n, ok := keyDigit(key); ok && n > 0 {
		return nil, normalCount{building: s.building, count: n}
	}

	switch key {
	case "g", "ctrl+w":
		// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 showcmd 만 바뀐다.
		return nil, normalPending{building: s.building, prefix: key}
	case "r":
		// 뒤에 바꿔 넣을 글자 한 개가 붙는다.
		return nil, normalReplace{building: s.building}
	case "d", "y", "c":
		// operator 를 두 번 치면 줄 단위다(`dd`).
		if s.building.op == key {
			return one(s.building.operate(motionWholeLines{}, 0)), normalStart{}
		}

		return nil, s.building.startOperator(key, 0)
	}

	return one(s.building.resolve(key, 0)), normalStart{}
}

func (s normalStart) showcmd() string { return s.building.showcmd() }

// normalCount 는 숫자를 모으는 중이다.
type normalCount struct {
	building partial
	count    int
}

func (s normalCount) press(key string) ([]action, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	key = keys[0]

	if n, ok := keyDigit(key); ok {
		if s.count >= maxCount {
			return nil, s
		}

		return nil, normalCount{building: s.building, count: s.count*10 + n}
	}

	switch key {
	case "g":
		// 접두 키는 숫자를 들고 다음 키를 기다린다. `10gg` 는 10 번째 줄이다.
		// `ctrl+w` 는 여기 없다 — 창 동작에 숫자는 뜻이 없어서 아래에서 버려진다.
		return nil, normalPending{building: s.building, prefix: key, count: s.count}
	case "r":
		// `3rx` 는 세 글자를 바꾼다.
		return nil, normalReplace{building: s.building, count: s.count}
	case "d", "y", "c":
		// `3dd` 는 세 줄이고 `d3d` 도 같다.
		if s.building.op == key {
			return one(s.building.operate(motionWholeLines{}, s.count)), normalStart{}
		}

		return nil, s.building.startOperator(key, s.count)
	}

	if built := s.building.resolve(key, s.count); built != nil {
		return one(built), normalStart{}
	}

	// 동작이 되지 않는 키다. 모으던 숫자를 버리고 그 키만 친 것으로 본다.
	// 이미 풀린 키라 normalStart 의 press 도 첫머리에서 그대로 빠져나온다.
	return normalStart{building: s.building}.press(key)
}

func (s normalCount) showcmd() string {
	return s.building.showcmd() + strconv.Itoa(s.count)
}

// normalPending 은 `g` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 여기서 끝난다. 짝이 없는 조합은 아무 일도 하지 않는다.
// `esc` 와 `ctrl+c` 도 여기로 와서 버려진다 — 잘못 누른 접두 키를 무르는 것이라 vim 과 같고,
// `g` 뒤에 손이 미끄러져서 편집기가 꺼지는 일도 없다.
//
// operator 아래에서도 같다. `dgd` 는 짝이 없어서 아무 일도 하지 않지 `dd` 가 아니다.
type normalPending struct {
	building partial
	prefix   string
	count    int
}

func (s normalPending) press(key string) ([]action, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpanded(s, keys)
	}
	key = keys[0]

	if mo, ok := prefixMotion(s.prefix, key); ok {
		return one(s.building.apply(mo, s.count)), normalStart{}
	}

	// motion 이 아닌 조합은 operator 뒤에 올 수 없다. `dgt` 는 아무것도 아니다.
	if s.building.op != "" {
		return nil, normalStart{}
	}

	return one(prefixAction(s.prefix, key)), normalStart{}
}

func (s normalPending) showcmd() string {
	return s.building.showcmd() + countString(s.count) + s.prefix
}

// normalReplace 는 `r` 을 먹고 바꿔 넣을 글자 한 개를 기다리는 상태다.
//
// 다음 키는 동작이 아니라 파일에 들어갈 글자다. 그래서 접두 키(normalPending) 와 따로 있다 —
// 한글 되돌림(ADR-0008) 을 이 한 키만 건너뛰어야 `r한` 이 한글을 넣는다(ADR-0018).
// 글자가 아닌 키는 실행하는 쪽이 무른다.
type normalReplace struct {
	building partial
	count    int
}

// 풀지 않는 상태는 이것 하나뿐이다. 그래서 다른 상태들이 첫머리에 두는 expandHangul 이 없다.
//
// operator 뒤의 `r` 은 짝이 없다 — `dr<글자>` 는 아무 일도 하지 않는다.
func (s normalReplace) press(key string) ([]action, normalState) {
	if s.building.op != "" {
		return nil, normalStart{}
	}

	return one(actionReplaceChar{key: key, count: s.count}), normalStart{}
}

func (s normalReplace) showcmd() string {
	return s.building.showcmd() + countString(s.count) + "r"
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
