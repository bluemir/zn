package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 다시 시작하는 것은 두 칸으로 확인됐고 env 가 아직 없을 때뿐이다(ADR-0072).
func TestNeedsRestart(t *testing.T) {
	tests := []struct {
		name  string
		width int
		env   string
		want  bool
	}{
		{name: "두 칸이고 env 가 없다", width: 2, want: true},
		{name: "한 칸이면 눈금이 이미 맞다", width: 1},
		{name: "재지 못했으면 아무것도 모른다", width: 0},
		{name: "env 가 켜져 있으면 이미 다시 시작한 뒤다", width: 2, env: "1"},
		{name: "env 를 끈 사람의 뜻을 뒤집지 않는다", width: 2, env: "0"},
		{name: "알아볼 수 없는 값도 사용자가 세운 것이다", width: 2, env: "아무거나"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.env != "" {
				t.Setenv(eastAsianEnv, test.env)
			}

			assert.Equal(t, test.want, needsRestart(test.width))
		})
	}
}
