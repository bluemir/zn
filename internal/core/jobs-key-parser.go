package core

// jobsState 는 `:jobs` 목록이 키를 받아가며 옮겨 다니는 상태다.
//
// normal·트리와 같은 모양이되 표를 나눠 가진다(normal-key-parser.go, sidebar-key-parser.go).
// 셋이 표를 나눠 갖는 이유는 같다 — normalStart 는 `d`·`y`·`c` 를 operator 로, 숫자를 count 로
// 먹어서 목록에서 `x`(취소) 앞에 친 숫자가 엉뚱하게 붙는다(ADR-0006).
//
// **지금 기다리는 상태가 없다.** 받는 키가 전부 한 개짜리(`j` `k` `x` `q` `esc` `ctrl+c`) 라
// jobsStart 하나뿐이고, 이 파서는 키를 그대로 이름으로 넘기기만 한다. 상태 기계로 둔 것은
// 목록에 붙을 것들이 접두 키와 숫자를 함께 데려오기 때문이다 — 끝난 작업의 산출물로 들어가기,
// 여러 파일 검색의 적중 목록(quickfix) 이 그렇다(docs/tasks.md, ADR-0027).
// 그때 상태를 여기 더하면 viewJobs 쪽은 `run` 의 이름 하나만 늘면 된다.
type jobsState interface {
	// press 는 키 하나를 먹여 완성된 명령들을 준다. 한글로 온 키를 그대로 받는다.
	// normalState 의 것과 같은 자리다(normal-key-parser.go).
	//
	// 이름만 주고 숫자를 주지 않아서 normalKey 같은 struct 가 아니라 string 이다.
	// sidebarState 와 같은 자리이고, 숫자 접두를 넣을 때 struct 가 된다.
	press(key string) ([]string, jobsState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string

	// expand 와 step 은 상태끼리 쓰는 것이다.
	expand(key string) []string
	step(key string) (string, jobsState)
}

// pressExpandedJobs 는 상태가 편 키들을 차례로 step 에 먹인다. normal 쪽 pressExpanded 와 같다.
func pressExpandedJobs(state jobsState, key string) ([]string, jobsState) {
	keys := state.expand(key)

	names := make([]string, 0, len(keys))
	for _, k := range keys {
		name, next := state.step(k)
		state = next

		// 아직 다음 키를 기다리는 중이다. 지금은 기다리는 상태가 없어서 여기로 오지 않는다.
		if name == "" {
			continue
		}

		names = append(names, name)
	}

	return names, state
}

// jobsStart 는 아무것도 먹지 않은 처음이다. 지금은 이것뿐이라 모든 키가 곧 이름이다.
type jobsStart struct{}

func (s jobsStart) step(key string) (string, jobsState) {
	return key, jobsStart{}
}

func (s jobsStart) press(key string) ([]string, jobsState) {
	return pressExpandedJobs(s, key)
}

func (s jobsStart) showcmd() string { return "" }

func (s jobsStart) expand(key string) []string { return expandHangul(key) }
