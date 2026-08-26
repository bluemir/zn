package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "키와 문자열 값",
			line: `  "name": "한글 이름",`,
			want: []string{`key:"name"`, `string:"한글 이름"`},
		},
		{
			name: "키 뒤의 빈 칸을 지나 `:` 를 본다",
			line: `"name"   : 1`,
			want: []string{`key:"name"`, "number:1"},
		},
		{
			name: "값으로 쓰인 문자열은 키가 아니다",
			line: `["a", "b"]`,
			want: []string{`string:"a"`, `string:"b"`},
		},
		{
			name: "숫자",
			line: `{"a": -1.5, "b": 1e9, "c": 2E+3}`,
			want: []string{
				`key:"a"`, "number:-1.5",
				`key:"b"`, "number:1e9",
				`key:"c"`, "number:2E+3",
			},
		},
		{
			name: "json5 의 16 진수",
			line: `{ mask: 0x1f }`,
			want: []string{"key:mask", "number:0x1f"},
		},
		{
			// 지수부에 숫자가 없어서 `1` 까지가 숫자인데, 그 뒤에 글자가 붙어 있으므로
			// 따옴표 없는 값으로 본다. 반쪽만 숫자 색을 입히지 않는다.
			name: "지수부가 비면 숫자가 아니다",
			line: `[1e]`,
			want: []string{},
		},
		{
			name: "값 낱말",
			line: `{"ok": true, "off": false, "none": null}`,
			want: []string{
				`key:"ok"`, "const:true",
				`key:"off"`, "const:false",
				`key:"none"`, "const:null",
			},
		},
		{
			name: "키로 쓰인 값 낱말은 키다",
			line: `{ "null": 1 }`,
			want: []string{`key:"null"`, "number:1"},
		},
		{
			name: "따옴표 없는 키는 hjson·json5 의 것이다",
			line: `{ name: "값" }`,
			want: []string{"key:name", `string:"값"`},
		},
		{
			name: "한글 키도 이름이다",
			line: `{ 이름: 1 }`,
			want: []string{"key:이름", "number:1"},
		},
		{
			name: "따옴표 없는 값은 색이 없다",
			line: `{ name: 한글 그대로 }`,
			want: []string{"key:name"},
		},
		{
			name: "숫자로 시작하는 따옴표 없는 값도 색이 없다",
			line: `{ ver: 1abc }`,
			want: []string{"key:ver"},
		},
		{
			name: "`//` 주석은 jsonc·json5·hjson 의 것이다",
			line: `{ "a": 1 } // 한글 주석`,
			want: []string{`key:"a"`, "number:1", "comment:// 한글 주석"},
		},
		{
			name: "`#` 주석은 hjson 의 것이다",
			line: `# 한글 주석`,
			want: []string{"comment:# 한글 주석"},
		},
		{
			name: "한 줄에서 닫히는 블록 주석",
			line: `{ /* 한글 */ "a": 1 }`,
			want: []string{"comment:/* 한글 */", `key:"a"`, "number:1"},
		},
		{
			name: "홑따옴표 문자열은 json5 의 것이다",
			line: `{ 'a': 'b' }`,
			want: []string{"key:'a'", "string:'b'"},
		},
		{
			name: "닫히지 않은 따옴표는 줄 끝까지다",
			line: `{ "a": "열었다`,
			want: []string{`key:"a"`, `string:"열었다`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, jsonNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, jsonNormal{}, next, "이 줄은 여러 줄에 걸치지 않는다")
		})
	}
}

// 블록 주석은 줄을 넘는다. 닫히면 그 뒤부터 다시 보통 문맥이다.
func TestJSONBlockCommentSpansLines(t *testing.T) {
	got, next := lexedAll(t, jsonNormal{},
		`{ /* 여러 줄에`,
		`   걸친 주석`,
		`   여기서 끝난다 */ "a": 1 }`,
	)

	assert.Equal(t, [][]string{
		{"comment:/* 여러 줄에"},
		{"comment:   걸친 주석"},
		{"comment:   여기서 끝난다 */", `key:"a"`, "number:1"},
	}, got)
	assert.Equal(t, jsonNormal{}, next)
}

// hjson 의 `”'` 는 여러 줄 문자열이다.
func TestJSONMultilineStringSpansLines(t *testing.T) {
	got, next := lexedAll(t, jsonNormal{},
		`{ text: '''여러 줄에`,
		`걸친 한글 글`,
		`''' }`,
	)

	assert.Equal(t, [][]string{
		{"key:text", "string:'''여러 줄에"},
		{"string:걸친 한글 글"},
		{"string:'''"},
	}, got)
	assert.Equal(t, jsonNormal{}, next)
}

// 빈 줄에도 문맥은 이어진다. 빈 토큰을 내지 않는다.
func TestJSONKeepsStateOnEmptyLine(t *testing.T) {
	got, next := lexedAll(t, jsonNormal{}, `{ /* 열었다`, ``, `닫는다 */ }`)

	assert.Equal(t, [][]string{{"comment:/* 열었다"}, {}, {"comment:닫는다 */"}}, got)
	assert.Equal(t, jsonNormal{}, next)
}

func TestDetectJSON(t *testing.T) {
	for _, path := range []string{"package.json", "tsconfig.jsonc", "a.json5", "b.hjson", "A.JSON"} {
		assert.Equal(t, jsonNormal{}, LanguageFor(path).State(), path)
	}

	assert.Nil(t, LanguageFor("jsonfile").State(), "이름에 json 이 들었을 뿐이다")
}

func TestJSONIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{name: "여는 중괄호", line: `{`, want: 1},
		{name: "키 뒤의 여는 중괄호", line: `  "a": {`, want: 1},
		{name: "여는 대괄호", line: `  "a": [`, want: 1},
		{name: "한 줄에서 여닫으면 그대로다", line: `  "a": {"b": 1},`, want: 0},
		{name: "평범한 줄", line: `  "a": 1,`, want: 0},

		// braceIndent 를 나눠 쓰면 이 줄이 switch 이름표로 잡힌다.
		{name: "`case` 라는 키는 이름표가 아니다", line: `  case: 1,`, want: 0},
		{name: "문자열 안의 괄호는 세지 않는다", line: `  "a": "{",`, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "package.json", test.line)

			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}

	rule := LanguageFor("package.json").Indent()
	assert.Equal(t, 1, rule.Close([]byte("}")), "닫는 괄호는 한 단계 나온다")
	assert.Equal(t, 0, rule.Close([]byte(`"a": 1`)))
}

// 뼈대는 키가 여는 블록이다. 깊은 설정 파일에서 지금 어느 키 안인지가 화면 위에 남는다.
func TestJSONOutline(t *testing.T) {
	depth, heads := outlineOf(t, "package.json", `  "scripts": {`)
	assert.Equal(t, 2, depth)
	assert.True(t, heads, "블록을 여는 키가 아래를 거느린다")

	_, heads = outlineOf(t, "package.json", `    "build": "go build",`)
	assert.False(t, heads)
}
