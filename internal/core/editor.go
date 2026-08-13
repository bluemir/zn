package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"
)

// editor 는 mode 가 바뀌어도 유지되는 상태다.
//
// mode 는 enum 이 아니라 화면 model 을 갈아끼워서 나타낸다(ADR-0002).
// mode 별 model 이 이것을 embed 해서 들고 다니고, 전환할 때 그대로 넘긴다.
type editor struct {
	buffers []Buffer
	active  int

	sidebar sidebar

	// search 는 마지막 검색이다. `n` 은 tab 을 옮겨서도 같은 것을 찾으므로 Buffer 가 아니라 여기 있다.
	search searchState

	// register 는 마지막으로 지운 내용이다. 붙여넣기가 tab 을 넘어 되어야 하므로 여기 있다.
	// vim 의 register 도 buffer 밖이다. 아직 읽는 곳이 없다 — `p` 를 넣을 때 쓴다.
	register register

	// git 은 statusBar 오른쪽에 찍는 저장소 상태다. 화면을 그릴 때 읽지 않고
	// 여기에 들고 있다가 파일을 열거나 저장할 때만 다시 읽는다(ADR-0009).
	git gitStatus

	width  int
	height int
}

// buffer 는 활성 buffer 를 가리킨다.
// slice 요소를 직접 가리켜야 커서 이동이 model 복사를 넘어 남는다.
func (e *editor) buffer() *Buffer {
	return &e.buffers[e.active]
}

func (e *editor) resize(msg tea.WindowSizeMsg) {
	e.width = msg.Width
	e.height = msg.Height
	e.buffer().scrollTo(e.contentWidth(), e.textHeight())
}

// nextTab, prevTab 은 활성 tab 을 옮긴다. 양끝에서 둘러 간다. vim 의 gt/gT 와 같다.
func (e *editor) nextTab() {
	e.active = (e.active + 1) % len(e.buffers)
	e.revealInSidebar(e.buffer().path)
}
func (e *editor) prevTab() {
	e.active = (e.active - 1 + len(e.buffers)) % len(e.buffers)
	e.revealInSidebar(e.buffer().path)
}

// revealInSidebar 는 트리를 그 파일 자리까지 펼치고 고른 뒤 화면 안으로 끌어온다.
// 보고 있는 파일이 바뀌는 모든 길이 이것을 부른다(ADR-0019).
//
// sidebar 를 열지는 않는다. `:tree` 로 닫혀 있으면 트리가 아예 없어서 아무 일도 하지 않고,
// 좁은 화면이라 감춰진 상태면 트리만 펼쳐 둔다 — 화면이 넓어지면 그 자리가 보인다.
//
// 이름 없는 buffer 는 경로가 빈 문자열이라 reveal 이 false 를 주고 고른 자리가 그대로 남는다.
func (e *editor) revealInSidebar(path string) {
	if !e.sidebar.reveal(path) {
		return
	}

	e.sidebar.scrollTo(e.sidebarHeight())
}

// newTab 은 이름 없는 빈 tab 을 활성 tab 바로 뒤에 끼우고 그리로 옮긴다.
// vim 의 :tabnew 와 같다. 맨 뒤가 아니라 보고 있던 것 옆에 생겨야 방금 만든 것을 찾기 쉽다.
//
// closeTab 과 같은 이유로 slice 를 새로 할당한다.
func (e *editor) newTab() {
	rest := make([]Buffer, 0, len(e.buffers)+1)
	rest = append(rest, e.buffers[:e.active+1]...)
	rest = append(rest, newEmptyBuffer(""))
	rest = append(rest, e.buffers[e.active+1:]...)

	e.buffers = rest
	e.active++
}

// openTab 은 파일을 tab 으로 연다. 이미 열려 있으면 새로 열지 않고 그 tab 으로 옮긴다.
//
// 같은 파일을 두 tab 에 열면 각각 독립된 Buffer 가 되어, 한쪽에서 저장하는 순간 다른 쪽의
// 편집이 사라진다. 나중 저장은 바깥 변경으로 잡혀 막히지만(ADR-0015) 두 편집을 합칠 길은
// 없다. 그래서 여는 것보다 찾는 것이 먼저다.
func (e *editor) openTab(path string) error {
	if index, ok := e.tabOf(path); ok {
		e.active = index
	} else {
		buf, err := OpenBuffer(path)
		if err != nil {
			return err
		}

		// 파일을 여는 동안 바깥에서 commit 이나 checkout 이 있었을 수 있다.
		e.git = readGitStatus()

		rest := make([]Buffer, 0, len(e.buffers)+1)
		rest = append(rest, e.buffers[:e.active+1]...)
		rest = append(rest, buf)
		rest = append(rest, e.buffers[e.active+1:]...)

		e.buffers = rest
		e.active++
	}

	e.revealInSidebar(path)

	return nil
}

// tabOf 는 그 파일을 이미 열어둔 tab 을 찾는다.
//
// 경로를 정규화해서 비교한다. CLI 로 연 파일은 상대 경로(`internal/core/editor.go`)이고
// 트리는 절대 경로를 주므로, 글자 그대로 비교하면 같은 파일을 못 알아보고 중복 Buffer 가 생긴다.
func (e editor) tabOf(path string) (int, bool) {
	want, err := filepath.Abs(path)
	if err != nil {
		return 0, false
	}

	for i, buf := range e.buffers {
		if buf.path == "" {
			continue
		}

		got, err := filepath.Abs(buf.path)
		if err != nil {
			continue
		}
		if got == want {
			return i, true
		}
	}

	return 0, false
}

// closeTab 은 활성 tab 을 닫는다. 마지막 하나뿐이면 닫지 않고 false 를 준다.
// 닫을 것이 없으면 부르는 쪽이 종료로 넘어간다.
//
// slice 를 제자리에서 줄이지 않고 새로 할당한다. mode model 이 editor 를 값으로 embed 해서
// backing array 를 공유하므로, 제자리에서 줄이면 아직 살아 있는 다른 복사본(확인창의 parent 등)이
// 어긋난 내용을 보게 된다. Buffer 는 slice header 뭉치라 복사가 싸다.
func (e *editor) closeTab() bool {
	if len(e.buffers) < 2 {
		return false
	}

	rest := make([]Buffer, 0, len(e.buffers)-1)
	rest = append(rest, e.buffers[:e.active]...)
	rest = append(rest, e.buffers[e.active+1:]...)

	e.buffers = rest
	// 마지막 tab 을 닫았으면 왼쪽으로 간다.
	e.active = min(e.active, len(e.buffers)-1)

	// 닫은 파일이 아니라 그 자리에 드러난 파일이 이제 보는 파일이다.
	e.revealInSidebar(e.buffer().path)

	return true
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

// toggleTree 는 sidebar 를 여닫는다. `:tree` 가 쓴다.
//
// 닫을 때 트리를 버린다. 다시 열면 뿌리부터 새로 읽으므로 여닫는 것이 곧 새로고침이다.
// 여닫으면 편집 영역 너비가 달라져서 줄바꿈이 바뀌므로 활성 buffer 를 다시 맞춘다.
// 보고 있지 않은 tab 은 gt 로 갈 때 scrollTo 를 지나면서 알아서 맞는다.
func (e *editor) toggleTree() error {
	if e.sidebar.open {
		e.sidebar = sidebar{}
	} else {
		root, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "cannot find current directory")
		}

		e.sidebar = openSidebar(root)

		// 닫을 때 트리를 버렸으므로 여는 이 자리에서 보고 있는 파일 자리를 다시 펼친다.
		e.revealInSidebar(e.buffer().path)
		e.sidebar.scrollTo(e.sidebarHeight())
	}

	e.buffer().scrollTo(e.contentWidth(), e.textHeight())

	return nil
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

	rows := e.screenRows(textRows, mode, bottom)

	view := tea.NewView(strings.Join(rows, "\n"))

	view.MouseMode = tea.MouseModeCellMotion
	view.AltScreen = true

	// 터미널에 PC-101 자리의 키를 같이 달라고 한다. 한글 입력 상태에서 `ctrl+p` 가
	// `ctrl+ㅔ` 로 오는 것을 터미널이 되돌려 준다(ADR-0014).
	view.KeyboardEnhancements.ReportAlternateKeys = true

	if x, y, ok := buf.cursorScreenPos(e.contentWidth(), height); ok {
		// cursorScreenPos 는 본문 안에서의 좌표를 주므로 화면 좌표로 옮긴다.
		view.Cursor = tea.NewCursor(x+e.contentLeft(), y+tablineHeight)
		view.Cursor.Shape = shape
	}

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
	tabline, _ := e.tabline(e.textWidth())

	right := make([]string, 0, height)
	right = append(right, tabline)
	right = append(right, textRows...)
	for len(right) < height {
		right = append(right, "")
	}

	rows := right
	if e.sidebarVisible() {
		cells := e.sidebar.cells(height)

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

// tabline 은 편집 영역 맨 위 한 줄이다. 열린 파일과 지금 보고 있는 것을 보여준다.
//
// 보고 있는 tab 만 편집 내용과 같은 색이고 나머지는 반전이다. vim 의 TabLine/TabLineSel 과 같다.
// 활성 tab 이 아래 내용과 이어져 보이는 것이 tab 이라는 비유 자체다.
//
// width 는 화면 너비가 아니라 편집 영역 너비다. sidebar 가 열려 있으면 그만큼 좁다.
//
// spans 는 tab 마다 실제로 그려진 칸 범위 [start, end) 다. 넘쳐서 잘린 tab 은 빈 범위다.
// 클릭이 어느 tab 인지 여기서 같이 내준다 — 배치 계산을 두 벌 두면 `+`(dirty) 하나로
// 칸이 밀렸을 때 클릭이 옆 tab 으로 간다.
func (e editor) tabline(width int) (string, [][2]int) {
	line := strings.Builder{}
	col := 0

	// put 은 남은 칸만큼만 쓴다. 넘치는 부분은 버린다.
	// 색을 입힌 뒤에는 escape 가 섞여서 폭을 셀 수 없으므로 자르는 것이 먼저다.
	put := func(text string, active bool) {
		if width > 0 {
			text = text[:offsetAtScreenCol([]byte(text), width-col)]
		}
		if text == "" {
			return
		}

		if active {
			line.WriteString(text)
		} else {
			line.WriteString(reverse.Render(text))
		}
		col += screenColAt([]byte(text), len(text))
	}

	// 넘치면 잘린다. 활성 tab 이 오른쪽 끝에 있으면 안 보이게 되는데 아직 다루지 않는다.
	spans := make([][2]int, 0, len(e.buffers))
	for i, buf := range e.buffers {
		name := filepath.Base(buf.path)
		if buf.path == "" {
			name = "[No Name]"
		}
		if buf.dirty {
			name += "+"
		}

		if i > 0 {
			put("│", false)
		}

		// put 이 쓴 만큼만 그 tab 의 자리다. 잘렸으면 start 와 end 가 같아진다.
		start := col
		put(fmt.Sprintf(" %d %s ", i+1, name), i == e.active)
		spans = append(spans, [2]int{start, col})
	}

	// 남은 칸도 채워야 줄 전체가 한 덩어리로 보인다.
	if width > col {
		put(strings.Repeat(" ", width-col), false)
	}

	return line.String(), spans
}

// tabAt 은 편집 영역 기준이 아니라 화면 칸 x 에 그려진 tab 번호다.
// 구분선과 오른쪽 빈 칸, 잘려서 안 보이는 tab 자리는 ok 가 false 다.
func (e editor) tabAt(x int) (int, bool) {
	_, spans := e.tabline(e.textWidth())

	col := x - e.sidebarLeft()
	for i, span := range spans {
		if col >= span[0] && col < span[1] {
			return i, true
		}
	}

	return 0, false
}

// statusBar 는 화면 아래 두 줄이다. 위 줄은 mode 와 파일과 git, 아래 줄은 부르는 쪽이 정한다.
//
// 위 줄만 반전이다. 아래 줄은 vim 처럼 명령줄이라 배경을 그대로 둔다.
// `:` 를 칠 때 배경이 뜨지 않고 명령 결과와 오류도 평범한 글자로 읽힌다.
//
// 줄은 sidebar 아래까지 화면 끝에서 끝까지 이어지지만 글자는 편집 영역 아래에서 시작한다.
// 편집 영역에 딸린 내용이라 그 왼쪽 끝에 맞추고, 줄 자체는 tabline 과 달리 끊지 않는다 —
// 화면 맨 아래를 가로지르는 한 줄이라야 편집기 전체의 상태 표시로 읽힌다.
func (e editor) statusBar(mode, bottom string) []string {
	buf := e.buffers[e.active]

	path := buf.path
	if path == "" {
		path = "[No Name]"
	}
	if buf.dirty {
		path += " [+]"
	}

	// sidebar 아래를 빈 칸으로 지난다. 반전 안에 두어야 색이 왼쪽 끝까지 이어진다.
	indent := strings.Repeat(" ", e.sidebarLeft())
	width := e.textWidth()

	// Width 가 남은 칸을 공백으로 채워서 줄 끝까지 색이 간다.
	return []string{
		reverse.Width(e.width).Render(indent + e.withGit(truncateToWidth(mode+"  "+path, width))),
		indent + truncateToWidth(bottom, width),
	}
}

// withGit 은 statusBar 위 줄 오른쪽 끝에 저장소 상태를 붙인다.
//
// 붙일 칸이 없으면 그대로 둔다 — 지금 무슨 mode 인지와 어느 파일인지가 먼저다.
// 사이를 두 칸 이상 띄운다. 한 칸이면 파일 이름이 긴 tab 에서 경로에 붙은 글자처럼 읽힌다.
func (e editor) withGit(top string) string {
	label := e.git.label()
	if label == "" {
		return top
	}

	pad := e.textWidth() - screenColAt([]byte(top), len(top)) - screenColAt([]byte(label), len(label))
	if pad < 2 {
		return top
	}

	return top + strings.Repeat(" ", pad) + label
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
