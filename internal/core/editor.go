package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/lsp"
)

// editor 는 mode 가 바뀌어도 유지되는 상태다.
//
// mode 는 enum 이 아니라 화면 model 을 갈아끼워서 나타낸다(ADR-0002).
// mode 별 model 이 이것을 포인터로 embed 하고, 전환할 때 그 포인터를 그대로 넘긴다.
// 편집기가 도는 동안 이것은 하나뿐이라 어느 mode 에서 고쳐도 다음 화면이 같은 것을 본다(ADR-0026).
type editor struct {
	buffers []Buffer
	active  int

	// tabScroll 은 tabline 에 처음으로 그리는 tab 의 index 다. tab 이 편집 영역 너비보다
	// 많아지면 활성 tab 이 보이도록 여기가 밀린다. 가려진 것은 양끝 표시가 알린다(ADR-0029).
	tabScroll int

	// draggingTab 은 tabline 에서 시작한 드래그 중인지다. tab 을 끌어서 옮기는 자리다(ADR-0090).
	//
	// **시작한 자리를 알아야 한다.** tabline 은 한 행이고 편집 영역이 바로 아래 한 칸이라,
	// tab 을 가로로 끄는 손이 아래로 새면 지금 자리만 보는 코드는 글을 고르기 시작한다.
	//
	// 누를 때 정하고 놓을 때 지운다. 드래그는 늘 누르기로 시작하므로 값이 어긋나도
	// 다음 누르기가 제자리를 찾는다(view-editor-normal.go).
	draggingTab bool

	sidebar sidebar

	// search 는 마지막 검색이다. `n` 은 tab 을 옮겨서도 같은 것을 찾으므로 Buffer 가 아니라 여기 있다.
	search searchState

	// registers 는 지우거나 복사한 내용이 담기는 자리다. 붙여넣기가 tab 을 넘어 되어야 하므로
	// 여기 있다. vim 의 register 도 buffer 밖이다. `d`·`c`·`y` 가 채우고 `p`·`P` 가 읽는다(ADR-0017).
	//
	// 무명·숫자·문자 셋을 한 덩이로 든 것은 담는 규칙이 셋에 걸쳐 있어서다 — `y` 하나가
	// 무명과 `"0` 을 같이 채운다. 어느 자리에 무엇이 들어가는지는 register.go 가 혼자
	// 안다(ADR-0058).
	registers registerSet

	// git 은 statusBar 오른쪽에 찍는 저장소 상태다. 화면을 그릴 때 읽지 않고 여기에 들고 있다가
	// 갱신 작업과 저장·파일 열기 직후에 다시 읽는다(ADR-0009, ADR-0030).
	git gitStatus

	// gitTickScheduled·fileTickScheduled 는 다음 tick 이 이미 걸려 있는지다.
	//
	// 갱신이 끝날 때마다 cooldown 을 새로 걸면 `:w` 나 포커스 복귀 한 번에 고리가 둘로
	// 갈라진다. 예약을 하나로 묶어두는 자리가 여기다(ADR-0043, ADR-0044).
	//
	// 셋이 되면 이름→cooldown 표로 옮긴다. 둘까지는 bool 두 개가 읽기 쉽다.
	gitTickScheduled  bool
	fileTickScheduled bool

	// gopls 는 도는 언어 서버다. Go 파일을 처음 열 때 뜨고 그 뒤로 하나뿐이다(ADR-0051).
	//
	// goplsStarting 은 뜨는 중인지고 goplsFailed 는 한 번 실패했는지다. 실패한 뒤에는
	// 다시 걸지 않는다 — 까닭이 대개 「깔려 있지 않다」 라서 파일을 열 때마다 다시 시도해도
	// 나아지지 않고, 그때마다 오류 문구가 화면 아래를 차지한다.
	gopls         *lsp.Client
	goplsStarting bool
	goplsFailed   bool

	// goplsDeaths 는 서버가 뜬 뒤에 죽은 횟수다. **되살리기를 멈추는 자리가 있어야 해서
	// 센다** — 뜨자마자 죽는 서버를 끝없이 되살리면 tick 마다 프로세스가 하나씩 뜬다
	// (goplsClient, ADR-0092).
	goplsDeaths int

	// goplsRoot 는 서버에게 준 뿌리, 곧 편집기를 연 자리다. 서버가 참조를 찾는 범위가
	// 여기까지라, 이름 바꾸기가 이 밖의 파일을 거절하는 근거가 된다(rename.go, ADR-0067).
	goplsRoot string

	// completion 은 insert 에서 떠 있는 자동완성 목록이다. 비어 있으면 닫힌 것이다.
	//
	// completionSeq 는 물어본 차례다. 답이 그것을 싣고 와서, 그 사이에 다시 물었으면 낡은
	// 답으로 버려진다. completionAsking 은 답을 기다리는 중인지고 — 글자마다 묻는 자리라
	// 겹쳐 묻지 않으려고 둔다(completion.go, ADR-0066).
	completion       completion
	completionSeq    int
	completionAsking bool

	// saveHooks 는 폴더마다 찾아 둔 저장 포매터다. 없다고 판정한 것은 nil 로 담긴다 —
	// 「아직 안 찾아봤다」와 「찾아봤는데 없다」가 갈려야 저장마다 다시 뒤지지 않는다.
	//
	// goimportsDeclined 는 설치를 거절했는지다. 거절한 뒤로는 묻지 않는다 — 저장은 손이
	// 가장 자주 가는 자리라, 물음이 되풀이되면 그것이 곧 방해다(save-hook.go, ADR-0065).
	saveHooks         map[string]*saveHook
	goimportsDeclined bool

	// lspTickScheduled 는 서버와 맞출 예약이 이미 걸려 있는지다. git·파일 검사와 같은 자리다
	// (ADR-0043). 이것이 타이핑을 모아 주는 자리이기도 하다 — 예약이 하나라 키를 여러 번 쳐도
	// 보내는 것은 250ms 뒤 한 번이다.
	lspTickScheduled bool

	// watch 는 디스크를 보고 있는 감시기다. 없으면 붙이지 못한 것이고, 그때도 주기 검사는
	// 그대로 돌아서 정확성은 남고 반응만 느려진다(watch.go, ADR-0093).
	watch *watcher

	// ctx 는 편집기의 수명이다. core.Run 이 받은 것을 그대로 든다.
	// 백그라운드 작업이 여기서 갈라져 나오므로 편집기를 끝내면 도는 것이 전부 정리된다(ADR-0027).
	ctx context.Context

	// jobs 는 백그라운드에서 도는 작업들이고 finished 는 최근에 끝난 것들이다.
	// statusBar 는 도는 것만 보고 `:jobs` 목록이 둘 다 본다.
	// 결과는 여기가 아니라 종류마다 자기 자리에 쌓인다(job.go).
	jobs     []job
	finished []job

	// notice 는 지금 아래 줄에 떠 있는 알림이다. 다음 키를 누르면 사라진다.
	//
	// mode 가 아니라 여기 있는 것은 백그라운드 작업의 실패가 어느 mode 에서든 도착하기 때문이다.
	// 아래 줄에 그리는 것은 normal·insert·트리뿐이다 — 명령줄과 검색은 그 줄을 자기가 쓴다.
	//
	// **여기에 직접 대입하지 않는다.** 쓰는 자리는 notice.go 의 notify·notifyError·
	// notifyFailure·clearNotice 넷뿐이다. 직접 넣으면 그 알림이 기록에서 빠진다(ADR-0053).
	notice string

	// notices 는 지금까지 뜬 알림 전부다. `:messages` 가 이것을 보여준다.
	//
	// 상한이 없다. 왜 두지 않는지는 notice.go 의 record 에 적었다.
	notices []notice

	// tipIndex 는 다음에 보여줄 tip 의 자리다. 문장은 internal/assets 에 살고
	// 고르는 규칙은 tip.go 다.
	//
	// **시계가 시간이 아니라 알림이다.** 아래 줄이 알림에 덮이는 그 순간이 문장을 갈 수 있는
	// 유일한 때다 — 타이머로 돌리면 가만히 보고 있는 화면의 글자가 저 혼자 움직인다(ADR-0061).
	// 미는 자리는 notice.go 의 record 하나이고, 목록 길이로 나누는 것은 읽는 쪽(fitTip) 이 한다.
	//
	// **한 칸씩 밀지 않는다.** 여는 자리와 미는 자리가 둘 다 무작위라(core.Run, tip.go 의
	// nextTip) 목록의 차례가 화면에 드러나지 않는다. 그리는 자리에서는 굴리지 않는다 —
	// 프레임마다 문장이 바뀐다.
	tipIndex int

	// files 는 팔레트가 고르는 파일 목록이다. 인덱싱 작업이 채운다.
	// 팔레트를 닫아도 남는다 — 인덱싱은 팔레트보다 오래 살고, 다시 열면 모아둔 것부터 보인다.
	files []string

	// grep 은 여러 파일 검색의 결과다. 검색 작업이 채운다.
	//
	// 결과 화면을 닫아도 남는다 — `files` 와 같은 까닭이고, 검색은 화면보다 오래 살아서
	// 닫았다 다시 열면 모아둔 것부터 보인다(ADR-0077).
	grep grepResult

	// boxChars 는 테두리·구분선에 쓸 글자다. 시작할 때 터미널을 재서 core.Run 이 넣어주고
	// 그 뒤로 바뀌지 않는다(ADR-0028).
	//
	// **잰 폭은 따로 들고 있지 않는다.** unicode 묶음은 Ambiguous 를 한 칸으로 그린다고
	// 확인된 터미널에서만 고르므로 이 묶음이 곧 그 답이고, 폭 계산은 시작할 때 터미널에
	// 맞춰 둔 눈금이 이미 답한다(ADR-0072).
	boxChars boxSet

	// symbols 는 특수문자 drawer 가 고르는 글자 목록이다. 여는 순간 큐레이션 표로 채우고
	// 훑는 작업이 나머지를 이어 붙인다(symbol.go).
	//
	// symbolsIndexed 는 그 훑기가 끝났는지다. 유니코드 표는 도중에 바뀌지 않으므로
	// 한 번 끝나면 다시 훑지 않는다 — 열 때마다 다시 읽는 파일 목록과 다른 점이다
	// (ADR-0011, ADR-0056).
	symbols        []assets.Symbol
	symbolsIndexed bool

	// jumps 는 뛰어다닌 자리의 이력이다. `ctrl+o`·`ctrl+i` 가 이것을 오간다.
	//
	// **tab 마다가 아니라 편집기 하나가 든다.** 되돌아오는 것이 파일을 넘어야 뜻이 있고,
	// tab 을 닫고 돌아오는 길도 있어야 한다(jumplist.go, ADR-0070).
	jumps jumpList

	// logs 는 방문 기록이다. jumps 와 달리 앞쪽을 버리지 않고 최근이 앞이다(jumplog.go, ADR-0074).
	logs jumpLog

	// drawerHeight 는 하단 drawer 가 편집 영역에서 가져간 행 수다. 0 이면 닫힌 것이다.
	// 여는 mode 가 세우고 나갈 때 되돌린다(docs/spec.md, ADR-0056).
	drawerHeight int

	width  int
	height int
}

// hasTab 은 열린 tab 이 있는지다. 없으면 빈 화면이다(ADR-0064).
//
// **buffer 를 만지는 공용 코드가 이것을 먼저 본다.** 그리기·mouse·주기 작업처럼 mode 를
// 가리지 않고 오는 자리가 그렇다 — 어느 mode 에서 왔는지로는 tab 이 있는지 알 수 없다.
func (e editor) hasTab() bool {
	return len(e.buffers) > 0
}

// refuseNoBuffer 는 tab 이 없으면 알리고 참을 준다. buffer 가 있어야 하는 동작이 첫 줄에서
// 부른다 — `:w` 처럼 명령줄로 오는 것과 팔레트 항목이 그렇다.
//
// refuseReadOnly 와 같은 꼴이다(readonly.go). **편집 키가 조용한 것과 갈리는 자리다** —
// 키는 눌러 본 것이지만 이쪽은 이름을 대고 고른 것이라, 아무 일도 안 나면 편집기가 먹지
// 않는 것으로 읽힌다(ADR-0064).
func (e *editor) refuseNoBuffer() bool {
	if e.hasTab() {
		return false
	}

	e.notify("열린 파일이 없습니다")

	return true
}

// activeBuffer 는 활성 activeBuffer 를 가리킨다.
// 값이 아니라 slice 요소를 가리켜야 커서 이동과 편집이 제자리에 남는다.
//
// **tab 이 없을 때 부르면 터진다.** nil 을 돌려주면 부르는 자리 일흔 곳이 다 nil 검사를
// 달아야 하고, 빠뜨린 한 자리는 터지는 대신 조용히 틀린다. tab 이 없는 동안 편집 동작이
// 이 자리에 오지 않는 것으로 지킨다 — 판을 갈아끼우는 것이 mode 라는 자리가 그것을
// 보증한다(ADR-0002, ADR-0064).
func (e *editor) activeBuffer() *Buffer {
	return &e.buffers[e.active]
}

// bufferByPath 는 그 경로로 열어둔 buffer 를 준다. 없으면 nil 이다.
//
// 백그라운드 검사가 결과를 넣을 자리를 찾는 데 쓴다(ADR-0044). index 로 기억해 두면 그 사이
// tab 이 닫혀서 다른 파일을 가리킬 수 있다.
func (e *editor) bufferByPath(path string) *Buffer {
	if path == "" {
		return nil
	}

	for i := range e.buffers {
		if e.buffers[i].path == path {
			return &e.buffers[i]
		}
	}

	return nil
}

// scrollToCursor 는 활성 buffer 를 지금 화면에 맞춘다. 커서가 화면 안에 들어오게 하고,
// 폭이 달라졌으면 줄바꿈도 다시 잡는다.
//
// 편집·이동 동작이 끝에 이것을 하고, 창 크기가 바뀌거나 sidebar 를 여닫거나 보는 buffer 가
// 바뀐 뒤에도 부른다 — 그 buffer 는 지금 폭을 본 적이 없을 수 있다.
func (e *editor) scrollToCursor() {
	// tab 이 없으면 맞출 커서가 없다. 창 크기가 바뀌는 길(resize) 이 어느 mode 에서든
	// 이리로 오므로 목구멍인 여기서 막는다(ADR-0064).
	if !e.hasTab() {
		return
	}

	e.activeBuffer().scrollTo(e.contentWidth(), e.textHeight())
}

func (e *editor) resize(msg tea.WindowSizeMsg) {
	e.width = msg.Width
	e.height = msg.Height
	e.scrollToCursor()
	e.scrollTabsTo()
}

// nextTab, prevTab 은 활성 tab 을 옮긴다. 양끝에서 둘러 간다. vim 의 gt/gT 와 같다.
//
// 트리가 그 파일 자리를 아직 읽지 않았으면 읽는 작업이 시작되므로 Cmd 가 나온다(ADR-0032).
func (e *editor) nextTab() tea.Cmd {
	// tab 이 없으면 옮길 자리가 없다. 나누는 수가 0 이라 셈부터 되지 않는다(ADR-0064).
	if !e.hasTab() {
		return nil
	}

	e.active = (e.active + 1) % len(e.buffers)
	e.scrollTabsTo()

	return e.revealInSidebar(e.activeBuffer().path)
}
func (e *editor) prevTab() tea.Cmd {
	if !e.hasTab() {
		return nil
	}

	e.active = (e.active - 1 + len(e.buffers)) % len(e.buffers)
	e.scrollTabsTo()

	return e.revealInSidebar(e.activeBuffer().path)
}

// activePath 는 지금 보고 있는 파일의 절대 경로다. sidebar 가 그 행을 굵게 그린다(ADR-0022).
//
// 트리 항목은 절대 경로이고 CLI 로 연 파일은 상대 경로다. tabOf·reveal 과 같은 이유로 맞춰 둔다.
// 이름 없는 buffer 는 빈 문자열이라 어느 행과도 맞지 않는다.
func (e editor) activePath() string {
	// tab 이 없으면 보고 있는 파일도 없다. 어느 트리 행도 굵지 않다(ADR-0064).
	if !e.hasTab() {
		return ""
	}

	path := e.buffers[e.active].path
	if path == "" {
		return ""
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	return abs
}

// revealInSidebar 는 트리를 그 파일 자리까지 펼치고 고른 뒤 화면 안으로 끌어온다.
// 보고 있는 파일이 바뀌는 모든 길이 이것을 부른다(ADR-0019).
//
// sidebar 를 열지는 않는다. `:tree` 로 닫혀 있으면 트리가 아예 없어서 아무 일도 하지 않고,
// 좁은 화면이라 감춰진 상태면 트리만 펼쳐 둔다 — 화면이 넓어지면 그 자리가 보인다.
//
// 이름 없는 buffer 는 경로가 빈 문자열이라 갈 자리를 세우지 못하고 고른 자리가 그대로 남는다.
//
// 자리까지 걸어가는 도중에 아직 읽지 않은 디렉터리를 만나면 그것을 읽는 작업이 시작된다.
// 그 Cmd 를 흘리면 트리가 따라오지 못하므로 부르는 쪽이 끝까지 들고 나가야 한다(ADR-0032).
func (e *editor) revealInSidebar(path string) tea.Cmd {
	if !e.sidebar.setRevealTarget(path) {
		return nil
	}

	return e.continueReveal()
}

// scrollSidebar 는 고른 항목을 화면 안으로 데려온다.
//
// 화면 크기를 아직 모르는 동안에는(시작 직후, WindowSizeMsg 앞) 아무것도 하지 않는다.
// scrollTo 는 height 가 0 이면 고른 자리를 0 으로 되돌리므로, 그대로 부르면 CLI 로 연 파일
// 자리를 펼쳐 두고도 뿌리를 고른 채로 시작한다. 데려오는 것은 sidebar 로 포커스가 올 때다.
func (e *editor) scrollSidebar() {
	if height := e.sidebarHeight(); height >= 1 {
		e.sidebar.scrollTo(height)
	}
}

// newTab 은 이름 없는 빈 tab 을 활성 tab 바로 뒤에 끼우고 그리로 옮긴다.
// vim 의 :tabnew 와 같다. 맨 뒤가 아니라 보고 있던 것 옆에 생겨야 방금 만든 것을 찾기 쉽다.
func (e *editor) newTab() {
	e.buffers = slices.Insert(e.buffers, e.active+1, newEmptyBuffer(""))
	e.active++
	e.scrollTabsTo()
}

// openTab 은 파일을 tab 으로 연다. 이미 열려 있으면 새로 열지 않고 그 tab 으로 옮긴다.
//
// 같은 파일을 두 tab 에 열면 각각 독립된 Buffer 가 되어, 한쪽에서 저장하는 순간 다른 쪽의
// 편집이 사라진다. 나중 저장은 바깥 변경으로 잡혀 막히지만(ADR-0015) 두 편집을 합칠 길은
// 없다. 그래서 여는 것보다 찾는 것이 먼저다.
//
// git 갱신은 여기서 하지 않는다. 언제 다시 읽을지는 정책이라 부르는 쪽이 `startGitRefresh` 을
// 같이 발행한다(ADR-0030).
// 트리를 그 파일 자리로 데려가는 작업이 시작되면 Cmd 가 나온다(ADR-0032).
func (e *editor) openTab(path string) (tea.Cmd, error) {
	if index, ok := e.tabOf(path); ok {
		e.active = index
	} else {
		buf, err := OpenBuffer(path)
		if err != nil {
			return nil, err
		}

		e.buffers = slices.Insert(e.buffers, e.active+1, buf)
		e.active++
	}

	e.scrollTabsTo()

	// Go 파일을 열었으면 언어 서버를 미리 띄운다. 첫 요청이 서버가 모듈을 훑는 동안 1 초
	// 남짓 걸려서, 파일을 여는 자리에서 시작해 두면 그 기다림이 `\gd` 앞으로 옮겨간다(ADR-0051).
	return tea.Batch(e.revealInSidebar(path), e.startGoplsForOpenFile(path)), nil
}

// replaceTab 은 활성 tab 의 내용을 그 파일로 갈아끼운다. tab 수는 그대로다. `:e <파일>` 이 쓴다.
//
// 이미 다른 tab 에 열려 있으면 갈아끼우지 않고 그 tab 으로 옮긴다. openTab 과 같은 이유다 —
// 같은 파일에 Buffer 가 둘이면 한쪽 저장이 다른 쪽 편집을 덮어쓴다. 옮겨가기만 하는 길이라
// 지금 tab 의 편집도 그대로 남는다(ADR-0021).
//
// 갈아끼우는 쪽은 지금 tab 의 저장하지 않은 변경을 잃는다. 물을지 말지는 부르는 쪽이 정한다 —
// 여기까지 왔으면 이미 정해진 것이다. Reload 와 같은 나눔이다.
//
// git 갱신은 openTab 과 같이 부르는 쪽의 몫이다.
func (e *editor) replaceTab(path string) (tea.Cmd, error) {
	// tab 이 없으면 갈아끼울 것이 없으므로 새로 여는 것이다. 잃을 것도 없어서 부르는 쪽이
	// 확인창을 띄울 일도 없다(ADR-0064).
	if !e.hasTab() {
		return e.openTab(path)
	}

	if index, ok := e.tabOf(path); ok {
		e.active = index
		e.scrollTabsTo()

		return e.revealInSidebar(path), nil
	}

	buf, err := OpenBuffer(path)
	if err != nil {
		return nil, err
	}

	e.buffers[e.active] = buf

	return tea.Batch(e.revealInSidebar(path), e.startGoplsForOpenFile(path)), nil
}

// tabOf 는 그 파일을 이미 열어둔 tab 을 찾는다.
func (e editor) tabOf(path string) (int, bool) {
	for i, buf := range e.buffers {
		// 이름 없는 buffer 는 어느 파일도 아니다.
		if buf.path == "" {
			continue
		}
		if samePath(buf.path, path) {
			return i, true
		}
	}

	return 0, false
}

// openTabUnder 는 그 자리에 딸린 파일 중 tab 에 열린 것을 준다. 없으면 ok 가 false 다.
//
// 트리에서 지우기가 이것을 먼저 묻는다. 열려 있는 파일을 지우면 buffer 와 디스크가 어긋난
// 채로 남아서, `:w` 한 번에 지운 것이 되살아나거나 tabline 이 없는 파일을 가리킨다.
// 그 상태를 만들지 않고 먼저 닫으라고 돌려보낸다(ADR-0054).
//
// isDir 이면 그 아래 전부를 본다 — 디렉터리를 통째로 지우는 것은 안의 파일을 지우는 것이다.
func (e editor) openTabUnder(path string, isDir bool) (string, bool) {
	for _, buf := range e.buffers {
		// 이름 없는 buffer 는 어느 파일도 아니다.
		if buf.path == "" {
			continue
		}

		if samePath(buf.path, path) {
			return buf.path, true
		}
		if !isDir {
			continue
		}

		// 디렉터리 안인지는 정규화한 절대 경로로 본다. CLI 로 연 파일은 상대 경로다.
		abs, err := filepath.Abs(buf.path)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, path+string(filepath.Separator)) {
			return buf.path, true
		}
	}

	return "", false
}

// renameBuffers 는 이름이 바뀐 파일을 보고 있는 tab 들의 경로를 새 이름으로 맞춘다.
//
// 트리에서 이름을 바꾸면 그 파일을 열어둔 tab 이 따라간다(ADR-0054). 경로만 갈아끼우므로
// 편집하던 내용과 커서 자리는 그대로다 — 저장하지 않은 변경도 새 이름으로 저장된다.
//
// 파일 내용도 mtime 도 그대로라 바깥 변경 검사(diskSize·diskTime·diskHash) 는 손대지 않는다.
// 문법 강조는 `syntaxCache.path` 가 buffer 의 경로와 어긋난 것을 보고 스스로 다시 고른다.
//
// isDir 이면 그 아래 전부의 앞부분을 갈아끼운다 — 디렉터리를 옮기면 안의 파일도 옮겨진 것이다.
func (e *editor) renameBuffers(from, to string, isDir bool) {
	for i := range e.buffers {
		if e.buffers[i].path == "" {
			continue
		}

		// CLI 로 연 파일은 상대 경로다. 트리가 주는 것은 절대 경로라 맞춰 둔다.
		abs, err := filepath.Abs(e.buffers[i].path)
		if err != nil {
			continue
		}

		switch {
		case abs == from:
			e.buffers[i].path = to
		case isDir && strings.HasPrefix(abs, from+string(filepath.Separator)):
			e.buffers[i].path = to + abs[len(from):]
		}
	}
}

// samePath 는 두 경로가 같은 파일을 가리키는지다.
//
// 정규화해서 비교한다. CLI 로 연 파일은 상대 경로(`internal/core/editor.go`)이고
// 트리는 절대 경로를 주므로, 글자 그대로 비교하면 같은 파일을 못 알아본다.
// 그러면 tabOf 가 중복 Buffer 를 만들고 `:w <파일>` 이 제자리 저장을 사본 쓰기로 본다.
func samePath(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}

	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}

	return absA == absB
}

// closeTab 은 활성 tab 을 닫는다. 닫을 tab 이 없으면 false 를 준다.
func (e *editor) closeTab() bool {
	return e.closeTabAt(e.active)
}

// closeTabAt 은 index 자리의 tab 을 닫는다. 그 자리에 tab 이 없으면 false 를 준다.
//
// **마지막 tab 도 닫는다.** 닫으면 tab 이 없는 상태가 되고 편집 영역이 빈 화면으로 바뀐다.
// 예전에는 마지막 하나를 거부해서 `:q` 가 그것을 종료로 번역했다(ADR-0064).
//
// **보고 있던 파일은 그대로 본다.** 닫은 것이 그 왼쪽이면 번호만 하나 당겨진다 — tabline
// 우클릭이 남의 tab 을 닫는 자리가 이것을 쓴다(ADR-0060).
// 보고 있던 것을 닫았으면 그 자리에 드러나는 tab 을 보고, 오른쪽 끝이었으면 왼쪽으로 간다.
// 마지막 하나였으면 활성 자리가 -1 이 된다 — 볼 tab 이 없다는 뜻이다.
func (e *editor) closeTabAt(index int) bool {
	if index < 0 || index >= len(e.buffers) {
		return false
	}

	e.buffers = slices.Delete(e.buffers, index, index+1)

	switch {
	case index < e.active:
		// 앞이 하나 빠졌으니 보던 tab 이 그만큼 왼쪽으로 밀린다.
		e.active--
	case index == e.active:
		// 마지막 tab 을 닫았으면 왼쪽으로 간다.
		e.active = min(e.active, len(e.buffers)-1)
	}

	// 닫은 자리만큼 오른쪽이 비므로 왼쪽에 가려둔 것이 도로 보일 수 있다.
	e.scrollTabsTo()

	// 드러난 파일 자리로 트리를 데려가는 것은 forceCloseTab 이 한다 — 그쪽이 Cmd 를
	// 돌려주는 자리다(ADR-0032).
	return true
}

// moveTab 은 활성 tab 을 to 자리로 옮긴다. 사이에 있던 것들이 그만큼 밀린다(ADR-0090).
//
// **보고 있는 파일은 그대로다.** 활성 자리를 새 index 로 따라 옮긴다 — 끌어서 옮기는 손은
// 순서만 바꾸려는 것이고, 보던 것이 바뀌면 그것은 다른 일이다.
//
// 옮기고 나면 활성 tab 이 가려진 쪽으로 갔을 수 있다. 그것을 안으로 당기는 것은 tabline 이
// 그릴 때 이미 한다(ADR-0029). 여기서도 한 번 맞춰 두는 것은 닫는 자리와 같은 손이다.
func (e *editor) moveTab(to int) {
	from := e.active
	if to < 0 || to >= len(e.buffers) || to == from {
		return
	}

	// **Delete 가 뒷마당을 그대로 쓴다.** 지운 자리 뒤를 앞으로 당기므로 buf 를 먼저 값으로
	// 떠 두어야 한다. 뜨지 않으면 밀린 뒤의 엉뚱한 buffer 를 넣는다.
	buf := e.buffers[from]
	e.buffers = slices.Insert(slices.Delete(e.buffers, from, from+1), to, buf)
	e.active = to

	e.scrollTabsTo()
}

// closeOtherTabs 는 활성 tab 만 남기고 나머지를 닫는다. 닫은 수를 준다.
//
// 남는 것이 보고 있던 tab 이라 편집 중인 파일도 sidebar 표시도 그대로다 —
// closeTab 과 달리 자리에 새로 드러나는 파일이 없어서 reveal 을 다시 하지 않는다.
func (e *editor) closeOtherTabs() int {
	closed := len(e.buffers) - 1

	e.buffers = []Buffer{e.buffers[e.active]}
	e.active = 0
	e.scrollTabsTo()

	return closed
}

// anyDirty 는 저장하지 않은 변경이 있는 buffer 가 하나라도 있는지다.
// 전체 종료는 보고 있지 않은 tab 의 변경도 잃게 하므로 활성 buffer 만 봐서는 안 된다.
func (e editor) anyDirty() bool {
	for _, buf := range e.buffers {
		if buf.dirty {
			return true
		}
	}

	return false
}

// otherDirty 는 보고 있지 않은 tab 에 저장하지 않은 변경이 있는지다.
// 다른 tab 을 모두 닫는 것은 활성 buffer 는 건드리지 않으므로 그것만 빼고 본다.
func (e editor) otherDirty() bool {
	for i, buf := range e.buffers {
		if i == e.active {
			continue
		}
		if buf.dirty {
			return true
		}
	}

	return false
}

// toggleTree 는 sidebar 를 여닫는다. `:tree` 가 쓴다.
//
// 닫을 때 트리를 버린다. 다시 열면 뿌리부터 새로 읽으므로 여닫는 것이 곧 새로고침이다.
// 그 읽기가 백그라운드 작업이라 여는 쪽에서 Cmd 가 나온다(ADR-0032).
//
// 여닫으면 편집 영역 너비가 달라져서 줄바꿈이 바뀌므로 활성 buffer 를 다시 맞춘다.
// 보고 있지 않은 tab 은 gt 로 갈 때 scrollTo 를 지나면서 알아서 맞는다.
func (e *editor) toggleTree() (tea.Cmd, error) {
	var cmd tea.Cmd

	if e.sidebar.open {
		e.sidebar = sidebar{}
	} else {
		root, err := os.Getwd()
		if err != nil {
			return nil, errors.Mark(err, errNoWorkingDir)
		}

		e.sidebar = openSidebar(root)

		// 닫을 때 트리를 버렸으므로 여는 이 자리에서 보고 있는 파일 자리를 다시 펼친다.
		// 이름 없는 buffer 면 갈 자리가 없어서 뿌리만 읽는다. tab 이 아예 없을 때도 같다.
		var target string
		if e.hasTab() {
			target = e.activeBuffer().path
		}

		e.sidebar.setRevealTarget(target)
		cmd = e.startTree()
	}

	e.scrollToCursor()
	// 편집 영역 너비가 32 칸 달라져서 tabline 에 들어가는 tab 수도 달라진다.
	e.scrollTabsTo()

	return cmd, nil
}

// startInitialJobs 는 첫 화면이 뜬 뒤에 시작하는 작업 전부다.
//
// **bubbletea 는 Init 을 첫 model 에게만 부른다.** 그 첫 model 이 normal 일 수도 빈 화면일
// 수도 있어서(인자 없이 시작하면 빈 화면이다) 두 Init 이 같은 이것을 부른다 — 한쪽에
// 빠뜨리면 git 표시와 트리 첫 읽기가 영영 돌지 않는다(ADR-0064).
//
// core.Run 은 Program 이 뜨기 전이라 Cmd 를 낼 자리가 없어서 그것들이 여기 모인다
// (ADR-0030, ADR-0032). CLI 인자로 Go 파일을 열고 시작하는 길도 여기다 — 언어 서버는
// 파일을 열 때 띄우는데(ADR-0051) 시작할 때 이미 열려 있는 것은 openTab 을 지나지 않는다.
//
// 첫 git 표시도 이 작업이 채운다. 그전까지 statusBar 오른쪽은 비어 있다 — 큰 저장소에서
// `git status` 를 기다리느라 편집기가 늦게 뜨는 것보다 낫다.
func (e *editor) startInitialJobs() tea.Cmd {
	return tea.Batch(e.startGitRefresh(), e.startOutsideCheck(), e.startTree(), e.startGoplsForOpenBuffers(), e.startWatch())
}

// editorView 는 mode 가 공유하는 화면이다.
//
// mode 마다 다른 것은 커서 모양과 statusBar 에 찍히는 것뿐이라 인자로 받는다.
// mode 별 model 이 자기 이름을 아는데 바깥에서 물을 필요가 없다(ADR-0002).
//
// bottom 은 statusBar 의 아래 줄이다. normal/insert 는 커서 위치를 넣고,
// command mode 는 치고 있는 명령을 넣는다. vim 처럼 맨 아래 줄을 명령줄로 쓰는 것이라
// 줄을 더 만들지 않아 편집 영역 높이가 흔들리지 않는다.
func (e *editor) editorView(shape tea.CursorShape, mode, bottom string) tea.View {
	// tab 이 없으면 그릴 파일이 없다. 본문 자리만 빈 화면으로 갈아끼운다(ADR-0064).
	//
	// **가르는 자리가 여기 하나다.** normal 만이 아니라 command·search·팔레트가 같은 자리를
	// 지나고, tab 없이 트리에 포커스를 둔 상태도 실재해서(`ctrl+w ctrl+w`) 그때 그리는 것은
	// viewSidebar 다 — 빈 화면 model 안에 두면 그 상태가 비거나 터진다.
	//
	// 커서는 얹지 않는다. 놓을 글자가 없어서 터미널이 숨긴다.
	if !e.hasTab() {
		return newView(e.renderScreen(e.renderEmptyScreen(), mode, bottom))
	}

	buf := e.activeBuffer()
	height := e.textHeight()

	// 화면보다 긴 줄은 visibleRows 가 이미 화면 행 여러 개로 나눠서 준다.
	rows := buf.visibleRows(e.contentWidth(), height)
	textRows := make([]string, 0, height)

	// 문법 토큰을 화면 맨 아래 줄까지 채운다. 줄 하나를 훑으려면 그 앞 줄을 끝낸 문맥이
	// 필요해서, 담아둔 것이 없으면 위에서부터 내려온다(syntax.go).
	//
	// 그리는 자리에서 캐시를 채우는 것이라 receiver 가 포인터다. 그리는 길이 이 함수 하나뿐이라
	// mode 마다 챙길 자리가 없다 — 빠뜨린 mode 에서 고리가 끊기는 것은 tick 에서 이미 겪은
	// 일이다(ADR-0038).
	if len(rows) > 0 {
		buf.lexSyntaxTo(rows[len(rows)-1].line)
	}

	// 검색 매칭은 줄 단위로 찾는다. wrap 된 줄은 행이 여럿이라 줄이 바뀔 때만 다시 찾는다.
	matchLine, matches := -1, [][]int(nil)

	// 고른 범위는 화면마다 한 번만 구한다. 줄마다의 구간은 selectionOn 이 잘라 준다.
	area, selecting := buf.selectionRange()

	for _, row := range rows {
		if row.line != matchLine {
			matchLine, matches = row.line, e.searchMatches(buf.lines[row.line])
		}

		// 커서가 선 매칭만 색이 다르다. 다른 줄이면 그런 매칭이 없다.
		cursorCol := -1
		if row.line == buf.cursorLine {
			cursorCol = buf.cursorCol
		}

		highlight := rowHighlight{
			matches:   matches,
			cursorCol: cursorCol,
			tokens:    buf.syntaxTokens(row.line),
		}
		if selecting {
			highlight.selection, highlight.toLineEnd, _ = buf.selectionOn(area, row.line)
		}

		textRows = append(textRows,
			e.renderGutter(buf, row)+
				renderRow(buf.lines[row.line], row, e.contentWidth(), highlight))
	}

	// 감싸는 머리줄로 본문 위 몇 행을 덮는다(ADR-0049).
	//
	// **행 수가 그대로다.** 자리를 떼지 않고 갈아끼우므로 textHeight·sidebarHeight·
	// visibleRows·positionAt·cursorScreenPos·regionAt 이 하나도 안 바뀐다. 붙는 줄 수가
	// 스크롤에 따라 바뀌는데 그것이 화면 높이가 되면 sidebar 까지 스크롤마다 늘었다 줄었다 한다.
	//
	// lipgloss 합성기(ADR-0011) 를 쓰지 않는다. 폭 전체를 쓰는 행이라 문자열을 통째로
	// 갈아끼우면 되고, 그래서 셀 버퍼를 지나며 화면이 다시 쓰이는 비용이 없다 — 머리줄이
	// 없는 화면은 이 기능을 넣기 전과 한 글자도 다르지 않다.
	//
	// 커서가 덮이지 않는 것은 scrollTo 가 맡는다. 파일 끝이라 그린 행이 모자랄 때만 여기서
	// 한 번 더 자른다.
	for i, line := range e.stickyRows(buf, height, len(textRows)) {
		textRows[i] = e.renderStickyRow(buf, line)
	}

	view := newView(e.renderScreen(textRows, mode, bottom))

	if x, y, ok := buf.cursorScreenPos(e.contentWidth(), height); ok {
		// cursorScreenPos 는 본문 안에서의 좌표를 주므로 화면 좌표로 옮긴다.
		view.Cursor = tea.NewCursor(x+e.contentLeft(), y+tablineHeight)
		view.Cursor.Shape = shape
	}

	return view
}

// newView 는 화면 전체 행으로 tea.View 를 만든다.
//
// 터미널 설정이 여기 한 곳에 있다. 편집 화면과 달리 tabline·sidebar 를 쓰지 않는 화면
// (`:jobs`) 도 같은 설정을 그대로 받아야 대체 화면과 키 확장이 어긋나지 않는다.
// 커서는 부르는 쪽이 얹는다 — 어디에 둘지가 화면마다 다르다.
//
// editor 를 받지 않는다. 그릴 것은 부르는 쪽이 이미 다 만들어서 오므로 이것은 층이
// 아니라 정해진 설정을 붙여 주는 자리다(ADR-0036).
func newView(rows []string) tea.View {
	view := tea.NewView(strings.Join(rows, "\n"))

	view.MouseMode = tea.MouseModeCellMotion
	view.AltScreen = true

	// 터미널에 PC-101 자리의 키를 같이 달라고 한다. 한글 입력 상태에서 `ctrl+p` 가
	// `ctrl+ㅔ` 로 오는 것을 터미널이 되돌려 준다(ADR-0014).
	view.KeyboardEnhancements.ReportAlternateKeys = true

	// 창을 오갈 때 알려달라고 한다. 다른 창에서 파일을 고치고 돌아오는 순간이 여기다(ADR-0031).
	// 터미널이 보고하지 않으면 msg 가 오지 않을 뿐이고 나머지는 그대로다.
	view.ReportFocus = true

	return view
}

// renderScreen 는 편집 내용에 tabline·sidebar·statusBar 를 맞물려 화면 전체 행을 만든다.
//
// tabline 은 편집 영역 위에만 그린다. sidebar 위에 걸치면 tab 목록이 지금 보고 있는 파일이
// 아니라 트리에 딸린 것처럼 읽힌다. 그래서 sidebar 가 화면 맨 윗줄부터 시작하고
// tabline 은 그 오른쪽에서 편집 영역 너비만큼만 그려진다.
//
// statusBar 는 다르다. 줄 자체는 화면 끝까지 이어지고 글자만 편집 영역 아래에서 시작한다.
// sidebar 는 그 앞에서 멈춘다(ADR-0005).
//
// sidebar 가 있으면 행마다 sidebar 와 오른쪽을 맞물려야 한다. 앞에 붙이기만 하면
// 파일이 짧을 때 채움 행에 sidebar 가 안 실려서 트리가 파일 길이만큼만 그려진다.
//
// sidebar 가 없으면 지금까지와 똑같이 그린다. 채움 행은 빈 문자열이고 본문 뒤에
// 빈 칸을 붙이지 않는다. 그래야 화면 문자열이 예전과 한 글자도 다르지 않다.
func (e editor) renderScreen(textRows []string, mode, bottom string) []string {
	height := e.sidebarHeight()

	// sidebar 오른쪽에 쌓이는 것들이다. 맨 위가 tabline 이고 그 아래가 편집 내용이다.
	right := make([]string, 0, height)
	right = append(right, e.renderTabline(e.textWidth()).line)
	right = append(right, textRows...)
	for len(right) < height {
		right = append(right, "")
	}

	rows := right
	if e.sidebarVisible() {
		cells := e.sidebar.renderCells(height, e.activePath(), e.boxChars)

		rows = make([]string, 0, height)
		for i := range height {
			rows = append(rows, cells[i]+right[i])
		}
	}

	return append(rows, e.renderStatusBar(mode, bottom)...)
}
