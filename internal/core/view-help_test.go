package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// helpOpen 은 도움말을 연 화면이다.
func helpOpen(t *testing.T) viewHelp {
	t.Helper()

	m := newTestEditor("foo\n", 90, 24)

	opened := send(m, ":", "h", "e", "l", "p", "enter")

	help, ok := opened.(viewHelp)
	require.True(t, ok, "`:help` 가 도움말 판을 열어야 한다")

	return help
}

// `:help` 가 판을 연다. 줄임말 `:h` 도 같다.
func TestHelpOpensFromCommand(t *testing.T) {
	assert.IsType(t, viewHelp{}, send(newTestEditor("foo\n", 90, 24), ":", "h", "e", "l", "p", "enter"))
	assert.IsType(t, viewHelp{}, send(newTestEditor("foo\n", 90, 24), ":", "h", "enter"))
}

// **볼 파일이 없어도 열린다.** 빈 화면에서 「이제 뭘 하지」를 묻는 자리가 곧 여기다.
func TestHelpOpensWithoutBuffer(t *testing.T) {
	empty := send(newTabsEditor("a.txt"), ":", "q", "enter")
	require.IsType(t, viewEditorEmpty{}, empty)

	assert.IsType(t, viewHelp{}, send(empty, ":", "h", "e", "l", "p", "enter"))
}

// 담긴 글이 embed 한 그것이다.
func TestHelpShowsEmbeddedText(t *testing.T) {
	help := helpOpen(t)

	// 글이 개행으로 끝나므로 줄 수가 개행 수와 같다.
	assert.Equal(t, strings.Count(assets.Help, "\n"), help.buf.LineCount())
	assert.Contains(t, string(help.buf.Line(0)), "zn 도움말")
}

// 고쳐지지 않는다. 담긴 것이 바이너리 안의 글이라 저장할 곳이 없다.
func TestHelpIsReadOnly(t *testing.T) {
	assert.True(t, helpOpen(t).buf.ReadOnly)
}

// `q` 와 `esc` 로 닫고 편집 화면으로 돌아간다.
func TestHelpClosesBackToNormal(t *testing.T) {
	assert.IsType(t, viewEditorNormal{}, send(helpOpen(t), "q"))
	assert.IsType(t, viewEditorNormal{}, send(helpOpen(t), "esc"))
}

// **tab 을 더럽히지 않는다.** 판이 창을 혼자 들어서 열기 전과 tab 수가 같다.
func TestHelpDoesNotAddTab(t *testing.T) {
	help := helpOpen(t)

	require.Len(t, help.buffers, 1)
	assert.NotEqual(t, helpPath, help.buffers[0].Path, "도움말이 tab 으로 들어가면 안 된다")

	closed := send(help, "q")
	assert.Len(t, closed.(viewEditorNormal).buffers, 1)
}

// `j` `k` 는 화면 행 단위다. 한 문단이 한 줄이라 줄 단위로 움직이면 한 번에 화면 절반이 지나간다.
func TestHelpMovesByScreenRow(t *testing.T) {
	help := helpOpen(t)
	require.Equal(t, 0, help.buf.Cursor.Line)

	// 첫 줄은 짧아서 한 행이다. 그 다음이 빈 줄이고 그 다음이 접히는 긴 문단이다.
	moved := send(help, "j", "j", "j").(viewHelp)

	assert.Greater(t, moved.buf.Cursor.Line, 0)
	assert.Less(t, moved.buf.Cursor.Line, 4, "행 단위라 줄이 그보다 적게 움직인다")
}

// `G` 는 끝으로, `g` 는 처음으로 간다. 판들은 `g` 가 한 번이다.
func TestHelpGoesToEndsWithSingleG(t *testing.T) {
	help := helpOpen(t)

	end := send(help, "G").(viewHelp)
	assert.Equal(t, end.buf.LineCount()-1, end.buf.Cursor.Line)

	back := send(end, "g").(viewHelp)
	assert.Equal(t, 0, back.buf.Cursor.Line)
}

// ── 찾기 ──

// `/` 로 친 글이 있는 자리로 간다.
func TestHelpSearchJumps(t *testing.T) {
	found := send(helpOpen(t), "/", "레", "지", "스", "터", "enter").(viewHelp)

	// 한글은 되돌리지 않는다. 찾을 글은 글자라 두벌식으로 바꾸면 안 된다.
	assert.Equal(t, "레지스터", found.query)
}

// 찾은 글이 실제로 그 줄에 있다.
func TestHelpSearchLandsOnMatch(t *testing.T) {
	found := send(helpOpen(t), "/", "c", "l", "i", "p", "b", "o", "a", "r", "d", "enter").(viewHelp)

	require.NotNil(t, found.pattern)
	assert.Contains(t, string(found.buf.Line(found.buf.Cursor.Line)), "clipboard")
}

// 찾지 못하면 알리고 들어온 자리로 돌아간다.
func TestHelpSearchMissNotifies(t *testing.T) {
	help := helpOpen(t)
	origin := help.buf.Cursor

	missed := send(help, "/", "z", "z", "z", "q", "q", "enter").(viewHelp)

	assert.Equal(t, origin, missed.buf.Cursor)
	assert.Contains(t, missed.notice, "찾지 못했습니다")
}

// `esc` 는 치던 것을 버리고 들어온 자리로 돌아간다.
func TestHelpSearchEscapeRestores(t *testing.T) {
	help := send(helpOpen(t), "G").(viewHelp)
	origin := help.buf.Cursor

	back := send(help, "/", "z", "n", "esc").(viewHelp)

	assert.Equal(t, origin, back.buf.Cursor)
}

// **편집 화면의 찾기를 덮지 않는다.** 판을 닫은 뒤 `n` 이 파일에서 찾던 것을 이어야 한다.
func TestHelpSearchDoesNotTouchEditorSearch(t *testing.T) {
	m := newTestEditor("alpha\nbeta\n", 90, 24)

	// 편집 화면에서 먼저 찾아 둔다.
	searched := send(m, "/", "b", "e", "t", "a", "enter")
	before := searched.(viewEditorNormal).search

	closed := send(searched, ":", "h", "e", "l", "p", "enter", "/", "r", "e", "g", "enter", "q")

	assert.Equal(t, before, closed.(viewEditorNormal).search)
}

// ── 글 자체 ──

// 도움말이 실제로 있는 명령만 적고 있는지 본다. 명령이 없어지면 글도 같이 고쳐야 한다.
func TestHelpMentionsRealCommands(t *testing.T) {
	for _, command := range []string{":w", ":q", ":tips", ":registers", ":graph", ":diff", ":help"} {
		assert.Contains(t, assets.Help, command, "도움말이 %s 를 적어야 한다", command)
	}
}

// 강제 줄바꿈이 없다. 판이 화면 폭에 맞춰 접으므로 소스에서 접으면 두 번 접힌다.
//
// 표와 코드펜스는 줄 경계가 뜻을 가지므로 재지 않는다. 산문 줄만 본다.
func TestHelpHasNoHardWrap(t *testing.T) {
	// **산문 두 줄이 빈 줄 없이 이어지면 강제 줄바꿈이다.** 한 문단이 한 줄이라는 규칙이
	// 지켜지면 문단마다 빈 줄이 그 앞뒤에 선다.
	//
	// 표(`|`), 목록(`- `), 머리줄(`#`), 코드펜스 안은 줄 경계가 뜻을 가지므로 세지 않는다.
	inFence, prose := false, 0

	for _, line := range strings.Split(assets.Help, "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			prose = 0

			continue
		}

		if inFence {
			continue
		}

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "- ") {
			prose = 0

			continue
		}

		prose++
		assert.LessOrEqual(t, prose, 1, "산문이 빈 줄 없이 이어진다. 강제 줄바꿈이다: %q", line)
	}
}
