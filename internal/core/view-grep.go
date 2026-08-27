package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// grepMinTextHeight 는 판을 열고도 편집 영역에 남아야 할 행 수다. 다른 판들과 같다.
const grepMinTextHeight = symbolMinTextHeight

// grepFrame 은 판이 목록 말고 쓰는 행 수다. 테두리 둘이다.
const grepFrame = 2

// grepMaxRows 는 목록에 보일 최대 행 수다. 다른 목록 판과 같은 값이다(ADR-0069).
//
// **담는 것(최대 5,000) 과 보이는 것(16) 이 다르다.** 방문 기록과 같은 생김새다 — 판이
// 화면을 다 먹으면 둘러보는 자리가 안 보이는데, 검색 결과는 짧으면 값이 없다. `j`/`k` 가
// 훑고 아래 줄이 몇 번째인지 적는다(ADR-0074, ADR-0078).
const grepMaxRows = locationsMaxRows

// viewGrep 은 여러 파일 검색의 결과를 둘러보는 하단 drawer 다. mode 는 `GREP` 다.
//
// **둘러보고 확정하는 판이다.** `j`/`k` 가 커서를 실제로 그 자리로 옮겨 보여주고 `enter` 가
// 판을 닫으며 확정한다. `q`·`esc` 는 취소라 판을 열기 전 자리로 되돌아가고 둘러보며 연 tab 도
// 닫는다. `GOTO`·되돌아간 자리·방문한 자리 판과 같은 손이다(ADR-0071, ADR-0073, ADR-0074).
//
// 처음에는 화면을 통째로 쓰는 목록이었다. 줄 내용을 목록에 넣었으니 가 보지 않고 고를 수
// 있다고 보았는데, **써 보니 그 줄 하나로는 판단이 안 되는 때가 많았다** — 위아래 문맥을 봐야
// 「이 매칭이 내가 찾던 것인가」가 갈린다. 그리고 그러려면 편집 화면이 같이 보여야 해서
// ADR-0069 가 `GOTO` 를 내린 자리로 이 판도 내려왔다(ADR-0078).
//
// **줄 내용 칸은 남겼다.** 문맥을 보려고 둘러보더라도 어디를 볼지 **고르는 것은 목록에서**
// 하고, 그 판단에 `경로:줄` 만으로는 모자란다.
//
// **빈 화면(tab 이 하나도 없는 상태) 에서도 띄운다.** 다른 판들은 거기서 거절하는데
// (`refuseNoBuffer`, ADR-0064) 그 근거는 「판이 지금 buffer 에 딸린 것」이었다 — register 는
// 무엇에 붙일지가, 정의 후보는 무엇을 물었는지가 buffer 에서 온다. **검색은 그렇지 않다.**
// 담은 것이 저장소 전체에서 온 것이라 지금 buffer 를 보지 않고, 여기서 고르는 일이 곧
// 「어느 파일을 열까」다 — 빈 화면이야말로 그것이 필요한 자리다(ADR-0078 §6).
//
// 되돌릴 자리가 없는 것은 저절로 풀린다. `e.here()` 가 거짓이라 `hasOrigin` 이 서지 않고,
// 취소는 둘러보며 연 tab 을 전부 닫아 빈 화면으로 되돌아간다(preview.go).
func grepMode(e *editor) (tea.Model, tea.Cmd) {
	// 알림은 이 판이 대신한다. 「무엇을 찾았고 몇 번째인가」는 아래 줄에 적힌다.
	e.clearNotice()

	m := viewGrep{editor: e}

	// 둘러보기 전 화면을 적어 둔다. 취소하면 이것으로 되돌린다(preview.go).
	//
	// **여는 것만으로는 옮기지 않는다.** 미리보기는 `j`/`k` 를 쳐야 시작한다 — 여는 순간
	// 옮기려 해도 적중이 아직 하나도 안 왔다.
	m.preview = startPreview(e)

	e.drawerHeight = m.grepDrawerHeight()
	e.scrollToCursor()

	return m, nil
}

type viewGrep struct {
	*editor

	selected int // rows() 안의 자리
	top      int // 판의 첫 행

	preview previewSession
	pending tea.Cmd

	// filter 는 목록을 좁히는 글자다. `/` 로 치기 시작한다.
	//
	// 검색을 다시 돌리는 것이 아니라 **이미 담은 적중을 거른다.** 저장소를 다시 훑지 않으므로
	// 글자마다 곧바로 좁아진다. 「목록을 띄운 뒤에도 검색을 이어 가는 길」이 이것이다.
	filter string

	// filtering 은 `/` 를 눌러 거를 글자를 치는 중인지다.
	//
	// 빈 filter 와 갈라야 한다 — `/` 만 누른 상태와 아무것도 안 한 상태에서 아래 줄이 달라야
	// 하고, `j` 가 목록을 옮길지 글자로 먹힐지도 이것으로 갈린다.
	filtering bool
}

// grepRows 는 목록에 쓸 수 있는 행 수다.
func (m viewGrep) grepRows() int {
	room := m.paneHeight() - grepFrame - grepMinTextHeight
	want := min(max(len(m.rows()), 1), grepMaxRows)

	return min(want, max(room, 1))
}

// grepDrawerHeight 는 판이 편집 영역에서 가져갈 행 수다.
func (m viewGrep) grepDrawerHeight() int {
	return m.grepRows() + grepFrame
}

func (m viewGrep) Init() tea.Cmd { return nil }

func (m viewGrep) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.reframe()

		return m, nil
	case tea.KeyPressMsg:
		// 거르는 중에는 글자를 그대로 받는다. 한글 되돌림도 하지 않는다 — 거를 글자는
		// 통째로 글자라 이 자리가 `msg.Text` 를 그대로 쓴다(ADR-0008).
		if m.filtering {
			return m.pressFilter(msg)
		}

		return m.press(msg.String())
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		cmd := m.pending
		m.pending = nil

		return m, cmd
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg:
		// **판 높이를 여기서 다시 잰다.** 적중이 도착하는 대로 목록이 자라므로 판도 같이
		// 자라야 하고, 열여섯에서 멈추니 곧 가라앉는다. 지우지는 않는다 — 넘어가는 곳이 또
		// 판이면 그쪽이 방금 잡은 높이를 우리가 지우게 된다(ADR-0069).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}
		m.reframe()

		return m, cmd
	default:
		return m, nil
	}
}

// reframe 은 목록 길이가 바뀐 뒤 판 높이와 화면을 다시 맞춘다.
func (m *viewGrep) reframe() {
	m.drawerHeight = m.grepDrawerHeight()
	m.scrollToCursor()
	m.scrollTo()
}

// pressFilter 는 거를 글자를 치는 동안의 키다.
//
// **여기서는 미리보기를 태우지 않는다.** 좁힐 때마다 커서가 옮겨 다니면 글자 하나에 파일이
// 하나씩 열린다. 좁혀 놓고 `j`/`k` 로 둘러보는 것이 이 기능의 손이다.
func (m viewGrep) pressFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "esc":
		m.filter, m.filtering = "", false
		m.reframe()

		return m, nil
	case "enter":
		m.filtering = false

		return m, nil
	case "backspace":
		// 다 지우면 거르기에서 나간다. 명령줄·검색창과 같은 손이다.
		if m.filter == "" {
			m.filtering = false

			return m, nil
		}
		m.filter = m.filter[:prevClusterStart([]byte(m.filter), 0, len(m.filter))]
		m.reframe()

		return m, nil
	default:
		if msg.Text == "" {
			return m, nil
		}
		m.filter += msg.Text

		// 좁아지면 고른 자리가 목록 밖으로 나갈 수 있다. 맨 위로 되돌린다 —
		// 남은 것 중 어디에 있었는지는 뜻이 없다.
		m.selected, m.top = 0, 0
		m.reframe()

		return m, nil
	}
}

// press 는 키 하나를 먹는다. 한글은 되돌린다 — 입력줄이 없는 상태의 판이다(ADR-0008).
//
// **미리보기가 낸 Cmd 를 모아서 나간다.** 자모 하나가 키 여럿으로 풀리므로 앞 키가 미리보기를
// 태우고 뒤 키가 이 화면을 벗어날 수 있다. 방문한 자리 판과 같은 자리이고 같은 까닭이다 —
// 흘리면 안 되는 것(파일을 연 뒤 언어 서버를 띄우는 Cmd) 이 그 안에 있다(ADR-0008, ADR-0051).
func (m viewGrep) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	var previews []tea.Cmd

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewGrep)
		if !ok {
			return model, tea.Batch(previews...)
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, tea.Batch(append(previews, cmd)...)
		}

		// 미리보기 Cmd 는 여기서 걷는다. run 에서 돌려주면 이 고리가 그것을 「이 화면을
		// 벗어났다」로 읽어 남은 자모를 버린다.
		if after, ok := next.(viewGrep); ok && after.pending != nil {
			previews = append(previews, after.pending)
			after.pending = nil
			next = after
		}

		model = next
	}

	return model, tea.Batch(previews...)
}

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
func (m viewGrep) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return m.cancel()
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
		m.move(pageRows(pageFull, m.grepDrawerHeight()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.grepDrawerHeight()))

		return m, nil
	case "g", "home":
		m.selectTo(0)

		return m, nil
	case "G", "end":
		m.selectTo(len(m.rows()) - 1)

		return m, nil
	case "/":
		// 거를 글자를 치기 시작한다. 검색을 다시 돌리는 것이 아니다.
		m.filtering = true

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 `enter` 다. 둘러보던 자리로 확정하고 판을 닫는다(ADR-0073).
//
// 뛰기 전 자리는 **판을 열기 전** 자리다. 둘러보며 커서가 이미 옮겨져 있으므로 지금 자리를
// 담으면 안 된다(preview.go 의 origin).
//
// 찾은 것을 `n` 으로 이어 짚을 수 있게 마지막 검색으로 굳힌다 — 뛰어간 자리에서 같은 패턴을
// 다시 칠 이유가 없고, `*` 가 커서 단어를 마지막 검색으로 두는 것과 같다.
func (m viewGrep) open() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m.cancel()
	}

	place := rows[min(m.selected, len(rows)-1)].place()

	if m.preview.hasOrigin {
		m.recordJumpFrom(m.preview.origin)
	}

	// 아직 그 자리를 안 밟았을 수 있다 — 열자마자 `enter` 를 친 경우다.
	cmd := m.goToPlace(place)
	m.arrive()

	m.preview.keep(m.editor, place.path)

	m.search = searchState{
		input:     m.grep.input,
		pattern:   m.grep.pattern,
		direction: searchForward,
		highlight: true,
	}

	model, next := normalMode(m.editor)

	return model, tea.Batch(next, cmd)
}

// cancel 은 `q`·`esc` 다. 판을 열기 전 자리로 되돌리고 둘러보며 연 tab 을 전부 닫는다.
func (m viewGrep) cancel() (tea.Model, tea.Cmd) {
	cmd := m.preview.restore(m.editor)

	model, next := normalMode(m.editor)

	return model, tea.Batch(next, cmd)
}

// place 는 이 적중이 가리키는 자리다. 둘러보기와 확정이 이것으로 뛴다.
func (h grepHit) place() jumpPlace {
	return jumpPlace{path: h.path, line: h.line, col: h.col}
}

// move 는 고른 자리를 옮기고 그 자리를 곧바로 보여준다.
func (m *viewGrep) move(delta int) {
	m.selectTo(m.selected + delta)
}

// selectTo 는 고른 자리를 그 index 로 옮기고 미리보기를 태운다. 양끝에서 멈춘다.
func (m *viewGrep) selectTo(index int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}

	before := m.selected

	m.selected = min(max(index, 0), len(rows)-1)
	m.scrollTo()

	if m.selected == before {
		return
	}

	m.pending = m.goToPlace(rows[m.selected].place())
}

// rows 는 목록에 그릴 적중들이다. 거를 글자가 있으면 좁힌 것이다.
//
// 거르는 자는 `경로 + 줄 내용` 에 그 글자가 들었는지다. 대소문자를 보지 않는다 — 좁히려고
// 치는 글자에 대소문자를 맞추라고 하는 것은 값이 없다. 정규식이 아닌 것은 이미 정규식으로
// 찾은 결과를 다시 정규식으로 좁힐 일이 드물어서다.
func (m viewGrep) rows() []grepHit {
	if m.filter == "" {
		return m.grep.hits
	}

	needle := strings.ToLower(m.filter)

	rows := make([]grepHit, 0, len(m.grep.hits))
	for _, hit := range m.grep.hits {
		if strings.Contains(strings.ToLower(hit.path+hit.text), needle) {
			rows = append(rows, hit)
		}
	}

	return rows
}

// scrollTo 는 고른 자리가 보이도록 첫 행을 최소한으로 움직인다.
//
// **담는 것이 보이는 것보다 훨씬 많아서** 이 판에서는 훑는 일이 늘 일어난다.
func (m *viewGrep) scrollTo() {
	rows, height := len(m.rows()), m.grepRows()
	if rows == 0 {
		m.selected, m.top = 0, 0

		return
	}

	m.selected = min(max(m.selected, 0), rows-1)
	m.top = min(max(m.top, 0), max(rows-height, 0))

	if m.selected < m.top {
		m.top = m.selected
	}
	if m.selected >= m.top+height {
		m.top = m.selected - height + 1
	}
}

func (m viewGrep) View() tea.View {
	view := m.editorView(tea.CursorBlock, "GREP", m.noticeOr(m.hint()))

	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	// 거르는 중이면 커서는 아래 줄 끝, 치는 자리에 있어야 한다. 검색창과 같다.
	if m.filtering {
		view.Cursor = tea.NewCursor(screenWidthOf(m.hint())+m.sidebarLeft(), m.height-1)
		view.Cursor.Shape = tea.CursorBlock
	}

	return view
}

// hint 는 statusBar 아래 줄에 적는 말이다.
//
// **몇 번째인지는 적지 않는다.** 그것은 아랫 테두리가 든다 — 숫자가 설명하는 대상 옆에 있는
// 것이 낫고, 여기는 그만큼 키 안내에 자리를 넘긴다(ADR-0079).
func (m viewGrep) hint() string {
	if m.filtering {
		return "거르기: " + m.filter
	}

	if len(m.rows()) == 0 {
		return m.label() + "  " + m.emptyReason() + "  esc 닫기"
	}

	return m.label() + "  j/k 둘러보기  enter 확정  / 거르기  esc 취소"
}

// label 은 무엇을 찾았는지다.
//
// `%q` 로 감싸지 않는다. 친 그대로를 보여야 하는데 그것이 `\s` 를 `\\s` 로 바꾼다 —
// 정규식에는 백슬래시가 흔해서 늘 어긋난다.
func (m viewGrep) label() string {
	label := "검색 \"" + m.grep.input + "\""

	// 상한에 닿았다는 것을 적는다. 조용히 자르면 「이게 전부」로 읽힌다.
	if m.grep.capped {
		label += "(상한)"
	}
	if m.filter != "" {
		label += " 거른 것 " + m.filter
	}

	return label
}

// emptyReason 은 목록이 빈 까닭이다. 도는 중과 없는 것은 다른 말이어야 한다.
func (m viewGrep) emptyReason() string {
	switch {
	case m.filter != "" && len(m.grep.hits) > 0:
		return "거른 결과가 없습니다"
	case m.jobRunning(grepJobName, []string{m.grep.input}):
		return "찾는 중입니다"
	default:
		return "찾은 곳이 없습니다"
	}
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
func (m viewGrep) renderDrawer() string {
	width := m.textWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{chars.topLeft + line + chars.topRight}
	rows = append(rows, m.renderListRows(inner)...)

	// 아랫 테두리가 몇 번째를 보고 있는지 든다. 담는 것이 보이는 것보다 훨씬 많아서
	// 목록만으로는 어디쯤인지 모른다(render-drawer-count.go, ADR-0079).
	rows = append(rows, renderCountBorder(chars, width, m.selected+1, len(m.rows())))

	return strings.Join(rows, "\n")
}

// grepMarkWidth 는 고른 자리 표시가 쓰는 폭이다.
const grepMarkWidth = 2

// renderListRows 는 목록 행들이다. 적중이 없으면 그 사실을 한 줄로 알린다.
func (m viewGrep) renderListRows(inner int) []string {
	side := m.boxChars.vertical
	height := m.grepRows()
	rows := m.rows()

	body := make([]string, 0, height)
	for at := m.top; at < len(rows) && len(body) < height; at++ {
		body = append(body, m.renderRow(rows[at], at == m.selected, inner))
	}

	if len(rows) == 0 && height > 0 {
		empty := padTo(truncateToWidth(m.emptyReason(), inner), inner)
		body = append(body, styleDetail.Render(empty))
	}

	for len(body) < height {
		body = append(body, strings.Repeat(" ", inner))
	}

	out := make([]string, 0, len(body))
	for _, text := range body {
		out = append(out, side+" "+text+" "+side)
	}

	return out
}

// grepPlaceWidth 는 `경로:줄` 칸의 폭이다. 판 안쪽 폭에 맞춰 잡는다.
//
// 절반을 넘기지 않되 너무 좁아지지도 않게 둔다 — 나머지가 줄 내용 칸이다.
func grepPlaceWidth(body int) int {
	return min(max(body/2, 16), 40)
}

// renderRow 는 적중 한 줄이다.
//
//	▸ internal/pubsub/hub.go:40  func NewHub(ctx context.Cont…
//	  …/syntax/go.go:12          func NewLexer() State
//
// 왼쪽이 `경로:줄`, 오른쪽이 줄 내용이다. 경로는 **왼쪽부터** 접고 내용은 오른쪽부터 접는다 —
// 경로에서 값을 지닌 것은 뒤쪽(파일 이름) 이고 내용에서는 앞쪽이다(ADR-0069).
//
// 줄 내용 칸을 판에까지 들고 온 것은, 둘러볼 자리를 **고르는 판단**이 목록에서 일어나기
// 때문이다. `경로:줄` 만으로는 어디를 볼지 정할 수 없다(ADR-0078).
func (m viewGrep) renderRow(hit grepHit, selected bool, inner int) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	body := max(inner-grepMarkWidth, 0)
	width := min(grepPlaceWidth(body), body)

	place := fmt.Sprintf("%s:%d", hit.path, hit.line+1)

	// 들여쓰기는 떼고 보인다. 깊이 들여쓴 줄이 오면 내용 칸이 공백으로만 찬다.
	// 뗀 만큼 매칭 자리도 당겨진다.
	text := strings.TrimLeft(hit.text, " \t")
	col := max(hit.col-(len(hit.text)-len(text)), 0)

	// 내용 칸은 경로 칸과 사이 한 칸을 뺀 나머지다.
	rest := max(body-width-1, 0)

	row := marker + padTo(trimLeftToWidth(place, width), width) + " " + grepWindow(text, col, rest)

	// 칸을 먼저 채우고 그다음에 반전을 입힌다 — 강조 뒤에는 폭을 잴 수 없다.
	row = padTo(truncateToWidth(row, inner), inner)
	if selected {
		return reverse.Render(row)
	}

	return row
}

// grepContextCols 는 매칭 앞에 남기는 칸 수다. 매칭이 칸 맨 앞에 붙어 있으면 그것이 무엇의
// 일부인지 안 보인다.
const grepContextCols = 8

// grepWindow 는 내용 칸에 넣을 글이다. **매칭이 보이도록 창을 옮긴다.**
//
// 줄이 칸보다 길고 매칭이 뒤쪽에 있으면, 줄 앞부터 그린 뒤 오른쪽을 자르는 것으로는 찾은
// 글자가 화면에 아예 안 나온다 — 목록에 줄 내용을 넣은 까닭이 사라진다. 그럴 때는 앞쪽을
// 왼쪽부터 접어서(`…`) 매칭을 칸 안으로 끌어온다.
//
// 줄이 칸에 다 들어가면 그대로 둔다. 자르는 것은 부르는 쪽이 한 번 더 한다.
func grepWindow(text string, col, width int) string {
	if width < 1 || screenWidthOf(text) <= width {
		return text
	}

	col = min(max(col, 0), len(text))

	return trimLeftToWidth(text[:col], grepContextCols) + text[col:]
}
