package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/lsp"

	"github.com/bluemir/zn/internal/textarea"
)

// newSymbolView 는 큐레이션 표만 넣고 연 특수문자 drawer 다.
// 유니코드 훑기를 타지 않아 목록이 고정된다.
func newSymbolView(t *testing.T, width, height int) viewSymbol {
	t.Helper()

	e := newTestEditor("abc\n", width, height).editor
	e.symbols = assets.CuratedSymbols
	e.symbolsIndexed = true

	m, _ := symbolMode(e)
	require.IsType(t, viewSymbol{}, m, "열려야 한다")

	return m.(viewSymbol)
}

// drawerRowsOf 는 화면에서 drawer 가 차지하는 칸만 떼어 색을 뺀다.
// 판은 sidebar 오른쪽에 붙으므로 왼쪽을 잘라내야 판만 남는다.
func drawerRowsOf(t *testing.T, m viewSymbol) []string {
	t.Helper()

	left, width := m.sidebarLeft(), m.textWidth()

	rows := []string{}
	for _, row := range strings.Split(m.View().Content, "\n") {
		plain := ansi.Strip(row)

		cut := ansi.Truncate(ansi.TruncateLeft(plain, left, ""), width, "")
		if strings.HasPrefix(cut, "┌") || strings.HasPrefix(cut, "│") ||
			strings.HasPrefix(cut, "└") || strings.HasPrefix(cut, "├") {
			rows = append(rows, cut)
		}
	}

	return rows
}

// filterSymbol 은 패턴을 한 글자씩 쳐 넣는다. 한글은 글자 하나가 키 하나다.
func filterSymbol(t *testing.T, m viewSymbol, pattern string) viewSymbol {
	t.Helper()

	var model tea.Model = m
	for _, r := range pattern {
		model = send(model, string(r))
	}

	require.IsType(t, viewSymbol{}, model)

	return model.(viewSymbol)
}

// charsOf 는 지금 걸린 글자들이다.
func charsOf(m viewSymbol) []string {
	chars := []string{}
	for _, hit := range m.hits {
		chars = append(chars, m.symbols[hit.index].Char)
	}

	return chars
}

func TestSymbolOpensFromPalette(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 20)

	m = send(m, "ctrl+p", ">", "특", "수")
	require.IsType(t, viewPalette{}, m)

	m = send(m, "enter")

	assert.IsType(t, viewSymbol{}, m)
	assert.Contains(t, barOf(t, m)[0], "SYMBOL")
}

// `:symbols` 로도 연다. 다른 판들과 달리 한동안 팔레트로만 열 수 있었다(ADR-0056).
func TestSymbolOpensFromCommand(t *testing.T) {
	m := runCommand(newTestEditor("abc\n", 80, 20), "symbols")

	assert.IsType(t, viewSymbol{}, m)
	assert.Contains(t, barOf(t, m)[0], "SYMBOL")
}

// 인자도 줄 범위도 받지 않는다. 조용히 버리면 그것에 무언가를 한 것처럼 보인다.
func TestSymbolCommandTakesNothing(t *testing.T) {
	m := runCommand(newTestEditor("abc\n", 80, 20), "symbols foo")
	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "알 수 없는 명령")

	m = runCommand(newTestEditor("abc\n", 80, 20), "1,5symbols")
	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "줄 범위를 받지 않습니다")
}

// 빈 화면에서는 열지 않는다. 넣을 커서가 없다(ADR-0064).
func TestSymbolCommandRefusesEmptyScreen(t *testing.T) {
	m := runCommand(newEmptyEditor(80, 20), "symbols")

	assert.IsType(t, viewEditorEmpty{}, m)
}

// 판을 얹어도 화면이 커지면 안 된다. 커지면 터미널이 줄을 흘려서 아래가 통째로 밀린다.
func TestSymbolDoesNotGrowScreen(t *testing.T) {
	plain := newTestEditor("a\nb\n", 80, 20)
	m := newSymbolView(t, 80, 20)

	before := strings.Split(plain.View().Content, "\n")
	after := strings.Split(m.View().Content, "\n")

	require.Len(t, after, len(before), "행 수가 같아야 한다")

	for i, row := range after {
		assert.LessOrEqual(t, textarea.WidthOf(row), m.width, "%d 행이 화면보다 넓다", i)
	}
}

// statusBar 는 drawer 가 먹지 않는다. 판은 그 위에서 멈춘다.
func TestSymbolKeepsStatusBar(t *testing.T) {
	m := newSymbolView(t, 80, 20)

	assert.Contains(t, barOf(t, m)[0], "SYMBOL")
	assert.Contains(t, barOf(t, m)[0], "test.txt")
}

// 판이 편집 영역의 행을 가져가므로 커서는 늘 판 위에 있다.
// 얹기만 하면 파일이 길 때 넣는 자리가 판 밑에 가려진다(ADR-0056).
func TestSymbolKeepsCursorAboveDrawer(t *testing.T) {
	e := newTestEditor(strings.Repeat("line\n", 100), 80, 20).editor
	e.symbols, e.symbolsIndexed = assets.CuratedSymbols, true

	// 파일 끝으로 내려가서 연다. 커서가 편집 영역 맨 아래에 있는 상태다.
	e.activeBuffer().Cursor.Line = 99
	e.scrollToCursor()

	model, _ := symbolMode(e)
	m := model.(viewSymbol)

	row := m.View().Cursor.Y
	drawerTop := tablineHeight + m.textHeight()

	assert.Equal(t, drawerTop+1, row, "커서는 판 안 입력줄에 있다")

	// 편집 커서가 그려지는 행도 판 위여야 한다.
	buf := m.activeBuffer()
	assert.Less(t, buf.Cursor.Line-buf.Top.Line, m.textHeight(), "편집 커서가 판에 가리면 안 된다")
}

// drawer 는 편집 영역 아래에만 있다. 폭이 편집 영역과 같아서 트리 옆을 지나가지 않는다.
func TestSymbolDoesNotShrinkSidebar(t *testing.T) {
	e := newTestEditor("abc\n", 100, 20).editor
	e.symbols, e.symbolsIndexed = assets.CuratedSymbols, true

	before := e.sidebarHeight()

	model, _ := symbolMode(e)
	m := model.(viewSymbol)

	assert.Equal(t, before, m.sidebarHeight(), "트리 높이는 그대로다")
	assert.Less(t, m.textHeight(), before-tablineHeight, "편집 영역만 줄어든다")
}

// 못 그릴 화면에서는 열지 않는다. paletteFits 와 같은 방어다.
func TestSymbolRefusesShortScreen(t *testing.T) {
	e := newTestEditor("abc\n", 80, 4).editor
	e.symbols, e.symbolsIndexed = assets.CuratedSymbols, true

	model, _ := symbolMode(e)

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "좁아")
	assert.Zero(t, e.drawerHeight, "닫힌 채로 남아야 한다")
}

// 열려 있는 동안 좁아지면 빠져나온다. 보이지 않는 mode 에 갇히면 안 된다.
func TestSymbolLeavesWhenScreenShrinks(t *testing.T) {
	m := newSymbolView(t, 80, 20)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 4})

	assert.IsType(t, viewEditorNormal{}, next)
	assert.Zero(t, m.drawerHeight, "판이 닫혀야 편집 영역이 돌아온다")
}

// 이 기능의 기준 장면이다. 한국어 이름으로 삼각형을 걸러낸다.
func TestSymbolFiltersByKoreanName(t *testing.T) {
	m := filterSymbol(t, newSymbolView(t, 80, 20), "삼각형")

	require.NotEmpty(t, m.hits)
	assert.Subset(t, charsOf(m), []string{"▲", "△", "▼", "▽", "◀", "▶"})

	for _, hit := range m.hits {
		assert.Contains(t, m.symbols[hit.index].Label(), "삼각형")
	}
}

func TestSymbolFiltersByKoreanKeyword(t *testing.T) {
	m := filterSymbol(t, newSymbolView(t, 80, 20), "하트")

	assert.Subset(t, charsOf(m), []string{"❤", "💔", "💕"})
}

// 큐레이션 표에 적은 영문도 같이 걸린다. 한국어 이름을 모르고도 찾을 수 있어야 한다.
func TestSymbolFiltersByEnglishName(t *testing.T) {
	m := filterSymbol(t, newSymbolView(t, 80, 20), "arrow")

	assert.Subset(t, charsOf(m), []string{"←", "→", "↑", "↓"})
}

// 훑기가 끝나면 표에 없는 글자도 영문 이름으로 걸린다.
func TestSymbolFiltersScannedByEnglishName(t *testing.T) {
	e := newTestEditor("abc\n", 80, 20).editor
	e.symbols = append(append([]assets.Symbol{}, assets.CuratedSymbols...),
		assets.Symbol{Char: "⏥", Name: "FLATNESS"})
	e.symbolsIndexed = true

	model, _ := symbolMode(e)
	m := filterSymbol(t, model.(viewSymbol), "flatness")

	assert.Equal(t, []string{"⏥"}, charsOf(m))
}

// 고르면 커서 뒤에 들어간다. `a` 와 같은 자리다.
func TestSymbolInsertsAfterCursor(t *testing.T) {
	m := filterSymbol(t, newSymbolView(t, 80, 20), "삼각형")

	entry, ok := m.selectedSymbol()
	require.True(t, ok)

	next, _ := m.Update(key("enter"))

	assert.IsType(t, viewSymbol{}, next, "연달아 넣을 수 있게 열린 채다")
	assert.Equal(t, "a"+entry.Char+"bc", string(bufferOf(t, next).Line(0)))
}

// 연속으로 넣는 것이 이 mode 의 쓰임새다. enter 를 칠 때마다 이어 붙는다.
func TestSymbolInsertsRepeatedly(t *testing.T) {
	m := filterSymbol(t, newSymbolView(t, 80, 20), "삼각형")

	entry, ok := m.selectedSymbol()
	require.True(t, ok)

	var model tea.Model = m
	model = send(model, "enter", "enter", "enter")

	require.IsType(t, viewSymbol{}, model)
	assert.Equal(t, "a"+strings.Repeat(entry.Char, 3)+"bc", string(bufferOf(t, model).Line(0)))
}

// 연달아 넣은 것은 한 undo 단위다. insert mode 에서 이어 치는 것과 같다.
func TestSymbolInsertsUndoAsOne(t *testing.T) {
	m := filterSymbol(t, newSymbolView(t, 80, 20), "삼각형")

	var model tea.Model = m
	model = send(model, "enter", "enter", "esc")
	require.IsType(t, viewEditorNormal{}, model)

	model = send(model, "u")
	assert.Equal(t, "abc", string(bufferOf(t, model).Line(0)))
}

// 아무것도 안 넣고 나가면 `a<Esc>` 처럼 제자리다.
func TestSymbolEscapeKeepsCursor(t *testing.T) {
	e := newTestEditor("abc\n", 80, 20).editor
	e.symbols, e.symbolsIndexed = assets.CuratedSymbols, true
	e.activeBuffer().Cursor.Col = 1

	model, _ := symbolMode(e)
	next, _ := model.(viewSymbol).Update(key("esc"))

	assert.IsType(t, viewEditorNormal{}, next)
	assert.Equal(t, 1, bufferOf(t, next).Cursor.Col, "열기 전 자리로 돌아온다")
	assert.Equal(t, "abc", string(bufferOf(t, next).Line(0)))
	assert.Zero(t, e.drawerHeight)
}

// 다 지우면 나간다. 팔레트·명령줄과 같다.
func TestSymbolBackspaceOnEmptyLeaves(t *testing.T) {
	m := newSymbolView(t, 80, 20)

	next, _ := m.Update(key("backspace"))

	assert.IsType(t, viewEditorNormal{}, next)
}

// insert 에서 ctrl+p 로 팔레트를 열고 drawer 까지 갔다가 돌아와도 커서가 제자리다.
func TestSymbolRoundTripsFromInsert(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 20)

	m = send(m, "i", "X")
	require.IsType(t, viewEditorInsert{}, m)
	require.Equal(t, "Xabc", string(bufferOf(t, m).Line(0)))

	before := bufferOf(t, m).Cursor.Col

	m = send(m, "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = send(m, ">", "특", "수", "enter")
	require.IsType(t, viewSymbol{}, m)

	assert.Equal(t, before, bufferOf(t, m).Cursor.Col, "치던 자리가 그대로다")

	m = send(m, "esc")
	assert.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, "Xabc", string(bufferOf(t, m).Line(0)))
}

// 커서가 줄 맨 앞일 때만 한 칸 오른쪽에서 시작한다.
//
// 팔레트가 어디서 열렸는지 기억하지 않으므로(ADR-0011) insert 의 커서를 normal 자리로
// 맞췄다가 `a` 로 되돌리는데, 0 칸에서는 왼쪽으로 갈 자리가 없어서 되돌아오지 않는다.
// `i<Esc>a` 가 같은 자리에서 한 칸 움직이는 것과 같다(ADR-0056).
func TestSymbolShiftsOnceAtLineStart(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 20)

	m = send(m, "i")
	require.Equal(t, 0, bufferOf(t, m).Cursor.Col)

	m = send(m, "ctrl+p", ">", "특", "수", "enter")
	require.IsType(t, viewSymbol{}, m)

	assert.Equal(t, 1, bufferOf(t, m).Cursor.Col, "0 칸에서는 한 칸 오른쪽이다")
}

// 격자는 두 칸짜리 글자가 섞여도 밀리지 않는다.
func TestSymbolGridRowsAreEvenWidth(t *testing.T) {
	for _, pattern := range []string{"", "삼각형", "하트", "화살표"} {
		m := filterSymbol(t, newSymbolView(t, 80, 20), pattern)

		// 합성 전의 판을 잰다. 화면에서 떼어 오면 넘친 부분이 이미 잘려 있다.
		rows := strings.Split(m.renderDrawer(), "\n")
		require.Len(t, rows, m.symbolGridRows()+symbolFrame, "패턴 %q", pattern)

		for i, row := range rows {
			assert.Equal(t, m.textWidth(), textarea.WidthOf(row),
				"패턴 %q 의 %d 행: %q", pattern, i, ansi.Strip(row))
		}

		// 그 판이 화면에도 그대로 실렸는지 본다.
		assert.Len(t, drawerRowsOf(t, m), len(rows), "패턴 %q", pattern)
	}
}

// 좁은 편집 영역에서도 상자를 넘치지 않는다.
//
// 「일치하는 것이 없습니다」는 22 칸인데 판 안쪽은 그보다 좁을 수 있다.
// padTo 는 채우기만 하고 자르지 않아서 오른쪽 테두리가 밀려났었다.
func TestSymbolNoMatchRowFitsNarrowBox(t *testing.T) {
	// sidebar 를 연 좁은 화면이 이 자리다. 편집 영역이 화면에서 32 칸을 뺀 만큼이다.
	for _, width := range []int{20, 22, 25, 30, 40} {
		e := newTestEditor("abc\n", width, 20).editor
		e.symbols, e.symbolsIndexed = assets.CuratedSymbols, true

		if !e.symbolFits() {
			continue
		}

		model, _ := symbolMode(e)
		m := filterSymbol(t, model.(viewSymbol), "없는이름")
		require.Empty(t, m.hits, "폭 %d", width)

		// 합성하기 전의 판을 그대로 잰다. 화면에서 떼어 오면 넘친 부분이 이미
		// 잘려 있어서(drawerRowsOf 의 ansi.Truncate) 넘침 자체를 못 본다.
		for i, row := range strings.Split(m.renderDrawer(), "\n") {
			assert.Equal(t, m.textWidth(), textarea.WidthOf(row), "폭 %d 의 %d 행: %q", width, i, row)
		}
	}
}

// 다른 판으로 넘어가면 자리를 그 판에게 넘긴다.
//
// **예전에는 여기서 판을 닫았다.** 넘어가는 곳이 전체 화면이었기 때문인데, 정의 후보와
// 사용처가 하단 판이 되면서(ADR-0069) 닫으면 **새 판이 방금 잡은 높이를 우리가 지우게**
// 된다 — 판이 열린 채로 편집 영역만 온전해져서 목록이 편집 내용을 덮는다.
//
// 판이 아닌 곳으로 나가는 길은 normalMode 가 닫는다(아래).
func TestSymbolHandsDrawerToNextDrawer(t *testing.T) {
	m := newSymbolView(t, 80, 20)
	require.NotZero(t, m.drawerHeight)

	room := m.textAndDrawerHeight()

	// 정의 후보가 여럿이면 handleJob 이 목록 판을 돌려준다(gopls.go).
	next, _ := m.Update(definitionMsg{locations: []lsp.Location{
		{URI: "file:///a.go"},
		{URI: "file:///b.go"},
	}})

	list, ok := next.(viewLocations)
	require.True(t, ok, "다른 판으로 넘어가야 하는 시험이다")

	assert.Equal(t, list.locationsDrawerHeight(), m.drawerHeight, "새 판의 높이여야 한다")
	assert.Equal(t, room-list.locationsDrawerHeight(), m.textHeight())

	// 그 판에서 나가면 닫힌다. 닫는 자리가 normalMode 하나다.
	back, _ := normalMode(m.editor)
	require.IsType(t, viewEditorNormal{}, back)
	assert.Zero(t, back.(viewEditorNormal).drawerHeight, "판이 닫혀야 한다")
	assert.Equal(t, room, back.(viewEditorNormal).textHeight(), "편집 영역이 돌아와야 한다")
}

// 고른 칸은 글자 폭이 아니라 칸 전체를 반전한다.
//
// 글자 폭에 맞춰 칠하면 폰트가 셀보다 넓게 그리는 글자(`⬠` `①` `⑴`) 에서 앞 절반만
// 칠해져 보인다(docs/issues/0004). 칸의 경계는 글자 폭을 보지 않으므로 고른 표시도 보지
// 않는다.
func TestSymbolReverseCoversWholeCell(t *testing.T) {
	// 폭이 갈리는 셋이다. 어느 쪽이든 칸 전체가 칠해져야 한다.
	for _, entry := range []assets.Symbol{
		{Char: "⬠", Name: "WHITE PENTAGON"},       // Neutral, 한 칸
		{Char: "㊮", Name: "CIRCLED IDEOGRAPH"},    // Wide, 두 칸
		{Char: "֙", Name: "HEBREW ACCENT PASHTA"}, // 폭 0
	} {
		e := newTestEditor("abc\n", 80, 20).editor
		e.symbols = []assets.Symbol{entry}
		e.symbolsIndexed = true

		model, _ := symbolMode(e)
		require.IsType(t, viewSymbol{}, model, entry.Name)
		m := model.(viewSymbol)

		plain, reversed := splitByReverse(m.renderCell(m.selected))

		assert.Empty(t, plain, "%s: 칸에 칠하지 않은 자리가 없다", entry.Name)
		assert.Equal(t, symbolCellWidth, textarea.WidthOf(reversed), "%s", entry.Name)
		assert.True(t, strings.HasPrefix(reversed, " "),
			"%s: 글자 앞에 한 칸을 둬서 쏠려 보이지 않게 한다", entry.Name)
	}
}

// 폭 0 인 결합 문자는 ◌ 에 얹어 그린다.
//
// 혼자서는 자리를 차지하지 않아 앞 칸의 빈 칸에 달라붙고, 반전도 그 빈 칸의 것이 되어
// 고른 자리가 화면에서 사라졌다.
func TestSymbolZeroWidthSitsOnDottedCircle(t *testing.T) {
	e := newTestEditor("abc\n", 80, 20).editor
	e.symbols = []assets.Symbol{{Char: "֙", Name: "HEBREW ACCENT PASHTA"}}
	e.symbolsIndexed = true

	model, _ := symbolMode(e)
	require.IsType(t, viewSymbol{}, model)
	m := model.(viewSymbol)

	_, reversed := splitByReverse(m.renderCell(m.selected))

	assert.Contains(t, reversed, dottedCircle+"֙", "얹은 모양이 칸 안에 있다")
	assert.Equal(t, symbolCellWidth, textarea.WidthOf(reversed))

	rows := drawerRowsOf(t, m)
	assert.Contains(t, rows[len(rows)-2], dottedCircle+"֙", "이름줄에도 얹은 모양이다")
}

// 넣는 것은 원래 글자다. ◌ 는 판에 그리려고 얹은 것이라 파일에 들어가면 안 된다.
func TestSymbolInsertsWithoutDottedCircle(t *testing.T) {
	e := newTestEditor("abc\n", 80, 20).editor
	e.symbols = []assets.Symbol{{Char: "֙", Name: "HEBREW ACCENT PASHTA"}}
	e.symbolsIndexed = true

	model, _ := symbolMode(e)
	require.IsType(t, viewSymbol{}, model)

	next, _ := model.Update(key("enter"))

	assert.Equal(t, "a֙bc", string(bufferOf(t, next).Line(0)))
}

// 읽기 전용 파일은 고치지 않는다.
func TestSymbolRefusesReadOnly(t *testing.T) {
	e := newTestEditor("abc\n", 80, 20).editor
	e.symbols, e.symbolsIndexed = assets.CuratedSymbols, true
	e.activeBuffer().ReadOnly = true

	model, _ := symbolMode(e)

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "읽기 전용")
	assert.Zero(t, e.drawerHeight)
}
