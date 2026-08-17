package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// pressAllSidebar 는 이미 풀린 키를 차례로 먹이고 마지막에 완성된 이름과 남은 상태를 돌려준다.
// normal 쪽 pressAll 과 같은 모양이다.
func pressAllSidebar(keys ...string) (string, sidebarState) {
	var state sidebarState = sidebarStart{}

	name := ""
	for _, k := range keys {
		var names []string
		names, state = state.press(k)

		// 그 키가 명령을 완성하지 못했으면 빈 것으로 되돌린다.
		name = ""
		if len(names) > 0 {
			name = names[len(names)-1]
		}
	}

	return name, state
}

func TestSidebarKeyParser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "키 하나는 그대로 이름이다", keys: []string{"j"}, want: "j"},
		{name: "화살표도 같다", keys: []string{"down"}, want: "down"},
		{name: "접두 키는 다음 키를 기다린다", keys: []string{"ctrl+w"}, want: ""},
		{name: "접두 키 뒤에서 이름이 완성된다", keys: []string{"ctrl+w", "ctrl+w"}, want: "ctrl+w ctrl+w"},
		{name: "ctrl+w w 도 같은 모양이다", keys: []string{"ctrl+w", "w"}, want: "ctrl+w w"},

		// 접두 키 뒤에서는 무엇이 와도 이름이 완성된다. 짝이 없는 조합은 실행하는 쪽이 버린다.
		// 이것이 `ctrl+w esc` 가 트리를 나가지 않는 이유다.
		{name: "접두 키가 esc 를 삼킨다", keys: []string{"ctrl+w", "esc"}, want: "ctrl+w esc"},
		{name: "접두 키가 ctrl+c 도 삼킨다", keys: []string{"ctrl+w", "ctrl+c"}, want: "ctrl+w ctrl+c"},

		// 삼키는 것은 그 한 키뿐이다. 그 뒤는 다시 처음이다.
		{name: "접두 키를 무른 뒤는 다시 명령이다", keys: []string{"ctrl+w", "esc", "esc"}, want: "esc"},

		// normalStart 와 달리 `d`·`y`·`c` 는 operator 가 아니고 숫자는 count 가 아니다.
		// 트리에서 이것들이 다음 키를 삼키면 안 된다(ADR-0005, ADR-0006).
		{name: "d 는 operator 가 아니다", keys: []string{"d"}, want: "d"},
		{name: "d 뒤의 키를 삼키지 않는다", keys: []string{"d", "j"}, want: "j"},
		{name: "숫자는 count 가 아니다", keys: []string{"3"}, want: "3"},
		{name: "숫자 뒤의 키를 삼키지 않는다", keys: []string{"3", "j"}, want: "j"},
		{name: "r 도 인자를 받지 않는다", keys: []string{"r", "j"}, want: "j"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, _ := pressAllSidebar(test.keys...)

			assert.Equal(t, test.want, name)
		})
	}
}

// showcmd 는 접두 키를 기다리는 동안만 보인다. normal 의 `g` 와 같은 자리다.
func TestSidebarShowcmd(t *testing.T) {
	_, state := pressAllSidebar()
	assert.Equal(t, "", state.showcmd(), "아무것도 안 먹었으면 보여줄 것이 없다")

	_, state = pressAllSidebar("ctrl+w")
	assert.Equal(t, "ctrl+w", state.showcmd(), "기다리는 동안 먹은 키가 보인다")

	_, state = pressAllSidebar("ctrl+w", "w")
	assert.Equal(t, "", state.showcmd(), "이름이 완성되면 사라진다")
}
