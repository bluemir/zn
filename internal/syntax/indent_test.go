package syntax

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nextOf 는 그 언어의 lexer 로 줄을 훑은 뒤 규칙에 물어본다. 문자열·주석 안의 괄호를 거르는
// 것이 토큰에 매여 있어서, 토큰을 손으로 적으면 그 거르기가 시험되지 않는다.
func nextOf(t *testing.T, path, line string) (int, string) {
	t.Helper()

	state := LanguageFor(path).State()
	require.NotNil(t, state, "언어를 알아보지 못했다: %s", path)

	rule := LanguageFor(path).Indent()
	require.NotNil(t, rule, "들여쓰기 규칙이 없다: %s", path)

	tokens, _ := state.Lex([]byte(line))
	level, prefix := rule.Next([]byte(line), tokens)

	return level, string(prefix)
}

func TestIndentForEveryLanguage(t *testing.T) {
	// 언어를 더하면서 칸을 빠뜨리면 그 언어만 조용히 앞 줄 잇기로 내려간다.
	for _, rule := range languageRules {
		assert.NotNil(t, rule.indent, "%v 에 들여쓰기 규칙이 없다", rule.aliases)
		assert.NotEmpty(t, rule.indent.Unit(), "%v 의 관례 단계가 비었다", rule.aliases)
	}

	assert.Nil(t, LanguageFor("notes.txt").Indent(), "모르는 확장자는 규칙이 없다")
	assert.NotNil(t, LanguageFor("Dockerfile.dev").Indent(), "이름 뒤에 붙는 것도 dockerfile 이다")
}

// `=` 로 다시 들여쓸 수 없는 언어는 markdown 과 yaml 이다.
//
// 둘 다 **들여쓰기가 곧 뜻**이라 앞 줄에서 되짚을 수 없다. markdown 은 목록의 깊이와 네 칸
// 코드 블록이 글쓴이가 정한 것이고, yaml 은 들여쓰기가 map 의 층이라 되짚으면 중첩이 평평해진다.
func TestReindentsIsFalseForIndentIsMeaning(t *testing.T) {
	cannot := []Indent{mdIndent{}, yamlIndent{}}

	for _, rule := range languageRules {
		want := !slices.Contains(cannot, rule.indent)
		assert.Equal(t, want, rule.indent.Reindents(), "%v", rule.aliases)
	}
}

// Close 는 줄 앞부분만 본다 — 끝을 알릴 것이 없다. 부르는 자리가 둘이라 그렇다: 치는 도중에는
// 커서까지가, 다시 들여쓸 때는 줄 전체가 들어온다.
func TestCloseLooksOnlyAtTheStartOfTheLine(t *testing.T) {
	tests := []struct {
		name string
		path string
		head string
		want int
	}{
		{name: "닫는 괄호 하나", path: "a.go", head: "}", want: 1},
		{name: "뒤에 무엇이 붙어도 같다", path: "a.go", head: "} else {", want: 1},
		{name: "줄끝이 붙어도 같다", path: "a.go", head: "}\n", want: 1},
		{name: "앞에 글자가 있으면 아니다", path: "a.go", head: "x}", want: 0},
		{name: "case 는 줄 전체로도 걸린다", path: "a.go", head: "case 1:", want: 1},
		{name: "shell 의 fi 는 줄 전체로", path: "a.sh", head: "fi\n", want: 1},
		{name: "shell 의 fi 뒤에 주석", path: "a.sh", head: "fi # 끝", want: 1},
		{name: "html 은 닫는 tag 뒤에 글이 있어도", path: "a.html", head: "</div> 글", want: 1},
		{name: "markdown 은 닫는 것이 없다", path: "a.md", head: "}", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := LanguageFor(test.path).Indent()
			require.NotNil(t, rule)
			assert.Equal(t, test.want, rule.Close([]byte(test.head)))
		})
	}
}

func TestBraceIndentNext(t *testing.T) {
	tests := []struct {
		name string
		path string
		line string
		want int
	}{
		{name: "여는 중괄호", path: "a.go", line: "func f() {", want: 1},
		{name: "닫는 중괄호로 끝난 줄", path: "a.go", line: "}", want: 0},
		{name: "한 줄에서 여닫으면 그대로", path: "a.go", line: "func f() { return }", want: 0},
		{name: "두 겹을 열어도 한 단계", path: "a.go", line: "foo(bar{", want: 1},
		{name: "문자열 안의 괄호는 세지 않는다", path: "a.go", line: `s := "{"`, want: 0},
		{name: "주석 안의 괄호도 세지 않는다", path: "a.go", line: "x := 1 // {", want: 0},
		{name: "raw string 안의 괄호", path: "a.go", line: "s := `{`", want: 0},
		{name: "case 아래는 들어간다", path: "a.go", line: "case 1:", want: 1},
		{name: "default 아래도 들어간다", path: "a.go", line: "default:", want: 1},
		{name: "label 은 case 가 아니다", path: "a.go", line: "loop:", want: 0},
		{name: "여는 소괄호", path: "a.js", line: "foo(", want: 1},
		{name: "css 의 선택자", path: "a.css", line: "body {", want: 1},
		{name: "css 주석 안의 괄호", path: "a.css", line: "/* { */", want: 0},
		{name: "그냥 줄", path: "a.go", line: "x := 1", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, test.path, test.line)
			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}
}

func TestBraceIndentClose(t *testing.T) {
	rule := braceIndent{unit: "\t"}

	tests := []struct {
		name string
		head string
		want int
	}{
		{name: "닫는 중괄호", head: "}", want: 1},
		{name: "닫는 소괄호", head: ")", want: 1},
		{name: "닫는 대괄호", head: "]", want: 1},
		{name: "여는 괄호는 아니다", head: "{", want: 0},
		{name: "글자를 친 뒤의 괄호는 아니다", head: "x}", want: 0},
		{name: "case 에 공백이 붙으면", head: "case ", want: 1},
		{name: "default 에 콜론이 붙으면", head: "default:", want: 1},
		{name: "case 만으로는 아직 아니다", head: "case", want: 0},
		{name: "낱말이 이어지면 아니다", head: "cases ", want: 0},
		{name: "case 로 시작하는 다른 이름", head: "casefold(", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, rule.Close([]byte(test.head)))
		})
	}
}

func TestPythonIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{name: "def 아래", line: "def f():", want: 1},
		{name: "if 아래", line: "if x:", want: 1},
		{name: "여는 괄호", line: "foo(", want: 1},
		{name: "주석 뒤의 콜론", line: "if x:  # 여기", want: 1},
		{name: "문자열 안의 콜론", line: `s = "a:"`, want: 0},
		{name: "dict 는 닫혀 있다", line: "d = {1: 2}", want: 0},
		{name: "slice 는 닫혀 있다", line: "a = b[1:]", want: 0},
		{name: "return 뒤는 나온다", line: "    return x", want: -1},
		{name: "pass 뒤도 나온다", line: "    pass", want: -1},
		{name: "그냥 줄", line: "x = 1", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "a.py", test.line)
			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}

	rule := pyIndent{}
	assert.Equal(t, 1, rule.Close([]byte("else:")), "else 는 한 단계 나온다")
	assert.Equal(t, 1, rule.Close([]byte("except ")), "except 도 그렇다")
	assert.Equal(t, 0, rule.Close([]byte("elsewhere ")), "낱말이 이어지면 아니다")
	assert.Equal(t, 0, rule.Close([]byte("else")), "마침 글자가 있어야 한다")
}

func TestShellIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{name: "then 아래", line: "if [ -f x ]; then", want: 1},
		{name: "do 아래", line: "for i in 1 2 3; do", want: 1},
		{name: "else 아래", line: "else", want: 1},
		{name: "여는 중괄호", line: "f() {", want: 1},
		{name: "then 으로 끝나는 이름은 아니다", line: "echo authen", want: 0},
		{name: "주석 안의 then", line: "echo x # then", want: 0},
		{name: "문자열 안의 then", line: `echo "then"`, want: 0},
		{name: "fi 는 열지 않는다", line: "fi", want: 0},
		{name: "case 갈래", line: "a)", want: 1},
		{name: "패턴이 여럿인 갈래", line: "b|c)", want: 1},
		{name: "기본 갈래", line: "*)", want: 1},
		{name: "함수 이름 뒤의 짝맞는 괄호", line: "f()", want: 0},
		{name: "명령 치환", line: "x=$(date)", want: 0},
		{name: "문자열로 끝나는 명령 치환", line: `echo "$(basename "$0")"`, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "a.sh", test.line)
			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}

	rule := shIndent{}
	assert.Equal(t, 0, rule.Close([]byte("a)\n")), "갈래는 나오지 않는다")
	assert.Equal(t, 1, rule.Close([]byte("fi\n")), "Enter 가 낱말을 끝낸다")
	assert.Equal(t, 1, rule.Close([]byte("done ")), "공백도 낱말을 끝낸다")
	assert.Equal(t, 1, rule.Close([]byte(";;")), "case 갈래의 끝")
	assert.Equal(t, 0, rule.Close([]byte("fi")), "아직 file 이 될 수 있다")
	assert.Equal(t, 0, rule.Close([]byte("file")), "이어 치면 걸리지 않는다")
}

// case 문은 갈래마다 한 단계 들어가고 `;;` 가 그것을 닫는다.
//
// **줄 하나로는 재지 못하는 짝이다.** `pattern)` 이 열지 않던 때는 `;;` 만 닫아서 갈래마다
// 한 단계씩 빠졌고, 갈래가 셋인 문에서 `esac` 이 시작한 자리보다 세 단계 밖으로 나갔다.
func TestShellCaseKeepsItsLevel(t *testing.T) {
	lines := []string{
		`case "$x" in`,
		`a)`,
		`echo a`,
		`;;`,
		`b|c)`,
		`echo b`,
		`;;`,
		`*)`,
		`echo other`,
		`;;`,
		`esac`,
	}
	want := []int{0, 1, 2, 1, 1, 2, 1, 1, 2, 1, 0}

	rule := shIndent{}
	level := 0

	for at, line := range lines {
		level -= rule.Close([]byte(line + "\n"))
		assert.Equal(t, want[at], level, "%d 번째 줄 %q", at, line)

		next, _ := rule.Next([]byte(line), nil)
		level += next
	}
}

func TestMarkdownListContinues(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "빗금 목록", line: "- 첫째", want: "- "},
		{name: "별표 목록", line: "* 첫째", want: "* "},
		{name: "더하기 목록", line: "+ 첫째", want: "+ "},
		{name: "인용문", line: "> 남의 말", want: "> "},
		{name: "번호가 오른다", line: "1. 첫째", want: "2. "},
		{name: "두 자리 번호", line: "9. 아홉째", want: "10. "},
		{name: "괄호를 쓴 번호", line: "1) 첫째", want: "2) "},
		{name: "깊은 목록도 표시는 같다", line: "\t- 깊은 것", want: "- "},
		{name: "빈 항목이면 목록이 끝난다", line: "- ", want: ""},
		{name: "빈 번호 항목도 끝난다", line: "1. ", want: ""},
		{name: "표시 뒤에 공백이 없으면 글이다", line: "-단어", want: ""},
		{name: "제목은 목록이 아니다", line: "# 제목", want: ""},
		{name: "그냥 글", line: "보통 글", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "a.md", test.line)
			assert.Equal(t, 0, level, "markdown 은 단계를 내지 않는다")
			assert.Equal(t, test.want, prefix)
		})
	}
}

func TestHTMLIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{name: "여는 tag", line: "<div>", want: 1},
		{name: "속성이 붙은 tag", line: `<div class="a">`, want: 1},
		{name: "한 줄에서 여닫으면 그대로", line: "<p>글</p>", want: 0},
		{name: "닫는 tag 만", line: "</div>", want: 0},
		{name: "void tag 는 안쪽이 없다", line: "<br>", want: 0},
		{name: "대문자 void tag", line: "<IMG src=x>", want: 0},
		{name: "자기를 닫는 tag", line: "<div/>", want: 0},
		{name: "주석", line: "<!-- 여기 -->", want: 0},
		{name: "선언", line: "<!DOCTYPE html>", want: 0},
		{name: "글만 있는 줄", line: "그냥 글", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "a.html", test.line)
			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}

	rule := htmlIndent{}
	assert.Equal(t, 1, rule.Close([]byte("</div>")), "닫는 tag 를 다 치면 당긴다")
	assert.Equal(t, 0, rule.Close([]byte("</div")), "아직 `>` 를 안 쳤다")
}

func TestMakefileIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "대상 줄 다음은 조리법이다", line: "build:", want: "\t"},
		{name: "의존이 붙은 대상도 조리법을 받는다", line: "build: deps", want: "\t"},
		{name: "설명 주석이 붙은 대상", line: "test: fmt vet ## Run test", want: "\t"},
		{name: "변수 대입", line: "VERSION := 1", want: ""},
		{name: "특수 대상은 조리법이 없다", line: ".PHONY: build", want: ""},
		{name: "콜론이 없는 줄", line: "include x.mk", want: ""},
		{name: "변수 대입은 대상이 아니다", line: "VERSION ::=", want: ""},
		{name: "조리법 줄", line: "\techo hi", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "Makefile", test.line)
			assert.Equal(t, 0, level, "makefile 은 단계를 내지 않는다")
			assert.Equal(t, test.want, prefix)
		})
	}

	assert.Equal(t, "\t", string(makeIndent{}.Unit()), "make 의 한 단계는 언제나 tab 이다")
}

func TestDockerfileIndent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{name: "이어짐이 시작한다", line: "RUN apt-get update \\", want: 1},
		{name: "이어짐이 이어진다", line: "    && apt-get install x \\", want: 0},
		{name: "이어짐이 끝난다", line: "    && rm -rf /var/lib", want: -1},
		{name: "그냥 명령", line: "FROM golang:1.26", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, prefix := nextOf(t, "Dockerfile", test.line)
			assert.Equal(t, test.want, level)
			assert.Empty(t, prefix)
		})
	}
}

func TestCodeBytesDoesNotTouchTheLine(t *testing.T) {
	// 받는 줄은 Buffer 가 든 read-only data 의 subslice 다(ADR-0001).
	line := []byte(`s := "{"`)
	tokens, _ := goNormal{}.Lex(line)

	code := codeBytes(line, tokens)

	assert.Equal(t, `s := "{"`, string(line), "원래 줄이 그대로여야 한다")
	assert.Equal(t, "s :=    ", string(code), "문자열 자리가 공백이다")
}
