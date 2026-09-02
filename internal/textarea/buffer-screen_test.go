package textarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 머리줄이 담아둔 것을 채우는지 보는 시험이다(buffer-screen.go, ADR-0129).

// 머리줄이 담아둔 것을 채우는지 보는 시험이다(buffer-screen.go).

// **stickyAt 이 스스로 토큰 캐시를 채운다.**
//
// scrollTo 는 Update 에서 돌고 토큰은 View 에서 채워진다. 채우지 않으면 두 쪽이 한 프레임
// 어긋나서 커서가 머리줄 아래에 그려진다.
func TestStickyAtFillsTokenCache(t *testing.T) {
	buf := NewBuffer("doc.md", []byte("# A\n## B\n본문\n"))
	require.Equal(t, 0, buf.syntax.valid, "아직 한 번도 안 그렸다")

	assert.Equal(t, []int{0, 1}, buf.StickyAt(2, 20), "그리기 전에도 답한다")
	assert.Greater(t, buf.syntax.valid, 2, "훑은 만큼 캐시가 찼다")
}
