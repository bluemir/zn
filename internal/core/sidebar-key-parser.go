package core

// sidebarState 는 트리가 키를 받아가며 옮겨 다니는 상태다.
//
// normal mode 의 normalState 와 같은 모양이지만 표를 나눠 가진다(normal-key-parser.go).
// normalState 를 그대로 쓸 수 없는 것은 normalStart 가 `d`·`y`·`c` 를 operator 로, 숫자를
// count 로 먹기 때문이다 — 트리에서 `d` 가 다음 키를 삼키면 안 된다(ADR-0005, ADR-0006).
//
// 지금 기다리는 모양은 접두 키 하나(`ctrl+w`) 뿐이다. 상태 기계로 둔 것은 트리에서 파일을
// 만들고 지우고 이름을 바꾸는 것이 들어올 자리이기 때문이다 — 이름을 Enter 까지 모으는 상태와
// 지우기 확인이 붙으면 필드 하나로는 버티지 못한다(docs/tasks.md).
type sidebarState interface {
	// press 는 키 하나를 먹여 완성된 명령들을 준다. 한글로 온 키를 그대로 받는다.
	// normalState 의 것과 같은 자리다(normal-key-parser.go).
	//
	// 이름만 주고 숫자를 주지 않아서 normalKey 같은 struct 가 아니라 string 이다.
	// 트리에는 아직 숫자 접두가 없다 — `5j` 를 넣을 때 struct 가 된다.
	press(key string) ([]string, sidebarState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string

	// expand 와 step 은 상태끼리 쓰는 것이다.

	// expand 는 키 하나를 이 상태가 읽을 키 나열로 편다.
	// 지금은 모든 상태가 한글을 푼다. 파일 이름을 모으는 상태가 들어오면 그것만 그대로
	// 하나를 준다 — 한글 파일명이 두벌식으로 풀리면 안 된다(docs/tasks.md).
	expand(key string) []string

	// step 은 이미 풀린 키 하나를 먹는다. 명령이 완성되지 않았으면 빈 이름을 준다.
	step(key string) (string, sidebarState)
}

// pressExpandedSidebar 는 상태가 편 키들을 차례로 step 에 먹인다. normal 쪽 pressExpanded 와 같다.
func pressExpandedSidebar(state sidebarState, key string) ([]string, sidebarState) {
	keys := state.expand(key)

	names := make([]string, 0, len(keys))
	for _, k := range keys {
		name, next := state.step(k)
		state = next

		// 아직 다음 키를 기다리는 중이다.
		if name == "" {
			continue
		}

		names = append(names, name)
	}

	return names, state
}

// sidebarStart 는 아무것도 먹지 않은 처음이다.
type sidebarStart struct{}

func (s sidebarStart) step(key string) (string, sidebarState) {
	// 뒤에 키가 하나 더 붙는다. 그때까지 화면은 showcmd 만 바뀐다.
	if key == "ctrl+w" {
		return "", sidebarPending{prefix: key}
	}

	return key, sidebarStart{}
}

func (s sidebarStart) press(key string) ([]string, sidebarState) {
	return pressExpandedSidebar(s, key)
}

func (s sidebarStart) showcmd() string { return "" }

func (s sidebarStart) expand(key string) []string { return expandHangul(key) }

// sidebarPending 은 `ctrl+w` 처럼 뒤에 키가 하나 더 붙는 접두 키를 먹은 뒤다.
//
// 다음 키가 무엇이든 이름이 완성된다. 짝이 없는 조합은 실행하는 쪽이 모르는 이름이라
// 아무 일도 하지 않는다. `esc` 와 `ctrl+c` 도 여기로 와서 버려진다 — 접두 키를 무르는 것이지
// 트리를 나가거나 편집기를 끄는 것이 아니다. normalPending 과 같은 규칙이다.
type sidebarPending struct {
	prefix string
}

func (s sidebarPending) step(key string) (string, sidebarState) {
	return s.prefix + " " + key, sidebarStart{}
}

func (s sidebarPending) press(key string) ([]string, sidebarState) {
	return pressExpandedSidebar(s, key)
}

func (s sidebarPending) showcmd() string { return s.prefix }

func (s sidebarPending) expand(key string) []string { return expandHangul(key) }
