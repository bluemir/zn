package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 표본은 이 저장소의 Makefile 에서 그대로 가져왔다.
func TestMakefileLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "변수 붙이기와 값 참조",
			line: "VERSION?=$(shell git describe --tags --dirty --always)",
			want: []string{"var:VERSION", "var:$(shell git describe --tags --dirty --always)"},
		},
		{
			name: "지시자가 변수 이름보다 먼저다",
			line: "export PATH:=./runtime/tools:$(PATH)",
			want: []string{"keyword:export", "var:$(PATH)"},
		},
		{
			name: "대상은 부르는 이름이다",
			line: "build: dep1 dep2",
			want: []string{"func:build"},
		},
		{
			name: "특수 대상",
			line: ".PHONY: default",
			want: []string{"func:.PHONY"},
		},
		{
			name: "패턴 대상",
			line: "%/.placeholder:",
			want: []string{"func:%/.placeholder"},
		},
		{
			name: "대상 뒤의 도움 문구는 주석이다",
			line: "clean: ## Clean up",
			want: []string{"func:clean", "comment:## Clean up"},
		},
		{
			name: "조리법 줄의 앞은 shell 이라 아무것도 아니다",
			line: "\t@go run -C scripts/tools/make-select .",
			want: []string{},
		},
		{
			name: "조리법 안의 값 참조만 집는다",
			line: "\trm -rf build/ $(OPTIONAL_CLEAN)",
			want: []string{"var:$(OPTIONAL_CLEAN)"},
		},
		{
			name: "조건 지시자",
			line: "ifneq ($(shell printf '%s'),$(MAKE_VERSION))",
			want: []string{"keyword:ifneq", "var:$(shell printf '%s')", "var:$(MAKE_VERSION)"},
		},
		{
			name: "include",
			line: "include scripts/makefile.d/*.mk",
			want: []string{"keyword:include"},
		},
		{
			name: "make 만 쓰는 한 글자 이름",
			line: "\ttouch $@ $< $^",
			want: []string{"var:$@", "var:$<", "var:$^"},
		},
		{
			name: "`$$` 는 값 참조가 아니라 `$` 한 글자다",
			line: "\tawk '{print $$2}'",
			want: []string{},
		},
		{
			name: "중괄호 참조",
			line: "\techo ${HOME}",
			want: []string{"var:${HOME}"},
		},
		{
			name: "줄 전체가 주석",
			line: "# go build args",
			want: []string{"comment:# go build args"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, makeNormal{}, test.line)

			assert.Equal(t, test.want, got)
		})
	}
}

// `\` 로 이어진 줄은 앞이 대상도 변수도 아니다.
func TestMakefileContinuation(t *testing.T) {
	got, next := lexedAll(t, makeNormal{},
		"build/$(APP_NAME): $(GO_SOURCES) \\",
		"\t\t-X '$(IMPORT_PATH)/internal/buildinfo.Version=$(VERSION)' \\",
		"\t\t-o $@ .",
	)

	assert.Equal(t, [][]string{
		{"func:build/$(APP_NAME)", "var:$(GO_SOURCES)"},
		{"var:$(IMPORT_PATH)", "var:$(VERSION)"},
		{"var:$@"},
	}, got)
	assert.Equal(t, makeNormal{}, next, "이어지지 않으면 보통 문맥으로 돌아온다")
}
