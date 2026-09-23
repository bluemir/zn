package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGomodLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "module 줄",
			line: "module github.com/bluemir/zn",
			want: []string{"keyword:module", "key:github.com/bluemir/zn"},
		},
		{
			name: "go 줄의 뒷자리는 숫자다",
			line: "go 1.24",
			want: []string{"keyword:go", "number:1.24"},
		},
		{
			name: "toolchain 은 go 가 붙은 버전이다",
			line: "toolchain go1.24.0",
			want: []string{"keyword:toolchain", "number:go1.24.0"},
		},
		{
			name: "한 줄짜리 require",
			line: "require github.com/charmbracelet/bubbletea v1.3.4",
			want: []string{"keyword:require", "key:github.com/charmbracelet/bubbletea", "number:v1.3.4"},
		},
		{
			name: "덩이를 여는 줄",
			line: "require (",
			want: []string{"keyword:require"},
		},
		{
			name: "replace 의 화살표는 지시자와 같은 갈래다",
			line: "replace example.com/x => ../x",
			want: []string{"keyword:replace", "key:example.com/x", "keyword:=>", "key:../x"},
		},
		{
			name: "retract 의 범위",
			line: "retract [v1.0.0, v1.1.0]",
			want: []string{"keyword:retract", "number:v1.0.0", "number:v1.1.0"},
		},
		{
			name: "godebug 는 이름과 값으로 갈린다",
			line: "godebug default=go1.21",
			want: []string{"keyword:godebug", "key:default", "number:go1.21"},
		},
		{
			name: "go.work 의 use",
			line: "use ./sub",
			want: []string{"keyword:use", "key:./sub"},
		},
		{
			name: "주석",
			line: "require example.com/x v1.0.0 // indirect",
			want: []string{"keyword:require", "key:example.com/x", "number:v1.0.0", "comment:// indirect"},
		},
		{
			name: "줄 전체가 주석",
			line: "// 한글 주석이다",
			want: []string{"comment:// 한글 주석이다"},
		},
		{
			name: "지시자가 아닌 첫 낱말",
			line: "example.com/x v1.0.0",
			want: []string{"key:example.com/x", "number:v1.0.0"},
		},
		{
			name: "숫자로 시작하는 모듈 경로는 버전이 아니다",
			line: "require 4d63.com/gochecknoglobals v0.2.1",
			want: []string{"keyword:require", "key:4d63.com/gochecknoglobals", "number:v0.2.1"},
		},
		{
			name: "go 로 시작하는 모듈 경로도 버전이 아니다",
			line: "require go.uber.org/zap v1.27.0",
			want: []string{"keyword:require", "key:go.uber.org/zap", "number:v1.27.0"},
		},
		{
			name: "빈 줄",
			line: "",
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, gomodNormal{}, test.line)
			assert.Equal(t, test.want, got)
		})
	}
}

// 덩이 안에서는 줄 앞의 낱말이 지시자가 아니다. `tool (` 안의 `example.com/go` 가 그 자리다.
func TestGomodBlockHasNoDirective(t *testing.T) {
	got, state := lexedAll(t, gomodNormal{},
		"tool (",
		"\texample.com/go",
		")",
		"go 1.24",
	)

	assert.Equal(t, [][]string{
		{"keyword:tool"},
		{"key:example.com/go"},
		{},
		{"keyword:go", "number:1.24"},
	}, got)
	assert.Equal(t, gomodNormal{}, state, "덩이를 닫으면 보통 문맥으로 돌아온다")
}

// 덩이를 여는 괄호 뒤에 주석이 와도 덩이가 열린다.
func TestGomodBlockOpensBehindComment(t *testing.T) {
	_, state := lexed(t, gomodNormal{}, "require ( // 여기부터다")

	assert.Equal(t, gomodBlock{}, state)
}

func TestDetectGomod(t *testing.T) {
	tests := []struct {
		name string
		path string
		want State
	}{
		{name: "go.mod", path: "go.mod", want: gomodNormal{}},
		{name: "경로가 붙은 go.mod", path: "internal/testdata/go.mod", want: gomodNormal{}},
		{name: "go.work", path: "go.work", want: gomodNormal{}},
		{name: "go.sum", path: "go.sum", want: gosumNormal{}},
		{name: "go.work.sum", path: "go.work.sum", want: gosumNormal{}},
		{name: "`.mod` 라고 go.mod 인 것은 아니다", path: "sound.mod", want: nil},
		{name: "`.sum` 도 마찬가지다", path: "checks.sum", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, LanguageFor(test.path).State())
		})
	}
}
