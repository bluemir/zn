package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// jumplogsMinTextHeight 는 판을 열고도 편집 영역에 남아야 할 행 수다. 다른 판들과 같다.
const jumplogsMinTextHeight = symbolMinTextHeight

// jumplogsFrame 은 판이 목록 말고 쓰는 행 수다. 테두리 둘이다.
const jumplogsFrame = 2

// jumplogsMaxRows 는 목록에 보일 최대 행 수다. 다른 목록 판과 같은 값이다(ADR-0069).
//
// **담는 것(256) 과 보이는 것(16) 이 다르다.** 판이 화면을 다 먹으면 간 자리가 안 보이는데,
// 기록은 짧으면 값이 없다 — 「아까 그 자리」가 스무 번째일 수 있다. `j`/`k` 가 훑는다.
const jumplogsMaxRows = locationsMaxRows

// jumplogsMode 는 `:jumplogs` 와 팔레트의 「방문한 자리」로 여는 하단 drawer 다(ADR-0074).
//
// 되돌아간 자리 판(view-jumps.go) 과 손이 같다 — `j`/`k` 가 그 자리를 보여주고 `enter` 가
// 판을 닫으며 확정하고 `q`·`esc` 는 취소다(ADR-0071, ADR-0073).
//
// **판을 합치지 않았다.** 저쪽은 `ctrl+o`·`ctrl+i` 가 서 있는 자리(`●`) 를 들고 확정할 때
// 그 자리를 옮기는데, 이쪽에는 그런 것이 없다 — 읽고 고르기만 한다. 그리는 줄도 다르다
// (저쪽은 표시가 둘, 이쪽은 하나). 같이 드는 「둘러보기 전 화면」만 나눠 쓴다(preview.go).
func jumplogsMode(e *editor) (tea.Model, tea.Cmd) {
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	e.clearNotice()

	m := viewJumplogs{editor: e}

	// 둘러보기 전 화면을 적어 둔다. 취소하면 이것으로 되돌린다(preview.go).
	//
	// **여는 것만으로는 옮기지 않는다.** 미리보기는 `j`/`k` 를 쳐야 시작한다.
	m.preview = startPreview(e)

	e.drawerHeight = m.jumplogsDrawerHeight()
	e.scrollToCursor()

	return m, nil
}

type viewJumplogs struct {
	*editor

	selected int // logs.places 안의 자리. 0 이 가장 최근이다
	top      int // 판의 첫 행

	preview previewSession
	pending tea.Cmd
}

// jumplogsRows 는 목록에 쓸 수 있는 행 수다.
func (m viewJumplogs) jumplogsRows() int {
	room := m.paneHeight() - jumplogsFrame - jumplogsMinTextHeight
	want := min(max(len(m.logs.places), 1), jumplogsMaxRows)

	return min(want, max(room, 1))
}

// jumplogsDrawerHeight 는 판이 편집 영역에서 가져갈 행 수다.
func (m viewJumplogs) jumplogsDrawerHeight() int {
	return m.jumplogsRows() + jumplogsFrame
}

func (m viewJumplogs) Init() tea.Cmd { return nil }

func (m viewJumplogs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		m.drawerHeight = m.jumplogsDrawerHeight()
		m.scrollToCursor()
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

		cmd := m.pending
		m.pending = nil

		return m, cmd
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg:
		// **판 높이는 여기서 지우지 않는다.** 넘어가는 곳이 또 판이면 그쪽이 방금 잡은
		// 높이를 우리가 지우게 된다(ADR-0069).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}
		m.scrollTo()

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다. 한글은 되돌린다 — 입력줄이 없는 판이다(ADR-0008).
func (m viewJumplogs) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	// **미리보기가 낸 Cmd 를 모아서 나간다.** 자모 하나가 키 여럿으로 풀리므로(`접` 은
	// `w`·`j`·`q` 다) 앞 키가 미리보기를 태우고 뒤 키가 이 화면을 벗어날 수 있다. 그때
	// 흘리면 안 되는 것이 그 안에 있다 — 파일을 연 뒤 언어 서버를 띄우는 Cmd 인데,
	// startGopls 는 이미 `goplsStarting` 을 세워 두어서 Cmd 를 버리면 서버가 영영 뜨지
	// 않는다(ADR-0008, ADR-0051).
	var previews []tea.Cmd

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewJumplogs)
		if !ok {
			return model, tea.Batch(previews...)
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, tea.Batch(append(previews, cmd)...)
		}

		// 미리보기 Cmd 는 여기서 걷는다. run 에서 돌려주면 이 고리가 그것을 「이 화면을
		// 벗어났다」로 읽어 남은 자모를 버린다.
		if after, ok := next.(viewJumplogs); ok && after.pending != nil {
			previews = append(previews, after.pending)
			after.pending = nil
			next = after
		}

		model = next
	}

	return model, tea.Batch(previews...)
}

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
//
// 기록을 지우는 키는 없다. 읽고 고르는 판이다.
func (m viewJumplogs) run(key string) (tea.Model, tea.Cmd) {
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
		m.move(pageRows(pageFull, m.jumplogsDrawerHeight()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.jumplogsDrawerHeight()))

		return m, nil
	case "g", "home":
		m.selectTo(0)

		return m, nil
	case "G", "end":
		m.selectTo(len(m.logs.places) - 1)

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 `enter` 다. 둘러보던 자리로 확정하고 판을 닫는다(ADR-0073).
//
// 남기는 것이 둘이다.
//
//   - **방문 기록에 담는다.** 실제로 그 자리에 갔으니 방문이고, 이미 목록에 있던 자리라
//     늘지 않고 맨 위로 올라온다. `ctrl+o` 를 방문으로 치는 것과 같은 규칙이다(ADR-0074).
//   - **되돌아오기 이력(jumplist) 에 떠난 자리를 담는다.** 기록에서 먼 자리로 뛰는 것은
//     진짜 jump 라 `ctrl+o` 로 돌아올 수 있어야 한다. 되돌아간 자리 판(view-jumps.go) 이
//     이것을 담지 않는 것과 갈리는 자리다 — 그쪽은 이력 자체를 오가는 것이라 담으면
//     이력이 어그러진다.
func (m viewJumplogs) open() (tea.Model, tea.Cmd) {
	if len(m.logs.places) == 0 {
		return m.cancel()
	}

	place := m.logs.places[m.selected]

	if m.preview.hasOrigin {
		m.recordJumpFrom(m.preview.origin)
	}

	// 아직 그 자리를 안 밟았을 수 있다 — 열자마자 `enter` 를 친 경우다.
	cmd := m.goToPlace(place)
	m.arrive()

	m.preview.keep(m.editor, place.path)

	model, next := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(next, cmd, m.startGitRefresh())
}

// cancel 은 `q`·`esc` 다. 판을 열기 전 자리로 되돌리고 둘러보며 연 tab 을 전부 닫는다.
func (m viewJumplogs) cancel() (tea.Model, tea.Cmd) {
	cmd := m.preview.restore(m.editor)

	model, next := normalMode(m.editor)

	return model, tea.Batch(next, cmd)
}

// move 는 고른 자리를 옮기고 그 자리를 곧바로 보여준다.
func (m *viewJumplogs) move(delta int) {
	m.selectTo(m.selected + delta)
}

// selectTo 는 고른 자리를 그 index 로 옮기고 미리보기를 태운다. 양끝에서 멈춘다.
func (m *viewJumplogs) selectTo(index int) {
	if len(m.logs.places) == 0 {
		return
	}

	before := m.selected

	m.selected = min(max(index, 0), len(m.logs.places)-1)
	m.scrollTo()

	if m.selected == before {
		return
	}

	m.pending = m.goToPlace(m.logs.places[m.selected])
}

// scrollTo 는 고른 자리가 보이도록 첫 행을 최소한으로 움직인다.
//
// **담는 것이 보이는 것보다 훨씬 많아서**(256 대 16) 이 판에서는 훑는 일이 늘 일어난다.
func (m *viewJumplogs) scrollTo() {
	rows, height := len(m.logs.places), m.jumplogsRows()
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

func (m viewJumplogs) View() tea.View {
	view := m.editorView(tea.CursorBlock, "JUMPLOGS", m.noticeOr(m.hint()))

	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	return view
}

// hint 는 statusBar 아래 줄에 적는 말이다.
//
// 몇 번째를 보고 있는지를 같이 적는다. 담는 것이 256 개라 목록만으로는 어디쯤인지 모른다 —
// 다른 판들은 열여섯 줄 안에 다 들어가서 그 값이 필요 없었다.
func (m viewJumplogs) hint() string {
	if len(m.logs.places) == 0 {
		return "방문한 자리 0 개  esc 닫기"
	}

	return fmt.Sprintf("방문한 자리 %d/%d  j/k 둘러보기  enter 확정  esc 취소",
		m.selected+1, len(m.logs.places))
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
func (m viewJumplogs) renderDrawer() string {
	width := m.textWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{chars.topLeft + line + chars.topRight}
	rows = append(rows, m.renderListRows(inner)...)
	rows = append(rows, chars.bottomLeft+line+chars.bottomRight)

	return strings.Join(rows, "\n")
}

// jumplogsMarkWidth 는 고른 자리 표시가 쓰는 폭이다.
const jumplogsMarkWidth = 2

// renderListRows 는 목록 행들이다. 기록이 없으면 그 사실을 한 줄로 알린다.
func (m viewJumplogs) renderListRows(inner int) []string {
	side := m.boxChars.vertical
	height := m.jumplogsRows()

	body := make([]string, 0, height)
	for at := m.top; at < len(m.logs.places) && len(body) < height; at++ {
		body = append(body, m.renderRow(at, inner))
	}

	if len(m.logs.places) == 0 && height > 0 {
		empty := padTo(truncateToWidth("방문한 자리가 없습니다", inner), inner)
		body = append(body, styleDetail.Render(empty))
	}

	for len(body) < height {
		body = append(body, strings.Repeat(" ", inner))
	}

	rows := make([]string, 0, len(body))
	for _, text := range body {
		rows = append(rows, side+" "+text+" "+side)
	}

	return rows
}

// renderRow 는 방문 한 줄이다. **맨 위가 가장 최근**이다 — 브라우저 방문 기록과 같다.
//
// 되돌아간 자리 판과 방향이 반대인데, 저쪽은 `ctrl+o` 거리 순이라 기준이 다르다(ADR-0074).
//
// 표시가 하나뿐이다. 저쪽의 `●`(지금 자리) 에 해당하는 것이 이 판에는 없다 — 읽고 고르기만 한다.
func (m viewJumplogs) renderRow(at, inner int) string {
	marker := "  "
	if at == m.selected {
		marker = "▸ "
	}

	place := m.logs.places[at]
	text := fmt.Sprintf("%s:%d", shortenPath(place.path), place.line+1)

	// 왼쪽부터 접는다. 고르는 데 쓰는 것이 파일 이름과 줄 번호다(view-locations.go).
	body := max(inner-jumplogsMarkWidth, 0)

	// 칸을 먼저 채우고 그다음에 반전을 입힌다 — 강조 뒤에는 폭을 잴 수 없다.
	row := marker + padTo(trimLeftToWidth(text, body), body)
	if at == m.selected {
		return reverse.Render(row)
	}

	return row
}

// runJumplogs 는 팔레트의 「방문한 자리」다. `:jumplogs` 와 같은 길이다.
func runJumplogs(e *editor) (tea.Model, tea.Cmd) {
	return jumplogsMode(e)
}
