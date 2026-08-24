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

	"github.com/bluemir/zn/internal/lsp"
)

// locationsFixture 는 파일 셋이 든 목록 화면이다. 셋 다 서로 다른 파일이라 어느 것을
// 골랐는지가 열린 tab 으로 드러난다.
func locationsFixture(t *testing.T) (viewLocations, []string) {
	t.Helper()

	dir := t.TempDir()

	paths := []string{}
	locations := []lsp.Location{}
	for _, name := range []string{"first.go", "second.go", "third.go"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc "+name[:5]+"() {}\n"), 0644))

		paths = append(paths, path)
		locations = append(locations, lsp.Location{
			URI:   "file://" + path,
			Range: lsp.Range{Start: lsp.Position{Line: 2, Character: 5}},
		})
	}

	buf, err := OpenBuffer(paths[0])
	require.NoError(t, err)

	e := &editor{buffers: []Buffer{buf}, width: 80, height: 20}

	model, _ := locationsMode(e, "정의 후보", locations)

	return model.(viewLocations), paths
}

func TestLocationsMoves(t *testing.T) {
	m, _ := locationsFixture(t)

	next, _ := m.press("j")
	m = next.(viewLocations)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("j")
	m = next.(viewLocations)
	assert.Equal(t, 2, m.selected)

	// 끝에서 멈춘다. 둘러 가면 목록의 끝이 어디인지 알 수 없다.
	next, _ = m.press("j")
	m = next.(viewLocations)
	assert.Equal(t, 2, m.selected)

	next, _ = m.press("k")
	m = next.(viewLocations)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("g")
	m = next.(viewLocations)
	assert.Equal(t, 0, m.selected)

	next, _ = m.press("G")
	m = next.(viewLocations)
	assert.Equal(t, 2, m.selected)
}

// 한글 상태로 쳐도 움직인다. 입력줄이 없는 화면이라 파서가 되돌린다(ADR-0008).
func TestLocationsMovesInHangul(t *testing.T) {
	m, _ := locationsFixture(t)

	next, _ := m.press("ㅓ") // j
	m = next.(viewLocations)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("ㅏ") // k
	m = next.(viewLocations)
	assert.Equal(t, 0, m.selected)
}

// enter 는 고른 자리를 새 tab 으로 열고 normal 로 돌아간다.
func TestLocationsOpensSelected(t *testing.T) {
	m, paths := locationsFixture(t)

	next, _ := m.press("j")
	m = next.(viewLocations)

	model, cmd := m.press("enter")

	assert.IsType(t, viewEditorNormal{}, model)
	assert.NotNil(t, cmd)

	require.Len(t, m.buffers, 2)
	assert.Equal(t, paths[1], m.activeBuffer().path)
	assert.Equal(t, 2, m.activeBuffer().cursorLine)
	assert.Equal(t, 5, m.activeBuffer().cursorCol)
}

// q 와 esc 는 고르지 않고 나간다. tab 도 커서도 그대로다.
func TestLocationsLeaves(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, _ := locationsFixture(t)

			model, _ := m.press(key)

			assert.IsType(t, viewEditorNormal{}, model)
			assert.Len(t, m.buffers, 1, "tab 이 늘지 않는다")
			assert.Equal(t, 0, m.activeBuffer().cursorLine)
		})
	}
}

// 모르는 키는 아무 일도 하지 않는다. 목록에 갇히지 않는지가 요점이다.
func TestLocationsIgnoresUnknownKeys(t *testing.T) {
	m, _ := locationsFixture(t)

	model, cmd := m.press("z")

	assert.IsType(t, viewLocations{}, model)
	assert.Nil(t, cmd)
	assert.Equal(t, 0, model.(viewLocations).selected)
}

// 목록이 화면보다 길면 고른 자리를 따라 스크롤한다.
func TestLocationsScrolls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "many.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0644))

	locations := make([]lsp.Location, 0, 40)
	for i := range 40 {
		locations = append(locations, lsp.Location{
			URI:   "file://" + path,
			Range: lsp.Range{Start: lsp.Position{Line: i}},
		})
	}

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{buffers: []Buffer{buf}, width: 80, height: 12}
	model, _ := locationsMode(e, "정의 후보", locations)
	m := model.(viewLocations)

	height := m.listHeight()
	require.Greater(t, height, 0)
	require.Less(t, height, len(locations), "화면보다 목록이 길어야 재는 뜻이 있다")

	// 화면 끝까지는 top 이 움직이지 않는다.
	for range height - 1 {
		next, _ := m.press("j")
		m = next.(viewLocations)
	}
	assert.Equal(t, 0, m.top)

	// 한 칸 더 내려가면 그만큼만 밀린다.
	next, _ := m.press("j")
	m = next.(viewLocations)
	assert.Equal(t, 1, m.top)

	// 맨 끝으로 가면 마지막 화면이 된다.
	next, _ = m.press("G")
	m = next.(viewLocations)
	assert.Equal(t, len(locations)-height, m.top)
}

// 화면에 제목과 개수와 고른 표시가 보인다.
func TestLocationsView(t *testing.T) {
	m, _ := locationsFixture(t)

	view := m.View()
	content := ansi.Strip(view.Content)

	assert.Contains(t, content, "정의 후보  3 개")
	assert.Contains(t, content, "▸ ")
	// 경로가 화면보다 길면 왼쪽이 접히고 파일 이름과 줄 번호가 남는다.
	assert.Contains(t, content, "first.go:3", "줄 번호는 1 부터 센다")
	assert.Contains(t, content, "GOTO")
	assert.Contains(t, content, "enter 열기")

	require.NotNil(t, view.Cursor)
	assert.Equal(t, tea.CursorBlock, view.Cursor.Shape)
}

// 저장소 안의 파일은 상대 경로로, 홈 아래의 것은 `~` 로 줄여 적는다.
func TestShortenPath(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	assert.Equal(t, "core.go", shortenPath(filepath.Join(cwd, "core.go")))
	assert.Equal(t, filepath.Join("internal", "lsp", "client.go"),
		shortenPath(filepath.Join(cwd, "internal", "lsp", "client.go")))

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// 지금 자리 **밖**은 상대 경로로 만들지 않는다. `../../..` 로 시작하는 경로는 어디를
	// 가리키는지 읽기 어렵다.
	outside := filepath.Join(cwd, "..", "lsp", "client.go")
	assert.NotContains(t, shortenPath(outside), "..")

	assert.Equal(t, filepath.Join("~", "go", "pkg", "mod", "x.go"),
		shortenPath(filepath.Join(home, "go", "pkg", "mod", "x.go")))

	// 어느 쪽도 아니면 적힌 그대로다.
	assert.Equal(t, string(filepath.Separator)+"tmp", shortenPath(string(filepath.Separator)+"tmp"))
}

// 긴 경로는 왼쪽부터 접힌다. 오른쪽부터 자르면 고르는 데 쓰는 파일 이름과 줄 번호가 먼저 사라진다.
func TestTrimLeftToWidth(t *testing.T) {
	place := "/Users/bluemir/go/pkg/mod/charm.land/bubbletea/v2@v2.0.9/tea.go:603"

	assert.Equal(t, place, trimLeftToWidth(place, 200), "들어가면 그대로 둔다")

	short := trimLeftToWidth(place, 20)
	assert.LessOrEqual(t, screenWidthOf(short), 20)
	assert.True(t, strings.HasPrefix(short, "…"), "접힌 것이 보여야 한다")
	assert.True(t, strings.HasSuffix(short, "tea.go:603"), "파일 이름과 줄 번호가 남아야 한다")

	// 한글이 든 경로도 칸으로 센다. 두 칸짜리 글자가 경계에 걸려도 넘치지 않는다.
	korean := trimLeftToWidth("/집/가나다라마바사/파일.go:12", 12)
	assert.LessOrEqual(t, screenWidthOf(korean), 12)
	assert.True(t, strings.HasSuffix(korean, ".go:12"))

	assert.Equal(t, "", trimLeftToWidth(place, 0))
}
