package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGosumLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "모듈 줄",
			line: "github.com/charmbracelet/bubbletea v1.3.4 h1:kCg7B+jSCFPLYRA52SDZjr51kG/fMUQoRoP0vNjbe9g=",
			want: []string{"key:github.com/charmbracelet/bubbletea", "number:v1.3.4"},
		},
		{
			name: "go.mod 만 있는 줄은 그 접미까지가 버전이다",
			line: "github.com/charmbracelet/bubbletea v1.3.4/go.mod h1:Vrl5uPQ8mCptEWUVjULjZjhjT0/PXPMlcC8bMs1PwOk=",
			want: []string{"key:github.com/charmbracelet/bubbletea", "number:v1.3.4/go.mod"},
		},
		{
			name: "해시가 없어도 앞의 둘은 나온다",
			line: "example.com/x v1.0.0",
			want: []string{"key:example.com/x", "number:v1.0.0"},
		},
		{
			name: "낱말이 하나뿐",
			line: "example.com/x",
			want: []string{"key:example.com/x"},
		},
		{
			name: "빈 줄",
			line: "",
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, state := lexed(t, gosumNormal{}, test.line)
			assert.Equal(t, test.want, got)
			assert.Equal(t, gosumNormal{}, state, "이어지는 것이 없어 문맥이 그대로다")
		})
	}
}
