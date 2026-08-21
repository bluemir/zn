package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "예약어와 부르는 이름",
			line: "const x = foo(1);",
			want: []string{"keyword:const", "func:foo", "number:1"},
		},
		{
			name: "class 뒤는 type 이다",
			line: "class Buffer extends Base {}",
			want: []string{"keyword:class", "type:Buffer", "keyword:extends"},
		},
		{
			name: "new 뒤도 type 이다",
			line: "const b = new Buffer();",
			want: []string{"keyword:const", "keyword:new", "type:Buffer"},
		},
		{
			name: "function 뒤는 부르는 이름이다",
			line: "function 이름(x) {}",
			want: []string{"keyword:function", "func:이름"},
		},
		{
			name: "값 낱말",
			line: "x = true || null || undefined;",
			want: []string{"const:true", "const:null", "const:undefined"},
		},
		{
			name: "줄 주석",
			line: "// 한글 주석이다",
			want: []string{"comment:// 한글 주석이다"},
		},
		{
			name: "한 줄에서 끝난 블록 주석",
			line: "x = 1; /* 주석 */ y = 2;",
			want: []string{"number:1", "comment:/* 주석 */", "number:2"},
		},
		{
			name: "숫자 꼴",
			line: "n = 0x1f + 1_000n + 0b101 + 1e9;",
			want: []string{"number:0x1f", "number:1_000n", "number:0b101", "number:1e9"},
		},
		{
			name: "따옴표 문자열",
			line: `s = "한글" + 'a\'b';`,
			want: []string{`string:"한글"`, `string:'a\'b'`},
		},
		{
			name: "한 줄에서 닫힌 백틱 문자열",
			line: "s = `한 줄`;",
			want: []string{"string:`한 줄`"},
		},
		{
			name: "백틱 안의 보간은 통째로 문자열이다",
			line: "s = `값은 ${x} 이다`;",
			want: []string{"string:`값은 ${x} 이다`"},
		},
		{
			name: "정규식 리터럴은 다루지 않는다 — `/` 가 그냥 지나간다",
			line: "const r = /ab+/;",
			want: []string{"keyword:const"},
		},
		{
			name: "닫히지 않은 따옴표는 줄을 넘지 않는다",
			line: `s = "닫지 않았다`,
			want: []string{`string:"닫지 않았다`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, jsNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, jsNormal{}, next, "한 줄에서 끝나면 문맥이 그대로다")
		})
	}
}

// 백틱 문자열과 블록 주석만 줄을 넘는다.
func TestJSMultiline(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "세 줄에 걸친 백틱 문자열",
			lines: []string{"const q = `SELECT", "FROM 사용자", "WHERE id = 1` + x;"},
			want: [][]string{
				{"keyword:const", "string:`SELECT"},
				{"string:FROM 사용자"},
				{"string:WHERE id = 1`"},
			},
			end: jsNormal{},
		},
		{
			name:  "두 줄에 걸친 블록 주석",
			lines: []string{"/* 여러 줄에", "걸친 주석 */ const x = 1;"},
			want: [][]string{
				{"comment:/* 여러 줄에"},
				{"comment:걸친 주석 */", "keyword:const", "number:1"},
			},
			end: jsNormal{},
		},
		{
			name:  "백틱 안의 `//` 는 주석이 아니다",
			lines: []string{"s = `열었다", "// 주석이 아니다", "`;"},
			want: [][]string{
				{"string:`열었다"}, {"string:// 주석이 아니다"}, {"string:`"},
			},
			end: jsNormal{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, jsNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}
