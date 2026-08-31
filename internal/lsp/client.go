package lsp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"
)

// Client 는 도는 언어 서버 하나다. 지금은 gopls 다(ADR-0051).
//
// 편집기가 도는 동안 하나만 있고, Go 파일을 처음 열 때 뜬다. 편집기가 끝나면 ctx 가 끊겨
// 프로세스도 같이 죽는다 — 작업들이 편집기 수명에서 갈라져 나오는 것과 같다(ADR-0027).
type Client struct {
	conn *conn
	cmd  *exec.Cmd

	// mu 는 docs 를 지킨다. 요청은 tea 의 Cmd goroutine 에서 오므로 여럿이 동시에 온다.
	mu   sync.Mutex
	docs map[string]*document

	// diagnostics 는 서버가 밀어준 진단이다. 우리가 물어서 받는 것이 아니라 서버가 자기 때에
	// 보내는 것이라, 답을 기다리는 자리가 아니라 담아 두는 자리가 필요하다(diagnostics.go).
	diagnostics *diagnosticStore

	// semantic 은 서버가 악수에서 알린 문법 토큰 이름표다(semantic.go).
	//
	// **악수에서 한 번 적고 그 뒤로는 읽기만 한다.** Start 가 돌려주기 전에 채워지므로
	// 여럿이 동시에 읽어도 mu 가 필요 없다 — docs 와 달리 바뀌지 않는 값이다.
	semantic semanticLegend
}

// Start 는 서버를 띄우고 첫 악수(initialize) 까지 끝낸다.
//
// 잰 값으로 300ms 남짓 걸린다. 화면을 그리는 goroutine 에서 부르면 그만큼 멈추므로
// 부르는 쪽이 Cmd 안에서 부른다(ADR-0051).
//
// root 는 서버가 볼 작업 폴더다. zn 을 띄운 자리를 그대로 넘긴다 — 파일 트리의 뿌리와 같다.
func Start(ctx context.Context, root string) (*Client, error) {
	binary, err := findGopls()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binary, "-mode=stdio")
	cmd.Dir = root

	// stderr 는 버린다. gopls 가 쓰는 것은 자기 로그이고, 우리 화면은 터미널을 나눠 쓰고 있어서
	// 그대로 두면 그림 위에 글자가 쏟아진다.
	cmd.Stderr = nil

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "gopls 의 stdin 을 열지 못했다")
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "gopls 의 stdout 을 열지 못했다")
	}

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "gopls 를 띄우지 못했다 (%s)", binary)
	}

	client := &Client{
		cmd:         cmd,
		docs:        map[string]*document{},
		diagnostics: newDiagnosticStore(),
	}

	// 알림을 받을 사람을 걸어 둔다. 지금 우리가 아는 알림은 진단 하나이고, 나머지
	// (`window/showMessage`·`$/progress`) 는 여기서 그대로 지나간다(ADR-0086).
	client.conn = newConn(in, out, func(method string, params json.RawMessage) {
		if method != "textDocument/publishDiagnostics" {
			return
		}

		client.diagnostics.publish(params)
	})

	if err := client.initialize(root); err != nil {
		client.Shutdown()

		return nil, err
	}

	return client, nil
}

// initialize 는 규격이 정한 첫 악수다. 이것을 끝내기 전에는 어떤 요청도 보낼 수 없다.
//
// 알리는 능력(capabilities) 은 비워 둔다. 우리가 쓰는 것은 정의 찾기 하나이고, 능력을 알리면
// 서버가 그만큼 우리에게 되묻기 시작한다 — 진행률 창을 만들라거나 설정을 달라거나 하는
// 요청이고, 답할 자리가 없으면 서버가 그 자리에서 기다린다(jsonrpc.go 의 readLoop).
//
// **진단도 이 빈 능력 그대로 온다.** 규격에는 `publishDiagnostics` 능력 칸이 있지만 gopls
// v0.23.0 은 알리지 않아도 보낸다(잰 값이다). 그래서 진단을 받으려고 이 자리를 채우지
// 않는다 — 받는 것이 하나 늘었을 뿐이고, 되묻기를 부르는 문은 그대로 닫혀 있다(ADR-0086).
//
// **문법 토큰은 설정으로 켠다.** gopls 의 `semanticTokens` 는 기본값이 꺼짐이라(v0.23.0 의
// `gopls api-json` 에서 확인) 켜지 않으면 악수 응답에 이름표가 아예 없다. 능력 칸이 아니라
// 서버 설정이라 이 한 줄로 끝나고, 빈 능력은 그대로다 — 재 보니 능력을 비운 채로도 토큰이
// 온다(ADR-0103).
func (c *Client) initialize(root string) error {
	result, err := c.conn.call("initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(root),
		"clientInfo": map[string]any{
			"name": "zn",
		},
		"capabilities": map[string]any{},
		"initializationOptions": map[string]any{
			"semanticTokens": true,
		},
	})
	if err != nil {
		return errors.Wrap(err, "gopls 와 악수하지 못했다")
	}

	c.semantic = readSemanticLegend(result)

	return c.conn.notify("initialized", map[string]any{})
}

// Open 은 파일 하나를 서버에 알린다. 여기서부터 그 파일의 사본을 우리도 든다.
//
// 이미 아는 파일이면 전문으로 다시 맞춘다 — 파일을 다시 읽었거나(`:e`) 밖에서 바뀐 것을
// 가져온 뒤라, 사본이 남아 있어도 믿을 것이 아니다.
func (c *Client) Open(path string, lines [][]byte) error {
	c.mu.Lock()
	_, known := c.docs[path]
	if known {
		c.mu.Unlock()

		return c.SyncFull(path, lines)
	}

	c.docs[path] = &document{version: 1, lines: copyLines(lines)}
	c.mu.Unlock()

	return c.conn.notify("textDocument/didOpen", map[string]any{
		"textDocument": textDocumentItem{
			URI:        fileURI(path),
			LanguageID: "go",
			Version:    1,
			Text:       text(lines),
		},
	})
}

// Close 는 파일을 닫았다고 알린다. 서버는 그때부터 디스크의 내용을 본다.
func (c *Client) Close(path string) error {
	c.mu.Lock()
	_, known := c.docs[path]
	delete(c.docs, path)
	c.mu.Unlock()

	if !known {
		return nil
	}

	return c.conn.notify("textDocument/didClose", map[string]any{
		"textDocument": textDocumentIdentifier{URI: fileURI(path)},
	})
}

// CloseAllExcept 는 목록에 없는 파일을 모두 닫는다. tab 을 닫은 것이 이 길로 온다.
//
// 닫는 자리에 갈고리를 걸지 않고 이렇게 하는 것은, 닫히는 길이 여럿이라서다 — tab 닫기,
// 다른 tab 모두 닫기, `:e` 로 갈아끼우기가 모두 파일을 하나 떨군다. 열려 있는 목록을
// 견주면 그 셋이 한 자리로 모인다(core/gopls.go 의 syncGopls).
func (c *Client) CloseAllExcept(alive map[string]bool) {
	c.mu.Lock()
	stale := make([]string, 0, len(c.docs))
	for path := range c.docs {
		if !alive[path] {
			stale = append(stale, path)
		}
	}
	c.mu.Unlock()

	for _, path := range stale {
		_ = c.Close(path)
	}
}

// Sync 는 사본과 지금의 차이만 보낸다. 평소 타이핑이 이 길로 간다.
//
// 달라진 것이 없으면 아무것도 보내지 않는다. 모르는 파일도 아무것도 하지 않는다 —
// 여는 것은 Open 의 몫이다.
func (c *Client) Sync(path string, lines [][]byte) error {
	c.mu.Lock()
	doc, known := c.docs[path]
	if !known {
		c.mu.Unlock()

		return nil
	}

	change, changed := diff(doc.lines, lines)
	if !changed {
		c.mu.Unlock()

		return nil
	}

	doc.version++
	doc.lines = copyLines(lines)
	version := doc.version
	c.mu.Unlock()

	return c.change(path, version, change)
}

// SyncFull 은 파일 전문을 보낸다. 요청 직전에 한 번 이것으로 맞춘다.
//
// 증분이 어긋날 자리는 없게 짜여 있지만(document.go), 어긋났다면 그것이 보이는 순간이
// 바로 무언가를 묻는 순간이다. 그 자리에서 처음부터 맞추면 틀린 답이 나올 창이 없다 —
// 주기적으로 맞추는 것과 달리 조용히 틀려 있는 동안이 아예 생기지 않는다(ADR-0051).
func (c *Client) SyncFull(path string, lines [][]byte) error {
	c.mu.Lock()
	doc, known := c.docs[path]
	if !known {
		c.mu.Unlock()

		return c.Open(path, lines)
	}

	doc.version++
	doc.lines = copyLines(lines)
	version := doc.version
	c.mu.Unlock()

	return c.change(path, version, contentChange{Text: text(lines)})
}

func (c *Client) change(path string, version int, change contentChange) error {
	return c.conn.notify("textDocument/didChange", map[string]any{
		"textDocument":   versionedTextDocumentIdentifier{URI: fileURI(path), Version: version},
		"contentChanges": []contentChange{change},
	})
}

// Tracks 는 그 파일을 서버에 알렸는지다. 부르는 쪽이 Open 과 Sync 를 가르는 데 쓴다.
func (c *Client) Tracks(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, known := c.docs[path]

	return known
}

// Closed 는 서버가 죽었는지다. 죽은 뒤의 요청은 전부 오류이므로 부르는 쪽이 이 자리를 보고
// 새로 띄운다(core 의 goplsClient).
func (c *Client) Closed() bool {
	return c.conn.isClosed()
}

// FileChangeKind 는 디스크에서 무엇이 일어났는지다. 규격의 FileChangeType 번호 그대로다.
type FileChangeKind int

const (
	FileCreated FileChangeKind = 1
	FileChanged FileChangeKind = 2
	FileDeleted FileChangeKind = 3
)

// FileChange 는 디스크에서 바뀐 파일 하나다.
type FileChange struct {
	Path string
	Kind FileChangeKind
}

// FilesChanged 는 **밖에서** 바뀐 파일들을 서버에 알린다.
//
// **이것 없이는 서버가 모른다.** gopls 는 스스로 디스크를 감시하지 않고 이 알림에 기댄다.
// 재보니 밖에서 만든 파일의 사용처가 몇 초를 기다려도 0 개였고, 이 알림 하나에 500ms 뒤
// 잡혔다. `git checkout` 뒤에 「쓰는 곳이 있는데 없다고 나온다」가 그것이다(ADR-0092).
//
// **능력을 알리지 않고 보낸다.** 규격은 `workspace.didChangeWatchedFiles` 능력과
// `client/registerCapability` 를 거치게 되어 있는데, 재보니 gopls 는 알리지 않아도 이
// 알림을 받는다. 그래서 ADR-0051 이 닫아 둔 「서버가 우리에게 되묻는」 문을 열지 않는다.
//
// **열어 둔 파일에는 소용이 없다.** overlay(didOpen 으로 보낸 사본) 가 디스크를 이긴다 —
// 재서 확인했다. 그쪽은 buffer 를 다시 읽어 didChange 로 보내야 한다(ADR-0092).
//
// 빈 목록이면 보내지 않는다. 서버를 깨울 이유가 없다.
func (c *Client) FilesChanged(changes []FileChange) error {
	if len(changes) == 0 {
		return nil
	}

	events := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		events = append(events, map[string]any{
			"uri":  fileURI(change.Path),
			"type": int(change.Kind),
		})
	}

	return c.conn.notify("workspace/didChangeWatchedFiles", map[string]any{
		"changes": events,
	})
}

// Definition 은 그 자리의 정의가 어디인지 묻는다.
//
// pos 의 열은 UTF-16 코드 단위다(protocol.go 의 Position). 부르는 쪽이 이미 바꿔서 준다.
//
// 첫 요청은 서버가 모듈을 훑는 동안 1 초 남짓 걸린다. 그 뒤로는 잰 값으로 15ms 아래다.
func (c *Client) Definition(path string, pos Position) ([]Location, error) {
	result, err := c.conn.call("textDocument/definition", map[string]any{
		"textDocument": textDocumentIdentifier{URI: fileURI(path)},
		"position":     pos,
	})
	if err != nil {
		return nil, errors.Wrap(err, "정의를 묻지 못했다")
	}

	return parseLocations(result)
}

// parseLocations 는 정의 응답을 읽는다.
//
// 규격이 `Location` 하나, `Location` 목록, `null` 셋을 다 허용한다. gopls 는 잰 모든 경우에
// 목록으로 답했지만(ADR-0051) 하나로 오는 것도 읽는다 — 남의 응답이라 우리 관측이 규격보다
// 좁을 이유가 없다.
func parseLocations(result json.RawMessage) ([]Location, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	var many []Location
	if err := json.Unmarshal(result, &many); err == nil {
		return many, nil
	}

	var one Location
	if err := json.Unmarshal(result, &one); err != nil {
		return nil, errors.Wrap(err, "정의 응답을 읽지 못했다")
	}

	return []Location{one}, nil
}

// Shutdown 은 서버를 내린다. 편집기를 끝낼 때 부른다.
//
// `exit` 를 알리고 stdin 을 닫는다. 규격은 앞에 `shutdown` 요청을 두라고 하지만 그 답을
// 기다리지 않는다 — 나가는 길에 남의 프로세스를 기다릴 이유가 없고, 답을 안 받을 요청은
// 보내지 않는 것이 낫다. 서버는 stdin 이 닫힌 것으로도 끝난다.
//
// 이것을 못 부르고 죽어도(SIGKILL) 남지 않는다. 프로세스가 ctx 에 매여 있고(exec.CommandContext)
// 우리가 죽으면 stdin 이 닫힌다.
func (c *Client) Shutdown() {
	_ = c.conn.notify("exit", nil)
	_ = c.conn.close()
}

// findGopls 는 gopls 실행 파일을 찾는다.
//
// **PATH 만 보지 않는다.** `go install` 은 `$GOBIN` 또는 `$GOPATH/bin` 에 넣고 그 자리가
// PATH 에 없는 기계가 흔하다 — 이 기능을 만든 기계가 그랬다(ADR-0051). 그래서 go 의 관례
// 자리까지 본다.
func findGopls() (string, error) {
	if path, err := exec.LookPath("gopls"); err == nil {
		return path, nil
	}

	for _, dir := range goBinDirs() {
		path := filepath.Join(dir, "gopls")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", errors.New("gopls 를 찾지 못했다. `go install golang.org/x/tools/gopls@latest` 로 넣는다")
}

// goBinDirs 는 `go install` 이 실행 파일을 넣는 자리들이다. 앞에 있는 것이 먼저다.
func goBinDirs() []string {
	if bin := os.Getenv("GOBIN"); bin != "" {
		return []string{bin}
	}

	dirs := []string{}
	for _, root := range filepath.SplitList(os.Getenv("GOPATH")) {
		if root != "" {
			dirs = append(dirs, filepath.Join(root, "bin"))
		}
	}

	// GOPATH 가 비어 있으면 go 의 기본값은 `~/go` 다.
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}

	return dirs
}
