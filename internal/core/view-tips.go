package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/textarea"
)

// viewTips 는 `:tips` 로 여는 tip 목록이다(ADR-0147).
//
// 화면을 통째로 쓰는 목록이라 `:messages`(ADR-0053)·`:jobs`(ADR-0027)·`:history`(ADR-0143)
// 와 같은 문법을 따른다. 제목 띠, 본문, 키 안내 한 줄, 트리를 지운 statusBar 다.
//
// **아래 줄의 tip 은 스쳐 지나간다**(ADR-0061). 한 번에 한 문장이고 알림이 뜨면 걷히고,
// 좁은 화면에서는 긴 문장이 아예 서지 못한다. 그래서 「방금 그거 뭐였지」와 「편집기에 뭐가
// 있더라」를 둘 다 답할 자리가 없었다. 이 화면이 그 자리다.
//
// **갈래를 보이지 않는다.** 소스에는 열여섯 갈래로 묶여 있지만 그것은 문장을 더할 때 같은
// 갈래 옆에 놓으려고 둔 주석이고(internal/assets/tips.go) 화면에서 갈래로 찾는 일은 거르기가
// 한다. 갈래를 내려면 목록이 문장 대신 구조를 들어야 하는데, 그 대가로 얻는 것이 머리줄
// 열여섯 줄이다.
//
// 번호는 `assets.Tips` 안의 자리다. **걸러도 번호가 바뀌지 않는다** — 걸러 본 뒤에도 그
// 문장이 전체에서 어디쯤인지가 남고, 같은 문장을 두 번 찾을 때 번호로 짚을 수 있다.
type viewTips struct {
	*editor

	selected int // rows() 안의 자리
	top      int // 화면 첫 행

	// filter 는 거르는 글이다. 비어 있으면 전부다. `/` 로 고치고(view-tips-filter.go)
	// 이 화면에 남는다 — 목록을 훑다가 거르고 다시 훑는 것이 이 판의 쓰임이다.
	filter string
}

// tipListRow 는 목록 한 줄이다. at 은 1 부터 센 번호다.
type tipListRow struct {
	at   int
	text string
}

// tipsMode 는 tip 목록을 연다.
//
// at 은 처음에 고를 문장의 `assets.Tips` 안 자리다. 아래 줄의 tip 을 눌러 들어오면 그
// 문장이고(tip.go 의 clickTip), `:tips`·팔레트로 들어오면 맨 위다. 누른 것에서 이어 보는
// 것이 클릭으로 여는 까닭이라, 들어온 길이 고를 자리를 정한다.
func tipsMode(e *editor, at int) (tea.Model, tea.Cmd) {
	m := viewTips{editor: e, selected: at}
	m.scrollTo()

	return m, nil
}

// rows 는 목록에 보일 문장이다. 거르는 중이면 그 글이 든 것만이다.
//
// **고른 자리와 첫 행은 이것 안의 자리다.** 걸러 놓고 `assets.Tips` 로 세면 목록에 없는
// 문장을 가리키게 된다. `:messages` 의 실패만 보기와 같은 규칙이다.
func (m viewTips) rows() []tipListRow {
	rows := make([]tipListRow, 0, len(assets.Tips))

	// 대소문자를 가리지 않는다. tip 은 한국어 문장이라 대소문자가 갈리는 것은 키 이름뿐이고,
	// `ctrl` 을 찾으려고 `Ctrl` 을 다시 쳐 보게 할 까닭이 없다.
	needle := strings.ToLower(m.filter)

	for i, tip := range assets.Tips {
		if needle != "" && !strings.Contains(strings.ToLower(tip), needle) {
			continue
		}

		rows = append(rows, tipListRow{at: i + 1, text: tip})
	}

	return rows
}

func (m viewTips) Init() tea.Cmd { return nil }

func (m viewTips) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
// 거르는 글을 치는 동안은 되돌리지 않는데, 그 자리는 화면이 따로다(view-tips-filter.go).
func (m viewTips) press(key string) (tea.Model, tea.Cmd) {
	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewTips)
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
// `enter` 가 없다. 읽는 판이라 고른 것으로 할 일이 없다 — 알림 기록과 같은 자리이고,
// 이력이 `enter` 를 가진 것은 그쪽이 「다시 쓰려고」 담는 것이어서다(ADR-0143).
func (m viewTips) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		// 거르는 중이어도 한 번에 나간다. 거른 것을 푸는 것은 `/` 를 다시 열어 지우는
		// 길이 있고, 판을 닫는 키가 무엇을 지우는지에 따라 갈리면 두 번 눌러야 한다.
		return normalMode(m.editor)
	case "/":
		return tipsFilterMode(m)
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 PageRows).
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

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다. 다른 목록들과 같은 규칙이다.
func (m *viewTips) move(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(rows)-1)
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. `:messages` 와 같은 규칙이다.
func (m *viewTips) scrollTo() {
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

func (m viewTips) View() tea.View {
	return m.viewWith(m.notice, nil)
}

// viewWith 는 이 판을 그린다. bottom 은 statusBar 아래 줄이고, cursor 가 있으면 그 자리다.
//
// **거르는 글을 치는 화면이 같은 함수를 지난다**(view-tips-filter.go). 목록을 그리는 코드가
// 두 벌이면 거르는 동안 보이는 것과 치고 난 뒤 보이는 것이 갈린다.
func (m viewTips) viewWith(bottom string, cursor *tea.Cursor) tea.View {
	height := m.listHeight()
	rows := m.rows()

	body := make([]string, 0, height)
	for i := m.top; i < len(rows) && len(body) < height; i++ {
		body = append(body, m.renderRow(rows[i], i == m.selected))
	}

	if len(rows) == 0 && height > 0 {
		body = append(body, " 든 문장이 없습니다")
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(m.renderHint()))
	screen = append(screen, m.renderBareStatusBar(bottom)...)

	view := newView(screen, m.renderWindowTitle())

	switch {
	case cursor != nil:
		view.Cursor = cursor
	case len(rows) > 0:
		view.Cursor = tea.NewCursor(0, m.selected-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	default:
		view.Cursor = nil
	}

	return view
}

// renderTitle 은 맨 윗줄이다. 전부 몇 개인지와 거르고 남은 것이 몇인지를 적는다.
//
// **거르는 중에도 전체를 센다.** 남은 수는 목록 길이가 이미 말하고, 제목이 같이 좁아지면
// 무엇에서 걸러낸 것인지 알 수 없다. `:messages` 의 제목과 같은 규칙이다.
func (m viewTips) renderTitle() string {
	label := fmt.Sprintf("tip  %d 개", len(assets.Tips))
	if m.filter != "" {
		label += fmt.Sprintf(" · %q 든 것 %d", m.filter, len(m.rows()))
	}

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// renderHint 는 키 안내 한 줄이다. `/` 는 지금 상태의 **반대**를 적는다 — 누르면 무엇이
// 되는지가 안내이고, 지금 무엇으로 거르는 중인지는 제목줄이 말한다.
func (m viewTips) renderHint() string {
	filter := "/ 거르기"
	if m.filter != "" {
		filter = "/ 거르기 고치기"
	}

	return " j/k 이동  g/G 처음·끝  " + filter + "  q 닫기"
}

// tipNumberWidth 는 번호 칸이 쓰는 앞머리 폭이다. `▸ ` 두 칸까지 더한 값이다.
const tipNumberWidth = 1 + 2 + 3 + 2 // 왼쪽 여백 + 표시 + 번호 세 자리 + 뒤 두 칸

// renderRow 는 tip 한 줄이다.
//
//	▸   1  `ctrl+p` 로 팔레트를 열면 파일 이름으로 바로 찾아 엽니다
//	   12  `%` 는 짝을 이루는 괄호로 건너뜁니다
//
// 번호는 오른쪽 맞춤이라 세 자리까지 세로 줄이 맞는다. 문장이 146 개라 지금은 세 자리다.
func (m viewTips) renderRow(row tipListRow, selected bool) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	head := " " + marker + styleDetail.Render(fmt.Sprintf("%3d", row.at)) + "  "

	return head + trimTextRight(row.text, m.width-tipNumberWidth-1)
}

// renderBareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다. `:messages` 와 같은
// 이유다 — 이 화면은 트리를 덮는다.
func (m viewTips) renderBareStatusBar(bottom string) []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	return bare.renderStatusBar("TIPS", bottom)
}

// runTips 는 팔레트의 「tip 목록」이다. `:tips` 와 같은 길이다.
func runTips(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return tipsMode(e, 0)
}
