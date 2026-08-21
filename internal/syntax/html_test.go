package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTMLLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
		end  State
	}{
		{
			name: "tag 이름과 속성",
			line: `<div class="a">`,
			want: []string{"keyword:<div", "var:class", `string:"a"`},
			end:  htmlNormal{},
		},
		{
			name: "홑따옴표 속성 값",
			line: `<a href='x'>`,
			want: []string{"keyword:<a", "var:href", "string:'x'"},
			end:  htmlNormal{},
		},
		{
			name: "따옴표 없는 속성 값",
			line: `<input type=text>`,
			want: []string{"keyword:<input", "var:type", "string:text"},
			end:  htmlNormal{},
		},
		{
			name: "닫는 tag",
			line: `</div>`,
			want: []string{"keyword:</div"},
			end:  htmlNormal{},
		},
		{
			name: "스스로 닫는 tag",
			line: `<br/>`,
			want: []string{"keyword:<br"},
			end:  htmlNormal{},
		},
		{
			name: "이름에 `-` 와 `:` 가 든다",
			line: `<my-tag data-id="1" xlink:href="x">`,
			want: []string{
				"keyword:<my-tag", "var:data-id", `string:"1"`, "var:xlink:href", `string:"x"`,
			},
			end: htmlNormal{},
		},
		{
			name: "한 줄에서 끝난 주석",
			line: `<!-- 한글 주석 --><p>`,
			want: []string{"comment:<!-- 한글 주석 -->", "keyword:<p"},
			end:  htmlNormal{},
		},
		{
			name: "doctype",
			line: `<!DOCTYPE html>`,
			want: []string{"keyword:<!DOCTYPE html>"},
			end:  htmlNormal{},
		},
		{
			name: "글자 이름",
			line: `a &amp; b &#39; c`,
			want: []string{"const:&amp;", "const:&#39;"},
			end:  htmlNormal{},
		},
		{
			name: "`<` 뒤에 이름이 없으면 tag 가 아니다",
			line: `a < b 이고 c > d`,
			want: []string{},
			end:  htmlNormal{},
		},
		{
			name: "`>` 가 없으면 다음 줄도 tag 안이다",
			line: `<div class="a"`,
			want: []string{"keyword:<div", "var:class", `string:"a"`},
			end:  htmlTag{},
		},
		{
			name: "따옴표가 닫히지 않으면 다음 줄도 값 안이다",
			line: `<div class="열었다`,
			want: []string{"keyword:<div", "var:class", `string:"열었다`},
			end:  htmlAttrValue{quote: '"'},
		},
		{
			name: "글 안의 여러 tag",
			line: `<p>한글 <b>굵게</b> 이다</p>`,
			want: []string{"keyword:<p", "keyword:<b", "keyword:</b", "keyword:</p"},
			end:  htmlNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, htmlNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// `<script>`·`<style>` 안은 html 이 아니다. 색을 입히지 않고 닫는 tag 를 기다린다.
func TestHTMLRawText(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "script 안은 js 로 훑는다. 문자열 안의 `</div>` 는 tag 가 아니다",
			lines: []string{"<script>", `const s = "</div>";`, "</script>"},
			want: [][]string{
				{"keyword:<script"},
				{"keyword:const", `string:"</div>"`},
				{"keyword:</script"},
			},
			end: htmlNormal{},
		},
		{
			name:  "대문자 tag 도 같은 문맥이다",
			lines: []string{"<SCRIPT>", "x", "</SCRIPT>"},
			want:  [][]string{{"keyword:<SCRIPT"}, {}, {"keyword:</SCRIPT"}},
			end:   htmlNormal{},
		},
		{
			name:  "style 안은 css 로 훑는다",
			lines: []string{"<style>", "a { color: red; }", "</style>"},
			want: [][]string{
				{"keyword:<style"},
				{"type:a", "keyword:color", "const:red"},
				{"keyword:</style"},
			},
			end: htmlNormal{},
		},
		{
			name:  "여는 tag 와 같은 줄의 안쪽도 그 언어로 훑는다",
			lines: []string{`<script src="a.js">let x = 1;`, "</script>"},
			want: [][]string{
				{"keyword:<script", "var:src", `string:"a.js"`, "keyword:let", "number:1"},
				{"keyword:</script"},
			},
			end: htmlNormal{},
		},
		{
			name:  "닫히지 않으면 계속 안쪽이다",
			lines: []string{"<script>", "x", "y"},
			want:  [][]string{{"keyword:<script"}, {}, {}},
			end:   htmlRawText{tag: "script", inner: jsNormal{}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, htmlNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 여러 줄에 걸치는 것들이 각자 제자리로 돌아온다.
func TestHTMLMultiline(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "두 줄에 걸친 주석",
			lines: []string{"<!-- 여러 줄에", "걸친 주석 --><p>"},
			want: [][]string{
				{"comment:<!-- 여러 줄에"},
				{"comment:걸친 주석 -->", "keyword:<p"},
			},
			end: htmlNormal{},
		},
		{
			name:  "두 줄에 걸친 tag",
			lines: []string{"<div", `class="a">글`},
			want: [][]string{
				{"keyword:<div"},
				{"var:class", `string:"a"`},
			},
			end: htmlNormal{},
		},
		{
			name:  "두 줄에 걸친 속성 값",
			lines: []string{`<div title="여러 줄에`, `걸친 값" id="x">`},
			want: [][]string{
				{"keyword:<div", "var:title", `string:"여러 줄에`},
				{`string:걸친 값"`, "var:id", `string:"x"`},
			},
			end: htmlNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, htmlNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 여는 tag 와 닫는 tag 가 같은 줄에 있으면 그 줄에서 안쪽이 끝난다.
//
// 이것을 놓쳐서 `<script>x</script>` 뒤의 **파일 나머지 전부**가 script 안으로 읽혔다.
// TestHTMLRawText 는 닫는 tag 를 다음 줄에만 두어서 못 잡았다.
func TestHTMLRawTextClosesOnSameLine(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "한 줄에서 여닫으면 다음 줄은 다시 html 이다",
			lines: []string{"<script>let x = 1;</script>", "<p>글</p>"},
			want: [][]string{
				{"keyword:<script", "keyword:let", "number:1", "keyword:</script"},
				{"keyword:<p", "keyword:</p"},
			},
			end: htmlNormal{},
		},
		{
			name:  "빈 script",
			lines: []string{"<script></script>", "<p>글</p>"},
			want: [][]string{
				{"keyword:<script", "keyword:</script"},
				{"keyword:<p", "keyword:</p"},
			},
			end: htmlNormal{},
		},
		{
			name:  "한 줄에 script 둘",
			lines: []string{"<script>a</script><script>b</script>"},
			want: [][]string{
				{"keyword:<script", "keyword:</script", "keyword:<script", "keyword:</script"},
			},
			end: htmlNormal{},
		},
		{
			name:  "속성이 있고 같은 줄에서 닫힌다",
			lines: []string{`<script src="a.js">let x = 1;</script>`, "<p>글</p>"},
			want: [][]string{
				{
					"keyword:<script", "var:src", `string:"a.js"`,
					"keyword:let", "number:1", "keyword:</script",
				},
				{"keyword:<p", "keyword:</p"},
			},
			end: htmlNormal{},
		},
		{
			name:  "style 도 같다",
			lines: []string{"<style>a { color: red; }</style>", "<p>글</p>"},
			want: [][]string{
				{"keyword:<style", "type:a", "keyword:color", "const:red", "keyword:</style"},
				{"keyword:<p", "keyword:</p"},
			},
			end: htmlNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, htmlNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// `</script>` 는 안쪽에서 무엇이 열려 있든 이긴다. 브라우저도 그렇게 읽는다.
func TestHTMLRawTextClosingBeatsInner(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
	}{
		{
			name:  "닫지 않은 블록 주석을 두고 script 를 닫는다",
			lines: []string{"<script>", "/* 열었다", "</script>", "<p>글</p>"},
			want: [][]string{
				{"keyword:<script"},
				{"comment:/* 열었다"},
				{"keyword:</script"},
				{"keyword:<p", "keyword:</p"},
			},
		},
		{
			name:  "닫지 않은 백틱 문자열도 그렇다",
			lines: []string{"<script>", "const q = `열었다", "</script>", "<p>글</p>"},
			want: [][]string{
				{"keyword:<script"},
				{"keyword:const", "string:`열었다"},
				{"keyword:</script"},
				{"keyword:<p", "keyword:</p"},
			},
		},
		{
			name:  "js 문자열 안의 `</script>` 도 블록을 닫는다 — 브라우저와 같다",
			lines: []string{`<script>const s = "</script>";`, "<p>글</p>"},
			want: [][]string{
				// 닫는 tag 앞까지만 js 에 넘기므로 열린 `"` 가 미완성 문자열로 남는다.
				// 그것이 맞다 — 브라우저도 여기서 script 를 끝낸다.
				{"keyword:<script", "keyword:const", `string:"`, "keyword:</script"},
				{"keyword:<p", "keyword:</p"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, htmlNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, htmlNormal{}, next, "닫혀서 html 로 돌아온다")
		})
	}
}
