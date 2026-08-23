package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/syntax"
)

// syntaxToken 은 시험에서 토큰을 짧게 적는 손이다.
func syntaxToken(start, end int, kind syntax.Kind) syntax.Token {
	return syntax.Token{Start: start, End: end, Kind: kind}
}

// 문법 강조는 위 층이 덮지 않은 자리를 메우고, 행을 빈틈 없이 덮는다.
//
// 색 없는 자리가 아무 속성 없는 style 인 구간으로 들어오는 것까지 본다 — 그것이
// 「강조가 없으면 화면이 예전과 같다」를 떠받친다(render-row.go 의 segments).
func TestRowSegmentsFillGapsWithSyntax(t *testing.T) {
	keyword := styleSyntax[syntax.KindKeyword]
	comment := styleSyntax[syntax.KindComment]

	var plain lipgloss.Style

	tests := []struct {
		name      string
		highlight rowHighlight
		want      []rowSegment
	}{
		{
			name:      "토큰이 없으면 행 전체가 색 없는 구간 하나다",
			highlight: rowHighlight{cursorCol: -1},
			want:      []rowSegment{{start: 0, end: 10, style: plain}},
		},
		{
			name:      "토큰 하나가 행 앞을 덮는다",
			highlight: rowHighlight{cursorCol: -1, tokens: []syntax.Token{syntaxToken(0, 4, syntax.KindKeyword)}},
			want: []rowSegment{
				{start: 0, end: 4, style: keyword},
				{start: 4, end: 10, style: plain},
			},
		},
		{
			name: "토큰 둘 사이는 색 없는 구간이다",
			highlight: rowHighlight{cursorCol: -1, tokens: []syntax.Token{
				syntaxToken(0, 2, syntax.KindKeyword), syntaxToken(5, 8, syntax.KindComment),
			}},
			want: []rowSegment{
				{start: 0, end: 2, style: keyword},
				{start: 2, end: 5, style: plain},
				{start: 5, end: 8, style: comment},
				{start: 8, end: 10, style: plain},
			},
		},
		{
			name: "색을 정하지 않은 갈래는 조각을 나누지 않는다",
			highlight: rowHighlight{cursorCol: -1, tokens: []syntax.Token{
				syntaxToken(2, 5, syntax.KindPlain),
			}},
			want: []rowSegment{{start: 0, end: 10, style: plain}},
		},
		{
			name: "검색이 토큰 가운데를 가르고 이긴다",
			highlight: rowHighlight{
				cursorCol: -1,
				matches:   [][]int{{3, 6}},
				tokens:    []syntax.Token{syntaxToken(0, 10, syntax.KindComment)},
			},
			want: []rowSegment{
				{start: 0, end: 3, style: comment},
				{start: 3, end: 6, style: styleSearchMatch},
				{start: 6, end: 10, style: comment},
			},
		},
		{
			name: "선택이 토큰 가운데를 가르고 이긴다",
			highlight: rowHighlight{
				cursorCol: -1,
				selection: []int{2, 5},
				tokens:    []syntax.Token{syntaxToken(0, 10, syntax.KindKeyword)},
			},
			want: []rowSegment{
				{start: 0, end: 2, style: keyword},
				{start: 2, end: 5, style: styleSelection},
				{start: 5, end: 10, style: keyword},
			},
		},
		{
			name: "검색·선택·문법 셋이 겹치면 검색 > 선택 > 문법 이다",
			highlight: rowHighlight{
				cursorCol: -1,
				matches:   [][]int{{4, 6}},
				selection: []int{2, 8},
				tokens:    []syntax.Token{syntaxToken(0, 10, syntax.KindComment)},
			},
			want: []rowSegment{
				{start: 0, end: 2, style: comment},
				{start: 2, end: 4, style: styleSelection},
				{start: 4, end: 6, style: styleSearchMatch},
				{start: 6, end: 8, style: styleSelection},
				{start: 8, end: 10, style: comment},
			},
		},
		{
			name: "행 밖의 토큰은 잘린다",
			highlight: rowHighlight{cursorCol: -1, tokens: []syntax.Token{
				syntaxToken(15, 18, syntax.KindKeyword),
			}},
			want: []rowSegment{{start: 0, end: 10, style: plain}},
		},
		{
			name: "행 경계에 걸친 토큰은 걸친 만큼만 칠한다",
			highlight: rowHighlight{cursorCol: -1, tokens: []syntax.Token{
				syntaxToken(7, 20, syntax.KindString),
			}},
			want: []rowSegment{
				{start: 0, end: 7, style: plain},
				{start: 7, end: 10, style: styleSyntax[syntax.KindString]},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.highlight.segments(0, 10)

			assert.Equal(t, test.want, got)

			// 어느 경우든 행을 빈틈 없이, 겹치지 않게, 앞에서부터 덮어야 한다.
			at := 0
			for _, segment := range got {
				assert.Equal(t, at, segment.start, "빈틈이나 겹침이 있습니다")
				at = segment.end
			}
			assert.Equal(t, 10, at, "행 끝까지 덮지 않았습니다")
		})
	}
}

// 토큰이 없으면 구간이 하나로 모여서, 문법 강조를 넣기 전과 그리는 순서가 같다.
func TestRowSegmentsWithoutTokensMatchTopLayer(t *testing.T) {
	highlight := rowHighlight{cursorCol: -1, matches: [][]int{{3, 5}}, selection: []int{0, 9}}

	assert.Equal(t, []rowSegment{
		{start: 0, end: 3, style: styleSelection},
		{start: 3, end: 5, style: styleSearchMatch},
		{start: 5, end: 9, style: styleSelection},
		{start: 9, end: 10},
	}, highlight.segments(0, 10), "위 층 사이와 뒤만 색 없는 구간으로 메운다")
}

// 색표에 없는 갈래는 조용히 색을 잃는다 — appendSyntax 가 흘려보내므로 컴파일도 걸리지
// 않는다. 갈래를 열어 둔 값이 이 시험이다(internal/syntax 의 Kind).
//
// 표본이 못 내는 갈래는 이 시험이 못 보므로, 언어를 더할 때 표본도 같이 넓힌다.
func TestEveryKindHasStyle(t *testing.T) {
	for kind := range emittedKinds(t) {
		_, ok := styleSyntax[kind]
		assert.True(t, ok,
			"갈래 %d 에 색이 없습니다 — style.go 의 styleSyntax 에 한 줄을 더하세요", kind)
	}
}

// emittedKinds 는 lexer 가 실제로 내보내는 갈래 전부다.
func emittedKinds(t *testing.T) map[syntax.Kind]bool {
	t.Helper()

	samples := []struct{ path, data string }{
		{"sample.go", "package main\n\n// 주석\nfunc main() {\n\tvar n int = 0x1f\n\ts := `raw`\n\t_ = nil\n}\n"},
		{"sample.md", "# 제목\n\n`코드` 와 *강조* 와 [글](https://x.com)\n> 인용\n```go\nx\n```\n"},
		{"Makefile", "# 주석\nVERSION?=$(shell git describe)\nbuild: dep ## 도움\n\t@go build -o $@ .\n"},
		{"Dockerfile", "# 주석\nFROM golang:1.26 AS build\nENV PATH=${GOPATH}/bin\nCMD [\"go\"]\n"},
		{"sample.py", "# 주석\nclass Buffer:\n    @property\n    def 이름(self):\n        return f\"{self.n}\"\n        n = 0x1f\n"},
		{"sample.css", "/* 주석 */\n.a, #b > c:hover {\n\tcolor: #fff;\n\t--x: rgba(0, 0, 0, 0.5);\n\tcontent: \"한글\" !important;\n}\n"},
		{"sample.js", "// 주석\nclass B extends A {}\nconst q = `여러 ${값} 줄`;\nconst n = 0x1f;\nlet t = true;\n"},
		{"sample.html", "<!DOCTYPE html>\n<!-- 주석 -->\n<div class=\"한글\" data-id=1>\n<p>글 &amp; 글</p>\n</div>\n"},
		{"sample.json", "{\n  // 주석\n  \"a\": \"한글\",\n  \"n\": 0x1f,\n  \"ok\": true\n}\n"},
		{"sample.yaml", "# 주석\nservices:\n  - name: web\n    port: 8080\n    tag: !!str x\n    base: &공통 y\n    ok: true\n    script: |\n      go build\n"},
	}

	kinds := map[syntax.Kind]bool{}
	for _, sample := range samples {
		state := syntax.Detect(sample.path)
		require.NotNil(t, state, "%s 를 알아보지 못했습니다", sample.path)

		for _, line := range strings.Split(sample.data, "\n") {
			var tokens []syntax.Token
			tokens, state = state.Lex([]byte(line))
			for _, token := range tokens {
				kinds[token.Kind] = true
			}
		}
	}

	require.NotEmpty(t, kinds)

	return kinds
}

// 강조하는 파일은 본문에 색이 붙는다.
func TestSyntaxColorsGoSource(t *testing.T) {
	m := newTestEditorFile("main.go", "func main() {\n", 40, 3)

	rows := contentRowsOf(t, m)
	require.NotEmpty(t, rows)

	assert.Equal(t, "func main() {", ansi.Strip(rows[0])[gutterWidthOf(m):], "글자는 그대로다")
	assert.Contains(t, rows[0], styleSyntax[syntax.KindKeyword].Render("func"), "예약어")
	assert.Contains(t, rows[0], styleSyntax[syntax.KindFunction].Render("main"), "부르는 이름")
}

// 강조하지 않는 확장자는 예전과 한 글자도 다르지 않다.
func TestSyntaxSkipsUnknownExtension(t *testing.T) {
	m := newTestEditorFile("notes.txt", "func main() {\n", 40, 1)

	assert.Equal(t, "func main() {", textOf(t, m), "강조하지 않는 확장자는 본문에 색이 없다")
}

// 검색이 문법을 이긴다. 찾은 자리가 어디인지가 갈래보다 급하다.
func TestSyntaxLosesToSearch(t *testing.T) {
	var m tea.Model = newTestEditorFile("main.go", "func main() {\n", 40, 3)

	m = typeInto(m, "/func")
	m = send(m, "enter")

	row := contentRowsOf(t, m)[0]
	assert.Contains(t, row, styleSearchCurrent.Render("func"), "검색이 이긴다")
	assert.NotContains(t, row, styleSyntax[syntax.KindKeyword].Render("func"),
		"그 자리에 문법 색이 남지 않는다")
}

// 선택도 문법을 이긴다. vim 과 같다.
func TestSyntaxLosesToSelection(t *testing.T) {
	var m tea.Model = newTestEditorFile("main.go", "func main() {\n", 40, 3)

	m = send(m, "v", "e")

	row := contentRowsOf(t, m)[0]
	assert.Contains(t, row, styleSelection.Render("func"), "선택이 이긴다")
	assert.NotContains(t, row, styleSyntax[syntax.KindKeyword].Render("func"),
		"그 자리에 문법 색이 남지 않는다")
}

// wrap 된 줄에서 행 경계에 걸친 토큰은 양쪽 행에 나뉘어 칠해진다.
// 검색 강조가 그렇게 나뉘는 것과 같은 자리다(TestSearchHighlightAcrossWrap).
func TestSyntaxAcrossWrap(t *testing.T) {
	// 줄번호 칸을 뺀 본문이 좁아야 한 낱말이 두 행에 걸린다.
	m := newTestEditorFile("main.go", "// 주석이 길어서 줄바꿈된다\n", 12, 4)

	rows := contentRowsOf(t, m)
	require.GreaterOrEqual(t, len(rows), 2)

	comment := styleSyntax[syntax.KindComment]
	for i, row := range rows[:2] {
		text := ansi.Strip(row)[gutterWidthOf(m):]
		if strings.TrimSpace(text) == "" {
			continue
		}

		assert.Contains(t, row, comment.Render(strings.TrimRight(text, " ")),
			"%d 번째 행에 주석 색이 걸쳐 있다", i)
	}
}

// 공백 마커는 문법 색 안에서도 자기 칸을 이긴다. 문법은 글자색만 정하고 마커가 그 위에
// 자기 글자색을 덮어쓴다(render-row.go 의 renderParts, ADR-0020).
func TestSyntaxKeepsWhitespaceMarker(t *testing.T) {
	m := newTestEditorFile("main.go", "\t// 주석\n", 40, 3)

	row := contentRowsOf(t, m)[0]

	assert.Equal(t, markerTab+"   // 주석", ansi.Strip(row)[gutterWidthOf(m):],
		"마커가 칸을 어긋내지 않는다")
	assert.Contains(t, row, styleSyntax[syntax.KindComment].Render("// 주석"), "주석은 주석 색이다")
	assert.NotContains(t, row, styleSyntax[syntax.KindComment].Render(markerTab),
		"마커에는 주석 색이 붙지 않는다")
}

// goSource 는 줄 수를 정해 만드는 Go 소스다. 화면보다 긴 파일이 필요한 캐시 시험에서 쓴다.
func goSource(lines int) string {
	out := strings.Builder{}
	out.WriteString("package main\n")
	for i := 1; i < lines; i++ {
		out.WriteString("var x int = 1\n")
	}

	return out.String()
}

// 캐시는 화면 맨 아래까지만 채운다. 그 아래는 볼 사람이 없다.
func TestSyntaxCacheFillsToScreenBottom(t *testing.T) {
	m := newTestEditorFile("main.go", goSource(100), 40, 5)
	contentRowsOf(t, m)

	buf := m.activeBuffer()
	assert.Equal(t, 5, buf.syntax.valid, "화면 다섯 행까지만 훑는다")
	assert.Len(t, buf.syntax.lines, len(buf.lines), "캐시가 줄과 나란하다")
}

// **고친 줄 아래가 색을 잃지 않아야 한다.**
//
// 편집은 valid 를 고친 줄까지 끌어내린다. 거기서 문맥이 수렴했을 때 valid 를 「수렴한 자리
// 바로 뒤」까지만 올리면, 화면에 보이는 그 아래 줄들이 아직 안 훑은 것으로 취급되어 색 없이
// 그려진다. 한 프레임에 한 줄씩만 되살아나서 눈에 띈다. filled 가 그것을 막는다.
func TestSyntaxKeepsColorBelowEdit(t *testing.T) {
	m := newTestEditorFile("main.go", goSource(100), 40, 5)
	contentRowsOf(t, m)

	buf := m.activeBuffer()
	buf.lexSyntaxTo(len(buf.lines) - 1)
	require.Equal(t, 100, buf.syntax.valid, "전체를 훑으면 파일 끝까지 찬다")

	buf.replaceLines(1, 1, [][]byte{[]byte("var y int = 2")})
	rows := contentRowsOf(t, m)

	keyword := styleSyntax[syntax.KindKeyword]
	for i, row := range rows[1:] {
		assert.Contains(t, row, keyword.Render("var"),
			"고친 줄 아래 %d 번째 행이 색을 잃었습니다", i+1)
	}
}

// **이 시험이 조기 중단의 증거다.**
//
// 파일 전체를 훑어 둔 뒤 한 글자를 넣는다. 그 줄을 끝낸 문맥이 전과 같으면 아래로 담아둔 것이
// 그대로 맞으므로 찬 만큼이 유지된다. 수렴을 보지 않으면 화면 아래가 버려져 다섯 줄로 떨어진다.
func TestSyntaxCacheStopsAtConvergence(t *testing.T) {
	var m tea.Model = newTestEditorFile("main.go", goSource(100), 40, 5)
	contentRowsOf(t, m)

	bufferOf(t, m)
	m.(viewEditorNormal).activeBuffer().lexSyntaxTo(99)
	require.Equal(t, 100, bufferOf(t, m).syntax.valid)

	// 여러 줄에 걸치는 것을 열지 않는 편집이다. `a` 로 insert 에 들어가 한 글자 넣는다 —
	// `A` 는 아직 없는 키라 아무 일도 일어나지 않는다(docs/tasks.md).
	m = send(m, "j", "a")
	m = typeInto(m, " ")
	require.Equal(t, "v ar x int = 1", string(bufferOf(t, m).lines[1]), "편집이 실제로 일어났다")

	contentRowsOf(t, m)

	buf := bufferOf(t, m)
	assert.Equal(t, 100, buf.syntax.valid, "문맥이 수렴해 찬 만큼이 그대로 남는다")
	assert.Equal(t, 0, buf.syntax.changedEnd, "달라진 자리를 다 지났다")
}

// 문맥이 달라지면 멈추지 못하고 화면 끝까지만 훑는다. 그리고 아래 줄이 실제로 주석 색이 된다.
func TestSyntaxCacheKeepsGoingWhenContextChanges(t *testing.T) {
	var m tea.Model = newTestEditorFile("main.go", goSource(100), 40, 5)
	contentRowsOf(t, m)
	m.(viewEditorNormal).activeBuffer().lexSyntaxTo(99)
	require.Equal(t, 100, bufferOf(t, m).syntax.valid)

	// 첫 줄에 여는 블록 주석을 넣는다. 그 아래 전부가 주석이 된다.
	m = send(m, "i")
	m = typeInto(m, "/* ")
	m = send(m, "esc")

	rows := contentRowsOf(t, m)
	buf := bufferOf(t, m)

	assert.Equal(t, 5, buf.syntax.valid, "수렴하지 못해 화면 끝까지만 훑었다")
	assert.Contains(t, rows[3], styleSyntax[syntax.KindComment].Render("var x int = 1"),
		"열린 주석 아래 줄도 주석 색이다")
}

// 줄 수가 그대로인 편집은 캐시 slice 를 새로 만들지 않는다. 키 하나가 파일 크기에 비례한
// 복사가 되면 안 된다.
func TestSyntaxCacheReusesSliceOnSameLineCount(t *testing.T) {
	m := newTestEditorFile("main.go", goSource(100), 40, 5)
	contentRowsOf(t, m)

	buf := m.activeBuffer()
	before := &buf.syntax.lines[0]

	buf.replaceLines(1, 1, [][]byte{[]byte("var y int = 2")})

	assert.Same(t, before, &buf.syntax.lines[0], "줄 수가 같으면 자리를 옮기지 않는다")
	assert.Equal(t, 1, buf.syntax.valid, "고친 줄부터 다시 훑는다")
}

// 편집 입구마다 캐시가 줄과 나란히 유지된다.
// **앞으로 편집 경로가 늘면 이 표에서 걸린다.**
func TestSyntaxCacheSurvivesEveryEdit(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{name: "타이핑", keys: []string{"i", "x", "esc"}},
		{name: "o 로 줄 만들기", keys: []string{"o"}},
		{name: "dd 로 줄 지우기", keys: []string{"d", "d"}},
		{name: "x 로 글자 지우기", keys: []string{"x"}},
		{name: "yy 뒤 p 로 붙이기", keys: []string{"y", "y", "p"}},
		{name: "r 로 글자 덮기", keys: []string{"r", "z"}},
		{name: "cc 로 줄 바꾸기", keys: []string{"c", "c", "esc"}},
		{name: "되돌리기", keys: []string{"d", "d", "u"}},
		{name: "다시 하기", keys: []string{"d", "d", "u", "ctrl+r"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var m tea.Model = newTestEditorFile("main.go", goSource(20), 40, 5)
			contentRowsOf(t, m)

			m = send(m, test.keys...)
			contentRowsOf(t, m)

			buf := bufferOf(t, m)
			assert.Len(t, buf.syntax.lines, len(buf.lines), "캐시가 줄과 어긋났습니다")
			assert.LessOrEqual(t, buf.syntax.valid, len(buf.lines))
		})
	}
}

// 어긋난 캐시는 스스로 다시 짓는다. 어떤 경로가 무효화를 빠뜨려도 느려질 뿐 틀리지 않는다.
func TestSyntaxCacheRebuildsWhenMisaligned(t *testing.T) {
	m := newTestEditorFile("main.go", goSource(20), 40, 5)
	contentRowsOf(t, m)

	// 무효화를 빠뜨린 경로를 흉내낸다.
	buf := m.activeBuffer()
	buf.syntax.lines = buf.syntax.lines[:3]

	assert.NotPanics(t, func() { contentRowsOf(t, m) })
	assert.Len(t, buf.syntax.lines, len(buf.lines), "통째로 다시 지었다")
	assert.Contains(t, contentRowsOf(t, m)[0],
		styleSyntax[syntax.KindKeyword].Render("package"), "색이 맞다")
}

// 이름 없이 열었다가 이름이 붙으면 그때 언어를 고른다(`:w foo.go`).
func TestSyntaxDetectsWhenPathAppears(t *testing.T) {
	m := newTestEditorFile("", "func main() {\n", 40, 1)
	require.NotContains(t, contentRowsOf(t, m)[0],
		styleSyntax[syntax.KindKeyword].Render("func"), "이름이 없으면 강조하지 않는다")

	m.activeBuffer().path = "main.go"

	assert.Contains(t, contentRowsOf(t, m)[0],
		styleSyntax[syntax.KindKeyword].Render("func"), "이름이 붙으면 강조한다")
}

// mdSource 는 코드펜스가 든 markdown 이다. 위임된 문맥이 캐시를 지나가는지 보는 데 쓴다.
func mdSource(lines int) string {
	out := strings.Builder{}
	out.WriteString("# 제목\n")
	out.WriteString("```go\n")
	for i := 2; i < lines-1; i++ {
		out.WriteString("var x int = 1\n")
	}
	out.WriteString("```\n")

	return out.String()
}

// **위임된 문맥이 캐시의 `==` 를 지난다.**
//
// 코드펜스 안의 문맥은 `mdFence{marker, size, inner}` 라 interface 칸을 든다. 견줄 수 없는
// 것이 그 안에 들어오면 여기서 편집기가 죽는다(ADR-0039, ADR-0040).
//
// 그리고 위임이 수렴을 깨뜨리지 않는지도 같이 본다 — 펜스 안의 줄을 고쳐도 찬 만큼이 남아야 한다.
func TestSyntaxCacheConvergesInsideFence(t *testing.T) {
	m := newTestEditorFile("doc.md", mdSource(100), 40, 5)
	contentRowsOf(t, m)

	buf := m.activeBuffer()
	assert.NotPanics(t, func() { buf.lexSyntaxTo(len(buf.lines) - 1) },
		"위임된 문맥을 `==` 로 견줄 수 없습니다")
	require.Equal(t, 100, buf.syntax.valid, "전체를 훑으면 파일 끝까지 찬다")

	// 펜스 안(3 번 줄) 을 고친다. 안쪽 언어의 문맥이 달라지지 않는 편집이다.
	buf.replaceLines(3, 1, [][]byte{[]byte("var y int = 2")})
	contentRowsOf(t, m)

	assert.Equal(t, 100, buf.syntax.valid, "문맥이 수렴해 찬 만큼이 그대로 남는다")
}

// 코드펜스 안이 그 언어로 칠해진다.
func TestSyntaxColorsInsideFence(t *testing.T) {
	m := newTestEditorFile("doc.md", "```go\nfunc main() {}\n```\n", 40, 4)

	rows := contentRowsOf(t, m)
	require.GreaterOrEqual(t, len(rows), 2)

	assert.Contains(t, rows[1], styleSyntax[syntax.KindKeyword].Render("func"),
		"펜스 안의 예약어에 색이 붙는다")
}

// 언어 이름이 없는 펜스는 위임을 넣기 전과 한 글자도 다르지 않다.
func TestSyntaxSkipsUnknownFence(t *testing.T) {
	m := newTestEditorFile("doc.md", "```\nfunc main() {}\n```\n", 40, 4)

	rows := contentRowsOf(t, m)
	require.GreaterOrEqual(t, len(rows), 2)

	assert.Equal(t, "func main() {}", ansi.Strip(rows[1])[gutterWidthOf(m):], "글자는 그대로다")
	assert.NotContains(t, rows[1], styleSyntax[syntax.KindKeyword].Render("func"),
		"모르는 언어는 색이 없다")
}
