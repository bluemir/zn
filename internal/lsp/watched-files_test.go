package lsp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 밖에서 바뀐 파일을 알리는 알림과, 서버가 죽었는지 보는 자리다(ADR-0092).

// clientOn 은 흉내 서버에 붙은 Client 다. 틀만 보는 시험이라 이것으로 충분하다.
func clientOn(server *fakeServer) *Client {
	return &Client{
		conn:        server.conn,
		docs:        map[string]*document{},
		diagnostics: newDiagnosticStore(),
	}
}

// FilesChanged 는 규격이 정한 모양으로 한 통을 보낸다.
func TestFilesChangedSendsNotification(t *testing.T) {
	server := newFakeServer(t)
	client := clientOn(server)

	require.NoError(t, client.FilesChanged([]FileChange{
		{Path: "/tmp/a.go", Kind: FileChanged},
		{Path: "/tmp/새 파일.go", Kind: FileCreated},
		{Path: "/tmp/b.go", Kind: FileDeleted},
	}))

	sent := <-server.requests

	assert.Equal(t, "workspace/didChangeWatchedFiles", sent.Method)
	assert.Nil(t, sent.ID, "답을 기다리지 않는 알림이다")

	var params struct {
		Changes []struct {
			URI  string `json:"uri"`
			Type int    `json:"type"`
		} `json:"changes"`
	}
	require.NoError(t, json.Unmarshal(sent.Params, &params))
	require.Len(t, params.Changes, 3)

	assert.Equal(t, "file:///tmp/a.go", params.Changes[0].URI)
	assert.Equal(t, 2, params.Changes[0].Type, "Changed 는 2 다")

	// 경로의 공백과 한글은 URI 규칙대로 감싼다. didOpen 과 같은 자를 쓴다.
	assert.Equal(t, fileURI("/tmp/새 파일.go"), params.Changes[1].URI)
	assert.Equal(t, 1, params.Changes[1].Type, "Created 는 1 이다")

	assert.Equal(t, 3, params.Changes[2].Type, "Deleted 는 3 이다")
}

// 빈 목록이면 보내지 않는다. 서버를 깨울 이유가 없다.
func TestFilesChangedSkipsEmpty(t *testing.T) {
	server := newFakeServer(t)
	client := clientOn(server)

	require.NoError(t, client.FilesChanged(nil))
	require.NoError(t, client.FilesChanged([]FileChange{}))

	select {
	case sent := <-server.requests:
		t.Fatalf("보내지 않아야 한다: %s", sent.Method)
	case <-time.After(50 * time.Millisecond):
	}
}

// Closed 는 서버가 죽었는지다. 죽은 뒤의 요청은 전부 오류라 부르는 쪽이 이것을 보고 새로 띄운다.
func TestClosedAfterServerDies(t *testing.T) {
	server := newFakeServer(t)
	client := clientOn(server)

	assert.False(t, client.Closed(), "떠 있는 동안은 false 다")

	// 서버 쪽 파이프를 닫는다. 프로세스가 죽은 것과 같다 — readLoop 이 끝나면서
	// drainPending 이 자리를 세운다.
	server.kill()

	assert.Eventually(t, client.Closed, time.Second, 5*time.Millisecond, "죽으면 true 가 된다")

	// 죽은 뒤의 요청은 답을 기다리지 않고 곧바로 오류다. 여기서 막히면 편집기가 선다.
	done := make(chan error, 1)
	go func() {
		_, err := client.References("/tmp/a.go", Position{})
		done <- err
	}()

	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("죽은 서버에 물었는데 답을 기다리고 있다")
	}
}
