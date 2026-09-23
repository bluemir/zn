package core

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// viewHistory 는 `:history` 로 여는 명령줄·검색 이력이다(ADR-0143 §4).
//
// 화면을 통째로 쓰는 목록이라 `:messages`(ADR-0053)·`:jobs`(ADR-0027) 와 같은 문법을 따른다.
// 제목 띠, 본문, 키 안내 한 줄, 트리를 지운 statusBar 다.
//
// **`:messages` 와 갈리는 것은 `enter` 하나다.** 그쪽은 지나간 것을 읽는 화면이라 고른 것으로
// 할 일이 없는데, 이력은 「다시 쓰려고」 담는 것이라 꺼내는 길이 판 안에도 있어야 한다.
// 꺼내는 것은 명령줄에 **실어서 여는** 것이고 돌리지는 않는다. 이력에 `:w!`·`:e!` 가 섞여
// 있어서, 한 칸 잘못 짚은 것이 되돌릴 수 없는 일이 되면 안 된다.
//
// 순서는 알림 기록과 같이 오래된 것이 위다. 맨 아래에서 시작한다 — 찾는 것은 대개 방금 친 것이다.
func historyMode(e *editor, kind historyKind) (tea.Model, tea.Cmd) {
	m := viewHistory{editor: e, kind: kind}

	m.selected = len(m.rows()) - 1
	m.scrollTo()

	return m, nil
}

// historyKind 는 `:history` 가 보일 갈래다. 인자가 정한다.
//
// 갈래를 나눠 담았으니 나눠 보인다. vim 과 같은 인자다(ADR-0143 §4).
type historyKind int

const (
	historyCommands historyKind = iota // `:history`
	historySearches                    // `:history /`, `:history ?`
	historyAll                         // `:history all`
)

// parseHistoryKind 는 `:history` 의 인자를 갈래로 읽는다. 모르는 인자는 오류다.
//
// 조용히 명령 이력을 보이지 않는다. 인자를 대고 친 것이라 조용하면 그 인자가 먹힌 것으로
// 읽힌다(ADR-0064 의 태도와 같다).
func parseHistoryKind(args []string) (historyKind, error) {
	if len(args) == 0 {
		return historyCommands, nil
	}

	switch args[0] {
	case ":", "cmd":
		return historyCommands, nil
	case "/", "?", "search":
		return historySearches, nil
	case "all":
		return historyAll, nil
	default:
		return historyCommands, fmt.Errorf("알 수 없는 이력 갈래: %s", args[0])
	}
}

type viewHistory struct {
	*editor

	kind historyKind

	selected int // rows() 안의 자리
	top      int // 화면 첫 행
}

// historyRow 는 목록의 한 줄이다. prompt 는 그 줄을 칠 때 맨 앞에 서던 글자다.
//
// **갈래를 글자로 들고 있다.** `all` 에서 무엇이 명령이고 무엇이 검색인지 보여야 하고,
// `enter` 가 어느 줄을 열지 정하는 것도 이 값이다. 색으로만 가르면 화면을 글자로 떠서 보는
// 길에서 갈래가 사라진다(.claude/skills/run-zn).
type historyRow struct {
	prompt string // ":" 또는 "/"
	text   string
}

// rows 는 목록에 보일 줄들이다. 오래된 것이 앞이다.
//
// `all` 은 명령을 먼저 놓고 검색을 뒤에 놓는다. 둘을 친 차례대로 섞으려면 담을 때 시각을
// 같이 적어야 하는데, 이력이 답하는 것은 「무엇을 쳤나」이지 「언제 쳤나」가 아니다.
func (m viewHistory) rows() []historyRow {
	rows := make([]historyRow, 0, len(m.commandHistory.entries)+len(m.searchHistory.entries))

	if m.kind == historyCommands || m.kind == historyAll {
		for _, entry := range m.commandHistory.entries {
			rows = append(rows, historyRow{prompt: ":", text: entry})
		}
	}
	if m.kind == historySearches || m.kind == historyAll {
		for _, entry := range m.searchHistory.entries {
			rows = append(rows, historyRow{prompt: "/", text: entry})
		}
	}

	return rows
}

func (m viewHistory) Init() tea.Cmd { return nil }

func (m viewHistory) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 백그라운드 작업은 mode 와 무관하다. 목록이 늘어날 일은 없다 — 이력은 사람이 줄을
		// 쳐야 늘고, 이 판이 떠 있는 동안에는 칠 줄이 없다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 실행한다.
//
// 한글은 되돌린다. 입력줄이 없는 화면이라 `ㅓ` 를 `j` 로 읽어야 한다(ADR-0008).
// `:messages`·`GOTO` 와 같은 손이다.
func (m viewHistory) press(key string) (tea.Model, tea.Cmd) {
	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewHistory)
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

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
//
// 지우는 키는 없다. 담는 규칙이 「친 것은 전부」라 지우는 키가 그 결정과 어긋난다.
func (m viewHistory) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return normalMode(m.editor)
	case "enter":
		return m.open()
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
		m.move(textarea.PageRows(textarea.PageFull, m.listHeight()))

		return m, nil
	case "pgup":
		m.move(-textarea.PageRows(textarea.PageFull, m.listHeight()))

		return m, nil
	case "g", "home":
		m.selected = 0
		m.scrollTo()

		return m, nil
	case "G", "end":
		m.selected = len(m.rows()) - 1
		m.scrollTo()

		return m, nil
	default:
		return m, nil
	}
}

// open 은 고른 줄을 그것을 친 자리에 실어서 연다. 돌리지는 않는다(ADR-0143 §4).
//
// 명령은 `commandModeWith` 로 간다. visual 의 `:` 가 `'<,'>` 를 실어 오는 그 길이다
// (ADR-0089). 검색은 `/` 로 연다 — 이력에 방향이 없어서 앞으로 찾는 쪽이다.
//
// **검색 줄은 미리보기를 돌리지 않은 채로 열린다.** 실어 준 것은 아직 친 것이 아니라서,
// 글자를 하나 더 치거나 Enter 를 누르는 순간 여느 검색과 같아진다.
func (m viewHistory) open() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}

	row := rows[m.selected]

	if row.prompt == "/" {
		next, cmd := searchMode(m.editor, textarea.SearchForward)

		search, ok := next.(viewEditorSearch)
		if !ok {
			return next, cmd
		}
		search.input = newInputLine(row.text)

		return search, cmd
	}

	return commandModeWith(m.editor, row.text)
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다. 다른 목록들과 같은 규칙이다.
func (m *viewHistory) move(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(rows)-1)
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. `:messages` 와 같은 규칙이다.
func (m *viewHistory) scrollTo() {
	rows, height := len(m.rows()), m.listHeight()
	if rows == 0 || height < 1 {
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

func (m viewHistory) View() tea.View {
	height := m.listHeight()

	rows := m.rows()

	body := make([]string, 0, height)
	for i := m.top; i < len(rows) && len(body) < height; i++ {
		body = append(body, m.renderRow(rows[i], i == m.selected))
	}

	if len(rows) == 0 && height > 0 {
		body = append(body, " "+m.emptyReason())
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(m.renderHint()))
	screen = append(screen, m.renderBareStatusBar()...)

	view := newView(screen, m.renderWindowTitle())

	if len(rows) > 0 {
		view.Cursor = tea.NewCursor(0, m.selected-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// renderTitle 은 맨 윗줄이다. 어느 갈래를 몇 개 보이는 중인지 적는다.
//
// 담는 상한이 50 이라 개수가 곧 「얼마나 찼나」다. 상한을 같이 적지 않는 것은 그 값이
// 사람이 정한 것이 아니라서다.
func (m viewHistory) renderTitle() string {
	label := fmt.Sprintf("%s  %d 개", m.kindLabel(), len(m.rows()))

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// kindLabel 은 지금 보이는 갈래의 이름이다.
func (m viewHistory) kindLabel() string {
	switch m.kind {
	case historySearches:
		return "검색 이력"
	case historyAll:
		return "명령줄·검색 이력"
	default:
		return "명령줄 이력"
	}
}

// emptyReason 은 목록이 빈 까닭이다. 갈래마다 다르다 — 「아직 친 것이 없다」가 어느
// 갈래의 말인지 보여야 이력이 사라진 것으로 읽히지 않는다.
func (m viewHistory) emptyReason() string {
	switch m.kind {
	case historySearches:
		return "찾은 것이 없습니다"
	case historyAll:
		return "친 것이 없습니다"
	default:
		return "친 명령이 없습니다"
	}
}

// renderHint 는 키 안내 한 줄이다.
func (m viewHistory) renderHint() string {
	return " j/k 이동  g/G 처음·끝  enter 명령줄에 싣기  q 닫기"
}

// historyPromptWidth 는 갈래 글자와 표시가 쓰는 앞머리 폭이다.
const historyPromptWidth = 1 + 2 + 1 + 1 // 왼쪽 여백 + `▸ ` + `:` + 공백

// renderRow 는 이력 한 줄이다.
//
//	▸ : %s/foo/bar/g
//	  / func main
//
// **갈래 글자를 늘 찍는다.** `all` 이 아닐 때도 찍는 것은 그 글자가 곧 그 줄을 칠 때 맨 앞에
// 서던 것이라, 목록의 한 줄이 「내가 친 그 줄」과 같은 모양으로 읽히기 때문이다.
func (m viewHistory) renderRow(row historyRow, selected bool) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	head := " " + marker + styleDetail.Render(row.prompt) + " "
	text := trimTextRight(row.text, m.width-historyPromptWidth-1)

	return head + text
}

// renderBareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다. `:messages` 와 같은
// 이유다(view-jobs.go) — 이 화면은 트리를 덮는다.
func (m viewHistory) renderBareStatusBar() []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	return bare.renderStatusBar("HISTORY", bare.notice)
}

// runHistory 는 팔레트의 「명령줄 이력」이다. `:history` 와 같은 길이다.
func runHistory(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return historyMode(e, historyCommands)
}
