package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// pressAllSidebar 는 이미 풀린 키를 차례로 먹이고 마지막에 완성된 동작과 남은 상태를 돌려준다.
// normal 쪽 pressAll 과 같은 모양이다.
func pressAllSidebar(keys ...string) (sidebarAction, sidebarState) {
	var state sidebarState = sidebarStart{}

	act := sidebarAction{}
	for _, k := range keys {
		var actions []sidebarAction
		actions, state = state.press(k)

		// 그 키가 동작을 완성하지 못했으면 빈 것으로 되돌린다.
		act = sidebarAction{}
		if len(actions) > 0 {
			act = actions[len(actions)-1]
		}
	}

	return act, state
}

func TestSidebarKeyParser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want sidebarAction
	}{
		{name: "키 하나는 그대로 이름이다", keys: []string{"j"}, want: sidebarAction{name: "j"}},
		{name: "화살표도 같다", keys: []string{"down"}, want: sidebarAction{name: "down"}},
		{name: "접두 키는 다음 키를 기다린다", keys: []string{"ctrl+w"}},
		{name: "접두 키 뒤에서 이름이 완성된다", keys: []string{"ctrl+w", "ctrl+w"}, want: sidebarAction{name: "ctrl+w ctrl+w"}},
		{name: "ctrl+w w 도 같은 모양이다", keys: []string{"ctrl+w", "w"}, want: sidebarAction{name: "ctrl+w w"}},

		// `m` 도 접두 키다. 파일을 만들고 지우고 이름을 바꾸는 셋이 그 뒤에 있다(ADR-0054).
		{name: "m 은 다음 키를 기다린다", keys: []string{"m"}},
		{name: "m c 는 만들기다", keys: []string{"m", "c"}, want: sidebarAction{name: "m c"}},
		{name: "m a 도 이름이 완성된다", keys: []string{"m", "a"}, want: sidebarAction{name: "m a"}},
		{name: "m d 는 지우기다", keys: []string{"m", "d"}, want: sidebarAction{name: "m d"}},
		{name: "m m 은 이름 바꾸기다", keys: []string{"m", "m"}, want: sidebarAction{name: "m m"}},
		{name: "짝이 없는 m 조합도 이름이 된다", keys: []string{"m", "x"}, want: sidebarAction{name: "m x"}},

		// 접두 키 뒤에서는 무엇이 와도 이름이 완성된다. 짝이 없는 조합은 실행하는 쪽이 버린다.
		// 이것이 `ctrl+w esc` 가 트리를 나가지 않는 이유다.
		{name: "접두 키가 esc 를 삼킨다", keys: []string{"ctrl+w", "esc"}, want: sidebarAction{name: "ctrl+w esc"}},
		{name: "접두 키가 ctrl+c 도 삼킨다", keys: []string{"ctrl+w", "ctrl+c"}, want: sidebarAction{name: "ctrl+w ctrl+c"}},

		// 삼키는 것은 그 한 키뿐이다. 그 뒤는 다시 처음이다.
		{name: "접두 키를 무른 뒤는 다시 동작이다", keys: []string{"ctrl+w", "esc", "esc"}, want: sidebarAction{name: "esc"}},

		// normalStart 와 달리 `d`·`y`·`c` 는 operator 가 아니다. 트리에서 이것들이 다음 키를
		// 삼키면 안 된다(ADR-0005, ADR-0006).
		{name: "d 는 operator 가 아니다", keys: []string{"d"}, want: sidebarAction{name: "d"}},
		{name: "d 뒤의 키를 삼키지 않는다", keys: []string{"d", "j"}, want: sidebarAction{name: "j"}},
		{name: "r 도 인자를 받지 않는다", keys: []string{"r", "j"}, want: sidebarAction{name: "j"}},

		// 숫자는 이름 앞에 붙는다(ADR-0059).
		{name: "숫자만으로는 완성되지 않는다", keys: []string{"3"}},
		{name: "숫자가 이동에 붙는다", keys: []string{"3", "j"}, want: sidebarAction{name: "j", count: 3}},
		{name: "여러 자리도 모인다", keys: []string{"2", "0", "j"}, want: sidebarAction{name: "j", count: 20}},
		{name: "0 은 첫 자리가 아니다", keys: []string{"0"}, want: sidebarAction{name: "0"}},
		{name: "숫자 뒤의 0 은 자리를 채운다", keys: []string{"1", "0", "k"}, want: sidebarAction{name: "k", count: 10}},

		// `G` 와 `gg` 는 숫자를 되풀이가 아니라 행 번호로 받는다. 숫자를 대지 않은 것을
		// 알아야 해서 count 가 0 으로 온다(sidebarAction).
		{name: "G 는 홀로 선다", keys: []string{"G"}, want: sidebarAction{name: "G"}},
		{name: "20G 는 행 번호다", keys: []string{"2", "0", "G"}, want: sidebarAction{name: "G", count: 20}},
		{name: "g 는 다음 키를 기다린다", keys: []string{"g"}},
		{name: "gg 가 처음으로 간다", keys: []string{"g", "g"}, want: sidebarAction{name: "g g"}},
		{name: "20gg 도 행 번호다", keys: []string{"2", "0", "g", "g"}, want: sidebarAction{name: "g g", count: 20}},
		{name: "짝이 없는 g 조합도 이름이 된다", keys: []string{"g", "x"}, want: sidebarAction{name: "g x"}},

		// 파일 동작과 창 동작에 숫자는 뜻이 없다. normal 의 `3ctrl+w` 와 같이 버린다.
		{name: "숫자는 m 앞에서 버려진다", keys: []string{"3", "m", "d"}, want: sidebarAction{name: "m d"}},
		{name: "숫자는 ctrl+w 앞에서도 버려진다", keys: []string{"3", "ctrl+w", "w"}, want: sidebarAction{name: "ctrl+w w"}},

		// 한글로 쳐도 같다. `ㅎㅎ` 가 `gg` 다 — `G` 는 shift 자리가 없어 한글로 낼 수 없다(hangul-key.go).
		{name: "한글 ㅎㅎ 가 gg 다", keys: []string{"ㅎ", "ㅎ"}, want: sidebarAction{name: "g g"}},
		{name: "숫자 뒤의 한글도 같다", keys: []string{"3", "ㅓ"}, want: sidebarAction{name: "j", count: 3}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			act, _ := pressAllSidebar(test.keys...)

			assert.Equal(t, test.want, act)
		})
	}
}

// showcmd 는 동작이 완성되기를 기다리는 동안만 보인다. normal 의 것과 같은 자리다.
func TestSidebarShowcmd(t *testing.T) {
	_, state := pressAllSidebar()
	assert.Equal(t, "", state.showcmd(), "아무것도 안 먹었으면 보여줄 것이 없다")

	_, state = pressAllSidebar("ctrl+w")
	assert.Equal(t, "ctrl+w", state.showcmd(), "기다리는 동안 먹은 키가 보인다")

	_, state = pressAllSidebar("ctrl+w", "w")
	assert.Equal(t, "", state.showcmd(), "이름이 완성되면 사라진다")

	_, state = pressAllSidebar("m")
	assert.Equal(t, "m", state.showcmd(), "파일 접두 키도 같은 자리에 보인다")

	_, state = pressAllSidebar("2", "0")
	assert.Equal(t, "20", state.showcmd(), "모으던 숫자가 보인다")

	_, state = pressAllSidebar("2", "0", "g")
	assert.Equal(t, "20g", state.showcmd(), "숫자와 접두 키가 같이 보인다")

	_, state = pressAllSidebar("2", "0", "g", "g")
	assert.Equal(t, "", state.showcmd(), "완성되면 숫자도 같이 사라진다")
}
