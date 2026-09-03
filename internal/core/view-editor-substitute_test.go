package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// startConfirm 은 `c` 를 붙인 `:s` 를 쳐서 물어보는 자리까지 간다.
func startConfirm(t *testing.T, data, input string) tea.Model {
	t.Helper()

	m := runCommand(newTestEditor(data, 60, 8), input)
	require.IsType(t, viewEditorSubstitute{}, m, barOf(t, m)[1])

	return m
}

func TestSubstituteConfirm(t *testing.T) {
	tests := []struct {
		name string
		data string
		keys []string
		want []string
	}{
		{name: "y 는 바꾼다", data: "a\na\n", keys: []string{"y", "y"}, want: []string{"X", "X"}},
		{name: "n 은 넘긴다", data: "a\na\n", keys: []string{"n", "y"}, want: []string{"a", "X"}},
		{name: "q 는 그만둔다", data: "a\na\n", keys: []string{"y", "q"}, want: []string{"X", "a"}},
		{name: "esc 도 그만둔다", data: "a\na\n", keys: []string{"y", "esc"}, want: []string{"X", "a"}},
		{name: "a 는 남은 것 전부", data: "a\na\na\n", keys: []string{"n", "a"}, want: []string{"a", "X", "X"}},
		{name: "첫 키가 a 면 다 바꾼다", data: "a\na\n", keys: []string{"a"}, want: []string{"X", "X"}},
		{name: "먹지 않는 키는 흘린다", data: "a\n", keys: []string{"z", "j", "y"}, want: []string{"X"}},
		{name: "매칭이 없는 줄은 건너뛴다", data: "a\nzz\na\n", keys: []string{"y", "y"}, want: []string{"X", "zz", "X"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := send(startConfirm(t, test.data, "%s/a/X/c"), test.keys...)

			assert.IsType(t, viewEditorNormal{}, m, "다 답하면 normal 로 나온다")
			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

// 한 줄에 자리가 여럿이면 `g` 가 그것을 차례로 묻는다.
//
// **앞의 것을 바꾸면 뒤의 자리가 밀린다.** 밀린 만큼(delta) 을 세지 않으면 엉뚱한 글자를
// 갈아끼우고, 잡은 것을 꺼낼 원본을 들고 있지 않으면 이미 바뀐 글에서 꺼내게 된다.
func TestSubstituteConfirmWithinLine(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "전부", keys: []string{"y", "y", "y"}, want: "[a]b[a]c[a]"},
		{name: "가운데만", keys: []string{"n", "y", "n"}, want: "ab[a]ca"},
		{name: "첫째만", keys: []string{"y", "n", "n"}, want: "[a]baca"},
		{name: "끝만", keys: []string{"n", "n", "y"}, want: "abac[a]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := send(startConfirm(t, "abaca\n", "%s/a/[a]/gc"), test.keys...)

			assert.Equal(t, []string{test.want}, linesOf(bufferOf(t, m)))
		})
	}
}

// 잡은 것은 그 줄에 들어설 때의 글에서 꺼낸다. 앞의 것을 바꾼 뒤에도 그대로여야 한다.
func TestSubstituteConfirmExpandsFromOriginalLine(t *testing.T) {
	m := send(startConfirm(t, "a1 a2 a3\n", `%s/a(\d)/[\1]/gc`), "y", "y", "y")

	assert.Equal(t, []string{"[1] [2] [3]"}, linesOf(bufferOf(t, m)))
}

// 물어보는 자리로 커서가 간다. 화면에 다른 색으로 칠해지는 것도 그 자리다.
func TestSubstituteConfirmMovesCursorToMatch(t *testing.T) {
	m := startConfirm(t, "zzz\nabc\n", "%s/b/X/c")

	buf := bufferOf(t, m)
	assert.Equal(t, 1, buf.Cursor.Line)
	assert.Equal(t, 1, buf.Cursor.Col)
}

// 되돌리기는 한 구간이다. 물어보며 바꾼 것 전부가 한 번의 `u` 로 돌아간다.
func TestSubstituteConfirmUndoIsOneStep(t *testing.T) {
	m := send(startConfirm(t, "a\na\na\n", "%s/a/X/c"), "y", "n", "y")
	require.Equal(t, []string{"X", "a", "X"}, linesOf(bufferOf(t, m)))

	m = send(m, "u")

	assert.Equal(t, []string{"a", "a", "a"}, linesOf(bufferOf(t, m)))
}

// 하나도 안 바꾸고 나가면 커서가 시작한 자리로 돌아온다. 훑고 다닌 것이 남을 까닭이 없다.
func TestSubstituteConfirmRestoresCursorWhenNothingChanged(t *testing.T) {
	start := newTestEditor("a\na\na\n", 60, 8)
	start.buffers[0].Cursor.Line = 2

	m := runCommand(start, "%s/a/X/c")
	require.IsType(t, viewEditorSubstitute{}, m)

	m = send(m, "n", "n", "n")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 2, bufferOf(t, m).Cursor.Line)
	assert.False(t, bufferOf(t, m).Dirty, "파일을 건드리지 않는다")
	assert.Contains(t, barOf(t, m)[1], "바꾼 것이 없습니다")
}

// 범위 안에 하나도 없으면 물어보는 자리를 열지 않는다. `q` 를 치게 할 까닭이 없다.
func TestSubstituteConfirmWithoutMatchStaysNormal(t *testing.T) {
	m := runCommand(newTestEditor("a\n", 60, 8), "%s/zzz/X/c")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "찾을 수 없음: zzz")
}

// 아래 줄이 무엇을 묻는지와 어떤 키가 듣는지를 든다.
func TestSubstituteConfirmAsks(t *testing.T) {
	m := startConfirm(t, "a\n", "%s/a/X/c")

	assert.Contains(t, barOf(t, m)[1], "바꿀까요?")
	assert.Contains(t, barOf(t, m)[1], "y 바꾸기")
	assert.Contains(t, barOf(t, m)[0], "SUBSTITUTE")
}

// 한글 입력 상태에서 온 키도 듣는다. 넷 다 두벌식 자리에 있다(ADR-0008).
func TestSubstituteConfirmTakesHangulKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []string
	}{
		{name: "ㅛ 는 y", keys: []string{"ㅛ", "ㅛ"}, want: []string{"X", "X"}},
		{name: "ㅜ 는 n", keys: []string{"ㅜ", "ㅛ"}, want: []string{"a", "X"}},
		{name: "ㅁ 는 a", keys: []string{"ㅁ"}, want: []string{"X", "X"}},
		{name: "ㅂ 는 q", keys: []string{"ㅛ", "ㅂ"}, want: []string{"X", "a"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := send(startConfirm(t, "a\na\n", "%s/a/X/c"), test.keys...)

			assert.IsType(t, viewEditorNormal{}, m)
			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

// 음절 하나가 키 여럿으로 풀린다. `요` 는 `d` 뒤에 `y` 라 앞의 조각은 흘리고 뒤의 것이 듣는다.
func TestSubstituteConfirmSpendsSyllableInOrder(t *testing.T) {
	m := send(startConfirm(t, "a\na\n", "%s/a/X/c"), "요")

	assert.IsType(t, viewEditorSubstitute{}, m, "한 자리만 답했으므로 아직 묻고 있다")
	assert.Equal(t, []string{"X", "a"}, linesOf(bufferOf(t, m)))
}

// 답하는 사이에 밖에서 파일이 바뀌어 buffer 가 통째로 갈아끼워질 수 있다. 하나도 안 바꾼
// 동안은 dirty 가 안 서서 그 길이 열려 있다(ADR-0044) — 남의 글을 고치지 않고 멈춘다.
func TestSubstituteConfirmStopsWhenBufferSwapped(t *testing.T) {
	m := startConfirm(t, "aaa\naaa\naaa\n", "%s/a/X/c")

	// 바깥 검사가 하는 일이다. 줄 수가 줄어서 시작할 때 정한 끝 줄이 남의 글을 가리킨다.
	swapped := m.(viewEditorSubstitute)
	swapped.buffers[swapped.active] = textarea.NewBuffer("test.txt", []byte("z\n"))

	next := send(swapped, "y")

	assert.IsType(t, viewEditorNormal{}, next)
	assert.Equal(t, []string{"z"}, linesOf(bufferOf(t, next)))
	assert.Contains(t, barOf(t, next)[1], "파일이 밖에서 바뀌어 치환을 멈췄습니다")
}
