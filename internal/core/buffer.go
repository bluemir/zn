package core

import (
	"bytes"
	"os"

	"github.com/charmbracelet/x/ansi"
	"github.com/cockroachdb/errors"
)

// lineEnding 은 파일의 줄끝 형식이다. 읽을 때 판정해서 저장할 때 그대로 되돌린다.
type lineEnding int

const (
	lineEndingLF lineEnding = iota
	lineEndingCRLF
)

func (e lineEnding) bytes() []byte {
	if e == lineEndingCRLF {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// Buffer 는 파일 하나에 대응 한다.
//
// data 는 파일을 통째로 읽은 것으로 읽은 뒤에는 바꾸지 않는다.
// lines 는 data 를 가리키는 subslice 라 줄 내용을 복사하지 않는다.
// 편집한 줄만 새로 할당한 []byte 로 갈아끼운다. (ADR-0001)
//
// lines 의 각 줄은 줄끝 문자를 담지 않는다. CRLF 파일이어도 \r 을 뺀 범위를 가리키므로
// 편집과 렌더는 줄끝을 신경 쓰지 않는다. 줄끝은 저장할 때 다시 끼워 넣는다.
type Buffer struct {
	path string

	data  []byte
	lines [][]byte

	lineEnding lineEnding

	// finalLineEnding 은 파일 마지막 줄이 줄끝 문자로 끝났는지다.
	// git 이 `\ No newline at end of file` 로 잡아내는 차이라 원본대로 되돌린다.
	finalLineEnding bool

	// 아래는 파일 내용이 아니라 이 파일을 어떻게 보고 있는지다.
	// tab 을 오갈 때 파일별로 유지되어야 하므로 Buffer 가 들고 있다.
	// 화면 분할을 도입하면 같은 파일에 커서가 둘이 되므로 그때는 밖으로 빼야 한다.

	cursorLine int // lines 의 index
	cursorCol  int // lines[cursorLine] 안의 byte offset

	// desiredCol 은 위아래로 움직일 때 유지할 화면 칸이다.
	// 짧은 줄을 지나가도 원래 열로 돌아오게 하려면 실제 열과 따로 기억해야 한다.
	// wrap 된 줄에서도 자연스럽게 움직이도록 화면 행 안에서의 칸으로 센다.
	desiredCol int

	// top, topRow 는 화면 최상단에 그릴 위치다. 커서에서 파생할 수 없다.
	// 커서를 두고 화면만 움직이는 동작이 있고, 커서가 화면 안에 있는 동안은 화면이 움직이지 않아야 한다.
	// 줄 하나가 화면 행 여러 개가 될 수 있어서 줄 번호만으로는 부족하다.
	top    int // lines 의 index
	topRow int // 그 줄의 몇 번째 wrap 행부터 그리는지

	// undo, redo 는 되돌리기 이력이다.
	// editing 은 열린 구간이 있는지다. 이어지는 타이핑을 한 항목으로 모은다.
	undo    []edit
	redo    []edit
	editing bool

	// dirty 는 마지막 저장 이후 바뀐 것이 있는지다.
	// undo 로 저장 시점 내용까지 되돌려도 켜진 채로 남는다. vim 은 이때 꺼주는데 아직 거기까지 하지 않는다.
	dirty bool
}

// edit 은 되돌릴 수 있는 변경 하나다. lines 의 [at, at+count) 를 before 로 바꾸면 되돌아간다.
//
// before 는 그 자리에 원래 있던 줄들을 참조로만 담는다. 갈아끼우기 방식이라 옛 줄이 그대로
// 살아있어서 내용 복사가 일어나지 않는다(ADR-0001).
//
// undo 할 때 지금 내용으로 역방향 edit 을 만들어 redo 에 넣는다. 그래서 한 型 으로 양방향을 다룬다.
type edit struct {
	at     int      // lines 의 시작 index
	before [][]byte // 그 자리에 원래 있던 줄들
	count  int      // 편집 후 그 자리를 차지하는 줄 수

	cursorLine, cursorCol int // 되돌린 뒤 커서를 놓을 자리
}

// screenRow 는 화면 한 행에 그려지는 논리 줄의 일부다.
type screenRow struct {
	line  int // lines 의 index
	start int // 줄 안의 byte offset. 포함
	end   int // 줄 안의 byte offset. 제외
}

// newEmptyBuffer 는 파일 없이 시작하는 빈 Buffer 를 만든다.
func newEmptyBuffer(path string) Buffer {
	return Buffer{
		path:            path,
		lines:           [][]byte{{}},
		finalLineEnding: true, // 새 파일은 줄끝으로 끝낸다
	}
}

// OpenBuffer 는 파일을 읽어서 Buffer 로 만든다.
// 파일이 없으면 빈 줄 하나짜리 새 Buffer 를 만든다.
func OpenBuffer(path string) (Buffer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newEmptyBuffer(path), nil
	}
	if err != nil {
		return Buffer{}, errors.Wrapf(err, "cannot read %s", path)
	}

	return newBuffer(path, data), nil
}

func newBuffer(path string, data []byte) Buffer {
	buf := Buffer{
		path:       path,
		data:       data,
		lineEnding: detectLineEnding(data),
	}

	// 마지막 줄끝은 빈 줄이 아니라 "줄끝으로 끝났다" 는 사실이므로 떼어내고 기록한다.
	rest := data
	if len(rest) > 0 && rest[len(rest)-1] == '\n' {
		buf.finalLineEnding = true
		rest = rest[:len(rest)-1]
		if len(rest) > 0 && rest[len(rest)-1] == '\r' {
			rest = rest[:len(rest)-1]
		}
	}

	buf.lines = bytes.Split(rest, []byte{'\n'})
	for i, line := range buf.lines {
		if len(line) > 0 && line[len(line)-1] == '\r' {
			buf.lines[i] = line[:len(line)-1]
		}
	}

	return buf
}

// detectLineEnding 은 첫 줄의 줄끝으로 파일 전체의 형식을 판정한다.
// 줄마다 섞여 있는 파일은 저장할 때 이 형식으로 통일된다.
func detectLineEnding(data []byte) lineEnding {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		return lineEndingCRLF
	}
	return lineEndingLF
}

// tabWidth 는 tab 이 다음 몇 칸 경계까지 밀어내는지다.
// vim 의 tabstop 기본값이자 터미널 기본 tab stop 이 8 이다.
const tabWidth = 8

// clusterAt 은 offset 에서 시작하는 grapheme cluster 의 byte 길이와 화면 폭을 돌려준다.
// col 은 그 글자가 시작하는 화면 칸이다. tab 이 시작 위치에 따라 폭이 달라서 필요하다.
//
// 커서 이동과 줄바꿈은 rune 이 아니라 이 단위로 해야 한다.
// NFD 한글(ᄒ+ᅡ+ᆫ), 결합 악센트(e+́), 이모지 조합(가족 이모지, 국기) 은 rune 여러 개가 한 글자다.
// 폭과 경계를 같은 함수에서 얻어야 "한 글자" 와 "그 폭" 이 어긋나지 않는다.
// lipgloss.Width 도 결국 이 경로를 쓴다.
func clusterAt(line []byte, offset, col int) (size, width int) {
	// ansi 는 tab 을 폭 0 으로 본다. 그대로 두면 들여쓰기가 화면에서 사라진다.
	if line[offset] == '\t' {
		return 1, tabWidth - col%tabWidth
	}

	cluster, w := ansi.FirstGraphemeCluster(line[offset:], ansi.GraphemeWidth)

	// 깨진 UTF-8 에서 0 이 나오면 진행하지 못하고 무한 반복한다.
	if len(cluster) < 1 {
		return 1, 1
	}
	return len(cluster), w
}

// clusterSize 는 폭이 필요 없을 때 쓴다. byte 길이는 시작 칸과 무관하다.
func clusterSize(line []byte, offset int) int {
	size, _ := clusterAt(line, offset, 0)
	return size
}

// screenColAt 은 offset 까지의 화면 칸 수다.
// 한글은 두 칸, tab 은 다음 tab stop 까지라서 byte offset 과 다르다.
func screenColAt(line []byte, offset int) int {
	col := 0
	for i := 0; i < offset && i < len(line); {
		size, w := clusterAt(line, i, col)
		col += w
		i += size
	}
	return col
}

// offsetAtScreenCol 은 화면 칸 col 에 해당하는 byte offset 을 찾는다.
// col 이 여러 칸을 쓰는 글자의 중간이면 그 글자의 시작으로 맞춘다.
func offsetAtScreenCol(line []byte, col int) int {
	width := 0
	for offset := 0; offset < len(line); {
		size, w := clusterAt(line, offset, width)
		if width+w > col {
			return offset
		}
		width += w
		offset += size
	}
	return len(line)
}

// prevClusterStart 는 offset 직전 글자의 시작을 돌려준다. from 은 글자 경계여야 한다.
//
// grapheme cluster 는 뒤에서 앞으로 읽을 수 없어서 알려진 경계에서부터 훑는다.
// 행 시작이 항상 경계이므로 훑는 범위는 화면 한 행으로 묶인다.
func prevClusterStart(line []byte, from, offset int) int {
	prev := from
	for i := from; i < offset && i < len(line); {
		prev = i
		i += clusterSize(line, i)
	}
	return prev
}

// wrapOffsets 는 줄이 width 칸에서 끊기는 지점, 즉 각 화면 행의 시작 byte offset 을 돌려준다.
// 가로 스크롤 대신 줄바꿈으로 보여주므로 화면보다 긴 줄은 화면 행 여러 개가 된다.
// 빈 줄도 행 하나를 차지하므로 항상 최소 하나를 돌려준다.
func wrapOffsets(line []byte, width int) []int {
	if width < 1 {
		return []int{0}
	}

	offsets := []int{0}
	col := 0
	for offset := 0; offset < len(line); {
		// col 은 행이 바뀔 때 0 으로 돌아간다. 화면 행이 왼쪽 끝에서 시작하므로 tab stop 도 거기부터다.
		size, w := clusterAt(line, offset, col)

		// col > 0 조건이 없으면 width 보다 넓은 글자에서 같은 지점을 계속 끊는다.
		if col > 0 && col+w > width {
			offsets = append(offsets, offset)
			col = 0
		}

		col += w
		offset += size
	}

	return offsets
}

// rowIndexAt 은 offset 이 속한 화면 행의 index 를 돌려준다.
func rowIndexAt(offsets []int, offset int) int {
	for i := len(offsets) - 1; i > 0; i-- {
		if offset >= offsets[i] {
			return i
		}
	}
	return 0
}

// rowRange 는 화면 행 하나가 담는 byte 범위를 돌려준다.
func rowRange(line []byte, offsets []int, row int) (int, int) {
	start := offsets[row]
	if row+1 < len(offsets) {
		return start, offsets[row+1]
	}
	return start, len(line)
}

// prevOffset 은 현재 줄에서 offset 직전 글자의 시작을 돌려준다.
func (buf Buffer) prevOffset(offset, width int) int {
	if offset == 0 {
		return 0
	}

	line := buf.lines[buf.cursorLine]
	offsets := wrapOffsets(line, width)
	row := rowIndexAt(offsets, offset)

	// 행 시작에서 왼쪽으로 가면 앞 행의 마지막 글자다.
	from := offsets[row]
	if from == offset && row > 0 {
		from = offsets[row-1]
	}

	return prevClusterStart(line, from, offset)
}

// clampToNormal 은 커서를 마지막 글자 위로 끌어온다.
//
// normal mode 의 커서는 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
// 그 칸은 insert mode 에서만 갈 수 있다. 빈 줄은 그대로 0 이다.
//
// desiredCol 은 건드리지 않는다. 짧은 줄을 지나가도 원래 칸으로 돌아와야 한다.
func (buf *Buffer) clampToNormal(width int) {
	line := buf.lines[buf.cursorLine]
	if len(line) == 0 || buf.cursorCol < len(line) {
		return
	}

	buf.cursorCol = buf.prevOffset(len(line), width)
}

// moveLeft, moveRight 는 grapheme cluster 단위로 n 글자 움직인다.
// rune 단위로 움직이면 결합 문자의 중간에 커서가 선다.
//
// 줄 양끝에 닿으면 남은 횟수를 버리고 거기서 멈춘다. vim 처럼 앞뒤 줄로 넘어가지 않는다.
func (buf *Buffer) moveLeft(n, width int) {
	if buf.cursorCol == 0 {
		return
	}

	for range n {
		if buf.cursorCol == 0 {
			break
		}

		buf.cursorCol = buf.prevOffset(buf.cursorCol, width)
	}

	buf.updateDesiredCol(width)
}

func (buf *Buffer) moveRight(n, width int) {
	line := buf.lines[buf.cursorLine]
	if buf.cursorCol >= len(line) {
		return
	}

	for range n {
		if buf.cursorCol >= len(line) {
			break
		}

		buf.cursorCol += clusterSize(line, buf.cursorCol)
	}

	buf.updateDesiredCol(width)
}

// updateDesiredCol 은 좌우로 움직인 뒤 유지할 화면 칸을 갱신한다.
// 화면 행 안에서의 칸이라 wrap 된 줄에서도 위아래 이동이 보이는 대로 움직인다.
func (buf *Buffer) updateDesiredCol(width int) {
	line := buf.lines[buf.cursorLine]
	offsets := wrapOffsets(line, width)
	start, _ := rowRange(line, offsets, rowIndexAt(offsets, buf.cursorCol))

	buf.desiredCol = screenColAt(line[start:], buf.cursorCol-start)
}

// moveUp, moveDown 은 화면 행 단위로 움직인다. wrap 된 줄에서는 같은 줄 안에서 행만 옮긴다.
// desiredCol 은 건드리지 않는다. 짧은 행을 지나가도 원래 칸으로 돌아오는 것이 그 필드의 목적이다.
func (buf *Buffer) moveUp(n, width int) {
	for range n {
		buf.moveUpRow(width)
	}
}

func (buf *Buffer) moveDown(n, width int) {
	for range n {
		buf.moveDownRow(width)
	}
}

func (buf *Buffer) moveUpRow(width int) {
	offsets := wrapOffsets(buf.lines[buf.cursorLine], width)

	if row := rowIndexAt(offsets, buf.cursorCol); row > 0 {
		buf.placeCursorInRow(buf.cursorLine, offsets, row-1, width)
		return
	}
	if buf.cursorLine == 0 {
		return
	}

	prev := buf.cursorLine - 1
	prevOffsets := wrapOffsets(buf.lines[prev], width)
	buf.placeCursorInRow(prev, prevOffsets, len(prevOffsets)-1, width)
}

func (buf *Buffer) moveDownRow(width int) {
	offsets := wrapOffsets(buf.lines[buf.cursorLine], width)

	if row := rowIndexAt(offsets, buf.cursorCol); row+1 < len(offsets) {
		buf.placeCursorInRow(buf.cursorLine, offsets, row+1, width)
		return
	}
	if buf.cursorLine+1 >= len(buf.lines) {
		return
	}

	next := buf.cursorLine + 1
	buf.placeCursorInRow(next, wrapOffsets(buf.lines[next], width), 0, width)
}

// moveLineStart, moveLineFirstNonBlank 는 줄 안에서 왼쪽으로 간다. vim 의 0, ^ 다.
func (buf *Buffer) moveLineStart(width int) {
	buf.cursorCol = 0
	buf.updateDesiredCol(width)
}

// moveLineFirstNonBlank 는 들여쓰기를 건너뛴 첫 글자로 간다.
// 공백뿐인 줄은 줄 끝이 되고, clampToNormal 이 마지막 글자 위로 끌어온다. vim 과 같다.
func (buf *Buffer) moveLineFirstNonBlank(width int) {
	line := buf.lines[buf.cursorLine]

	col := 0
	for col < len(line) && (line[col] == ' ' || line[col] == '\t') {
		col++
	}

	buf.cursorCol = col
	buf.updateDesiredCol(width)
}

// moveLineEnd 는 줄 끝으로 간다. vim 의 $ 다.
// count 는 되풀이가 아니라 줄 수다 — `3$` 는 두 줄 아래의 줄 끝이다.
func (buf *Buffer) moveLineEnd(n, width int) {
	buf.cursorLine = min(buf.cursorLine+n-1, len(buf.lines)-1)
	buf.cursorCol = len(buf.lines[buf.cursorLine])
	buf.updateDesiredCol(width)
}

// moveToLine 은 그 줄의 첫 글자로 간다. vim 의 gg, G 다. 범위를 넘으면 양끝으로 맞춘다.
func (buf *Buffer) moveToLine(line, width int) {
	buf.cursorLine = min(max(line, 0), len(buf.lines)-1)
	buf.moveLineFirstNonBlank(width)
}

// moveUpLine, moveDownLine 은 논리 줄 단위로 움직인다.
// wrap 되어 화면 행이 여럿인 줄도 한 번에 건넌다. vim 의 j/k 다.
//
// 화면 행 단위인 moveUp/moveDown 과 나뉜다. vim 에서 j/k 와 gj/gk 가 나뉜 것과 같다(ADR-0006).
// 줄 index 를 직접 옮기므로 반복하지 않고 한 번에 끝난다.
func (buf *Buffer) moveUpLine(n int) {
	buf.placeCursorInLine(max(buf.cursorLine-n, 0))
}

func (buf *Buffer) moveDownLine(n int) {
	buf.placeCursorInLine(min(buf.cursorLine+n, len(buf.lines)-1))
}

// placeCursorInLine 은 커서를 그 줄의 desiredCol 칸에 놓는다.
//
// desiredCol 은 화면 행 안에서 센 칸이라, wrap 된 줄의 둘째 행 이후에서 넘어오면
// vim 과 칸이 다르다. wrap 되지 않은 줄에서는 줄 시작에서 센 칸과 같아서 vim 과 같다.
func (buf *Buffer) placeCursorInLine(line int) {
	buf.cursorLine = line
	buf.cursorCol = offsetAtScreenCol(buf.lines[line], buf.desiredCol)
}

// placeCursorInRow 는 커서를 지정한 화면 행의 desiredCol 칸으로 옮긴다.
func (buf *Buffer) placeCursorInRow(line int, offsets []int, row, width int) {
	start, end := rowRange(buf.lines[line], offsets, row)

	// 행 끝 칸을 넘어가면 다음 행의 시작 offset 이 되어 한 행을 더 내려간 것처럼 보인다.
	col := buf.desiredCol
	if width > 0 && row+1 < len(offsets) {
		col = min(col, width-1)
	}

	buf.cursorLine = line
	buf.cursorCol = start + offsetAtScreenCol(buf.lines[line][start:end], col)
}

// scrollTo 는 커서가 화면 안에 들어오도록 top 을 최소한으로 움직인다.
// 커서가 이미 화면 안이면 아무것도 하지 않는다.
func (buf *Buffer) scrollTo(width, height int) {
	if height < 1 {
		return
	}

	buf.clampTop(width)

	cursorRow := rowIndexAt(wrapOffsets(buf.lines[buf.cursorLine], width), buf.cursorCol)

	// 위로 벗어났으면 커서 행을 최상단으로 올린다.
	if rowBefore(buf.cursorLine, cursorRow, buf.top, buf.topRow) {
		buf.top, buf.topRow = buf.cursorLine, cursorRow
		return
	}

	// 커서에서 height-1 행 위로 올라간 지점이 top 의 하한이다.
	// 뒤에서 앞으로 세기 때문에 화면 높이만큼만 훑는다.
	limitLine, limitRow := buf.retreatRows(buf.cursorLine, cursorRow, height-1, width)
	if rowBefore(buf.top, buf.topRow, limitLine, limitRow) {
		buf.top, buf.topRow = limitLine, limitRow
	}
}

// clampTop 은 top, topRow 를 지금 너비에서 실제로 있는 화면 행으로 맞춘다.
//
// topRow 는 그 줄이 몇 번째 wrap 행부터 그려지는지인데, 화면이 넓어지면 그 줄의 wrap 행 수가
// 줄어서 예전 topRow 가 없는 행을 가리키게 된다. 그대로 두면 visibleRows 가 그 줄을 통째로
// 건너뛰어서 panic 없이 화면이 한 줄씩 밀린다.
//
// 폭이 바뀌는 경로가 여럿(터미널 리사이즈, tab 전환, sidebar 여닫기)이라
// 부르는 쪽마다 챙기지 않고 scrollTo 안에서 한 번에 맞춘다.
func (buf *Buffer) clampTop(width int) {
	buf.top = min(buf.top, len(buf.lines)-1)
	buf.topRow = min(buf.topRow, len(wrapOffsets(buf.lines[buf.top], width))-1)
}

// rowBefore 는 화면 행 (line1,row1) 이 (line2,row2) 보다 위인지 본다.
func rowBefore(line1, row1, line2, row2 int) bool {
	return line1 < line2 || (line1 == line2 && row1 < row2)
}

// retreatRows 는 (line,row) 에서 화면 행 n 개 위로 올라간 위치를 돌려준다.
func (buf Buffer) retreatRows(line, row, n, width int) (int, int) {
	for range n {
		switch {
		case row > 0:
			row--
		case line > 0:
			line--
			row = len(wrapOffsets(buf.lines[line], width)) - 1
		default:
			return 0, 0
		}
	}
	return line, row
}

// visibleRows 는 화면에 그릴 행들을 위에서부터 돌려준다.
func (buf Buffer) visibleRows(width, height int) []screenRow {
	if height < 1 {
		return nil
	}

	rows := make([]screenRow, 0, height)
	line, row := buf.top, buf.topRow

	for len(rows) < height && line < len(buf.lines) {
		offsets := wrapOffsets(buf.lines[line], width)
		if row >= len(offsets) {
			line, row = line+1, 0
			continue
		}

		start, end := rowRange(buf.lines[line], offsets, row)
		rows = append(rows, screenRow{line: line, start: start, end: end})
		row++
	}

	return rows
}

// cursorScreenPos 는 커서의 화면 좌표를 돌려준다. 커서가 화면 밖이면 ok 가 false 다.
func (buf Buffer) cursorScreenPos(width, height int) (x, y int, ok bool) {
	line := buf.lines[buf.cursorLine]

	for y, row := range buf.visibleRows(width, height) {
		if row.line != buf.cursorLine || buf.cursorCol < row.start {
			continue
		}
		// 행 경계의 offset 은 앞 행의 끝이 아니라 다음 행의 시작으로 본다.
		// 줄 끝일 때만 마지막 행의 끝에 놓는다.
		if buf.cursorCol > row.end || (buf.cursorCol == row.end && row.end != len(line)) {
			continue
		}

		return screenColAt(line[row.start:row.end], buf.cursorCol-row.start), y, true
	}

	return 0, 0, false
}

// Save 는 buffer 를 파일에 쓴다.
// 줄끝 형식과 파일 끝 줄끝 유무는 읽었을 때 그대로 되돌린다.
func (buf *Buffer) Save() error {
	// :tabnew 로 만든 buffer 는 이름이 없어서 쓸 곳이 없다. vim 의 E32 와 같다.
	// 이름을 주는 방법(`:w <파일>`) 은 아직 없으므로 알리고 끝낸다.
	if buf.path == "" {
		return errors.New("파일 이름이 없습니다")
	}

	eol := buf.lineEnding.bytes()

	size := 0
	for _, line := range buf.lines {
		size += len(line) + len(eol)
	}

	out := make([]byte, 0, size)
	for i, line := range buf.lines {
		if i > 0 {
			out = append(out, eol...)
		}
		out = append(out, line...)
	}
	if buf.finalLineEnding {
		out = append(out, eol...)
	}

	// 이미 있는 파일은 원래 권한을 유지한다. 0644 는 새로 만들 때만 쓰인다.
	if err := os.WriteFile(buf.path, out, 0644); err != nil {
		return errors.Wrapf(err, "cannot write %s", buf.path)
	}

	buf.dirty = false

	return nil
}
