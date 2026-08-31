package core

import (
	"bytes"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/syntax"
)

// markdown 표를 칸에 맞춰 다시 그리는 자리다 (ADR-0106).
//
// **어디까지가 표이고 칸을 어디서 가르는지는 강조가 아는 것을 그대로 쓴다**
// (syntax.MarkdownTableCells). 화면에서 칸으로 보이던 자리와 실제로 잘리는 자리가 달라지면
// 사람이 보고 있는 것과 다른 글이 저장된다.

// formatTablesIn 은 [from, to) 의 표를 맞추고 결과를 알린 채 normal 로 돌아간다.
//
// 팔레트의 「표 맞추기」와 `\mt` 가 같이 쓴다. 알림 문구가 한 자리에 있어야 부르는 길에
// 따라 다른 말이 나오지 않는다.
func formatTablesIn(e *editor, from, to int) (tea.Model, tea.Cmd) {
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return normalMode(e)
	}

	buf := e.activeBuffer()
	width := e.contentWidth()

	found, changed := buf.formatTables(from, to, width)

	switch {
	case found == 0:
		return normalModeMessage(e, "맞출 표가 없습니다")
	case changed == 0:
		return normalModeMessage(e, "표가 이미 맞춰져 있습니다")
	}

	buf.clampToNormal(width)
	e.scrollToCursor()

	return normalModeMessage(e, fmt.Sprintf("표 %s 개를 맞췄습니다", formatCount(changed)))
}

// tableAlign 은 구분줄이 말하는 칸의 정렬이다.
//
// tableNone 과 tableLeft 는 글을 붙이는 쪽이 같고 구분줄에 적는 모양만 다르다. 적어 두지
// 않은 표에 `:` 를 새로 달지 않으려고 가른다.
type tableAlign int

const (
	tableNone tableAlign = iota
	tableLeft
	tableRight
	tableCenter
)

// formatTables 는 [from, to) 에 걸친 markdown 표를 다 맞춘다.
//
// found 는 찾은 표의 수이고 changed 는 그중 모양이 달라진 것의 수다. 둘을 가르는 것은
// 「표가 없다」와 「이미 맞춰져 있다」가 다른 말이기 때문이다.
//
// **범위에 걸치기만 하면 그 표를 통째로 맞춘다.** 고른 범위가 표의 가운데를 자를 때
// 그 안쪽만 맞추면 한 표의 위아래가 서로 다른 폭이 된다. 표는 칸이 세로로 서야 표다.
func (buf *Buffer) formatTables(from, to, width int) (found, changed int) {
	// **담아둔 문맥을 쓰지 않고 여기서 다시 훑는다.** 담아둔 것은 화면 아래를 비워 두고
	// (lexSyntaxTo) 편집 뒤에는 수렴한 자리에서 멈춰서, 파일 끝까지 차 있다는 보장이 없다.
	// 없는 자리를 「표가 아니다」로 읽으면 아래쪽 표가 조용히 빠진다. 사람이 한 번 부르는
	// 명령이라 파일을 한 번 더 훑는 값이 눈에 띄지 않는다.
	state := buf.language.State()
	if state == nil {
		return 0, 0
	}

	states := make([]syntax.State, len(buf.lines))
	for at, line := range buf.lines {
		states[at] = state
		_, state = state.Lex(line)
	}

	next := make([][]byte, len(buf.lines))
	copy(next, buf.lines)

	first, last := -1, -1

	for at := 0; at < len(buf.lines); {
		rows, delimiter, end := tableAt(buf.lines, states, at)
		if end == at {
			at++

			continue
		}

		// 범위에 걸치지 않는 표는 지나간다. 걸치기만 하면 통째로 맞춘다.
		if end <= from || at >= to {
			at = end

			continue
		}

		found++

		// 들여쓴 표는 그 들여쓰기를 지킨다. 목록 안의 표가 그렇고, 이 저장소의 목록은
		// tab 으로 들여쓴다 — 빈 칸만 떼면 tab 이 칸 글에 딸려 들어가 폭이 어긋난다.
		//
		// 덩이의 첫 줄 것을 모든 행에 쓴다. 행마다 다르면 칸이 세로로 서지 않는다.
		head := buf.lines[at]
		indent := head[:len(head)-len(bytes.TrimLeft(head, " \t"))]

		moved := false
		for i, line := range renderTable(rows, delimiter, string(indent)) {
			if bytes.Equal(line, buf.lines[at+i]) {
				continue
			}

			next[at+i] = line
			moved = true

			if first < 0 {
				first = at + i
			}
			last = at + i
		}

		if moved {
			changed++
		}

		at = end
	}

	if first < 0 {
		return found, 0
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
	// 줄 수가 그대로라 growEdit 은 부르지 않는다(trimTrailingSpace 와 같은 손이다).
	buf.endEdit()
	buf.beginEdit(first, last-first+1)
	buf.replaceLines(first, last-first+1, next[first:last+1])

	// 커서가 잘려나간 자리에 서 있었으면 줄 끝으로 당긴다.
	buf.cursorCol = min(buf.cursorCol, len(buf.lines[buf.cursorLine]))
	buf.updateDesiredCol(width)

	buf.endEdit()

	return found, changed
}

// tableAt 은 at 에서 시작하는 표다. 표가 아니면 end 가 at 그대로다.
//
// 표는 `|` 로 시작하는 줄이 이어지는 만큼이다. 구분줄은 그중 첫 번째이고, 없으면 -1 이다.
// states 는 줄을 **시작한** 문맥이라 코드펜스 안의 `|` 줄이 여기서 갈린다.
func tableAt(lines [][]byte, states []syntax.State, at int) (rows [][]string, delimiter, end int) {
	delimiter = -1

	for end = at; end < len(lines); end++ {
		cells, isDelimiter, ok := syntax.MarkdownTableCells(states[end], lines[end])
		if !ok {
			break
		}

		if isDelimiter && delimiter < 0 {
			delimiter = end - at
		}

		rows = append(rows, cells)
	}

	return rows, delimiter, end
}

// renderTable 은 표 한 덩이를 다시 그린다. 줄 수는 그대로다.
//
// **줄을 더하지도 빼지도 않는다.** 구분줄이 없는 덩이에 그것을 만들어 넣으면 되돌리기가
// 줄 수를 바꾸고, 무엇보다 사람이 적지 않은 줄이 생긴다.
func renderTable(rows [][]string, delimiter int, indent string) [][]byte {
	columns := 0
	for _, row := range rows {
		columns = max(columns, len(row))
	}

	aligns := make([]tableAlign, columns)
	if delimiter >= 0 {
		for at, cell := range rows[delimiter] {
			aligns[at] = tableAlignOf(cell)
		}
	}

	// 폭은 칸 글의 **화면 폭**이다. 한글 한 글자가 두 칸이라 byte 나 글자 수로 세면
	// 한글이 든 표가 어긋난다. 그 자는 터미널에서 재서 맞춰 두었다(ADR-0020, ADR-0072).
	widths := make([]int, columns)
	for at, row := range rows {
		if at == delimiter {
			continue
		}

		for column, cell := range row {
			widths[column] = max(widths[column], screenWidthOf(cell))
		}
	}

	// 빈 칸이라도 구분줄이 `---` 셋은 되어야 표로 읽힌다.
	for at := range widths {
		widths[at] = max(widths[at], 1)
	}

	out := make([][]byte, 0, len(rows))
	for at, row := range rows {
		if at == delimiter {
			out = append(out, renderTableDelimiter(widths, aligns, indent))

			continue
		}

		out = append(out, renderTableRow(row, widths, aligns, indent))
	}

	return out
}

// renderTableRow 는 칸 글이 든 행 하나다.
func renderTableRow(row []string, widths []int, aligns []tableAlign, indent string) []byte {
	out := strings.Builder{}
	out.WriteString(indent)
	out.WriteByte('|')

	for at, width := range widths {
		cell := ""
		if at < len(row) {
			cell = row[at]
		}

		out.WriteByte(' ')
		out.WriteString(padCell(cell, width, aligns[at]))
		out.WriteString(" |")
	}

	return []byte(out.String())
}

// renderTableDelimiter 는 구분줄이다. 칸 글 양옆의 빈 칸 자리까지 `-` 로 채운다.
func renderTableDelimiter(widths []int, aligns []tableAlign, indent string) []byte {
	out := strings.Builder{}
	out.WriteString(indent)
	out.WriteByte('|')

	for at, width := range widths {
		switch aligns[at] {
		case tableLeft:
			out.WriteString(":" + strings.Repeat("-", width+1))
		case tableRight:
			out.WriteString(strings.Repeat("-", width+1) + ":")
		case tableCenter:
			out.WriteString(":" + strings.Repeat("-", width) + ":")
		default:
			out.WriteString(strings.Repeat("-", width+2))
		}

		out.WriteByte('|')
	}

	return []byte(out.String())
}

// padCell 은 칸 글을 그 폭에 맞춰 민다. 글이 폭보다 넓으면 그대로 둔다.
func padCell(cell string, width int, align tableAlign) string {
	pad := width - screenWidthOf(cell)
	if pad < 1 {
		return cell
	}

	switch align {
	case tableRight:
		return strings.Repeat(" ", pad) + cell
	case tableCenter:
		return strings.Repeat(" ", pad/2) + cell + strings.Repeat(" ", pad-pad/2)
	}

	return cell + strings.Repeat(" ", pad)
}

// tableAlignOf 는 구분줄의 칸 하나가 말하는 정렬이다.
func tableAlignOf(cell string) tableAlign {
	left := strings.HasPrefix(cell, ":")
	right := strings.HasSuffix(cell, ":")

	switch {
	case left && right:
		return tableCenter
	case right:
		return tableRight
	case left:
		return tableLeft
	}

	return tableNone
}
