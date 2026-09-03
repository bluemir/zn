package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPaletteView 는 파일 목록을 직접 넣은 팔레트다. os.Getwd 와 git 을 타지 않아 결과가 고정된다.
func newPaletteView(t *testing.T, width, height int, files ...string) viewPalette {
	t.Helper()

	e := newTestEditor("a\nb\n", width, height).editor
	e.files = files

	m := viewPalette{editor: e}
	m.filter()

	return m
}

// boxRowsOf 는 화면에서 박스가 차지하는 칸만 떼어 색을 빼고 돌려준다.
// 박스는 sidebar 나 편집 내용 위에 겹쳐 있으므로 양옆을 잘라내야 박스만 남는다.
func boxRowsOf(t *testing.T, m viewPalette) []string {
	t.Helper()

	left, width := m.paletteLeft(), m.paletteWidth()

	rows := []string{}
	for _, row := range strings.Split(m.View().Content, "\n") {
		plain := ansi.Strip(row)
		if !strings.ContainsAny(plain, "┌│└├") {
			continue
		}

		cut := ansi.Truncate(ansi.TruncateLeft(plain, left, ""), width, "")
		if strings.HasPrefix(cut, "┌") || strings.HasPrefix(cut, "│") ||
			strings.HasPrefix(cut, "└") || strings.HasPrefix(cut, "├") {
			rows = append(rows, cut)
		}
	}

	return rows
}

func TestPaletteOpensWithCtrlP(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 20)

	m = send(m, "ctrl+p")

	assert.IsType(t, viewPalette{}, m)
	assert.Contains(t, barOf(t, m)[0], "PALETTE")
}

// sidebar 에서 열어도 끝나면 편집 영역으로 나온다.
func TestPaletteFromSidebarReturnsToNormal(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 20)

	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, m)

	m = send(m, "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(m, "esc")
	assert.IsType(t, viewEditorNormal{}, m)
}

// 박스를 얹어도 화면이 커지면 안 된다. 커지면 터미널이 줄을 흘려서 아래가 통째로 밀린다.
func TestPaletteDoesNotGrowScreen(t *testing.T) {
	plain := newTestEditor("a\nb\n", 80, 20)
	m := newPaletteView(t, 80, 20, "internal/core/edit.go", "main.go")

	before := strings.Split(plain.View().Content, "\n")
	after := strings.Split(m.View().Content, "\n")

	require.Equal(t, len(before), len(after))
	for i, row := range after {
		assert.LessOrEqual(t, widthOf(row), m.width, "행 %d", i)
	}
}

// 박스는 편집 영역이 아니라 화면 가운데다. sidebar 를 여닫아도 자리가 그대로여야 한다.
func TestPaletteBoxIsCenteredOnScreen(t *testing.T) {
	m := newPaletteView(t, 80, 20, "main.go")
	tree := newPaletteView(t, 80, 20, "main.go")
	tree.sidebar = openSidebarSync(t, newTreeFixture(t))

	for _, view := range []viewPalette{m, tree} {
		rows := boxRowsOf(t, view)
		require.NotEmpty(t, rows)

		// 박스가 그 자리에 있어야 boxRowsOf 가 테두리로 시작하는 행을 찾아낸다.
		assert.Equal(t, (80-view.paletteWidth())/2, view.paletteLeft())
		assert.True(t, strings.HasPrefix(rows[0], "┌"))

		for _, row := range rows {
			assert.Equal(t, view.paletteWidth(), widthOf(row))
		}
	}
}

// statusBar 는 팔레트가 떠도 그대로 화면 끝까지 이어진다.
func TestPaletteKeepsStatusBar(t *testing.T) {
	m := newPaletteView(t, 80, 20, "main.go")

	bar := barOf(t, m)

	require.Len(t, bar, statusBarHeight)
	assert.Contains(t, bar[0], "PALETTE")
	assert.Contains(t, bar[1], "1/1", "몇 개 중 몇 개가 걸렸는지")
}

func TestPaletteRefusesNarrowScreen(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 20, 20)

	m = send(m, "ctrl+p")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "화면이 좁아")
}

// 열린 뒤 좁아지면 나간다. 보이지 않는 mode 에 갇히면 키를 쳐도 아무 일이 안 난다.
func TestPaletteLeavesWhenScreenShrinks(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "main.go")

	m, _ = m.Update(tea.WindowSizeMsg{Width: 20, Height: 20})

	assert.IsType(t, viewEditorNormal{}, m)
}

func TestPaletteMovesSelectionWithinBounds(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "a.go", "b.go")

	m = send(m, "up")
	assert.Zero(t, m.(viewPalette).selected, "위 끝에서 멈춘다")

	m = send(m, "down", "down", "down")
	assert.Equal(t, 1, m.(viewPalette).selected, "아래 끝에서 멈춘다")
}

// 여는 키를 다시 눌러도 아무 일도 하지 않는다.
func TestPaletteIgnoresCtrlP(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "a.go", "b.go")
	m = send(m, "down")

	m = send(m, "ctrl+p")

	assert.Equal(t, 1, m.(viewPalette).selected)
}

func TestPaletteBackspaceOnEmptyLeaves(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "a.go")

	m = send(m, "a", "backspace")
	require.IsType(t, viewPalette{}, m)

	m = send(m, "backspace")
	assert.IsType(t, viewEditorNormal{}, m)
}

// `>` 는 별도 상태가 아니다. 지우면 파일 목록으로 저절로 돌아온다.
func TestPaletteSwitchesToCommandsWithAngle(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "a.go", "b.go")

	m = send(m, ">")
	// 여덟이 목록에 없다(paletteCommand.when). tab 이 하나뿐이라 tab 을 닫는 둘(「다른 tab 모두
	// 닫기」·「오른쪽 tab 모두 닫기」) 이 빠지고, 고른 범위가 없어서 「선택 영역 …」 넷과
	// 대소문자 둘이 빠진다.
	assert.Len(t, m.(viewPalette).hits, len(paletteCommands)-8)

	m = send(m, "t", "r", "e", "e")
	require.NotEmpty(t, m.(viewPalette).hits)
	// 이어진 `tree` 를 그대로 품은 것이 가장 앞이다. 자모가 흩어져 걸린 것들
	// (`regis`t`e`r 처럼) 도 목록에 남지만 점수가 낮다.
	assert.Equal(t, "파일 트리 열기/닫기", m.(viewPalette).commands()[m.(viewPalette).hits[0].index].name)

	m = send(m, "backspace", "backspace", "backspace", "backspace", "backspace")
	assert.Len(t, m.(viewPalette).hits, 2, "파일 목록으로 돌아온다")
}

// 명령을 고르면 그 자리에서 실행된다.
func TestPaletteRunsCommand(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "a.go")

	m = send(m, ">", "t", "r", "e", "e", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.True(t, m.(viewEditorNormal).sidebar.open, "파일 트리가 열렸다")
}

// 「검색 강조 끄기」는 `:noh` 와 같다. 강조만 끄고 마지막 검색은 남긴다.
func TestPaletteDisablesSearchHighlight(t *testing.T) {
	// 팔레트가 들어갈 만큼 넓어야 한다. 좁으면 `ctrl+p` 가 알림만 내고 끝난다.
	var m tea.Model = newTestEditor("foo bar\n", 80, 20)

	m = typeInto(m, "/foo")
	m = send(m, "enter")
	require.Contains(t, contentRowsOf(t, m)[0], styleSearchCurrent.Render("foo"))

	m = send(m, "ctrl+p")
	require.IsType(t, viewPalette{}, m)
	m = typeInto(m, ">noh")
	m = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.NotContains(t, contentRowsOf(t, m)[0], styleSearchCurrent.Render("foo"))

	m = send(m, "n")
	assert.Contains(t, barOf(t, m)[1], "아래에서 처음으로", "마지막 검색은 남아 있다")
}

// 입력줄이 있는 mode 라 한글은 글자다(ADR-0008).
func TestPaletteTakesHangulAsText(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "a.go")

	m = send(m, "ㅁ")

	assert.Equal(t, "ㅁ", m.(viewPalette).input.text)
}

// 두 칸 글자가 경계에 걸려도 행은 정확히 박스 폭이다.
func TestPaletteRowWidthWithWideChars(t *testing.T) {
	m := newPaletteView(t, 40, 20, strings.Repeat("한글", 20)+".go")

	for _, row := range boxRowsOf(t, m) {
		assert.Equal(t, m.paletteWidth(), widthOf(strings.TrimLeft(row, " ")))
	}
}

// 맞은 글자에만 색이 붙는다.
func TestPaletteHighlightsMatch(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "edit.go", "editor.go")
	m = send(m, "e", "d")

	// 고른 행은 반전만 쓰므로 아래 행에서 강조를 본다.
	m = send(m, "down")

	// 합성을 지나면 색 escape 가 다시 만들어져서 속성 순서가 달라진다. 색 번호와 글자로 본다.
	assert.Regexp(t, `38;5;214[^m]*med`, m.View().Content)
}

// 파일을 고르면 tab 으로 열린다. 이미 열려 있으면 그 tab 으로 간다(openTab).
func TestPaletteOpensFile(t *testing.T) {
	// cwd 가 이 package 디렉터리라 실제로 있는 파일을 쓴다.
	var m tea.Model = newPaletteView(t, 80, 20, "editor.go")

	m = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, m)
	normal := m.(viewEditorNormal)
	assert.Len(t, normal.buffers, 2)
	assert.Equal(t, "editor.go", normal.buffers[normal.active].Path)
}

// 없는 파일을 고르면 알리고 만다. 목록을 읽은 뒤에 지워졌을 수 있다.
func TestPaletteTellsWhenFileIsGone(t *testing.T) {
	var m tea.Model = newPaletteView(t, 80, 20, "없는-파일.go")

	m = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
	assert.NotEmpty(t, barOf(t, m)[1])
}

// newFilePalette 는 실제로 있는 파일을 연 buffer 위에 명령 목록을 띄운 팔레트다.
// 다시 읽기는 파일을 정말로 읽으므로 newTestEditor 의 없는 경로로는 볼 수 없다.
func newFilePalette(t *testing.T, path, input string) viewPalette {
	t.Helper()

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	m := viewPalette{
		editor: &editor{
			buffers: []viewport{buf},
			width:   80,
			height:  20 + tablineHeight + statusBarHeight,
		},
		input: newInputLine(input),
	}
	m.filter()

	// hits 의 자리는 commands() 안의 것이다. paletteCommands 로 집으면 when 이 거른 만큼 어긋난다.
	require.Equal(t, "파일 다시 읽기", m.commands()[m.hits[m.selected].index].name)

	return m
}

// 저장하지 않은 변경이 없으면 묻지 않고 다시 읽는다 (ADR-0016).
func TestPaletteReloadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	m := newFilePalette(t, path, "> reload")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	var next tea.Model = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, next)
	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, next).Line(0)))
	assert.Contains(t, barOf(t, next)[1], "다시 읽음")
}

// 저장하지 않은 변경이 있으면 한 번 더 묻는다. 팔레트 항목에는 `!` 를 붙일 자리가 없다.
func TestPaletteReloadAsksWhenDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	m := newFilePalette(t, path, "> reload")
	m.activeBuffer().Insert([]byte("X"))
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	confirm := send(m, "enter")
	require.IsType(t, viewConfirmDiscard{}, confirm)

	// No 로 취소하면 편집이 그대로 남고 팔레트가 아니라 normal 로 돌아간다.
	cancelled := send(confirm, "n", "enter")
	require.IsType(t, viewEditorNormal{}, cancelled)
	assert.Equal(t, "Xabc", string(bufferOf(t, cancelled).Line(0)))

	reloaded := send(confirm, "y", "enter")
	require.IsType(t, viewEditorNormal{}, reloaded)
	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, reloaded).Line(0)))
}

// 이름 없는 buffer 는 읽을 곳이 없다. 변경이 있어도 묻지 않고 알리고 만다 —
// Yes 를 눌러도 실패로 끝나는 확인창은 띄우지 않는다.
func TestPaletteReloadTellsWhenBufferHasNoName(t *testing.T) {
	m := viewPalette{
		editor: &editor{
			buffers: []viewport{newEmptyBuffer("")},
			width:   80,
			height:  20 + tablineHeight + statusBarHeight,
		},
		input: newInputLine("> reload"),
	}
	m.filter()
	m.activeBuffer().Insert([]byte("X"))

	var next tea.Model = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, next)
	assert.Contains(t, barOf(t, next)[1], "파일 이름이 없습니다")
}

// 여는 순간 목록을 다 읽고 기다리지 않는다. 인덱싱을 시작하고 바로 뜬다.
func TestPaletteStartsIndexingOnOpen(t *testing.T) {
	next, cmd := tea.Model(newTestEditor("abc\n", 80, 20)).Update(key("ctrl+p"))

	require.IsType(t, viewPalette{}, next)
	assert.NotNil(t, cmd, "인덱싱 작업이 시작된다")
	assert.True(t, next.(viewPalette).jobRunning("파일 인덱싱", nil))
}

// 인덱싱이 부은 파일도 치고 있는 패턴에 걸린다.
func TestPaletteGrowsWhileIndexing(t *testing.T) {
	m := newPaletteView(t, 80, 20, "main.go")
	m.input = newInputLine("go")
	m.filter()
	require.Len(t, m.hits, 1)

	found := progressOf("파일 인덱싱", 2, 2)
	found.apply = func(e *editor) { e.files = []string{"main.go", "edit.go"} }

	next, cmd := tea.Model(m).Update(found)

	require.IsType(t, viewPalette{}, next)
	assert.NotNil(t, cmd, "다음 조각을 받을 고리가 이어진다")
	assert.Len(t, next.(viewPalette).hits, 2)
}

// 파일이 붙을 때마다 고른 자리가 맨 위로 튀면 목록을 훑을 수 없다.
func TestPaletteKeepsSelectionWhileIndexing(t *testing.T) {
	m := newPaletteView(t, 80, 20, "a.go", "b.go", "c.go", "d.go")

	var moved tea.Model = send(m, "down", "down")
	require.Equal(t, 2, moved.(viewPalette).selected)

	found := progressOf("파일 인덱싱", 6, 6)
	found.apply = func(e *editor) { e.files = []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"} }

	next, _ := moved.Update(found)

	assert.Equal(t, 2, next.(viewPalette).selected, "고른 자리는 그대로다")
	assert.Len(t, next.(viewPalette).hits, 6)
}

// 닫았다 다시 열면 모아둔 목록부터 보인다. 인덱싱은 팔레트보다 오래 산다.
func TestPaletteReopenKeepsIndexedFiles(t *testing.T) {
	m := newPaletteView(t, 80, 20, "main.go", "edit.go")

	var back tea.Model = send(m, "esc")
	require.IsType(t, viewEditorNormal{}, back)

	again, _ := back.Update(key("ctrl+p"))

	require.IsType(t, viewPalette{}, again)
	assert.Equal(t, []string{"main.go", "edit.go"}, again.(viewPalette).files)
}

// visual 에서도 `ctrl+p` 로 연다. **고른 것을 두고 연다** — 상자 뒤로 그대로 칠해져 있다.
//
// 화면을 82 칸으로 잡는다. 상자는 가운데 64 칸이고(paletteMaxWidth) 왼쪽 gutter 는 8 칸이라
// (ADR-0086 의 마커 칸) 그 사이에 아래 화면이 한 칸 남는다. 80 칸이면 상자가 그 한 칸까지
// 덮어서 볼 자리가 없다.
func TestPaletteOpensFromVisualKeepingSelection(t *testing.T) {
	var m tea.Model = send(newTestEditor("foo bar\nbaz\n", 84, 20), "v", "l", "l")
	require.IsType(t, viewEditorVisual{}, m)

	m = send(m, "ctrl+p")

	require.IsType(t, viewPalette{}, m)
	assert.True(t, bufferOf(t, m).Selection.Active, "고른 것이 살아 있다")

	// 상자가 그 줄의 대부분을 덮어서 왼쪽 한 칸만 남는다. 그 한 칸이 선택 색이면 된다.
	assert.Contains(t, contentRowsOf(t, m)[0], styleSelection.Render("f"), "상자 뒤로 칠해져 있다")
}

// **고르는 순간 놓는다.** 무엇을 고르든 이 문을 지나므로 놓는 자리가 하나다.
func TestPaletteReleasesSelectionWhenPicking(t *testing.T) {
	var m tea.Model = send(newTestEditor("foo bar\nbaz\n", 80, 20), "v", "l", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = typeInto(m, ">tree")
	m = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.False(t, bufferOf(t, m).Selection.Active)
}

// `esc` 로 물러도 놓는다. normalMode 가 이미 놓는 자리다(ADR-0037).
func TestPaletteReleasesSelectionOnEscape(t *testing.T) {
	var m tea.Model = send(newTestEditor("foo bar\n", 80, 20), "v", "l", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(m, "esc")

	require.IsType(t, viewEditorNormal{}, m)
	assert.False(t, bufferOf(t, m).Selection.Active)
}

// 다른 파일을 열어도 **원래 tab 에 유령 강조가 남지 않는다.**
// normalMode 는 새 buffer 만 지우므로, 놓는 자리가 tab 이 바뀌기 전이어야 한다.
func TestPaletteReleasesSelectionBeforeOpeningAnotherFile(t *testing.T) {
	var m tea.Model = send(newPaletteView(t, 80, 20, "editor.go"), "esc")
	m = send(m, "v", "l")
	require.IsType(t, viewEditorVisual{}, m)

	from := m.(viewEditorVisual).active

	m = send(m, "ctrl+p")
	m = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, m)
	normal := m.(viewEditorNormal)
	require.NotEqual(t, from, normal.active, "다른 tab 으로 갔다")
	assert.False(t, normal.buffers[from].Selection.Active, "떠나온 tab 에 강조가 남지 않는다")
}

// ── 고른 범위를 받는 명령 (ADR-0111, ADR-0112) ──

// 「선택 영역 줄 정렬」은 고른 줄만 정렬한다. 놓기 전에 사본을 실어 보낸 결과다.
func TestPaletteSortsSelectedLinesOnly(t *testing.T) {
	// 앞 세 줄만 고른다. 넷째 줄은 정렬하면 맨 앞으로 갈 자리라 자리를 지키는지 드러난다.
	var m tea.Model = send(newTestEditor("c\nb\nd\na\n", 80, 20), "v", "j", "j", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(typeInto(m, ">sort selection"), "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, []string{"b", "c", "d", "a"}, linesOf(bufferOf(t, m)))
	assert.False(t, bufferOf(t, m).Selection.Active, "고른 것은 놓는다")
}

// 짝이 되는 「줄 정렬」은 파일 전체다. 범위가 없을 때만 목록에 선다.
func TestPaletteSortsWholeFileWithoutSelection(t *testing.T) {
	var m tea.Model = send(newTestEditor("c\nb\nd\na\n", 80, 20), "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(typeInto(m, ">sort lines"), "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, []string{"a", "b", "c", "d"}, linesOf(bufferOf(t, m)))
}

// 고른 줄 끝의 공백만 지운다. 「중복 공백 지우기」도 같은 손이다.
func TestPaletteTrimsSelectedLinesOnly(t *testing.T) {
	var m tea.Model = send(newTestEditor("a  \nb  \nc  \n", 80, 20), "v", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(typeInto(m, ">trim"), "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, []string{"a", "b  ", "c  "}, linesOf(bufferOf(t, m)))
}

// 대소문자 맞추기는 칸까지 본다. 줄로 넓히는 넷과 갈리는 자리다.
func TestPaletteChangesCaseOfSelection(t *testing.T) {
	var m tea.Model = send(newTestEditor("foo bar\n", 80, 20), "v", "l", "l", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(typeInto(m, ">upper"), "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, []string{"FOO bar"}, linesOf(bufferOf(t, m)))
	// 커서는 범위의 시작이다. visual 의 `U` 와 같다(ADR-0100).
	assert.Equal(t, 0, bufferOf(t, m).Cursor.Col)
}

// 짝이 되는 둘 중 하나만 목록에 선다. 무엇에 걸리는지가 고르기 전에 이름에 있다(ADR-0112).
func TestPaletteSplitsRangeCommandsBySelection(t *testing.T) {
	without := newPaletteView(t, 80, 20, "a.go")
	without.input = newInputLine(">")
	without.filter()

	names := make([]string, 0, len(without.commands()))
	for _, command := range without.commands() {
		names = append(names, command.name)
	}

	assert.Contains(t, names, "줄 정렬")
	assert.NotContains(t, names, "선택 영역 줄 정렬")

	m := send(newTestEditor("a\nb\n", 80, 20), "v", "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	with := m.(viewPalette)
	picked := make([]string, 0, len(with.commands()))
	for _, command := range with.commands() {
		picked = append(picked, command.name)
	}

	assert.Contains(t, picked, "선택 영역 줄 정렬")
	assert.NotContains(t, picked, "줄 정렬", "고른 범위가 있으면 전체에 거는 쪽은 사라진다")
	assert.Contains(t, picked, "선택 영역 표 맞추기")
	assert.NotContains(t, picked, "표 맞추기")
}

// 고른 범위가 없으면 대소문자 맞추기는 아예 목록에 없다. 파일 전체로 갈음하지 않는다.
func TestPaletteHidesCaseCommandsWithoutSelection(t *testing.T) {
	m := newPaletteView(t, 80, 20, "a.go")
	m.input = newInputLine(">")
	m.filter()

	names := make([]string, 0, len(m.commands()))
	for _, command := range m.commands() {
		names = append(names, command.name)
	}

	assert.NotContains(t, names, "대문자로 맞추기")
	assert.NotContains(t, names, "소문자로 맞추기")
	assert.Contains(t, names, "줄 정렬", "파일 전체로 갈음되는 것은 그대로 뜬다")
}

// 성립하지 않는 명령은 목록에 뜨지 않는다. 볼 파일이 없는 화면이 그 자리다.
func TestPaletteHidesCommandsThatDoNotHold(t *testing.T) {
	e := &editor{width: 80, height: 24, boxChars: boxUnicode, active: -1}
	m := viewPalette{editor: e, input: newInputLine(">")}
	m.filter()

	names := make([]string, 0, len(m.commands()))
	for _, command := range m.commands() {
		names = append(names, command.name)
	}

	assert.NotContains(t, names, "화면을 평문으로 내보내기")
	assert.NotContains(t, names, "특수문자 넣기")
	assert.NotContains(t, names, "정의로 가기")
	assert.Contains(t, names, "작업 목록", "파일과 무관한 것은 그대로 뜬다")
	assert.Contains(t, names, "프로젝트 검색", "빈 화면에서도 띄우기로 한 것이다")
}

// 읽기 전용 파일에서는 고치는 명령이 빠진다. 거절하는 조건을 그대로 옮겨 적은 결과다.
func TestPaletteHidesEditingCommandsOnReadOnly(t *testing.T) {
	m := newPaletteView(t, 80, 20, "a.go")
	m.activeBuffer().ReadOnly = true
	m.input = newInputLine(">")
	m.filter()

	names := make([]string, 0, len(m.commands()))
	for _, command := range m.commands() {
		names = append(names, command.name)
	}

	assert.NotContains(t, names, "줄 끝 공백 지우기")
	assert.NotContains(t, names, "특수문자 넣기")
	assert.NotContains(t, names, "오늘 날짜 넣기")
	assert.NotContains(t, names, "오늘 날짜와 시각 넣기")
	assert.Contains(t, names, "화면을 평문으로 내보내기", "내보내는 것은 파일을 안 건드린다")
}
