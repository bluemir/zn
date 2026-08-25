package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pressAll 은 이미 풀린 키를 차례로 먹이고 마지막에 완성된 동작과 남은 상태를 돌려준다.
// 중간에 동작이 완성되면 뒤 키는 새 동작을 만든다. 마지막 것만 본다.
//
// 이미 풀린 키만 먹이므로 press 가 돌려주는 목록은 길이가 0 아니면 1 이다.
// 한글을 풀어 목록이 여럿이 되는 쪽은 따로 본다(TestNormalKeyParserHangul).
func pressAll(keys ...string) (action, normalState) {
	var state normalState = normalStart{}

	var built action
	for _, k := range keys {
		var actions []action
		actions, state = state.press(k)

		// 그 키가 동작을 완성하지 못했으면 nil 이다.
		built = nil
		if len(actions) > 0 {
			built = actions[len(actions)-1]
		}
	}

	return built, state
}

// 파서는 이름이 아니라 동작을 만든다. 동작이 자기가 어떻게 실행되는지 안다(ADR-0034).
func TestNormalKeyParser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: "키 하나", keys: []string{"h"}, want: actionMove{motion: motionLeft{}}},
		{name: "count 한 자리", keys: []string{"3", "j"}, want: actionMove{motion: motionLineDown{}, count: 3}},
		{name: "count 두 자리", keys: []string{"1", "0", "j"}, want: actionMove{motion: motionLineDown{}, count: 10}},
		{name: "count 세 자리", keys: []string{"1", "2", "3", "k"}, want: actionMove{motion: motionLineUp{}, count: 123}},
		{name: "hjkl 넷 다 count 를 받는다", keys: []string{"5", "l"}, want: actionMove{motion: motionRight{}, count: 5}},
		{name: "단어 이동도 count 를 받는다", keys: []string{"3", "w"}, want: actionMove{motion: motionWordForward{kind: smallWord}, count: 3}},
		{name: "큰 단어 이동도 마찬가지", keys: []string{"2", "B"}, want: actionMove{motion: motionWordBack{kind: bigWord}, count: 2}},
		{name: "$ 는 줄 수를 받는다", keys: []string{"3", "$"}, want: actionMove{motion: motionLineEnd{}, count: 3}},
		{name: "G 는 줄 번호를 받는다", keys: []string{"4", "2", "G"}, want: actionMove{motion: motionToLastLine{}, count: 42}},

		// `0` 은 vim 에서 줄 시작으로 가는 키라 count 의 첫 자리가 될 수 없다.
		{name: "`0` 하나는 count 가 아니다", keys: []string{"0"}, want: actionMove{motion: motionLineStart{}}},
		{name: "`0` 뒤의 이동 키는 count 없이 움직인다", keys: []string{"0", "j"}, want: actionMove{motion: motionLineDown{}}},
		{name: "`0` 은 둘째 자리부터 숫자다", keys: []string{"2", "0", "k"}, want: actionMove{motion: motionLineUp{}, count: 20}},

		{name: "접두 키", keys: []string{"g", "t"}, want: actionNextTab{}},
		{name: "접두 키 대문자", keys: []string{"g", "T"}, want: actionPrevTab{}},
		{name: "window 접두 키", keys: []string{"ctrl+w", "ctrl+w"}, want: actionFocusTree{}},
		{name: "ctrl+w w 도 같다", keys: []string{"ctrl+w", "w"}, want: actionFocusTree{}},

		// 짝이 없는 조합은 동작이 되지 않는다. 예전에는 이름만 만들어 두고 실행하는 쪽이 버렸다.
		{name: "짝 없는 조합", keys: []string{"g", "x"}, want: nil},
		{name: "접두 키가 esc 를 삼킨다", keys: []string{"g", "esc"}, want: nil},
		{name: "접두 키가 ctrl+c 를 삼킨다", keys: []string{"g", "ctrl+c"}, want: nil},

		// 숫자를 쓰지 않는 동작은 그냥 무시한다. 걸러낼 표가 따로 없다.
		{name: "count 를 받지 않는 키", keys: []string{"3", "i"}, want: actionInsert{}},
		// 접두 키는 숫자를 들고 간다. `10gg` 가 10 번째 줄이라 접두 키 앞의 숫자가 살아 있어야 한다.
		{name: "접두 키가 숫자를 들고 간다", keys: []string{"1", "0", "g", "g"}, want: actionMove{motion: motionToFirstLine{}, count: 10}},
		{name: "숫자를 쓰지 않는 조합은 그냥 무시한다", keys: []string{"3", "g", "t"}, want: actionNextTab{}},
		{name: "count 뒤의 esc 는 아무것도 아니다", keys: []string{"3", "esc"}, want: nil},

		// 동작이 끝나면 처음 상태로 돌아간다. 앞의 count 가 다음 키에 남으면 안 된다.
		{name: "count 는 동작 하나에만 붙는다", keys: []string{"3", "j", "j"}, want: actionMove{motion: motionLineDown{}}},

		// operator 는 motion 을 감싼 동작이 된다.
		{name: "operator 를 두 번 치면 줄 단위", keys: []string{"d", "d"}, want: actionDelete{motion: motionWholeLines{}}},
		{name: "operator 와 motion", keys: []string{"d", "w"}, want: actionDelete{motion: motionWordForward{kind: smallWord}}},
		{name: "operator 와 접두 키 motion", keys: []string{"d", "g", "g"}, want: actionDelete{motion: motionToFirstLine{}}},
		{name: "operator 앞의 숫자", keys: []string{"3", "d", "d"}, want: actionDelete{motion: motionWholeLines{}, count: 3}},
		{name: "motion 앞의 숫자", keys: []string{"d", "3", "w"}, want: actionDelete{motion: motionWordForward{kind: smallWord}, count: 3}},
		{name: "숫자 뒤에 operator 를 되풀이", keys: []string{"d", "3", "d"}, want: actionDelete{motion: motionWholeLines{}, count: 3}},
		{name: "숫자 둘은 곱한다", keys: []string{"3", "d", "2", "w"}, want: actionDelete{motion: motionWordForward{kind: smallWord}, count: 6}},
		{name: "숫자가 접두 키 motion 까지 실려 간다", keys: []string{"1", "0", "d", "g", "g"}, want: actionDelete{motion: motionToFirstLine{}, count: 10}},
		{name: "operator 뒤 숫자 없는 G", keys: []string{"d", "G"}, want: actionDelete{motion: motionToLastLine{}}},
		{name: "operator 뒤 G 의 숫자는 줄 번호", keys: []string{"d", "2", "G"}, want: actionDelete{motion: motionToLastLine{}, count: 2}},
		{name: "yank 도 같은 자리를 쓴다", keys: []string{"y", "w"}, want: actionYank{motion: motionWordForward{kind: smallWord}}},

		// motion 이 아닌 키는 operator 뒤에 올 수 없다.
		{name: "operator 뒤의 홀로 서는 동작", keys: []string{"d", "i"}, want: nil},
		{name: "operator 가 esc 를 삼킨다", keys: []string{"d", "esc"}, want: nil},
		{name: "operator 뒤의 접두 키 동작", keys: []string{"d", "g", "t"}, want: nil},
		{name: "operator 뒤의 r", keys: []string{"d", "r", "x"}, want: nil},

		{name: "x 는 count 를 받는다", keys: []string{"3", "x"}, want: actionDelete{motion: motionRight{}, count: 3}},
		{name: "r 은 글자를 들고 온다", keys: []string{"3", "r", "z"}, want: actionReplaceChar{key: "z", count: 3}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, state := pressAll(test.keys...)

			assert.Equal(t, test.want, got)
			assert.IsType(t, normalStart{}, state, "동작이 끝나면 처음 상태로 돌아간다")
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

			assert.Nil(t, got, "아직 동작이 완성되지 않았다")
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
		{name: "동작이 끝나면 사라진다", keys: []string{"3", "j"}, want: ""},
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

	move, ok := got.(actionMove)
	require.True(t, ok, "이동 동작이다")
	assert.Equal(t, motionLineDown{}, move.motion)
	assert.Positive(t, move.count)
	assert.LessOrEqual(t, move.count, maxCount*10, "한계를 넘어서면 더 커지지 않는다")
}

// 한글로 온 키는 파서가 받아서 푼다. 키 하나가 동작 여럿이 된다(ADR-0008).
//
// 계약은 `press` 와 `showcmd` 둘뿐이라 밖에서 풀린 키를 따로 먹일 길이 없다.
// 풀지 말지는 상태가 정한다 — `normalReplace` 만 그대로 받는다(ADR-0018).
func TestNormalKeyParserHangul(t *testing.T) {
	t.Run("겹모음은 동작 둘이다", func(t *testing.T) {
		// 두벌식에서 hjkl 이 모두 모음 자리라 이동 키를 이어 누르면 한 음절로 합쳐진다.
		actions, state := normalStart{}.press("ㅘ")

		assert.Equal(t, []action{
			actionMove{motion: motionLeft{}},
			actionMove{motion: motionLineUp{}},
		}, actions)
		assert.Equal(t, normalStart{}, state)
	})

	t.Run("자모 하나는 영문 키 하나다", func(t *testing.T) {
		// 풀어도 개수가 1 이라 「여럿이면 되먹인다」 로만 걸러지지 않는다. 글자가 바뀌는 것이 요점이다.
		actions, _ := normalStart{}.press("ㅁ")

		assert.Equal(t, []action{actionAppend{}}, actions)
	})

	t.Run("짝이 없으면 동작이 하나도 안 나온다", func(t *testing.T) {
		// `한` 은 g k s 다. `g k` 도 `s` 도 짝이 없어서 아무것도 되지 않는다.
		actions, state := normalStart{}.press("한")

		assert.Empty(t, actions)
		assert.Equal(t, normalStart{}, state)
	})

	t.Run("기다리는 중이면 빈 목록이다", func(t *testing.T) {
		// `ㅎ` 은 `g` 라 접두 키가 된다.
		actions, state := normalStart{}.press("ㅎ")

		assert.Empty(t, actions)
		assert.Equal(t, normalPending{prefix: "g"}, state)
	})

	t.Run("r 뒤의 한 키는 풀지 않는다", func(t *testing.T) {
		actions, state := normalReplace{}.press("한")

		assert.Equal(t, []action{actionReplaceChar{key: "한"}}, actions)
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

// operator 는 중첩 상태가 아니라 partial 로 들고 다닌다. 그 결과로 갈리는 자리들이다.
//
// `d` 를 먹으면 building 에 적고 상태는 normalStart 로 돌아가므로, 뒤따르는 키는 이동 키를
// 그냥 친 것과 똑같은 길을 지난다 — 숫자와 접두 키 규칙이 한 벌로 유지된다(ADR-0006, ADR-0013).
func TestNormalKeyParserOperatorEdges(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		// 접두 키가 기다리는 키를 operator 가 가로채지 않는다.
		// 중첩으로 들던 때는 `key == op` 를 먼저 봐서 `dgd` 가 `dd` 가 됐다.
		{name: "dgd 는 dd 가 아니다", keys: []string{"d", "g", "d"}, want: nil},
		{name: "drd 도 마찬가지", keys: []string{"d", "r", "d"}, want: nil},

		// 다른 operator 를 이어 치면 앞의 것을 무르고 새로 연다. vim 과 같다 —
		// 중첩으로 들던 때는 `d y w` 라는 짝 없는 이름이 되어 아무 일도 하지 않았다.
		{name: "dy 는 앞의 d 를 무른다", keys: []string{"d", "y", "w"},
			want: actionYank{motion: motionWordForward{kind: smallWord}}},
		{name: "무르면 앞의 숫자도 버린다", keys: []string{"3", "d", "y", "w"},
			want: actionYank{motion: motionWordForward{kind: smallWord}}},

		// 숫자를 안 받는 키는 motion 쪽 숫자를 버린다.
		{name: "d3esc", keys: []string{"d", "3", "esc"}, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := pressAll(test.keys...)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestNormalKeyParserOperatorShowcmd(t *testing.T) {
	tests := []struct {
		keys []string
		want string
	}{
		{keys: []string{"d"}, want: "d"},
		{keys: []string{"3", "d"}, want: "3d"},
		{keys: []string{"d", "2"}, want: "d2"},
		{keys: []string{"3", "d", "2"}, want: "3d2"},
		{keys: []string{"d", "g"}, want: "dg"},
		{keys: []string{"3", "d", "2", "g"}, want: "3d2g"},
		{keys: []string{"d", "r"}, want: "dr"},
	}

	for _, test := range tests {
		t.Run(strings.Join(test.keys, ""), func(t *testing.T) {
			_, state := pressAll(test.keys...)

			assert.Equal(t, test.want, state.showcmd())
		})
	}
}

// `ctrl+o` 와 `tab` 이 되돌아오기다(ADR-0070).
//
// **`ctrl+i` 는 `tab` 으로 온다.** 터미널이 그 둘을 같은 바이트(0x09) 로 주고
// bubbletea 가 그것을 `tab` 이라 이름 붙인다(ultraviolet 의 key_table.go 를 보고 넣었다).
// normal mode 에 `tab` 이 비어 있어서 부딪히는 것은 없다.
func TestNormalKeyParserJumps(t *testing.T) {
	built, state := pressAll("ctrl+o")
	assert.Equal(t, actionJumpBack{}, built)
	assert.Equal(t, normalStart{}, state)

	built, state = pressAll("tab")
	assert.Equal(t, actionJumpForward{}, built)
	assert.Equal(t, normalStart{}, state)
}

// leader(`\`) 는 접두 키가 두 키인 유일한 자리다. `\gd` 가 정의로, `\gr` 이 사용처로
// 간다(ADR-0051, ADR-0068).
func TestNormalKeyParserLeader(t *testing.T) {
	t.Run("\\gd 는 정의로 간다", func(t *testing.T) {
		built, state := pressAll("\\", "g", "d")

		assert.Equal(t, actionGotoDefinition{}, built)
		assert.Equal(t, normalStart{}, state)
	})

	t.Run("\\gr 은 사용처로 간다", func(t *testing.T) {
		built, state := pressAll("\\", "g", "r")

		assert.Equal(t, actionGotoReferences{}, built)
		assert.Equal(t, normalStart{}, state)
	})

	t.Run("\\ 하나는 기다린다", func(t *testing.T) {
		built, state := pressAll("\\")

		assert.Nil(t, built)
		assert.Equal(t, "\\", state.showcmd())
	})

	t.Run("\\g 도 기다린다", func(t *testing.T) {
		built, state := pressAll("\\", "g")

		assert.Nil(t, built)
		assert.Equal(t, "\\g", state.showcmd())
	})

	// `\g` 뒤에 짝이 없는 키가 오면 그 자리에서 끝난다. 다음 키까지 삼키지 않는다.
	t.Run("짝 없는 조합은 그 자리에서 끝난다", func(t *testing.T) {
		built, state := pressAll("\\", "g", "z")

		assert.Nil(t, built)
		assert.Equal(t, normalStart{}, state)
	})

	// `\` 뒤에 `g` 가 아닌 키가 오면 접두 키가 자라지 않고 끝난다.
	t.Run("leader 뒤의 모르는 키", func(t *testing.T) {
		built, state := pressAll("\\", "x")

		assert.Nil(t, built)
		assert.Equal(t, normalStart{}, state)
	})

	// leader 를 잘못 짚고 이어 치는 것이 다음 동작을 먹지 않는지 본다.
	t.Run("무른 뒤의 키는 그대로 동작이 된다", func(t *testing.T) {
		built, _ := pressAll("\\", "x", "j")

		assert.Equal(t, actionMove{motion: motionLineDown{}}, built)
	})

	// operator 뒤에는 올 수 없다. `d\gd` 는 아무것도 아니다 — 정의로 가는 것은 범위가 아니다.
	t.Run("operator 뒤에는 짝이 없다", func(t *testing.T) {
		built, state := pressAll("d", "\\", "g", "d")

		assert.Nil(t, built)
		assert.Equal(t, normalStart{}, state)
	})

	// `gg` 는 그대로다. leader 를 더한 것이 한 글자 접두 키를 건드리지 않는다.
	t.Run("gg 는 그대로다", func(t *testing.T) {
		built, _ := pressAll("g", "g")

		assert.Equal(t, actionMove{motion: motionToFirstLine{}}, built)
	})
}

// 한글 자판에서 leader 자리의 키는 원화 기호를 낸다. 그것도 leader 다(ADR-0014, ADR-0051).
func TestNormalKeyParserLeaderInHangul(t *testing.T) {
	built, state := pressAll("₩", "g", "d")

	assert.Equal(t, actionGotoDefinition{}, built)
	assert.Equal(t, normalStart{}, state)

	// 문 앞에서 `\` 로 맞추므로 showcmd 도 어느 글자로 들어왔는지와 무관하다.
	_, state = pressAll("₩")
	assert.Equal(t, "\\", state.showcmd())

	// 한글 상태로 친 `\gd` 는 `g`·`d` 자리가 `ㅎ`·`ㅇ` 로 온다. 파서가 되돌린다.
	built, _ = pressAll("₩", "ㅎ", "ㅇ")
	assert.Equal(t, actionGotoDefinition{}, built)

	// `\gr` 도 같다. `r` 자리는 `ㄱ` 이다.
	built, _ = pressAll("₩", "ㅎ", "ㄱ")
	assert.Equal(t, actionGotoReferences{}, built)
}

// `"` 는 register 이름 한 개를 기다린다. 이름을 받는 동작은 붙여넣기 둘뿐이다(ADR-0058).
func TestNormalKeyParserRegister(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: `"1p`, keys: []string{`"`, "1", "p"},
			want: actionPasteAfter{reg: "1"}},
		{name: `"0P`, keys: []string{`"`, "0", "P"},
			want: actionPasteBefore{reg: "0"}},
		// 숫자는 이름 뒤에 와도 앞에 와도 같은 되풀이다. 이름은 한 글자에서 끝난다.
		{name: `"13p`, keys: []string{`"`, "1", "3", "p"},
			want: actionPasteAfter{count: 3, reg: "1"}},
		{name: `3"1p`, keys: []string{"3", `"`, "1", "p"},
			want: actionPasteAfter{count: 3, reg: "1"}},
		// 이름을 대지 않은 것은 그대로다.
		{name: "p", keys: []string{"p"}, want: actionPasteAfter{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			built, state := pressAll(test.keys...)

			assert.Equal(t, test.want, built)
			assert.Equal(t, normalStart{}, state)
		})
	}
}

// 이름을 고른 뒤 붙여넣기가 아닌 키가 오면 아무 일도 하지 않는다.
//
// 담기지 않은 것을 담은 척하지 않는다 — `"1yy` 가 무명에 담아 버리면 「`"1` 에 넣었다」로
// 읽히고, 나중에 숫자 register 에 직접 쓰는 것을 넣을 때 동작이 바뀐다(ADR-0058).
func TestNormalKeyParserRegisterSwallowsOthers(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{name: `"1yy`, keys: []string{`"`, "1", "y", "y"}},
		// `dd` 는 resolve 를 지나지 않고 operate 로 곧장 간다. 여기가 빠지기 쉬운 자리다.
		{name: `"1dd`, keys: []string{`"`, "1", "d", "d"}},
		{name: `"1dw`, keys: []string{`"`, "1", "d", "w"}},
		{name: `"1x`, keys: []string{`"`, "1", "x"}},
		{name: `"1w 는 이동도 아니다`, keys: []string{`"`, "1", "w"}},
		{name: `"1gg`, keys: []string{`"`, "1", "g", "g"}},
		{name: `"1rx`, keys: []string{`"`, "1", "r", "x"}},
		// operator 를 무르는 자리에서도 이름이 살아 있어야 뒤가 규칙대로 버려진다.
		{name: `"1dyy`, keys: []string{`"`, "1", "d", "y", "y"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			built, _ := pressAll(test.keys...)

			assert.Nil(t, built)
		})
	}
}

// 아직 없는 이름도 이름이다. 무르지 않아야 뒤의 `p` 가 무명을 붙이지 않는다 —
// 그 register 가 비어서 붙여넣기가 조용히 끝난다(register.go 의 byName).
func TestNormalKeyParserUnknownRegisterIsStillAName(t *testing.T) {
	built, state := pressAll(`"`, "a", "p")

	assert.Equal(t, actionPasteAfter{reg: "a"}, built)
	assert.Equal(t, normalStart{}, state)
}

// 글자 하나가 아닌 키는 이름이 될 수 없어 무른다. 접두 키와 같다(normalPending).
func TestNormalKeyParserRegisterAborts(t *testing.T) {
	for _, key := range []string{"esc", "ctrl+c", "enter"} {
		t.Run(key, func(t *testing.T) {
			built, state := pressAll(`"`, key)

			assert.Nil(t, built)
			assert.Equal(t, normalStart{}, state)
		})
	}
}

// 이름 자리는 한글을 되돌린다. `r` 뒤의 글자를 그대로 받는 것과 반대다(ADR-0008, ADR-0058).
func TestNormalKeyParserRegisterShowcmd(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: `"`, keys: []string{`"`}, want: `"`},
		{name: `3"`, keys: []string{"3", `"`}, want: `3"`},
		{name: `"1`, keys: []string{`"`, "1"}, want: `"1`},
		{name: `"1d`, keys: []string{`"`, "1", "d"}, want: `"1d`},
		// 숫자를 먼저 친 것도 이름이 앞에 놓인다. 어느 register 인지가 늘 같은 자리다.
		{name: `3"1`, keys: []string{"3", `"`, "1"}, want: `"13`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, state := pressAll(test.keys...)

			assert.Equal(t, test.want, state.showcmd())
		})
	}
}

// 문자 이름은 담는 동작에도 실린다. 숫자와 갈리는 자리다(ADR-0058).
func TestNormalKeyParserWritableRegister(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: `"ayy`, keys: []string{`"`, "a", "y", "y"},
			want: actionYank{motion: motionWholeLines{}, reg: "a"}},
		{name: `"add`, keys: []string{`"`, "a", "d", "d"},
			want: actionDelete{motion: motionWholeLines{}, reg: "a"}},
		{name: `"acw`, keys: []string{`"`, "a", "c", "w"},
			want: actionChange{motion: motionChangeWord{}, reg: "a"}},
		// `x` 는 `dl` 이다. 짓는 자리가 operate 가 아니라 standaloneAction 이라 이름을
		// 짓는 쪽에서 실으면 여기가 빠진다.
		{name: `"ax`, keys: []string{`"`, "a", "x"},
			want: actionDelete{motion: motionRight{}, reg: "a"}},
		{name: `"a2yw`, keys: []string{`"`, "a", "2", "y", "w"},
			want: actionYank{motion: motionWordForward{}, count: 2, reg: "a"}},
		// 대문자도 이름이다. 덮지 않고 잇는 것은 담는 자리가 정한다(register.go).
		{name: `"Ayy`, keys: []string{`"`, "A", "y", "y"},
			want: actionYank{motion: motionWholeLines{}, reg: "A"}},
		// 붙이는 것은 그대로다.
		{name: `"ap`, keys: []string{`"`, "a", "p"}, want: actionPasteAfter{reg: "a"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			built, state := pressAll(test.keys...)

			assert.Equal(t, test.want, built)
			assert.Equal(t, normalStart{}, state)
		})
	}
}

// 이름 뒤의 `esc`·`ctrl+c` 는 이름을 무른다. 무른 뒤 한 번 더 누르면 제 일을 한다.
//
// 접두 키가 하는 것과 같다(normalPending) — 손이 미끄러진 `"` 뒤의 `ctrl+c` 로 편집기가
// 꺼지지 않는다.
func TestNormalKeyParserRegisterAbortsWithEscape(t *testing.T) {
	for _, key := range []string{"esc", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			built, state := pressAll(`"`, "a", key)

			assert.Nil(t, built, "이름을 무르는 것으로 끝난다")
			require.Equal(t, normalStart{}, state, "이름이 남아 있으면 안 된다")

			// 한 번 더 누르면 제 일을 한다.
			again, _ := state.press(key)
			if key == "ctrl+c" {
				assert.Equal(t, []action{actionQuit{}}, again)
			} else {
				assert.Empty(t, again)
			}
		})
	}
}
