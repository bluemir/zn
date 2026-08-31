package lsp

import (
	"encoding/json"
	"sync"
)

// 서버가 밀어주는 진단(오류·경고) 을 받아 두는 자리다(ADR-0086).
//
// **묻지 않고 받는 유일한 것이다.** 정의·사용처·이름 바꾸기는 우리가 물어서 답을 받는데,
// 진단은 서버가 자기 때에 보낸다. 그래서 「답을 기다리는 사람」이 없고, 온 것을 어딘가에
// 두었다가 화면이 읽어가야 한다.
//
// **담아 두고 종을 울린다.** 온 것을 채널로 흘려보내면 읽는 goroutine 이 채널에서 막히고,
// 그 하나가 막히면 뒤따르는 모든 응답이 멈춘다(jsonrpc.go 의 readLoop). 그래서 최신 것을
// 파일별로 덮어써 두고, 「바뀌었다」만 칸이 하나인 채널로 알린다 — 종이 이미 울려 있으면
// 울리지 않고 지나간다. 진단은 사건이 아니라 상태라서 이렇게 할 수 있다. publish 는 그
// 파일의 진단 **전부**이고, 다음 publish 가 앞의 것을 통째로 갈아치운다.

// Severity 는 진단의 갈래다. 규격이 정한 숫자 그대로다.
//
// 잰 값으로 gopls v0.23.0 이 보내는 것은 1 과 2 뿐이었다 — 컴파일·문법 오류가 1 이고
// 분석기(`printf` 등) 경고가 2 다. 3·4(정보·힌트) 는 오지 않았다(ADR-0086).
type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// Diagnostic 은 진단 하나다. 우리가 쓰는 자리만 담는다 — `codeDescription`·`tags`·
// `relatedInformation` 은 서버가 보내도 버려진다(protocol.go 머리글).
type Diagnostic struct {
	Range    Range    `json:"range"`
	Severity Severity `json:"severity"`
	Source   string   `json:"source"` // "compiler" "syntax" 또는 분석기 이름
	Message  string   `json:"message"`
}

// publishDiagnosticsParams 는 `textDocument/publishDiagnostics` 알림의 본문이다.
//
// Version 은 서버가 이 진단을 낸 판이다. 우리는 보지 않는다 — 낡은 판의 진단도 그대로 두고
// 다음 publish 가 갈아치우게 한다(ADR-0086).
type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// diagnosticStore 는 파일별 최신 진단이다. Client 가 하나 들고 있다.
type diagnosticStore struct {
	mu sync.Mutex

	// byPath 는 경로별 진단이다. 빈 목록이 온 파일은 자리에서 지운다 —
	// 「진단이 없다」와 「그 파일 이야기를 못 들었다」를 가를 데가 없고, 가릴 이유도 없다.
	byPath map[string][]Diagnostic

	// changed 는 「바뀌었다」를 알리는 종이다. 칸이 하나고 넘치면 버린다.
	changed chan struct{}
}

func newDiagnosticStore() *diagnosticStore {
	return &diagnosticStore{
		byPath:  map[string][]Diagnostic{},
		changed: make(chan struct{}, 1),
	}
}

// publish 는 알림 하나를 담는다. 읽는 goroutine 에서 불리므로 막히면 안 된다.
func (s *diagnosticStore) publish(params json.RawMessage) {
	var got publishDiagnosticsParams
	if err := json.Unmarshal(params, &got); err != nil {
		// 읽지 못한 알림은 버린다. 답을 보낼 상대도, 알릴 자리도 없다.
		return
	}

	path := Location{URI: got.URI}.Path()

	s.mu.Lock()
	if len(got.Diagnostics) == 0 {
		delete(s.byPath, path)
	} else {
		s.byPath[path] = got.Diagnostics
	}
	s.mu.Unlock()

	// 종이 이미 울려 있으면 지나간다. 받는 쪽은 종 하나에 지금 상태 전부를 읽어간다.
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Diagnostics 는 그 파일의 지금 진단이다. 없으면 nil 이다.
//
// 담아 둔 것을 그대로 준다. 받는 쪽이 고치지 않는다 — 다음 publish 는 목록을 갈아끼우므로
// (publish) 지금 넘긴 것은 이 순간의 모습으로 굳는다.
func (c *Client) Diagnostics(path string) []Diagnostic {
	c.diagnostics.mu.Lock()
	defer c.diagnostics.mu.Unlock()

	return c.diagnostics.byPath[path]
}

// DiagnosticsChanged 는 진단이 바뀌었다고 울리는 종이다.
//
// 받은 쪽은 Diagnostics 로 지금 상태를 읽어간다. 무엇이 바뀌었는지는 싣지 않는다 —
// 열려 있는 파일을 훑어 다시 맞추는 것이 서버와 맞추는 자리에서 이미 쓰는 손이다
// (core/language-server.go 의 syncServers).
func (c *Client) DiagnosticsChanged() <-chan struct{} {
	return c.diagnostics.changed
}
