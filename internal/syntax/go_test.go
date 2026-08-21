package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "예약어와 부르는 이름",
			line: "func main() {",
			want: []string{"keyword:func", "func:main"},
		},
		{
			name: "미리 선언된 이름은 표가 정한다",
			line: "var x int = 3",
			want: []string{"keyword:var", "type:int", "number:3"},
		},
		{
			name: "`type` 바로 뒤는 새 type 의 이름이다",
			line: "type goNormal struct{}",
			want: []string{"keyword:type", "type:goNormal", "keyword:struct"},
		},
		{
			name: "숫자 꼴 전부",
			line: "\tn := 0x1f + 1e9 + 3i",
			want: []string{"number:0x1f", "number:1e9", "number:3i"},
		},
		{
			name: "글자 literal 은 문자열과 같은 갈래다",
			line: "\tc := 'x'",
			want: []string{"string:'x'"},
		},
		{
			name: "한글 문자열과 한글 주석",
			line: "\tname := \"한글 이름\" // 한글 주석이다",
			want: []string{"string:\"한글 이름\"", "comment:// 한글 주석이다"},
		},
		{
			name: "패키지 이름 뒤의 부르는 이름",
			line: "\treturn nil, errors.New(\"실패\")",
			want: []string{"keyword:return", "const:nil", "func:New", "string:\"실패\""},
		},
		{
			name: "줄 주석만 있는 줄",
			line: "// colorWhitespace 는 공백 마커의 색이다(ADR-0005).",
			want: []string{"comment:// colorWhitespace 는 공백 마커의 색이다(ADR-0005)."},
		},
		{
			name: "연산자와 괄호는 갈래가 없어 토큰이 되지 않는다",
			line: "\tx = (a + b) * c",
			want: []string{},
		},
		{
			name: "깨진 글자에도 죽지 않는다",
			line: "@#$%",
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, goNormal{}, test.line)

			assert.Equal(t, test.want, got)
		})
	}
}

// 줄 끝에 저절로 끼는 `;` 는 소스에 없는 글자라 토큰이 되면 없는 byte 에 색을 입힌다.
func TestGoDropsAutoSemicolon(t *testing.T) {
	line := "\treturn"

	tokens, _ := goNormal{}.Lex([]byte(line))

	for _, token := range tokens {
		assert.LessOrEqual(t, token.End, len(line), "자리가 줄을 넘습니다")
	}
	assert.Equal(t, []Token{{Start: 1, End: 7, Kind: KindKeyword}}, tokens)
}

// Go 에서 줄을 넘는 것은 raw string 과 block comment 둘뿐이다.
func TestGoNextState(t *testing.T) {
	tests := []struct {
		name string
		line string
		want State
	}{
		{name: "보통 줄", line: "func main() {", want: goNormal{}},
		{name: "raw string 을 열었다", line: "\ts := `raw", want: goRawString{}},
		{name: "raw string 을 열고 닫았다", line: "\ts := `raw`", want: goNormal{}},
		{
			name: "백틱 하나는 여는 조건과 닫는 조건을 동시에 만족한다",
			line: "\ts := `",
			want: goRawString{},
		},
		{name: "빈 raw string", line: "\ts := ``", want: goNormal{}},
		{name: "닫은 뒤 다시 열었다", line: "\ts := `a` + `b", want: goRawString{}},
		{name: "block comment 를 열었다", line: "\t/* 주석", want: goBlockComment{}},
		{name: "block comment 를 열고 닫았다", line: "\t/* 주석 */", want: goNormal{}},
		{
			name: "`/*/` 는 끝 두 글자가 `*/` 인데 아직 열려 있다",
			line: "/*/",
			want: goBlockComment{},
		},
		{name: "가장 짧게 닫힌 block comment", line: "/**/", want: goNormal{}},
		{
			name: "`\"` 문자열은 닫히지 않아도 줄을 넘지 않는다",
			line: "\ts2 := \"닫지 않았다",
			want: goNormal{},
		},
		{name: "빈 줄", line: "", want: goNormal{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, next := lexed(t, goNormal{}, test.line)

			assert.Equal(t, test.want, next)
		})
	}
}

// 여러 줄에 걸친 raw string 은 줄마다 통째로 문자열이고, 닫은 뒤 꼬리는 다시 보통 문맥이다.
func TestGoRawStringAcrossLines(t *testing.T) {
	got, next := lexedAll(t, goNormal{},
		"\ts := `여러 줄에",
		"가운데 줄",
		"걸친 문자열` + len(x)",
	)

	assert.Equal(t, [][]string{
		{"string:`여러 줄에"},
		{"string:가운데 줄"},
		{"string:걸친 문자열`", "func:len"},
	}, got)
	assert.Equal(t, goNormal{}, next, "닫혔으면 보통 문맥으로 돌아온다")
}

// 여러 줄에 걸친 block comment 도 같다. 닫은 자리 뒤는 lexTail 이 자리를 되돌린다.
func TestGoBlockCommentAcrossLines(t *testing.T) {
	got, next := lexedAll(t, goNormal{},
		"\t/* 블록 주석이",
		"\t   여기서 끝난다 */ n := 0x1f",
	)

	assert.Equal(t, [][]string{
		{"comment:/* 블록 주석이"},
		{"comment:\t   여기서 끝난다 */", "number:0x1f"},
	}, got)
	assert.Equal(t, goNormal{}, next)
}

// 여러 줄에 걸친 것 안의 빈 줄은 그것을 닫지 않는다.
func TestGoEmptyLineInsideMultiline(t *testing.T) {
	for _, state := range []State{goRawString{}, goBlockComment{}} {
		tokens, next := state.Lex(nil)

		assert.Empty(t, tokens)
		assert.Equal(t, state, next, "%T 가 빈 줄에서 닫혔습니다", state)
	}
}

// 식별자 둘이 공백만 두고 붙으면 뒤쪽은 type 자리다.
// `const aa BB = "ss"` 의 `BB` 처럼 표에도 없고 `type` 뒤도 아닌 이름이 여기서 잡힌다.
func TestGoTypePosition(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "const 선언의 type",
			line: `const aa BB = "ss"`,
			want: []string{"keyword:const", "type:BB", `string:"ss"`},
		},
		{
			name: "var 선언의 type",
			line: "var cache syntaxCache",
			want: []string{"keyword:var", "type:syntaxCache"},
		},
		{
			name: "struct 필드",
			line: "\tlines []syntaxLine",
			want: []string{"type:syntaxLine"},
		},
		{
			name: "map 의 key 와 값 type",
			line: "\tnested map[string]Kind",
			want: []string{"keyword:map", "type:string", "type:Kind"},
		},
		{
			name: "포인터 receiver 와 인자·결과 type",
			line: "func (buf *Buffer) write(n int) error {",
			want: []string{
				"keyword:func", "type:Buffer", "func:write", "type:int", "type:error",
			},
		},
		{
			name: "package 이름이 붙은 type 은 둘 다 색이 붙는다",
			line: "\tstart syntax.State",
			want: []string{"type:syntax", "type:State"},
		},
		{
			name: "곱셈은 type 자리가 아니다 — gofmt 가 `*` 양쪽에 빈 칸을 둔다",
			line: "\tn := a * b",
			want: []string{},
		},
		{
			name: "괄호를 지난 곱셈도 그렇다",
			line: "\tn := (a + b) * c",
			want: []string{},
		},
		{
			name: "부르는 자리는 type 이 아니다",
			line: "\tout := errors.Wrapf(err, \"실패\")",
			want: []string{"func:Wrapf", `string:"실패"`},
		},
		{
			name: "필드 읽기는 type 이 아니다",
			line: "\tbuf.diskHash = sum",
			want: []string{},
		},
		{
			name: "예약어 뒤는 type 자리가 아니다",
			line: "package core",
			want: []string{"keyword:package"},
		},
		{
			name: "`return x` 도 아니다",
			line: "\treturn err",
			want: []string{"keyword:return"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, goNormal{}, test.line)

			assert.Equal(t, test.want, got)
		})
	}
}
