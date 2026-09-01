package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/buildinfo"
)

// newEmptyEditor 는 tab 이 하나도 없는 편집기다. 인자 없이 실행한 것과 같다.
//
// active 가 -1 인 것이 tab 이 없다는 뜻이다. 여기서 첫 tab 이 0 번 자리에 생긴다(ADR-0064).
func newEmptyEditor(width, height int) viewEditorEmpty {
	return viewEditorEmpty{
		editor: &editor{
			boxChars: boxUnicode,
			buffers:  nil,
			active:   -1,
			width:    width,
			height:   height + tablineHeight + statusBarHeight,
		},
	}
}

// 인자 없이 시작하면 tab 이 아니라 빈 화면이다. 가르는 자리는 normalMode 하나다.
func TestStartWithoutFilesShowsEmptyScreen(t *testing.T) {
	e := newEmptyEditor(80, 20).editor

	m, _ := normalMode(e)

	require.IsType(t, viewEditorEmpty{}, m)
}

// tabline 은 줄만 남고 비어 있다. 반전으로 채우면 아무것도 안 적힌 막대가 뜬다.
func TestEmptyScreenTablineIsBlank(t *testing.T) {
	m := newEmptyEditor(80, 20)

	plain, reversed := splitByReverse(rawTablineOf(t, m.View()))

	assert.Empty(t, plain, "적을 tab 이 없다")
	assert.Empty(t, reversed, "칠하지도 않는다")
}

func TestEmptyScreenShowsLogoAndVersion(t *testing.T) {
	m := newEmptyEditor(80, 20)

	content := m.View().Content

	assert.Contains(t, content, "ZINC", "원소 칸 로고를 그린다")
	assert.Contains(t, content, "65.38")
	assert.Contains(t, content, buildinfo.Describe(), "버전은 `:version` 과 같은 줄이다")
}

// 자리가 모자라면 자르지 않고 버린다. 폭이 좁으면 원소 칸이 이름 한 줄로 내려간다.
func TestEmptyScreenShrinks(t *testing.T) {
	narrow := newEmptyEditor(24, 20)

	content := narrow.View().Content
	assert.NotContains(t, content, "ZINC", "잘린 아스키 아트는 로고로 읽히지 않는다")
	assert.Contains(t, content, smallLogo)

	// 낮은 화면도 같다. 원소 칸은 열네 행이라 들어갈 자리가 없다.
	low := newEmptyEditor(80, 4)

	content = low.View().Content
	assert.NotContains(t, content, "ZINC")
	assert.Contains(t, content, buildinfo.Describe(), "마지막까지 남는 것은 버전 줄이다")
}

// 행 수와 폭은 편집 화면과 같아야 한다. 어긋나면 statusBar 와 sidebar 가 밀린다.
func TestEmptyScreenKeepsScreenShape(t *testing.T) {
	m := newEmptyEditor(80, 20)

	rows := strings.Split(m.View().Content, "\n")

	assert.Len(t, rows, 20+tablineHeight+statusBarHeight)
	for i, row := range rows {
		assert.LessOrEqual(t, widthOf(ansi.Strip(row)), 80, "%d 번째 행이 넘친다", i)
	}
}

// 빈 화면 tip 도 흐린 글씨다. 로고·버전보다 뒤로 물러나 있어야 한다(ADR-0061 §6).
func TestEmptyScreenTipIsDim(t *testing.T) {
	m := newEmptyEditor(80, 20)

	rows := strings.Split(m.View().Content, "\n")

	tip := ""
	for _, row := range rows {
		if strings.Contains(row, assets.Tips[0]) {
			tip = row
		}
	}
	require.NotEmpty(t, tip, "tip 줄이 있어야 한다")

	assert.Contains(t, tip, styleTip.Render(assets.Tips[0]), "색을 입힌다")

	// 가운데 정렬이 escape 를 폭으로 세면 tip 만 왼쪽으로 밀린다.
	left := len(tip) - len(strings.TrimLeft(tip, " "))
	assert.Equal(t, (80-widthOf(assets.Tips[0]))/2, left, "%q", tip)
}

// statusBar 에서 파일에 딸린 것들이 사라진다. `[No Name]` 도 적지 않는다.
func TestEmptyScreenStatusBarHasNoPath(t *testing.T) {
	m := newEmptyEditor(80, 20)

	bar := barOf(t, m)

	assert.True(t, strings.HasPrefix(bar[0], "NORMAL"), "mode 는 normal 이다: %q", bar[0])
	assert.NotContains(t, bar[0], "[No Name]", "없는 tab 이 있는 것처럼 보인다")
	assert.NotContains(t, bar[1], "줄)", "커서도 줄 수도 없다")
	assert.Empty(t, strings.TrimSpace(bar[1]), "본문이 이미 tip 을 보여주므로 아래 줄은 비어 있다")
}

// 커서는 세우지 않는다. 놓을 글자가 없다.
func TestEmptyScreenHasNoCursor(t *testing.T) {
	m := newEmptyEditor(80, 20)

	assert.Nil(t, m.View().Cursor)
}

// 편집하는 키는 조용히 아무 일도 하지 않는다. `g` 뒤에 짝이 없는 키와 같은 자리다.
//
// **activeBuffer 를 부르는 자리 일흔 곳에 대한 안전망이 이 검사다.** 키를 늘리기 쉽게 짠다.
func TestEmptyScreenIgnoresEditKeys(t *testing.T) {
	for _, keys := range [][]string{
		{"i"}, {"a"}, {"o"}, {"O"}, {"v"}, {"V"}, {"x"}, {"p"}, {"P"}, {"u"}, {"ctrl+r"},
		{"d", "d"}, {"c", "c"}, {"y", "y"}, {"g", "t"}, {"g", "T"}, {"g", "g"}, {"G"},
		{"3", "j"}, {"/"}, {"?"}, {"n"}, {"N"}, {"*"}, {"#"}, {"ctrl+d"}, {"ctrl+f"},
		{"r", "x"}, {"\"", "a", "y"}, {">", ">"},
	} {
		var m tea.Model = newEmptyEditor(80, 20)

		m = send(m, keys...)

		require.IsType(t, viewEditorEmpty{}, m, "%v", keys)
		assert.Empty(t, m.(viewEditorEmpty).buffers, "%v", keys)
	}
}

// 빈 화면에서 치는 `:q` 는 종료다. 닫으라는 것이 아니라 나가겠다는 뜻이다(ADR-0064).
func TestEmptyScreenQuitExits(t *testing.T) {
	var m tea.Model = newEmptyEditor(80, 20)

	m = send(m, ":", "q", "enter")

	assert.IsType(t, finalExit{}, m, "잃을 것이 없어 묻지도 않는다")
}

// 그래서 `:q` 를 두 번 치면 편집기가 끝난다. vim 에서 오는 손버릇이 그대로 산다.
func TestQuitTwiceExits(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "q", "enter")
	require.IsType(t, viewEditorEmpty{}, m)

	m = send(m, ":", "q", "enter")

	assert.IsType(t, finalExit{}, m)
}

func TestEmptyScreenQuitAllExits(t *testing.T) {
	var m tea.Model = newEmptyEditor(80, 20)

	m = send(m, ":", "q", "a", "enter")

	assert.IsType(t, finalExit{}, m)
}

func TestEmptyScreenCtrlCExits(t *testing.T) {
	var m tea.Model = newEmptyEditor(80, 20)

	m = send(m, "ctrl+c")

	assert.IsType(t, finalExit{}, m)
}

// 빈 화면에서 파일을 열면 편집 화면으로 나간다. 첫 tab 은 0 번 자리에 생긴다.
func TestEmptyScreenOpensFile(t *testing.T) {
	for _, keys := range [][]string{
		{":", "e", " ", "a", ".", "t", "x", "t", "enter"},
		{":", "t", "a", "b", "n", "e", "w", "enter"},
	} {
		var m tea.Model = newEmptyEditor(80, 20)

		m = send(m, keys...)

		require.IsType(t, viewEditorNormal{}, m, "%v", keys)

		normal := m.(viewEditorNormal)
		require.Len(t, normal.buffers, 1)
		assert.Equal(t, 0, normal.active)
	}
}

// 갈아끼울 tab 이 없으면 `:e` 는 새로 여는 것이라 잃을 것이 없다 — 확인창이 뜨지 않는다.
func TestEmptyScreenEditDoesNotConfirm(t *testing.T) {
	var m tea.Model = newEmptyEditor(80, 20)

	m = send(m, ":", "e", " ", "a", ".", "t", "x", "t", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, "a.txt", m.(viewEditorNormal).activeBuffer().path)
}

// buffer 가 있어야 하는 명령은 알리고 물러난다. 이름을 대고 친 것이라 조용하면 안 된다.
func TestEmptyScreenRefusesBufferCommands(t *testing.T) {
	for _, keys := range [][]string{
		{":", "w", "enter"},
		{":", "w", "q", "enter"},
		{":", "5", "enter"},
		{":", "d", "enter"},
		{":", "r", "e", "g", "enter"},
	} {
		var m tea.Model = newEmptyEditor(80, 20)

		m = send(m, keys...)

		require.IsType(t, viewEditorEmpty{}, m, "%v", keys)
		assert.Contains(t, barOf(t, m)[1], "열린 파일이 없습니다", "%v", keys)
	}
}

// 화면을 통째로 쓰는 판은 열린다. 편집기 틀을 쓰지 않아 볼 파일이 없어도 된다.
func TestEmptyScreenOpensFullScreenViews(t *testing.T) {
	var m tea.Model = newEmptyEditor(80, 20)
	assert.IsType(t, viewJobs{}, send(m, ":", "j", "o", "b", "s", "enter"))

	m = newEmptyEditor(80, 20)
	assert.IsType(t, viewMessages{}, send(m, ":", "m", "e", "s", "enter"))
}

// 트리로 포커스를 옮길 수 있다. 그때 편집 영역은 빈 화면 그대로다.
func TestEmptyScreenFocusesTree(t *testing.T) {
	root := newTreeFixture(t)

	empty := newEmptyEditor(80, 20)
	empty.sidebar = openSidebarSync(t, root)

	var m tea.Model = empty
	m = send(m, "ctrl+w", "ctrl+w")

	require.IsType(t, viewSidebar{}, m)
	assert.Contains(t, m.View().Content, "ZINC", "본문은 빈 화면이다")

	// 트리에서 돌아오면 다시 빈 화면이다.
	m = send(m, "ctrl+w", "ctrl+w")

	assert.IsType(t, viewEditorEmpty{}, m)
}

// 보고 있는 파일이 없으므로 굵은 행도 없다.
func TestEmptyScreenSidebarHasNoActivePath(t *testing.T) {
	root := newTreeFixture(t)

	m := newEmptyEditor(80, 20)
	m.sidebar = openSidebarSync(t, root)

	assert.Empty(t, m.activePath())
}

// 마지막 tab 을 닫아도 트리가 고른 자리는 그대로다. 닫은 사람은 대개 그 옆의 것을 열려는 참이다.
func TestCloseLastTabKeepsTreeSelection(t *testing.T) {
	root := newTreeFixture(t)

	normal := newTabsEditor("a.txt")
	normal.width = 80
	normal.sidebar = openSidebarSync(t, root)
	normal.sidebar.selectRow(2, normal.sidebarHeight())

	selected := normal.sidebar.selectedNode()
	require.NotNil(t, selected)

	var m tea.Model = normal
	m = send(m, ":", "q", "enter")

	require.IsType(t, viewEditorEmpty{}, m)
	assert.Same(t, selected, m.(viewEditorEmpty).sidebar.selectedNode(), "고른 자리가 그대로다")
}

// 팔레트의 「정의로 가기」도 볼 파일이 없으면 알리고 물러난다.
//
// 가드는 `\gd` 와 팔레트가 함께 지나는 gotoDefinition 에 있다(ADR-0064).
func TestEmptyScreenRefusesGotoDefinition(t *testing.T) {
	e := newEmptyEditor(80, 20).editor

	m, _ := runGotoDefinition(e)

	require.IsType(t, viewEditorEmpty{}, m)
	assert.Contains(t, barOf(t, m)[1], "열린 파일이 없습니다")
}
