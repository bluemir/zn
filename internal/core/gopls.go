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

// gopls 와 이어지는 자리다. 하는 일은 셋이다 — 띄우기, 보고 있는 Go 파일을 서버와 맞추기,
// 커서 자리의 정의를 묻기(ADR-0051).
//
// 서버 쪽 이야기는 internal/lsp 가 안다. 여기 있는 것은 「언제 무엇을 맞추고 무엇을 묻는가」다.

// goplsReadyMsg 는 서버가 떴다는 것이다. 실패도 이 길로 온다.
type goplsReadyMsg struct {
	client *lsp.Client
	err    error
}

// definitionMsg 는 정의를 물은 답이다.
type definitionMsg struct {
	locations []lsp.Location
	err       error
}

// isGoFile 은 gopls 가 볼 파일인지다.
//
// syntax 의 언어 표(detect.go) 를 보지 않는다. 그 표는 lexer 를 고르는 표이고 이것은
// 「어느 언어 서버가 보는가」다 — 서버가 하나뿐인 지금 그 표에 칸을 더하면, 칸은 아홉 줄에
// 생기고 값은 한 줄에만 있게 된다.
func isGoFile(path string) bool {
	return strings.HasSuffix(path, ".go")
}

// goplsPath 는 그 buffer 를 서버에 알릴 때 쓸 절대 경로다. 알릴 것이 아니면 false 다.
//
// 절대 경로로 맞추는 것은 URI 가 절대 경로여야 하기 때문이고, 이름 없는 buffer(새 파일) 를
// 빼는 것은 알릴 파일이 없기 때문이다.
func goplsPath(path string) (string, bool) {
	if path == "" || !isGoFile(path) {
		return "", false
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	return absolute, true
}

// startGopls 는 서버를 띄운다. 이미 떴거나 뜨는 중이거나 한 번 실패했으면 아무것도 하지 않는다.
//
// 한 번 실패하면 다시 걸지 않는다. 실패하는 까닭은 대개 「깔려 있지 않다」 이고, 그것은
// 파일을 열 때마다 다시 시도해서 나아지는 종류가 아니다. 사용자가 깔고 나면 편집기를
// 다시 띄우거나 `\gd` 를 한 번 더 치는 것이 다시 거는 자리다.
func (e *editor) startGopls() tea.Cmd {
	if e.gopls != nil || e.goplsStarting || e.goplsFailed {
		return nil
	}

	e.goplsStarting = true
	ctx := e.rootContext()

	// 작업 폴더를 못 읽으면 띄우지 않는다. 서버가 볼 뿌리가 없으면 모듈을 찾을 수 없다.
	root, err := os.Getwd()
	if err != nil {
		e.goplsStarting = false
		e.goplsFailed = true

		return nil
	}

	e.goplsRoot = root

	return func() tea.Msg {
		client, err := lsp.Start(ctx, root)

		return goplsReadyMsg{client: client, err: err}
	}
}

// startGoplsForOpenFile 은 방금 연 파일이 Go 파일이면 서버를 미리 띄운다.
//
// 묻는 순간까지 기다리지 않는 것은 첫 요청이 서버가 모듈을 훑는 동안 1 초 남짓 걸리기
// 때문이다(잰 값이다). 파일을 열 때 시작해 두면 그 기다림이 `\gd` 앞으로 옮겨간다.
func (e *editor) startGoplsForOpenFile(path string) tea.Cmd {
	if !isGoFile(path) {
		return nil
	}

	return e.startGopls()
}

// startGoplsForOpenBuffers 는 이미 열려 있는 tab 중에 Go 파일이 있으면 서버를 띄운다.
//
// 시작할 때 CLI 인자로 받은 파일들이 이 길이다. 그것들은 openTab 을 지나지 않아서
// 파일을 여는 자리의 갈고리에 걸리지 않는다(core.go 의 openBuffers).
func (e *editor) startGoplsForOpenBuffers() tea.Cmd {
	for i := range e.buffers {
		if _, ok := goplsPath(e.buffers[i].path); ok {
			return e.startGopls()
		}
	}

	return nil
}

// finishGopls 는 서버가 떴거나 실패한 것을 받는다.
func (e *editor) finishGopls(msg goplsReadyMsg) tea.Cmd {
	e.goplsStarting = false

	if msg.err != nil {
		e.goplsFailed = true
		e.notifyError(msg.err)

		return nil
	}

	e.gopls = msg.client

	// 이미 열려 있는 Go 파일들을 알린다. 지금 보고 있는 것만이 아니다 — tab 여럿이
	// 열린 채로 서버가 떴을 수 있다.
	//
	// 진단을 기다리는 고리도 여기서 시작한다. 서버가 떠 있는 동안만 도는 고리이고, 시작하는
	// 자리가 여기 하나다 — 알리는 자리(syncGopls) 와 같이 두면 tick 마다 고리가 하나씩
	// 늘어난다(ADR-0086).
	//
	// 문법 토큰은 여기서 곧바로 묻지 않고 tick 을 하나 건다. 지금은 파일을 이제 막 알리는
	// 참이라 서버가 아직 그 파일을 모르고, 250ms 뒤 그 tick 이 묻는다. 이것이 없으면 파일을
	// 열어 두고 손을 대지 않는 동안에는 서버의 색이 오지 않는다(semantic.go, ADR-0103).
	return tea.Batch(e.syncGopls(), waitDiagnostics(e.gopls), e.scheduleEditTick())
}

// syncGopls 는 열려 있는 Go 파일들을 서버와 맞춘다.
//
// **무엇이 바뀌었는지 적어 두지 않는다.** 지금 열려 있는 목록과 서버가 아는 목록을 견주어
// 새로 연 것은 알리고, 닫은 것은 닫고, 남은 것은 차이만 보낸다. 편집·열기·닫기 자리마다
// 갈고리를 걸지 않아도 되고, 어느 자리를 빠뜨려도 다음 tick 에 제자리를 찾는다.
//
// 보내는 것은 goroutine 에서 한다. 남의 프로세스의 파이프에 쓰는 일이라, 그 프로세스가
// 잠깐 읽지 않으면 화면을 그리는 쪽이 같이 멈춘다.
func (e *editor) syncGopls() tea.Cmd {
	// **죽은 서버를 여기서 알아채고 되살린다.** tick 마다 지나는 자리라 사람이 무엇을 묻기
	// 전에 돌아온다(ADR-0092).
	//
	// **죽은 것과 뜬 적 없는 것을 가른다.** 죽기 전에 미리 재 두는 것이 그 일이다 —
	// goplsClient 가 자리를 비운 뒤에는 둘이 같은 모습이 된다. 뜬 적이 없는데 여기서
	// 띄우면 Go 파일을 열지도 않은 세션에서 서버가 뜬다. 처음 띄우는 자리는 파일을 열 때다
	// (startGoplsForOpenFile).
	//
	// 되살리기를 멈출 자리는 goplsClient 가 든다. 너무 자주 죽으면 goplsFailed 를 세우고,
	// 그러면 아래 startGopls 가 곧바로 물러난다.
	died := e.gopls != nil && e.gopls.Closed()

	client := e.goplsClient()
	if client == nil {
		if died {
			return e.startGopls()
		}

		return nil
	}

	// 줄 목록은 그대로 넘겨도 된다. 편집은 겉껍데기를 새로 만들어 갈아끼우므로(edit.go 의
	// replaceLines) 지금 넘긴 것은 이 순간의 모습으로 굳는다.
	type snapshot struct {
		path  string
		lines [][]byte
	}

	open := make([]snapshot, 0, len(e.buffers))
	for i := range e.buffers {
		path, ok := goplsPath(e.buffers[i].path)
		if !ok {
			continue
		}

		open = append(open, snapshot{path: path, lines: e.buffers[i].lines})
	}

	return func() tea.Msg {
		alive := map[string]bool{}

		for _, doc := range open {
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
	}
}

const goplsJobName = "gopls 설치"

// gotoDefinition 은 커서 자리의 정의로 가거나, gopls 가 없으면 설치를 묻는다.
func gotoDefinition(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	// 볼 파일이 없으면 커서도 없다. `\gd` 와 팔레트가 함께 지나는 자리라 여기서 막는다
	// (ADR-0064).
	if e.refuseNoBuffer() {
		return nil, nil
	}

	buf := e.activeBuffer()

	_, ok := goplsPath(buf.path)
	if !ok {
		e.notify("Go 파일에서만 정의를 찾습니다")

		return nil, nil
	}

	// 죽은 서버는 여기서 자리를 비운다. 그러면 아래가 「없다」로 읽어 새로 띄운다(ADR-0092).
	if e.goplsClient() == nil {
		if e.jobRunning(goplsJobName, nil) {
			e.notify("gopls 를 설치하는 중입니다")

			return nil, nil
		}

		if e.goplsStarting {
			e.notify("gopls 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

			return nil, nil
		}

		if e.goplsFailed {
			return goplsInstallConfirmMode(parent, e)
		}

		e.notify("gopls 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return nil, e.startGopls()
	}

	return nil, e.startDefinition()
}

// installGopls 는 go install 로 gopls 를 설치하는 백그라운드 작업을 시작한다.
func (e *editor) installGopls() tea.Cmd {
	if _, err := exec.LookPath("go"); err != nil {
		e.notifyError(errors.New("go 명령어를 찾을 수 없습니다"))

		return nil
	}

	return e.startJob(goplsJobName, nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress)

		go func() {
			defer close(ch)

			cmd := exec.CommandContext(ctx, "go", "install", "golang.org/x/tools/gopls@latest")
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
				summary: "gopls 설치 완료",
				apply: func(e *editor) {
					e.goplsFailed = false
					e.notify("gopls 설치가 끝났습니다")
				},
			}
		}()

		return ch
	})
}

// startDefinition 은 커서 자리의 정의가 어디인지 묻는다. `\gd` 와 팔레트의 「정의로 가기」다.
func (e *editor) startDefinition() tea.Cmd {
	buf := e.activeBuffer()

	path, ok := goplsPath(buf.path)
	if !ok {
		e.notify("Go 파일에서만 정의를 찾습니다")

		return nil
	}

	if e.goplsClient() == nil {
		if e.goplsFailed {
			e.notify("gopls 가 없어 정의를 찾을 수 없습니다")

			return nil
		}

		e.notify("gopls 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return e.startGopls()
	}

	client := e.gopls
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

	return tea.Batch(cmd, e.startGoplsForOpenFile(target.Path()))
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

// shutdownGopls 는 편집기를 끝내며 서버를 내린다.
//
// ctx 가 끊기면 프로세스도 죽으므로 이것 없이도 남지 않는다. 그래도 부르는 것은 나가는
// 길이 하나로 보이게 하기 위해서다.
func (e *editor) shutdownGopls() {
	if e.gopls == nil {
		return
	}

	e.gopls.Shutdown()
	e.gopls = nil
}

// goplsMaxRevives 는 한 세션에서 되살려 보는 횟수다.
//
// 셋으로 둔 것은 이 수가 **「어쩌다 한 번 죽은 것」과 「뜨자마자 죽는 것」을 가르기** 때문이다.
// 앞쪽은 한 번 되살리면 돌아오고, 뒤쪽은 세 번이면 그것이 되살려서 나아지는 종류가 아님이
// 드러난다(ADR-0092).
const goplsMaxRevives = 3

// goplsClient 는 지금 쓸 수 있는 서버다. 죽어 있으면 자리를 비우고 nil 을 준다.
//
// **죽은 채로 남겨 두면 그 세션 내내 오류였다.** 자리를 비우는 문이 나가는 길(shutdownGopls)
// 하나뿐이었고 startGopls 는 `e.gopls != nil` 이면 곧바로 돌아가므로, 서버가 한 번 죽으면
// `\gd`·`\gr`·이름 바꾸기·자동완성이 끝까지 「연결이 닫혔다」였다(ADR-0092).
//
// **다시 띄우는 것은 여기가 아니다.** 여기는 자리를 비우고 세는 일만 한다 — Cmd 를 돌려줄 수
// 없는 자리에서도 불리기 때문이다. 띄우는 것은 syncGopls 와, 사람이 묻는 자리들이 이미 하는
// `e.gopls == nil` 길이다.
//
// **너무 자주 죽으면 실패한 것으로 둔다.** goplsFailed 를 세우면 startGopls 가 그 자리를 보고
// 물러나므로 되살리는 고리가 끊긴다. 설치가 끝나면 그 자리가 다시 풀린다(installGopls).
//
// **알림은 한 번뿐이다.** 알린 뒤 자리가 비므로 다음 부름은 첫 `if` 에서 끝난다.
func (e *editor) goplsClient() *lsp.Client {
	if e.gopls == nil || !e.gopls.Closed() {
		return e.gopls
	}

	e.gopls = nil
	e.recordGoplsDeath()

	return nil
}

// recordGoplsDeath 는 죽은 것을 셈에 넣고 사람에게 알린다.
//
// goplsClient 에서 갈라 둔 것은 **여기가 정하는 자리**이기 때문이다 — 몇 번까지 되살릴지와
// 무엇을 알릴지가 여기 있고, 저쪽은 「죽었나」를 묻는 한 줄이다. 그 한 줄은 lsp 가 시험하고
// (Closed) 이 판단은 여기서 시험한다.
func (e *editor) recordGoplsDeath() {
	e.goplsDeaths++

	if e.goplsDeaths > goplsMaxRevives {
		e.goplsFailed = true
		e.notify("gopls 가 자꾸 멎습니다. 더 띄우지 않습니다")

		return
	}

	e.notify("gopls 가 멎었습니다. 다시 띄웁니다")
}
