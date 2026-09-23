package core

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commandLineOf 는 명령줄·검색창이 치고 있는 글이다.
func commandLineOf(t *testing.T, m tea.Model) string {
	t.Helper()

	switch v := m.(type) {
	case viewEditorCommand:
		return v.input.text
	case viewEditorSearch:
		return v.input.text
	default:
		t.Fatalf("명령줄이 아니다: %T", m)

		return ""
	}
}

// 친 명령이 담기고 위로 꺼내진다.
func TestCommandHistoryRecallsWithUp(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "noh")

	m = send(m, ":", "up")

	require.IsType(t, viewEditorCommand{}, m)
	assert.Equal(t, "noh", commandLineOf(t, m))
}

// 뜯다 막힌 줄도 담는다. 고쳐서 다시 치려면 그것이 이력에 있어야 한다(ADR-0143 §5).
func TestCommandHistoryRecordsRejectedLines(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "nosuchcommand")

	m = send(m, ":", "up")

	assert.Equal(t, "nosuchcommand", commandLineOf(t, m))
}

// 친 것을 접두로 거른다. `:e` 까지 치고 위를 누르면 `:e` 로 시작한 것만 나온다.
func TestCommandHistoryFiltersByPrefix(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	m = runCommand(m, "noh")
	m = runCommand(m, "version")

	m = send(m, ":", "n", "o", "up")

	assert.Equal(t, "noh", commandLineOf(t, m), "`version` 은 걸러진다")
}

// 맨 아래로 내려오면 치던 글이 돌아온다.
func TestCommandHistoryRestoresTypedTextOnTheWayDown(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "noh")

	m = send(m, ":", "n", "up")
	require.Equal(t, "noh", commandLineOf(t, m))

	m = send(m, "down")
	assert.Equal(t, "n", commandLineOf(t, m))
}

// 글을 고치면 훑기를 놓는다. 다음 위는 새 접두로 시작한다.
func TestCommandHistoryRestartsAfterEditing(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	m = runCommand(m, "noh")
	m = runCommand(m, "version")

	// 위로 `version` 을 꺼낸 뒤 지우고 `n` 을 치면 거를 접두가 `n` 이 된다.
	m = send(m, ":", "up")
	require.Equal(t, "version", commandLineOf(t, m))

	m = send(m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	require.Equal(t, "", commandLineOf(t, m))

	m = send(m, "n", "up")
	assert.Equal(t, "noh", commandLineOf(t, m))
}

// 커서만 옮기는 것은 훑던 자리를 지킨다. 거를 접두가 그대로다(ADR-0143 §2).
func TestCommandHistoryKeepsBrowsingWhenCursorMoves(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	m = runCommand(m, "noh")
	m = runCommand(m, "version")

	m = send(m, ":", "up")
	require.Equal(t, "version", commandLineOf(t, m))

	m = send(m, "home", "up")
	assert.Equal(t, "noh", commandLineOf(t, m), "훑던 자리에서 이어 올라간다")
}

// 검색 이력은 명령 이력과 따로다. `/` 에서 위를 눌렀을 때 `:w` 가 나오면 안 된다(ADR-0143 §1).
func TestSearchHistoryIsSeparateFromCommands(t *testing.T) {
	m := tea.Model(newTestEditor("foo\nbar\n", 60, 8))
	m = runCommand(m, "noh")
	m = send(m, "/", "f", "o", "o", "enter")

	m = send(m, "/", "up")
	require.IsType(t, viewEditorSearch{}, m)
	assert.Equal(t, "foo", commandLineOf(t, m))

	m = send(m, "up")
	assert.Equal(t, "foo", commandLineOf(t, m), "명령 이력이 섞여 오지 않는다")
}

// `?` 로 친 것도 같은 검색 이력에 담긴다. 방향은 찾을 때 정하는 것이지 패턴의 성질이 아니다.
func TestSearchHistoryIsSharedBetweenDirections(t *testing.T) {
	m := tea.Model(newTestEditor("foo\nbar\n", 60, 8))
	m = send(m, "?", "b", "a", "r", "enter")

	m = send(m, "/", "up")
	assert.Equal(t, "bar", commandLineOf(t, m))
}

// `:s` 는 전문이 명령 이력에, 패턴만 검색 이력에 담긴다. 한 줄이 두 갈래에 담기는 자리다.
func TestSubstitutePatternGoesToBothHistories(t *testing.T) {
	m := runCommand(newTestEditor("foo\nfoo\n", 60, 8), "%s/foo/bar/g")

	search := send(m, "/", "up")
	assert.Equal(t, "foo", commandLineOf(t, search), "패턴이 검색 이력에 있다")

	command := send(m, ":", "up")
	assert.Equal(t, "%s/foo/bar/g", commandLineOf(t, command), "전문이 명령 이력에 있다")
}

// historyViewOf 는 판을 명령줄을 거치지 않고 연다.
//
// `:history` 는 자기 자신도 이력에 담으므로(담는 규칙이 「친 것은 전부」다) 목록의 내용을
// 보는 시험은 그 한 줄이 섞이지 않는 이 길로 연다. 명령줄 배선을 보는 시험만 `:history` 를 친다.
func historyViewOf(t *testing.T, m tea.Model, kind historyKind) viewHistory {
	t.Helper()

	normal, ok := m.(viewEditorNormal)
	require.True(t, ok, "normal 에서 열어야 한다: %T", m)

	view, _ := historyMode(normal.editor, kind)
	require.IsType(t, viewHistory{}, view)

	return view.(viewHistory)
}

// `:history` 는 명령 이력을, `:history /` 는 검색 이력을 보인다.
func TestHistoryViewSplitsByArgument(t *testing.T) {
	m := tea.Model(newTestEditor("foo\nbar\n", 60, 8))
	m = runCommand(m, "noh")
	m = send(m, "/", "f", "o", "o", "enter")

	assert.Equal(t, []historyRow{{prompt: ":", text: "noh"}}, historyViewOf(t, m, historyCommands).rows())
	assert.Equal(t, []historyRow{{prompt: "/", text: "foo"}}, historyViewOf(t, m, historySearches).rows())
}

// 인자가 갈래를 정한다. 모르는 인자는 오류다.
func TestParseHistoryKind(t *testing.T) {
	tests := []struct {
		args []string
		want historyKind
	}{
		{args: nil, want: historyCommands},
		{args: []string{":"}, want: historyCommands},
		{args: []string{"/"}, want: historySearches},
		{args: []string{"?"}, want: historySearches},
		{args: []string{"all"}, want: historyAll},
	}
	for _, test := range tests {
		kind, err := parseHistoryKind(test.args)
		require.NoError(t, err)
		assert.Equal(t, test.want, kind, "%v", test.args)
	}

	_, err := parseHistoryKind([]string{"nope"})
	assert.Error(t, err)
}

// `:history all` 은 둘 다 보인다. 명령이 앞이고 검색이 뒤다.
func TestHistoryViewShowsBothForAll(t *testing.T) {
	m := tea.Model(newTestEditor("foo\nbar\n", 60, 8))
	m = runCommand(m, "noh")
	m = send(m, "/", "f", "o", "o", "enter")

	assert.Equal(t, []historyRow{
		{prompt: ":", text: "noh"},
		{prompt: "/", text: "foo"},
	}, historyViewOf(t, m, historyAll).rows())
}

// 모르는 인자는 조용히 넘기지 않는다. 인자를 대고 친 것이라 조용하면 먹힌 것으로 읽힌다.
func TestHistoryViewRejectsUnknownArgument(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "history nope")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, m.(viewEditorNormal).notice, "알 수 없는 이력 갈래")
}

// 맨 아래에서 시작한다. 찾는 것은 대개 방금 친 것이다.
func TestHistoryViewStartsAtTheNewest(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	m = runCommand(m, "noh")
	m = runCommand(m, "version")

	view := historyViewOf(t, m, historyCommands)

	rows := view.rows()
	require.Equal(t, len(rows)-1, view.selected)
	assert.Equal(t, "version", rows[view.selected].text)
}

// `enter` 는 고른 것을 명령줄에 실어서 연다. 돌리지 않는다(ADR-0143 §4).
func TestHistoryViewLoadsSelectedIntoCommandLine(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	m = runCommand(m, "version")

	next := pressKeys(historyViewOf(t, m, historyCommands), "enter")

	require.IsType(t, viewEditorCommand{}, next)
	assert.Equal(t, "version", commandLineOf(t, next))
}

// 검색 줄은 `/` 로 열린다. 이력에 방향이 없어서 앞으로 찾는 쪽이다.
func TestHistoryViewLoadsSearchIntoSearchLine(t *testing.T) {
	m := tea.Model(newTestEditor("foo\nbar\n", 60, 8))
	m = send(m, "?", "b", "a", "r", "enter")

	next := pressKeys(historyViewOf(t, m, historySearches), "enter")

	require.IsType(t, viewEditorSearch{}, next)
	assert.Equal(t, "bar", commandLineOf(t, next))
	assert.Equal(t, "/", next.(viewEditorSearch).prompt())
}

// 빈 이력에서 `enter` 는 아무 일도 하지 않는다.
func TestHistoryViewEnterDoesNothingWhenEmpty(t *testing.T) {
	view := historyViewOf(t, newTestEditor("a\nb\n", 60, 8), historySearches)

	next := pressKeys(view, "enter")

	assert.IsType(t, viewHistory{}, next)
}

// 갈래 글자가 줄마다 찍힌다. 색으로만 가르면 화면을 글자로 떠서 보는 길에서 갈래가 사라진다.
func TestHistoryViewRendersPromptPerRow(t *testing.T) {
	m := tea.Model(newTestEditor("foo\nbar\n", 60, 8))
	m = runCommand(m, "noh")
	m = send(m, "/", "f", "o", "o", "enter")

	screen := stripANSI(historyViewOf(t, m, historyAll).View().Content)

	assert.Contains(t, screen, ": noh")
	assert.Contains(t, screen, "/ foo")
}

// 빈 까닭은 갈래마다 다르다. 어느 갈래의 말인지 보여야 이력이 사라진 것으로 읽히지 않는다.
func TestHistoryViewTellsWhichKindIsEmpty(t *testing.T) {
	view := historyViewOf(t, newTestEditor("a\nb\n", 60, 8), historySearches)

	screen := stripANSI(view.View().Content)
	assert.Contains(t, screen, "찾은 것이 없습니다")
	assert.NotContains(t, screen, "친 명령이 없습니다")
}

// `:his` 를 줄임말로 받는다. vim 이 그렇다.
func TestHistoryViewAcceptsAbbreviation(t *testing.T) {
	view := runCommand(newTestEditor("a\nb\n", 60, 8), "his")

	assert.IsType(t, viewHistory{}, view)
}

// `:history` 를 친 것 자체도 이력에 담긴다. 담는 규칙이 「친 것은 전부」다.
func TestHistoryViewRecordsItself(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n", 60, 8), "history")

	require.IsType(t, viewHistory{}, m)
	assert.Equal(t, []string{"history"}, m.(viewHistory).commandHistory.entries)
}

// 화면이 이력보다 짧으면 고른 줄이 보이도록 첫 행이 밀린다.
func TestHistoryViewScrollsToSelected(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	for i := range 20 {
		m = runCommand(m, fmt.Sprintf("noh%d", i))
	}

	view := historyViewOf(t, m, historyCommands)

	require.Greater(t, len(view.rows()), view.listHeight(), "목록이 화면보다 길어야 한다")
	assert.Less(t, view.selected, view.top+view.listHeight())
	assert.GreaterOrEqual(t, view.selected, view.top)
}

// 한글 입력 상태에서도 `j`·`k` 가 먹는다. 입력줄이 없는 화면이라 되돌려 읽는다(ADR-0008).
func TestHistoryViewExpandsHangulKeys(t *testing.T) {
	m := tea.Model(newTestEditor("a\nb\n", 60, 8))
	m = runCommand(m, "noh")
	m = runCommand(m, "version")

	view := historyViewOf(t, m, historyCommands)

	next := pressKeys(view, "ㅏ")

	require.IsType(t, viewHistory{}, next)
	assert.Equal(t, view.selected-1, next.(viewHistory).selected, "`ㅏ` 가 `k` 로 읽힌다")
}

// stripANSI 는 색 escape 를 걷어낸 화면이다.
func stripANSI(screen string) string {
	var out strings.Builder

	for i := 0; i < len(screen); {
		if screen[i] == 0x1b {
			for i < len(screen) && screen[i] != 'm' {
				i++
			}
			i++

			continue
		}
		out.WriteByte(screen[i])
		i++
	}

	return out.String()
}
