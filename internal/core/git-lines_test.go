package core

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// toLines 는 시험에서 쓰는 줄 묶음이다.
func toLines(text string) [][]byte {
	out, _ := splitLines([]byte(text))

	return out
}

// 고친 줄에는 `~`, 새로 넣은 줄에는 `+` 다.
func TestGitLineMarksAddedAndModified(t *testing.T) {
	base := toLines("one\ntwo\nthree\n")
	now := toLines("one\nTWO\nnew\nthree\n")

	marks := gitLineMarks(base, now)

	assert.Equal(t, gitLineModified, marks[1], "고친 줄")
	assert.Equal(t, gitLineAdded, marks[2], "넣은 줄")
	assert.Equal(t, gitLineNone, marks[0], "그대로인 줄에는 없다")
	assert.Equal(t, gitLineNone, marks[3])
}

// 지운 줄은 화면에 없다. 표시는 그 앞 줄에 남는다.
func TestGitLineMarksRemoved(t *testing.T) {
	base := toLines("one\ntwo\nthree\nfour\n")
	now := toLines("one\nfour\n")

	marks := gitLineMarks(base, now)

	assert.Equal(t, gitLineRemoved, marks[0], "없어진 자리 앞 줄에 선다")
	assert.Equal(t, gitLineNone, marks[1])
}

// 파일 맨 앞이 지워지면 갈 곳이 없다. 첫 줄에 선다.
func TestGitLineMarksRemovedAtTop(t *testing.T) {
	base := toLines("one\ntwo\nthree\n")
	now := toLines("three\n")

	assert.Equal(t, gitLineRemoved, gitLineMarks(base, now)[0])
}

// 한 줄에 둘이 겹치면 그 줄 자신에 대한 사실이 이긴다.
func TestGitLineMarksKeepsOwnFactOverRemoved(t *testing.T) {
	base := toLines("one\ntwo\nthree\nfour\n")
	now := toLines("ONE\nfour\n")

	marks := gitLineMarks(base, now)

	assert.Equal(t, gitLineModified, marks[0], "고친 줄 표시가 지움 표시에 밀리지 않는다")
}

// 견줄 원본이 없으면 마커도 없다. 추적하지 않는 새 파일이 이것이다.
func TestGitLineMarksWithoutBase(t *testing.T) {
	assert.Nil(t, gitLineMarks(nil, toLines("one\ntwo\n")))
}

// 같은 내용이면 아무것도 없다.
func TestGitLineMarksUnchanged(t *testing.T) {
	assert.Empty(t, gitLineMarks(toLines("one\ntwo\n"), toLines("one\ntwo\n")))
}

// 통째로 지우면 첫 줄에만 표시가 선다.
func TestGitLineMarksEmptiedFile(t *testing.T) {
	marks := gitLineMarks(toLines("one\ntwo\n"), toLines(""))

	assert.Equal(t, gitLineRemoved, marks[0])
	assert.Len(t, marks, 1)
}

// 앞뒤 공통 줄을 떼는 덕에 큰 파일에서도 고친 크기만큼만 든다(ADR-0094 §5).
func TestGitLineMarksInBigFile(t *testing.T) {
	var builder strings.Builder
	for i := range 20000 {
		builder.WriteString("line " + strconv.Itoa(i) + "\n")
	}

	base := toLines(builder.String())
	now := append([][]byte(nil), base...)
	now[19000] = []byte("고친 줄")

	marks := gitLineMarks(base, now)

	assert.Equal(t, gitLineModified, marks[19000])
	assert.Len(t, marks, 1)
}

// 마커 칸은 진단 칸 다음, 줄번호 앞이다.
func TestGitMarkerInGutter(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 40, 2)
	m.buffers[0].git.base = toLines("one\nTWO\n")
	m.buffers[0].refreshGitLines()

	assert.Equal(t, []string{"    1  0 ", " ~  2  1 "}, gutterOf(t, m))
}
