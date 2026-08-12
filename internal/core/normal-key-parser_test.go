package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pressAll 은 키를 차례로 먹이고 마지막에 완성된 명령과 남은 상태를 돌려준다.
// 중간에 명령이 완성되면 뒤 키는 새 명령을 만든다. 마지막 것만 본다.
func pressAll(keys ...string) (normalKey, normalState) {
	var state normalState = normalStart{}

	key := normalKey{}
	for _, k := range keys {
		key, state = state.press(k)
	}

	return key, state
}

func TestNormalKeyParser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want normalKey
	}{
		{name: "키 하나", keys: []string{"h"}, want: normalKey{name: "h"}},
		{name: "count 한 자리", keys: []string{"3", "j"}, want: normalKey{name: "j", count: 3}},
		{name: "count 두 자리", keys: []string{"1", "0", "j"}, want: normalKey{name: "j", count: 10}},
		{name: "count 세 자리", keys: []string{"1", "2", "3", "k"}, want: normalKey{name: "k", count: 123}},
		{name: "hjkl 넷 다 count 를 받는다", keys: []string{"5", "l"}, want: normalKey{name: "l", count: 5}},
		{name: "단어 이동도 count 를 받는다", keys: []string{"3", "w"}, want: normalKey{name: "w", count: 3}},
		{name: "큰 단어 이동도 마찬가지", keys: []string{"2", "B"}, want: normalKey{name: "B", count: 2}},
		{name: "$ 는 줄 수를 받는다", keys: []string{"3", "$"}, want: normalKey{name: "$", count: 3}},
		{name: "G 는 줄 번호를 받는다", keys: []string{"4", "2", "G"}, want: normalKey{name: "G", count: 42}},

		// `0` 은 vim 에서 줄 시작으로 가는 키라 count 의 첫 자리가 될 수 없다.
		// 아직 그 기능이 없어서 이름만 완성되고 실행하는 쪽이 모르는 이름으로 버린다.
		{name: "`0` 하나는 count 가 아니다", keys: []string{"0"}, want: normalKey{name: "0"}},
		{name: "`0` 뒤의 이동 키는 count 없이 움직인다", keys: []string{"0", "j"}, want: normalKey{name: "j"}},
		{name: "`0` 은 둘째 자리부터 숫자다", keys: []string{"2", "0", "k"}, want: normalKey{name: "k", count: 20}},

		{name: "접두 키", keys: []string{"g", "t"}, want: normalKey{name: "g t"}},
		{name: "접두 키 대문자", keys: []string{"g", "T"}, want: normalKey{name: "g T"}},
		{name: "window 접두 키", keys: []string{"ctrl+w", "ctrl+w"}, want: normalKey{name: "ctrl+w ctrl+w"}},
		{name: "짝 없는 조합도 이름은 완성된다", keys: []string{"g", "x"}, want: normalKey{name: "g x"}},
		{name: "접두 키가 esc 를 삼킨다", keys: []string{"g", "esc"}, want: normalKey{name: "g esc"}},
		{name: "접두 키가 ctrl+c 를 삼킨다", keys: []string{"g", "ctrl+c"}, want: normalKey{name: "g ctrl+c"}},

		// count 는 hjkl 에만 붙는다(ADR-0006). 나머지는 숫자를 버리고 한 번만 동작한다.
		{name: "count 를 받지 않는 키", keys: []string{"3", "i"}, want: normalKey{name: "i"}},
		// 접두 키는 숫자를 들고 간다. `10gg` 가 10 번째 줄이라 접두 키 앞의 숫자가 살아 있어야 한다.
		{name: "접두 키가 숫자를 들고 간다", keys: []string{"1", "0", "g", "g"}, want: normalKey{name: "g g", count: 10}},
		{name: "숫자를 쓰지 않는 조합에도 실려 온다", keys: []string{"3", "g", "t"}, want: normalKey{name: "g t", count: 3}},
		{name: "count 뒤의 esc 는 숫자만 버린다", keys: []string{"3", "esc"}, want: normalKey{name: "esc"}},

		// 명령이 끝나면 처음 상태로 돌아간다. 앞의 count 가 다음 키에 남으면 안 된다.
		{name: "count 는 명령 하나에만 붙는다", keys: []string{"3", "j", "j"}, want: normalKey{name: "j"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, state := pressAll(test.keys...)

			assert.Equal(t, test.want, got)
			assert.IsType(t, normalStart{}, state, "명령이 끝나면 처음 상태로 돌아간다")
		})
	}
}

func TestNormalKeyParserWaits(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{name: "숫자를 모으는 중", keys: []string{"3"}},
		{name: "숫자를 여럿 모으는 중", keys: []string{"1", "0"}},
		{name: "접두 키 뒤", keys: []string{"g"}},
		{name: "window 접두 키 뒤", keys: []string{"ctrl+w"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := pressAll(test.keys...)

			assert.Empty(t, got.name, "아직 명령이 완성되지 않았다")
		})
	}
}

func TestNormalKeyParserShowcmd(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "아무것도 안 쳤으면 비어 있다", keys: nil, want: ""},
		{name: "숫자", keys: []string{"3"}, want: "3"},
		{name: "숫자 여럿", keys: []string{"1", "0"}, want: "10"},
		{name: "접두 키", keys: []string{"g"}, want: "g"},
		{name: "window 접두 키", keys: []string{"ctrl+w"}, want: "ctrl+w"},
		{name: "명령이 끝나면 사라진다", keys: []string{"3", "j"}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, state := pressAll(test.keys...)

			assert.Equal(t, test.want, state.showcmd())
		})
	}
}

// 숫자를 계속 치면 int 가 넘칠 수 있다. 한계에서 멈추고 그 뒤 자리는 버린다.
func TestNormalKeyParserCapsCount(t *testing.T) {
	keys := []string{"9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "j"}

	got, _ := pressAll(keys...)

	require.Equal(t, "j", got.name)
	assert.Positive(t, got.count)
	assert.LessOrEqual(t, got.count, maxCount*10, "한계를 넘어서면 더 커지지 않는다")
}
