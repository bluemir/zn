package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPythonLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "def 뒤는 부르는 이름이다",
			line: "def 이름(x):",
			want: []string{"keyword:def", "func:이름"},
		},
		{
			name: "class 뒤는 type 이다",
			line: "class Buffer:",
			want: []string{"keyword:class", "type:Buffer"},
		},
		{
			name: "장식자",
			line: "@property",
			want: []string{"func:@property"},
		},
		{
			name: "점이 든 장식자",
			line: "@app.route",
			want: []string{"func:@app.route"},
		},
		{
			name: "줄 가운데의 `@` 는 장식자가 아니다",
			line: "x = a @ b",
			want: []string{},
		},
		{
			name: "값 낱말",
			line: "x = True or None",
			want: []string{"const:True", "keyword:or", "const:None"},
		},
		{
			name: "주석",
			line: "# 한글 주석이다",
			want: []string{"comment:# 한글 주석이다"},
		},
		{
			name: "주석 안의 세 겹 따옴표는 문자열을 열지 않는다",
			line: `# 여기 """ 는 문자열이 아니다`,
			want: []string{`comment:# 여기 """ 는 문자열이 아니다`},
		},
		{
			name: "숫자 꼴",
			line: "n = 0x1f + 1_000 + 1e9 + 3j",
			want: []string{"number:0x1f", "number:1_000", "number:1e9", "number:3j"},
		},
		{
			name: "지수의 부호는 숫자에 든다",
			line: "n = 1e-9",
			want: []string{"number:1e-9"},
		},
		{
			name: "접두 붙은 문자열",
			line: `p = r'\d+'`,
			want: []string{`string:r'\d+'`},
		},
		{
			name: "f-string 은 통째로 문자열이다",
			line: `s = f"{x} 개"`,
			want: []string{`string:f"{x} 개"`},
		},
		{
			name: "한 줄에서 닫힌 세 겹 따옴표",
			line: `s = """한 줄"""`,
			want: []string{`string:"""한 줄"""`},
		},
		{
			name: "`match` 는 예약어로 보지 않는다",
			line: "match = 1",
			want: []string{"number:1"},
		},
		{
			name: "닫히지 않은 한 겹 따옴표는 줄을 넘지 않는다",
			line: `s = "닫지 않았다`,
			want: []string{`string:"닫지 않았다`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, pyNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, pyNormal{}, next, "한 줄에서 끝나면 문맥이 그대로다")
		})
	}
}

// 세 겹 따옴표는 여는 글자와 같은 것으로만 닫힌다.
func TestPythonTripleQuote(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "세 줄에 걸친 docstring",
			lines: []string{`"""문서`, "가운데", `끝"""`},
			want: [][]string{
				{`string:"""문서`}, {"string:가운데"}, {`string:끝"""`},
			},
			end: pyNormal{},
		},
		{
			name:  "홑따옴표로 연 것은 쌍따옴표로 닫히지 않는다",
			lines: []string{"'''열었다", `"""아니다`},
			want:  [][]string{{"string:'''열었다"}, {`string:"""아니다`}},
			end:   pyTripleQuote{quote: '\''},
		},
		{
			name:  "닫은 뒤의 꼬리는 다시 보통 문맥이다",
			lines: []string{`s = """열었다`, `끝""" + str(1)`},
			want: [][]string{
				{`string:"""열었다`},
				{`string:끝"""`, "number:1"},
			},
			end: pyNormal{},
		},
		{
			name:  "안쪽의 `#` 은 주석이 아니다",
			lines: []string{`"""열었다`, "# 주석이 아니다", `"""`},
			want:  [][]string{{`string:"""열었다`}, {"string:# 주석이 아니다"}, {`string:"""`}},
			end:   pyNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, pyNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}
