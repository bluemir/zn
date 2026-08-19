package core

import (
	"context"
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

	// register 는 마지막으로 지우거나 복사한 내용이다. 붙여넣기가 tab 을 넘어 되어야 하므로
	// 여기 있다. vim 의 register 도 buffer 밖이다. `d`·`c`·`y` 가 채우고 `p`·`P` 가 읽는다(ADR-0017).
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

	// boxChars 는 테두리·구분선에 쓸 글자다. 시작할 때 터미널을 재서 core.Run 이 넣어주고
	// 그 뒤로 바뀌지 않는다(ADR-0028).
	boxChars boxSet

	width  int
	height int
}

// activeBuffer 는 활성 activeBuffer 를 가리킨다.
// 값이 아니라 slice 요소를 가리켜야 커서 이동과 편집이 제자리에 남는다.
func (e *editor) activeBuffer() *Buffer {
	return &e.buffers[e.active]
}

// scrollToCursor 는 활성 buffer 를 지금 화면에 맞춘다. 커서가 화면 안에 들어오게 하고,
// 폭이 달라졌으면 줄바꿈도 다시 잡는다.
//
// 편집·이동 동작이 끝에 이것을 하고, 창 크기가 바뀌거나 sidebar 를 여닫거나 보는 buffer 가
// 바뀐 뒤에도 부른다 — 그 buffer 는 지금 폭을 본 적이 없을 수 있다.
func (e *editor) scrollToCursor() {
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
	e.active = (e.active + 1) % len(e.buffers)
	e.scrollTabsTo()

	return e.revealInSidebar(e.activeBuffer().path)
}
func (e *editor) prevTab() tea.Cmd {
	e.active = (e.active - 1 + len(e.buffers)) % len(e.buffers)
	e.scrollTabsTo()

	return e.revealInSidebar(e.activeBuffer().path)
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
		e.sidebar.setRevealTarget(e.activeBuffer().path)
		cmd = e.startTree()
	}

	e.scrollToCursor()
	// 편집 영역 너비가 32 칸 달라져서 tabline 에 들어가는 tab 수도 달라진다.
	e.scrollTabsTo()

	return cmd, nil
}

// editorView 는 mode 가 공유하는 화면이다.
//
// mode 마다 다른 것은 커서 모양과 statusBar 에 찍히는 것뿐이라 인자로 받는다.
// mode 별 model 이 자기 이름을 아는데 바깥에서 물을 필요가 없다(ADR-0002).
//
// bottom 은 statusBar 의 아래 줄이다. normal/insert 는 커서 위치를 넣고,
// command mode 는 치고 있는 명령을 넣는다. vim 처럼 맨 아래 줄을 명령줄로 쓰는 것이라
// 줄을 더 만들지 않아 편집 영역 높이가 흔들리지 않는다.
func (e editor) editorView(shape tea.CursorShape, mode, bottom string) tea.View {
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
			e.renderLineNumber(buf.cursorLine, row)+renderRow(buf.lines[row.line], row, matches, cursorCol))
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

// reverse 는 편집 내용과 구분되는 색이다. 색을 정하지 않고 터미널의 전경·배경을 뒤집기만 한다.
// 밝은 테마든 어두운 테마든 알아서 맞고 팔레트를 정할 필요가 없다(ADR-0004).
var reverse = lipgloss.NewStyle().Reverse(true)
