package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 여기 있는 기대값은 vim 9.1(`-u NONE`) 을 실제로 재서 옮긴 것이다. 문장부호 뒤 공백만
// 다르다 — 거기서 vim 은 둘을 넣고 우리는 하나다(ADR-0087).

// joinOf 는 `J` 를 치고 남은 줄들과 커서 자리를 돌려준다.
func joinOf(t *testing.T, data string, keys ...string) (lines []string, line, col int) {
	t.Helper()

	m := send(tea.Model(newTestEditor(data, 80, 10)), keys...)
	require.IsType(t, viewEditorNormal{}, m)

	buf := bufferOf(t, m)

	return linesOf(buf), buf.cursor.Line, buf.cursor.Col
}

// 다음 줄의 들여쓰기를 걷고 공백 하나로 잇는다. 커서는 이은 자리다.
func TestJoinRemovesIndentAndAddsSpace(t *testing.T) {
	lines, line, col := joinOf(t, "foo\n    bar\nbaz\n", "J")

	assert.Equal(t, []string{"foo bar", "baz"}, lines)
	assert.Equal(t, 0, line)
	assert.Equal(t, 3, col, "넣은 공백 자리다")
}

// tab 들여쓰기도 같이 걷는다.
func TestJoinRemovesTabIndent(t *testing.T) {
	lines, _, _ := joinOf(t, "foo\n\tbar\n", "J")

	assert.Equal(t, []string{"foo bar"}, lines)
}

// 이미 공백으로 끝난 줄에는 더 넣지 않는다. 있는 공백은 그대로 둔다.
func TestJoinKeepsExistingTrailingSpace(t *testing.T) {
	lines, _, col := joinOf(t, "foo   \n    bar\n", "J")

	assert.Equal(t, []string{"foo   bar"}, lines)
	assert.Equal(t, 6, col, "이은 자리는 원래 줄 끝 다음이라 다음 줄의 첫 글자다")
}

// tab 으로 끝난 줄도 공백으로 끝난 것이다.
func TestJoinKeepsExistingTrailingTab(t *testing.T) {
	lines, _, _ := joinOf(t, "foo\t\nbar\n", "J")

	assert.Equal(t, []string{"foo\tbar"}, lines)
}

// 다음 줄이 `)` 로 시작하면 공백을 넣지 않는다. `foo(` 와 `)` 를 이으면 `foo()` 여야 한다.
func TestJoinNoSpaceBeforeCloseParen(t *testing.T) {
	lines, _, _ := joinOf(t, "foo(\n    )\n", "J")
	assert.Equal(t, []string{"foo()"}, lines)

	// `]` 는 예외가 아니다. vim 도 `)` 하나만 본다.
	lines, _, _ = joinOf(t, "foo\n    ]bar\n", "J")
	assert.Equal(t, []string{"foo ]bar"}, lines)
}

// 문장부호 뒤에도 공백 하나다. vim 기본값(joinspaces) 과 갈리는 유일한 자리다.
func TestJoinSingleSpaceAfterSentenceEnd(t *testing.T) {
	for _, mark := range []string{".", "?", "!"} {
		lines, _, _ := joinOf(t, "End"+mark+"\n    Next\n", "J")

		assert.Equal(t, []string{"End" + mark + " Next"}, lines, "문장부호: "+mark)
	}
}

// 다음 줄이 비어 있으면 줄바꿈만 걷는다. 공백을 만들지 않는다.
func TestJoinWithEmptyNextLine(t *testing.T) {
	lines, line, col := joinOf(t, "foo\n\nbaz\n", "J")

	assert.Equal(t, []string{"foo", "baz"}, lines)
	assert.Equal(t, 0, line)
	assert.Equal(t, 2, col, "이은 자리가 줄 끝을 넘어서 normal 커서 자리로 당겨진다")
}

// 다음 줄이 공백뿐이면 그 공백이 통째로 걷힌다. 이은 자리에 공백도 남지 않는다.
func TestJoinWithBlankNextLine(t *testing.T) {
	lines, _, _ := joinOf(t, "foo\n    \n", "J")

	assert.Equal(t, []string{"foo"}, lines)
}

// 지금 줄이 비어 있으면 다음 줄이 그대로 올라온다. 앞에 공백이 붙지 않는다.
func TestJoinFromEmptyLine(t *testing.T) {
	lines, _, col := joinOf(t, "\nbar\n", "J")

	assert.Equal(t, []string{"bar"}, lines)
	assert.Equal(t, 0, col)
}

// count 는 이을 줄 수다. `J`·`1J`·`2J` 가 모두 두 줄이고 `3J` 가 세 줄이다.
func TestJoinCount(t *testing.T) {
	data := "a\nb\nc\nd\ne\n"

	lines, _, col := joinOf(t, data, "3", "J")
	assert.Equal(t, []string{"a b c", "d", "e"}, lines)
	assert.Equal(t, 3, col, "커서는 마지막으로 이은 자리다")

	lines, _, _ = joinOf(t, data, "2", "J")
	assert.Equal(t, []string{"a b", "c", "d", "e"}, lines)

	lines, _, _ = joinOf(t, data, "1", "J")
	assert.Equal(t, []string{"a b", "c", "d", "e"}, lines, "`1J` 도 두 줄이다")
}

// 남은 줄이 count 보다 적으면 있는 만큼 잇는다. 오류가 아니다.
func TestJoinCountBeyondFileEnd(t *testing.T) {
	lines, _, _ := joinOf(t, "a\nb\nc", "9", "J")

	assert.Equal(t, []string{"a b c"}, lines)
}

// 마지막 줄에서는 아무 일도 없다. 파일도 커서도 그대로다.
func TestJoinOnLastLineDoesNothing(t *testing.T) {
	lines, line, col := joinOf(t, "foo\nbar", "j", "J")

	assert.Equal(t, []string{"foo", "bar"}, lines)
	assert.Equal(t, 1, line)
	assert.Equal(t, 0, col)
}

// 커서가 줄 가운데 있어도 이은 자리로 간다. 치기 전 자리는 잊는다.
func TestJoinMovesCursorFromMidLine(t *testing.T) {
	lines, _, col := joinOf(t, "hello world\n    next\n", "l", "l", "J")

	assert.Equal(t, []string{"hello world next"}, lines)
	assert.Equal(t, 11, col)
}

// `3J` 를 `u` 한 번으로 되돌린다. 친 것이 하나였으므로 무르는 것도 하나다.
func TestJoinUndoIsOneStep(t *testing.T) {
	lines, _, _ := joinOf(t, "a\nb\nc\nd\n", "3", "J", "u")

	assert.Equal(t, []string{"a", "b", "c", "d"}, lines)
}

// 이어서 친 글자와 한 구간이 되지 않는다. `u` 가 남의 편집까지 걷어가면 안 된다.
func TestJoinDoesNotMergeWithTyping(t *testing.T) {
	lines, _, _ := joinOf(t, "foo\nbar\n", "i", "X", "esc", "J", "u")

	assert.Equal(t, []string{"Xfoo", "bar"}, lines, "`J` 만 물러난다")
}
