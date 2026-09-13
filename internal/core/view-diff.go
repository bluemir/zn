package core

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/textarea"
)

// diff 판이다. mode 는 `DIFF` 다(ADR-0140).
//
// `GRAPH`·`COMMIT` 과 같이 화면을 통째로 쓴다. 하단 drawer 로 내리지 않는 것은, 판을 내리는
// 근거가 늘 「편집 화면이 같이 보여야 고를 수 있다」였기 때문이다(ADR-0069, ADR-0078).
// diff 는 고르는 목록이 아니라 읽는 글이고, 열여섯 줄로는 조각 하나도 다 안 들어간다.

// diffPlace 는 이 판에서 보고 있는 자리다.
//
// **화면 행 번호가 아니다.** 보기를 바꾸면 같은 조각이 차지하는 행 수가 달라진다. 고친 줄
// 하나가 unified 에서 두 행이고 side-by-side 에서 한 행이라, 행 번호로 들면 `tab` 을 누를
// 때마다 보던 자리가 밀린다(ADR-0140 §2).
type diffPlace struct {
	hunk int

	// row 는 조각 안의 자리다. -1 이면 조각 머리줄이다.
	row int

	// part 는 unified 에서 고친 자리가 낸 두 행 중 어느 쪽인지다. 아랫줄(`+`) 이 1 이다.
	// side-by-side 에서는 늘 0 이다.
	part int
}

// diffTabWidth 는 이 판이 tab 을 펴는 폭이다.
//
// 파일마다 재지 않는다. 견주는 두 쪽이 서로 다른 파일일 수 있고(`:diff <A> <B>`) 한쪽은
// 커밋 안의 blob 이라 열려 있는 buffer 가 아니다. 두 열의 칸이 맞아야 나란히 읽히므로
// 한 값으로 편다(ADR-0096 이 파일마다 재기로 한 것은 편집 화면의 이야기다).
const diffTabWidth = textarea.DefaultTabWidth

// diffMode 는 diff 판을 연다. parent 가 있으면 `q` 가 그리로 돌아간다.
//
// **볼 파일이 없어도 연다.** 견줄 두 쪽을 request 가 이미 들고 있어서 지금 buffer 를 물어볼
// 일이 없다. 무엇을 견줄지 고르는 것은 부르는 쪽의 몫이다(view-editor-command.go).
func diffMode(parent tea.Model, e *editor, request diffRequest) (tea.Model, tea.Cmd) {
	e.clearNotice()

	m := viewDiff{editor: e, parent: parent, request: request}

	return m, readDiff(request)
}

// viewDiff 는 diff 판이다.
type viewDiff struct {
	*editor

	// parent 는 이 판을 연 화면이다. nil 이면 `q` 가 normal 로 간다.
	parent tea.Model

	request diffRequest

	left, right diffSide
	hunks       []diffHunk
	ready       bool

	// side 는 side-by-side 로 보고 있는지다. `tab` 이 뒤집는다.
	side bool

	cursor diffPlace
	top    int // 화면 첫 행

	// pending 은 `]`·`[` 를 먹고 짝을 기다리는 상태다. 안 기다리면 빈 문자열이다.
	pending string
}

func (m viewDiff) Init() tea.Cmd { return nil }

func (m viewDiff) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.scrollTo()

		return m, nil
	case diffMsg:
		if msg.err != nil {
			m.notifyError(msg.err)

			return m.close()
		}

		m.left, m.right, m.hunks, m.ready = msg.left, msg.right, msg.hunks, true

		// 다른 것이 없으면 볼 것도 없다. 빈 판을 띄우면 「무엇이 잘못됐나」를 묻게 된다.
		if len(m.hunks) == 0 {
			m.notify("다른 것이 없습니다: " + m.left.label + " 와 " + m.right.label)

			return m.close()
		}

		if msg.note != "" {
			m.notify(msg.note)
		}

		m.cursor = diffPlace{hunk: 0, row: 0}
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseClickMsg:
		// 왼쪽만 본다. 오른쪽은 tabline 의 tab 을 닫는 손인데(ADR-0060) 이 판은 tabline 을
		// 덮고 있어서 누를 tab 이 없다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			m.click(mouse.Y)
		}

		return m, nil
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다. 한글은 두벌식 자리의 영문 키로 되돌린다(ADR-0008).
func (m viewDiff) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewDiff)
		if !ok {
			return model, nil
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, cmd
		}

		model = next
	}

	return model, nil
}

// run 은 동작 하나다. 읽기만 하는 판이라 고치는 키가 없다(ADR-0140 §5).
func (m viewDiff) run(key string) (tea.Model, tea.Cmd) {
	// `]`·`[` 를 먹고 있었으면 짝을 먼저 본다. 짝이 아니면 먹은 것을 버리고 이 키를 그대로
	// 본다 — 접두 키가 아무 키나 삼키면 키가 안 먹는 편집기가 된다.
	if pending := m.pending; pending != "" {
		m.pending = ""

		if key == "c" {
			m.jumpHunk(pending)

			return m, nil
		}
	}

	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return m.close()
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		m.move(textarea.PageRows(textarea.PageFull, m.listHeight()))

		return m, nil
	case "pgup":
		m.move(-textarea.PageRows(textarea.PageFull, m.listHeight()))

		return m, nil
	case "g", "home":
		m.moveTo(0)

		return m, nil
	case "G", "end":
		m.moveTo(len(m.layout()))

		return m, nil
	case "tab":
		m.toggleSide()

		return m, nil
	case "]", "[":
		m.pending = key

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// close 는 `q` 다. 연 화면이 있으면 그리로, 없으면 normal 로 돌아간다.
func (m viewDiff) close() (tea.Model, tea.Cmd) {
	if m.parent != nil {
		return m.parent, nil
	}

	return normalMode(m.editor)
}

// toggleSide 는 `tab` 이다. unified 와 side-by-side 를 오간다.
//
// **보던 조각은 그대로 남는다.** 자리를 행 번호가 아니라 조각으로 들고 있어서 옮길 것이
// 없다. unified 가 낸 아랫줄(`+`) 에 서 있었으면 그 짝으로 접힌다(ADR-0140 §2).
func (m *viewDiff) toggleSide() {
	m.side = !m.side
	m.cursor.part = 0
	m.scrollTo()
}

// jumpHunk 는 `]c`·`[c` 다. 조각의 첫 줄 사이를 뛴다.
//
// **감아 돈다.** 편집 화면의 마커 뛰기와 같은 규칙이고 알리는 글자도 같다(ADR-0095 §3).
// 이 판에서 이 키는 vim 본래의 자리이기도 하다 — vim 의 `]c` 가 diff mode 안에서만 뜻이
// 있는 그 키다(ADR-0095 §1).
func (m *viewDiff) jumpHunk(key string) {
	if len(m.hunks) < 1 {
		return
	}

	next, wrapped := m.cursor.hunk+1, false
	if key == "[" {
		next = m.cursor.hunk - 1
	}

	switch {
	case next >= len(m.hunks):
		next, wrapped = 0, true
	case next < 0:
		next, wrapped = len(m.hunks)-1, true
	}

	m.cursor = diffPlace{hunk: next, row: m.hunks[next].firstChanged()}
	m.scrollTo()

	if wrapped {
		m.notify(markerWrapMessage(diffDirection(key)))
	}
}

// diffDirection 은 `]`·`[` 를 마커 뛰기의 방향으로 옮긴다. 알리는 글을 나눠 쓰기 위한 것이다.
func diffDirection(key string) markerDirection {
	if key == "[" {
		return markerBackward
	}

	return markerForward
}

// open 은 `enter` 다. **그 줄로 뛰고 판을 닫는다**(ADR-0140 §5).
//
// 닿는 곳은 늘 오른쪽 쪽의 작업본이다. 커밋 diff 를 보다 눌러도 그 커밋의 blob 이 아니라
// 지금 디스크의 그 파일로 간다 — 열 수 없는 것을 tab 에 끼우지 않는다.
//
// **뛰기 전 자리를 이력에 담는다.** 파일을 가로지르는 이동이라 `ctrl+o` 가 돌려줄 수 있어야
// 한다(ADR-0070, ADR-0074, ADR-0095 §6).
func (m viewDiff) open() (tea.Model, tea.Cmd) {
	if !m.ready || m.right.path == "" {
		return m, nil
	}

	line, ok := m.cursorLine()
	if !ok {
		return m, nil
	}

	// **닿을 자리를 확인한 뒤에 담는다.** 못 열면 커서가 그대로라 담을 것도 없다.
	reveal, err := m.openTab(m.right.path)
	if err != nil {
		return normalModeError(m.editor, err)
	}

	m.recordJump()

	buf := m.activeBuffer()
	buf.MoveTo(scheme.Cursor{Line: line})
	buf.ClampToNormal()
	m.scrollToCursor()
	m.arrive()

	model, next := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(next, reveal, m.startGitRefresh())
}

// cursorLine 은 커서가 선 자리의 **오른쪽** 줄번호다. 0 부터다.
//
// **들어낸 줄에 서 있으면 그 자리에 남은 줄로 간다.** 그 줄은 오른쪽에 없어서 가리킬 자리가
// 없으므로, 위로 올라가며 오른쪽에 든 첫 줄을 찾는다. 위에도 없으면 조각의 시작이라 파일
// 맨 앞이다.
func (m viewDiff) cursorLine() (int, bool) {
	if m.cursor.hunk >= len(m.hunks) {
		return 0, false
	}

	rows := m.hunks[m.cursor.hunk].rows
	for at := min(m.cursor.row, len(rows)-1); at >= 0; at-- {
		if rows[at].right.exists() {
			return rows[at].right.line, true
		}
	}

	return 0, true
}

// layout 은 화면 행마다 어느 자리인지다. 그리기 전에 자리를 세는 곳이 여기 하나다.
//
// **보기가 이 목록의 길이를 바꾼다.** unified 는 고친 자리를 두 행으로 내고 side-by-side 는
// 한 행으로 낸다(ADR-0140 §2).
func (m viewDiff) layout() []diffPlace {
	places := make([]diffPlace, 0, len(m.hunks)*(diffContext*2+2))

	for h := range m.hunks {
		places = append(places, diffPlace{hunk: h, row: -1})

		for r, row := range m.hunks[h].rows {
			places = append(places, diffPlace{hunk: h, row: r})

			if !m.side && row.kind == diffRowChanged {
				places = append(places, diffPlace{hunk: h, row: r, part: 1})
			}
		}
	}

	return places
}

// indexOf 는 그 자리가 지금 보기에서 몇 번째 행인지다.
//
// **꼭 맞는 자리를 먼저 찾고, 없을 때만 같은 줄의 첫 행으로 내려앉는다.** 보기를 바꾸면
// part 가 사라진 자리가 있어서 그 대비가 필요한데, 그것을 먼저 보면 `part` 가 1 인 자리를
// 늘 0 인 자리로 되돌린다 — 고친 줄에서 `j` 가 통째로 먹지 않던 자리다. 아랫줄(`+`) 로
// 내려가자마자 scrollTo 가 커서를 윗줄(`-`) 로 끌어올려서 제자리걸음이 됐다.
func (m viewDiff) indexOf(places []diffPlace, place diffPlace) int {
	near, found := 0, false

	for at, here := range places {
		if here == place {
			return at
		}

		if !found && here.hunk == place.hunk && here.row == place.row {
			near, found = at, true
		}
	}

	return near
}

// click 은 누른 화면 행으로 커서를 옮긴다. 판 밖이면 아무 일도 하지 않는다.
//
// **가로는 보지 않는다.** 이 판에서 고르는 것이 행 하나라 칸에는 뜻이 없다. 편집 영역이
// 칸까지 보는 것은 거기서 커서가 글자를 가리키기 때문이다(mouse.go 의 clickText).
//
// 열지는 않는다. 누르는 것은 커서를 옮기는 일이고 여는 것은 `enter` 다. 편집 영역에서
// 누르기가 커서 이동인 것과 같은 규칙이다(ADR-0012).
func (m *viewDiff) click(y int) {
	// 제목줄 위와 안내·statusBar 아래는 고를 행이 아니다.
	row := y - jobsTitleHeight
	if row < 0 || row >= m.listHeight() {
		return
	}

	// 담긴 것보다 아래를 누른 것이다. 빈 행에는 고를 자리가 없다.
	places := m.layout()
	if m.top+row >= len(places) {
		return
	}

	m.cursor = places[m.top+row]
	m.scrollTo()
}

// move 는 커서를 delta 행 옮긴다. 양끝에서 멈춘다.
func (m *viewDiff) move(delta int) {
	places := m.layout()

	m.moveTo(m.indexOf(places, m.cursor) + delta)
}

// moveTo 는 커서를 그 행으로 옮긴다.
func (m *viewDiff) moveTo(at int) {
	places := m.layout()
	if len(places) == 0 {
		return
	}

	m.cursor = places[min(max(at, 0), len(places)-1)]
	m.scrollTo()
}

// scrollTo 는 커서가 보이도록 첫 행을 최소한으로 움직인다.
func (m *viewDiff) scrollTo() {
	places, height := m.layout(), m.listHeight()
	if len(places) == 0 || height < 1 {
		m.top = 0

		return
	}

	at := m.indexOf(places, m.cursor)
	m.cursor = places[at]

	m.top = min(max(m.top, 0), max(len(places)-height, 0))

	if at < m.top {
		m.top = at
	}
	if at >= m.top+height {
		m.top = at - height + 1
	}
}

func (m viewDiff) View() tea.View {
	places, height := m.layout(), m.listHeight()

	body := make([]string, 0, height)
	for at := m.top; at < len(places) && len(body) < height; at++ {
		body = append(body, m.renderPlace(places[at]))
	}

	if !m.ready && height > 0 {
		body = append(body, " 읽는 중입니다")
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	// **자른다.** 안내가 편집 영역보다 길면 터미널이 접어서 그 아래가 통째로 밀린다.
	// 좁은 화면에서 실제로 넘치는 줄이라, 다른 판보다 안내가 길어진 것이 여기서 드러났다.
	screen = append(screen, styleDetail.Render(truncateToWidth(" "+m.hint(), m.width)))
	// **접두 키를 오른쪽 끝에 붙이는 폭도 트리를 지운 뒤에 잰다.** renderWithShowcmd 는
	// textWidth 로 재는데 그것이 sidebar 를 뺀 값이라, 지우기 전에 재면 `]` 가 sidebar
	// 폭만큼 왼쪽에 선다. 판은 트리를 덮으므로 뺄 것이 없다(viewJobs 도 같은 손이다).
	bare := *m.editor
	bare.sidebar = sidebar{}

	screen = append(screen, bare.bareStatusBar("DIFF", bare.renderWithShowcmd(m.notice, m.pending))...)

	view := newView(screen, m.renderWindowTitle())

	// 커서는 고른 행 왼쪽 끝이다. 제목줄이 한 행을 쓰므로 그 아래에서 센다.
	if m.ready && len(places) > 0 {
		view.Cursor = tea.NewCursor(0, m.indexOf(places, m.cursor)-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// hint 는 아래 줄의 키 안내다. 지금 보기의 반대쪽을 적는다.
func (m viewDiff) hint() string {
	other := "나란히"
	if m.side {
		other = "이어서"
	}

	return "j/k 이동  ]c/[c 조각  tab " + other + "  enter 그 줄로  q 닫기"
}

// renderTitle 은 맨 윗줄이다. 무엇과 무엇을 견주는지와 조각 수다.
//
// tabline 과 같이 반전이라 「다른 화면으로 왔다」로 읽힌다(ADR-0115 §1).
func (m viewDiff) renderTitle() string {
	if !m.ready {
		return reverse.Width(m.width).Render(truncateToWidth("diff  읽는 중", m.width))
	}

	label := fmt.Sprintf("diff  %s → %s  ·  조각 %s 개",
		m.left.label, m.right.label, formatCount(len(m.hunks)))

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// renderPlace 는 화면 행 하나다. 머리줄이면 조각 머리줄이고 아니면 내용 행이다.
func (m viewDiff) renderPlace(place diffPlace) string {
	hunk := m.hunks[place.hunk]

	if place.row < 0 {
		return truncateToWidth(styleDiffHeader.Render(hunk.header()), m.width)
	}

	row := hunk.rows[place.row]

	if m.side {
		return m.renderSideRow(row)
	}

	return m.renderUnifiedRow(row, place.part)
}

// renderUnifiedRow 는 이어서 보는 한 행이다.
//
//	47   47   const timeout = 100 * time.Millisecond
//	48        - const timeout = 100 * time.Millisecond
//	     48   + const timeout = 200 * time.Millisecond
//
// **양쪽 줄번호를 다 보인다.** `git diff` 는 안 적는데, 그러면 `enter` 가 어느 줄로 가는지
// 화면에 없다. 왼쪽이 기준의 번호이고 오른쪽이 지금의 번호다.
//
// 고친 자리는 두 행이 된다. part 가 그중 어느 쪽인지다.
func (m viewDiff) renderUnifiedRow(row diffRow, part int) string {
	left, right := diffDigits(len(m.left.lines)), diffDigits(len(m.right.lines))

	// 그릴 쪽을 고른다. 고친 자리는 윗줄이 왼쪽(`-`), 아랫줄이 오른쪽(`+`) 이다.
	side, cell, sign, paint := m.right, row.right, "+", diffPaint{colorDiffAdded, colorDiffAddedStrong}

	switch {
	case row.kind == diffRowSame:
		side, cell, sign, paint = m.right, row.right, " ", diffPaint{colorDiffSame, colorDiffSame}
	case row.kind == diffRowRemoved || (row.kind == diffRowChanged && part == 0):
		side, cell, sign, paint = m.left, row.left, "-", diffPaint{colorDiffRemoved, colorDiffRemovedStrong}
	}

	// 고친 자리의 두 행은 자기 쪽 번호만 세운다. 같은 줄이 두 번 적히면 어느 쪽 번호인지가
	// 흐려진다.
	leftLine, rightLine := row.left.line, row.right.line
	if row.kind == diffRowChanged {
		if part == 0 {
			rightLine = -1
		} else {
			leftLine = -1
		}
	}

	gutter := renderDiffNumber(leftLine, left, paint.base) +
		lipgloss.NewStyle().Background(paint.base).Render(" ") +
		renderDiffNumber(rightLine, right, paint.base) +
		lipgloss.NewStyle().Background(paint.base).Foreground(colorDiffNumber).Render(" "+sign+" ")

	width := max(m.width-left-right-4, 0)

	return gutter + renderDiffText(side.textAt(cell.line), side.tokensAt(cell.line),
		cell.spans, paint, diffTabWidth, width)
}

// renderSideRow 는 나란히 보는 한 행이다. 왼쪽이 기준이고 오른쪽이 지금이다.
//
//	47 const timeout = 100 │  47 const timeout = 200
//	                       │  48 // 새로 넣은 줄
//
// **한쪽에 줄이 없으면 그 자리가 filler 다.** 넣은 것도 들어낸 것도 아니라 색이 없고,
// 「여기에는 아무것도 없다」만 말한다.
func (m viewDiff) renderSideRow(row diffRow) string {
	divider := m.boxChars.vertical

	// 구분선 한 칸을 빼고 남은 것을 반씩 나눈다. 홀수면 왼쪽이 한 칸 더 가져간다.
	rest := max(m.width-1, 0)
	leftWidth, rightWidth := rest-rest/2, rest/2

	leftPaint := diffPaint{colorDiffRemoved, colorDiffRemovedStrong}
	rightPaint := diffPaint{colorDiffAdded, colorDiffAddedStrong}

	if row.kind == diffRowSame {
		leftPaint = diffPaint{colorDiffSame, colorDiffSame}
		rightPaint = leftPaint
	}

	if !row.left.exists() {
		leftPaint = diffPaint{colorDiffFiller, colorDiffFiller}
	}
	if !row.right.exists() {
		rightPaint = diffPaint{colorDiffFiller, colorDiffFiller}
	}

	return m.renderSideHalf(m.left, row.left, leftPaint, leftWidth) +
		styleDetail.Render(divider) +
		m.renderSideHalf(m.right, row.right, rightPaint, rightWidth)
}

// renderSideHalf 는 나란히 보기의 한쪽이다. 번호 칸과 본문을 합쳐 width 칸을 꽉 채운다.
func (m viewDiff) renderSideHalf(side diffSide, cell diffCell, paint diffPaint, width int) string {
	digits := diffDigits(len(side.lines))
	if width <= digits+1 {
		// 번호를 세울 자리도 없다. 본문에 다 준다.
		return renderDiffText(side.textAt(cell.line), side.tokensAt(cell.line),
			cell.spans, paint, diffTabWidth, width)
	}

	gutter := renderDiffNumber(cell.line, digits, paint.base) +
		lipgloss.NewStyle().Background(paint.base).Render(" ")

	return gutter + renderDiffText(side.textAt(cell.line), side.tokensAt(cell.line),
		cell.spans, paint, diffTabWidth, width-digits-1)
}

// header 는 조각 머리줄이다. `git diff` 와 같은 `@@ -47,2 +47,4 @@` 다.
func (hunk diffHunk) header() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		hunk.leftStart, hunk.leftCount, hunk.rightStart, hunk.rightCount)
}

// firstChanged 는 조각 안에서 처음으로 바뀐 자리다. `]c` 가 여기로 데려간다.
//
// **문맥 줄이 아니라 바뀐 줄이다.** 조각의 첫 줄로 데려가면 앞 문맥 셋을 지나야 정작 볼
// 것이 나온다. vim 이 `]c` 를 「변경의 **시작**으로」라고 적어 둔 그 자리다(ADR-0095 §4).
func (hunk diffHunk) firstChanged() int {
	for at, row := range hunk.rows {
		if row.kind.changed() {
			return at
		}
	}

	return 0
}
