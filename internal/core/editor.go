package core

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// editor 는 mode 가 바뀌어도 유지되는 상태다.
//
// mode 는 enum 이 아니라 화면 model 을 갈아끼워서 나타낸다(ADR-0002).
// mode 별 model 이 이것을 embed 해서 들고 다니고, 전환할 때 그대로 넘긴다.
type editor struct {
	buffers []Buffer
	active  int

	sidebar sidebar

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
	e.buffer().scrollTo(e.textWidth(), e.textHeight())
}

// nextTab, prevTab 은 활성 tab 을 옮긴다. 양끝에서 둘러 간다. vim 의 gt/gT 와 같다.
func (e *editor) nextTab() {
	e.active = (e.active + 1) % len(e.buffers)
}
func (e *editor) prevTab() {
	e.active = (e.active - 1 + len(e.buffers)) % len(e.buffers)
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

// tablineHeight 는 화면 위 tabline 이 차지하는 줄 수다(docs/spec.md).
const tablineHeight = 1

// statusBarHeight 는 화면 아래 statusBar 가 차지하는 줄 수다(docs/spec.md).
const statusBarHeight = 2

// textHeight 는 편집 내용을 그릴 수 있는 높이다.
// tabline 이 화면 위를, statusBar 가 화면 아래를 차지한다.
func (e editor) textHeight() int {
	return max(0, e.height-tablineHeight-statusBarHeight)
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

// textWidth 는 편집 내용을 그릴 수 있는 너비다. 줄을 어디서 접을지가 이 값으로 정해진다.
//
// e.width 는 터미널 너비이고 tabline·statusBar 처럼 화면 끝까지 칠하는 것만 그것을 쓴다.
// sidebar 가 없으면 둘이 같은 값이라 지금까지 구분할 필요가 없었다.
func (e editor) textWidth() int {
	return max(0, e.width-e.sidebarLeft())
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

	rows := []string{e.tabline()}

	// 화면보다 긴 줄은 visibleRows 가 이미 화면 행 여러 개로 나눠서 준다.
	for _, row := range buf.visibleRows(e.textWidth(), height) {
		rows = append(rows, expandTabs(buf.lines[row.line][row.start:row.end]))
	}

	// 파일이 화면보다 짧아도 statusBar 는 화면 아래에 붙어 있어야 한다.
	for len(rows) < tablineHeight+height {
		rows = append(rows, "")
	}
	rows = append(rows, e.statusBar(mode, bottom)...)

	view := tea.NewView(strings.Join(rows, "\n"))

	view.MouseMode = tea.MouseModeCellMotion
	view.AltScreen = true
	if x, y, ok := buf.cursorScreenPos(e.textWidth(), height); ok {
		// cursorScreenPos 는 편집 영역 안에서의 행을 주므로 tabline 만큼 내린다.
		view.Cursor = tea.NewCursor(x, y+tablineHeight)
		view.Cursor.Shape = shape
	}

	return view
}

// reverse 는 편집 내용과 구분되는 색이다. 색을 정하지 않고 터미널의 전경·배경을 뒤집기만 한다.
// 밝은 테마든 어두운 테마든 알아서 맞고 팔레트를 정할 필요가 없다(ADR-0004).
var reverse = lipgloss.NewStyle().Reverse(true)

// tabline 은 화면 맨 위 한 줄이다. 열린 파일과 지금 보고 있는 것을 보여준다.
//
// 보고 있는 tab 만 편집 내용과 같은 색이고 나머지는 반전이다. vim 의 TabLine/TabLineSel 과 같다.
// 활성 tab 이 아래 내용과 이어져 보이는 것이 tab 이라는 비유 자체다.
func (e editor) tabline() string {
	line := strings.Builder{}
	col := 0

	// put 은 남은 화면 칸만큼만 쓴다. 넘치는 부분은 버린다.
	// 색을 입힌 뒤에는 escape 가 섞여서 폭을 셀 수 없으므로 자르는 것이 먼저다.
	put := func(text string, active bool) {
		if e.width > 0 {
			text = text[:offsetAtScreenCol([]byte(text), e.width-col)]
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
		put(fmt.Sprintf(" %d %s ", i+1, name), i == e.active)
	}

	// 남은 칸도 채워야 줄 전체가 한 덩어리로 보인다.
	if e.width > col {
		put(strings.Repeat(" ", e.width-col), false)
	}

	return line.String()
}

// statusBar 는 화면 아래 두 줄이다. 위 줄은 mode 와 파일, 아래 줄은 부르는 쪽이 정한다.
//
// 위 줄만 반전이다. 아래 줄은 vim 처럼 명령줄이라 배경을 그대로 둔다.
// `:` 를 칠 때 배경이 뜨지 않고 명령 결과와 오류도 평범한 글자로 읽힌다.
func (e editor) statusBar(mode, bottom string) []string {
	buf := e.buffers[e.active]

	path := buf.path
	if path == "" {
		path = "[No Name]"
	}
	if buf.dirty {
		path += " [+]"
	}

	// Width 가 남은 칸을 공백으로 채워서 줄 끝까지 색이 간다.
	return []string{
		reverse.Width(e.width).Render(truncateToWidth(mode+"  "+path, e.width)),
		truncateToWidth(bottom, e.width),
	}
}

// position 은 커서 위치와 전체 줄 수다. normal/insert 의 statusBar 아래 줄이다.
// 줄과 칸은 1 부터 세고, 칸은 byte offset 이 아니라 화면 칸이다.
func (e editor) position() string {
	buf := e.buffers[e.active]
	col := screenColAt(buf.lines[buf.cursorLine], buf.cursorCol)

	return fmt.Sprintf("%d:%d  (%d 줄)", buf.cursorLine+1, col+1, len(buf.lines))
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

// expandTabs 는 화면 행의 tab 을 공백으로 펼친다.
//
// bubbletea 의 셀 렌더러는 폭 0 인 제어문자를 셀에 담지 못해 버린다.
// tab 을 그대로 넘기면 화면에서 들여쓰기가 사라진다. Go 소스는 tab 들여쓰기라 바로 드러난다.
// 행은 화면 왼쪽 끝에서 시작하므로 tab stop 도 행 시작 기준으로 센다.
func expandTabs(row []byte) string {
	if !bytes.ContainsRune(row, '\t') {
		return string(row)
	}

	out := strings.Builder{}
	col := 0
	for offset := 0; offset < len(row); {
		size, w := clusterAt(row, offset, col)

		if row[offset] == '\t' {
			out.WriteString(strings.Repeat(" ", w))
		} else {
			out.Write(row[offset : offset+size])
		}

		col += w
		offset += size
	}

	return out.String()
}
