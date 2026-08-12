package core

import (
	"bytes"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// editor 는 mode 가 바뀌어도 유지되는 상태다.
//
// mode 는 enum 이 아니라 화면 model 을 갈아끼워서 나타낸다(ADR-0002).
// mode 별 model 이 이것을 embed 해서 들고 다니고, 전환할 때 그대로 넘긴다.
type editor struct {
	buffers []Buffer
	active  int

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
	e.buffer().scrollTo(e.width, e.textHeight())
}

// statusBarHeight 는 화면 아래 statusBar 가 차지하는 줄 수다(docs/spec.md).
const statusBarHeight = 2

// textHeight 는 편집 내용을 그릴 수 있는 높이다. statusBar 가 화면 아래를 차지한다.
func (e editor) textHeight() int {
	return max(0, e.height-statusBarHeight)
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
	rows := []string{}
	for _, row := range buf.visibleRows(e.width, height) {
		rows = append(rows, expandTabs(buf.lines[row.line][row.start:row.end]))
	}

	// 파일이 화면보다 짧아도 statusBar 는 화면 아래에 붙어 있어야 한다.
	for len(rows) < height {
		rows = append(rows, "")
	}
	rows = append(rows, e.statusBar(mode, bottom)...)

	view := tea.NewView(strings.Join(rows, "\n"))

	view.MouseMode = tea.MouseModeCellMotion
	view.AltScreen = true
	if x, y, ok := buf.cursorScreenPos(e.width, height); ok {
		view.Cursor = tea.NewCursor(x, y)
		view.Cursor.Shape = shape
	}

	return view
}

// statusBar 는 화면 아래 두 줄이다. 위 줄은 mode 와 파일, 아래 줄은 부르는 쪽이 정한다.
func (e editor) statusBar(mode, bottom string) []string {
	buf := e.buffers[e.active]

	path := buf.path
	if path == "" {
		path = "[No Name]"
	}
	if buf.dirty {
		path += " [+]"
	}

	return []string{
		truncateToWidth(mode+"  "+path, e.width),
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
