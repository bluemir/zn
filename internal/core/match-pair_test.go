package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `%` 로 짝 사이를 오간다. 괄호 셋과 html 의 tag 쌍이다(ADR-0132).

// 이동 키로 그냥 친다. 여는 쪽과 닫는 쪽 어디서 쳐도 반대쪽으로 간다.
func TestMatchPairMoves(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "func f(a) {\n\tg(a)\n}\n", 60, 8)

	m = atCol(m, 6)
	m = send(m, "%")
	line, col := cursorOf(t, m)
	assert.Equal(t, [2]int{0, 8}, [2]int{line, col}, "`(` 에서 `)` 로")

	m = send(m, "%")
	line, col = cursorOf(t, m)
	assert.Equal(t, [2]int{0, 6}, [2]int{line, col}, "다시 치면 돌아온다")

	// 줄을 넘는 짝이다. `{` 에서 마지막 줄의 `}` 로 간다.
	m = atCol(m, 4)
	m = send(m, "%")
	line, col = cursorOf(t, m)
	assert.Equal(t, [2]int{2, 0}, [2]int{line, col})
}

// html 은 tag 쌍으로도 간다. `<` 부터 `>` 까지 안 아무 곳에서 친다.
func TestMatchPairMovesBetweenTags(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.html", "<div>\n  <p>a</p>\n</div>\n", 60, 8)

	m = atCol(m, 2) // tag 이름 위다
	m = send(m, "%")
	line, col := cursorOf(t, m)
	assert.Equal(t, [2]int{2, 5}, [2]int{line, col}, "`</div>` 의 마지막 글자로")

	m = send(m, "%")
	line, col = cursorOf(t, m)
	assert.Equal(t, [2]int{0, 0}, [2]int{line, col}, "다시 치면 돌아온다")
}

// 숫자를 버린다. `3%` 는 `%` 와 같다 — vim 의 「파일의 3% 자리」가 아니다.
func TestMatchPairIgnoresCount(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "f(a)\nb\nc\nd\ne\n", 60, 8)

	m = atCol(m, 1)
	m = send(m, "5", "0", "%")

	line, col := cursorOf(t, m)
	assert.Equal(t, [2]int{0, 3}, [2]int{line, col})
}

// 짝이 없으면 커서를 두고 알린다. 아무 일도 안 나면 키가 안 먹은 것으로 읽힌다.
func TestMatchPairWithoutPairNotifies(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "f(a\n", 60, 8)

	m = atCol(m, 1)
	m = send(m, "%")
	next := m.(viewEditorNormal)

	assert.Equal(t, 1, next.activeBuffer().Cursor.Col, "커서는 그대로다")
	assert.Equal(t, "짝을 찾지 못했습니다", next.notice)

	// 괄호 위가 아닌 자리도 같다. **그 줄의 오른쪽에서 괄호를 찾아 주지 않는다.**
	m = send(m, "0", "%")
	assert.Equal(t, "짝을 찾지 못했습니다", m.(viewEditorNormal).notice)
	assert.Equal(t, 0, m.(viewEditorNormal).activeBuffer().Cursor.Col)
}

// operator 뒤에 온다. 범위는 양쪽 괄호를 다 담는다(vim 의 inclusive motion).
func TestDeleteToMatchPair(t *testing.T) {
	tests := []struct {
		name string
		data string
		col  int
		keys []string
		want []string
	}{
		{name: "d% 는 괄호까지 지운다", data: "f(a)b\n", col: 1, keys: []string{"d", "%"}, want: []string{"fb"}},
		{name: "닫는 쪽에서도 같다", data: "f(a)b\n", col: 3, keys: []string{"d", "%"}, want: []string{"fb"}},
		{name: "y% 는 지우지 않는다", data: "f(a)b\n", col: 1, keys: []string{"y", "%"}, want: []string{"f(a)b"}},
		{name: "짝이 없으면 아무 일도 없다", data: "f(a\n", col: 1, keys: []string{"d", "%"}, want: []string{"f(a"}},
		{name: "여러 줄", data: "a{\nb\n}c\n", col: 1, keys: []string{"d", "%"}, want: []string{"ac"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = newTestEditorFile("a.go", tt.data, 60, 8)
			m = atCol(m, tt.col)
			m = send(m, tt.keys...)

			assert.Equal(t, tt.want, linesOf(bufferOf(t, m)))
		})
	}
}

// visual 에서는 고른 범위를 짝까지 늘린다. 지우기 전에 무엇이 사라질지 눈으로 본다.
func TestMatchPairExtendsSelection(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "f(a)b\n", 60, 8)

	m = atCol(m, 1)
	m = send(m, "v", "%")
	require.True(t, bufferOf(t, m).Selection.Active)

	m = send(m, "d")
	assert.Equal(t, []string{"fb"}, linesOf(bufferOf(t, m)), "고른 것이 괄호 둘을 다 담았다")
}

// tag 쌍도 visual 로 고른다. 여는 tag 부터 닫는 tag 끝까지다.
func TestMatchPairExtendsSelectionOverTags(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.html", "x<p>a</p>y\n", 60, 8)

	m = atCol(m, 1)
	m = send(m, "v", "%", "d")

	assert.Equal(t, []string{"xy"}, linesOf(bufferOf(t, m)))
}

// 멀리 뛰는 이동이라 되돌아오기 이력에 담는다. `ctrl+o` 로 돌아온다(ADR-0082).
func TestMatchPairRecordsJump(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "func f() {\n\tx := 1\n}\n", 60, 8)

	m = atCol(m, 9)
	m = send(m, "%")
	require.Equal(t, 2, bufferOf(t, m).Cursor.Line)

	m = send(m, "ctrl+o")
	assert.Equal(t, 0, bufferOf(t, m).Cursor.Line)
}
