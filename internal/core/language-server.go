package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/lsp"
)

// 언어 서버와 이어지는 자리다. 하는 일은 셋이다 — 띄우기, 보고 있는 파일을 서버와 맞추기,
// 커서 자리의 정의를 묻기(ADR-0051, ADR-0107).
//
// 서버 쪽 이야기는 internal/lsp 가 안다. 여기 있는 것은 「언제 무엇을 맞추고 무엇을 묻는가」다.
//
// **어느 파일에 어느 서버가 붙는지도 저쪽 표가 안다**(lsp/server.go 의 servers). 여기서
// 확장자를 보지 않는 것이 서버를 더할 때 손대는 자리를 한 곳에 남긴다.

// languageServer 는 서버 하나의 지금 상태다.
//
// **서버마다 따로 든다.** 뜨는 중인지·실패했는지·몇 번 죽었는지가 서버마다 다른 값이라,
// 하나로 묶으면 gopls 가 깔려 있지 않은 것 때문에 pyright 도 함께 물러난다(ADR-0107).
type languageServer struct {
	// server 는 표의 그 줄이다. 상태를 든 자리가 서버 줄도 같이 들어야, 죽은 것을 되살리는
	// 자리가 이름만 들고 표를 되짚지 않는다(syncServers).
	server *lsp.Server

	client *lsp.Client

	// starting 은 뜨는 중인지고 failed 는 한 번 실패했는지다. 실패한 뒤에는 다시 걸지
	// 않는다 — 까닭이 대개 「깔려 있지 않다」 라서 파일을 열 때마다 다시 시도해도 나아지지
	// 않고, 그때마다 오류 문구가 화면 아래를 차지한다.
	starting bool
	failed   bool

	// deaths 는 서버가 뜬 뒤에 죽은 횟수다. **되살리기를 멈추는 자리가 있어야 해서 센다** —
	// 뜨자마자 죽는 서버를 끝없이 되살리면 tick 마다 프로세스가 하나씩 뜬다(ADR-0092).
	deaths int
}

// serverReadyMsg 는 서버가 떴다는 것이다. 실패도 이 길로 온다.
//
// 어느 서버인지를 싣는다. 서버가 여럿이라 답만 보고는 누구의 것인지 알 수 없다.
type serverReadyMsg struct {
	server *lsp.Server
	client *lsp.Client
	err    error
}

// definitionMsg 는 정의를 물은 답이다.
type definitionMsg struct {
	locations []lsp.Location
	err       error
}

// serverPath 는 그 buffer 를 볼 서버와, 서버에 알릴 때 쓸 절대 경로다.
// 볼 서버가 없거나 알릴 파일이 없으면 false 다.
//
// 절대 경로로 맞추는 것은 URI 가 절대 경로여야 하기 때문이고, 이름 없는 buffer(새 파일) 를
// 빼는 것은 알릴 파일이 없기 때문이다.
func serverPath(path string) (*lsp.Server, string, bool) {
	if path == "" {
		return nil, "", false
	}

	server := lsp.ServerFor(path)
	if server == nil {
		return nil, "", false
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", false
	}

	return server, absolute, true
}

// serverState 는 그 서버의 상태 자리다. 아직 없으면 만들어 둔다.
func (e *editor) serverState(server *lsp.Server) *languageServer {
	if e.servers == nil {
		e.servers = map[string]*languageServer{}
	}

	state, ok := e.servers[server.Name]
	if !ok {
		state = &languageServer{server: server}
		e.servers[server.Name] = state
	}

	return state
}

// startServer 는 서버를 띄운다. 이미 떴거나 뜨는 중이거나 한 번 실패했으면 아무것도 하지 않는다.
//
// 한 번 실패하면 다시 걸지 않는다. 실패하는 까닭은 대개 「깔려 있지 않다」 이고, 그것은
// 파일을 열 때마다 다시 시도해서 나아지는 종류가 아니다. 사용자가 깔고 나면 편집기를
// 다시 띄우거나 `\gd` 를 한 번 더 치는 것이 다시 거는 자리다.
func (e *editor) startServer(server *lsp.Server) tea.Cmd {
	state := e.serverState(server)
	if state.client != nil || state.starting || state.failed {
		return nil
	}

	state.starting = true
	ctx := e.rootContext()

	// 작업 폴더를 못 읽으면 띄우지 않는다. 서버가 볼 뿌리가 없으면 모듈을 찾을 수 없다.
	root, err := os.Getwd()
	if err != nil {
		state.starting = false
		state.failed = true

		return nil
	}

	e.serverRoot = root

	return func() tea.Msg {
		client, err := lsp.Start(ctx, root, server)

		return serverReadyMsg{server: server, client: client, err: err}
	}
}

// startServerForOpenFile 은 방금 연 파일을 볼 서버가 있으면 미리 띄운다.
//
// 묻는 순간까지 기다리지 않는 것은 첫 요청이 서버가 모듈을 훑는 동안 1 초 남짓 걸리기
// 때문이다(잰 값이다). 파일을 열 때 시작해 두면 그 기다림이 `\gd` 앞으로 옮겨간다.
func (e *editor) startServerForOpenFile(path string) tea.Cmd {
	server := lsp.ServerFor(path)
	if server == nil {
		return nil
	}

	return e.startServer(server)
}

// startServersForOpenBuffers 는 이미 열려 있는 tab 이 쓰는 서버들을 띄운다.
//
// 시작할 때 CLI 인자로 받은 파일들이 이 길이다. 그것들은 openTab 을 지나지 않아서
// 파일을 여는 자리의 갈고리에 걸리지 않는다(core.go 의 openBuffers).
//
// **하나에서 멈추지 않는다.** `zn main.go app.py` 처럼 여러 언어를 한꺼번에 열 수 있어서,
// 첫 서버를 띄우고 돌아가면 나머지 언어는 파일을 다시 열 때까지 서버가 없다.
func (e *editor) startServersForOpenBuffers() tea.Cmd {
	cmds := []tea.Cmd{}
	started := map[string]bool{}

	for i := range e.buffers {
		server, _, ok := serverPath(e.buffers[i].path)
		if !ok || started[server.Name] {
			continue
		}

		started[server.Name] = true

		if cmd := e.startServer(server); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) == 0 {
		return nil
	}

	return tea.Batch(cmds...)
}

// finishServer 는 서버가 떴거나 실패한 것을 받는다.
func (e *editor) finishServer(msg serverReadyMsg) tea.Cmd {
	state := e.serverState(msg.server)
	state.starting = false

	if msg.err != nil {
		state.failed = true
		e.notifyError(msg.err)

		return nil
	}

	state.client = msg.client

	// 이미 열려 있는 파일들을 알린다. 지금 보고 있는 것만이 아니다 — tab 여럿이 열린 채로
	// 서버가 떴을 수 있다.
	//
	// 진단을 기다리는 고리도 여기서 시작한다. 서버가 떠 있는 동안만 도는 고리이고, 시작하는
	// 자리가 여기 하나다 — 알리는 자리(syncServers) 와 같이 두면 tick 마다 고리가 하나씩
	// 늘어난다(ADR-0086).
	//
	// 문법 토큰은 여기서 곧바로 묻지 않고 tick 을 하나 건다. 지금은 파일을 이제 막 알리는
	// 참이라 서버가 아직 그 파일을 모르고, 250ms 뒤 그 tick 이 묻는다. 이것이 없으면 파일을
	// 열어 두고 손을 대지 않는 동안에는 서버의 색이 오지 않는다(semantic.go, ADR-0103).
	return tea.Batch(e.syncServers(), waitDiagnostics(msg.server.Name, state.client), e.scheduleEditTick())
}

// syncServers 는 열려 있는 파일들을 각자의 서버와 맞춘다.
//
// **무엇이 바뀌었는지 적어 두지 않는다.** 지금 열려 있는 목록과 서버가 아는 목록을 견주어
// 새로 연 것은 알리고, 닫은 것은 닫고, 남은 것은 차이만 보낸다. 편집·열기·닫기 자리마다
// 갈고리를 걸지 않아도 되고, 어느 자리를 빠뜨려도 다음 tick 에 제자리를 찾는다.
//
// 보내는 것은 goroutine 에서 한다. 남의 프로세스의 파이프에 쓰는 일이라, 그 프로세스가
// 잠깐 읽지 않으면 화면을 그리는 쪽이 같이 멈춘다.
//
// **서버마다 자기 파일만 본다.** 한 서버에 남의 언어 파일을 알리면 그 서버가 그것을 자기
// 언어로 읽으려 든다. 그래서 열린 목록을 서버별로 갈라서 각자에게 그 몫만 준다.
func (e *editor) syncServers() tea.Cmd {
	// 줄 목록은 그대로 넘겨도 된다. 편집은 겉껍데기를 새로 만들어 갈아끼우므로(edit.go 의
	// replaceLines) 지금 넘긴 것은 이 순간의 모습으로 굳는다.
	type snapshot struct {
		path  string
		lines [][]byte
	}

	open := map[string][]snapshot{}
	for i := range e.buffers {
		server, path, ok := serverPath(e.buffers[i].path)
		if !ok {
			continue
		}

		open[server.Name] = append(open[server.Name], snapshot{path: path, lines: e.buffers[i].lines})
	}

	cmds := []tea.Cmd{}

	for name, state := range e.servers {
		// **죽은 서버를 여기서 알아채고 되살린다.** tick 마다 지나는 자리라 사람이 무엇을
		// 묻기 전에 돌아온다(ADR-0092).
		//
		// **죽은 것과 뜬 적 없는 것을 가른다.** 죽기 전에 미리 재 두는 것이 그 일이다 —
		// clientOf 가 자리를 비운 뒤에는 둘이 같은 모습이 된다. 뜬 적이 없는데 여기서
		// 띄우면 그 언어의 파일을 열지도 않은 세션에서 서버가 뜬다. 처음 띄우는 자리는
		// 파일을 열 때다(startServerForOpenFile).
		died := state.client != nil && state.client.Closed()

		client := e.clientNamed(name)
		if client == nil {
			if died {
				cmds = append(cmds, e.startServer(state.server))
			}

			continue
		}

		docs := open[name]

		cmds = append(cmds, func() tea.Msg {
			alive := map[string]bool{}

			for _, doc := range docs {
				alive[doc.path] = true

				if client.Tracks(doc.path) {
					_ = client.Sync(doc.path, doc.lines)

					continue
				}

				_ = client.Open(doc.path, doc.lines)
			}

			client.CloseAllExcept(alive)

			// 알릴 것이 없다. 다음 tick 은 다음 키가 예약한다.
			//
			// nil 을 내는 Cmd 는 bubbletea 가 그냥 건너뛴다(tea.go 의 `if msg == nil`).
			// 받는 쪽이 없는 msg 를 만들지 않으려고 이렇게 둔다.
			return nil
		})
	}

	if len(cmds) == 0 {
		return nil
	}

	return tea.Batch(cmds...)
}

// serverJobName 은 그 서버의 설치 작업 이름이다. `:jobs` 에 이 이름으로 뜬다.
func serverJobName(server *lsp.Server) string {
	return server.Name + " 설치"
}

// serverForJobName 은 그 작업이 어느 서버의 설치인지다. 서버 것이 아니면 nil 이다.
//
// 끝난 작업을 받는 자리가 「방금 깐 것을 띄워도 되는가」를 묻는 데 쓴다(job.go 의 jobDoneMsg).
// 이름을 되짚는 것은 설치 작업 이름이 서버마다 달라서 `switch` 의 case 로 적을 수 없기
// 때문이고, 표를 견주므로 `goimports 설치` 같은 남의 작업이 여기 걸리지 않는다.
func serverForJobName(name string) *lsp.Server {
	for _, server := range lsp.Servers() {
		if serverJobName(server) == name {
			return server
		}
	}

	return nil
}

// gotoDefinition 은 커서 자리의 정의로 가거나, 서버가 없으면 설치를 묻는다.
func gotoDefinition(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	// 볼 파일이 없으면 커서도 없다. `\gd` 와 팔레트가 함께 지나는 자리라 여기서 막는다
	// (ADR-0064).
	if e.refuseNoBuffer() {
		return nil, nil
	}

	server, _, ok := serverPath(e.activeBuffer().path)
	if !ok {
		e.notify("언어 서버가 붙는 파일에서만 정의를 찾습니다")

		return nil, nil
	}

	// 죽은 서버는 여기서 자리를 비운다. 그러면 아래가 「없다」로 읽어 새로 띄운다(ADR-0092).
	if e.clientOf(server) == nil {
		state := e.serverState(server)

		if e.jobRunning(serverJobName(server), nil) {
			e.notify(server.Name + " 를 설치하는 중입니다")

			return nil, nil
		}

		if state.starting {
			e.notify(server.Name + " 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

			return nil, nil
		}

		if state.failed {
			return serverInstallConfirmMode(parent, e, server)
		}

		e.notify(server.Name + " 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return nil, e.startServer(server)
	}

	return nil, e.startDefinition()
}

// installServer 는 그 서버를 깔는 백그라운드 작업을 시작한다.
//
// **깔 명령은 표가 든다**(lsp/server.go 의 install). gopls 는 `go install` 이고 pyright 는
// `uv tool install` 인데, 둘 다 「명령 하나와 인자들」이라 표에 값으로 담긴다 — 서버마다
// 함수를 두면 깔는 자리가 서버 수만큼 늘고 하는 일은 같다.
//
// 첫 칸의 도구가 없으면 깔지 않고 알린다. `uv` 가 없는 기계에서 pyright 를 깔 길이 없고,
// 그것은 우리가 대신 정해 줄 일이 아니다.
func (e *editor) installServer(server *lsp.Server) tea.Cmd {
	install := server.Install()
	if len(install) == 0 {
		return nil
	}

	tool := install[0]
	if _, err := exec.LookPath(tool); err != nil {
		e.notifyError(errors.Errorf("%s 명령어를 찾을 수 없습니다", tool))

		return nil
	}

	name := serverJobName(server)

	return e.startJob(name, nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress)

		go func() {
			defer close(ch)

			cmd := exec.CommandContext(ctx, install[0], install[1:]...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errMsg := strings.TrimSpace(string(out))
				if errMsg == "" {
					errMsg = err.Error()
				}

				ch <- jobProgress{err: errors.New(errMsg)}

				return
			}

			ch <- jobProgress{
				summary: server.Name + " 설치 완료",
				apply: func(e *editor) {
					e.serverState(server).failed = false
					e.notify(server.Name + " 설치가 끝났습니다")
				},
			}
		}()

		return ch
	})
}

// startDefinition 은 커서 자리의 정의가 어디인지 묻는다. `\gd` 와 팔레트의 「정의로 가기」다.
func (e *editor) startDefinition() tea.Cmd {
	buf := e.activeBuffer()

	server, path, ok := serverPath(buf.path)
	if !ok {
		e.notify("언어 서버가 붙는 파일에서만 정의를 찾습니다")

		return nil
	}

	client := e.clientOf(server)
	if client == nil {
		if e.serverState(server).failed {
			e.notify(server.Name + " 가 없어 정의를 찾을 수 없습니다")

			return nil
		}

		e.notify(server.Name + " 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return e.startServer(server)
	}

	lines := buf.lines
	position := lsp.Position{
		Line:      buf.cursorLine,
		Character: lsp.UTF16Column(buf.lines[buf.cursorLine], buf.cursorCol),
	}

	e.notify("정의를 찾는 중입니다")

	return func() tea.Msg {
		// 묻기 직전에 전문으로 맞춘다. 저장하지 않은 편집도 이 한 번으로 서버에 닿으므로,
		// 답이 지금 화면에 보이는 글을 기준으로 나온다(ADR-0051).
		if err := client.SyncFull(path, lines); err != nil {
			return definitionMsg{err: err}
		}

		locations, err := client.Definition(path, position)

		return definitionMsg{locations: locations, err: err}
	}
}

// finishDefinition 은 답을 받아 그 자리로 간다.
//
// 하나면 곧바로 뛴다. 여럿이면 고르는 화면을 연다 — model 을 돌려주는 것이 그것이고,
// 작업 결과가 mode 를 바꾸는 자리는 이것 하나다(job.go 의 handleJob).
//
// 후보가 여럿인 것이 드물지 않다. 식별자 위에서는 늘 하나인데, **import 경로나 패키지 이름
// 위에서는 그 패키지의 파일 수만큼** 온다 — 파일마다 있는 `package` 절이 모두 후보다.
// `charm.land/bubbletea/v2` 에서 28 개였다(ADR-0051).
func (e *editor) finishDefinition(msg definitionMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		e.notifyError(msg.err)

		return nil, nil
	}

	switch len(msg.locations) {
	case 0:
		e.notify("정의를 찾지 못했습니다")

		return nil, nil
	case 1:
		// 뛰기 전 자리를 이력에 담는다. 판이 열리는 쪽은 판이 담는다(view-locations.go, ADR-0070).
		e.recordJump()

		cmd := e.jumpTo(msg.locations[0])
		e.arrive()

		return nil, cmd
	}

	return locationsMode(e, "정의 후보", msg.locations)
}

// jumpTo 는 자리 하나로 간다. 다른 파일이면 새 tab 이고 같은 파일이면 커서만 옮긴다 —
// openTab 이 이미 열린 파일을 그 tab 으로 데려가므로 여기서 가르지 않는다.
func (e *editor) jumpTo(target lsp.Location) tea.Cmd {
	cmd, err := e.openTab(target.Path())
	if err != nil {
		e.notifyError(err)

		return nil
	}

	e.moveToLocation(target)
	e.clearNotice()

	return tea.Batch(cmd, e.startServerForOpenFile(target.Path()))
}

// moveToLocation 은 서버가 준 자리로 커서를 옮긴다.
//
// 서버의 열은 UTF-16 이라 byte 로 바꿔야 한다(lsp/position.go). 줄이 파일 밖을 가리키면
// moveTo 가 안쪽으로 잡아 준다 — 서버가 보던 판과 지금 판이 어긋난 경우이고, 그때는
// 엉뚱한 줄보다 파일 끝이 낫다.
func (e *editor) moveToLocation(target lsp.Location) {
	buf := e.activeBuffer()

	line := target.Range.Start.Line
	column := 0
	if line >= 0 && line < len(buf.lines) {
		column = lsp.ByteColumn(buf.lines[line], target.Range.Start.Character)
	}

	buf.moveTo(line, column, e.contentWidth())
	buf.clampToNormal(e.contentWidth())
	e.scrollToCursor()
}

// shutdownServers 는 편집기를 끝내며 서버들을 내린다.
//
// ctx 가 끊기면 프로세스도 죽으므로 이것 없이도 남지 않는다. 그래도 부르는 것은 나가는
// 길이 하나로 보이게 하기 위해서다.
func (e *editor) shutdownServers() {
	for _, state := range e.servers {
		if state.client == nil {
			continue
		}

		state.client.Shutdown()
		state.client = nil
	}
}

// serverMaxRevives 는 한 세션에서 서버 하나를 되살려 보는 횟수다.
//
// 셋으로 둔 것은 이 수가 **「어쩌다 한 번 죽은 것」과 「뜨자마자 죽는 것」을 가르기** 때문이다.
// 앞쪽은 한 번 되살리면 돌아오고, 뒤쪽은 세 번이면 그것이 되살려서 나아지는 종류가 아님이
// 드러난다(ADR-0092).
const serverMaxRevives = 3

// clientOf 는 그 서버의 지금 쓸 수 있는 client 다. 죽어 있으면 자리를 비우고 nil 을 준다.
//
// **죽은 채로 남겨 두면 그 세션 내내 오류였다.** 자리를 비우는 문이 나가는 길(shutdownServers)
// 하나뿐이었고 startServer 는 client 가 있으면 곧바로 돌아가므로, 서버가 한 번 죽으면
// `\gd`·`\gr`·이름 바꾸기·자동완성이 끝까지 「연결이 닫혔다」였다(ADR-0092).
//
// **다시 띄우는 것은 여기가 아니다.** 여기는 자리를 비우고 세는 일만 한다 — Cmd 를 돌려줄 수
// 없는 자리에서도 불리기 때문이다. 띄우는 것은 syncServers 와, 사람이 묻는 자리들이 이미 하는
// 「없으면 띄운다」 길이다.
//
// **너무 자주 죽으면 실패한 것으로 둔다.** failed 를 세우면 startServer 가 그 자리를 보고
// 물러나므로 되살리는 고리가 끊긴다. 설치가 끝나면 그 자리가 다시 풀린다(installServer).
//
// **알림은 한 번뿐이다.** 알린 뒤 자리가 비므로 다음 부름은 첫 `if` 에서 끝난다.
func (e *editor) clientOf(server *lsp.Server) *lsp.Client {
	return e.clientNamed(server.Name)
}

// clientNamed 는 이름으로 찾은 client 다. clientOf 와 같은 일을 하고, 서버 줄이 아니라
// 이름만 들고 있는 자리(syncServers 의 순회) 가 이것을 쓴다.
func (e *editor) clientNamed(name string) *lsp.Client {
	state, ok := e.servers[name]
	if !ok || state.client == nil {
		return nil
	}

	if !state.client.Closed() {
		return state.client
	}

	state.client = nil
	e.recordServerDeath(name, state)

	return nil
}

// recordServerDeath 는 죽은 것을 셈에 넣고 사람에게 알린다.
//
// clientNamed 에서 갈라 둔 것은 **여기가 정하는 자리**이기 때문이다 — 몇 번까지 되살릴지와
// 무엇을 알릴지가 여기 있고, 저쪽은 「죽었나」를 묻는 한 줄이다. 그 한 줄은 lsp 가 시험하고
// (Closed) 이 판단은 여기서 시험한다.
func (e *editor) recordServerDeath(name string, state *languageServer) {
	state.deaths++

	if state.deaths > serverMaxRevives {
		state.failed = true
		e.notify(name + " 가 자꾸 멎습니다. 더 띄우지 않습니다")

		return
	}

	e.notify(name + " 가 멎었습니다. 다시 띄웁니다")
}

// runningClients 는 지금 떠 있는 서버들을 이름→client 로 뜬 것이다. 하나도 없으면 nil 이다.
//
// **백그라운드 작업에 넘길 사본이다.** 작업이 도는 동안 editor 를 읽을 수 없으므로 뜨는
// 자리가 Update 안이어야 한다(git.go 의 startGitRefresh). client 자체는 자기 잠금을 들고
// 있어서 작업 goroutine 에서 그대로 써도 된다(lsp/client.go 의 mu).
func (e *editor) runningClients() map[string]*lsp.Client {
	clients := map[string]*lsp.Client{}

	for name := range e.servers {
		if client := e.clientNamed(name); client != nil {
			clients[name] = client
		}
	}

	if len(clients) == 0 {
		return nil
	}

	return clients
}

// anyServerRunning 은 서버가 하나라도 떠 있는지다. 「250ms 뒤에 할 일이 있는가」를 묻는
// 자리가 쓴다(edit-tick.go).
func (e *editor) anyServerRunning() bool {
	for _, state := range e.servers {
		if state.client != nil {
			return true
		}
	}

	return false
}
