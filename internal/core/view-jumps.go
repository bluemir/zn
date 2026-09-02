package core

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// jumpsMinTextHeight 는 판을 열고도 편집 영역에 남아야 할 행 수다. 다른 판들과 같다.
const jumpsMinTextHeight = symbolMinTextHeight

// jumpsFrame 은 판이 목록 말고 쓰는 행 수다. 테두리 둘이다.
const jumpsFrame = 2

// jumpsMaxRows 는 목록에 보일 최대 행 수다. `GOTO` 판과 같은 값이고 같은 까닭이다 —
// 이력은 백 개까지 쌓이므로(jumpListMax) 상한이 없으면 판이 화면을 다 먹는다(ADR-0069).
const jumpsMaxRows = locationsMaxRows

// jumpsMode 는 `:jumps` 와 팔레트의 「되돌아간 자리」로 여는 하단 drawer 다(ADR-0070).
//
// **둘러보고 확정하는 판이다.** `j`/`k` 가 커서를 실제로 그 자리로 옮겨 **보여주고**,
// `enter` 가 판을 닫으며 그것으로 **확정**한다. `q`·`esc` 는 취소라 판을 열기 전 자리로
// 되돌아간다 — 검색의 `esc` 가 커서를 되돌리는 것과 같은 손이다(ADR-0071).
//
// **`GOTO` 판(ADR-0069) 과 손이 다르다.** 그쪽은 사용처 여러 군데를 차례로 **훑는** 것이
// 목적이라 `enter` 로 뛰어도 닫히지 않는데, 이쪽은 이력에서 한 자리를 **고르는** 것이다.
// 그리는 것이 닮았다고 손까지 같아야 하는 것은 아니다.
//
// 든 것도 다르다 — `GOTO` 는 순서에 뜻이 없는 **후보 집합**이고 이쪽은 **현재 자리가 있는
// 이력**이다. `ctrl+o`·`ctrl+i` 가 그 자리를 앞뒤로 옮기고, 판은 그것을 들여다보는 창일
// 뿐이라 판을 안 열고도 이력은 돈다.
func jumpsMode(e *editor) (tea.Model, tea.Cmd) {
	// 되돌아갈 자리를 보는 판이라 편집 맥락에 딸려 있다. 다른 판들과 같다(ADR-0064).
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	e.clearNotice()

	m := viewJumps{editor: e}

	// 둘러보기 전 화면을 적어 둔다. 취소하면 이것으로 되돌린다(preview.go).
	m.preview = startPreview(e)

	// 지금 자리가 보이도록 열린다. 이력 끝에 서 있으면 맨 아래다.
	// **여는 것만으로는 옮기지 않는다.** 미리보기는 `j`/`k` 를 쳐야 시작한다.
	m.selected = min(e.jumps.at, max(len(e.jumps.places)-1, 0))
	m.scrollTo()

	e.drawerHeight = m.jumpsDrawerHeight()
	e.scrollToCursor()

	return m, nil
}

type viewJumps struct {
	*editor

	selected int // jumps.places 안의 자리
	top      int // 판의 첫 행

	// preview 는 둘러보기 전 화면이다. 취소하면 이것으로 되돌린다(preview.go).
	preview previewSession

	// pending 은 방금 미리보기가 낸 Cmd 다. selectTo 가 포인터 수신자라 Cmd 를 돌려줄
	// 자리가 없어서 여기 놓고 부르는 쪽이 집어 간다 — 파일을 열면 트리를 데려가는 Cmd 가
	// 딸려 온다.
	pending tea.Cmd
}

// jumpsRows 는 목록에 쓸 수 있는 행 수다. `GOTO` 판과 같은 셈이다.
func (m viewJumps) jumpsRows() int {
	room := m.textAndDrawerHeight() - jumpsFrame - jumpsMinTextHeight
	want := min(max(len(m.jumps.places), 1), jumpsMaxRows)

	return min(want, max(room, 1))
}

// jumpsDrawerHeight 는 판이 편집 영역에서 가져갈 행 수다.
func (m viewJumps) jumpsDrawerHeight() int {
	return m.jumpsRows() + jumpsFrame
}

func (m viewJumps) Init() tea.Cmd { return nil }

func (m viewJumps) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		m.drawerHeight = m.jumpsDrawerHeight()
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
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// **판 높이는 여기서 지우지 않는다.** 넘어가는 곳이 또 판이면 그쪽이 방금 잡은
		// 높이를 우리가 지우게 된다. 판이 아닌 곳으로 가는 길은 normalMode 가 지운다(ADR-0069).
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
func (m viewJumps) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	// **미리보기가 낸 Cmd 를 모아서 나간다.** 자모 하나가 키 여럿으로 풀리므로(`접` 은
	// `w`·`j`·`q` 다) 앞 키가 미리보기를 태우고 뒤 키가 이 화면을 벗어날 수 있다. 그때
	// 흘리면 안 되는 것이 그 안에 있다 — 파일을 연 뒤 언어 서버를 띄우는 Cmd 인데,
	// startServer 는 이미 그 서버의 `starting` 을 세워 두어서 Cmd 를 버리면 서버가 영영 뜨지
	// 않는다(ADR-0008, ADR-0051).
	var previews []tea.Cmd

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewJumps)
		if !ok {
			return model, tea.Batch(previews...)
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, tea.Batch(append(previews, cmd)...)
		}

		// 미리보기 Cmd 는 여기서 걷는다. run 에서 돌려주면 이 고리가 그것을 「이 화면을
		// 벗어났다」로 읽어 남은 자모를 버린다.
		if after, ok := next.(viewJumps); ok && after.pending != nil {
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
// 이력을 지우는 키는 없다. 읽고 고르는 판이다.
func (m viewJumps) run(key string) (tea.Model, tea.Cmd) {
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
		m.move(pageRows(pageFull, m.jumpsDrawerHeight()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.jumpsDrawerHeight()))

		return m, nil
	case "g", "home":
		m.selectTo(0)

		return m, nil
	case "G", "end":
		m.selectTo(len(m.jumps.places) - 1)

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 `enter` 다. 둘러보던 자리로 **확정하고 판을 닫는다**(ADR-0071).
//
// **이력에 담지 않는다.** 되짚는 이동이라 `ctrl+o` 와 같은 갈래다(jumplist.go 의 goToPlace).
// 대신 **지금 자리를 그리로 옮긴다** — 판을 닫고 `ctrl+o` 를 치면 거기서부터 이어진다.
func (m viewJumps) open() (tea.Model, tea.Cmd) {
	if len(m.jumps.places) == 0 {
		return m.cancel()
	}

	place := m.jumps.places[m.selected]

	// 아직 그 자리를 안 밟았을 수 있다 — 열자마자 `enter` 를 친 경우다.
	cmd := m.goToPlace(place)
	m.arrive()
	m.jumps.at = m.selected

	// 둘러보느라 연 tab 중 확정한 것만 남긴다.
	m.preview.keep(m.editor, place.path)

	model, next := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(next, cmd, m.startGitRefresh())
}

// cancel 은 `q`·`esc` 다. 판을 열기 전 자리로 되돌리고 둘러보며 연 tab 을 전부 닫는다.
//
// **둘러본 것이 없던 일이 되어야 마음 놓고 훑을 수 있다.** 검색의 `esc` 가 커서·화면을
// 되돌리는 것과 같은 약속이다(ADR-0010, ADR-0071).
//
// 지금 자리(`jumps.at`) 도 건드리지 않는다 — 확정한 것이 없으니 이력은 그대로다.
func (m viewJumps) cancel() (tea.Model, tea.Cmd) {
	cmd := m.preview.restore(m.editor)

	model, next := normalMode(m.editor)

	return model, tea.Batch(next, cmd)
}

// move 는 고른 자리를 옮기고 **그 자리를 곧바로 보여준다**(ADR-0071).
//
// 옮기는 것이 미리보기다 — 확정 전이라 이력의 지금 자리(`jumps.at`) 는 그대로이고,
// 판에서 `●` 가 움직이지 않는 것으로 그것이 드러난다.
//
// 닫혀 있는 파일은 tab 으로 열린다. 그렇게 연 것은 나갈 때 정리한다(preview.go 의 closeOpened).
func (m *viewJumps) move(delta int) {
	if len(m.jumps.places) == 0 {
		return
	}

	m.selectTo(m.selected + delta)
}

// selectTo 는 고른 자리를 그 index 로 옮기고 미리보기를 태운다. 양끝에서 멈춘다.
// `j`/`k`·`g`/`G`·휠이 모두 여기로 온다 — 「고른 자리가 바뀌면 보여준다」가 한 자리에 있다.
func (m *viewJumps) selectTo(index int) {
	if len(m.jumps.places) == 0 {
		return
	}

	before := m.selected

	m.selected = min(max(index, 0), len(m.jumps.places)-1)
	m.scrollTo()

	// 양끝에서 멈췄으면 다시 그리로 갈 것이 없다.
	if m.selected == before {
		return
	}

	m.pending = m.goToPlace(m.jumps.places[m.selected])
}

// scrollTo 는 고른 자리가 보이도록 첫 행을 최소한으로 움직인다.
func (m *viewJumps) scrollTo() {
	rows, height := len(m.jumps.places), m.jumpsRows()
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

func (m viewJumps) View() tea.View {
	// 커서는 편집 내용에 그대로 둔다. 간 자리가 보이는 편이 낫다 — 다른 판들과 같다.
	view := m.editorView(tea.CursorBlock, "JUMPS", m.noticeOr(m.hint()))

	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	return view
}

// hint 는 statusBar 아래 줄에 적는 말이다. `GOTO` 판과 같은 자리다(ADR-0069).
func (m viewJumps) hint() string {
	// 담긴 것이 없으면 둘러볼 것도 없다. 먹지 않는 키를 권하지 않는다.
	if len(m.jumps.places) == 0 {
		return "되돌아간 자리 0 개  esc 닫기"
	}

	// 개수는 아랫 테두리가 든다(ADR-0079). 여기 남는 것은 표시의 뜻과 키다.
	return "되돌아간 자리  ● 지금  j/k 둘러보기  enter 확정  esc 취소"
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
func (m viewJumps) renderDrawer() string {
	return drawer{
		chars:  m.boxChars,
		width:  m.textWidth(),
		height: m.jumpsRows(),
		top:    m.top,
		count:  len(m.jumps.places),
		empty:  "되돌아간 자리가 없습니다",
		row:    m.renderRow,
		at:     m.selected + 1,
	}.render()
}

// jumpsMarkWidth 는 표시 둘(고른 자리·지금 자리) 과 그 뒤 한 칸이다.
const jumpsMarkWidth = 3

// renderRow 는 이력 한 줄이다. 위가 오래된 것이고 아래로 갈수록 새것이다. vim 의 `:jumps` 와 같다.
//
//	▸● internal/core/language-server.go:7
//	   internal/core/rename.go:42
//
// **표시가 둘이다.** `▸` 는 이 판에서 고른 줄이고 `●` 는 `ctrl+o`·`ctrl+i` 가 서 있는
// 지금 자리다. 둘은 겹칠 수 있어서 칸을 나눠 가진다 — 한 칸에 몰면 서로를 가린다.
//
// **반전만으로 고른 줄을 나타내지 않는다.** 화면을 글자로 떠서 보는 길이 있어서
// (.claude/skills/run-zn) 색으로만 갈리면 그 길에서 사라진다(view-messages.go).
//
// 이력 끝에 서 있으면(`at == len(places)`) `●` 가 어디에도 없다. 되짚어 들어오지 않은
// 상태이고, 그때 `ctrl+o` 는 맨 아래 줄로 간다.
func (m viewJumps) renderRow(at, inner int) string {
	pick := " "
	if at == m.selected {
		pick = "▸"
	}

	now := " "
	if at == m.jumps.at {
		now = "●"
	}

	place := m.jumps.places[at]
	text := fmt.Sprintf("%s:%d", shortenPath(place.path), place.line+1)

	// 왼쪽부터 접는다. 고르는 데 쓰는 것이 파일 이름과 줄 번호다(view-locations.go).
	body := max(inner-jumpsMarkWidth, 0)

	// 칸을 먼저 채우고 그다음에 반전을 입힌다 — 강조 뒤에는 폭을 잴 수 없다.
	row := pick + now + " " + padTo(trimLeftToWidth(text, body), body)
	if at == m.selected {
		return reverse.Render(row)
	}

	return row
}

// runJumps 는 팔레트의 「되돌아간 자리」다. `:jumps` 와 같은 길이다.
func runJumps(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return jumpsMode(e)
}
