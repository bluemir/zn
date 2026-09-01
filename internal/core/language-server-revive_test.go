package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// 죽은 언어 서버를 되살리는 자리다(ADR-0092 §5).

// 죽을 때마다 알리고 되살린다. 너무 자주 죽으면 실패한 것으로 두고 그만둔다.
func TestRecordServerDeath(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("a.go")}, width: 80, height: 20}
	server := lsp.ServerFor("a.go")
	state := e.serverState(server)

	// 되살려 보는 횟수만큼은 「다시 띄운다」다.
	for i := 1; i <= serverMaxRevives; i++ {
		e.recordServerDeath(server.Name, state)

		assert.Equal(t, i, state.deaths)
		assert.False(t, state.failed, "%d 번째까지는 되살린다", i)
		assert.Contains(t, e.notice, "다시 띄웁니다")
	}

	// 한 번 더 죽으면 그만둔다. startServer 가 failed 를 보고 물러난다.
	e.recordServerDeath(server.Name, state)

	assert.True(t, state.failed, "더 띄우지 않는다")
	assert.Contains(t, e.notice, "자꾸 멎습니다")
}

// 알림에 그 서버의 이름이 든다. 서버가 여럿이라 「무엇이 멎었나」가 문구에 있어야 한다.
func TestRecordServerDeathNamesTheServer(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("app.py")}, width: 80, height: 20}
	server := lsp.ServerFor("app.py")

	e.recordServerDeath(server.Name, e.serverState(server))

	assert.Contains(t, e.notice, "pyright")
}

// 그만둔 뒤에는 startServer 가 물러난다. 되살리는 고리가 여기서 끊긴다.
func TestServerStopsAfterTooManyDeaths(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("a.go")}, width: 80, height: 20}
	server := lsp.ServerFor("a.go")
	state := e.serverState(server)

	state.deaths = serverMaxRevives
	e.recordServerDeath(server.Name, state)
	require.True(t, state.failed)

	assert.Nil(t, e.startServer(server), "물러난다")
	assert.False(t, state.starting, "뜨는 중으로도 서지 않는다")
}

// 한 서버가 그만둔 것이 다른 서버를 막지 않는다. 상태를 서버마다 따로 드는 값이 이것이다.
func TestServerFailureIsPerServer(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("a.go")}, width: 80, height: 20}

	goServer := lsp.ServerFor("a.go")
	pyServer := lsp.ServerFor("a.py")

	goState := e.serverState(goServer)
	goState.deaths = serverMaxRevives
	e.recordServerDeath(goServer.Name, goState)
	require.True(t, goState.failed)

	assert.False(t, e.serverState(pyServer).failed, "남의 실패가 옮지 않는다")
	assert.NotNil(t, e.startServer(pyServer), "python 서버는 그대로 띄울 수 있다")
}

// 설치가 끝나면 그 자리가 풀린다. 실패로 세운 것을 되돌리는 문이 그것이다(installServer).
func TestServerFailedClearedByInstall(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("a.go")}, width: 80, height: 20}
	server := lsp.ServerFor("a.go")
	state := e.serverState(server)

	state.deaths = serverMaxRevives
	e.recordServerDeath(server.Name, state)
	require.True(t, state.failed)

	// installServer 의 apply 가 하는 것과 같다.
	state.failed = false

	assert.NotNil(t, e.startServer(server), "다시 띄울 수 있다")
}

// 서버가 없으면 clientOf 는 조용히 nil 이다. 죽은 것과 뜬 적 없는 것을 셈에서 가른다.
func TestClientOfWithoutServer(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("a.go")}, width: 80, height: 20}
	server := lsp.ServerFor("a.go")

	assert.Nil(t, e.clientOf(server))
	assert.Equal(t, 0, e.serverState(server).deaths, "뜬 적이 없는 것은 죽음이 아니다")
	assert.False(t, e.serverState(server).failed)
	assert.Empty(t, e.notice, "알릴 것도 없다")
}
