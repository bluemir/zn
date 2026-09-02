package core

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
		{"sample.md", "# 제목\n\n`코드` 와 *강조* 와 **굵게** 와 ~~지운 것~~ 과 [글](https://x.com)\n> 인용\n```go\nx\n```\n"},
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
		state := syntax.LanguageFor(sample.path).State()
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

// goSource 는 줄 수를 정해 만드는 Go 소스다. 화면보다 긴 파일이 필요한 캐시 시험에서 쓴다.
func goSource(lines int) string {
	out := strings.Builder{}
	out.WriteString("package main\n")
	for i := 1; i < lines; i++ {
		out.WriteString("var x int = 1\n")
	}

	return out.String()
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
