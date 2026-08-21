package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarkdownLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{name: "제목은 줄 하나가 통째로다", line: "# 제목", want: []string{"heading:# 제목"}},
		{name: "여섯 단계까지 제목이다", line: "###### 제목", want: []string{"heading:###### 제목"}},
		{
			name: "일곱 개는 제목이 아니다",
			line: "####### 제목",
			want: []string{},
		},
		{name: "`#` 뒤에 빈 칸이 없으면 제목이 아니다", line: "#hashtag", want: []string{}},
		{name: "빈 제목", line: "#", want: []string{"heading:#"}},
		{name: "인용문은 주석과 같은 자리다", line: "> 남의 말이다", want: []string{"comment:> 남의 말이다"}},
		{
			name: "코드 스팬",
			line: "`replaceLines` 가 유일한 자리다",
			want: []string{"string:`replaceLines`"},
		},
		{
			name: "닫히지 않은 백틱은 코드 스팬이 아니다",
			line: "`replaceLines 가",
			want: []string{},
		},
		{
			name: "링크는 주소만 색이 붙는다",
			line: "[문서](https://example.com) 를 본다",
			want: []string{"link:https://example.com"},
		},
		{
			name: "꺾쇠 링크",
			line: "<https://example.com> 이다",
			want: []string{"link:<https://example.com>"},
		},
		{
			name: "꺾쇠 안에 빈 칸이 있으면 링크가 아니다",
			line: "<a b> 이다",
			want: []string{},
		},
		{name: "별표 강조", line: "이것은 *중요* 하다", want: []string{"em:*중요*"}},
		{
			name: "표시 둘은 굵기다. 기울임과 갈래가 다르다",
			line: "이것은 **아주** 하다",
			want: []string{"strong:**아주**"},
		},
		{
			name: "겹친 밑줄도 굵기다",
			line: "이것은 __아주__ 하다",
			want: []string{"strong:__아주__"},
		},
		{name: "밑줄 강조", line: "이것은 _중요_ 하다", want: []string{"em:_중요_"}},
		{
			name: "여는 표시 뒤에 빈 칸이 있으면 강조가 아니다",
			line: "3 * 4 * 5",
			want: []string{},
		},
		{
			name: "snake_case 는 강조를 열지 않는다",
			line: "styleSearchMatch 와 style_search_match 다",
			want: []string{},
		},
		{
			name: "코드 스팬이 강조보다 세다",
			line: "`a * b * c` 다",
			want: []string{"string:`a * b * c`"},
		},
		{
			name: "목록 표시에는 색을 주지 않는다",
			line: "- [ ] 할 일이다",
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, mdNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, mdNormal{}, next, "줄 안의 것은 문맥을 바꾸지 않는다")
		})
	}
}

// 링크와 문자열은 갈래가 다르다. 주소가 문자열 색이면 문서에서 링크와 인용된 글자가
// 같아 보인다.
func TestMarkdownLinkIsNotString(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "상대 경로 링크도 링크다 — 그래서 이름에 Web 을 붙이지 않았다",
			line: "[명세](docs/spec.md) 를 본다",
			want: []string{"link:docs/spec.md"},
		},
		{
			name: "링크와 코드 스팬이 같은 줄에 있으면 갈래가 갈린다",
			line: "`Detect` 는 [문서](https://example.com) 를 본다",
			want: []string{"string:`Detect`", "link:https://example.com"},
		},
		{
			name: "글은 본문과 같이 두고 주소만 칠한다",
			line: "[아주 긴 한글 링크 이름](x.md)",
			want: []string{"link:x.md"},
		},
		{
			name: "주소가 비면 링크가 아니다",
			line: "[글]() 이다",
			want: []string{},
		},
		{
			name: "닫는 괄호가 없으면 링크가 아니다",
			line: "[글](주소 를 안 닫았다",
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, mdNormal{}, test.line)

			assert.Equal(t, test.want, got)
		})
	}
}

// 코드펜스는 여는 글자와 길이가 같은 것으로만 닫힌다.
func TestMarkdownFence(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "백틱 셋으로 열고 닫는다",
			lines: []string{"```", "code", "```"},
			want:  [][]string{{"keyword:```"}, {}, {"keyword:```"}},
			end:   mdNormal{},
		},
		{
			name:  "여는 표시 뒤의 낱말이 언어를 정하고, 안쪽이 그 언어로 훑힌다",
			lines: []string{"```go", "func main() {}", "```"},
			want: [][]string{
				{"keyword:```", "type:go"},
				{"keyword:func", "func:main"},
				{"keyword:```"},
			},
			end: mdNormal{},
		},
		{
			name:  "물결로 연 것은 백틱으로 닫히지 않는다",
			lines: []string{"~~~", "```"},
			want:  [][]string{{"keyword:~~~"}, {}},
			end:   mdFence{marker: '~', size: 3},
		},
		{
			name:  "넷으로 연 것은 셋으로 닫히지 않는다",
			lines: []string{"````", "```", "````"},
			want:  [][]string{{"keyword:````"}, {}, {"keyword:````"}},
			end:   mdNormal{},
		},
		{
			name:  "더 긴 것으로는 닫힌다",
			lines: []string{"```", "````"},
			want:  [][]string{{"keyword:```"}, {"keyword:````"}},
			end:   mdNormal{},
		},
		{
			name:  "안쪽은 제목도 강조도 아니다",
			lines: []string{"```", "# 제목이 아니다", "*강조도 아니다*"},
			want:  [][]string{{"keyword:```"}, {}, {}},
			end:   mdFence{marker: '`', size: 3},
		},
		{
			name:  "닫는 표시 뒤에 글자가 있으면 닫히지 않는다",
			lines: []string{"```", "``` 아직"},
			want:  [][]string{{"keyword:```"}, {}},
			end:   mdFence{marker: '`', size: 3},
		},
		{
			name:  "둘만으로는 열리지 않는다",
			lines: []string{"``"},
			want:  [][]string{{}},
			end:   mdNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, mdNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 코드펜스 안은 info string 이 가리키는 언어로 훑는다.
func TestMarkdownFenceDelegates(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "golang 도 go 다",
			lines: []string{"```golang", "var x int = 1"},
			want: [][]string{
				{"keyword:```", "type:golang"},
				{"keyword:var", "type:int", "number:1"},
			},
			end: mdFence{marker: '`', size: 3, inner: goNormal{}},
		},
		{
			name:  "js",
			lines: []string{"```js", "const x = null;"},
			want: [][]string{
				{"keyword:```", "type:js"},
				{"keyword:const", "const:null"},
			},
			end: mdFence{marker: '`', size: 3, inner: jsNormal{}},
		},
		{
			name:  "sh",
			lines: []string{"```sh", "make build      # 만든다"},
			want: [][]string{
				{"keyword:```", "type:sh"},
				{"comment:# 만든다"},
			},
			end: mdFence{marker: '`', size: 3, inner: shNormal{}},
		},
		{
			name:  "첫 낱말만 언어를 정하고 색은 전체에 붙는다",
			lines: []string{"```go title=x", "var x int = 1"},
			want: [][]string{
				{"keyword:```", "type:go title=x"},
				{"keyword:var", "type:int", "number:1"},
			},
			end: mdFence{marker: '`', size: 3, inner: goNormal{}},
		},
		{
			name:  "모르는 언어는 색이 없다 — 위임을 넣기 전과 같다",
			lines: []string{"```brainfuck", "++++[>++++<-]"},
			want:  [][]string{{"keyword:```", "type:brainfuck"}, {}},
			end:   mdFence{marker: '`', size: 3},
		},
		{
			name:  "언어 이름이 없으면 색이 없다",
			lines: []string{"```", "func main() {}"},
			want:  [][]string{{"keyword:```"}, {}},
			end:   mdFence{marker: '`', size: 3},
		},
		{
			name:  "안쪽 언어의 문맥이 줄을 넘어 이어진다",
			lines: []string{"```go", "s := `여러 줄에", "걸친 문자열`", "```"},
			want: [][]string{
				{"keyword:```", "type:go"},
				{"string:`여러 줄에"},
				{"string:걸친 문자열`"},
				{"keyword:```"},
			},
			end: mdNormal{},
		},
		{
			name:  "들여쓴 펜스도 안쪽을 훑는다. 들여쓰기는 벗기지 않는다",
			lines: []string{"  ```go", "  var x int = 1", "  ```"},
			want: [][]string{
				{"keyword:```", "type:go"},
				{"keyword:var", "type:int", "number:1"},
				{"keyword:```"},
			},
			end: mdNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, mdNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// **닫는 표시가 안쪽 언어보다 세다.**
//
// 이 순서가 아니면 안쪽에서 열린 raw string 이나 블록 주석이 닫는 펜스를 먹어서 펜스가 영영
// 닫히지 않고, 그때부터 문서 나머지가 코드로 그려진다(ADR-0040).
func TestMarkdownFenceClosingBeatsInner(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
	}{
		{
			name:  "닫지 않은 raw string 을 두고 펜스를 닫는다",
			lines: []string{"```go", "s := `열었다", "```", "# 다시 제목이다"},
			want: [][]string{
				{"keyword:```", "type:go"},
				{"string:`열었다"},
				{"keyword:```"},
				{"heading:# 다시 제목이다"},
			},
		},
		{
			name:  "닫지 않은 블록 주석도 그렇다",
			lines: []string{"```go", "/* 열었다", "```", "# 다시 제목이다"},
			want: [][]string{
				{"keyword:```", "type:go"},
				{"comment:/* 열었다"},
				{"keyword:```"},
				{"heading:# 다시 제목이다"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, mdNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, mdNormal{}, next, "펜스가 닫혀 markdown 으로 돌아온다")
		})
	}
}

// 펜스가 겹쳐도 안쪽 언어가 이어진다. 바깥 펜스는 더 긴 표시로만 닫힌다.
func TestMarkdownFenceNests(t *testing.T) {
	got, next := lexedAll(t, mdNormal{},
		"````markdown",
		"```go",
		"var x int = 1",
		"```",
		"# 안쪽 markdown 의 제목",
		"````",
		"# 바깥 markdown 의 제목",
	)

	assert.Equal(t, [][]string{
		{"keyword:````", "type:markdown"},
		{"keyword:```", "type:go"},
		{"keyword:var", "type:int", "number:1"},
		{"keyword:```"},
		{"heading:# 안쪽 markdown 의 제목"},
		{"keyword:````"},
		{"heading:# 바깥 markdown 의 제목"},
	}, got)
	assert.Equal(t, mdNormal{}, next)
}
