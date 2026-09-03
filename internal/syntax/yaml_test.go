package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYAMLLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "키와 따옴표 없는 값",
			line: "name: 한글 이름",
			want: []string{"key:name"},
		},
		{
			name: "따옴표 있는 값",
			line: `  name: "한글 이름"`,
			want: []string{"key:name", `string:"한글 이름"`},
		},
		{
			name: "홑따옴표도 문자열이다",
			line: "name: '한글'",
			want: []string{"key:name", "string:'한글'"},
		},
		{
			name: "값 없는 키",
			line: "services:",
			want: []string{"key:services"},
		},
		{
			name: "목록 표시 뒤의 키",
			line: "  - name: web",
			want: []string{"key:name"},
		},
		{
			name: "목록 항목이 값뿐이면 키가 없다",
			line: "  - 80:8080",
			want: []string{},
		},
		{
			name: "키에 붙은 빈 칸은 이름에서 뗀다",
			line: "name  : x",
			want: []string{"key:name"},
		},
		{
			name: "`:` 뒤에 빈 칸이 없으면 키가 아니다",
			line: "  url: http://example.com/a",
			want: []string{"key:url"},
		},
		{
			name: "따옴표로 적은 키는 안의 `:` 를 품는다",
			line: `"a: b": 1`,
			want: []string{`key:"a: b"`, "number:1"},
		},
		{
			name: "숫자",
			line: "port: 8080",
			want: []string{"key:port", "number:8080"},
		},
		{
			name: "날짜는 숫자로 쪼개지 않는다",
			line: "date: 2026-08-23",
			want: []string{"key:date"},
		},
		{
			name: "값 낱말",
			line: "enabled: true",
			want: []string{"key:enabled", "const:true"},
		},
		{
			name: "yaml 1.1 의 값 낱말",
			line: "enabled: no",
			want: []string{"key:enabled", "const:no"},
		},
		{
			name: "낱말 뒤에 값이 이어지면 낱말이 아니다",
			line: "answer: no problem",
			want: []string{"key:answer"},
		},
		{
			name: "주석",
			line: "port: 80  # 한글 주석",
			want: []string{"key:port", "number:80", "comment:# 한글 주석"},
		},
		{
			name: "줄 전체가 주석",
			line: "  # 한글 주석",
			want: []string{"comment:# 한글 주석"},
		},
		{
			name: "주석만 있는 줄에는 키가 없다",
			line: "# a: b",
			want: []string{"comment:# a: b"},
		},
		{
			name: "값 안의 `#` 은 주석이 아니다",
			line: "color: '#fff'",
			want: []string{"key:color", "string:'#fff'"},
		},
		{
			name: "anchor 와 alias 는 가리키는 자리다",
			line: "base: &공통 x",
			want: []string{"key:base", "var:&공통"},
		},
		{
			name: "alias 로 가리킨다",
			line: "dev: *공통",
			want: []string{"key:dev", "var:*공통"},
		},
		{
			name: "tag 는 값의 type 이다",
			line: "port: !!str 80",
			want: []string{"key:port", "type:!!str", "number:80"},
		},
		{
			name: "문서 경계",
			line: "---",
			want: []string{"keyword:---"},
		},
		{
			name: "문서 경계 뒤에 tag 가 붙는다",
			line: "--- !!map",
			want: []string{"keyword:---", "type:!!map"},
		},
		{
			name: "흐름 모음 안에서는 키를 찾지 않는다",
			line: "ports: [80, 443]",
			want: []string{"key:ports", "number:80", "number:443"},
		},
		{
			name: "`a | b` 의 `|` 는 블록 표시가 아니다",
			line: "cmd: sh -c a | b",
			want: []string{"key:cmd"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, yamlNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, yamlNormal{}, next, "이 줄은 블록을 열지 않는다")
		})
	}
}

// 블록 스칼라는 들여쓰기가 끝을 알린다. 표시가 있던 줄보다 깊은 줄이 그 내용이다.
func TestYAMLBlockScalar(t *testing.T) {
	got, next := lexedAll(t, yamlNormal{},
		"  script: |",
		"    go build ./...",
		"    echo 한글",
		"",
		"    make test",
		"  next: 1",
	)

	assert.Equal(t, [][]string{
		{"key:script", "keyword:|"},
		{"string:    go build ./..."},
		{"string:    echo 한글"},
		{},
		{"string:    make test"},
		{"key:next", "number:1"},
	}, got)
	assert.Equal(t, yamlNormal{}, next, "블록이 끝나면 보통 문맥이다")
}

// 자르기와 들여쓰기 숫자가 붙어도 표시다. 뒤에 주석은 올 수 있다.
func TestYAMLBlockScalarModifiers(t *testing.T) {
	tests := []string{"text: >-", "text: |+", "text: |2", "text: | # 한글"}

	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			_, next := lexed(t, yamlNormal{}, line)

			assert.Equal(t, yamlBlock{indent: 0}, next, "블록으로 들어간다")
		})
	}
}

func TestDetectYAML(t *testing.T) {
	for _, path := range []string{"a.yaml", ".github/workflows/ci.yml", "A.YAML"} {
		assert.Equal(t, yamlNormal{}, LanguageFor(path).State(), path)
	}
}

func TestYAMLIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{name: "값 없는 키 아래는 그 내용이다", line: "services:", want: 1},
		{name: "값이 있으면 그대로다", line: "port: 8080", want: 0},

		// codeBytes 로 문자열까지 지우면 `key:` 로 보여서 값 있는 줄이 들여쓰인다.
		{name: "값이 문자열이어도 그대로다", line: `name: "한글"`, want: 0},
		{name: "값 뒤에 주석이 붙어도 그대로다", line: "port: 80 # 한글", want: 0},
		{name: "키 뒤에 주석만 있으면 내용이 온다", line: "services: # 한글", want: 1},
		{name: "블록 표시 뒤는 그 내용이다", line: "script: |", want: 1},
		{name: "자르기가 붙은 블록 표시도 같다", line: "script: >-", want: 1},
		{name: "목록 표시만 있는 줄", line: "  -", want: 1},
		{name: "값이 `-` 로 끝나는 것은 목록 표시가 아니다", line: "name: 값-", want: 0},
		{name: "여는 흐름 모음", line: "ports: [", want: 1},
		{name: "한 줄에서 여닫으면 그대로다", line: "ports: [80, 443]", want: 0},
		{name: "빈 줄", line: "", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "a.yaml", test.line)

			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}
}

// yaml 은 들여쓰기가 곧 뜻이라 `=` 로 되짚지 않는다. markdown 과 같은 까닭이다.
func TestYAMLDoesNotReindent(t *testing.T) {
	assert.False(t, LanguageFor("a.yaml").Indent().Reindents())

	_, indents := LanguageFor("a.yaml").Indent().TabIndentsLine([]byte("  - 항목"))
	assert.False(t, indents)

	assert.Equal(t, []byte("  "), LanguageFor("a.yaml").Indent().Unit())
}

// 뼈대는 값 없는 키다. 깊은 설정 파일에서 지금 어느 키 안인지가 화면 위에 남는다.
func TestYAMLOutline(t *testing.T) {
	depth, heads := outlineOf(t, "a.yaml", "  services:")
	assert.Equal(t, 2, depth)
	assert.True(t, heads, "값 없는 키가 아래를 거느린다")

	depth, heads = outlineOf(t, "a.yaml", "    image: nginx")
	assert.Equal(t, 4, depth)
	assert.False(t, heads)

	_, heads = outlineOf(t, "a.yaml", "# 한글 주석")
	assert.False(t, heads, "주석은 거느리지 않는다")
}
