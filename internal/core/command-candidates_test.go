package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 고른 후보는 마지막 조각의 이름 자리만 갈아끼운다. 디렉터리 자리는 친 그대로다.
func TestApplyCandidate(t *testing.T) {
	tests := []struct {
		name string
		head string
		pick string
		want string
	}{
		{name: "지금 자리", head: "e doc", pick: "docx.md", want: "e docx.md"},
		{name: "디렉터리 안", head: "e internal/co", pick: "core/", want: "e internal/core/"},
		{name: "`~` 는 친 그대로", head: "tabnew ~/pro", pick: "projects/", want: "tabnew ~/projects/"},
		{name: "빈 조각", head: "e ", pick: "a.go", want: "e a.go"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, applyCandidate(test.head, test.pick))
		})
	}
}

// 창이 뜨면 첫 후보가 짚혀 있고, 위·아래가 그것을 옮긴다. 끝에서는 그 자리에 선다.
func TestCommandCandidateArrowsMoveSelection(t *testing.T) {
	completeDir(t, "a1", "a2", "a3")

	m := commandAfter(t, ":", "e", " ", "a", "tab")
	assert.Equal(t, 0, m.selected, "첫 후보에서 시작한다")

	m = commandAfter(t, ":", "e", " ", "a", "tab", "down", "down")
	assert.Equal(t, 2, m.selected)
	assert.Equal(t, "e a", m.input.text, "옮기기만 하고 넣지 않는다")

	m = commandAfter(t, ":", "e", " ", "a", "tab", "down", "down", "down", "up", "up", "up", "up")
	assert.Equal(t, 0, m.selected, "끝에서 더 가지 않는다")
}

// `tab` 과 `enter` 는 고른 것을 넣는다. `enter` 가 명령을 실행하지 않는다.
func TestCommandCandidatePick(t *testing.T) {
	for _, key := range []string{"tab", "enter"} {
		t.Run(key, func(t *testing.T) {
			completeDir(t, "a1", "a2", "a3")

			m := commandAfter(t, ":", "e", " ", "a", "tab", "down", key)

			assert.Equal(t, "e a2", m.input.text)
			assert.Empty(t, m.candidates, "넣으면 창이 닫힌다")
		})
	}
}

// 디렉터리를 고르면 그 안의 후보가 곧바로 뜬다. `tab` 을 한 번 더 친 것과 같다.
func TestCommandCandidatePickDirectoryOpensIt(t *testing.T) {
	completeDir(t, "docs/a.md", "docs/b.md", "docx.md")

	m := commandAfter(t, ":", "e", " ", "d", "tab", "enter")

	assert.Equal(t, "e docs/", m.input.text)
	assert.Equal(t, []string{"a.md", "b.md"}, m.candidates)
	assert.Equal(t, 0, m.selected, "새 목록은 첫 줄부터다")
}

// 창이 없으면 위·아래는 이력, `enter` 는 실행이다. 창이 그 키들을 늘 먹지 않는다.
func TestCommandArrowsBrowseHistoryWithoutCandidates(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 60, 8)
	m = send(m, ":", "n", "o", "h", "enter", ":", "up")

	here, ok := m.(viewEditorCommand)
	require.True(t, ok)
	assert.Equal(t, "noh", here.input.text)
}

// 커서를 옆으로 옮기면 창이 닫힌다. 갈아끼울 조각이 창을 띄운 때의 것이 아니다.
func TestCommandCandidatesCloseOnCursorMove(t *testing.T) {
	completeDir(t, "a1", "a2")

	m := commandAfter(t, ":", "e", " ", "a", "tab", "left")

	assert.Empty(t, m.candidates)
}

// 창보다 후보가 많으면 고른 줄을 따라 창이 밀리고 아래에 몇째인지 적는다.
func TestCommandCandidateWindowFollowsSelection(t *testing.T) {
	completeDir(t, "d01", "d02", "d03", "d04", "d05", "d06", "d07", "d08", "d09", "d10")

	var m tea.Model = newTestEditor("abc\n", 60, 20)
	m = send(m, ":", "e", " ", "d", "tab")
	for range 9 {
		m = send(m, "down")
	}

	here := m.(viewEditorCommand)
	require.Equal(t, 8, here.candidateRows())
	assert.Equal(t, 9, here.selected)
	assert.Equal(t, 2, here.candidateStart(), "고른 줄이 창 맨 아래에 온다")

	box := here.renderCandidateBox()
	assert.Contains(t, box, "d10")
	assert.NotContains(t, box, "d01", "위로 밀려난 줄은 보이지 않는다")
	assert.Contains(t, box, " 10/10")
}

// candidateBoxHeight 는 그린 창의 줄 수와 같다. 클릭을 받는 자리가 이것으로 창을 놓는다.
func TestCommandCandidateBoxHeightMatchesRender(t *testing.T) {
	for _, names := range [][]string{{"a1", "a2"}, {"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9"}} {
		completeDir(t, names...)

		var m tea.Model = newTestEditor("abc\n", 60, 20)
		m = send(m, ":", "e", " ", "a", "tab")

		here := m.(viewEditorCommand)
		assert.Equal(t, len(strings.Split(here.renderCandidateBox(), "\n")), here.candidateBoxHeight())
	}
}

// 후보 줄을 누르면 그것을 넣는다. 테두리나 창 밖을 누르면 아무 일도 없다.
func TestCommandCandidateClick(t *testing.T) {
	completeDir(t, "a1", "a2", "a3")

	var m tea.Model = newTestEditor("abc\n", 60, 12)
	m = send(m, ":", "e", " ", "a", "tab")

	here := m.(viewEditorCommand)
	left, top := here.candidateBoxPos()

	// 테두리는 후보가 아니다.
	m, _ = here.Update(click(left+2, top))
	assert.Equal(t, "e a", m.(viewEditorCommand).input.text)

	// 창 밖은 받지 않는다. 치던 명령이 사라지지 않는다.
	m, _ = here.Update(click(left+commandCompleteWidth+1, top+1))
	assert.Equal(t, "e a", m.(viewEditorCommand).input.text)
	assert.NotEmpty(t, m.(viewEditorCommand).candidates)

	// 셋째 줄(a3) 을 누른다.
	m, _ = here.Update(click(left+2, top+3))
	assert.Equal(t, "e a3", m.(viewEditorCommand).input.text)
	assert.Empty(t, m.(viewEditorCommand).candidates)
}

// 창 위의 휠은 창을 한 줄씩 민다. 고른 줄은 창 안으로 끌려온다.
func TestCommandCandidateWheel(t *testing.T) {
	completeDir(t, "d01", "d02", "d03", "d04", "d05", "d06", "d07", "d08", "d09", "d10")

	var m tea.Model = newTestEditor("abc\n", 60, 20)
	m = send(m, ":", "e", " ", "d", "tab")

	here := m.(viewEditorCommand)
	left, top := here.candidateBoxPos()

	m, _ = here.Update(wheel(left+2, top+1, false))
	m, _ = m.Update(wheel(left+2, top+1, false))

	here = m.(viewEditorCommand)
	assert.Equal(t, 2, here.candidateStart())
	assert.Equal(t, 2, here.selected, "밀려난 고른 줄이 창 첫 줄로 온다")

	// 끝을 넘지 않는다.
	for range 5 {
		m, _ = m.Update(wheel(left+2, top+1, false))
	}
	assert.Equal(t, 2, m.(viewEditorCommand).candidateStart(), "열 개 중 여덟 줄이라 2 가 끝이다")
}
