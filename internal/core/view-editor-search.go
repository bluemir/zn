package core

import (
	"regexp"

	tea "charm.land/bubbletea/v2"
)

// viewEditorSearch 는 `/` `?` 로 들어가는 검색 입력이다. command mode 와 같은 자리를 쓴다.
//
// 치는 동안 첫 매칭으로 커서와 화면이 따라간다(vim 의 incsearch). 그래서 들어온 자리와
// 들어올 때의 검색을 들고 있다가 `Esc` 로 나가면 되돌린다 — 미리보기가 실제 이동으로 남으면
// 검색을 무를 방법이 없다.
func searchMode(e *editor, direction searchDirection) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	return viewEditorSearch{
		editor:    e,
		direction: direction,
		origin: searchOrigin{
			cursorLine: buf.cursorLine,
			cursorCol:  buf.cursorCol,
			top:        buf.top,
			topRow:     buf.topRow,
			search:     e.search,
		},
	}, nil
}

type viewEditorSearch struct {
	*editor

	direction searchDirection
	input     string // `/` 나 `?` 뒤에 친 것

	origin searchOrigin
}

// searchOrigin 은 검색을 시작한 자리다.
//
// 화면 위치까지 들고 있어야 되돌릴 때 화면이 튀지 않는다. 커서만 되돌리면 scrollTo 가
// 이미 옮겨둔 화면을 그대로 두어서, 커서는 제자리인데 보이는 곳이 달라진다.
type searchOrigin struct {
	cursorLine, cursorCol int
	top, topRow           int

	search searchState
}

func (m viewEditorSearch) Init() tea.Cmd { return nil }

func (m viewEditorSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			m.restore()

			return normalMode(m.editor)
		case "enter":
			return m.run()
		case "backspace":
			// command mode 와 같이 `/` 까지 지우면 검색에서 나간다.
			if m.input == "" {
				m.restore()

				return normalMode(m.editor)
			}
			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]
			m.preview()

			return m, nil
		default:
			if msg.Text == "" {
				return m, nil
			}
			m.input += msg.Text
			m.preview()

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 명령줄과 같다. 다만 다음 글자를 치면 preview 가 첫 매칭으로 화면을 다시 잡아당기므로
		// 굴려둔 것은 그때 사라진다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go). 파일 검사 tick 은
		// 여기서 보지 않고 주기만 이어 간다 — 보는 것은 normal·트리다(ADR-0038).
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// preview 는 치는 동안 첫 매칭으로 화면을 옮긴다. vim 의 incsearch 다.
//
// 항상 시작 자리에서 다시 찾는다. 옮겨간 자리에서 이어 찾으면 글자를 지웠을 때 커서가
// 앞으로 돌아오지 않아서, 같은 글자를 쳤는데 다른 곳에 서 있게 된다.
//
// 패턴이 아직 정규식으로 말이 되지 않거나 못 찾으면 시작 자리에 머문다.
// 치는 도중에는 대부분이 그 상태라 오류를 띄우지 않는다. 오류는 Enter 때 낸다.
func (m *viewEditorSearch) preview() {
	m.restore()

	if m.input == "" {
		return
	}

	pattern, err := parseSearchPattern(m.input)
	if err != nil {
		return
	}

	buf := m.activeBuffer()

	result, ok := buf.find(pattern, m.direction, m.origin.cursorLine, m.origin.cursorCol)
	if !ok {
		return
	}

	// 찾은 자리를 미리 강조한다. 아직 마지막 검색으로 굳히는 것은 아니라 Esc 로 되돌아간다.
	m.search = searchState{input: m.input, pattern: pattern, direction: m.direction, highlight: true}

	buf.moveTo(result.line, result.col, m.contentWidth())
	buf.clampToNormal(m.contentWidth())
	m.scrollToCursor()
}

// restore 는 미리보기로 옮긴 커서와 화면을 시작 자리로 되돌린다.
func (m *viewEditorSearch) restore() {
	buf := m.activeBuffer()

	buf.cursorLine, buf.cursorCol = m.origin.cursorLine, m.origin.cursorCol
	buf.top, buf.topRow = m.origin.top, m.origin.topRow
	buf.updateDesiredCol(m.contentWidth())

	m.search = m.origin.search
}

// run 은 친 패턴으로 실제 검색을 한다.
func (m viewEditorSearch) run() (tea.Model, tea.Cmd) {
	// 미리보기로 옮겨둔 커서에서 다시 찾으면 첫 매칭을 건너뛴다. 시작 자리에서 찾는다.
	m.restore()

	// 빈 채로 Enter 는 마지막 검색을 이 방향으로 되풀이한다. vim 과 같다.
	pattern, input := m.origin.search.pattern, m.origin.search.input
	if m.input != "" {
		compiled, err := parseSearchPattern(m.input)
		if err != nil {
			return normalModeError(m.editor, err)
		}

		pattern, input = compiled, m.input
	}
	if pattern == nil {
		return normalModeMessage(m.editor, "이전 검색이 없습니다")
	}

	m.search = searchState{input: input, pattern: pattern, direction: m.direction, highlight: true}

	// 여기가 진짜 mode 전환이다 — SEARCH 를 끝내고 normal 로 나온다.
	// 알림은 jumpToMatch 가 세워 두었으므로 그대로 들고 나간다.
	m.jumpToMatch(m.direction, 1)

	return normalMode(m.editor)
}

func (m viewEditorSearch) View() tea.View {
	line := m.prompt() + m.input
	view := m.editorView(tea.CursorBlock, "SEARCH", line)

	// 커서는 본문이 아니라 명령줄 끝에 있어야 한다. command mode 와 같은 자리다.
	view.Cursor = tea.NewCursor(screenWidthOf(line)+m.sidebarLeft(), m.height-1)

	return view
}

// prompt 는 명령줄 맨 앞 글자다. 어느 방향으로 찾는 중인지가 이것으로 보인다. vim 과 같다.
func (m viewEditorSearch) prompt() string {
	if m.direction == searchBackward {
		return "?"
	}

	return "/"
}

// jumpToMatch 는 지금 검색을 n 번 되풀이해 커서를 옮기고 결과를 아래 줄에 알린다.
// `/` `?` `n` `N` `*` `#` 가 모두 이 길로 온다.
//
// **mode 는 정하지 않는다.** 부르는 쪽이 둘이고 원하는 것이 다르다 — `/` 는 SEARCH 를 끝내고
// normal 로 나오는 진짜 전환이지만, `n` 은 normal 에 머문다. 여기서 `normalMode(e)` 를
// 돌려주면 그 둘이 같은 모양이 되어 `press` 가 mode 가 바뀌었는지 알 수 없다(ADR-0034).
func (e *editor) jumpToMatch(direction searchDirection, n int) {
	if e.search.pattern == nil {
		e.notify("이전 검색이 없습니다")

		return
	}

	buf := e.activeBuffer()
	width := e.contentWidth()

	line, col := buf.cursorLine, buf.cursorCol
	wrapped := false

	for range n {
		result, ok := buf.find(e.search.pattern, direction, line, col)
		if !ok {
			// 하나도 못 찾았으면 커서를 두고 알리기만 한다. 도중까지 옮기면 어디로 갔는지 알 수 없다.
			e.notify("찾을 수 없음: " + e.search.input)

			return
		}

		line, col = result.line, result.col
		wrapped = wrapped || result.wrapped
	}

	// **찾은 것을 확인한 뒤에 담는다.** 못 찾으면 커서가 그대로라 담을 것도 없다.
	// 여기가 `/` `?` `n` `N` `*` `#` 이 모두 지나는 자리다(ADR-0070).
	e.recordJump()

	buf.moveTo(line, col, width)
	buf.clampToNormal(width)
	e.scrollToCursor()

	// 닿은 자리도 방문 기록에 남는다. 떠난 자리는 위의 recordJump 가 남겼다(ADR-0074).
	e.arrive()

	if wrapped {
		e.notify(wrapMessage(direction))
	}
}

// wrapMessage 는 파일 끝을 지나 감쌌음을 알리는 말이다.
func wrapMessage(direction searchDirection) string {
	if direction == searchBackward {
		return "위에서 끝으로 돌아옴"
	}

	return "아래에서 처음으로 돌아옴"
}

// searchWord 는 커서 아래 단어를 그대로 찾는다. vim 의 `*` `#` 다.
func (e *editor) searchWord(direction searchDirection, n int) {
	buf := e.activeBuffer()

	word, col, ok := buf.wordUnderCursor()
	if !ok {
		e.notify("커서 아래에 단어가 없습니다")

		return
	}

	// 커서를 단어 앞으로 옮기고 거기서 찾는다. 옮기지 않으면 커서 오른쪽에 있던 그 단어가
	// 첫 매칭이 되어, `*` 를 눌렀는데 제자리에서 한 칸 옆으로 가는 것으로 끝난다. vim 과 같다.
	buf.moveTo(buf.cursorLine, col, e.contentWidth())

	// QuoteMeta 를 거친 글자와 `\b` 뿐이라 정규식이 될 수 없는 경우가 없다.
	input := wordSearchPattern(word)
	e.search = searchState{
		input:     input,
		pattern:   regexp.MustCompile(input),
		direction: direction,
		highlight: true,
	}

	e.jumpToMatch(direction, n)
}
