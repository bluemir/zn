package lsp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publishOf 는 서버가 보내는 알림 본문 하나다.
func publishOf(t *testing.T, uri string, diagnostics ...Diagnostic) json.RawMessage {
	t.Helper()

	body, err := json.Marshal(publishDiagnosticsParams{URI: uri, Diagnostics: diagnostics})
	require.NoError(t, err)

	return body
}

// 담은 것을 경로로 읽어간다. URI 의 `file://` 과 %XX 는 벗겨진다.
func TestDiagnosticStorePublish(t *testing.T) {
	store := newDiagnosticStore()

	store.publish(publishOf(t, "file:///tmp/%ED%95%9C/main.go",
		Diagnostic{Severity: SeverityError, Source: "compiler", Message: "undefined: x"},
	))

	client := &Client{diagnostics: store}

	got := client.Diagnostics("/tmp/한/main.go")
	require.Len(t, got, 1)
	assert.Equal(t, "undefined: x", got[0].Message)
	assert.Equal(t, SeverityError, got[0].Severity)
}

// 다음 publish 가 앞의 것을 통째로 갈아치운다. 진단은 사건이 아니라 상태다.
func TestDiagnosticStoreReplaces(t *testing.T) {
	store := newDiagnosticStore()
	client := &Client{diagnostics: store}

	store.publish(publishOf(t, "file:///tmp/main.go",
		Diagnostic{Severity: SeverityError, Message: "첫째"},
		Diagnostic{Severity: SeverityWarning, Message: "둘째"},
	))
	require.Len(t, client.Diagnostics("/tmp/main.go"), 2)

	store.publish(publishOf(t, "file:///tmp/main.go",
		Diagnostic{Severity: SeverityError, Message: "셋째"},
	))

	got := client.Diagnostics("/tmp/main.go")
	require.Len(t, got, 1, "더하지 않고 갈아치운다")
	assert.Equal(t, "셋째", got[0].Message)
}

// 빈 목록은 「이제 없다」다. gopls 는 고친 파일에 이것을 보낸다(잰 값이다).
func TestDiagnosticStoreClears(t *testing.T) {
	store := newDiagnosticStore()
	client := &Client{diagnostics: store}

	store.publish(publishOf(t, "file:///tmp/main.go", Diagnostic{Severity: SeverityError, Message: "오류"}))
	require.NotEmpty(t, client.Diagnostics("/tmp/main.go"))

	store.publish(publishOf(t, "file:///tmp/main.go"))

	assert.Empty(t, client.Diagnostics("/tmp/main.go"))
}

// 종은 한 칸이다. 여러 번 울려도 막히지 않고, 받는 쪽은 종 하나에 지금 상태 전부를 읽어간다.
//
// **이것이 막히면 응답 전체가 멈춘다.** 읽는 goroutine 하나가 알림도 같이 나르기 때문이다
// (jsonrpc.go 의 readLoop). 그래서 아무도 종을 받지 않는 상태에서 publish 를 되풀이해 본다.
func TestDiagnosticStoreBellNeverBlocks(t *testing.T) {
	store := newDiagnosticStore()

	for i := 0; i < 100; i++ {
		store.publish(publishOf(t, "file:///tmp/main.go", Diagnostic{Severity: SeverityError, Message: "오류"}))
	}

	client := &Client{diagnostics: store}

	assert.Len(t, client.DiagnosticsChanged(), 1, "종은 한 번만 울려 있다")
	assert.NotEmpty(t, client.Diagnostics("/tmp/main.go"), "종을 버려도 담은 것은 남는다")
}

// 읽지 못한 알림은 버린다. 답을 보낼 상대도 알릴 자리도 없다.
func TestDiagnosticStoreIgnoresBrokenParams(t *testing.T) {
	store := newDiagnosticStore()

	store.publish(json.RawMessage(`{"uri": 42}`))

	client := &Client{diagnostics: store}
	assert.Empty(t, client.Diagnostics(""))
	assert.Empty(t, client.DiagnosticsChanged(), "종도 울리지 않는다")
}

// 흉내 서버가 보낸 알림이 conn 을 지나 받는 사람에게 닿는다.
func TestConnRoutesNotifications(t *testing.T) {
	server := newFakeServer(t)

	server.pushNotification(t, "textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         "file:///tmp/main.go",
		Diagnostics: []Diagnostic{{Severity: SeverityError, Message: "오류"}},
	})

	got := <-server.notified
	assert.Equal(t, "textDocument/publishDiagnostics", got.Method)

	var params publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(got.Params, &params))
	assert.Equal(t, "file:///tmp/main.go", params.URI)
}
