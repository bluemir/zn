package core

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/textarea"
)

// marksMinTextHeight 는 판을 열고도 편집 영역에 남아야 할 행 수다. 다른 판들과 같다.
const marksMinTextHeight = symbolMinTextHeight

// marksFrame 은 판이 목록 말고 쓰는 행 수다. 테두리 둘이다.
const marksFrame = 2

// marksMode 는 `:marks` 와 팔레트의 「mark 목록」으로 여는 하단 drawer 다(ADR-0154).
//
// **되돌아간 자리 판(`:jumps`) 과 손이 같다.** `j`/`k` 가 커서를 그 mark 로 옮겨 보여주고,
// `enter` 가 확정, `q`·`esc` 가 취소다(ADR-0071). 둘 다 「이 중 어디로 갈까」를 고르는 판이라
// 같은 손이어야 한다.
//
// **지우는 키가 하나 있다.** `x` 다. `:jobs` 가 고른 작업을 거두는 키와 같다. mark 를 지우는
// 길은 이것과 같은 이름으로 다시 찍는 것뿐이다 — `:delmarks` 는 두지 않았다(ADR-0154).
//
// 이 파일의 mark 만 보인다. mark 가 buffer 에 딸려 있어서 tab 을 넘지 않는다. 그래서
// `:jumps` 와 달리 둘러보며 tab 을 열 일이 없고, 되돌릴 것은 커서 자리 하나다.
func marksMode(e *editor) (tea.Model, tea.Cmd) {
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	e.clearNotice()

	m := viewMarks{editor: e, origin: e.activeBuffer().Cursor}
	m.originPlace, m.hasOriginPlace = e.here()

	e.setDrawerHeight(m.marksDrawerHeight())
	e.scrollToCursor()

	return m, nil
}

type viewMarks struct {
	*editor

	selected int // marks() 안의 자리
	top      int // 판의 첫 행

	// origin 은 판을 열기 전 커서 자리다. 취소하면 여기로 돌아간다.
	origin scheme.Cursor

	// originPlace 는 확정할 때 되돌아오기 이력에 담을 자리다. `'a` 로 뛴 것과 같아서다.
	// 이름 없는 buffer 면 담을 수 없어 hasOriginPlace 가 거짓이다(jumplist.go 의 here).
	originPlace    jumpPlace
	hasOriginPlace bool
}

// marks 는 지금 파일에 찍힌 것이다. 이름 순이다.
func (m viewMarks) marks() []textarea.Mark {
	return m.activeBuffer().Marks()
}

// marksRows 는 목록에 쓸 수 있는 행 수다. mark 는 스물여섯을 넘지 않아서 따로 상한을 두지 않는다.
func (m viewMarks) marksRows() int {
	room := m.textAndDrawerHeight() - marksFrame - marksMinTextHeight

	return min(max(len(m.marks()), 1), max(room, 1))
}

// marksDrawerHeight 는 판이 편집 영역에서 가져갈 행 수다.
func (m viewMarks) marksDrawerHeight() int {
	return m.marksRows() + marksFrame
}

func (m viewMarks) Init() tea.Cmd { return nil }

func (m viewMarks) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		m.setDrawerHeight(m.marksDrawerHeight())
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

		return m, nil
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
func (m viewMarks) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewMarks)
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
func (m viewMarks) run(key string) (tea.Model, tea.Cmd) {
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
	case "g", "home":
		m.selectTo(0)

		return m, nil
	case "G", "end":
		m.selectTo(len(m.marks()) - 1)

		return m, nil
	case "x":
		m.deleteSelected()

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 `enter` 다. 고른 mark 로 확정하고 판을 닫는다.
//
// **되돌아오기 이력에 담는다.** `'a` 로 뛴 것과 같은 일이라서다 — 담는 것은 판을 열기 전
// 자리다(ADR-0154). `:jumps` 의 `enter` 가 담지 않는 것은 그쪽이 이력을 되짚는 이동이라서다.
func (m viewMarks) open() (tea.Model, tea.Cmd) {
	marks := m.marks()
	if len(marks) == 0 {
		return m.cancel()
	}

	// 아직 그 자리를 안 밟았을 수 있다 — 열자마자 `enter` 를 친 경우다.
	m.goToMark(marks[m.selected])

	if m.hasOriginPlace {
		m.recordJumpMove(m.originPlace)
	}

	return normalMode(m.editor)
}

// cancel 은 `q`·`esc` 다. 판을 열기 전 자리로 되돌린다. 둘러본 것은 없던 일이다(ADR-0071).
//
// 지운 mark 는 되돌리지 않는다. 지우는 것은 둘러보기가 아니라 시킨 일이다.
func (m viewMarks) cancel() (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()
	buf.MoveTo(m.origin)
	buf.ClampToNormal()
	m.scrollToCursor()

	return normalMode(m.editor)
}

// move 는 고른 자리를 옮기고 그 mark 를 곧바로 보여준다.
func (m *viewMarks) move(delta int) {
	m.selectTo(m.selected + delta)
}

// selectTo 는 고른 자리를 그 index 로 옮기고 커서를 그 mark 로 데려간다. 양끝에서 멈춘다.
//
// **`:jumps` 와 달리 양끝에서 멈춰도 데려간다.** 판을 열 때 첫 줄을 고른 채로 옮기지는 않아서
// 커서가 아직 그 mark 에 없다. 첫 줄에서 친 `k` 가 그리로 가는 길이다.
func (m *viewMarks) selectTo(index int) {
	marks := m.marks()
	if len(marks) == 0 {
		return
	}

	m.selected = min(max(index, 0), len(marks)-1)
	m.scrollTo()

	m.goToMark(marks[m.selected])
}

// goToMark 는 커서를 그 mark 의 칸으로 옮긴다. 백틱 a 가 가는 자리다.
func (m *viewMarks) goToMark(mark textarea.Mark) {
	buf := m.activeBuffer()
	buf.MoveTo(mark.At)
	buf.ClampToNormal()
	m.scrollToCursor()
}

// deleteSelected 는 `x` 다. 고른 mark 를 지운다. 커서는 그 자리에 둔다.
func (m *viewMarks) deleteSelected() {
	marks := m.marks()
	if len(marks) == 0 {
		return
	}

	m.activeBuffer().DeleteMark(marks[m.selected].Name)
	m.notify("mark 를 지웠습니다: " + marks[m.selected].Name)

	// 판이 줄었으면 편집 영역에 돌려준다. 다른 판들이 크기를 다시 재는 자리와 같다.
	m.setDrawerHeight(m.marksDrawerHeight())
	m.scrollToCursor()
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 첫 행을 최소한으로 움직인다.
func (m *viewMarks) scrollTo() {
	rows, height := len(m.marks()), m.marksRows()
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

func (m viewMarks) View() tea.View {
	// 커서는 편집 내용에 그대로 둔다. 간 자리가 보이는 편이 낫다 — 다른 판들과 같다.
	view := m.editorView(tea.CursorBlock, "MARKS", m.noticeOr(m.hint()))

	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	return view
}

// hint 는 statusBar 아래 줄에 적는 말이다.
func (m viewMarks) hint() string {
	// 찍은 것이 없으면 둘러볼 것도 없다. 먹지 않는 키를 권하지 않는다.
	if len(m.marks()) == 0 {
		return "mark 0 개  esc 닫기"
	}

	return "mark  j/k 둘러보기  enter 확정  x 지우기  esc 취소"
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
func (m viewMarks) renderDrawer() string {
	return drawer{
		chars:  m.boxChars,
		width:  m.textWidth(),
		height: m.marksRows(),
		top:    m.top,
		count:  len(m.marks()),
		empty:  "찍은 mark 가 없습니다",
		row:    m.renderRow,
		at:     m.selected + 1,
	}.render()
}

// marksPickWidth 는 고른 줄 표시와 그 뒤 한 칸이다.
const marksPickWidth = 2

// renderRow 는 mark 한 줄이다. vim 의 `:marks` 와 같이 이름·자리·그 줄의 글이다.
//
//	▸a 12:5   func main() {
//	 b 40:1   return nil
//
// 자리는 statusBar 의 `줄:칸` 과 같은 단위다. 칸이 byte 가 아니라 화면 칸이라 눈에 보이는
// 값과 같다(render-status-bar.go 의 renderPosition).
//
// **반전만으로 고른 줄을 나타내지 않는다.** 화면을 글자로 떠서 보는 길이 있어서 `▸` 를
// 같이 둔다(view-jumps.go).
func (m viewMarks) renderRow(at, inner int) string {
	mark := m.marks()[at]

	pick := " "
	if at == m.selected {
		pick = "▸"
	}

	buf := m.activeBuffer()
	line := buf.Line(mark.At.Line)
	col := textarea.ScreenColAt(line, min(mark.At.Col, len(line)), buf.TabWidth())

	head := padTo(fmt.Sprintf("%s %d:%d", mark.Name, mark.At.Line+1, col+1), 10) + " "

	// 칸을 먼저 채우고 그다음에 반전을 입힌다 — 강조 뒤에는 폭을 잴 수 없다.
	body := max(inner-marksPickWidth-textarea.WidthOf(head), 0)
	row := pick + " " + head + padTo(trimTextRight(string(line), body), body)
	if at == m.selected {
		return reverse.Render(row)
	}

	return row
}

// runMarks 는 팔레트의 「mark 목록」이다. `:marks` 와 같은 길이다.
func runMarks(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return marksMode(e)
}
