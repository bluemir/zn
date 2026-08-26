package syntax

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		path string
		want State
	}{
		{name: "Go 소스", path: "internal/core/buffer.go", want: goNormal{}},
		{name: "경로 없이 이름만", path: "main.go", want: goNormal{}},
		{name: "확장자가 대문자", path: "MAIN.GO", want: goNormal{}},
		{name: "모르는 확장자", path: "notes.txt", want: nil},
		{name: "확장자 없는 이름", path: "LICENSE", want: nil},
		{name: "이름 없는 buffer", path: "", want: nil},
		{name: "markdown", path: "docs/tasks.md", want: mdNormal{}},
		{name: "확장자를 다 적은 markdown", path: "notes.markdown", want: mdNormal{}},
		{name: "이름에 go 가 들었을 뿐", path: "cmd/gopher/notes.txt", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, LanguageFor(test.path).State())
		})
	}
}

// 표에 든 언어는 모두 시작 문맥을 준다. 한 줄을 더하면서 state 를 빠뜨리면 여기서 걸린다.
func TestLanguageRulesHaveState(t *testing.T) {
	for _, rule := range languageRules {
		assert.NotNil(t, rule.state, "%v %v 에 시작 문맥이 없습니다", rule.exts, rule.names)
		assert.NotEmpty(t, append(append([]string{}, rule.exts...), rule.names...),
			"%T 를 고를 길이 없습니다", rule.state)
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name string
		path string
		want State
	}{
		{name: "sh", path: "scripts/build.sh", want: shNormal{}},
		{name: "bash", path: "x.bash", want: shNormal{}},
		{name: "zsh", path: "x.zsh", want: shNormal{}},
		{name: "확장자 없는 script 는 알아보지 못한다", path: "scripts/build", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, LanguageFor(test.path).State())
		})
	}
}

// languageByName 은 경로가 아니라 언어 이름을 본다. 코드펜스의 info string 이 그것이다.
func TestLanguageByName(t *testing.T) {
	tests := []struct {
		name string
		want State
	}{
		{name: "go", want: goNormal{}},
		{name: "golang", want: goNormal{}},
		{name: "GO", want: goNormal{}},
		{name: "js", want: jsNormal{}},
		{name: "javascript", want: jsNormal{}},
		{name: "py", want: pyNormal{}},
		{name: "python", want: pyNormal{}},
		{name: "sh", want: shNormal{}},
		{name: "bash", want: shNormal{}},
		{name: "css", want: cssNormal{}},
		{name: "html", want: htmlNormal{}},
		{name: "md", want: mdNormal{}},
		{name: "markdown", want: mdNormal{}},
		{name: "make", want: makeNormal{}},
		{name: "dockerfile", want: dockerNormal{}},
		{name: "brainfuck", want: nil},
		{name: "", want: nil},
		{name: ".go", want: nil},
		{name: "gnumakefile", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, languageByName(test.name))
		})
	}
}

// 언어마다 이름이 하나는 있어야 코드펜스에서 고를 수 있다. 이름이 둘 이상의 언어에 붙으면
// 표에서 앞선 것이 조용히 이긴다.
func TestLanguageRulesHaveDistinctAliases(t *testing.T) {
	seen := map[string]bool{}

	for _, rule := range languageRules {
		assert.NotEmpty(t, rule.aliases, "%T 를 코드펜스에서 고를 이름이 없습니다", rule.state)

		for _, alias := range rule.aliases {
			assert.Equal(t, strings.ToLower(alias), alias, "이름은 소문자로 적는다: %q", alias)
			assert.False(t, seen[alias], "이름 %q 가 두 언어에 붙었습니다", alias)
			seen[alias] = true
		}
	}
}
