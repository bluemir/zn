package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"
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

	sidebar sidebar

	// search 는 마지막 검색이다. `n` 은 tab 을 옮겨서도 같은 것을 찾으므로 Buffer 가 아니라 여기 있다.
	search searchState

	// register 는 마지막으로 지운 내용이다. 붙여넣기가 tab 을 넘어 되어야 하므로 여기 있다.
	// vim 의 register 도 buffer 밖이다. 아직 읽는 곳이 없다 — `p` 를 넣을 때 쓴다.
	register register

	// git 은 statusBar 오른쪽에 찍는 저장소 상태다. 화면을 그릴 때 읽지 않고 여기에 들고 있다가
	// 5 초마다 도는 갱신 작업과 저장·파일 열기 직후에 다시 읽는다(ADR-0009, ADR-0030).
	git gitStatus

	// ctx 는 편집기의 수명이다. core.Run 이 받은 것을 그대로 든다.
	// 백그라운드 작업이 여기서 갈라져 나오므로 편집기를 끝내면 도는 것이 전부 정리된다(ADR-0027).
	ctx context.Context

	// jobs 는 백그라운드에서 도는 작업들이고 finished 는 최근에 끝난 것들이다.
	// statusBar 는 도는 것만 보고 `:jobs` 목록이 둘 다 본다.
	// 결과는 여기가 아니라 종류마다 자기 자리에 쌓인다(job.go).
	jobs     []job
	finished []job

	// message 는 명령 결과나 오류다. 다음 키를 누르면 사라진다.
	//
	// mode 가 아니라 여기 있는 것은 백그라운드 작업의 실패가 어느 mode 에서든 도착하기 때문이다.
	// 아래 줄에 그리는 것은 normal·insert·트리뿐이다 — 명령줄과 검색은 그 줄을 자기가 쓴다.
	message string

	// files 는 팔레트가 고르는 파일 목록이다. 인덱싱 작업이 채운다.
	// 팔레트를 닫아도 남는다 — 인덱싱은 팔레트보다 오래 살고, 다시 열면 모아둔 것부터 보인다.
	files []string

	// asciiBox 는 테두리·구분선을 ASCII 로 그릴지다. 시작할 때 터미널을 재서 한 번 정하고
	// 그 뒤로 바뀌지 않는다(ADR-0028). zero value 는 유니코드 박스라 재보지 않은 곳 —
	// 테스트가 그렇다 — 은 지금까지와 똑같이 그린다.
	asciiBox bool

	width  int
	height int
}

// boxChars 는 이 편집기가 쓸 테두리·구분선 글자다.
func (e editor) boxChars() boxSet {
	if e.asciiBox {
		return boxASCII
	}

	return boxUnicode
}

// buffer 는 활성 buffer 를 가리킨다.
// 값이 아니라 slice 요소를 가리켜야 커서 이동과 편집이 제자리에 남는다.
func (e *editor) buffer() *Buffer {
	return &e.buffers[e.active]
}

func (e *editor) resize(msg tea.WindowSizeMsg) {
	e.width = msg.Width
	e.height = msg.Height
	e.buffer().scrollTo(e.contentWidth(), e.textHeight())
	e.scrollTabsTo()
}

// nextTab, prevTab 은 활성 tab 을 옮긴다. 양끝에서 둘러 간다. vim 의 gt/gT 와 같다.
//
// 트리가 그 파일 자리를 아직 읽지 않았으면 읽는 작업이 시작되므로 Cmd 가 나온다(ADR-0032).
func (e *editor) nextTab() tea.Cmd {
	e.active = (e.active + 1) % len(e.buffers)
	e.scrollTabsTo()

	return e.revealInSidebar(e.buffer().path)
}
func (e *editor) prevTab() tea.Cmd {
	e.active = (e.active - 1 + len(e.buffers)) % len(e.buffers)
	e.scrollTabsTo()

	return e.revealInSidebar(e.buffer().path)
}

// activePath 는 지금 보고 있는 파일의 절대 경로다. sidebar 가 그 행을 굵게 그린다(ADR-0022).
//
// 트리 항목은 절대 경로이고 CLI 로 연 파일은 상대 경로다. tabOf·reveal 과 같은 이유로 맞춰 둔다.
// 이름 없는 buffer 는 빈 문자열이라 어느 행과도 맞지 않는다.
func (e editor) activePath() string {
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
// git 갱신은 여기서 하지 않는다. 언제 다시 읽을지는 정책이라 부르는 쪽이 `refreshGit` 을
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

	return e.revealInSidebar(path), nil
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

	return e.revealInSidebar(path), nil
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

// closeTab 은 활성 tab 을 닫는다. 마지막 하나뿐이면 닫지 않고 false 를 준다.
// 닫을 것이 없으면 부르는 쪽이 종료로 넘어간다.
func (e *editor) closeTab() bool {
	if len(e.buffers) < 2 {
		return false
	}

	e.buffers = slices.Delete(e.buffers, e.active, e.active+1)
	// 마지막 tab 을 닫았으면 왼쪽으로 간다.
	e.active = min(e.active, len(e.buffers)-1)
	// 닫은 자리만큼 오른쪽이 비므로 왼쪽에 가려둔 것이 도로 보일 수 있다.
	e.scrollTabsTo()

	// 드러난 파일 자리로 트리를 데려가는 것은 forceCloseTab 이 한다 — 그쪽이 Cmd 를
	// 돌려주는 자리다(ADR-0032).
	return true
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
			return nil, errors.Wrap(err, "cannot find current directory")
		}

		e.sidebar = openSidebar(root)

		// 닫을 때 트리를 버렸으므로 여는 이 자리에서 보고 있는 파일 자리를 다시 펼친다.
		// 이름 없는 buffer 면 갈 자리가 없어서 뿌리만 읽는다.
		e.sidebar.setRevealTarget(e.buffer().path)
		cmd = e.startTree()
	}

	e.buffer().scrollTo(e.contentWidth(), e.textHeight())
	// 편집 영역 너비가 32 칸 달라져서 tabline 에 들어가는 tab 수도 달라진다.
	e.scrollTabsTo()

	return cmd, nil
}

// tablineHeight 는 편집 영역 위 tabline 이 차지하는 줄 수다(docs/spec.md).
const tablineHeight = 1

// statusBarHeight 는 화면 아래 statusBar 가 차지하는 줄 수다(docs/spec.md).
const statusBarHeight = 2

// textHeight 는 편집 내용을 그릴 수 있는 높이다.
// tabline 이 편집 영역 위를, statusBar 가 그 아래를 차지한다.
func (e editor) textHeight() int {
	return max(0, e.height-tablineHeight-statusBarHeight)
}

// sidebarHeight 는 sidebar 가 차지하는 높이다.
//
// tabline 은 편집 영역 위에만 있으므로 sidebar 가 그 옆줄까지 올라간다.
// 아래로는 statusBar 앞에서 멈춘다 — statusBar 는 화면 끝까지 이어지는 한 줄이고
// 글자만 편집 영역 아래에서 시작한다(ADR-0005).
//
// 트리 이동과 스크롤은 편집 영역이 아니라 이 높이를 기준으로 세야 맨 윗줄이 잘리지 않는다.
func (e editor) sidebarHeight() int {
	return tablineHeight + e.textHeight()
}

// sidebarVisible 은 sidebar 가 실제로 그려지는지다.
//
// sidebar.open 은 사용자의 의도일 뿐이라 그것만 보면 안 된다. 화면이 좁으면 켜져 있어도
// 그리지 않는다. 24 칸을 떼고 나면 편집할 자리가 없고, textWidth 가 음수가 되면
// 줄바꿈 계산과 빈 칸 채우기가 무너진다.
//
// 폭과 관련된 모든 곳이 open 이 아니라 이것 하나만 봐야 한다. 한 군데라도 어긋나면
// 화면 절반만 밀린 상태가 된다.
func (e editor) sidebarVisible() bool {
	return e.sidebar.open && e.width >= sidebarWidth+minTextWidth
}

// sidebarLeft 는 편집 영역이 시작하는 화면 칸이다. 커서 좌표를 옮길 때 쓴다.
func (e editor) sidebarLeft() int {
	if e.sidebarVisible() {
		return sidebarWidth
	}
	return 0
}

// textWidth 는 편집 영역 너비다. tabline 과 statusBar 의 글자가 이 너비 안에 든다.
// e.width 는 statusBar 처럼 화면 끝까지 칠하는 것과 sidebar 를 그릴지 말지를 정할 때만 쓴다.
func (e editor) textWidth() int {
	return max(0, e.width-e.sidebarLeft())
}

// contentWidth 는 파일 내용을 그릴 너비다. 줄을 어디서 접을지가 이 값으로 정해진다.
// 편집 영역에서 줄번호 칸을 뗀 나머지다.
//
// 줄바꿈·스크롤·커서 계산은 모두 이 값을 써야 한다. textWidth 를 쓰면 줄번호 칸만큼
// 넓게 잡아서 줄이 화면 오른쪽으로 삐져나간다.
func (e editor) contentWidth() int {
	return max(0, e.textWidth()-e.lineNumberWidth())
}

// contentLeft 는 파일 내용이 시작하는 화면 칸이다. 커서 좌표를 옮길 때 쓴다.
func (e editor) contentLeft() int {
	return e.sidebarLeft() + e.lineNumberWidth()
}

// 줄번호 칸의 최소 자릿수다. 파일이 짧아도 이만큼은 잡아서 줄을 오갈 때 본문이 흔들리지 않는다.
//
// minAbsoluteDigits 3 은 vim 의 `numberwidth` 기본값 4 와 같은 자리다. vim 은 뒤 공백까지
// 포함한 총 폭이고 여기서는 자릿수라 하나 작다. 그래서 vim 과 같이 999 줄까지는 안 흔들리고
// 1000 줄에서 한 칸 늘어난다(ADR-0007). 이 값을 올리면 그 지점이 vim 과 갈린다.
const (
	minAbsoluteDigits = 3
	minRelativeDigits = 2
)

// lineNumberDigits 는 절대·상대 번호가 각각 쓰는 자릿수다.
//
// 절대번호는 전체 줄 수까지, 상대번호는 화면 높이까지만 커진다.
// 상대번호는 화면 밖으로 나가면 볼 수 없으므로 줄 수와 무관하다.
func (e editor) lineNumberDigits() (absolute, relative int) {
	return max(digits(len(e.buffers[e.active].lines)), minAbsoluteDigits),
		max(digits(e.textHeight()), minRelativeDigits)
}

// lineNumberWidth 는 줄번호 칸이 차지하는 폭이다. 안 그릴 때는 0 이다.
func (e editor) lineNumberWidth() int {
	absolute, relative := e.lineNumberDigits()

	// 번호 칸을 떼고 나면 본문이 남지 않는 좁은 화면에서는 그리지 않는다. sidebar 와 같은 규칙이다.
	width := absolute + 1 + relative + 1
	if e.textWidth()-width < minTextWidth {
		return 0
	}

	return width
}

// digits 는 십진수 자릿수다.
func digits(n int) int {
	count := 1
	for n >= 10 {
		n /= 10
		count++
	}

	return count
}

var (
	// styleLineNumberAbsolute 는 절대 줄번호 색이다. vim 의 `LineNr` 과 같은 노란색(ANSI 3)이라
	// 256 색 고정값과 달리 터미널 테마가 정한 노랑을 따른다(ADR-0007).
	styleLineNumberAbsolute = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	// styleLineNumberRelative 는 상대 줄번호 색이다. sidebar 의 흐린 색과 같은 값이다(ADR-0005).
	// 절대번호와 색이 달라, 둘이 나란히 있어도 어느 쪽이 무엇인지 색으로 갈린다.
	styleLineNumberRelative = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// lineNumber 는 화면 행 앞에 붙는 줄번호 칸이다. `절대 상대 ` 순서다.
//
// wrap 되어 이어지는 행은 빈 칸이다. 번호가 있는 행이 곧 논리 줄의 시작이라
// 화면에서 줄을 셀 때 헷갈리지 않는다. vim 과 같다.
func (e editor) lineNumber(cursorLine int, row screenRow) string {
	width := e.lineNumberWidth()
	if width == 0 {
		return ""
	}
	if row.start != 0 {
		return strings.Repeat(" ", width)
	}

	absolute, relative := e.lineNumberDigits()

	// 커서 줄은 0 이다. 절대번호가 바로 옆에 있어서 거기에 또 찍을 이유가 없다.
	distance := row.line - cursorLine
	if distance < 0 {
		distance = -distance
	}

	return styleLineNumberAbsolute.Render(fmt.Sprintf("%*d", absolute, row.line+1)) + " " +
		styleLineNumberRelative.Render(fmt.Sprintf("%*d", relative, distance)) + " "
}

// render 는 mode 가 공유하는 화면이다.
//
// mode 마다 다른 것은 커서 모양과 statusBar 에 찍히는 것뿐이라 인자로 받는다.
// mode 별 model 이 자기 이름을 아는데 바깥에서 물을 필요가 없다(ADR-0002).
//
// bottom 은 statusBar 의 아래 줄이다. normal/insert 는 커서 위치를 넣고,
// command mode 는 치고 있는 명령을 넣는다. vim 처럼 맨 아래 줄을 명령줄로 쓰는 것이라
// 줄을 더 만들지 않아 편집 영역 높이가 흔들리지 않는다.
func (e editor) render(shape tea.CursorShape, mode, bottom string) tea.View {
	buf := e.buffers[e.active]
	height := e.textHeight()

	// 화면보다 긴 줄은 visibleRows 가 이미 화면 행 여러 개로 나눠서 준다.
	textRows := make([]string, 0, height)

	// 검색 매칭은 줄 단위로 찾는다. wrap 된 줄은 행이 여럿이라 줄이 바뀔 때만 다시 찾는다.
	matchLine, matches := -1, [][]int(nil)

	for _, row := range buf.visibleRows(e.contentWidth(), height) {
		if row.line != matchLine {
			matchLine, matches = row.line, e.searchMatches(buf.lines[row.line])
		}

		// 커서가 선 매칭만 색이 다르다. 다른 줄이면 그런 매칭이 없다.
		cursorCol := -1
		if row.line == buf.cursorLine {
			cursorCol = buf.cursorCol
		}

		textRows = append(textRows,
			e.lineNumber(buf.cursorLine, row)+highlightRow(buf.lines[row.line], row, matches, cursorCol))
	}

	view := e.viewRows(textRows, mode, bottom)

	if x, y, ok := buf.cursorScreenPos(e.contentWidth(), height); ok {
		// cursorScreenPos 는 본문 안에서의 좌표를 주므로 화면 좌표로 옮긴다.
		view.Cursor = tea.NewCursor(x+e.contentLeft(), y+tablineHeight)
		view.Cursor.Shape = shape
	}

	return view
}

// viewRows 는 편집 영역에 그릴 행들을 받아 편집기 틀에 얹은 화면을 만든다.
// tabline 과 sidebar 가 따라온다.
func (e editor) viewRows(textRows []string, mode, bottom string) tea.View {
	return e.screenView(e.screenRows(textRows, mode, bottom))
}

// screenView 는 화면 전체 행을 받아 tea.View 를 만든다.
//
// 터미널 설정이 여기 한 곳에 있다. 편집 화면과 달리 tabline·sidebar 를 쓰지 않는 화면
// (`:jobs`) 도 같은 설정을 그대로 받아야 대체 화면과 키 확장이 어긋나지 않는다.
// 커서는 부르는 쪽이 얹는다 — 어디에 둘지가 화면마다 다르다.
func (e editor) screenView(rows []string) tea.View {
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

// screenRows 는 화면 전체 행이다.
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
func (e editor) screenRows(textRows []string, mode, bottom string) []string {
	height := e.sidebarHeight()

	// sidebar 오른쪽에 쌓이는 것들이다. 맨 위가 tabline 이고 그 아래가 편집 내용이다.
	right := make([]string, 0, height)
	right = append(right, e.tabline(e.textWidth()).line)
	right = append(right, textRows...)
	for len(right) < height {
		right = append(right, "")
	}

	rows := right
	if e.sidebarVisible() {
		cells := e.sidebar.cells(height, e.activePath(), e.boxChars())

		rows = make([]string, 0, height)
		for i := range height {
			rows = append(rows, cells[i]+right[i])
		}
	}

	return append(rows, e.statusBar(mode, bottom)...)
}

// reverse 는 편집 내용과 구분되는 색이다. 색을 정하지 않고 터미널의 전경·배경을 뒤집기만 한다.
// 밝은 테마든 어두운 테마든 알아서 맞고 팔레트를 정할 필요가 없다(ADR-0004).
var reverse = lipgloss.NewStyle().Reverse(true)

// tablineRow 는 tabline 한 줄과 그 줄의 어디에 무엇이 그려졌는지다.
//
// 자리표를 그리는 자리에서 같이 내는 것은 클릭 때문이다. 배치 계산을 두 벌 두면
// `+`(dirty) 하나로 칸이 밀렸을 때 클릭이 옆 tab 으로 간다.
type tablineRow struct {
	line string

	// tabs 는 tab 마다 그려진 칸 범위 [start, end) 다. 밀려나서 안 그려진 tab 은 빈 범위다.
	tabs [][2]int

	// left, right 는 가려짐 표시가 그려진 칸 범위다. 가린 것이 없으면 빈 범위다.
	left, right [2]int
}

// inSpan 은 칸 col 이 그 범위 안인지다. 빈 범위는 어느 칸도 품지 않는다.
func inSpan(span [2]int, col int) bool {
	return col >= span[0] && col < span[1]
}

// tabLabel 은 tabline 에 그리는 tab 한 칸의 글자다. `번호 파일이름` 이고 경로는 쓰지 않는다.
func (e editor) tabLabel(index int) string {
	buf := e.buffers[index]

	name := filepath.Base(buf.path)
	if buf.path == "" {
		name = "[No Name]"
	}
	if buf.dirty {
		name += "+"
	}

	return fmt.Sprintf(" %d %s ", index+1, name)
}

// tabWindow 는 tabline 에 그릴 것들이다. scroll 자리부터 count 개의 tab 을 그리고
// 양끝에 가려짐 표시를 붙인다.
//
// 표시보다 tab 이 먼저다. 좁아서 하나를 버려야 하면 표시가 빠져 빈 문자열이 된다 —
// 지금 보고 있는 파일 이름이 몇 개가 가려졌는지보다 중요하다.
type tabWindow struct {
	count       int
	left, right string
}

// layoutTabs 는 scroll 자리부터 width 칸에 들어가는 만큼을 배치한다.
//
// 반쯤 걸친 tab 은 넣지 않는다. 잘린 이름은 어느 파일인지 알려주지도 못하면서
// 가려진 개수에서도 빠져 `n>` 의 숫자를 틀리게 만든다.
//
// 오른쪽 표시의 폭이 가려진 개수의 자릿수를 타므로 한 번에 셀 수 없다. 표시가 없다고 보고
// 채운 뒤, 그 자리가 모자라면 tab 을 하나씩 물린다. 물릴 때마다 개수가 늘어 표시가
// 길어질 수 있으므로 다시 본다.
func (e editor) layoutTabs(scroll, width int) tabWindow {
	window := tabWindow{}
	if scroll > 0 {
		window.left = fmt.Sprintf("<%d", scroll)
	}

	// cost 는 그 tab 이 먹는 칸이다. 앞에 이미 그린 것이 있으면 구분선 한 칸이 붙는다.
	cost := func(index, drawn int, left string) int {
		cells := screenWidthOf(e.tabLabel(index))
		if drawn > 0 || left != "" {
			cells++
		}

		return cells
	}

	fill := func(left string) (count, used int) {
		rest := width
		if left != "" {
			rest -= screenWidthOf(left) + 1 // 표시와 그 뒤 구분선
		}

		for i := scroll; i < len(e.buffers); i++ {
			next := cost(i, count, left)
			if used+next > rest {
				break
			}

			used += next
			count++
		}

		// 오른쪽 표시 자리를 만드느라 tab 을 물린다. 마지막 하나는 물리지 않는다.
		for count > 1 && scroll+count < len(e.buffers) {
			hidden := len(e.buffers) - scroll - count
			if used+1+screenWidthOf(fmt.Sprintf("%d>", hidden)) <= rest {
				break
			}

			count--
			used -= cost(scroll+count, count, left)
		}

		return count, used
	}

	count, used := fill(window.left)

	// tab 이 하나도 안 들어가면 왼쪽 표시를 떼고 그 자리를 tab 에 준다.
	if count == 0 && window.left != "" {
		window.left = ""
		count, used = fill("")
	}

	// 그래도 안 들어갈 만큼 좁으면 잘려도 하나는 그린다. 활성 tab 이 아예 사라지는 것보다 낫다.
	if count == 0 {
		count = 1
	}

	window.count = count

	if hidden := len(e.buffers) - scroll - count; hidden > 0 {
		right := fmt.Sprintf("%d>", hidden)
		rest := width - used
		if window.left != "" {
			rest -= screenWidthOf(window.left) + 1
		}

		if 1+screenWidthOf(right) <= rest {
			window.right = right
		}
	}

	return window
}

// tabline 은 편집 영역 맨 위 한 줄이다. 열린 파일과 지금 보고 있는 것을 보여준다.
//
// 보고 있는 tab 만 편집 내용과 같은 색이고 나머지는 반전이다. vim 의 TabLine/TabLineSel 과 같다.
// 활성 tab 이 아래 내용과 이어져 보이는 것이 tab 이라는 비유 자체다.
//
// width 는 화면 너비가 아니라 편집 영역 너비다. sidebar 가 열려 있으면 그만큼 좁다.
// 다 그릴 수 없으면 tabScroll 자리부터 그리고 남은 것은 양끝의 `<n`·`n>` 이 알린다(ADR-0029).
func (e editor) tabline(width int) tablineRow {
	row := tablineRow{tabs: make([][2]int, len(e.buffers))}

	line := strings.Builder{}
	col := 0

	// put 은 남은 칸만큼만 쓰고 그린 자리를 돌려준다. 넘치는 부분은 버린다.
	// 색을 입힌 뒤에는 escape 가 섞여서 폭을 셀 수 없으므로 자르는 것이 먼저다.
	put := func(text string, active bool) [2]int {
		if width > 0 {
			text = text[:offsetAtScreenCol([]byte(text), width-col)]
		}
		if text == "" {
			return [2]int{}
		}

		start := col
		if active {
			line.WriteString(text)
		} else {
			line.WriteString(reverse.Render(text))
		}
		col += screenColAt([]byte(text), len(text))

		return [2]int{start, col}
	}

	scroll := min(max(e.tabScroll, 0), len(e.buffers)-1)
	window := e.layoutTabs(scroll, width)

	if window.left != "" {
		row.left = put(window.left, false)
	}

	for i := scroll; i < scroll+window.count; i++ {
		// 이미 그린 것이 있으면 그 사이를 가른다. 가려짐 표시와 tab 사이도 같다.
		if col > 0 {
			put(e.boxChars().vertical, false)
		}

		row.tabs[i] = put(e.tabLabel(i), i == e.active)
	}

	if window.right != "" {
		if col > 0 {
			put(e.boxChars().vertical, false)
		}

		row.right = put(window.right, false)
	}

	// 남은 칸도 채워야 줄 전체가 한 덩어리로 보인다.
	if width > col {
		put(strings.Repeat(" ", width-col), false)
	}

	row.line = line.String()

	return row
}

// scrollTabsTo 는 활성 tab 이 tabline 에 온전히 보이도록 tabScroll 을 맞춘다.
// 보고 있는 tab 이 바뀌거나 편집 영역 너비가 바뀌는 자리가 부른다. buffer 의 scrollTo 와 같다.
//
// 최소한만 민다. 화면 밖으로 나간 만큼만 따라가야 tabline 이 덜 흔들린다.
// 뒤쪽이 남아 도는 것도 당긴다 — tab 을 닫거나 화면이 넓어져 오른쪽에 빈 칸이 생기면
// 왼쪽에 가려둔 것을 도로 보여준다.
func (e *editor) scrollTabsTo() {
	width := e.textWidth()

	e.tabScroll = min(max(e.tabScroll, 0), len(e.buffers)-1)

	if e.tabScroll > e.active {
		e.tabScroll = e.active
	}

	// 한 칸씩 미는 것은 tab 마다 폭이 달라서다. 몇 개를 밀면 되는지 셈으로 알 수 없다.
	for e.tabScroll < e.active && e.active >= e.tabScroll+e.layoutTabs(e.tabScroll, width).count {
		e.tabScroll++
	}

	for e.tabScroll > 0 && e.tabScroll-1+e.layoutTabs(e.tabScroll-1, width).count >= len(e.buffers) {
		e.tabScroll--
	}
}

// statusBar 는 화면 아래 두 줄이다. 위 줄은 mode 와 파일과 git, 아래 줄은 부르는 쪽이 정한다.
//
// 위 줄만 반전이다. 아래 줄은 vim 처럼 명령줄이라 배경을 그대로 둔다.
// `:` 를 칠 때 배경이 뜨지 않고 명령 결과와 오류도 평범한 글자로 읽힌다.
//
// 줄은 sidebar 아래까지 화면 끝에서 끝까지 이어진다. 줄 자체는 tabline 과 달리 끊지 않는다 —
// 화면 맨 아래를 가로지르는 한 줄이라야 편집기 전체의 상태 표시로 읽힌다.
//
// 위 줄은 mode 가 sidebar 아래, 경로가 편집 영역 아래다. 둘 다 자기가 가리키는 것 바로 밑에
// 서게 된다. TREE 는 트리 아래에, 경로는 그 파일을 편집하는 자리 아래에 온다.
//
// sidebar 가 없으면 mode 를 놓을 왼쪽 칸 자체가 없으므로 경로 앞에 나란히 붙인다.
//
// 아래 줄은 mode 를 따라가지 않고 편집 영역에 맞춰 들여쓴다. 명령줄과 커서 위치는 편집 중인
// 파일에 딸린 것이라 위 줄의 경로와 세로로 맞아야 읽힌다.
func (e editor) statusBar(mode, bottom string) []string {
	buf := e.buffers[e.active]

	path := buf.path
	if path == "" {
		path = "[No Name]"
	}
	if buf.dirty {
		path += " [+]"
	}

	// `[!]` 는 마지막으로 맞춰 봤을 때 바깥이 달라져 있었다는 것이다. `[+]` 가 내 손의 미저장
	// 변경이고 이것은 남의 변경이라, 둘이 같이 붙으면 양쪽에 잃을 것이 있다는 뜻이다(ADR-0031).
	if buf.outside != outsideSame {
		path += " [!]"
	}

	// 반전 안에 두어야 색이 왼쪽 끝까지 이어진다.
	// 자르는 것이 채우는 것보다 먼저다 — 두 칸짜리 글자가 경계에 걸치면 통째로 버려진다.
	left, text := "", mode+"  "+path
	if e.sidebarVisible() {
		label := truncateToWidth(mode, sidebarWidth)
		left = label + strings.Repeat(" ", max(0, sidebarWidth-screenColAt([]byte(label), len(label))))
		text = path
	}

	width := e.textWidth()

	// Width 가 남은 칸을 공백으로 채워서 줄 끝까지 색이 간다.
	return []string{
		reverse.Width(e.width).Render(left + e.withStatus(truncateToWidth(text, width))),
		strings.Repeat(" ", e.sidebarLeft()) + truncateToWidth(bottom, width),
	}
}

// withStatus 는 statusBar 위 줄 오른쪽 끝에 진행 표시와 저장소 상태를 붙인다.
//
// 붙일 칸이 없으면 그대로 둔다 — 지금 무슨 mode 인지와 어느 파일인지가 먼저다.
// 사이를 두 칸 이상 띄운다. 한 칸이면 파일 이름이 긴 tab 에서 경로에 붙은 글자처럼 읽힌다.
//
// 오른쪽 끝이 git 이고 그 왼쪽이 진행 표시다. 칸이 모자라면 진행 표시부터 줄인다 —
// 막대를 떼고, 그래도 모자라면 진행 표시를 통째로 뺀다. git 은 늘 같은 자리에 있어야 눈이 찾는다.
func (e editor) withStatus(top string) string {
	git := e.git.label()

	// 진행 표시와 git 을 잇는다. 한쪽이 비면 나머지만 남는다.
	right := func(progress string) string {
		switch {
		case progress == "":
			return git
		case git == "":
			return progress
		default:
			return progress + "  " + git
		}
	}

	used := screenColAt([]byte(top), len(top))

	for _, label := range []string{right(e.jobText(e.jobBar())), right(e.jobText("")), git} {
		if label == "" {
			continue
		}

		pad := e.textWidth() - used - screenColAt([]byte(label), len(label))
		if pad < 2 {
			continue
		}

		return top + strings.Repeat(" ", pad) + label
	}

	return top
}

// messageOr 는 statusBar 아래 줄에 무엇을 쓸지다. 알림이 있으면 그것이 먼저다.
//
// 명령줄·검색은 그 줄을 자기 입력에 쓰므로 이것을 부르지 않는다.
func (e editor) messageOr(fallback string) string {
	if e.message != "" {
		return e.message
	}

	return fallback
}

// position 은 커서 위치와 전체 줄 수다. normal/insert 의 statusBar 아래 줄이다.
// 줄과 칸은 1 부터 세고, 칸은 byte offset 이 아니라 화면 칸이다.
func (e editor) position() string {
	buf := e.buffers[e.active]
	col := screenColAt(buf.lines[buf.cursorLine], buf.cursorCol)

	return fmt.Sprintf("%d:%d  (%d 줄)", buf.cursorLine+1, col+1, len(buf.lines))
}

// withShowcmd 는 statusBar 아래 줄 오른쪽 끝에 치고 있는 키를 붙인다. vim 의 showcmd 와 같은 자리다.
//
// 숫자나 접두 키를 치는 동안 화면에 아무 표시가 없으면 편집기가 그 키를 먹었는지 알 수 없다.
// 붙일 칸이 없으면 아래 줄을 그대로 둔다 — 커서 위치나 명령 결과가 밀려나는 것이 더 나쁘다.
func (e editor) withShowcmd(bottom, showcmd string) string {
	if showcmd == "" {
		return bottom
	}

	pad := e.textWidth() - screenColAt([]byte(bottom), len(bottom)) - screenColAt([]byte(showcmd), len(showcmd))
	if pad < 1 {
		return bottom
	}

	return bottom + strings.Repeat(" ", pad) + showcmd
}

// truncateToWidth 는 화면 너비를 넘는 부분을 자른다.
// statusBar 가 넘치면 터미널이 줄바꿈해서 화면이 밀린다.
func truncateToWidth(s string, width int) string {
	if width < 1 {
		return s
	}

	line := []byte(s)
	return string(line[:offsetAtScreenCol(line, width)])
}
