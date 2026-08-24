package core

// sidebarAction 은 완성된 동작 하나다. 이름과 그 앞에 붙은 숫자다.
//
// normal mode 처럼 동작마다 type 을 두지 않는다. 저쪽은 operator 가 범위와 짝지어야 해서
// 동작이 값을 들어야 했는데(ADR-0013) 트리의 동작은 이름 하나로 끝나고, 숫자를 보는 것은
// 이동 여덟뿐이다. 그 여덟 때문에 스무 개의 type 을 만들 값이 없다(ADR-0059, ADR-0063).
type sidebarAction struct {
	name string

	// count 는 이름 앞에 붙은 숫자다. **0 은 대지 않은 것이다.**
	//
	// 1 로 메워 받지 않는 것은 `G` 가 그 둘을 가려야 하기 때문이다 — 숫자가 없으면 마지막
	// 행이고 있으면 그 행이다. normal 의 motionToLastLine 이 count 를 그대로 받는 것과 같다.
	count int
}

// sidebarState 는 트리가 키를 받아가며 옮겨 다니는 상태다.
//
// normal mode 의 normalState 와 같은 모양이지만 표를 나눠 가진다(normal-key-parser.go).
// normalState 를 그대로 쓸 수 없는 것은 normalStart 가 `d`·`y`·`c` 를 operator 로 먹기
// 때문이다 — 트리에서 `d` 가 다음 키를 삼키면 안 된다(ADR-0005, ADR-0006).
//
// 기다리는 모양은 숫자 접두와 접두 키 셋(`ctrl+w`·`m`·`g`) 이다. 이름을 모으는 것과 지우기
// 확인은 여기에 상태를 더하지 않고 mode 를 따로 뒀다 — 그 둘은 키가 동작이 아니라 글자이거나
// 예/아니오라서, 같은 표에 섞으면 한글 파일명이 두벌식 자리로 되돌려진다(ADR-0008, ADR-0054).
type sidebarState interface {
	// press 는 키 하나를 먹여 완성된 동작들을 준다. 한글로 온 키를 그대로 받는다.
	// normalState 의 것과 같은 자리이고 계약도 이 둘뿐이다(normal-key-parser.go).
	press(key string) ([]sidebarAction, sidebarState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// pressExpandedSidebar 는 풀린 키들을 차례로 먹인다. normal 쪽 pressExpanded 와 같다.
func pressExpandedSidebar(state sidebarState, keys []string) ([]sidebarAction, sidebarState) {
	actions := make([]sidebarAction, 0, len(keys))
	for _, k := range keys {
		next, state2 := state.press(k)
		state = state2

		actions = append(actions, next...)
	}

	return actions, state
}

// sidebarStart 는 아무것도 먹지 않은 처음이다.
type sidebarStart struct{}

func (s sidebarStart) press(key string) ([]sidebarAction, sidebarState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedSidebar(s, keys)
	}
	// 자모 하나는 영문 키 하나로 바뀐다 — `ㅁ` 이 `a` 다. 개수가 같아도 글자가 다르다.
	key = keys[0]

	// `0` 은 숫자의 첫 자리가 될 수 없다. normal 에서 줄 시작으로 가는 키라 비워 둔 자리이고,
	// 트리에서도 같이 비워 둔다 — 한쪽에서만 숫자가 되면 손이 자리를 두 벌로 기억한다.
	if n, ok := keyDigit(key); ok && n > 0 {
		return nil, sidebarCount{count: n}
	}

	// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 showcmd 만 바뀐다.
	//
	// `m` 은 파일을 만들고 지우는 접두 키다(`mc`·`md`). vim 의 mark 가 아니다 —
	// 트리에는 표시할 자리가 없고, NERDTree 의 파일 메뉴가 이 자리에 있다(ADR-0054).
	//
	// `g` 는 처음으로 가는 `gg` 하나뿐이지만 접두 키다. normal 과 같은 손버릇이라야 해서
	// 목록 판들이 쓰는 홀로 선 `g` 를 따르지 않았다(ADR-0059).
	if key == "ctrl+w" || key == "m" || key == "g" {
		return nil, sidebarPending{prefix: key}
	}

	return []sidebarAction{{name: key}}, sidebarStart{}
}

func (s sidebarStart) showcmd() string { return "" }

// sidebarCount 는 숫자를 모으는 중이다. `20j` 의 `20` 이 여기 쌓인다.
type sidebarCount struct {
	count int
}

func (s sidebarCount) press(key string) ([]sidebarAction, sidebarState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedSidebar(s, keys)
	}
	key = keys[0]

	if n, ok := keyDigit(key); ok {
		// 넘치지 않게만 막는다. 행 수보다 훨씬 크면 어차피 양끝에서 멈춘다(normal 의 maxCount).
		if s.count >= maxCount {
			return nil, s
		}

		return nil, sidebarCount{count: s.count*10 + n}
	}

	switch key {
	case "g":
		// 접두 키가 숫자를 들고 다음 키를 기다린다. `20gg` 는 20 번째 행이다.
		return nil, sidebarPending{prefix: key, count: s.count}
	case "m", "ctrl+w":
		// 파일 동작과 창 동작에 숫자는 뜻이 없다. 모으던 것을 버리고 접두 키만 남긴다 —
		// normal 에서 `3ctrl+w` 가 `ctrl+w` 인 것과 같은 자리다.
		return nil, sidebarPending{prefix: key}
	}

	return []sidebarAction{{name: key, count: s.count}}, sidebarStart{}
}

func (s sidebarCount) showcmd() string { return countString(s.count) }

// sidebarPending 은 `ctrl+w`·`m`·`g` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 이름이 완성된다. 짝이 없는 조합은 실행하는 쪽이 모르는 이름이라
// 아무 일도 하지 않는다. `esc` 와 `ctrl+c` 도 여기로 와서 버려진다 — 접두 키를 무르는 것이지
// 트리를 나가거나 편집기를 끄는 것이 아니다. normalPending 과 같은 규칙이다.
type sidebarPending struct {
	prefix string
	count  int
}

func (s sidebarPending) press(key string) ([]sidebarAction, sidebarState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedSidebar(s, keys)
	}
	key = keys[0]

	return []sidebarAction{{name: s.prefix + " " + key, count: s.count}}, sidebarStart{}
}

func (s sidebarPending) showcmd() string { return countString(s.count) + s.prefix }
