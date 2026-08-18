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
		assert.LessOrEqual(t, ansi.StringWidth(row), m.width, "행 %d", i)
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
			assert.Equal(t, view.paletteWidth(), ansi.StringWidth(row))
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
	assert.Len(t, m.(viewPalette).hits, len(paletteCommands))

	m = send(m, "t", "r", "e", "e")
	require.Len(t, m.(viewPalette).hits, 1)
	assert.Equal(t, "파일 트리 열기/닫기", paletteCommands[m.(viewPalette).hits[0].index].name)

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

	assert.Equal(t, "ㅁ", m.(viewPalette).input)
}

// 두 칸 글자가 경계에 걸려도 행은 정확히 박스 폭이다.
func TestPaletteRowWidthWithWideChars(t *testing.T) {
	m := newPaletteView(t, 40, 20, strings.Repeat("한글", 20)+".go")

	for _, row := range boxRowsOf(t, m) {
		assert.Equal(t, m.paletteWidth(), ansi.StringWidth(strings.TrimLeft(row, " ")))
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
	assert.Equal(t, "editor.go", normal.buffers[normal.active].path)
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
			buffers: []Buffer{buf},
			width:   80,
			height:  20 + tablineHeight + statusBarHeight,
		},
		input: input,
	}
	m.filter()

	require.Equal(t, "파일 다시 읽기", paletteCommands[m.hits[m.selected].index].name)

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
	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, next).lines[0]))
	assert.Contains(t, barOf(t, next)[1], "다시 읽음")
}

// 저장하지 않은 변경이 있으면 한 번 더 묻는다. 팔레트 항목에는 `!` 를 붙일 자리가 없다.
func TestPaletteReloadAsksWhenDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	m := newFilePalette(t, path, "> reload")
	m.activeBuffer().insert([]byte("X"), m.contentWidth())
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	confirm := send(m, "enter")
	require.IsType(t, viewConfirmDiscard{}, confirm)

	// No 로 취소하면 편집이 그대로 남고 팔레트가 아니라 normal 로 돌아간다.
	cancelled := send(confirm, "n", "enter")
	require.IsType(t, viewEditorNormal{}, cancelled)
	assert.Equal(t, "Xabc", string(bufferOf(t, cancelled).lines[0]))

	reloaded := send(confirm, "y", "enter")
	require.IsType(t, viewEditorNormal{}, reloaded)
	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, reloaded).lines[0]))
}

// 이름 없는 buffer 는 읽을 곳이 없다. 변경이 있어도 묻지 않고 알리고 만다 —
// Yes 를 눌러도 실패로 끝나는 확인창은 띄우지 않는다.
func TestPaletteReloadTellsWhenBufferHasNoName(t *testing.T) {
	m := viewPalette{
		editor: &editor{
			buffers: []Buffer{newEmptyBuffer("")},
			width:   80,
			height:  20 + tablineHeight + statusBarHeight,
		},
		input: "> reload",
	}
	m.filter()
	m.activeBuffer().insert([]byte("X"), m.contentWidth())

	var next tea.Model = send(m, "enter")

	require.IsType(t, viewEditorNormal{}, next)
	assert.Contains(t, barOf(t, next)[1], "파일 이름이 없습니다")
}

// 여는 순간 목록을 다 읽고 기다리지 않는다. 인덱싱을 시작하고 바로 뜬다.
func TestPaletteStartsIndexingOnOpen(t *testing.T) {
	next, cmd := tea.Model(newTestEditor("abc\n", 80, 20)).Update(key("ctrl+p"))

	require.IsType(t, viewPalette{}, next)
	assert.NotNil(t, cmd, "인덱싱 작업이 시작된다")
	assert.True(t, next.(viewPalette).jobRunning("파일 인덱싱"))
}

// 인덱싱이 부은 파일도 치고 있는 패턴에 걸린다.
func TestPaletteGrowsWhileIndexing(t *testing.T) {
	m := newPaletteView(t, 80, 20, "main.go")
	m.input = "go"
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
