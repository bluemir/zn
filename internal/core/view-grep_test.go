package core

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newGrepView 는 적중이 담긴 결과 판이다. 파일은 jumpEditor 가 만든 셋을 쓴다.
//
// 적중은 실제로 있는 파일과 줄을 가리켜야 한다 — `j`/`k` 가 그 자리로 커서를 옮겨 보여주므로
// 없는 자리를 담으면 미리보기가 오류를 낸다(ADR-0078).
func newGrepView(t *testing.T, input string, hits ...grepHit) viewGrep {
	t.Helper()

	e, paths := jumpEditor(t)

	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	e.grep = grepResult{
		input:   input,
		pattern: regexp.MustCompile(regexp.QuoteMeta(input)),
		hits:    hits,
		files:   1,
	}

	model, _ := grepMode(e)

	here, ok := model.(viewGrep)
	require.True(t, ok, "판이 열려야 한다")

	return here
}

// grepHitsIn 은 그 파일의 줄들을 적중으로 만든다.
func grepHitsIn(path string, lines ...int) []grepHit {
	hits := make([]grepHit, 0, len(lines))
	for _, line := range lines {
		hits = append(hits, grepHit{path: path, line: line, text: "func a() {}"})
	}

	return hits
}

// grepDrawerRowsOf 는 판 안쪽 행만 색과 테두리를 빼고 돌려준다.
func grepDrawerRowsOf(t *testing.T, m viewGrep) []string {
	t.Helper()

	rows := strings.Split(ansi.Strip(m.renderDrawer()), "\n")
	require.Greater(t, len(rows), grepFrame)

	body := []string{}
	for _, row := range rows[1 : len(rows)-1] {
		trimmed := strings.Trim(row, m.boxChars.vertical)
		if plain := strings.TrimRight(trimmed, " "); strings.TrimSpace(plain) != "" {
			body = append(body, plain)
		}
	}

	return body
}

// 판은 편집 영역의 행을 가져간다. 다른 판들과 같다(ADR-0069).
//
// **적중이 셋이어도 높이는 그대로다.** 결과가 조각으로 도착하는 동안 판이 자라면서 본문을
// 밀어내지 않도록 처음부터 상한 높이다(grepRows).
func TestGrepTakesDrawerRows(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	m := newGrepView(t, "func", grepHitsIn(paths[0], 2, 4, 6)...)

	rows := min(grepMaxRows, m.paneHeight()-grepFrame-grepMinTextHeight)

	assert.Equal(t, rows, m.grepRows())
	assert.Equal(t, rows+grepFrame, m.drawerHeight)
	assert.Equal(t, m.paneHeight()-m.drawerHeight, m.textHeight())
}

// 적중 수가 판 높이를 바꾸지 않는다. 하나여도 열여섯이어도 같다(grepRows).
func TestGrepHeightIgnoresHitCount(t *testing.T) {
	e, paths := jumpEditor(t)
	e.height = 25
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	one := newGrepView(t, "func", grepHitsIn(paths[0], 2)...)
	many := newGrepView(t, "func", grepHitsIn(paths[0], 2, 4, 6)...)

	assert.Equal(t, one.grepRows(), many.grepRows())
	assert.Greater(t, one.grepRows(), 1, "적중 하나에 한 줄로 쪼그라들지 않는다")
}

// **담는 것과 보이는 것이 다르다.** 적중이 수천이어도 판은 열여섯 줄이고 `j`/`k` 가 훑는다.
func TestGrepShowsAtMostMaxRows(t *testing.T) {
	e, paths := jumpEditor(t)

	// 열여섯 줄이 실제로 걸리려면 화면이 그만큼 높아야 한다.
	e.height = 25
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	hits := make([]grepHit, 0, 60)
	for i := range 60 {
		hits = append(hits, grepHit{path: paths[0], line: i % 6, text: "func a() {}"})
	}

	e.grep = grepResult{input: "func", pattern: regexp.MustCompile("func"), hits: hits}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	assert.Equal(t, grepMaxRows, m.grepRows())
	assert.Len(t, grepDrawerRowsOf(t, m), grepMaxRows)
}

// 여는 것만으로는 커서를 옮기지 않는다. 미리보기는 `j`/`k` 를 쳐야 시작한다.
func TestGrepDoesNotMoveOnOpen(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits:    grepHitsIn(paths[1], 4),
	}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	assert.Equal(t, paths[0], m.activeBuffer().path, "다른 파일로 옮기지 않았다")
	assert.Zero(t, m.activeBuffer().cursorLine)
}

// `j`/`k` 는 커서를 실제로 그 자리로 옮겨 보여준다. jumplist 판들과 같은 손이다.
func TestGrepPreviewsWhileMoving(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits: []grepHit{
			{path: paths[0], line: 2, text: "func a() {}"},
			{path: paths[1], line: 4, text: "func b() {}"},
		},
	}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	next, _ := m.press("j")
	m = next.(viewGrep)

	assert.Equal(t, 1, m.selected)
	assert.Equal(t, paths[1], m.activeBuffer().path, "다른 파일도 열어 보여준다")
	assert.Equal(t, 4, m.activeBuffer().cursorLine, "그 자리를 보여준다")
}

// `q`·`esc` 는 취소다. 열기 전 자리로 되돌아가고 둘러보며 연 tab 을 닫는다.
func TestGrepCancelRestoresOrigin(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))
	e.activeBuffer().moveTo(2, 0, e.contentWidth())

	tabs := len(e.buffers)

	// 적중이 하나면 `j` 가 움직일 곳이 없어 미리보기가 안 태워진다. 둘을 담는다.
	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits: []grepHit{
			{path: paths[0], line: 2, text: "func a() {}"},
			{path: paths[2], line: 4, text: "func b() {}"},
		},
	}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	// 둘러보며 다른 파일을 연다.
	next, _ := m.press("j")
	m = next.(viewGrep)
	require.Equal(t, paths[2], m.activeBuffer().path)

	back, _ := m.press("q")

	require.IsType(t, viewEditorNormal{}, back)
	e = back.(viewEditorNormal).editor
	assert.Equal(t, paths[0], e.activeBuffer().path, "열기 전 파일로 되돌아온다")
	assert.Equal(t, 2, e.activeBuffer().cursorLine, "커서도 되돌아온다")
	assert.Len(t, e.buffers, tabs, "둘러보며 연 tab 을 닫는다")
	assert.Zero(t, e.drawerHeight, "판이 걷힌다")
}

// `enter` 는 확정이다. 그 자리에 남고 떠난 자리는 되돌아오기 이력에 담긴다.
func TestGrepConfirmKeepsPlaceAndRecordsJump(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))
	e.activeBuffer().moveTo(2, 0, e.contentWidth())

	e.grep = grepResult{
		input:   "func b",
		pattern: regexp.MustCompile("func b"),
		hits:    grepHitsIn(paths[1], 4),
	}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	next, _ := m.press("j")
	m = next.(viewGrep)

	done, _ := m.press("enter")

	require.IsType(t, viewEditorNormal{}, done)
	e = done.(viewEditorNormal).editor

	assert.Equal(t, paths[1], e.activeBuffer().path, "고른 자리에 남는다")
	assert.Equal(t, 4, e.activeBuffer().cursorLine)

	require.NotEmpty(t, e.jumps.places, "떠난 자리를 담는다")
	last := e.jumps.places[len(e.jumps.places)-1]
	assert.Equal(t, paths[0], last.path, "판을 열기 전 자리다. 둘러보던 자리가 아니다")
	assert.Equal(t, 2, last.line)

	// 간 자리에서 `n` 이 그 패턴을 이어 짚는다.
	assert.Equal(t, "func b", e.search.input)
	assert.True(t, e.search.highlight)
}

// 열자마자 `enter` 를 쳐도 그 자리로 간다. 아직 미리보기를 안 밟은 경우다.
func TestGrepConfirmWithoutBrowsing(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits:    grepHitsIn(paths[2], 6),
	}

	model, _ := grepMode(e)

	done, _ := model.(viewGrep).press("enter")

	e = done.(viewEditorNormal).editor
	assert.Equal(t, paths[2], e.activeBuffer().path)
	assert.Equal(t, 6, e.activeBuffer().cursorLine)
}

// 목록에 `경로:줄` 과 줄 내용이 같이 온다. 어디를 둘러볼지 고르는 판단이 목록에서 일어난다.
func TestGrepDrawerShowsPlaceAndText(t *testing.T) {
	m := newGrepView(t, "func", grepHit{path: "a/b.go", line: 41, col: 5, text: "\t\tfunc NewThing()"})

	rows := grepDrawerRowsOf(t, m)

	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "b.go:42", "줄 번호는 1 부터다")
	assert.Contains(t, rows[0], "func NewThing()")
	assert.NotContains(t, rows[0], "\t", "들여쓰기는 떼고 보인다")
}

// 아래 줄이 무엇을 찾았고 몇 번째인지 적는다. 담는 것이 많아 목록만으로는 어디쯤인지 모른다.
func TestGrepHintCountsPosition(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	e.grep = grepResult{
		input:   `func\s+a`,
		pattern: regexp.MustCompile(`func\s+a`),
		hits:    grepHitsIn(paths[0], 2, 4, 6),
	}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	assert.Contains(t, m.hint(), `"func\s+a"`, "친 그대로 적는다")
	assert.NotContains(t, m.hint(), `\\s`)

	// **몇 번째인지는 아랫 테두리가 든다.** 숫자가 설명하는 대상 옆에 있어야 한다(ADR-0079).
	assert.NotContains(t, m.hint(), "1/3", "아래 줄에는 적지 않는다")

	bottom := lastLine(ansi.Strip(m.renderDrawer()))
	assert.Contains(t, bottom, "1/3")

	next, _ := m.press("j")
	assert.Contains(t, lastLine(ansi.Strip(next.(viewGrep).renderDrawer())), "2/3")
}

// lastLine 은 판의 아랫 테두리 줄이다.
func lastLine(text string) string {
	rows := strings.Split(text, "\n")

	return rows[len(rows)-1]
}

// 상한에 닿았으면 아래 줄이 그것을 적는다. 조용히 자르면 「이게 전부」로 읽힌다.
func TestGrepHintMarksCap(t *testing.T) {
	m := newGrepView(t, "func", grepHit{path: "a.go", text: "func"})
	m.grep.capped = true

	assert.Contains(t, m.hint(), "상한")
}

// `/` 는 담은 적중을 좁힌다. 저장소를 다시 훑지 않는다.
func TestGrepFilterNarrowsLoadedHits(t *testing.T) {
	m := newGrepView(t, "New",
		grepHit{path: "core/a.go", text: "func New()"},
		grepHit{path: "lsp/b.go", text: "func NewClient()"})

	require.Len(t, grepDrawerRowsOf(t, m), 2)

	narrowed := send(m, "/", "l", "s", "p")
	rows := grepDrawerRowsOf(t, narrowed.(viewGrep))

	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "lsp/b.go")

	// 줄 내용으로도 좁혀지고 대소문자를 보지 않는다.
	assert.Len(t, grepDrawerRowsOf(t, send(m, "/", "N", "E", "W", "C").(viewGrep)), 1)

	// `esc` 는 거르기를 물린다 — 판을 닫는 것이 아니다.
	restored := send(narrowed, "esc")
	require.IsType(t, viewGrep{}, restored)
	assert.Len(t, grepDrawerRowsOf(t, restored.(viewGrep)), 2)
}

// 거르는 동안 `j` 는 글자다. 목록을 옮기지 않고 미리보기도 태우지 않는다.
func TestGrepFilterEatsMovementKeys(t *testing.T) {
	// **경로는 가짜다.** 거르는 자가 경로도 보므로 진짜 임시 경로를 쓰면 그 안의 글자에
	// 우연히 걸린다 — `/var/folders/…/4j4xvw…` 에 `j` 가 들어 있어 한 번 걸렸다.
	// 여기서는 미리보기를 태우지 않으므로 없는 파일이어도 된다.
	m := newGrepView(t, "x",
		grepHit{path: "aj.go", line: 0, text: "든 줄"},
		grepHit{path: "b.go", line: 0, text: "없다"})

	before := m.activeBuffer().path

	filtered := send(m, "/", "j").(viewGrep)

	assert.Equal(t, "j", filtered.filter.text)
	assert.Zero(t, filtered.selected, "목록은 움직이지 않았다")
	assert.Equal(t, before, filtered.activeBuffer().path, "미리보기를 태우지 않았다")
	assert.Len(t, grepDrawerRowsOf(t, filtered), 1)
}

// 거르기를 마치면(`enter`) 좁힌 채로 목록으로 돌아간다. 판을 닫는 것이 아니다.
func TestGrepFilterKeepsNarrowedOnEnter(t *testing.T) {
	m := newGrepView(t, "x",
		grepHit{path: "aj.go", text: "x"},
		grepHit{path: "b.go", text: "x"})

	back := send(m, "/", "j", "enter")

	require.IsType(t, viewGrep{}, back)
	assert.False(t, back.(viewGrep).filtering)
	assert.Equal(t, "j", back.(viewGrep).filter.text)
	assert.Len(t, grepDrawerRowsOf(t, back.(viewGrep)), 1)
}

// 빈 까닭을 갈라 적는다. 도는 중과 없는 것은 다른 말이어야 한다.
func TestGrepTellsWhyListIsEmpty(t *testing.T) {
	empty := newGrepView(t, "없는것")
	assert.Contains(t, empty.hint(), "찾은 곳이 없습니다")

	running := newGrepView(t, "찾는중")
	running.putJob(job{name: grepJobName, args: []string{"찾는중"}})
	assert.Contains(t, running.hint(), "찾는 중입니다")

	// 거르는 중에는 아래 줄이 치는 글자다. `enter` 로 마친 뒤에 까닭이 보인다.
	narrowed := send(newGrepView(t, "x", grepHit{path: "a.go", text: "x"}), "/", "z", "z", "enter")
	assert.Contains(t, narrowed.(viewGrep).hint(), "거른 결과가 없습니다")
}

// grepWindow 는 매칭이 칸 밖으로 밀리면 앞쪽을 접어 끌어온다.
func TestGrepWindowKeepsMatchVisible(t *testing.T) {
	text := strings.Repeat("a", 60) + "MATCH" + strings.Repeat("b", 60)

	// 칸에 다 들어가면 그대로다.
	short, at := grepWindow("short", 0, 40)
	assert.Equal(t, "short", short)
	assert.Equal(t, 0, at)

	// 매칭이 뒤쪽이면 앞을 접는다. 접은 표시가 붙고 매칭이 남는다.
	windowed, at := grepWindow(text, 60, 40)
	assert.Contains(t, windowed, "…")
	assert.Contains(t, truncateToWidth(windowed, 40), "MATCH", "칸 안에 매칭이 든다")
	assert.Equal(t, "MATCH", windowed[at:at+5], "접은 뒤의 자리를 같이 준다")

	// 매칭이 앞쪽이면 접지 않는다 — 오른쪽을 자르는 것으로 충분하다.
	kept, at := grepWindow(text, 0, 40)
	assert.NotContains(t, kept, "…")
	assert.Equal(t, 0, at)
}

// 목록의 매칭 글자에만 색이 붙는다(styleSearchMatch).
func TestGrepRowHighlightsMatch(t *testing.T) {
	e, _ := jumpEditor(t)
	e.height = 25
	e.width = 80

	m := viewGrep{editor: e}

	// 한글이 앞에 있어서 byte 자리와 칸 자리가 갈린다. 자리는 글에서 재서 넣는다.
	text := "함수 MATCH 뒤"
	col := strings.Index(text, "MATCH")
	hit := grepHit{path: "a.go", line: 0, col: col, end: col + len("MATCH"), text: text}

	row := m.renderRow(hit, false, 70)
	assert.Contains(t, row, styleSearchMatch.Render("MATCH"), "매칭만 검색 색이다")

	// 고른 행에서도 매칭은 제 색이다. 나머지는 반전이다.
	selected := m.renderRow(hit, true, 70)
	assert.Contains(t, selected, styleSearchMatch.Render("MATCH"))
	assert.Contains(t, selected, "\x1b[7m", "나머지는 반전이 걸린다")
}

// 목록 이동 키는 다른 판들과 같다.
func TestGrepDrawerMovementKeys(t *testing.T) {
	e, paths := jumpEditor(t)
	e.height = 25
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	hits := make([]grepHit, 0, 40)
	for i := range 40 {
		hits = append(hits, grepHit{path: paths[0], line: i % 6, text: "func a() {}"})
	}
	e.grep = grepResult{input: "func", pattern: regexp.MustCompile("func"), hits: hits}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	assert.Equal(t, 1, selectedOfGrep(t, send(m, "j")))
	assert.Equal(t, len(hits)-1, selectedOfGrep(t, send(m, "G")))
	assert.Zero(t, selectedOfGrep(t, send(send(m, "G"), "g")))
	assert.Equal(t, len(hits)-1, selectedOfGrep(t, send(m, "end")))
	assert.Equal(t, pageRows(pageFull, m.grepDrawerHeight()), selectedOfGrep(t, send(m, "pgdown")))
}

// selectedOfGrep 은 판이 고른 자리다. 판을 벗어났으면 시험을 멈춘다.
func selectedOfGrep(t *testing.T, m tea.Model) int {
	t.Helper()

	here, ok := m.(viewGrep)
	require.True(t, ok, "판에 남아 있어야 한다: %T", m)

	return here.selected
}

// **빈 화면에서도 판을 띄운다.** 다른 판들은 거기서 거절하는데(ADR-0064) 검색은 담은 것이
// 지금 buffer 에서 온 것이 아니고, 여기서 고르는 일이 곧 「어느 파일을 열까」다(ADR-0078 §6).
func TestGrepOpensOnEmptyScreen(t *testing.T) {
	e := &editor{boxChars: boxUnicode, width: 80, height: 20, active: -1}
	require.False(t, e.hasTab())

	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits:    []grepHit{{path: "a.go", line: 0, text: "func a() {}"}},
	}

	model, _ := grepMode(e)

	m, ok := model.(viewGrep)
	require.True(t, ok, "판이 열려야 한다")
	assert.False(t, m.preview.hasOrigin, "되돌릴 자리가 없다")

	// 그려지는지까지 본다 — 빈 화면 본문이 판만큼 줄고 판이 그 아래 얹힌다.
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	assert.Len(t, rows, e.height, "화면 행 수는 그대로다")
	assert.Contains(t, m.View().Content, m.boxChars.topLeft, "판 테두리가 그려진다")
	assert.Contains(t, ansi.Strip(m.renderDrawer()), "a.go:1")
}

// 빈 화면에서 확정하면 그 파일이 열린다. 취소하면 다시 빈 화면이다.
func TestGrepOnEmptyScreenOpensAndCancels(t *testing.T) {
	_, paths := jumpEditor(t)

	e := &editor{boxChars: boxUnicode, width: 80, height: 20, active: -1}
	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits:    grepHitsIn(paths[0], 2),
	}

	model, _ := grepMode(e)

	done, _ := model.(viewGrep).press("enter")
	require.IsType(t, viewEditorNormal{}, done)
	opened := done.(viewEditorNormal).editor
	assert.Equal(t, paths[0], opened.activeBuffer().path, "고른 파일이 열린다")
	assert.Equal(t, 2, opened.activeBuffer().cursorLine)
	assert.Empty(t, opened.jumps.places, "떠날 자리가 없었으므로 이력에 담을 것도 없다")

	// 다시 빈 화면에서 열고 취소한다.
	e2 := &editor{boxChars: boxUnicode, width: 80, height: 20, active: -1}
	e2.grep = e.grep

	model2, _ := grepMode(e2)
	browsed, _ := model2.(viewGrep).press("enter")
	require.IsType(t, viewEditorNormal{}, browsed)

	// 확정이 아니라 취소였다면 둘러보며 연 tab 이 닫혀 빈 화면으로 돌아간다.
	e3 := &editor{boxChars: boxUnicode, width: 80, height: 20, active: -1}
	e3.grep = e.grep

	model3, _ := grepMode(e3)
	m3 := model3.(viewGrep)

	// 열자마자는 옮기지 않으므로 손으로 미리보기를 태운다.
	m3.selectTo(0)
	m3.pending = m3.goToPlace(m3.rows()[0].place())
	require.True(t, m3.hasTab(), "둘러보며 tab 이 열렸다")

	back, _ := m3.press("q")
	require.IsType(t, viewEditorEmpty{}, back, "빈 화면으로 되돌아간다")
	assert.False(t, back.(viewEditorEmpty).hasTab())
	assert.Zero(t, back.(viewEditorEmpty).drawerHeight, "판이 걷힌다")
}

func TestGrepExpandTabs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		col, end int
		want     string
		wantCol  int
		wantEnd  int
	}{
		{name: "tab 이 없으면 그대로다", text: "func f()", col: 0, end: 4,
			want: "func f()", wantCol: 0, wantEnd: 4},
		{name: "다음 칸 경계까지 편다", text: "Age\tint", col: 0, end: 3,
			want: "Age int", wantCol: 0, wantEnd: 3},
		{name: "경계에 서 있으면 한 칸 전부", text: "Name\tstring", col: 0, end: 4,
			want: "Name    string", wantCol: 0, wantEnd: 4},
		{name: "tab 뒤의 매칭은 편 만큼 밀린다", text: "Age\tint", col: 4, end: 7,
			want: "Age int", wantCol: 4, wantEnd: 7},
		{name: "tab 둘", text: "a\tb\tc", col: 0, end: 1,
			want: "a   b   c", wantCol: 0, wantEnd: 1},
		{name: "매칭이 tab 을 품으면 그만큼 길어진다", text: "a\tb", col: 0, end: 3,
			want: "a   b", wantCol: 0, wantEnd: 5},
		{name: "한글 뒤의 tab 은 두 칸으로 세고 민다", text: "한\tx", col: 0, end: 3,
			want: "한  x", wantCol: 0, wantEnd: 3},
		{name: "매칭이 줄 끝까지", text: "a\tbb", col: 2, end: 5,
			want: "a   bb", wantCol: 4, wantEnd: 6},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text, col, end := grepExpandScreen(test.text, test.col, test.end)

			assert.Equal(t, test.want, text)
			assert.Equal(t, test.wantCol, col, "매칭 시작")
			assert.Equal(t, test.wantEnd, end, "매칭 끝")
		})
	}
}

// tab 이 든 줄도 다른 행과 **정확히 같은 폭**이어야 한다.
//
// 펴지 않으면 bubbletea 가 tab 을 버리는데 칸을 채우는 자는 폭 있는 것으로 세어서, 그 행만
// 짧아지고 오른쪽 테두리가 들쭉날쭉해진다(ADR-0098).
//
// 판 전체를 잰다. 목록 행을 따로 낼 자리가 없어졌고(ADR-0101), 테두리도 같은 폭이라
// 자를 이유가 없다.
func TestGrepRowKeepsWidthWithTabs(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	// 줄 가운데 tab 이 든 적중을 손으로 짓는다. 뛰지 않으므로 줄 번호만 있으면 된다.
	m := newGrepView(t, "int",
		grepHit{path: paths[0], line: 2, col: 4, end: 7, text: "\tAge\tint"},
		grepHit{path: paths[0], line: 4, col: 0, end: 3, text: "func a() {}"},
	)

	for i, row := range strings.Split(m.renderDrawer(), "\n") {
		plain := ansi.Strip(row)
		assert.Equal(t, m.textWidth(), widthOf(plain),
			"행 %d 이 판 폭과 같다: %q", i, plain)
		assert.NotContains(t, plain, "\t", "행 %d 에 tab 이 남지 않는다", i)
	}
}
