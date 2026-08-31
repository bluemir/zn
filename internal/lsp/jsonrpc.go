// Package lsp 는 언어 서버와 주고받는 것만 안다. 편집기도 화면도 모른다.
//
// 말을 거는 상대가 서버마다 하나씩 있다(ADR-0051, ADR-0107). 그래도 서버를 고르는 interface 는 두지 않았다 —
// 둘째 서버가 생기면 그때 무엇이 갈리는지 보고 가른다.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// message 는 오가는 JSON-RPC 한 통이다. 요청·응답·알림이 한 모양을 쓴다 — 무엇인지는
// 채워진 자리가 가른다. id 가 있으면 답을 기다리는 것이고, method 가 있으면 보내는 쪽이다.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *responseError) Error() string {
	return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
}

// conn 은 언어 서버와의 한 연결이다. 틀(Content-Length 머리와 본문) 과 id 짝짓기만 한다.
//
// 읽기는 goroutine 하나가 도맡는다. 답을 기다리는 쪽은 자기 id 의 채널에서 받는다 —
// 여러 요청이 동시에 떠 있어도 각자 자기 답을 받고, 서버가 보내는 알림은 읽는 쪽에서 버린다.
type conn struct {
	out io.WriteCloser // 서버의 stdin. 우리가 쓴다
	in  *bufio.Reader  // 서버의 stdout. 우리가 읽는다

	// onNotify 는 서버가 보내는 알림을 받을 사람이다. nil 이면 알림을 버린다.
	//
	// **여기서 막히면 안 된다.** 읽는 goroutine 이 하나라, 이것이 기다리면 뒤따르는 모든
	// 응답이 같이 멈춘다(readLoop). 그래서 채널로 받지 않고 함수로 받는다 — 받는 쪽이
	// 「담아 두고 종만 울리는」 모양이 되도록 강제하는 자리다(diagnostics.go 의 publish).
	onNotify func(method string, params json.RawMessage)

	// writeMu 는 쓰는 자리를 하나로 만든다. 머리와 본문이 한 통으로 붙어 나가야 하므로
	// 두 goroutine 이 같이 쓰면 통이 섞인다.
	writeMu sync.Mutex

	// mu 는 아래 셋을 지킨다. 요청을 내는 쪽은 tea 의 Cmd goroutine 이라 여럿이다.
	mu      sync.Mutex
	nextID  int
	pending map[int]chan message
	closed  bool
}

func newConn(out io.WriteCloser, in io.Reader, onNotify func(method string, params json.RawMessage)) *conn {
	c := &conn{
		out:      out,
		in:       bufio.NewReader(in),
		onNotify: onNotify,
		pending:  map[int]chan message{},
	}

	go c.readLoop()

	return c
}

// notify 는 답을 기다리지 않는 알림을 보낸다. didOpen·didChange 가 이것이다.
func (c *conn) notify(method string, params any) error {
	return c.write(message{Method: method, Params: mustMarshal(params)})
}

// isClosed 는 연결이 끊겼는지다. 서버가 죽으면 readLoop 이 끝나면서 서고(drainPending),
// 그 뒤의 모든 요청은 답을 받을 수 없다.
func (c *conn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closed
}

// call 은 요청을 보내고 답을 기다린다. 서버가 오류를 주면 그것이 err 이다.
//
// 기다리는 것을 ctx 로 끊지 않는다. 끊으면 답이 왔을 때 받을 사람이 없어 읽는 goroutine 이
// 채널에 막히고, 그 하나가 막히면 뒤따르는 모든 답이 멈춘다. 부르는 쪽이 기다리기를 그만두려면
// 연결을 닫는다 — 그러면 pending 이 전부 풀린다.
func (c *conn) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return nil, io.ErrClosedPipe
	}
	c.nextID++
	id := c.nextID
	reply := make(chan message, 1)
	c.pending[id] = reply
	c.mu.Unlock()

	if err := c.write(message{ID: &id, Method: method, Params: mustMarshal(params)}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()

		return nil, err
	}

	got, ok := <-reply
	if !ok {
		// 연결이 닫혔다. 답은 오지 않는다.
		return nil, io.ErrClosedPipe
	}

	if got.Error != nil {
		return nil, got.Error
	}

	return got.Result, nil
}

// write 는 한 통을 내보낸다. 머리에 본문 길이를 적는 것이 LSP 의 틀이다.
func (c *conn) write(m message) error {
	m.JSONRPC = "2.0"

	body, err := json.Marshal(m)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := fmt.Fprintf(c.out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.out.Write(body)

	return err
}

// readLoop 은 오는 통을 전부 읽어 답을 기다리는 쪽에 넘긴다.
//
// 서버가 우리에게 보내는 *요청* 은 반드시 답해야 한다. 답하지 않으면 서버가 그 자리에서
// 기다리다 멈추는 일이 있다. 우리는 아는 것이 없으므로 "그런 method 없다" 로 답한다 —
// 능력(capabilities) 을 최소로 알렸으므로 실제로 오는 것은 거의 없다(ADR-0051).
//
// 알림(id 없는 것) 은 onNotify 에게 넘긴다. 받을 사람이 없으면 버린다 — 진행률(`$/progress`)
// 처럼 화면에 자리가 없는 것이 그대로 지나간다(ADR-0086).
func (c *conn) readLoop() {
	defer c.drainPending()

	for {
		m, err := c.read()
		if err != nil {
			return
		}

		switch {
		case m.ID != nil && m.Method != "":
			// 서버가 우리에게 물었다.
			_ = c.write(message{ID: m.ID, Error: &responseError{Code: -32601, Message: "method not found"}})
		case m.ID != nil:
			c.deliver(m)
		case m.Method != "" && c.onNotify != nil:
			c.onNotify(m.Method, m.Params)
		}
	}
}

// deliver 는 답을 기다리는 쪽에 넘긴다. 기다리는 사람이 없으면 버린다.
func (c *conn) deliver(m message) {
	c.mu.Lock()
	reply, ok := c.pending[*m.ID]
	delete(c.pending, *m.ID)
	c.mu.Unlock()

	if ok {
		reply <- m
	}
}

// drainPending 은 기다리는 채널을 모두 닫는다. 읽기가 끝났으면 답은 영원히 오지 않는다.
func (c *conn) drainPending() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	for id, reply := range c.pending {
		close(reply)
		delete(c.pending, id)
	}
}

// read 는 한 통을 읽는다. 머리에서 길이를 얻고 그만큼 본문을 읽는다.
func (c *conn) read() (message, error) {
	length := -1

	for {
		line, err := c.in.ReadString('\n')
		if err != nil {
			return message{}, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // 머리 끝. 다음이 본문이다
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "content-length") {
			continue // 우리가 보는 것은 길이 하나다
		}

		length, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return message{}, fmt.Errorf("content-length 를 읽지 못했다: %q", line)
		}
	}

	if length < 0 {
		return message{}, fmt.Errorf("content-length 가 없는 통이 왔다")
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(c.in, body); err != nil {
		return message{}, err
	}

	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return message{}, err
	}

	return m, nil
}

// close 는 서버의 stdin 을 닫는다. 서버는 그것으로 끝난 것을 안다.
func (c *conn) close() error {
	return c.out.Close()
}

// mustMarshal 은 보낼 params 를 JSON 으로 바꾼다.
//
// 실패할 수 없다 — 넘기는 것이 모두 이 패키지 안에서 만든 struct 와 map 이라,
// 실패한다면 그것은 자료가 아니라 코드가 틀린 것이다. 그래서 오류를 위로 나르지 않고
// 빈 값으로 두어 서버가 거절하게 한다.
func mustMarshal(params any) json.RawMessage {
	if params == nil {
		return nil
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil
	}

	return body
}
