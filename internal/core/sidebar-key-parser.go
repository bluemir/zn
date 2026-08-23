package core

// sidebarState 는 트리가 키를 받아가며 옮겨 다니는 상태다.
//
// normal mode 의 normalState 와 같은 모양이지만 표를 나눠 가진다(normal-key-parser.go).
// normalState 를 그대로 쓸 수 없는 것은 normalStart 가 `d`·`y`·`c` 를 operator 로, 숫자를
// count 로 먹기 때문이다 — 트리에서 `d` 가 다음 키를 삼키면 안 된다(ADR-0005, ADR-0006).
//
// 기다리는 모양은 접두 키 둘(`ctrl+w`·`m`) 이다. 이름을 모으는 것과 지우기 확인은 여기에
// 상태를 더하지 않고 mode 를 따로 뒀다 — 그 둘은 키가 동작이 아니라 글자이거나 예/아니오라서,
// 같은 표에 섞으면 한글 파일명이 두벌식 자리로 되돌려진다(ADR-0008, ADR-0054).
type sidebarState interface {
	// press 는 키 하나를 먹여 완성된 동작들을 준다. 한글로 온 키를 그대로 받는다.
	// normalState 의 것과 같은 자리이고 계약도 이 둘뿐이다(normal-key-parser.go).
	//
	// 이름만 주고 숫자를 주지 않아서 normal mode 처럼 동작 type 을 두지 않고 string 이다.
	// 트리에는 아직 숫자 접두가 없다 — `5j` 를 넣을 때 struct 가 된다.
	press(key string) ([]string, sidebarState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// pressExpandedSidebar 는 풀린 키들을 차례로 먹인다. normal 쪽 pressExpanded 와 같다.
func pressExpandedSidebar(state sidebarState, keys []string) ([]string, sidebarState) {
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		next, state2 := state.press(k)
		state = state2

		names = append(names, next...)
	}

	return names, state
}

// sidebarStart 는 아무것도 먹지 않은 처음이다.
type sidebarStart struct{}

func (s sidebarStart) press(key string) ([]string, sidebarState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedSidebar(s, keys)
	}
	// 자모 하나는 영문 키 하나로 바뀐다 — `ㅁ` 이 `a` 다. 개수가 같아도 글자가 다르다.
	key = keys[0]

	// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 showcmd 만 바뀐다.
	//
	// `m` 은 파일을 만들고 지우는 접두 키다(`mc`·`md`). vim 의 mark 가 아니다 —
	// 트리에는 표시할 자리가 없고, NERDTree 의 파일 메뉴가 이 자리에 있다(ADR-0054).
	if key == "ctrl+w" || key == "m" {
		return nil, sidebarPending{prefix: key}
	}

	return []string{key}, sidebarStart{}
}

func (s sidebarStart) showcmd() string { return "" }

// sidebarPending 은 `ctrl+w`·`m` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 이름이 완성된다. 짝이 없는 조합은 실행하는 쪽이 모르는 이름이라
// 아무 일도 하지 않는다. `esc` 와 `ctrl+c` 도 여기로 와서 버려진다 — 접두 키를 무르는 것이지
// 트리를 나가거나 편집기를 끄는 것이 아니다. normalPending 과 같은 규칙이다.
type sidebarPending struct {
	prefix string
}

func (s sidebarPending) press(key string) ([]string, sidebarState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedSidebar(s, keys)
	}
	key = keys[0]

	return []string{s.prefix + " " + key}, sidebarStart{}
}

func (s sidebarPending) showcmd() string { return s.prefix }
