package textarea

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/syntax"

	"github.com/bluemir/zn/internal/scheme"
)

// syntaxToken 은 시험에서 토큰을 짧게 적는 손이다.
func SyntaxToken(Start, End int, kind syntax.Kind) syntax.Token {
	return syntax.Token{Start: Start, End: End, Kind: kind}
}

// emittedKinds 는 lexer 가 실제로 내보내는 갈래 전부다.
func emittedKinds(t *testing.T) map[syntax.Kind]bool {
	t.Helper()

	samples := []struct{ Path, data string }{
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
		state := syntax.LanguageFor(sample.Path).State()
		require.NotNil(t, state, "%s 를 알아보지 못했습니다", sample.Path)

		for _, Line := range strings.Split(sample.data, "\n") {
			var tokens []syntax.Token
			tokens, state = state.Lex([]byte(Line))
			for _, Token := range tokens {
				kinds[Token.Kind] = true
			}
		}
	}

	require.NotEmpty(t, kinds)

	return kinds
}

// goSource 는 줄 수를 정해 만드는 Go 소스다. 화면보다 긴 파일이 필요한 캐시 시험에서 쓴다.
func goSource(Lines int) string {
	out := strings.Builder{}
	out.WriteString("package main\n")
	for i := 1; i < Lines; i++ {
		out.WriteString("var x int = 1\n")
	}

	return out.String()
}

// mdSource 는 코드펜스가 든 markdown 이다. 위임된 문맥이 캐시를 지나가는지 보는 데 쓴다.
func mdSource(Lines int) string {
	out := strings.Builder{}
	out.WriteString("# 제목\n")
	out.WriteString("```go\n")
	for i := 2; i < Lines-1; i++ {
		out.WriteString("var x int = 1\n")
	}
	out.WriteString("```\n")

	return out.String()
}

// 아래는 **담아둔 것의 불변**을 보는 시험이다. 밖에서 보면 안 되는 값이라 이 패키지 안에 있고,
// 그리기 대신 `LexSyntaxTo` 로 몬다 — 그리는 쪽이 하는 일이 그것이다 (ADR-0129).

// 담아둔 것은 몰아 준 줄까지만 찬다. 그 아래는 볼 사람이 없다.
func TestSyntaxCacheFillsToWhereItWasDriven(t *testing.T) {
	buf := NewBuffer("main.go", []byte(goSource(100)))

	buf.LexSyntaxTo(4)

	assert.Equal(t, 5, buf.syntax.valid, "다섯 줄까지만 훑는다")
	assert.Len(t, buf.syntax.Lines, len(buf.Lines), "캐시가 줄과 나란하다")
}

// **이 시험이 조기 중단의 증거다.**
//
// 파일 전체를 훑어 둔 뒤 한 글자를 넣는다. 그 줄을 끝낸 문맥이 전과 같으면 아래로 담아둔 것이
// 그대로 맞으므로 찬 만큼이 유지된다. 수렴을 보지 않으면 화면 아래가 버려져 다섯 줄로 떨어진다.
func TestSyntaxCacheStopsAtConvergence(t *testing.T) {
	buf := NewBuffer("main.go", []byte(goSource(100)))
	setContentWidth(&buf, 40, 5)

	buf.LexSyntaxTo(len(buf.Lines) - 1)
	require.Equal(t, 100, buf.syntax.valid, "전체를 훑으면 파일 끝까지 찬다")

	// 여러 줄에 걸치는 것을 열지 않는 편집이다. 한 글자만 넣는다.
	buf.MoveTo(scheme.Cursor{Line: 1, Col: 1})
	buf.Insert([]byte(" "))
	require.Equal(t, "v ar x int = 1", string(buf.Lines[1]), "편집이 실제로 일어났다")

	buf.LexSyntaxTo(4)

	assert.Equal(t, 100, buf.syntax.valid, "문맥이 수렴해 찬 만큼이 그대로 남는다")
	assert.Equal(t, 0, buf.syntax.changedEnd, "달라진 자리를 다 지났다")
}

// 문맥이 달라지면 멈추지 못하고 몰아 준 줄까지만 훑는다.
func TestSyntaxCacheKeepsGoingWhenContextChanges(t *testing.T) {
	buf := NewBuffer("main.go", []byte(goSource(100)))
	setContentWidth(&buf, 40, 5)

	buf.LexSyntaxTo(len(buf.Lines) - 1)
	require.Equal(t, 100, buf.syntax.valid)

	// 첫 줄에 여는 블록 주석을 넣는다. 그 아래 전부가 주석이 된다.
	buf.MoveTo(scheme.Cursor{Line: 0})
	buf.Insert([]byte("/* "))

	buf.LexSyntaxTo(4)

	assert.Equal(t, 5, buf.syntax.valid, "수렴하지 못해 몰아 준 줄까지만 훑었다")
}

// 줄 수가 그대로인 편집은 캐시 slice 를 새로 만들지 않는다. 키 하나가 파일 크기에 비례한
// 복사가 되면 안 된다.
func TestSyntaxCacheReusesSliceOnSameLineCount(t *testing.T) {
	buf := NewBuffer("main.go", []byte(goSource(100)))
	buf.LexSyntaxTo(4)

	before := &buf.syntax.Lines[0]

	buf.ReplaceLines(1, 1, [][]byte{[]byte("var y int = 2")})

	assert.Same(t, before, &buf.syntax.Lines[0], "줄 수가 같으면 자리를 옮기지 않는다")
	assert.Equal(t, 1, buf.syntax.valid, "고친 줄부터 다시 훑는다")
}

// 편집 입구마다 캐시가 줄과 나란히 유지된다.
// **앞으로 편집 경로가 늘면 이 표에서 걸린다.**
func TestSyntaxCacheSurvivesEveryEdit(t *testing.T) {
	tests := []struct {
		name string
		edit func(buf *Viewport)
	}{
		{name: "타이핑", edit: func(buf *Viewport) { buf.Insert([]byte("x")) }},
		{name: "줄 만들기", edit: func(buf *Viewport) { buf.OpenLineBelow() }},
		{name: "줄 지우기", edit: func(buf *Viewport) { buf.deleteLines(0, 0) }},
		{name: "글자 지우기", edit: func(buf *Viewport) { buf.DeleteForward() }},
		{name: "복사한 것 붙이기", edit: func(buf *Viewport) {
			block := buf.yankLines(0, 0)
			buf.PasteAfter(block, 1)
		}},
		{name: "글자 덮기", edit: func(buf *Viewport) { buf.ReplaceChar([]byte("z"), 1) }},
		{name: "줄 바꾸기", edit: func(buf *Viewport) { buf.changeLines(0, 0) }},
		{name: "되돌리기", edit: func(buf *Viewport) {
			buf.deleteLines(0, 0)
			buf.ApplyUndo()
		}},
		{name: "다시 하기", edit: func(buf *Viewport) {
			buf.deleteLines(0, 0)
			buf.ApplyUndo()
			buf.ApplyRedo()
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("main.go", []byte(goSource(20)))
			setContentWidth(&buf, 40, 5)
			buf.LexSyntaxTo(4)

			test.edit(&buf)
			buf.LexSyntaxTo(min(4, len(buf.Lines)-1))

			assert.Len(t, buf.syntax.Lines, len(buf.Lines), "캐시가 줄과 어긋났습니다")
			assert.LessOrEqual(t, buf.syntax.valid, len(buf.Lines))
		})
	}
}

// 어긋난 캐시는 스스로 다시 짓는다. 어떤 경로가 무효화를 빠뜨려도 느려질 뿐 틀리지 않는다.
func TestSyntaxCacheRebuildsWhenMisaligned(t *testing.T) {
	buf := NewBuffer("main.go", []byte(goSource(20)))
	buf.LexSyntaxTo(4)

	// 무효화를 빠뜨린 경로를 흉내낸다.
	buf.syntax.Lines = buf.syntax.Lines[:3]

	assert.NotPanics(t, func() { buf.LexSyntaxTo(4) })
	assert.Len(t, buf.syntax.Lines, len(buf.Lines), "통째로 다시 지었다")
	assert.NotEmpty(t, buf.SyntaxTokens(0), "색이 다시 붙었다")
}

// **위임된 문맥이 캐시의 `==` 를 지난다.**
//
// 코드펜스 안의 문맥은 `mdFence{marker, size, inner}` 라 interface 칸을 든다. 견줄 수 없는
// 것이 그 안에 들어오면 여기서 편집기가 죽는다(ADR-0039, ADR-0040).
//
// 그리고 위임이 수렴을 깨뜨리지 않는지도 같이 본다 — 펜스 안의 줄을 고쳐도 찬 만큼이 남아야 한다.
func TestSyntaxCacheConvergesInsideFence(t *testing.T) {
	buf := NewBuffer("doc.md", []byte(mdSource(100)))
	setContentWidth(&buf, 40, 5)

	assert.NotPanics(t, func() { buf.LexSyntaxTo(len(buf.Lines) - 1) },
		"위임된 문맥을 `==` 로 견줄 수 없습니다")
	require.Equal(t, 100, buf.syntax.valid, "전체를 훑으면 파일 끝까지 찬다")

	// 펜스 안(3 번 줄) 을 고친다. 안쪽 언어의 문맥이 달라지지 않는 편집이다.
	buf.ReplaceLines(3, 1, [][]byte{[]byte("var y int = 2")})
	buf.LexSyntaxTo(4)

	assert.Equal(t, 100, buf.syntax.valid, "문맥이 수렴해 찬 만큼이 그대로 남는다")
}
