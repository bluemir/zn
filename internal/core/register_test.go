package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registersOf 는 무명과 숫자 register 를 한꺼번에 보기 좋게 꺼낸다.
//
// 담긴 내용을 문자열로 잇는다. 링이 밀렸는지는 자리마다 무엇이 들었는지로 드러나므로
// 줄 단위였는지는 여기서 보지 않는다 — 그것을 보는 시험은 아래에 따로 있다.
func registersOf(t *testing.T, m tea.Model) map[string]string {
	t.Helper()

	// `c` 는 insert 로 넘어가므로 mode 를 가리지 않는다. bufferOf 와 같은 자리다.
	var here *editor
	switch v := m.(type) {
	case viewEditorNormal:
		here = v.editor
	case viewEditorInsert:
		here = v.editor
	case viewEditorVisual:
		here = v.editor
	default:
		require.Fail(t, "register 를 볼 수 없는 mode 다")
	}

	out := map[string]string{}
	for _, name := range registerNames {
		if reg := here.registers.byName(name); reg.filled() {
			out[registerLabel(name)] = previewOf(reg)
		}
	}

	return out
}

// `y` 는 무명과 `"0` 에 담는다. 숫자 링은 밀지 않는다.
func TestYankFillsUnnamedAndZero(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	after := send(m, "y", "y")

	assert.Equal(t, map[string]string{
		`""`: "foo bar⏎",
		`"0`: "foo bar⏎",
	}, registersOf(t, after))
}

// 지우기는 무명과 `"1` 에 담고 링을 한 칸 밀어낸다. `"0` 은 건드리지 않는다.
func TestDeletePushesRing(t *testing.T) {
	m := newTestEditor("one\ntwo\nthree\nfour", 80, 20)

	// 복사를 먼저 해서 `"0` 이 지우기에 밀리지 않는 것을 같이 본다.
	after := send(m, "y", "y", "d", "d", "d", "d", "d", "d")

	assert.Equal(t, map[string]string{
		`""`: "three⏎",
		`"0`: "one⏎",
		`"1`: "three⏎",
		`"2`: "two⏎",
		`"3`: "one⏎",
	}, registersOf(t, after))
}

// **`x` 도 링을 민다.** vim 은 한 줄 안에서 지운 것을 소삭제 register `"-` 로 보내고 숫자
// 링을 건드리지 않는데, 여기는 규칙을 하나로 두었다(ADR-0058). 이 시험이 그 결정을 붙잡는다.
func TestSmallDeletePushesRingUnlikeVim(t *testing.T) {
	m := newTestEditor("abc", 80, 20)

	after := send(m, "x", "x", "x")

	assert.Equal(t, map[string]string{
		`""`: "c",
		`"1`: "c",
		`"2`: "b",
		`"3`: "a",
	}, registersOf(t, after))
}

// `"9` 를 넘어간 것은 떨어져 나간다.
func TestRingDropsPastNine(t *testing.T) {
	m := newTestEditor("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk", 80, 20)

	// 열한 번 지우면 첫 두 개(`a` `b`) 가 링에서 밀려 나간다.
	keys := []string{}
	for range 11 {
		keys = append(keys, "d", "d")
	}

	assert.Equal(t, map[string]string{
		`""`: "k⏎",
		`"1`: "k⏎",
		`"2`: "j⏎",
		`"3`: "i⏎",
		`"4`: "h⏎",
		`"5`: "g⏎",
		`"6`: "f⏎",
		`"7`: "e⏎",
		`"8`: "d⏎",
		`"9`: "c⏎",
	}, registersOf(t, send(m, keys...)))
}

// 바꿀 것이 없는 `c` 는 링을 밀지 않는다. register 를 덮지 않는 것과 같은 자리다(ADR-0033).
func TestEmptyChangeKeepsRing(t *testing.T) {
	m := newTestEditor("word\n\nnext", 80, 20)

	// 첫 줄을 지워 링에 하나 넣고, 빈 줄로 내려가 `cw` 를 친다.
	after := send(m, "d", "d", "c", "w")
	require.IsType(t, viewEditorInsert{}, after, "mode 는 바뀐다")

	here, ok := after.(viewEditorInsert)
	require.True(t, ok)

	assert.Equal(t, "word⏎", previewOf(here.registers.byName("1")))
	assert.Equal(t, "word⏎", previewOf(here.registers.unnamed), "무명도 그대로다")
}

// visual 의 지우기·복사도 같은 자리에 담는다. normal 과 같은 길을 지난다.
func TestVisualStoresToRing(t *testing.T) {
	m := newTestEditor("one\ntwo\nthree", 80, 20)

	after := send(m, "V", "y", "V", "d")

	assert.Equal(t, map[string]string{
		`""`: "one⏎",
		`"0`: "one⏎",
		`"1`: "one⏎",
	}, registersOf(t, after))
}

// `:[범위]d` 와 `:[범위]y` 도 같다. 세는 법이 갈리면 `dd` 와 `:.d` 가 다른 자리에 담긴다.
func TestCommandLineStoresToRing(t *testing.T) {
	m := newTestEditor("one\ntwo\nthree", 80, 20)

	after := send(m, ":", "1", "y", "enter")
	after = send(after, ":", "2", "d", "enter")

	assert.Equal(t, map[string]string{
		`""`: "two⏎",
		`"0`: "one⏎",
		`"1`: "two⏎",
	}, registersOf(t, after))
}

// 담긴 갈래(줄 단위인지) 도 자리마다 그대로 남는다. 붙여넣기가 그것으로 갈린다(ADR-0017).
func TestRingKeepsLinewise(t *testing.T) {
	m := newTestEditor("foo bar", 80, 20)

	after := send(m, "d", "w", "y", "y")

	here, ok := after.(viewEditorNormal)
	require.True(t, ok)

	assert.False(t, here.registers.byName("1").linewise, "`dw` 는 글자 단위다")
	assert.True(t, here.registers.byName("0").linewise, "`yy` 는 줄 단위다")
}

// 이름을 대면 그 자리에 담는다. `"ayy` `"add` 가 문자 register 의 본체다(ADR-0058).
func TestNamedRegisterStores(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want map[string]string
	}{
		{name: `"ayy`, keys: []string{`"`, "a", "y", "y"},
			want: map[string]string{`""`: "one⏎", `"a`: "one⏎"}},
		{name: `"add`, keys: []string{`"`, "a", "d", "d"},
			want: map[string]string{`""`: "one⏎", `"a`: "one⏎"}},
		{name: `"ayw`, keys: []string{`"`, "a", "y", "w"},
			want: map[string]string{`""`: "one", `"a`: "one"}},
		// `x` 도 담는 동작이다. normal 의 `x` 는 `dl` 이다(ADR-0013).
		{name: `"ax`, keys: []string{`"`, "a", "x"},
			want: map[string]string{`""`: "o", `"a`: "o"}},
		{name: `"acw`, keys: []string{`"`, "a", "c", "w"},
			want: map[string]string{`""`: "one", `"a`: "one"}},
		// 이름이 다르면 자리도 다르다.
		{name: `"ayy 다음 "bdd`, keys: []string{`"`, "a", "y", "y", `"`, "b", "d", "d"},
			want: map[string]string{`""`: "one⏎", `"a`: "one⏎", `"b`: "one⏎"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after := send(newTestEditor("one\ntwo\nthree", 80, 20), test.keys...)

			assert.Equal(t, test.want, registersOf(t, after))
		})
	}
}

// **이름을 대면 숫자는 건드리지 않는다.** vim 과 같다 — 링 설명의 전제가
// 「다른 register 를 지정하지 않은 한」이다(ADR-0058).
func TestNamedRegisterLeavesNumberedAlone(t *testing.T) {
	// 무명으로 한 번 지워 `"1` 을 채우고 복사로 `"0` 을 채운 뒤, 이름을 대고 지운다.
	m := send(newTestEditor("one\ntwo\nthree\nfour", 80, 20), "y", "y", "d", "d")

	after := send(m, `"`, "a", "d", "d")

	assert.Equal(t, map[string]string{
		`""`: "two⏎",
		`"0`: "one⏎",
		`"1`: "one⏎",
		`"a`: "two⏎",
	}, registersOf(t, after))
}

// 대문자는 덮지 않고 뒤에 잇는다. 흩어진 줄을 모으는 데 쓴다(ADR-0058).
//
// 기대값은 vim 9.1 에서 같은 키를 쳐서 확인한 것이다.
func TestUppercaseRegisterAppends(t *testing.T) {
	// 낱말 뒤에 공백이 있는 자료다. `yw` 가 그 공백까지 담으므로 이음매가 눈에 보인다.
	const data = "one two\nthree four"

	tests := []struct {
		name     string
		keys     []string
		want     string
		linewise bool
	}{
		{name: "줄에 줄", keys: []string{`"`, "a", "y", "y", "j", `"`, "A", "y", "y"},
			want: "one two⏎three four⏎", linewise: true},
		// 글자에 글자는 이음매에서 한 줄로 붙는다.
		{name: "글자에 글자", keys: []string{`"`, "a", "y", "w", "j", `"`, "A", "y", "w"},
			want: "one three "},
		// 하나라도 줄 단위면 결과도 줄 단위다. 글자 조각이 자기 줄을 얻는다.
		{name: "줄에 글자", keys: []string{`"`, "a", "y", "y", "j", `"`, "A", "y", "w"},
			want: "one two⏎three ⏎", linewise: true},
		{name: "글자에 줄", keys: []string{`"`, "a", "y", "w", "j", `"`, "A", "y", "y"},
			want: "one ⏎three four⏎", linewise: true},
		// 빈 자리에 덧붙이면 그것이 첫 내용이다.
		{name: "빈 자리에", keys: []string{`"`, "A", "y", "y"}, want: "one two⏎", linewise: true},
		// 지우기도 덧붙인다.
		{name: "지운 것을 잇는다", keys: []string{`"`, "a", "d", "d", `"`, "A", "d", "d"},
			want: "one two⏎three four⏎", linewise: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after := send(newTestEditor(data, 80, 20), test.keys...)

			here, ok := after.(viewEditorNormal)
			require.True(t, ok)

			assert.Equal(t, test.want, previewOf(here.registers.byName("a")))
			assert.Equal(t, test.linewise, here.registers.byName("a").linewise)
		})
	}
}

// 덧붙일 때 buffer 의 줄을 제자리에서 늘리지 않는다.
//
// register 는 buffer 의 줄을 그대로 가리키므로(ADR-0017) `append` 가 그 backing array 에
// 닿으면 파일이 조용히 바뀐다. 글자 단위끼리 이을 때만 나는 일이다.
func TestAppendDoesNotTouchBuffer(t *testing.T) {
	after := send(newTestEditor("one two three", 80, 20),
		`"`, "a", "y", "w", "w", `"`, "A", "y", "w")

	assert.Equal(t, []string{"one two three"}, linesOf(bufferOf(t, after)), "파일은 그대로다")

	here, ok := after.(viewEditorNormal)
	require.True(t, ok)
	assert.Equal(t, "one two ", previewOf(here.registers.byName("a")))
}

// 대문자로 담은 것도 소문자로 붙인다. 대문자로 갈리는 것은 담을 때뿐이다.
func TestUppercaseNameIsTheSamePlace(t *testing.T) {
	m := send(newTestEditor("one\ntwo", 80, 20), `"`, "A", "y", "y")

	here, ok := m.(viewEditorNormal)
	require.True(t, ok)

	assert.Equal(t, "one⏎", previewOf(here.registers.byName("a")))
	assert.Equal(t, "one⏎", previewOf(here.registers.byName("A")), "`\"Ap` 도 같은 것을 붙인다")
}

// visual 에서도 이름을 댈 수 있다. 범위를 눈으로 고른 뒤 어디에 넣을지 고르는 손이다(ADR-0058).
func TestVisualNamedRegister(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want map[string]string
	}{
		{name: `V j "ay`, keys: []string{"V", "j", `"`, "a", "y"},
			want: map[string]string{`""`: "one⏎two⏎", `"a`: "one⏎two⏎"}},
		{name: `V "ad`, keys: []string{"V", `"`, "a", "d"},
			want: map[string]string{`""`: "one⏎", `"a`: "one⏎"}},
		{name: `v l "ay`, keys: []string{"v", "l", `"`, "a", "y"},
			want: map[string]string{`""`: "on", `"a`: "on"}},
		// visual 에서도 대문자는 덧붙인다.
		{name: `V "ay 다음 V j "Ay`, keys: []string{"V", `"`, "a", "y", "j", "V", `"`, "A", "y"},
			want: map[string]string{`""`: "two⏎", `"a`: "one⏎two⏎"}},
		// visual 의 지우기도 링을 밀지 않는다.
		{name: `V "ad 는 링을 안 민다`, keys: []string{"V", `"`, "a", "d"},
			want: map[string]string{`""`: "one⏎", `"a`: "one⏎"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after := send(newTestEditor("one\ntwo\nthree", 80, 20), test.keys...)

			assert.Equal(t, test.want, registersOf(t, after))
		})
	}
}
