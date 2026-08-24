package core

import (
	"strconv"
	"unicode/utf8"
)

// partial 은 짓는 중인 동작에서 이미 정해진 부분이다. 모든 상태가 이것을 들고 다닌다.
//
// **operator 를 중첩 상태로 들지 않는 이유가 여기 있다.** `d` 를 먹으면 여기에 적어 두고
// 상태는 처음(normalStart) 으로 돌아간다. 그러면 motion 자리의 키가 최상위와 똑같은 상태들을
// 그대로 지나가므로, 숫자 접두(`d3w`) 와 접두 키(`dgg`) 의 규칙이 한 벌로 유지된다(ADR-0013).
//
// "무엇을 기다리는가" 는 type 이 나타내고 "무엇이 정해졌는가" 는 이 값이 나타낸다.
type partial struct {
	op      string // 이미 먹은 operator(`d` `y` `c` `>` `<` `=`). "" 면 없음
	opCount int    // 그 앞에 붙은 숫자
	reg     string // `"` 로 고른 register 이름. "" 면 무명이다(ADR-0058)
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

// built 는 지은 동작을 내보낸다. register 이름을 여기서 싣고, 받지 않는 동작이면 버린다.
//
// **싣고 거르는 자리가 여기 하나다.** 상태들이 동작을 내보내는 길이 여럿이라 (`dd` 는
// operate 로, `x` 는 standaloneAction 으로, `gg` 는 apply 로, `r` 은 자기 자리에서) 짓는
// 자리마다 이름을 실으면 한 군데를 빼먹는다 — `x` 가 `dl` 이라는 것이 그런 자리다.
// 그래서 짓는 쪽은 이름을 모르고, 내보내는 문에서 갈래를 보고 싣는다(ADR-0058).
//
// 이름을 받는 동작은 둘로 갈린다.
//
//   - 붙이는 것(`p` `P`) — 이름이 무엇이든 받는다. 없는 이름은 빈 register 라 조용하다
//   - 담는 것(`d` `y` `c` `x`) — **문자 이름만** 받는다. 숫자는 지울 때마다 저절로 채워지는
//     자리라 손으로 담아 두어도 다음 지우기가 밀어낸다(registerWritable)
//
// 버리는 것은 「아무 일도 하지 않음」이다. `"1yy` 는 지우지도 담지도 않는다 — 담기지 않은
// 것을 담은 척하지 않고, 나중에 숫자에도 담게 할 때 지금 동작하던 것이 바뀌는 자리가 없다.
// 이름을 받지 않는 키(`"1w` `"1gg` `"1rx`) 도 같이 버려진다.
//
// `esc`·`ctrl+c` 도 여기서 버려진다. 잘못 짚은 이름을 무르는 것이고, 무른 뒤 한 번 더
// 누르면 제 일을 한다 — 접두 키가 하는 것과 같다(normalPending). 손이 미끄러진 `"` 뒤의
// `ctrl+c` 로 편집기가 꺼지지 않는 것도 그쪽과 같다.
func (p partial) built(built action) []action {
	if p.reg == "" {
		return one(built)
	}

	// 붙이는 것은 이름을 가리지 않는다. 없는 이름은 빈 register 라 조용하다.
	switch a := built.(type) {
	case actionPasteAfter:
		a.reg = p.reg

		return one(a)
	case actionPasteBefore:
		a.reg = p.reg

		return one(a)
	}

	// 담는 것은 문자 이름만 받는다.
	if !registerWritable(p.reg) {
		return nil
	}

	switch a := built.(type) {
	case actionDelete:
		a.reg = p.reg

		return one(a)
	case actionYank:
		a.reg = p.reg

		return one(a)
	case actionChange:
		a.reg = p.reg

		return one(a)
	}

	return nil
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
	case ">":
		return actionIndent{motion: mo, count: n, direction: indentRight}
	case "<":
		return actionIndent{motion: mo, count: n, direction: indentLeft}
	case "=":
		return actionReindent{motion: mo, count: n}
	}

	return nil
}

// showcmd 는 이미 정해진 부분을 화면에 찍는 글자다. 상태가 자기 몫을 뒤에 잇는다.
//
// register 이름이 맨 앞이다. `3"1p` 처럼 숫자를 먼저 친 경우에도 `"13` 으로 보이는데,
// 친 순서와 어긋나는 대신 「어느 register 인가」가 늘 같은 자리에 있다.
func (p partial) showcmd() string {
	return registerString(p.reg) + countString(p.opCount) + p.op
}

// startOperator 는 operator 키를 먹었을 때의 다음 상태다. count 는 그 앞에 모아둔 숫자다.
//
// 이미 operator 가 있는데 다른 것을 또 치면(`dy`) 앞의 것을 무르고 새로 연다. vim 이 그렇다 —
// 잘못된 motion 에서 바로 무르고 다음 키를 동작으로 받는다(tmux 로 vim 9.1 확인).
// 무르는 것이므로 모아둔 숫자도 앞 동작의 것이라 같이 버린다.
func (p partial) startOperator(op string, count int) normalState {
	// register 이름은 무르지 않는다. `"1dy` 도 `"1` 을 들고 있어야 그 뒤가 규칙대로 버려진다.
	if p.op != "" {
		return normalStart{building: partial{op: op, reg: p.reg}}
	}

	return normalStart{building: partial{op: op, opCount: count, reg: p.reg}}
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
	case "v":
		return actionVisualStart{}
	case "V":
		return actionVisualStart{linewise: true}
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
	case "ctrl+d":
		return actionPage{direction: pageDown, span: pageHalf, count: count}
	case "ctrl+u":
		return actionPage{direction: pageUp, span: pageHalf, count: count}
	case "ctrl+f":
		return actionPage{direction: pageDown, span: pageFull, count: count}
	case "ctrl+b":
		return actionPage{direction: pageUp, span: pageFull, count: count}
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
	case leaderKey + "g":
		switch key {
		case "d":
			return actionGotoDefinition{}
		}
	}

	return nil
}

// leaderKey 는 vim 의 `<Leader>` 기본값이다. 이 키로 시작하는 조합은 vim 이 비워 둔 자리라
// 우리가 붙이는 것과 부딪히지 않는다.
const leaderKey = "\\"

// leaderWon 은 한글 입력 상태에서 같은 자리의 키가 내는 글자다.
//
// 한글 자판에서 그 키는 `\` 가 아니라 원화 기호를 낸다. `ctrl` 조합은 터미널에게 PC-101
// 자리를 물어서 풀었지만(ADR-0014) 이것은 modifier 가 없어서 그 길이 없다 — 글자를 보고
// 아는 수밖에 없다.
//
// 자모가 아니라서 한글 되돌리기(expandHangul) 를 지나오지 않는다. 그래서 여기서 받는다.
// 다른 쓸모가 없는 글자라 부딪힐 것도 없다.
const leaderWon = "₩"

// expectsMoreKeys 는 그 접두 키 뒤에 키가 **또** 붙어야 하는지다.
//
// 접두 키가 두 키를 넘는 것은 leader 뿐이다 — `\gd` 는 세 키다. `\g` 까지 온 것만 여기서
// 참이고, 짝이 없는 `\x` 는 거짓이라 그 자리에서 아무 일도 없이 끝난다. 접두 키를 잘못
// 짚었을 때 다음 키까지 삼키지 않는다.
func expectsMoreKeys(prefix string) bool {
	return prefix == leaderKey+"g"
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
	case leaderKey, leaderWon:
		// leader 는 뒤에 둘이 더 붙는다(`\gd`) — 몇 개가 남았는지는 normalPending 이 안다.
		//
		// 한글 자판의 원화 기호도 여기서 leader 가 된다. **문 앞에서 `\` 로 맞춰 둔다** —
		// 그러면 접두 키를 쌓는 자리와 showcmd 가 어느 글자로 들어왔는지 몰라도 된다.
		return nil, normalPending{building: s.building, prefix: leaderKey}
	case "r":
		// 뒤에 바꿔 넣을 글자 한 개가 붙는다.
		return nil, normalReplace{building: s.building}
	case "\"":
		// 뒤에 register 이름 한 개가 붙는다.
		return nil, normalRegister{building: s.building}
	case "d", "y", "c", ">", "<", "=":
		// operator 를 두 번 치면 줄 단위다(`dd` `>>` `==`).
		if s.building.op == key {
			return s.building.built(s.building.operate(motionWholeLines{}, 0)), normalStart{}
		}

		return nil, s.building.startOperator(key, 0)
	}

	return s.building.built(s.building.resolve(key, 0)), normalStart{}
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
	case "\"":
		// `3"1p` 다. 모아둔 숫자를 들고 이름을 기다린다.
		return nil, normalRegister{building: s.building, count: s.count}
	case "d", "y", "c", ">", "<", "=":
		// `3dd` 는 세 줄이고 `d3d` 도 같다. `3>>` 도 같은 자리다.
		if s.building.op == key {
			return s.building.built(s.building.operate(motionWholeLines{}, s.count)), normalStart{}
		}

		return nil, s.building.startOperator(key, s.count)
	}

	if built := s.building.resolve(key, s.count); built != nil {
		return s.building.built(built), normalStart{}
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

	// 접두 키가 자라는 중이다. `\` 뒤의 `g` 가 여기로 와서 `\g` 가 되고, 뜻은 그다음 키가 정한다.
	if next := s.prefix + key; expectsMoreKeys(next) {
		return nil, normalPending{building: s.building, prefix: next, count: s.count}
	}

	if mo, ok := prefixMotion(s.prefix, key); ok {
		return s.building.built(s.building.apply(mo, s.count)), normalStart{}
	}

	// motion 이 아닌 조합은 operator 뒤에 올 수 없다. `dgt` 는 아무것도 아니다.
	if s.building.op != "" {
		return nil, normalStart{}
	}

	return s.building.built(prefixAction(s.prefix, key)), normalStart{}
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

	return s.building.built(actionReplaceChar{key: key, count: s.count}), normalStart{}
}

func (s normalReplace) showcmd() string {
	return s.building.showcmd() + countString(s.count) + "r"
}

// normalRegister 는 `"` 를 먹고 register 이름 한 개를 기다리는 상태다(ADR-0058).
//
// **한글을 되돌린다.** `r` 뒤의 글자를 그대로 받는 normalReplace 와 반대다 — 이름은 파일에
// 들어갈 글자가 아니라 키라서, 문자 register 를 넣을 때 `ㅁ` 이 `a` 여야 한다(ADR-0008).
// 지금은 숫자만 받으므로 되돌려도 달라지는 것이 없지만, 그때 이 자리를 다시 볼 이유를 없앤다.
//
// **글자 하나면 무엇이든 이름으로 받는다.** 숫자인지는 여기서 보지 않는다 — 아직 없는
// 이름은 빈 register 라서 붙여넣기가 조용히 아무 일도 하지 않는다(register.go 의
// registerNamed). 이름 자리에서 가려내면 `"ap` 가 이름을 무르고 뒤의 `p` 만 남아 무명을
// 붙이는데, 그것은 부탁하지 않은 것을 붙이는 것이다.
//
// 글자 하나가 아닌 키(`esc` `ctrl+c`) 는 이름이 될 수 없어 무른다. 접두 키를 잘못 짚은 것을
// 무르는 것과 같다(normalPending).
type normalRegister struct {
	building partial
	count    int
}

func (s normalRegister) press(key string) ([]action, normalState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		// `ㅘ` 처럼 둘로 풀리는 키다. 이름 한 개 자리에 둘을 넣을 수 없다.
		return nil, normalStart{}
	}
	key = keys[0]

	if utf8.RuneCountInString(key) != 1 {
		return nil, normalStart{}
	}

	s.building.reg = key

	// 모아둔 숫자를 지키려면 그 상태로 돌아가야 한다. `3"1p` 의 `3` 이 여기 있다.
	if s.count > 0 {
		return nil, normalCount{building: s.building, count: s.count}
	}

	return nil, normalStart{building: s.building}
}

func (s normalRegister) showcmd() string {
	return s.building.showcmd() + countString(s.count) + `"`
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

// registerString 은 showcmd 에 붙일 register 이름이다. 무명이면 빈 값이다.
func registerString(reg string) string {
	if reg == "" {
		return ""
	}

	return `"` + reg
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
