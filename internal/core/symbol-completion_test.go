package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// 커서 앞에서 별칭을 떼어내는 규칙이다. `:` 뒤 한 글자부터 걸린다.
func TestSymbolAliasAt(t *testing.T) {
	for _, c := range []struct {
		line  string
		alias string
		start int
		ok    bool
	}{
		{line: ":+", alias: "+", start: 0, ok: true},
		{line: ":+1", alias: "+1", start: 0, ok: true},
		// 닫는 `:` 까지 친 것도 같은 별칭이다
		{line: ":+1:", alias: "+1", start: 0, ok: true},

		// start 는 byte 열이다. `글` 이 세 byte 라 `:` 가 4 다
		{line: "글 :tada", alias: "tada", start: 4, ok: true},
		{line: ":ok_hand", alias: "ok_hand", start: 0, ok: true},

		// `:` 만 친 것은 아직 별칭이 아니다. 산문과 yaml 에서 걸리는 자리가 너무 넓다.
		{line: ":"},
		{line: "key: "},
		{line: "x := 1"},
		{line: "tada"},
		{line: ""},
	} {
		alias, start, ok := symbolAliasAt([]byte(c.line), len(c.line))

		assert.Equal(t, c.ok, ok, "%q", c.line)
		if !c.ok {
			continue
		}

		assert.Equal(t, c.alias, alias, "%q", c.line)
		assert.Equal(t, c.start, start, "%q 의 `:` 자리", c.line)
	}
}

// 앞에서부터 맞아야 걸린다. 흩어진 맞추기를 쓰지 않는다.
func TestAliasMatchesFromTheFront(t *testing.T) {
	assert.True(t, aliasMatches("tada", "ta"))
	assert.True(t, aliasMatches("tada", "tada"))
	assert.False(t, aliasMatches("tada", "ada"), "가운데부터는 걸리지 않는다")
	assert.False(t, aliasMatches("tada", "tadaa"), "친 것이 더 길면 걸리지 않는다")
}

// insertTyping 은 insert 로 들어가 글자를 하나씩 치는 화면이다.
func insertTyping(t *testing.T, text string) tea.Model {
	t.Helper()

	m := send(newTestEditorFile("a.md", "", 80, 12), "i")
	require.IsType(t, viewEditorInsert{}, m)

	for _, r := range text {
		m = send(m, string(r))
	}

	return m
}

// 이 기능의 기준 장면이다. `:+1` 을 치면 목록이 뜨고 enter 가 이모지를 넣는다.
func TestSymbolCompletionInsertsEmoji(t *testing.T) {
	m := insertTyping(t, ":+1")

	require.IsType(t, viewEditorInsert{}, m)
	require.True(t, m.(viewEditorInsert).completionOpen(), "목록이 떠야 한다")

	m = send(m, "enter")

	assert.Equal(t, "👍", string(bufferOf(t, m).Line(0)), "친 `:+1` 을 덮어쓴다")
	assert.False(t, m.(viewEditorInsert).completionOpen(), "넣고 나면 닫힌다")
}

// 닫는 `:` 까지 친 사람도 같은 답에 닿는다.
func TestSymbolCompletionAcceptsClosingColon(t *testing.T) {
	m := insertTyping(t, ":tada:")

	require.True(t, m.(viewEditorInsert).completionOpen())

	m = send(m, "enter")

	assert.Equal(t, "🎉", string(bufferOf(t, m).Line(0)))
}

// `:` 뒤 한 글자부터 뜬다. 그래야 목록을 보고 고를 수 있다.
func TestSymbolCompletionOpensAfterOneChar(t *testing.T) {
	m := insertTyping(t, ":")
	assert.False(t, m.(viewEditorInsert).completionOpen(), "`:` 만으로는 뜨지 않는다")

	m = send(m, "t")
	assert.True(t, m.(viewEditorInsert).completionOpen(), "한 글자를 더 치면 뜬다")
}

// 걸리는 별칭이 없으면 뜨지 않는다. 그것이 yaml·코드에서 걸리는 자리를 좁힌다.
func TestSymbolCompletionStaysClosedWithoutMatch(t *testing.T) {
	m := insertTyping(t, "key:zzz")

	assert.False(t, m.(viewEditorInsert).completionOpen())
	assert.Equal(t, "key:zzz", string(bufferOf(t, m).Line(0)), "친 것은 그대로 들어간다")
}

// 이어 치면 목록이 좁혀진다. 확정하지 않으면 평소대로 글이 들어간다.
func TestSymbolCompletionNarrowsAsYouType(t *testing.T) {
	m := insertTyping(t, ":s")
	wide := len(m.(viewEditorInsert).completion.items)
	require.Greater(t, wide, 1, "`:s` 로는 여럿이 걸린다")

	m = send(m, "p")
	narrow := len(m.(viewEditorInsert).completion.items)

	assert.Less(t, narrow, wide, "좁혀져야 한다")
	assert.Equal(t, ":sp", string(bufferOf(t, m).Line(0)), "친 것은 buffer 에 그대로 있다")
}

// 별칭 표는 큐레이션 표에서 읽는다. 훑어서 얻은 글자에는 별칭이 없다.
func TestSymbolsWithAliasComeFromCuratedTable(t *testing.T) {
	with := symbolsWithAlias()

	require.NotEmpty(t, with)
	assert.Less(t, len(with), len(assets.CuratedSymbols), "표의 일부다")

	for _, entry := range with {
		assert.NotEmpty(t, entry.AliasList(), "%q", entry.Char)

		for _, name := range entry.AliasList() {
			for i := range len(name) {
				assert.True(t, symbolAliasChar(name[i]),
					"%q 의 별칭 %q 에 `:` 뒤로 칠 수 없는 글자가 있다", entry.Char, name)
			}
		}
	}
}
