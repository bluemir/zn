package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 오래된 것이 앞, 최근에 친 것이 뒤다.
func TestHistoryKeepsTypedOrder(t *testing.T) {
	var h history
	h.add("w")
	h.add("q")

	assert.Equal(t, []string{"w", "q"}, h.entries)
}

// 빈 줄은 담지 않는다. `:` 만 치고 Enter 를 누른 것이 이력에 남을 것은 없다.
func TestHistorySkipsEmpty(t *testing.T) {
	var h history
	h.add("")

	assert.Empty(t, h.entries)
}

// 같은 것을 다시 치면 앞의 것을 지우고 맨 뒤로 보낸다. `:w` 를 열 번 쳐도 이력에 하나다.
func TestHistoryMovesRepeatToTheBack(t *testing.T) {
	var h history
	h.add("w")
	h.add("e a.txt")
	h.add("w")

	assert.Equal(t, []string{"e a.txt", "w"}, h.entries, "연달아 친 것만 접는 것이 아니다")
}

// 50 개까지다. 넘으면 가장 오래된 것부터 빠진다.
func TestHistoryDropsOldestOverLimit(t *testing.T) {
	var h history
	for i := range historyLimit + 10 {
		h.add(fmt.Sprintf("e %d.txt", i))
	}

	require.Len(t, h.entries, historyLimit)
	assert.Equal(t, "e 10.txt", h.entries[0], "가장 오래된 열 개가 빠졌다")
	assert.Equal(t, "e 59.txt", h.entries[len(h.entries)-1])
}

// 접두로 거른다. 빈 접두는 전부다 — 거르는 경우와 전부 보는 경우가 같은 규칙의 양끝이다.
func TestHistoryMatchFiltersByPrefix(t *testing.T) {
	var h history
	h.add("w")
	h.add("e a.txt")
	h.add("e b.txt")

	assert.Equal(t, []string{"e a.txt", "e b.txt"}, h.match("e "))
	assert.Equal(t, []string{"w", "e a.txt", "e b.txt"}, h.match(""))
	assert.Empty(t, h.match("q"))
}

// 첫 위는 가장 최근 것이고 계속 누르면 거슬러 올라간다. 맨 위에서 멈춘다.
func TestHistoryBrowseWalksBackFromNewest(t *testing.T) {
	var h history
	h.add("w")
	h.add("q")

	var b historyBrowse

	text, ok := b.move(h, "", -1)
	require.True(t, ok)
	assert.Equal(t, "q", text)

	text, _ = b.move(h, "", -1)
	assert.Equal(t, "w", text)

	text, _ = b.move(h, "", -1)
	assert.Equal(t, "w", text, "맨 위에서 멈춘다")
}

// 맨 아래까지 내려오면 훑기 전에 치던 글이 돌아온다.
func TestHistoryBrowseReturnsTypedTextAtTheBottom(t *testing.T) {
	var h history
	h.add("wq")

	var b historyBrowse

	text, _ := b.move(h, "w", -1)
	require.Equal(t, "wq", text)

	text, ok := b.move(h, "w", 1)
	require.True(t, ok)
	assert.Equal(t, "w", text, "치던 글이 돌아온다")
}

// 훑기를 시작한 뒤에는 typed 인자가 달라져도 처음 붙든 것을 쓴다.
// 그래야 꺼낸 글이 다음 접두가 되어 목록이 한 칸마다 좁아지는 일이 없다.
func TestHistoryBrowseHoldsTheFirstPrefix(t *testing.T) {
	var h history
	h.add("e a.txt")
	h.add("w")

	var b historyBrowse

	text, _ := b.move(h, "", -1)
	require.Equal(t, "w", text)

	// 꺼낸 글("w") 이 새 접두로 들어와도 목록은 그대로다.
	text, _ = b.move(h, "w", -1)
	assert.Equal(t, "e a.txt", text)
}

// 꺼낼 것이 없으면 아무 일도 하지 않는다.
func TestHistoryBrowseGivesNothingWhenNoMatch(t *testing.T) {
	var h history
	h.add("w")

	var b historyBrowse

	_, ok := b.move(h, "zzz", -1)
	assert.False(t, ok)
}

// 글을 고치면 훑기를 놓는다. 다음 위는 새 접두로 다시 시작한다.
func TestHistoryBrowseStopResetsPrefix(t *testing.T) {
	var h history
	h.add("e a.txt")
	h.add("w")

	var b historyBrowse

	text, _ := b.move(h, "", -1)
	require.Equal(t, "w", text)

	b.stop()

	text, _ = b.move(h, "e", -1)
	assert.Equal(t, "e a.txt", text, "새 접두로 걸러진다")
}
