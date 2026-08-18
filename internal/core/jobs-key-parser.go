package core

// jobsState 는 `:jobs` 목록이 키를 받아가며 옮겨 다니는 상태다.
//
// normal·트리와 같은 모양이되 표를 나눠 가진다(normal-key-parser.go, sidebar-key-parser.go).
// 셋이 표를 나눠 갖는 이유는 같다 — normalStart 는 `d`·`y`·`c` 를 operator 로, 숫자를 count 로
// 먹어서 목록에서 `x`(취소) 앞에 친 숫자가 엉뚱하게 붙는다(ADR-0006).
//
// **지금 기다리는 상태가 없다.** 받는 키가 전부 한 개짜리(`j` `k` `x` `q` `esc` `ctrl+c`) 라
// jobsStart 하나뿐이고, 이 파서는 한글을 푸는 것 말고는 키를 그대로 이름으로 넘긴다.
// 상태 기계로 둔 것은 목록에 붙을 것들이 접두 키와 숫자를 함께 데려오기 때문이다 —
// 끝난 작업의 산출물로 들어가기, 여러 파일 검색의 적중 목록(quickfix) 이 그렇다
// (docs/tasks.md, ADR-0027). 그때 상태를 여기 더하면 viewJobs 쪽은 `run` 의 이름 하나만 늘면 된다.
type jobsState interface {
	// press 는 키 하나를 먹여 완성된 명령들을 준다. 한글로 온 키를 그대로 받는다.
	// normalState 의 것과 같은 자리이고 계약도 이 둘뿐이다(normal-key-parser.go).
	//
	// 이름만 주고 숫자를 주지 않아서 normal mode 처럼 명령 type 을 두지 않고 string 이다.
	// sidebarState 와 같은 자리이고, 숫자 접두를 넣을 때 struct 가 된다.
	press(key string) ([]string, jobsState)

	// showcmd 는 지금까지 먹은 키다. statusBar 아래 줄 오른쪽에 그대로 보인다.
	showcmd() string
}

// pressExpandedJobs 는 풀린 키들을 차례로 먹인다. normal 쪽 pressExpanded 와 같다.
func pressExpandedJobs(state jobsState, keys []string) ([]string, jobsState) {
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		next, state2 := state.press(k)
		state = state2

		names = append(names, next...)
	}

	return names, state
}

// jobsStart 는 아무것도 먹지 않은 처음이다. 지금은 이것뿐이라 풀린 키가 곧 이름이다.
type jobsStart struct{}

func (s jobsStart) press(key string) ([]string, jobsState) {
	keys := expandHangul(key)
	if len(keys) > 1 {
		return pressExpandedJobs(s, keys)
	}
	// 자모 하나는 영문 키 하나로 바뀐다 — `ㅁ` 이 `a` 다. 개수가 같아도 글자가 다르다.
	return []string{keys[0]}, jobsStart{}
}

func (s jobsStart) showcmd() string { return "" }
