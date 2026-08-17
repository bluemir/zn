package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pressAll 은 이미 풀린 키를 차례로 먹이고 마지막에 완성된 명령과 남은 상태를 돌려준다.
// 중간에 명령이 완성되면 뒤 키는 새 명령을 만든다. 마지막 것만 본다.
//
// 이미 풀린 키만 먹이므로 press 가 돌려주는 목록은 길이가 0 아니면 1 이다.
// 한글을 풀어 목록이 여럿이 되는 쪽은 따로 본다(TestNormalKeyParserHangul).
func pressAll(keys ...string) (normalKey, normalState) {
	var state normalState = normalStart{}

	key := normalKey{}
	for _, k := range keys {
		var commands []normalKey
		commands, state = state.press(k)

		// 그 키가 명령을 완성하지 못했으면 빈 것으로 되돌린다. press 가 목록이 되기 전과 같은 뜻이다.
		key = normalKey{}
		if len(commands) > 0 {
			key = commands[len(commands)-1]
		}
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

		// operator 는 뒤에 붙은 motion 과 한 이름이 된다. 접두 키와 같은 모양이다.
		{name: "operator 를 두 번 치면 줄 단위", keys: []string{"d", "d"}, want: normalKey{name: "d d"}},
		{name: "operator 와 motion", keys: []string{"d", "w"}, want: normalKey{name: "d w"}},
		{name: "operator 와 접두 키 motion", keys: []string{"d", "g", "g"}, want: normalKey{name: "d g g"}},
		{name: "operator 앞의 숫자", keys: []string{"3", "d", "d"}, want: normalKey{name: "d d", count: 3}},
		{name: "motion 앞의 숫자", keys: []string{"d", "3", "w"}, want: normalKey{name: "d w", count: 3}},
		{name: "숫자 뒤에 operator 를 되풀이", keys: []string{"d", "3", "d"}, want: normalKey{name: "d d", count: 3}},
		{name: "숫자 둘은 곱한다", keys: []string{"3", "d", "2", "w"}, want: normalKey{name: "d w", count: 6}},
		{name: "숫자가 접두 키 motion 까지 실려 간다", keys: []string{"1", "0", "d", "g", "g"}, want: normalKey{name: "d g g", count: 10}},
		{name: "operator 뒤 숫자 없는 G", keys: []string{"d", "G"}, want: normalKey{name: "d G"}},
		{name: "operator 뒤 G 의 숫자는 줄 번호", keys: []string{"d", "2", "G"}, want: normalKey{name: "d G", count: 2}},
		{name: "짝 없는 operator 조합도 이름은 완성된다", keys: []string{"d", "i"}, want: normalKey{name: "d i"}},
		{name: "operator 가 esc 를 삼킨다", keys: []string{"d", "esc"}, want: normalKey{name: "d esc"}},
		{name: "x 는 count 를 받는다", keys: []string{"3", "x"}, want: normalKey{name: "x", count: 3}},
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
		{name: "operator 뒤", keys: []string{"d"}},
		{name: "operator 뒤 숫자를 모으는 중", keys: []string{"d", "3"}},
		{name: "operator 뒤 접두 키", keys: []string{"d", "g"}},
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
		{name: "operator", keys: []string{"d"}, want: "d"},
		{name: "operator 앞뒤의 숫자", keys: []string{"3", "d", "2"}, want: "3d2"},
		{name: "operator 뒤 접두 키", keys: []string{"d", "g"}, want: "dg"},
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

// 한글로 온 키는 파서가 받아서 푼다. 키 하나가 명령 여럿이 된다(ADR-0008).
//
// 계약은 `press` 와 `showcmd` 둘뿐이라 밖에서 풀린 키를 따로 먹일 길이 없다.
// 풀지 말지는 상태가 정한다 — `normalReplace` 만 그대로 받는다(ADR-0018).
func TestNormalKeyParserHangul(t *testing.T) {
	names := func(commands []normalKey) []string {
		out := []string{}
		for _, c := range commands {
			out = append(out, c.name)
		}

		return out
	}

	t.Run("겹모음은 명령 둘이다", func(t *testing.T) {
		// 두벌식에서 hjkl 이 모두 모음 자리라 이동 키를 이어 누르면 한 음절로 합쳐진다.
		commands, state := normalStart{}.press("ㅘ")

		assert.Equal(t, []string{"h", "k"}, names(commands))
		assert.Equal(t, normalStart{}, state)
	})

	t.Run("음절은 초성·중성·종성으로 풀린다", func(t *testing.T) {
		// `한` 은 g k s 이고 앞의 둘이 접두 키 조합 `g k` 로 묶인다.
		commands, _ := normalStart{}.press("한")

		assert.Equal(t, []string{"g k", "s"}, names(commands))
	})

	t.Run("자모 하나는 영문 키 하나다", func(t *testing.T) {
		// 풀어도 개수가 1 이라 「여럿이면 되먹인다」 로만 걸러지지 않는다. 글자가 바뀌는 것이 요점이다.
		commands, _ := normalStart{}.press("ㅁ")

		assert.Equal(t, []string{"a"}, names(commands))
	})

	t.Run("기다리는 중이면 빈 목록이다", func(t *testing.T) {
		// `ㅎ` 은 `g` 라 접두 키가 된다.
		commands, state := normalStart{}.press("ㅎ")

		assert.Empty(t, commands)
		assert.Equal(t, normalPending{prefix: "g"}, state)
	})

	t.Run("r 뒤의 한 키는 풀지 않는다", func(t *testing.T) {
		commands, state := normalReplace{}.press("한")

		assert.Equal(t, []string{"r 한"}, names(commands))
		assert.Equal(t, normalStart{}, state)
	})

	t.Run("풀린 조각은 다시 풀리지 않는다", func(t *testing.T) {
		// pressExpanded 가 조각을 press 로 되먹이면서도 한 겹에서 끝나는 근거다(expandHangul).
		for _, key := range []string{"한", "값", "ctrl+ㅔ"} {
			for _, piece := range expandHangul(key) {
				assert.Equal(t, []string{piece}, expandHangul(piece), "%q 의 조각 %q", key, piece)
			}
		}
	})
}
