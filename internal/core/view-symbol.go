package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/assets"

	"github.com/bluemir/zn/internal/textarea"
)

// drawer 크기다. 팔레트의 수치와 같은 자리에 둔다(docs/spec.md).
const (
	symbolCellWidth     = 4 // 격자 한 칸. 두 칸짜리 글자와 사이 공백이 들어간다
	symbolMaxGridRows   = 4 // 격자에 보여줄 최대 행 수
	symbolMinColumns    = 4 // 이보다 좁으면 격자라고 할 것이 없다
	symbolMinTextHeight = 3 // drawer 를 열고도 편집 영역에 남아야 할 행 수
)

// symbolFrame 은 drawer 가 격자 말고 쓰는 행 수다.
// 테두리 둘, 입력줄, 가름줄, 고른 것의 이름줄이다.
const symbolFrame = 5

// symbolGridRows 는 격자에 쓸 수 있는 행 수다. 편집 영역을 다 먹지 않는 선까지다.
func (e editor) symbolGridRows() int {
	return min(symbolMaxGridRows, e.textAndDrawerHeight()-symbolFrame-symbolMinTextHeight)
}

// symbolColumns 는 격자 한 행에 들어가는 칸 수다.
func (e editor) symbolColumns() int {
	return (e.textWidth() - 4) / symbolCellWidth
}

// symbolFits 는 drawer 를 그릴 수 있는지다. paletteFits 와 같은 종류의 방어다.
func (e editor) symbolFits() bool {
	return e.symbolGridRows() >= 1 && e.symbolColumns() >= symbolMinColumns
}

// symbolDrawerHeight 는 drawer 가 편집 영역에서 가져갈 행 수다.
func (e editor) symbolDrawerHeight() int {
	return e.symbolGridRows() + symbolFrame
}

// symbolMode 는 `:symbols` 와 팔레트의 「특수문자 넣기」로 여는 하단 drawer 다.
//
// 편집 화면 아래에 붙어서 편집 영역의 행을 가져간다. 얹지 않는 것은 넣는 자리인 커서가
// 그 밑에 가려지면 안 되기 때문이다 — 연속으로 넣는 것이 이 mode 의 쓰임새다(ADR-0056).
//
// 커서는 `a` 와 같이 글자 뒤로 간다. 팔레트는 어디서 열렸는지 기억하지 않으므로(ADR-0011)
// 여기 오는 커서는 늘 normal 자리다. insert 에서 연 경우는 그쪽에서 맞춰 놓고 넘긴다.
func symbolMode(e *editor) (tea.Model, tea.Cmd) {
	// 넣을 커서가 없으면 열지 않는다. 하단 drawer 는 편집 맥락에 딸린 자리다(ADR-0064).
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	// 읽기 전용 파일은 고치지 않는다(readonly.go). 알림은 그쪽이 적는다.
	if e.refuseReadOnly() {
		return normalMode(e)
	}
	if !e.symbolFits() {
		return normalModeMessage(e, "화면이 좁아 특수문자 창을 열 수 없습니다")
	}

	// 큐레이션 표는 곧바로 쓴다. 유니코드 전체를 훑는 것은 백그라운드로 미룬다 —
	// 한국어로 찾는 것은 이 표가 하는 일이라 훑기를 기다릴 이유가 없다.
	if len(e.symbols) == 0 {
		e.symbols = assets.CuratedSymbols
	}

	cmd := tea.Cmd(nil)
	if !e.symbolsIndexed {
		cmd = e.startJob("유니코드 훑기", nil, indexSymbols)
	}

	e.setDrawerHeight(e.symbolDrawerHeight())

	e.activeBuffer().MoveRight(1)
	e.scrollToCursor()

	m := viewSymbol{editor: e}
	m.filter()

	return m, cmd
}

type viewSymbol struct {
	*editor

	// input 은 이름을 거르는 패턴이다. 친 그대로다.
	input inputLine

	hits     []paletteHit
	selected int // hits 안의 자리
	top      int // 격자의 첫 행
}

func (m viewSymbol) Init() tea.Cmd { return nil }

func (m viewSymbol) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 좁아져서 못 그리게 됐다. 보이지 않는 mode 에 갇히면 키를 쳐도 아무 일이 안 난다.
		if !m.symbolFits() {
			return m.leave()
		}

		m.setDrawerHeight(m.symbolDrawerHeight())
		m.scrollToCursor()
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return m.leave()
		case "enter":
			return m.insertSelected()
		case "left":
			m.move(-1)

			return m, nil
		case "right":
			m.move(1)

			return m, nil
		case "up":
			m.move(-m.symbolColumns())

			return m, nil
		case "down":
			m.move(m.symbolColumns())

			return m, nil
		case "pgdown":
			// 격자라 한 화면이 행 수 곱 칸 수다. 세는 자는 다른 판과 같다(page.go).
			m.move(textarea.PageRows(textarea.PageFull, m.symbolDrawerHeight()) * m.symbolColumns())

			return m, nil
		case "pgup":
			m.move(-textarea.PageRows(textarea.PageFull, m.symbolDrawerHeight()) * m.symbolColumns())

			return m, nil
		case "home", "end":
			// **`←`·`→` 와 갈린다**(ADR-0113 §2). 격자에서 옆 칸은 세로 이동으로 대신할 수
			// 없으므로 그 둘만 격자에 남기고, 나머지는 입력줄 것이다. 목록 처음·끝은
			// `pgup`·`pgdown` 으로 간다.
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()
			m.filter()

			return m, nil
		case "ctrl+p":
			// 팔레트를 여는 키다. 여기서는 아무 일도 하지 않는다 — 팔레트가 자기 키에
			// 그렇게 하는 것과 같다(ADR-0011).
			return m, nil
		case "backspace":
			// 팔레트·명령줄과 같이 다 지우면 나간다.
			if m.input.empty() {
				return m.leave()
			}
			m.input.deleteBackward()
			m.filter()

			return m, nil
		default:
			// 한글은 여기서 글자다. 입력줄이 있는 mode 는 키를 되돌리지 않는다(ADR-0008).
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)
			m.filter()

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 격자는 화면 아래에 붙은 판이라 아래 편집 내용을 굴려도 볼 수 없다.
		// 팔레트와 같이 포인터가 어디 있든 목록을 오르내린다.
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-m.symbolColumns())
		case tea.MouseWheelDown:
			m.move(m.symbolColumns())
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 다른 mode 와 같이 공용 처리에 넘기고, 여기서만 목록을 다시 거른다.
		// 훑기가 끝나면 후보가 한꺼번에 늘어나므로 치고 있던 패턴에 새것도 걸려야 한다.
		next, cmd := m.handleJob(msg)
		if next != nil {
			// 다른 화면으로 넘어간다. 넣던 것을 끝내고 커서를 normal 자리로 되돌린다.
			//
			// **판 높이는 여기서 지우지 않는다.** 넘어가는 곳이 또 판이면(정의 후보·사용처
			// 목록) 그쪽이 방금 잡은 높이를 우리가 지우게 된다. 판이 아닌 곳으로 가는
			// 길은 normalMode 가 지운다(ADR-0069).
			m.finishEdit()

			return next, cmd
		}
		if len(m.symbols) > len(assets.CuratedSymbols) {
			m.symbolsIndexed = true
		}
		m.refilter()

		return m, cmd
	default:
		return m, nil
	}
}

// finishEdit 는 넣던 것을 끝내고 커서를 normal 자리로 되돌린다.
//
// insert mode 의 `esc` 와 똑같다(view-editor-insert.go). 넣은 것이 있으면 마지막 글자 위에
// 서고, 아무것도 안 넣었으면 `a<Esc>` 처럼 열기 전 자리로 돌아온다.
//
// **이 mode 를 떠나는 모든 길이 이것을 지나야 한다.** 판 높이를 지우는 것은 여기가 아니라
// normalMode 다 — 판에서 판으로 넘어가는 길이 생기면서 갈렸다(ADR-0069).
func (m viewSymbol) finishEdit() {
	buf := m.activeBuffer()

	buf.EndEdit()
	buf.MoveLeft(1)
	m.scrollToCursor()
}

// leave 는 판을 닫고 normal 로 돌아간다. 판 높이는 normalMode 가 지운다.
func (m viewSymbol) leave() (tea.Model, tea.Cmd) {
	m.finishEdit()

	return normalMode(m.editor)
}

// filter 는 입력으로 목록을 다시 거른다. 입력이 바뀌었으므로 고른 자리는 처음으로 돌아간다.
func (m *viewSymbol) filter() {
	m.hits = filterPalette(m.input.text, m.labels())
	m.selected, m.top = 0, 0
}

// refilter 는 후보가 늘었을 때 다시 거른다. filter 와 달리 고른 자리를 그대로 둔다.
// 훑기가 끝나며 목록이 한꺼번에 길어질 때 커서가 맨 앞으로 튀지 않게 한다(palette.go).
func (m *viewSymbol) refilter() {
	m.hits = filterPalette(m.input.text, m.labels())
	m.scrollTo()
}

// labels 는 매칭 대상이 되는 글자들이다.
func (m viewSymbol) labels() []string {
	labels := make([]string, 0, len(m.symbols))
	for _, entry := range m.symbols {
		labels = append(labels, entry.Label())
	}

	return labels
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다 — 둘러 가면 목록의 끝이 어디인지 알 수 없다.
func (m *viewSymbol) move(delta int) {
	if len(m.hits) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(m.hits)-1)
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. 팔레트의 것과 같은 규칙이되
// 세는 단위가 항목이 아니라 격자 행이다.
func (m *viewSymbol) scrollTo() {
	columns, rows := m.symbolColumns(), m.symbolGridRows()
	if columns < 1 {
		return
	}

	m.selected = min(max(m.selected, 0), max(len(m.hits)-1, 0))

	lastRow := max((len(m.hits)-1)/columns, 0)
	m.top = min(max(m.top, 0), max(lastRow-rows+1, 0))

	row := m.selected / columns
	if row < m.top {
		m.top = row
	}
	if row >= m.top+rows {
		m.top = row - rows + 1
	}
}

// selectedSymbol 은 지금 고른 글자다. 걸린 것이 없으면 빈 것이다.
func (m viewSymbol) selectedSymbol() (assets.Symbol, bool) {
	if len(m.hits) == 0 {
		return assets.Symbol{}, false
	}

	return m.symbols[m.hits[m.selected].index], true
}

// insertSelected 는 고른 글자를 커서 자리에 넣는다.
//
// drawer 는 열린 채로 둔다. 같은 갈래에서 몇 개를 잇달아 넣는 것이 이 mode 의 쓰임새다.
// endEdit 을 부르지 않아서 연달아 넣은 것이 `u` 한 번에 통째로 돌아온다 — insert mode 와 같다.
func (m viewSymbol) insertSelected() (tea.Model, tea.Cmd) {
	entry, ok := m.selectedSymbol()
	if !ok {
		return m, nil
	}

	m.activeBuffer().Insert([]byte(entry.Char))
	m.scrollToCursor()

	return m, m.scheduleEditTick()
}

func (m viewSymbol) View() tea.View {
	view := m.editorView(tea.CursorBar, "SYMBOL", "")

	// 편집 화면을 다 그린 뒤 그 아래 빈 자리에 판을 얹는다. textHeight 가 이미 그만큼
	// 줄어 있어서 덮는 것이 없다. 셀 단위라 두 칸 글자와 색이 어긋나지 않는다.
	// 합성은 이 mode 에서만 태운다 — 셀 버퍼를 지나면 줄 끝의 빈 칸이 잘린다(view-palette.go).
	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	// 커서는 편집 내용이 아니라 판 안 치는 자리에 있어야 한다.
	_, cursor := m.inputText(m.textWidth() - 4)
	view.Cursor = tea.NewCursor(left+2+cursor, top+1)
	view.Cursor.Shape = tea.CursorBar

	return view
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
//
// 테두리는 팔레트와 같이 손으로 붙인다. lipgloss 의 Border 는 안쪽 내용의 폭을 스스로 재는데
// 강조 escape 가 이미 섞여 있어서 그 계산을 믿을 수 없다.
func (m viewSymbol) renderDrawer() string {
	width := m.textWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{
		chars.topLeft + line + chars.topRight,
		m.renderInputRow(inner),
		chars.leftTee + line + chars.rightTee,
	}
	rows = append(rows, m.renderGridRows(inner)...)
	rows = append(rows, m.renderNameRow(inner))
	rows = append(rows, chars.bottomLeft+line+chars.bottomRight)

	return strings.Join(rows, "\n")
}

// renderInputRow 는 치고 있는 것과 걸린 개수를 보여주는 줄이다.
//
// 개수를 statusBar 가 아니라 여기 두는 것은 판이 이미 아래에 있어서다 — 눈이 두 군데를
// 오갈 이유가 없다. statusBar 아래 줄은 알림 자리로 비워 둔다.
func (m viewSymbol) renderInputRow(inner int) string {
	side := m.boxChars.vertical

	counter := m.symbolCounter()

	body, _ := m.inputText(inner)
	style := lipgloss.NewStyle()
	if m.input.empty() {
		body = "이름으로 찾습니다. 삼각형·하트·화살표"
		style = styleDetail
	}

	// 이름이 먼저다. 붙일 칸이 없으면 개수를 뗀다 — statusBar 의 git 표시와 같은 규칙이다.
	body = truncateToWidth(body, inner)
	if textarea.WidthOf(body)+2+textarea.WidthOf(counter) > inner {
		counter = ""
	}

	pad := strings.Repeat(" ", max(inner-textarea.WidthOf(body)-textarea.WidthOf(counter), 0))

	return side + " " + style.Render(body) + pad + styleDetail.Render(counter) + " " + side
}

// symbolCounter 는 몇 개 중 몇 개가 걸렸는지다. 입력줄 오른쪽 끝에 붙는다.
func (m viewSymbol) symbolCounter() string {
	return fmt.Sprintf("%d/%d", len(m.hits), len(m.symbols))
}

// inputText 는 입력줄에 그릴 글과 그 안에서 커서가 설 칸이다.
//
// **넘치면 커서가 보이도록 왼쪽부터 접는다.** 팔레트·grep 검색창과 같은 답이다
// (input-line.go). 오른쪽 끝의 개수와 그 앞 빈 칸 둘, 그리고 커서 한 칸을 비켜 둔다 —
// 개수를 뗄지는 접고 나서 정한다(renderInputRow).
func (m viewSymbol) inputText(inner int) (text string, cursorCol int) {
	room := inner - 2 - textarea.WidthOf(m.symbolCounter()) - 1

	return m.input.visible("", max(room, 1))
}

// renderGridRows 는 격자 행들이다. 걸린 것이 없으면 그 사실을 한 줄로 알린다.
func (m viewSymbol) renderGridRows(inner int) []string {
	side := m.boxChars.vertical

	if len(m.hits) == 0 {
		// padTo 는 채우기만 하고 자르지 않는다. 좁은 편집 영역에서는 이 문구가 상자보다
		// 넓어서 오른쪽 테두리를 밀어낸다 — 입력줄과 같이 먼저 자른다.
		empty := padTo(truncateToWidth("일치하는 것이 없습니다", inner), inner)

		rows := []string{side + " " + styleDetail.Render(empty) + " " + side}
		for len(rows) < m.symbolGridRows() {
			rows = append(rows, side+" "+strings.Repeat(" ", inner)+" "+side)
		}

		return rows
	}

	columns := m.symbolColumns()

	rows := []string{}
	for row := m.top; len(rows) < m.symbolGridRows(); row++ {
		cells := strings.Builder{}
		filled := 0

		for column := range columns {
			at := row*columns + column
			if at >= len(m.hits) {
				break
			}

			cells.WriteString(m.renderCell(at))
			filled += symbolCellWidth
		}

		rows = append(rows, side+" "+cells.String()+strings.Repeat(" ", max(inner-filled, 0))+" "+side)
	}

	return rows
}

// renderCell 은 격자 한 칸이다. 글자를 놓고 남은 자리를 채워 늘 symbolCellWidth 칸이다.
//
// 폭은 화면의 나머지와 같은 자로 잰다. Ambiguous 글자(`→ ± × °`) 를 두 칸으로 그리는
// 터미널에서는 lines.WidthOf 가 이미 두 칸으로 답한다 — 눈금을 시작할 때 터미널에 맞춰
// 놓았기 때문이고, 그 전에는 이 자리에만 있던 특례가 그 일을 했다(ADR-0072, ADR-0056).
//
// **반전은 칸 전체에 건다. 글자 폭에 맞추지 않는다.** 격자는 글자가 한 칸이든 두 칸이든
// 칸을 늘 symbolCellWidth 로 두고 남는 자리를 빈 칸으로 채운다. 칸의 경계가 글자 폭을 보지
// 않으므로 고른 표시도 보지 않는다. 글자 폭에 맞춰 칠하면 폰트가 셀보다 넓게 그리는
// 글자(`⬠` `①` `⑴`) 에서 앞 절반만 칠해지고, 폭 0 글자에서는 칠할 자리가 없어진다.
// 어긋난 값이 폰트 안에 있어서 폭 계산으로는 닿지 않는다(docs/issues/0004).
//
// **글자 앞에 한 칸을 둔다.** 칸을 통째로 칠하면서 글자를 칸 왼쪽에 붙여 두면 반전 블록
// 안에서 글자가 한쪽으로 쏠려 보인다. 앞 여백은 모든 칸에 같이 주므로 글자가 서는 자리는
// 칸마다 그대로여서 세로줄이 맞는다. 두 칸짜리 글자는 이 여백으로 칸 가운데에 온다.
func (m viewSymbol) renderCell(at int) string {
	entry := m.symbols[m.hits[at].index]

	head := " " + symbolGlyph(entry.Char)
	cell := head + strings.Repeat(" ", max(symbolCellWidth-textarea.WidthOf(head), 0))

	// 고른 칸은 반전만 쓴다. 색을 섞으면 안쪽의 색 초기화가 반전까지 꺼버린다(view-palette.go).
	if at == m.selected {
		return reverse.Render(cell)
	}

	return cell
}

// dottedCircle 은 폭 0 인 글자를 낱개로 보일 때 얹는 자리 글자다.
const dottedCircle = "◌"

// symbolGlyph 는 글자를 판에 그릴 때 쓰는 모양이다. 그대로 그려도 되는 글자면 그것이다.
//
// **폭 0 인 결합 문자는 ◌ 에 얹는다.** `HEBREW ACCENT PASHTA` 같은 것은 혼자서 자리를
// 차지하지 않아 앞 칸의 빈 칸에 달라붙고, 고른 칸의 반전도 그 빈 칸의 것이 되어 사라진다 —
// 어디를 골랐는지 화면에서 보이지 않았다. 유니코드 표가 결합 문자를 낱개로 보일 때 쓰는
// 방식이 이것이다.
//
// **넣는 것은 원래 글자다.** 이 모양은 격자와 이름줄에만 쓴다(insertSelected).
func symbolGlyph(char string) string {
	if textarea.WidthOf(char) == 0 {
		return dottedCircle + char
	}

	return char
}

// renderNameRow 는 고른 글자가 무엇인지 알려주는 줄이다.
// 격자에는 글자만 있어서 이 줄이 없으면 무엇을 넣는지 모른 채 enter 를 치게 된다.
func (m viewSymbol) renderNameRow(inner int) string {
	side := m.boxChars.vertical

	entry, ok := m.selectedSymbol()
	if !ok {
		return side + " " + strings.Repeat(" ", inner) + " " + side
	}

	// 격자 첫 칸과 같은 자로 만든다. 앞 여백까지 같아야 글자가 위아래로 한 줄에 선다.
	head := " " + symbolGlyph(entry.Char)
	head += strings.Repeat(" ", max(symbolCellWidth-textarea.WidthOf(head), 0))

	// 맞은 자리는 label() 안의 offset 이라 paletteRow 가 그대로 갈라 준다.
	row := paletteRow{
		left:      entry.Name,
		right:     entry.Keywords,
		positions: m.hits[m.selected].positions,
	}

	return side + " " + head + row.render(max(inner-symbolCellWidth, 0)) + " " + side
}

// runInsertSymbol 은 팔레트 `>` 목록의 「특수문자 넣기」다.
func runInsertSymbol(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return symbolMode(e)
}
