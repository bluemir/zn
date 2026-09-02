package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"
)

// `home`·`end`·`pgup`·`pgdown`·`delete` 가 실제로 먹는지 보는 자리다.
//
// 다섯 다 한 곳도 걸려 있지 않아서 어느 플랫폼에서도 조용히 씹혔다. mac 에서 먼저 눈에 띈
// 것은 그 키가 `fn` 조합이라서일 뿐이다(ADR-0076).

// normal 의 `home`·`end` 는 화면 행의 양끝이다. wrap 된 줄에서 `0`·`$` 와 갈린다.
func TestNormalHomeEndMoveWithinScreenRow(t *testing.T) {
	// 폭을 좁혀 첫 줄이 화면 행 여럿이 되게 한다. 줄은 세 행보다 길어야 한다 — 딱 두 행이면
	// 둘째 행의 끝이 곧 줄 끝이라 화면 행과 논리 줄이 같은 답을 내서 가름이 드러나지 않는다.
	m := newTestEditor(strings.Repeat("a", 60)+"\nsecond\n", 20, 8)
	width := m.contentWidth()
	require.Less(t, 2*width, 60, "둘째 행의 끝이 줄 끝이 아니어야 한다")

	// 자리를 매번 다시 잡는다. `m` 은 값이지만 buffer 는 같은 editor 를 가리켜서,
	// 앞 검사가 옮긴 커서에서 다음 검사가 출발한다.
	m.activeBuffer().MoveTo(scheme.Cursor{Line: 0, Col: width + 3})
	end := bufferOf(t, send(m, "end"))
	assert.Equal(t, 2*width, end.Cursor.Col, "지금 행의 끝이다. 줄 끝(60) 이 아니다")

	m.activeBuffer().MoveTo(scheme.Cursor{Line: 0, Col: width + 3})
	home := bufferOf(t, send(m, "home"))
	assert.Equal(t, width, home.Cursor.Col, "지금 행의 앞이다. 줄 맨 앞(0) 이 아니다")
}

// 접히지 않은 줄에서는 `0`·`$` 와 같은 자리다.
func TestNormalHomeEndOnUnwrappedLine(t *testing.T) {
	m := newTestEditor("hello world\nsecond\n", wide, 8)

	m.activeBuffer().MoveTo(scheme.Cursor{Line: 0, Col: 4})
	assert.Zero(t, bufferOf(t, send(m, "home")).Cursor.Col)

	// normal 은 마지막 글자 위에 선다(clampToNormal). `$` 와 같다.
	m.activeBuffer().MoveTo(scheme.Cursor{Line: 0, Col: 4})
	assert.Equal(t, len("hello world")-1, bufferOf(t, send(m, "end")).Cursor.Col)
}

// `pgdown`·`pgup` 은 `ctrl+f`·`ctrl+b` 와 같은 한 화면이다. 숫자도 그대로 받는다.
func TestNormalPageKeysMatchCtrlFB(t *testing.T) {
	data := strings.Repeat("line\n", 200)

	pgdown := bufferOf(t, send(newTestEditor(data, wide, 10), "pgdown"))
	ctrlF := bufferOf(t, send(newTestEditor(data, wide, 10), "ctrl+f"))
	assert.Equal(t, ctrlF.Cursor.Line, pgdown.Cursor.Line)
	assert.Equal(t, ctrlF.Top, pgdown.Top)

	thrice := bufferOf(t, send(newTestEditor(data, wide, 10), "3", "pgdown"))
	assert.Greater(t, thrice.Cursor.Line, pgdown.Cursor.Line, "숫자는 되풀이다")

	back := bufferOf(t, send(send(newTestEditor(data, wide, 10), "pgdown"), "pgup"))
	assert.Zero(t, back.Cursor.Line, "되돌아온다")
}

// normal 의 `delete` 는 아무 일도 하지 않는다. 그 자리에는 `x` 가 이미 있다.
func TestNormalDeleteDoesNothing(t *testing.T) {
	m := newTestEditor("abc\n", wide, 8)

	after := bufferOf(t, send(m, "delete"))

	assert.Equal(t, "abc", string(after.Lines[0]))
	assert.False(t, after.Dirty)
}

// insert 의 `delete` 는 커서 자리 글자를 지운다.
func TestInsertDeleteRemovesCharAtCursor(t *testing.T) {
	m := send(newTestEditor("abc\n", wide, 8), "i", "delete")

	assert.Equal(t, "bc", string(bufferOf(t, m).Lines[0]))
}

// insert 의 `home`·`end` 도 화면 행의 양끝이고, 커서를 옮기므로 undo 구간이 끊긴다.
func TestInsertHomeEndMoveAndBreakUndo(t *testing.T) {
	m := send(newTestEditor("abc\n", wide, 8), "i", "x", "end")

	buf := bufferOf(t, m)
	assert.Equal(t, len("xabc"), buf.Cursor.Col)

	// **커서를 옮기면 undo 구간이 끊긴다.** 담아둔 상태를 보지 않고 무른 결과로 본다 —
	// `end` 뒤에 친 `y` 와 그 앞에 친 `x` 가 따로 돌아와야 한다(ADR-0033, ADR-0129).
	broke := send(m, "y", "esc", "u")
	assert.Equal(t, []string{"xabc"}, linesOf(bufferOf(t, broke)),
		"한 번의 u 가 `y` 만 물러야 한다 — 둘 다 물면 구간이 안 끊긴 것이다")

	assert.Zero(t, bufferOf(t, send(m, "home")).Cursor.Col)
}

// insert 의 `pgdown`·`pgup` 도 한 화면이다.
func TestInsertPageKeysMovePage(t *testing.T) {
	m := send(newTestEditor(strings.Repeat("line\n", 200), wide, 10), "i", "pgdown")

	assert.Greater(t, bufferOf(t, m).Cursor.Line, 0)
}

// 목록 판에서 `home`·`end` 는 `g`·`G` 와 같은 자리로 간다. `pgup`·`pgdown` 은 한 화면이다.
//
// 판 아홉이 같은 규칙을 쓴다. 여기서는 알림 목록으로 대표해 본다 — 나머지는 같은 case 에
// 이름만 더한 것이라 갈릴 자리가 없다(ADR-0076).
func TestMessagesHomeEndPageKeys(t *testing.T) {
	texts := make([]string, 0, 40)
	for i := range 40 {
		texts = append(texts, string(rune('a'+i%26))+strings.Repeat("x", i%3))
	}

	m := messagesFixture(t, texts...)
	require.Greater(t, len(texts), m.listHeight(), "목록이 화면보다 길어야 한다")

	assert.Zero(t, send(m, "home").(viewMessages).selected)
	assert.Equal(t, len(texts)-1, send(m, "end").(viewMessages).selected)

	// 한 화면은 편집 영역·트리와 같은 자다(page.go 의 pageRows).
	page := pageRows(pageFull, m.listHeight())
	top := send(m, "home").(viewMessages)
	assert.Equal(t, page, send(top, "pgdown").(viewMessages).selected)
	assert.Zero(t, send(send(top, "pgdown"), "pgup").(viewMessages).selected)
}

// 트리의 `home`·`end` 는 첫·마지막 행이다. 숫자는 보지 않는다 — `gg`·`G` 와 갈리는 자리다.
func TestSidebarHomeEndIgnoreCount(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	rows := len(m.(viewSidebar).sidebar.rows())
	require.Greater(t, rows, 1)

	m = send(m, "end")
	assert.Equal(t, rows-1, m.(viewSidebar).sidebar.selected)

	m = send(m, "home")
	assert.Zero(t, m.(viewSidebar).sidebar.selected)

	// `3G` 는 세 번째 행인데 `3end` 는 마지막 행이다.
	assert.Equal(t, rows-1, send(m, "3", "end").(viewSidebar).sidebar.selected)
	assert.Equal(t, 2, send(m, "3", "G").(viewSidebar).sidebar.selected)
}

// 트리의 `pgup`·`pgdown` 은 `ctrl+f`·`ctrl+b` 와 같은 한 화면이다.
func TestSidebarPageKeysMatchCtrlFB(t *testing.T) {
	open := func() tea.Model { return send(newTreeEditor(t, 80, 6), "ctrl+w", "ctrl+w") }

	assert.Equal(t,
		send(open(), "ctrl+f").(viewSidebar).sidebar.selected,
		send(open(), "pgdown").(viewSidebar).sidebar.selected)

	down := send(open(), "pgdown")
	assert.Zero(t, send(down, "pgup").(viewSidebar).sidebar.selected)
}
