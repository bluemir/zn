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

	// 테두리를 unicode 로 두는 것은 다른 두 판의 시험과 같은 약속이다(ADR-0028, ADR-0056).
	e := &editor{
		boxChars: boxUnicode,
		buffers:  []viewport{buf},
		width:    80,
		height:   20,
	}

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

// 판은 편집 영역의 행을 가져간다. 화면을 통째로 쓰지 않는다(ADR-0069).
func TestLocationsTakesDrawerRows(t *testing.T) {
	m, _ := locationsFixture(t)

	// 셋이면 목록 세 줄에 테두리 둘이다.
	assert.Equal(t, 3, m.locationsRows())
	assert.Equal(t, 5, m.drawerHeight)
	assert.Equal(t, m.textAndDrawerHeight()-5, m.textHeight(), "편집 영역이 그만큼 줄어든다")
}

// 담긴 것이 많아도 열여섯 줄에서 멈춘다. 판이 화면을 다 먹으면 간 자리가 안 보인다.
func TestLocationsStopsAtMaxRows(t *testing.T) {
	m := manyLocationsFixture(t, 40, 80, 40)

	assert.Equal(t, locationsMaxRows, m.locationsRows())
}

// **`j`/`k` 가 커서를 실제로 그 자리로 옮겨 보여준다**(ADR-0073).
func TestLocationsPreviewsWhileMoving(t *testing.T) {
	m, paths := locationsFixture(t)

	require.Equal(t, paths[0], m.activeBuffer().path)
	require.Equal(t, 0, m.activeBuffer().cursor.Line, "여는 것만으로는 옮기지 않는다")

	next, _ := m.press("j")
	m = next.(viewLocations)

	assert.Equal(t, 1, m.selected)
	assert.Equal(t, paths[1], m.activeBuffer().path, "그 자리를 보여준다")
	assert.Equal(t, 2, m.activeBuffer().cursor.Line)
	assert.Empty(t, m.jumps.places, "확정 전이라 이력에 담기지 않는다")

	next, _ = m.press("j")
	m = next.(viewLocations)
	assert.Equal(t, paths[2], m.activeBuffer().path)

	// 아래 끝에서 멈추면 커서도 그대로다.
	next, _ = m.press("j")
	m = next.(viewLocations)
	assert.Equal(t, 2, m.selected)
	assert.Equal(t, paths[2], m.activeBuffer().path)
}

// enter 는 **판을 닫으며 확정한다**(ADR-0073).
func TestLocationsConfirmsAndCloses(t *testing.T) {
	m, paths := locationsFixture(t)

	next, _ := m.press("j")
	m = next.(viewLocations)

	model, cmd := m.press("enter")

	require.IsType(t, viewEditorNormal{}, model, "판이 닫혀야 한다")
	assert.Zero(t, model.(viewEditorNormal).drawerHeight)
	assert.NotNil(t, cmd)

	assert.Equal(t, paths[1], m.activeBuffer().path)
	assert.Equal(t, 2, m.activeBuffer().cursor.Line)
	assert.Equal(t, 5, m.activeBuffer().cursor.Col)
}

// **q 와 esc 는 취소다.** 둘러본 것이 없던 일이 되어 판을 열기 전 자리로 돌아가고,
// 둘러보느라 연 tab 도 닫힌다(ADR-0073).
func TestLocationsCancelRestoresOrigin(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, paths := locationsFixture(t)

			room := m.textAndDrawerHeight()

			// 둘러본다. 커서가 실제로 움직이고 tab 도 열린다.
			next, _ := m.press("j")
			m = next.(viewLocations)
			next, _ = m.press("j")
			m = next.(viewLocations)
			require.Len(t, m.buffers, 3, "미리보기가 tab 을 열었다")

			model, _ := m.press(key)

			require.IsType(t, viewEditorNormal{}, model)
			assert.Zero(t, model.(viewEditorNormal).drawerHeight, "판이 닫혀야 한다")
			assert.Equal(t, room, model.(viewEditorNormal).textHeight(),
				"편집 영역이 돌아와야 한다")

			assert.Len(t, m.buffers, 1, "둘러보며 연 tab 이 닫힌다")
			assert.Equal(t, paths[0], m.activeBuffer().path, "열기 전 파일로 돌아온다")
			assert.Equal(t, 0, m.activeBuffer().cursor.Line, "열기 전 줄로 돌아온다")
			assert.Empty(t, m.jumps.places, "아무 데도 안 갔으니 이력도 비어 있다")
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

// manyLocationsFixture 는 같은 파일의 여러 줄이 든 판이다. 높이 계산과 스크롤을 재는 데 쓴다.
func manyLocationsFixture(t *testing.T, count, width, height int) viewLocations {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "many.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0644))

	locations := make([]lsp.Location, 0, count)
	for i := range count {
		locations = append(locations, lsp.Location{
			URI:   "file://" + path,
			Range: lsp.Range{Start: lsp.Position{Line: i}},
		})
	}

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{
		boxChars: boxUnicode,
		buffers:  []viewport{buf},
		width:    width,
		height:   height,
	}
	model, _ := locationsMode(e, "정의 후보", locations)

	return model.(viewLocations)
}

// 목록이 판보다 길면 고른 자리를 따라 스크롤한다.
func TestLocationsScrolls(t *testing.T) {
	locations := 40

	// 낮은 화면이라 남는 자리가 최대치보다 작다. 조용히 감추지 않고 훑게 한다.
	m := manyLocationsFixture(t, locations, 80, 12)

	height := m.locationsRows()
	require.Greater(t, height, 0)
	require.Less(t, height, locations, "판보다 목록이 길어야 재는 뜻이 있다")

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
	assert.Equal(t, locations-height, m.top)
}

// 화면에 제목과 개수와 고른 표시가 보인다.
//
// 「무엇을 고르는 중인가」는 판 안이 아니라 statusBar 아래 줄에 있다 — 판에 제목줄을 두면
// 목록에 쓸 행이 하나 줄고, 낮은 화면에서는 그 한 줄이 목록의 절반이다.
//
// 「몇 번째 중 몇 개인가」는 아랫 테두리가 든다. 다른 판 셋과 같은 자리다(ADR-0101).
func TestLocationsView(t *testing.T) {
	m, _ := locationsFixture(t)

	view := m.View()
	content := ansi.Strip(view.Content)

	assert.Contains(t, content, "정의 후보  j/k 둘러보기")
	assert.Contains(t, lastLine(ansi.Strip(m.renderDrawer())), "1/3")
	assert.Contains(t, content, "▸ ")
	// 경로가 화면보다 길면 왼쪽이 접히고 파일 이름과 줄 번호가 남는다.
	assert.Contains(t, content, "first.go:3", "줄 번호는 1 부터 센다")
	assert.Contains(t, content, "GOTO")
	assert.Contains(t, content, "enter 확정")

	require.NotNil(t, view.Cursor)
	assert.Equal(t, tea.CursorBlock, view.Cursor.Shape)
}

// 판의 모든 행이 정확히 편집 영역 폭이다. 한 행이라도 넘치면 그 아래가 통째로 밀린다.
// 기호 판·register 판과 같은 검사다.
func TestLocationsDrawerWidth(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120} {
		m := manyLocationsFixture(t, 20, width, 20)

		for i, row := range strings.Split(m.renderDrawer(), "\n") {
			assert.Equal(t, m.textWidth(), widthOf(row), "폭 %d 의 %d 행: %q", width, i, row)
		}
	}
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
	assert.LessOrEqual(t, widthOf(short), 20)
	assert.True(t, strings.HasPrefix(short, "…"), "접힌 것이 보여야 한다")
	assert.True(t, strings.HasSuffix(short, "tea.go:603"), "파일 이름과 줄 번호가 남아야 한다")

	// 한글이 든 경로도 칸으로 센다. 두 칸짜리 글자가 경계에 걸려도 넘치지 않는다.
	korean := trimLeftToWidth("/집/가나다라마바사/파일.go:12", 12)
	assert.LessOrEqual(t, widthOf(korean), 12)
	assert.True(t, strings.HasSuffix(korean, ".go:12"))

	assert.Equal(t, "", trimLeftToWidth(place, 0))
}

// **판을 열기 전 자리 하나만 이력에 담는다.** 몇 군데를 둘러봤든 `ctrl+o` 한 번이면
// 물어보던 자리로 돌아온다(ADR-0070, ADR-0073).
//
// 커서는 확정할 때 이미 둘러보던 곳에 가 있으므로, 담기는 것은 **적어 둔 자리**여야 한다.
func TestLocationsRecordsOriginOnConfirm(t *testing.T) {
	m, paths := locationsFixture(t)

	require.Equal(t, paths[0], m.activeBuffer().path)
	require.Equal(t, 0, m.activeBuffer().cursor.Line)
	require.Empty(t, m.jumps.places, "판을 여는 것만으로는 담지 않는다")

	// 세 군데를 둘러보고 마지막에서 확정한다.
	next, _ := m.press("j")
	m = next.(viewLocations)
	next, _ = m.press("j")
	m = next.(viewLocations)
	require.Empty(t, m.jumps.places, "둘러보는 동안에는 담기지 않는다")

	model, _ := m.press("enter")
	require.IsType(t, viewEditorNormal{}, model)

	require.Len(t, m.jumps.places, 1, "둘러본 만큼이 아니라 하나다")
	assert.Equal(t, paths[0], m.jumps.places[0].path, "판을 열기 전 자리다")
	assert.Equal(t, 0, m.jumps.places[0].line)

	// `ctrl+o` 한 번이면 물어보던 자리다.
	m.jumpBack()

	assert.Equal(t, paths[0], m.activeBuffer().path)
	assert.Equal(t, 0, m.activeBuffer().cursor.Line)
}

// **자모 하나가 키 여럿으로 풀릴 때 미리보기 Cmd 를 흘리지 않는다.**
//
// `접` 은 두벌식에서 `w`·`j`·`q` 다 — `j` 가 미리보기를 태우고 `q` 가 그 자리에서 나간다.
// 예전에는 나가는 길이 앞서 모은 Cmd 를 버렸는데, 그 안에 「파일을 연 뒤 언어 서버를
// 띄우는」 Cmd 가 있다. startServer 는 이미 그 서버의 `starting` 을 세워 두므로 버리면 서버가
// 영영 뜨지 않는다(ADR-0008, ADR-0051).
func TestLocationsKeepsPreviewCmdWhenLeavingMidKey(t *testing.T) {
	m, paths := locationsFixture(t)

	require.Equal(t, []string{"w", "j", "q"}, expandHangul("접"), "두벌식 자리를 먼저 확인한다")

	model, cmd := m.press("접")

	require.IsType(t, viewEditorNormal{}, model, "`q` 로 나가야 한다")

	// `j` 의 미리보기가 Go 파일을 열면서 서버를 띄우기 시작했다. 그 Cmd 가 같이 나와야 한다.
	require.True(t, m.serverState(lsp.ServerFor("main.go")).starting,
		"미리보기가 서버를 띄우기 시작한 상태여야 재는 뜻이 있다")
	assert.NotNil(t, cmd, "그 Cmd 를 흘리면 starting 이 참으로 굳어 서버가 영영 안 뜬다")

	// 나가는 길은 취소라 판을 열기 전 자리로 돌아온다.
	assert.Equal(t, paths[0], m.activeBuffer().path)
	assert.Equal(t, 0, m.activeBuffer().cursor.Line)
}
