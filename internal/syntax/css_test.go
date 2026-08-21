package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCSSLexLine(t *testing.T) {
	tests := []struct {
		name  string
		state State
		line  string
		want  []string
		end   State
	}{
		{
			name:  "선택자 자리의 이름은 무엇을 가리키는지다",
			state: cssNormal{},
			line:  ".a, #b > c:hover {",
			want:  []string{"type:.a", "type:#b", "type:c", "type:hover"},
			end:   cssBlock{},
		},
		{
			name:  "선언은 속성과 값으로 갈린다",
			state: cssBlock{},
			line:  "color: red;",
			want:  []string{"keyword:color", "const:red"},
			end:   cssBlock{},
		},
		{
			name:  "세미콜론 뒤는 다시 속성 자리다",
			state: cssBlock{},
			line:  "display: block; color: red;",
			want:  []string{"keyword:display", "const:block", "keyword:color", "const:red"},
			end:   cssBlock{},
		},
		{
			name:  "값 자리의 `#fff` 는 색이다",
			state: cssBlock{},
			line:  "color: #fff;",
			want:  []string{"keyword:color", "number:#fff"},
			end:   cssBlock{},
		},
		{
			name:  "숫자와 단위는 하나다",
			state: cssBlock{},
			line:  "margin: 0.5em 10px 50%;",
			want:  []string{"keyword:margin", "number:0.5em", "number:10px", "number:50%"},
			end:   cssBlock{},
		},
		{
			name:  "사용자 속성",
			state: cssBlock{},
			line:  "--my-color: red;",
			want:  []string{"var:--my-color", "const:red"},
			end:   cssBlock{},
		},
		{
			name:  "var() 안의 사용자 속성",
			state: cssBlock{},
			line:  "color: var(--my-color);",
			want:  []string{"keyword:color", "const:var", "var:--my-color"},
			end:   cssBlock{},
		},
		{
			name:  "at-rule",
			state: cssNormal{},
			line:  "@media (min-width: 700px) {",
			want:  []string{"keyword:@media", "type:min-width", "number:700px"},
			end:   cssBlock{},
		},
		{
			name:  "!important",
			state: cssBlock{},
			line:  "color: red !important;",
			want:  []string{"keyword:color", "const:red", "keyword:!important"},
			end:   cssBlock{},
		},
		{
			name:  "문자열 안의 중괄호는 블록을 닫지 않는다",
			state: cssBlock{},
			line:  `content: "}";`,
			want:  []string{"keyword:content", `string:"}"`},
			end:   cssBlock{},
		},
		{
			name:  "중괄호를 닫으면 선택자 자리로 돌아온다",
			state: cssBlock{},
			line:  "}",
			want:  []string{},
			end:   cssNormal{},
		},
		{
			name:  "한 줄에서 열고 닫는다",
			state: cssNormal{},
			line:  "a { color: red; }",
			want:  []string{"type:a", "keyword:color", "const:red"},
			end:   cssNormal{},
		},
		{
			name:  "한 줄에서 끝난 주석",
			state: cssBlock{},
			line:  "color: red; /* 한글 주석 */",
			want:  []string{"keyword:color", "const:red", "comment:/* 한글 주석 */"},
			end:   cssBlock{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, test.state, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 주석은 열린 자리로 돌아간다. 그래서 cssComment 가 어디서 열렸는지를 든다.
func TestCSSCommentReturnsToItsPlace(t *testing.T) {
	tests := []struct {
		name  string
		state State
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "선택자 자리에서 열린 주석",
			state: cssNormal{},
			lines: []string{"/* 여러 줄에", "걸친 주석 */ a {"},
			want: [][]string{
				{"comment:/* 여러 줄에"},
				{"comment:걸친 주석 */", "type:a"},
			},
			end: cssBlock{},
		},
		{
			name:  "선언 자리에서 열린 주석",
			state: cssBlock{},
			lines: []string{"/* 여러 줄에", "걸친 주석 */ color: red;"},
			want: [][]string{
				{"comment:/* 여러 줄에"},
				{"comment:걸친 주석 */", "keyword:color", "const:red"},
			},
			end: cssBlock{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, test.state, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 여러 줄에 걸친 선언은 상태를 더 두지 않고 색이 이어진다.
func TestCSSDeclarationAcrossLines(t *testing.T) {
	got, next := lexedAll(t, cssNormal{},
		"a {",
		"\tbox-shadow: 0 1px 2px",
		"\t\trgba(0, 0, 0, 0.5);",
		"}",
	)

	assert.Equal(t, [][]string{
		{"type:a"},
		{"keyword:box-shadow", "number:0", "number:1px", "number:2px"},
		{"const:rgba", "number:0", "number:0", "number:0", "number:0.5"},
		{},
	}, got)
	assert.Equal(t, cssNormal{}, next)
}
