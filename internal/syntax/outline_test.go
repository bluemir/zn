package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outlineOf 는 그 언어의 lexer 로 줄을 훑은 뒤 뼈대 규칙에 물어본다.
//
// nextOf 와 같은 손이다 — 코드펜스 걸러내기와 문자열·주석 거르기가 토큰에 매여 있어서,
// 토큰을 손으로 적으면 그 거르기가 시험되지 않는다.
func outlineOf(t *testing.T, path, line string) (int, bool) {
	t.Helper()

	rule := LanguageFor(path).Outline()
	require.NotNil(t, rule, "%s 에 뼈대 규칙이 없다", path)

	state := LanguageFor(path).State()
	require.NotNil(t, state, "%s 를 훑을 수 없다", path)

	tokens, _ := state.Lex([]byte(line))

	return rule.Depth([]byte(line), tokens), rule.Heads([]byte(line), tokens)
}

func TestOutlineForEveryLanguage(t *testing.T) {
	// 언어를 더하면서 칸을 빠뜨리면 그 언어만 조용히 머리줄이 없어진다.
	for _, rule := range languageRules {
		assert.NotNil(t, rule.outline, "%v 에 뼈대 규칙이 없다", rule.aliases)
	}

	assert.Nil(t, LanguageFor("notes.txt").Outline(), "모르는 확장자는 규칙이 없다")
	assert.NotNil(t, LanguageFor("Dockerfile.dev").Outline(), "이름 뒤에 붙는 것도 dockerfile 이다")
}

// 제목이 아래 글을 거느리고, 깊이는 `#` 개수다.
func TestOutlineMarkdownHeading(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		depth int
		heads bool
	}{
		{"h1", "# 제목", 0, true},
		{"h2", "## 제목", 1, true},
		{"h6", "###### 제목", 5, true},
		{"본문은 어느 제목보다 깊다", "그냥 글", OutlineDeep, false},
		{"목록도 본문이다", "- 항목", OutlineDeep, false},
		{"빈 줄", "", OutlineDeep, false},
		{"`#` 뒤에 빈 칸이 없으면 제목이 아니다", "#hashtag", OutlineDeep, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			depth, heads := outlineOf(t, "doc.md", test.line)
			assert.Equal(t, test.depth, depth)
			assert.Equal(t, test.heads, heads)
		})
	}
}

// **코드펜스 안의 `#` 은 제목이 아니다.**
//
// 원문을 `#` 로 훑으면 python 주석이 제목이 되어, 코드 블록을 지날 때마다 엉뚱한 줄이
// 화면 위에 붙는다. 토큰을 보는 것이 그것을 막는다(ADR-0040).
func TestOutlineMarkdownIgnoresFence(t *testing.T) {
	rule := LanguageFor("doc.md").Outline()

	state := LanguageFor("doc.md").State()
	_, state = state.Lex([]byte("```py"))

	line := []byte("# 주석이지 제목이 아니다")
	tokens, _ := state.Lex(line)

	assert.False(t, rule.Heads(line, tokens), "펜스 안에서는 머리줄이 아니다")
}

// 블록을 여는 줄이 머리줄이고, 깊이는 줄 앞 공백이다.
func TestOutlineIndentLanguages(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		line  string
		depth int
		heads bool
	}{
		{"go 함수", "main.go", "func main() {", 0, true},
		{"go 안쪽 블록", "main.go", "\tif x {", 4, true},
		{"go 본문", "main.go", "\tx := 1", 4, false},
		{"go 닫는 괄호", "main.go", "}", 0, false},
		{"go struct", "main.go", "type Buffer struct {", 0, true},
		{"문자열 안의 괄호는 안 센다", "main.go", `x := "{"`, 0, false},
		{"python def", "a.py", "def f():", 0, true},
		{"python 안쪽", "a.py", "    if x:", 4, true},
		{"shell 함수", "a.sh", "if x; then", 0, true},
		{"css 규칙", "a.css", "body {", 0, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			depth, heads := outlineOf(t, test.path, test.line)
			assert.Equal(t, test.depth, depth)
			assert.Equal(t, test.heads, heads)
		})
	}
}

// makefile 의 대상 줄이 조리법을 거느린다.
//
// makeIndent.Next 는 대상 줄에 단계가 아니라 tab prefix 를 내보내서 단계가 0 이다.
// indentOutline 을 줬다면 makefile 에 머리줄이 하나도 없었을 자리다.
func TestOutlineMakefileTarget(t *testing.T) {
	depth, heads := outlineOf(t, "Makefile", "build: deps")
	assert.Equal(t, 0, depth)
	assert.True(t, heads, "대상 줄이 머리줄이다")

	depth, heads = outlineOf(t, "Makefile", "\tgo build ./...")
	assert.Equal(t, 4, depth, "조리법은 대상보다 깊다")
	assert.False(t, heads)

	_, heads = outlineOf(t, "Makefile", "VAR := x")
	assert.False(t, heads, "변수 대입은 조리법을 갖지 않는다")
}

// dockerfile 에는 머리줄이 없다.
//
// dockerIndent.Next 는 `\` 로 이어지는 줄에 단계 1 을 준다. indentOutline 을 줬다면
// 이어지는 줄마다 머리줄이 되어 화면 위가 `RUN` 으로 뒤덮였을 자리다.
func TestOutlineDockerfileHasNoHead(t *testing.T) {
	_, heads := outlineOf(t, "Dockerfile", "RUN apt-get update \\")
	assert.False(t, heads, "줄 이어짐은 블록이 아니다")

	_, heads = outlineOf(t, "Dockerfile", "FROM golang:1.22")
	assert.False(t, heads)
}

// **깊이는 byte 가 아니라 칸으로 센다.**
//
// byte 로 세면 tab 하나(1) 가 space 네 칸(4) 보다 얕아 보여서, 섞어 쓴 파일에서 안쪽 줄이
// 바깥 줄보다 얕다고 나온다.
func TestOutlineDepthCountsColumns(t *testing.T) {
	tab, _ := outlineOf(t, "main.go", "\tx := 1")
	space, _ := outlineOf(t, "main.go", "    x := 1")

	assert.Equal(t, space, tab, "tab 하나와 space 네 칸이 같은 깊이다")
}

// **깊이를 말할 수 없는 줄이 있다.**
//
// 빈 줄과 여러 줄에 걸친 문자열·주석 안의 줄이다. 그런 줄은 코드가 아니라 글이라 왼쪽 끝에서
// 시작해도 「가장 바깥」이 아니다. 이것이 없으면 raw string 안에 왼쪽 끝으로 붙여 쓴 SQL 한
// 줄이 그 위의 함수를 통째로 가린다(ADR-0049).
func TestOutlineDepthIsDeepForText(t *testing.T) {
	rule := LanguageFor("main.go").Outline()

	state := LanguageFor("main.go").State()
	_, state = state.Lex([]byte("const q = `"))

	line := []byte("SELECT *")
	tokens, _ := state.Lex(line)
	assert.Equal(t, OutlineDeep, rule.Depth(line, tokens),
		"raw string 안은 깊이를 말하지 않는다")

	depth, _ := outlineOf(t, "main.go", "")
	assert.Equal(t, OutlineDeep, depth, "빈 줄은 깊이를 말하지 않는다")

	depth, _ = outlineOf(t, "main.go", "// 주석")
	assert.Equal(t, OutlineDeep, depth, "주석 줄은 깊이를 말하지 않는다")
}

// indent 칸과 outline 칸이 같은 규칙을 봐야 한다. 갈리면 한쪽만 고쳐지는 날이 온다.
func TestOutlineAgreesWithIndentColumn(t *testing.T) {
	for _, rule := range languageRules {
		block, ok := rule.outline.(blockOutline)
		if !ok {
			continue
		}

		assert.Equal(t, rule.indent, block.opens, "%v 의 두 칸이 갈렸다", rule.aliases)
	}
}
