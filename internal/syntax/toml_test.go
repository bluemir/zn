package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTOMLLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "표 머리는 제목이다",
			line: "[package]",
			want: []string{"heading:[package]"},
		},
		{
			name: "배열 표 머리",
			line: "[[bin]]",
			want: []string{"heading:[[bin]]"},
		},
		{
			name: "점으로 이은 머리와 따옴표 이름 안의 `]`",
			line: `[target."cfg(a])".dependencies] # 주석`,
			want: []string{`heading:[target."cfg(a])".dependencies]`, "comment:# 주석"},
		},
		{
			name: "키와 문자열",
			line: `name = "zn"`,
			want: []string{"key:name", `string:"zn"`},
		},
		{
			name: "`-` 가 든 키와 점으로 이은 키",
			line: `serde-json.version = "1"`,
			want: []string{"key:serde-json.version", `string:"1"`},
		},
		{
			name: "따옴표 키",
			line: `"my crate" = 1`,
			want: []string{`key:"my crate"`, "number:1"},
		},
		{
			name: "인라인 표 안의 키",
			line: `serde = { version = "1.0", features = ["derive"], optional = true }`,
			want: []string{"key:serde", "key:version", `string:"1.0"`, "key:features", `string:"derive"`, "key:optional", "const:true"},
		},
		{
			name: "수 꼴",
			line: "a = [1_000, 0xff, 3.14, -1e6, inf, +nan]",
			want: []string{"key:a", "number:1_000", "number:0xff", "number:3.14", "number:-1e6", "number:inf", "number:+nan"},
		},
		{
			name: "날짜와 시각",
			line: "at = 1979-05-27T07:32:00Z",
			want: []string{"key:at", "number:1979-05-27T07:32:00Z"},
		},
		{
			name: "literal 문자열은 escape 가 없다",
			line: `path = 'C:\dir\' # 주석`,
			want: []string{"key:path", `string:'C:\dir\'`, "comment:# 주석"},
		},
		{
			name: "basic 문자열의 escape 된 따옴표",
			line: `s = "a\"b" # 주석`,
			want: []string{"key:s", `string:"a\"b"`, "comment:# 주석"},
		},
		{
			name: "문자열 안의 `#` 는 주석이 아니다",
			line: `url = "https://x#frag"`,
			want: []string{"key:url", `string:"https://x#frag"`},
		},
		{
			name: "한 줄에서 닫힌 세 겹 따옴표",
			line: `s = """a "b" c"""`,
			want: []string{"key:s", `string:"""a "b" c"""`},
		},
		{
			name: "닫는 세 겹 앞에 따옴표가 더 붙는다",
			line: `s = """"a""""`,
			want: []string{"key:s", `string:""""a""""`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, tomlNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, tomlNormal{}, next, "한 줄에서 끝나면 문맥이 그대로다")
		})
	}
}

// 줄을 넘는 것은 여러 줄 배열과 세 겹 따옴표다.
func TestTOMLMultiline(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "여러 줄 배열 안의 `[` 는 표 머리가 아니다",
			lines: []string{"matrix = [", `    [1, "a"],`, "]", "[next]"},
			want: [][]string{
				{"key:matrix"},
				{"number:1", `string:"a"`},
				{},
				{"heading:[next]"},
			},
			end: tomlNormal{},
		},
		{
			name:  "여러 줄 배열 안의 문자열은 키가 아니다",
			lines: []string{"keywords = [", `    "editor", # 주석`},
			want: [][]string{
				{"key:keywords"},
				{`string:"editor"`, "comment:# 주석"},
			},
			end: tomlNormal{depth: 1},
		},
		{
			name:  "세 겹 따옴표가 줄을 넘는다",
			lines: []string{`desc = """`, `[이것은 머리가 아니다]`, `a = 1`, `끝""" # 주석`},
			want: [][]string{
				{"key:desc", `string:"""`},
				{"string:[이것은 머리가 아니다]"},
				{"string:a = 1"},
				{`string:끝"""`, "comment:# 주석"},
			},
			end: tomlNormal{},
		},
		{
			name:  "배열 안에서 연 세 겹 따옴표는 닫힌 뒤 배열로 돌아간다",
			lines: []string{`a = [ '''`, `안'''`},
			want: [][]string{
				{"key:a", "string:'''"},
				{"string:안'''"},
			},
			end: tomlNormal{depth: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, tomlNormal{}, test.lines...)

			for i := range got {
				if got[i] == nil {
					got[i] = []string{}
				}
			}

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 여러 줄 배열과 인라인 표만 들여쓴다. 표 머리는 같은 줄에서 닫혀서 단계를 열지 않는다.
func TestTOMLIndent(t *testing.T) {
	rule := tomlIndent{}

	depth, _ := rule.Next([]byte("keywords = ["), nil)
	assert.Equal(t, 1, depth)

	depth, _ = rule.Next([]byte("[package]"), nil)
	assert.Equal(t, 0, depth)

	assert.Equal(t, 1, rule.Close([]byte("]")))
	assert.Equal(t, []byte("    "), rule.Unit())
}
