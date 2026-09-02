package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 본문 앞 마커 칸에 무엇이 서는지 보는 시험이다(render-marker.go).

// 마커 칸은 진단 칸 다음, 줄번호 앞이다.
func TestGitMarkerInGutter(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 40, 2)
	m.buffers[0].git.base = toLines("one\nTWO\n")
	m.buffers[0].refreshGitLines()

	assert.Equal(t, []string{"    1  0 ", " ~  2  1 "}, gutterOf(t, m))
}
