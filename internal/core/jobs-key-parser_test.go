package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// pressAllJobs 는 이미 풀린 키를 차례로 먹이고 마지막에 완성된 이름과 남은 상태를 돌려준다.
// normal·트리 쪽 pressAll 과 같은 모양이다.
func pressAllJobs(keys ...string) (string, jobsState) {
	var state jobsState = jobsStart{}

	name := ""
	for _, k := range keys {
		var names []string
		names, state = state.press(k)

		// 그 키가 명령을 완성하지 못했으면 빈 것으로 되돌린다.
		name = ""
		if len(names) > 0 {
			name = names[len(names)-1]
		}
	}

	return name, state
}

// 지금은 기다리는 상태가 없어서 모든 키가 곧 이름이다.
// 상태를 더할 때 여기가 그것을 적는 자리가 된다(jobs-key-parser.go).
func TestJobsKeyParser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "키 하나는 그대로 이름이다", keys: []string{"j"}, want: "j"},
		{name: "화살표도 같다", keys: []string{"down"}, want: "down"},
		{name: "취소 키", keys: []string{"x"}, want: "x"},
		{name: "닫기 키", keys: []string{"q"}, want: "q"},

		// normalStart 와 달리 `d`·`y`·`c` 는 operator 가 아니고 숫자는 count 가 아니다.
		// 목록에서 이것들이 다음 키를 삼키면 `3x` 가 취소를 놓친다(ADR-0006).
		{name: "d 는 operator 가 아니다", keys: []string{"d", "x"}, want: "x"},
		{name: "숫자는 count 가 아니다", keys: []string{"3", "x"}, want: "x"},
		{name: "g 는 접두 키가 아니다", keys: []string{"g", "j"}, want: "j"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, _ := pressAllJobs(test.keys...)

			assert.Equal(t, test.want, name)
		})
	}
}

// 기다리는 상태가 없으므로 보여줄 키도 없다. 상태가 생기면 이 값이 달라진다.
func TestJobsShowcmd(t *testing.T) {
	_, state := pressAllJobs("x")

	assert.Equal(t, "", state.showcmd())
}
