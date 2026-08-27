package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// barOf 는 statusBar 두 줄 중 편집 영역 아래 부분만 색을 뺀 글자로 돌려준다.
// 색 자체를 보는 것은 rawBarOf 로, sidebar 아래 칸에 있는 mode 는 modeOf 로 본다.
//
// sidebar 가 열려 있으면 왼쪽 칸을 떼어낸다. 그래야 sidebar 를 여닫아도 같은 것을 본다.
func barOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	left := 0
	if e, ok := m.(interface{ sidebarLeft() int }); ok {
		left = e.sidebarLeft()
	}

	plain := make([]string, 0, statusBarHeight)
	for _, row := range rawBarOf(t, m) {
		line := []byte(ansi.Strip(row))
		plain = append(plain, string(line[offsetAtScreenCol(line, left):]))
	}

	return plain
}

// modeOf 는 statusBar 위 줄에 그려진 mode 다.
//
// sidebar 가 열려 있으면 그 아래 칸에 있고, 닫혀 있으면 경로 앞에 나란히 붙는다.
// 어느 쪽이든 위 줄 맨 앞이라 첫 낱말을 떼어내면 된다.
func modeOf(t *testing.T, m tea.Model) string {
	t.Helper()

	top := ansi.Strip(rawBarOf(t, m)[0])

	return strings.Fields(top)[0]
}

// rawBarOf 는 statusBar 두 줄을 색이 붙은 그대로 돌려준다.
func rawBarOf(t *testing.T, m tea.Model) []string {
	t.Helper()

	rows := strings.Split(m.View().Content, "\n")
	require.GreaterOrEqual(t, len(rows), statusBarHeight)

	return rows[len(rows)-statusBarHeight:]
}

// mode 이름은 각 mode 의 View 가 직접 넘긴다. 바깥에서 mode 를 물을 필요가 없다(ADR-0002).
func TestStatusBarShowsMode(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 3)

	assert.Equal(t, "NORMAL", modeOf(t, m))

	m = send(m, "i")
	assert.Equal(t, "INSERT", modeOf(t, m))

	m = send(m, "esc")
	assert.Equal(t, "NORMAL", modeOf(t, m))
}

// sidebar 가 열려 있으면 mode 는 그 아래 칸에, 경로는 편집 영역 아래에 선다.
func TestStatusBarPutsModeUnderSidebar(t *testing.T) {
	m := newTreeEditor(t, 80, 6)
	require.True(t, m.sidebarVisible())

	top := ansi.Strip(rawBarOf(t, m)[0])

	assert.True(t, strings.HasPrefix(top, "NORMAL"), "mode 는 화면 왼쪽 끝에서 시작한다")
	assert.Equal(t, sidebarWidth, strings.Index(top, "main.go"), "경로는 편집 영역 왼쪽 끝에 맞는다")
	assert.Equal(t, "main.go", strings.TrimSpace(barOf(t, m)[0]), "편집 영역 아래에는 경로만 있다")
}

// sidebar 를 놓을 칸이 없으면 mode 는 경로 앞에 나란히 붙는다.
func TestStatusBarKeepsModeInlineWithoutSidebar(t *testing.T) {
	m := newTreeEditor(t, sidebarWidth+minTextWidth-1, 6)
	require.False(t, m.sidebarVisible())

	assert.Equal(t, "NORMAL  main.go", strings.TrimRight(ansi.Strip(rawBarOf(t, m)[0]), " "))
}

// 아래 줄은 mode 를 따라가지 않는다. 위 줄의 경로와 세로로 맞아야 편집 중인 파일에 딸린 것으로 읽힌다.
func TestStatusBarBottomLineAlignsWithPath(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	top, bottom := ansi.Strip(rawBarOf(t, m)[0]), ansi.Strip(rawBarOf(t, m)[1])

	assert.Equal(t, strings.Index(top, "main.go"), strings.Index(bottom, "1:1"))
}

func TestStatusBarShowsPath(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)

	assert.Contains(t, barOf(t, m)[0], "test.txt")
}

func TestStatusBarShowsNoNameForEmptyPath(t *testing.T) {
	m := viewEditorNormal{
		editor: &editor{
			buffers: []Buffer{newEmptyBuffer("")},
			width:   40,
			height:  3 + tablineHeight + statusBarHeight,
		},
	}

	assert.Contains(t, barOf(t, m)[0], "[No Name]")
}

// 줄과 칸은 1 부터 세고, 칸은 화면 칸이라 한글은 2 씩 늘어난다.
func TestStatusBarShowsCursorPosition(t *testing.T) {
	var m tea.Model = newTestEditor("한글abc\nsecond\n", 40, 3)

	assert.Contains(t, barOf(t, m)[1], "1:1")

	m = send(m, "right")
	assert.Contains(t, barOf(t, m)[1], "1:3", "한글 한 글자 뒤는 3 칸")

	m = send(m, "down")
	assert.Contains(t, barOf(t, m)[1], "2:3")
}

func TestStatusBarShowsLineCount(t *testing.T) {
	m := newTestEditor("a\nb\nc\n", 40, 3)

	assert.Contains(t, barOf(t, m)[1], "3 줄")
}

// 숫자나 접두 키를 치는 동안 그것을 보여준다. vim 의 showcmd 와 같은 자리다.
func TestStatusBarShowsPendingKeys(t *testing.T) {
	var m tea.Model = newTestEditor("abc\nsecond\n", 40, 3)

	m = send(m, "3")
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], "3"), "모으는 중인 숫자가 오른쪽 끝에")
	assert.Contains(t, barOf(t, m)[1], "1:1", "커서 위치는 그대로 있다")

	m = send(m, "1")
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], "31"), "자릿수가 붙는다")

	m = send(m, "j")
	assert.False(t, strings.HasSuffix(barOf(t, m)[1], "31"), "동작이 끝나면 사라진다")

	m = send(m, "g")
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], "g"), "접두 키도 보인다")
}

// git 표시는 위 줄 오른쪽 끝이다. mode 와 파일 이름 반대편이라 서로 밀지 않는다.
func TestStatusBarShowsGitOnTheRight(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)
	m.git = gitStatus{branch: "master", commit: "a1b2c3d"}

	top := barOf(t, m)[0]

	assert.True(t, strings.HasSuffix(strings.TrimRight(top, " "), "master(a1b2c3d)"))
	assert.Contains(t, top, "NORMAL", "mode 와 파일은 그대로 왼쪽에 있다")
	assert.Contains(t, top, "test.txt")
}

func TestStatusBarShowsGitDirtyMark(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)
	m.git = gitStatus{branch: "master", commit: "a1b2c3d", dirty: true}

	assert.Contains(t, barOf(t, m)[0], "master(a1b2c3d*)")
}

// detached HEAD 는 가리킬 branch 가 없어서 해시만 찍는다.
func TestStatusBarShowsGitDetachedHead(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)
	m.git = gitStatus{commit: "a1b2c3d"}

	assert.Contains(t, barOf(t, m)[0], "a1b2c3d")
	assert.NotContains(t, barOf(t, m)[0], "(", "빈 괄호를 두지 않는다")
}

// 저장소가 아니면 아무것도 찍지 않는다. git 이 없는 곳에서도 편집기는 그대로 열린다.
func TestStatusBarOmitsGitOutsideRepository(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)

	assert.Empty(t, gitStatus{}.label())
	assert.Equal(t, "NORMAL  test.txt", strings.TrimRight(barOf(t, m)[0], " "))
}

// 붙일 칸이 없으면 mode 와 파일 이름이 먼저다. git 을 넣겠다고 그것을 밀어내지 않는다.
func TestStatusBarSkipsGitWhenNarrow(t *testing.T) {
	m := newTestEditor("abc\n", 24, 3)
	m.git = gitStatus{branch: "very-long-branch-name", commit: "a1b2c3d"}

	top := barOf(t, m)[0]

	assert.Equal(t, "NORMAL  test.txt", strings.TrimRight(top, " "))
	assert.LessOrEqual(t, screenColAt([]byte(top), len(top)), 24)
}

// 붙일 칸이 없으면 아래 줄을 그대로 둔다. 커서 위치가 밀려나는 것이 더 나쁘다.
func TestStatusBarSkipsShowcmdWhenNarrow(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 12, 3)

	before := barOf(t, m)[1]
	m = send(m, "3")

	assert.Equal(t, before, barOf(t, m)[1])
}

// statusBar 가 화면 너비를 넘으면 터미널이 줄바꿈해서 화면이 밀린다.
func TestStatusBarTruncatesToWidth(t *testing.T) {
	m := viewEditorNormal{
		editor: &editor{
			buffers: []Buffer{newBuffer(strings.Repeat("long-path/", 20)+"file.txt", []byte("abc\n"))},
			width:   20,
			height:  3 + tablineHeight + statusBarHeight,
		},
	}

	for _, row := range barOf(t, m) {
		assert.LessOrEqual(t, screenColAt([]byte(row), len(row)), 20)
	}
}

// 파일이 화면보다 짧아도 statusBar 는 화면 아래에 붙어 있어야 한다.
func TestStatusBarStaysAtBottom(t *testing.T) {
	m := newTestEditor("only\n", 40, 5)

	rows := strings.Split(m.View().Content, "\n")

	require.Len(t, rows, tablineHeight+5+statusBarHeight)
	assert.Equal(t, "only", strings.Split(textOf(t, m), "\n")[0])
	assert.Contains(t, rows[tablineHeight+5], "NORMAL", "빈 줄로 채우고 맨 아래에 붙인다")
}

// tabline 과 statusBar 가 자리를 차지하므로 편집 내용 높이는 그만큼 줄어든다.
func TestTextHeightExcludesTablineAndStatusBar(t *testing.T) {
	m := newTestEditor("", 40, 0)
	m.height = 10

	assert.Equal(t, 10-tablineHeight-statusBarHeight, m.textHeight())
}

// statusBar 위 줄은 편집 내용과 눈으로 구분되어야 한다.
// 색을 정하지 않고 터미널의 전경·배경을 뒤집기만 한다(ADR-0004).
func TestStatusBarTopLineIsReversed(t *testing.T) {
	m := newTestEditor("abc\n", 40, 3)

	plain, reversed := splitByReverse(rawBarOf(t, m)[0])

	assert.Empty(t, plain, "위 줄은 통째로 반전이다")
	assert.Contains(t, reversed, "NORMAL")
	assert.Equal(t, 40, screenColAt([]byte(reversed), len(reversed)), "빈 칸까지 줄 끝을 채운다")
}

// 아래 줄은 vim 처럼 명령줄이라 배경을 그대로 둔다.
// `:` 를 칠 때 배경이 뜨지 않고 명령 결과와 오류도 평범한 글자로 읽힌다.
//
// 색이 붙는 자리는 오른쪽 끝 tip 하나다. 그것도 배경이 아니라 흐린 글자색이다(ADR-0061 §6).
func TestStatusBarBottomLineIsPlain(t *testing.T) {
	// tip 이 설 만큼 넓게 연다. 좁으면 아예 붙지 않아서(fitTip) 색이 하나도 없는 줄이 된다 —
	// 그것은 이 검사가 보려는 것이 아니다.
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	bottom := rawBarOf(t, m)[1]
	before, _, _ := strings.Cut(bottom, "\x1b[")

	assert.Equal(t, "1:1  (1 줄)", strings.TrimRight(before, " "), "커서 위치에는 색이 없다")
	assert.Equal(t, styleTip.Render(assets.Tips[0]), bottom[len(before):], "색이 붙는 것은 tip 뿐이다")

	m = send(m, ":", "w", "q")

	assert.Equal(t, ":wq", rawBarOf(t, m)[1], "명령줄에도 색이 없다")
}

// truncateToWidth 는 escape 를 폭으로 세지 않는다. 세면 색을 입힌 줄이 그만큼 일찍 잘린다.
func TestTruncateSkipsEscapes(t *testing.T) {
	line := "가나" + styleTip.Render("다라")

	assert.Equal(t, line, truncateToWidth(line, 8), "여덟 칸에 다 들어간다")
	assert.Equal(t, "가나\x1b[38;5;244m다"+ansi.ResetStyle, truncateToWidth(line, 6),
		"자를 때는 색을 끄고 끝낸다")
	assert.Equal(t, "가", truncateToWidth(line, 3), "색이 시작되기 전에 잘리면 그대로다")
}

// 편집 내용에는 색이 가지 않는다.
func TestTextAreaHasNoColor(t *testing.T) {
	m := newTestEditor("abc\ndef\n", 40, 3)

	assert.NotContains(t, textOf(t, m), "\x1b[")
}

// 화면이 statusBar 보다 작아도 죽지 않아야 한다.
func TestTinyScreenDoesNotPanic(t *testing.T) {
	for _, height := range []int{0, 1, 2, 3} {
		m := viewEditorNormal{
			editor: &editor{
				buffers: []Buffer{newBuffer("t", []byte("a\nb\n"))},
				width:   10,
				height:  height,
			},
		}

		assert.NotPanics(t, func() { m.View() }, "height=%d", height)
	}
}
