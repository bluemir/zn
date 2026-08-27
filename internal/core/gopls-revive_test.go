package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 죽은 gopls 를 되살리는 자리다(ADR-0092 §5).

// 죽을 때마다 알리고 되살린다. 너무 자주 죽으면 실패한 것으로 두고 그만둔다.
func TestRecordGoplsDeath(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.go")}, width: 80, height: 20}

	// 되살려 보는 횟수만큼은 「다시 띄운다」다.
	for i := 1; i <= goplsMaxRevives; i++ {
		e.recordGoplsDeath()

		assert.Equal(t, i, e.goplsDeaths)
		assert.False(t, e.goplsFailed, "%d 번째까지는 되살린다", i)
		assert.Contains(t, e.notice, "다시 띄웁니다")
	}

	// 한 번 더 죽으면 그만둔다. startGopls 가 goplsFailed 를 보고 물러난다.
	e.recordGoplsDeath()

	assert.True(t, e.goplsFailed, "더 띄우지 않는다")
	assert.Contains(t, e.notice, "자꾸 멎습니다")
}

// 그만둔 뒤에는 startGopls 가 물러난다. 되살리는 고리가 여기서 끊긴다.
func TestGoplsStopsAfterTooManyDeaths(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.go")}, width: 80, height: 20}

	e.goplsDeaths = goplsMaxRevives
	e.recordGoplsDeath()
	require.True(t, e.goplsFailed)

	assert.Nil(t, e.startGopls(), "물러난다")
	assert.False(t, e.goplsStarting, "뜨는 중으로도 서지 않는다")
}

// 설치가 끝나면 그 자리가 풀린다. 실패로 세운 것을 되돌리는 문이 그것이다(installGopls).
func TestGoplsFailedClearedByInstall(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.go")}, width: 80, height: 20}

	e.goplsDeaths = goplsMaxRevives
	e.recordGoplsDeath()
	require.True(t, e.goplsFailed)

	// installGopls 의 apply 가 하는 것과 같다.
	e.goplsFailed = false

	assert.NotNil(t, e.startGopls(), "다시 띄울 수 있다")
}

// 서버가 없으면 goplsClient 는 조용히 nil 이다. 죽은 것과 뜬 적 없는 것을 셈에서 가른다.
func TestGoplsClientWithoutServer(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.go")}, width: 80, height: 20}

	assert.Nil(t, e.goplsClient())
	assert.Equal(t, 0, e.goplsDeaths, "뜬 적이 없는 것은 죽음이 아니다")
	assert.False(t, e.goplsFailed)
	assert.Empty(t, e.notice, "알릴 것도 없다")
}
