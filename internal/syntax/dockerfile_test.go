package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerfileLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "명령과 단계 이름",
			line: "FROM golang:1.26 AS build",
			want: []string{"keyword:FROM", "keyword:AS"},
		},
		{
			name: "명령 이름은 대소문자를 가리지 않는다",
			line: "from golang:1.26 as build",
			want: []string{"keyword:from", "keyword:as"},
		},
		{
			name: "값 참조",
			line: "ENV PATH=${GOPATH}/bin",
			want: []string{"keyword:ENV", "var:${GOPATH}"},
		},
		{
			name: "괄호 없는 값 참조",
			line: "RUN echo $HOME",
			want: []string{"keyword:RUN", "var:$HOME"},
		},
		{
			name: "flag 이름",
			line: "COPY --from=build /a /b",
			want: []string{"keyword:COPY", "var:--from"},
		},
		{
			name: "JSON 꼴 CMD 는 따옴표가 문자열이다",
			line: `CMD ["go", "run", "."]`,
			want: []string{"keyword:CMD", `string:"go"`, `string:"run"`, `string:"."`},
		},
		{
			name: "줄 전체가 주석",
			line: "# 이미지를 만든다",
			want: []string{"comment:# 이미지를 만든다"},
		},
		{
			name: "`# syntax=` 지시자도 그냥 주석이다",
			line: "# syntax=docker/dockerfile:1",
			want: []string{"comment:# syntax=docker/dockerfile:1"},
		},
		{
			name: "명령이 아닌 낱말은 색이 없다",
			line: "NOTACOMMAND foo",
			want: []string{},
		},
		{
			name: "낱말 가운데의 `--` 는 flag 가 아니다",
			line: "RUN a--b",
			want: []string{"keyword:RUN"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, dockerNormal{}, test.line)

			assert.Equal(t, test.want, got)
		})
	}
}

// `\` 로 이어진 줄은 앞에 명령 이름이 없다.
func TestDockerfileContinuation(t *testing.T) {
	got, next := lexedAll(t, dockerNormal{},
		"RUN apt-get update \\",
		" && apt-get install -y ${PKG} \\",
		" && rm -rf /var/lib/apt/lists/*",
	)

	assert.Equal(t, [][]string{
		{"keyword:RUN"},
		{"var:${PKG}"},
		{},
	}, got)
	assert.Equal(t, dockerNormal{}, next)
}

// 이어지는 줄에서 명령 이름처럼 보이는 낱말이 예약어가 되면 안 된다.
func TestDockerfileContinuationHasNoInstruction(t *testing.T) {
	got, _ := lexed(t, dockerContinued{}, " && RUN something")

	assert.Equal(t, []string{}, got, "이어지는 줄의 앞은 명령 자리가 아니다")
}
