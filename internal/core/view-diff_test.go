package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// newDiffView 는 두 글을 견준 판이다. 읽어 온 것을 그 자리에 넣는다.
func newDiffView(t *testing.T, left, right string) viewDiff {
	t.Helper()

	e := &editor{boxChars: boxUnicode, width: 80, height: 20}

	leftLines, rightLines := splitDiffLines(left), splitDiffLines(right)

	return viewDiff{
		editor: e,
		left:   diffSide{label: "HEAD", path: "a.txt", lines: leftLines},
		right:  diffSide{label: "지금", path: "a.txt", lines: rightLines},
		hunks:  buildDiffHunks(leftLines, rightLines),
		ready:  true,
	}
}

// diffScreen 은 판이 그린 화면에서 색을 뗀 글이다.
func diffScreen(m viewDiff) string {
	return ansi.Strip(m.View().Content)
}

// unified 는 고친 자리를 두 행으로 내고 side-by-side 는 한 행으로 낸다(ADR-0140 §2).
func TestDiffLayoutDependsOnView(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")

	// 머리줄 하나 + 문맥 둘 + 고친 자리 둘이다.
	assert.Len(t, m.layout(), 1+2+2)

	m.side = true
	assert.Len(t, m.layout(), 1+2+1)
}

// `tab` 을 눌러도 보던 조각이 그대로 남는다. 자리를 행이 아니라 조각으로 들고 있어서다.
func TestDiffToggleKeepsHunk(t *testing.T) {
	before := splitDiffLines(strings.Repeat("x\n", 30) + "end")
	after := splitDiffLines("A\n" + strings.Repeat("x\n", 28) + "B\nend")

	m := newDiffView(t, "", "")
	m.left.lines, m.right.lines = before, after
	m.hunks = buildDiffHunks(before, after)
	require.Len(t, m.hunks, 2)

	m.cursor = diffPlace{hunk: 1, row: 2}

	after1, ok := pressKeys(m, "tab").(viewDiff)
	require.True(t, ok)

	assert.True(t, after1.side, "나란히 보기로 넘어간다")
	assert.Equal(t, 1, after1.cursor.hunk, "보던 조각이 그대로다")
	assert.Equal(t, 2, after1.cursor.row)
}

// 고친 자리를 지나 아래로 계속 내려간다.
//
// **`j` 가 통째로 먹지 않던 자리다.** 고친 자리는 unified 에서 두 행인데, 아랫줄(`+`) 로
// 내려가자마자 자리를 되짚는 쪽이 윗줄(`-`) 을 먼저 집어서 커서를 끌어올렸다. 화면에서는
// 키가 아예 안 먹는 것으로 보인다(indexOf).
func TestDiffMovesThroughChangedRow(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")

	places := m.layout()
	require.Equal(t, diffRowChanged, m.hunks[0].rows[1].kind)

	// 커서는 제목줄 다음, 곧 첫 내용 행에서 시작한다.
	require.Equal(t, 1, m.indexOf(places, m.cursor))

	var model tea.Model = m
	for at := 2; at < len(places); at++ {
		next, ok := pressKeys(model, "j").(viewDiff)
		require.True(t, ok)

		assert.Equal(t, at, next.indexOf(places, next.cursor), "한 번에 한 행씩 내려간다")

		model = next
	}
}

// 누르면 그 행으로 커서가 간다. 열지는 않는다(ADR-0012).
func TestDiffClickMovesCursor(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")

	places := m.layout()
	require.Greater(t, len(places), 3)

	next, _ := m.Update(click(0, 3+jobsTitleHeight))

	after, ok := next.(viewDiff)
	require.True(t, ok, "판에 그대로 있는다")
	assert.Equal(t, places[3], after.cursor)
}

// 담긴 것보다 아래를 누르면 아무 일도 없다. 빈 행에는 고를 자리가 없다.
func TestDiffClickBelowContentDoesNothing(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")
	m.cursor = diffPlace{hunk: 0, row: 0}

	next, _ := m.Update(click(0, m.listHeight()+jobsTitleHeight))

	after, ok := next.(viewDiff)
	require.True(t, ok)
	assert.Equal(t, diffPlace{hunk: 0, row: 0}, after.cursor)
}

// 제목줄을 눌러도 아무 일도 없다. 고를 행이 아니다.
func TestDiffClickOnTitleDoesNothing(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")
	m.cursor = diffPlace{hunk: 0, row: 1}

	next, _ := m.Update(click(0, 0))

	after, ok := next.(viewDiff)
	require.True(t, ok)
	assert.Equal(t, diffPlace{hunk: 0, row: 1}, after.cursor)
}

// `]c` 는 다음 조각으로 가고 끝에서 감아 돈다(ADR-0095 §3).
func TestDiffJumpsBetweenHunks(t *testing.T) {
	before := splitDiffLines(strings.Repeat("x\n", 30) + "end")
	after := splitDiffLines("A\n" + strings.Repeat("x\n", 28) + "B\nend")

	m := newDiffView(t, "", "")
	m.left.lines, m.right.lines = before, after
	m.hunks = buildDiffHunks(before, after)
	require.Len(t, m.hunks, 2)

	next, ok := pressKeys(m, "]", "c").(viewDiff)
	require.True(t, ok)
	assert.Equal(t, 1, next.cursor.hunk)

	wrapped, ok := pressKeys(next, "]", "c").(viewDiff)
	require.True(t, ok)
	assert.Equal(t, 0, wrapped.cursor.hunk, "끝에서 감아 돈다")
	assert.Contains(t, wrapped.notice, "처음으로 돌아옴")
}

// `]` 뒤에 짝이 아닌 키가 오면 먹은 것을 버리고 그 키를 그대로 본다.
func TestDiffPrefixDropsUnpairedKey(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")

	after, ok := pressKeys(m, "]", "j").(viewDiff)
	require.True(t, ok)

	assert.Empty(t, after.pending, "기다리던 것을 놓는다")
	assert.Equal(t, 2, after.indexOf(after.layout(), after.cursor), "`j` 가 그대로 먹는다")
}

// unified 는 양쪽 줄번호를 다 적는다. `enter` 가 어느 줄로 가는지가 화면에 있어야 한다.
func TestDiffUnifiedShowsBothNumbers(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")

	screen := diffScreen(m)

	assert.Contains(t, screen, "@@ -1,3 +1,3 @@")
	assert.Contains(t, screen, "- b")
	assert.Contains(t, screen, "+ B")
}

// side-by-side 는 한 행에 좌우를 담고 가운데를 구분선으로 가른다.
func TestDiffSideBySideSplitsRow(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")
	m.side = true

	for _, line := range strings.Split(diffScreen(m), "\n") {
		if strings.Contains(line, "b") && strings.Contains(line, "B") {
			assert.Contains(t, line, boxUnicode.vertical, "가운데에 구분선이 선다")

			return
		}
	}

	t.Fatal("좌우를 한 행에 담은 줄이 없다")
}

// 좁아도 나눠 그리고 줄을 자른다. 고른 보기를 화면이 되돌리지 않는다(ADR-0140 §8).
func TestDiffSideBySideStaysWhenNarrow(t *testing.T) {
	m := newDiffView(t, "a\naaaaaaaaaaaaaaaaaaaaaaaaaaaa\nc", "a\nbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nc")
	m.side = true
	m.width = 30

	screen := diffScreen(m)

	assert.Contains(t, screen, boxUnicode.vertical, "여전히 나뉘어 있다")

	// **칸으로 잰다.** 글자 수가 아니다. 한글이 두 칸이라 글자로 세면 넘치는 줄을 놓친다.
	for _, line := range strings.Split(screen, "\n") {
		assert.LessOrEqual(t, textarea.WidthOf(line), m.width, "어느 행도 넘치지 않는다")
	}
}

// 그린 행은 폭을 꽉 채운다. 바탕이 줄 끝까지 이어져야 두 열의 경계가 선다(ADR-0140 §6).
func TestDiffTextFillsWidth(t *testing.T) {
	paint := diffPaint{base: colorDiffAdded, strong: colorDiffAddedStrong}

	short := renderDiffText([]byte("ab"), nil, nil, paint, diffTabWidth, 10)
	assert.Equal(t, 10, len(ansi.Strip(short)))

	long := renderDiffText([]byte(strings.Repeat("x", 40)), nil, nil, paint, diffTabWidth, 10)
	assert.Equal(t, 10, len(ansi.Strip(long)), "넘치면 자른다")
}

// 바탕과 글자 구간의 색이 서로 다르다. 한 단계 진한 것이 달라진 글자다(ADR-0140 §7).
func TestDiffTextPaintsChangedBytes(t *testing.T) {
	paint := diffPaint{base: colorDiffAdded, strong: colorDiffAddedStrong}

	line := renderDiffText([]byte("abcd"), nil, []diffSpan{{start: 1, end: 3}}, paint, diffTabWidth, 8)

	assert.Equal(t, "abcd    ", ansi.Strip(line), "글자는 그대로이고 뒤가 채워진다")
	assert.NotEqual(t, ansi.Strip(line), line, "색이 붙어 있다")
	assert.Greater(t, strings.Count(line, "\x1b["), 2, "바탕이 구간에서 갈린다")
}

// 들어낸 줄에 서서 `enter` 를 누르면 그 자리에 남은 줄로 간다(ADR-0140 §5).
func TestDiffCursorLineFallsBack(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nc")

	require.Len(t, m.hunks, 1)
	require.Equal(t, diffRowRemoved, m.hunks[0].rows[1].kind)

	m.cursor = diffPlace{hunk: 0, row: 1}

	line, ok := m.cursorLine()
	require.True(t, ok)
	assert.Equal(t, 0, line, "지운 줄 앞의 오른쪽 줄이다")
}

// 읽어 오기 전에는 그렇다고 적는다. 빈 판을 두면 견줄 것이 없는 것처럼 보인다.
func TestDiffViewWhileReading(t *testing.T) {
	m := newDiffView(t, "a", "b")
	m.ready, m.hunks = false, nil

	assert.Contains(t, diffScreen(m), "읽는 중")
}

// 다른 것이 없으면 판을 열지 않고 알린다.
func TestDiffViewClosesWhenSame(t *testing.T) {
	m := newDiffView(t, "", "")
	m.ready = false

	lines := splitDiffLines("a\nb")

	next, _ := m.Update(diffMsg{
		left:  diffSide{label: "HEAD", lines: lines},
		right: diffSide{label: "지금", lines: lines},
	})

	_, still := next.(viewDiff)
	assert.False(t, still, "판을 닫는다")
	assert.Contains(t, m.notice, "다른 것이 없습니다")
}

// 한글 상태로 친 키도 먹는다(ADR-0008).
func TestDiffViewTakesHangulKeys(t *testing.T) {
	m := newDiffView(t, "a\nb\nc", "a\nB\nc")

	after, ok := pressKeys(m, "ㅓ").(viewDiff)
	require.True(t, ok)

	assert.Equal(t, 2, after.indexOf(after.layout(), after.cursor), "첫 내용 행에서 한 칸 내려간다")
}
