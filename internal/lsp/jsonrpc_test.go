package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServer 는 언어 서버 흉내다. 우리가 보낸 통을 읽고 대본대로 답한다.
//
// 진짜 gopls 로 재는 것은 여기서 하지 않는다 — 그것은 깔려 있어야 하고 1 초 남짓 걸린다.
// 여기서 보는 것은 틀(Content-Length 와 id 짝짓기) 이라 흉내로 충분하다.
type fakeServer struct {
	conn *conn

	// requests 는 서버가 받은 통들이다. 테스트가 무엇이 갔는지 본다.
	requests chan message

	// raw 는 서버가 우리에게 쓰는 자리다.
	raw io.Writer
}

// newFakeServer 는 conn 과 흉내 서버를 파이프 둘로 잇는다.
func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()

	clientReads, serverWrites := io.Pipe()
	serverReads, clientWrites := io.Pipe()

	server := &fakeServer{
		requests: make(chan message, 16),
		raw:      serverWrites,
	}
	server.conn = newConn(clientWrites, clientReads)

	go func() {
		reader := bufio.NewReader(serverReads)
		for {
			m, err := readFrom(reader)
			if err != nil {
				close(server.requests)

				return
			}

			server.requests <- m
		}
	}()

	t.Cleanup(func() {
		_ = server.conn.close()
		_ = serverWrites.Close()
	})

	return server
}

// reply 는 서버가 답을 보낸다.
func (s *fakeServer) reply(id int, result any) {
	body, err := json.Marshal(message{JSONRPC: "2.0", ID: &id, Result: mustMarshal(result)})
	if err != nil {
		panic(err)
	}

	writeFrame(s.raw, body)
}

// notifyClient 는 서버가 알림을 보낸다. 답을 기다리지 않는 통이다.
func (s *fakeServer) notifyClient(method string) {
	body, _ := json.Marshal(message{JSONRPC: "2.0", Method: method, Params: mustMarshal(map[string]any{"a": 1})})
	writeFrame(s.raw, body)
}

// askClient 는 서버가 우리에게 묻는다. 답하지 않으면 진짜 서버는 여기서 멈춘다.
func (s *fakeServer) askClient(id int, method string) {
	body, _ := json.Marshal(message{JSONRPC: "2.0", ID: &id, Method: method})
	writeFrame(s.raw, body)
}

func writeFrame(w io.Writer, body []byte) {
	_, _ = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	_, _ = w.Write(body)
}

// readFrom 은 conn.read 와 같은 일을 하는 테스트 쪽 읽기다.
func readFrom(reader *bufio.Reader) (message, error) {
	c := &conn{in: reader}

	return c.read()
}

// 답은 id 로 짝지어 온다. 여러 요청이 동시에 떠 있어도 각자 자기 답을 받는다.
func TestConnCallMatchesByID(t *testing.T) {
	server := newFakeServer(t)

	first := make(chan string, 1)
	second := make(chan string, 1)

	go func() {
		result, err := server.conn.call("first", map[string]any{})
		require.NoError(t, err)
		first <- string(result)
	}()

	got := <-server.requests
	require.NotNil(t, got.ID)
	firstID := *got.ID
	assert.Equal(t, "first", got.Method)

	go func() {
		result, err := server.conn.call("second", map[string]any{})
		require.NoError(t, err)
		second <- string(result)
	}()

	got = <-server.requests
	require.NotNil(t, got.ID)
	secondID := *got.ID
	assert.Equal(t, "second", got.Method)
	assert.NotEqual(t, firstID, secondID, "id 는 요청마다 달라야 한다")

	// 나중 요청에 먼저 답한다. 짝짓기가 순서가 아니라 id 로 되는지 본다.
	server.reply(secondID, map[string]any{"who": "second"})
	assert.JSONEq(t, `{"who":"second"}`, <-second)

	server.reply(firstID, map[string]any{"who": "first"})
	assert.JSONEq(t, `{"who":"first"}`, <-first)
}

// 서버가 오류로 답하면 그것이 err 이다.
func TestConnCallServerError(t *testing.T) {
	server := newFakeServer(t)

	done := make(chan error, 1)
	go func() {
		_, err := server.conn.call("textDocument/definition", map[string]any{})
		done <- err
	}()

	got := <-server.requests

	body, _ := json.Marshal(message{
		JSONRPC: "2.0",
		ID:      got.ID,
		Error:   &responseError{Code: 0, Message: "column is beyond end of line"},
	})
	writeFrame(server.raw, body)

	err := <-done
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column is beyond end of line")
}

// 서버가 우리에게 물으면 반드시 답한다. 답하지 않으면 진짜 서버가 그 자리에서 멈춘다.
func TestConnAnswersServerRequests(t *testing.T) {
	server := newFakeServer(t)

	server.askClient(77, "window/workDoneProgress/create")

	answer := <-server.requests
	require.NotNil(t, answer.ID)
	assert.Equal(t, 77, *answer.ID)
	require.NotNil(t, answer.Error)
	assert.Equal(t, -32601, answer.Error.Code)
}

// 서버의 알림은 버린다. 버리는 것이 읽기를 막지 않는지가 요점이다 —
// 막히면 그 뒤의 모든 답이 함께 멈춘다.
func TestConnIgnoresNotifications(t *testing.T) {
	server := newFakeServer(t)

	for range 10 {
		server.notifyClient("textDocument/publishDiagnostics")
	}

	done := make(chan error, 1)
	go func() {
		_, err := server.conn.call("initialize", map[string]any{})
		done <- err
	}()

	got := <-server.requests
	server.reply(*got.ID, map[string]any{"capabilities": map[string]any{}})

	require.NoError(t, <-done)
}

// 연결이 닫히면 기다리던 요청이 풀린다. 그러지 않으면 gopls 가 죽었을 때 Cmd goroutine 이 남는다.
func TestConnCloseUnblocksPending(t *testing.T) {
	server := newFakeServer(t)

	done := make(chan error, 1)
	go func() {
		_, err := server.conn.call("initialize", map[string]any{})
		done <- err
	}()

	<-server.requests

	// 서버가 죽은 것과 같다. 우리 쪽 읽기가 끝나고 기다리는 채널이 닫힌다.
	_ = server.raw.(io.Closer).Close()

	assert.Error(t, <-done)
}

// 한글이 든 통은 길이를 byte 로 세야 한다. 글자 수로 세면 본문이 잘린다.
func TestConnFramesByBytes(t *testing.T) {
	server := newFakeServer(t)

	require.NoError(t, server.conn.notify("textDocument/didOpen", map[string]any{
		"text": "가나다라마 🙂",
	}))

	got := <-server.requests
	assert.Equal(t, "textDocument/didOpen", got.Method)

	var params map[string]string
	require.NoError(t, json.Unmarshal(got.Params, &params))
	assert.Equal(t, "가나다라마 🙂", params["text"])
}
