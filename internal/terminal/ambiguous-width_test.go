package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// columnOf 는 커서 위치 답에서 열만 뽑는다. 앞에 사용자가 미리 쳐둔 키가 섞일 수 있다.
func TestColumnOf(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		want   int
	}{
		{name: "한 칸", answer: "\x1b[12;2R", want: 2},
		{name: "두 칸", answer: "\x1b[12;3R", want: 3},
		{name: "DECXCPR 형태", answer: "\x1b[?12;3R", want: 3},
		{name: "앞에 친 키가 섞임", answer: "jj\x1b[7;2R", want: 2},
		{name: "답이 없음", answer: "", want: 0},
		{name: "끝나지 않음", answer: "\x1b[12;2", want: 0},
		{name: "열이 없음", answer: "\x1b[12R", want: 0},
		{name: "숫자가 아님", answer: "\x1b[12;xR", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, columnOf([]byte(test.answer)))
		})
	}
}

// tty 가 아니면 재지 않고 곧바로 0 이다. 파이프로 돌리는 테스트·CI 가 그렇다.
func TestProbeWithoutTerminal(t *testing.T) {
	assert.Equal(t, 0, ProbeAmbiguousWidth())
}
