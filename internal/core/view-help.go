package core

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/textarea"
)

// viewHelp 는 `:help` 로 여는 도움말이다(ADR-0149).
//
// `:tips`·`:messages` 와 같이 화면을 통째로 쓴다. 제목 띠, 본문, 키 안내 한 줄, 트리를 지운
// statusBar 다. 하단 drawer 로 내리지 않는 것은 diff 판과 같은 까닭이다 — 고르는 목록이
// 아니라 읽는 글이고, 열여섯 줄로는 한 절도 다 안 들어간다(ADR-0140).
//
// # 목록 판들과 갈리는 자리
//
// **줄이 아니라 글이다.** 목록 판들은 항목 하나가 화면 한 행인데 도움말은 한 문단이 한
// 줄이라(저장소의 문서 규칙) 화면 폭에 맞춰 접어야 한다. 접기·스크롤·검색은 편집 화면이
// 이미 다 갖고 있어서, 이 판은 그것을 **창 하나로** 빌린다.
//
// **창을 들되 tab 에 넣지 않는다.** `textarea.NewBuffer` 로 지은 것을 이 model 이 혼자
// 들고 있다. tab 으로 열면 tabline 에 도움말이 서고 있지도 않은 파일 이름이 statusBar 에
// 뜬다. 판으로 두면 `q` 로 닫히고 편집하던 자리가 그대로 남는다.
//
// 그래서 여기서 새로 지은 것은 키 표와 그리는 차례뿐이다. 접기(`VisibleRows`), 문법
// 강조(`LexSyntaxTo`), 찾기(`Find`) 는 전부 창의 것이다.
type viewHelp struct {
	*editor

	// buf 는 도움말 글을 담은 창이다. 판을 열 때 짓고 닫으면 버린다.
	//
	// **포인터다.** `LexSyntaxTo` 가 훑은 것을 창 안에 담아 두는데(editor.go 의 그리는
	// 자리와 같다) 값으로 들면 그리는 함수가 받은 복사본에만 담겨서 프레임마다 다시 훑는다.
	buf *textarea.Viewport

	// pattern 은 `/` 로 찾은 것이다. 비어 있으면 색이 붙지 않는다.
	//
	// **editor.search 를 쓰지 않는다.** 그쪽은 편집 화면의 찾기라, 도움말에서 찾은 것이
	// 거기 남으면 판을 닫은 뒤 `n` 이 도움말에서 찾던 것을 파일에서 찾는다.
	pattern *regexp.Regexp
	query   string
}

// helpPath 는 창에게 주는 이름이다. 실제로 있는 파일이 아니다.
//
// **확장자가 하는 일이 있다.** `syntax.LanguageFor` 가 이것을 보고 마크다운을 고르므로
// 머리줄과 표에 색이 붙는다. 이름을 `help` 로만 두면 색 없는 글이 된다.
const helpPath = "help.md"

// helpMode 는 도움말을 연다.
//
// **볼 파일이 없어도 연다.** 보여줄 글을 제가 들고 있어서 지금 buffer 를 물어볼 일이 없다.
// 빈 화면에서 「이제 뭘 하지」를 묻는 자리가 곧 여기라, 그때 못 열면 쓸모의 절반이 빠진다.
func helpMode(e *editor) (tea.Model, tea.Cmd) {
	e.clearNotice()

	buf := textarea.NewBuffer(helpPath, []byte(assets.Help))

	// 고쳐지지 않는다. 담긴 것이 바이너리 안의 글이라 저장할 곳도 없다.
	buf.ReadOnly = true

	m := viewHelp{editor: e, buf: &buf}
	m.layout()

	return m, nil
}

// layout 은 창에게 그릴 크기를 준다. 화면이 바뀔 때마다 부른다.
//
// 목록 판들이 `listHeight` 를 쓰는 것과 같은 자리다. 제목줄과 키 안내가 한 줄씩 가져간다.
func (m *viewHelp) layout() {
	m.buf.Size = textarea.ViewSize{Width: m.width, Height: m.listHeight()}
	m.buf.ScrollTo(m.listHeight())
}

func (m viewHelp) Init() tea.Cmd { return nil }

func (m viewHelp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.layout()

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

// press 는 키 하나를 받는다. 한글은 두벌식 자리의 영문 키로 되돌린다(ADR-0008).
func (m viewHelp) press(key string) (tea.Model, tea.Cmd) {
	for _, one := range expandHangul(key) {
		next, cmd := m.run(one)
		if next != m {
			return next, cmd
		}
	}

	return m, nil
}

func (m viewHelp) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return normalMode(m.editor)
	case "/":
		return helpSearchMode(m)
	case "n":
		m.jump(textarea.SearchForward)
	case "N":
		m.jump(textarea.SearchBackward)
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d":
		m.move(textarea.PageRows(textarea.PageHalf, m.listHeight()))
	case "ctrl+u":
		m.move(-textarea.PageRows(textarea.PageHalf, m.listHeight()))
	case "pgdown", "ctrl+f":
		m.move(textarea.PageRows(textarea.PageFull, m.listHeight()))
	case "pgup", "ctrl+b":
		m.move(-textarea.PageRows(textarea.PageFull, m.listHeight()))
	case "g", "home":
		// 판들은 `g` 한 번이다(`:tips`·`:messages`). 편집 영역의 `gg` 와 갈리는데, 저쪽은
		// `g` 가 접두 키라 다음 키를 기다려야 하고 여기는 기다릴 것이 없다.
		m.buf.MoveToLine(0)
		m.buf.ScrollTo(m.listHeight())
	case "G", "end":
		m.buf.MoveToLine(m.buf.LineCount() - 1)
		m.buf.ScrollTo(m.listHeight())
	}

	return m, nil
}

// move 는 커서를 화면 행 단위로 옮긴다. 접힌 긴 문단 안에서도 한 행씩 움직인다.
//
// **줄이 아니라 행이다.** 한 문단이 한 줄이라 줄 단위로 움직이면 `j` 한 번에 화면 절반이
// 지나간다. 편집 화면의 `j` 가 줄 단위인 것과 갈리는데, 저쪽은 고치는 자리를 잡는 손이고
// 이쪽은 읽어 내려가는 손이다.
func (m *viewHelp) move(delta int) {
	if delta > 0 {
		m.buf.MoveDownRow(delta)
	} else {
		m.buf.MoveUpRow(-delta)
	}

	m.buf.ScrollTo(m.listHeight())
}

// jump 는 찾은 다음 자리로 간다. `n` `N` 이 부른다. 찾는 것이 없으면 아무 일도 하지 않는다.
func (m *viewHelp) jump(direction textarea.SearchDirection) {
	if m.pattern == nil {
		return
	}

	found, ok := m.buf.Find(m.pattern, direction, m.buf.Cursor)
	if !ok {
		m.notify(fmt.Sprintf("%q 를 찾지 못했습니다", m.query))

		return
	}

	m.buf.Cursor = found.Cursor
	m.buf.UpdateDesiredCol()
	m.buf.ScrollTo(m.listHeight())
}

// matchesOn 은 그 줄에서 찾은 자리들이다. 편집 화면의 searchMatches 와 같은 일을 이 판의
// pattern 으로 한다.
func (m viewHelp) matchesOn(line []byte) [][]int {
	if m.pattern == nil {
		return nil
	}

	return m.pattern.FindAllIndex(line, -1)
}

func (m viewHelp) View() tea.View {
	return m.viewWith(m.notice, nil)
}

// viewWith 는 이 판을 그린다. bottom 은 statusBar 아래 줄이고, cursor 가 있으면 그 자리다.
//
// **찾는 글을 치는 화면이 같은 함수를 지난다**(view-help-search.go). tips 판과 같은 손이다.
func (m viewHelp) viewWith(bottom string, cursor *tea.Cursor) tea.View {
	height := m.listHeight()
	rows := m.buf.VisibleRows(height)

	// 문법 토큰을 화면 맨 아래 줄까지 채운다. 줄 하나를 훑으려면 그 앞 줄을 끝낸 문맥이
	// 필요해서 위에서부터 내려온다(editor.go 의 그리는 자리와 같다).
	if len(rows) > 0 {
		m.buf.LexSyntaxTo(rows[len(rows)-1].Line)
	}

	// 왼쪽 여백은 창이 잡은 gutter 폭 그대로다. **번호를 그리지 않고 빈 칸으로 둔다** —
	// 접기 계산이 이 폭을 이미 뺐으므로 여기를 비우면 폭이 어긋나고, 산문에 줄번호를
	// 붙이면 읽는 눈이 그쪽으로 끌린다.
	margin := strings.Repeat(" ", m.buf.GutterWidth())

	body := make([]string, 0, height)
	for _, row := range rows {
		line := m.buf.Line(row.Line)

		highlight := rowHighlight{
			matches:   m.matchesOn(line),
			cursorCol: -1,
			tokens:    m.buf.SyntaxTokens(row.Line),
		}

		body = append(body, margin+renderRow(line, row, m.buf.ContentWidth(), highlight, m.buf.TabWidth()))
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(m.renderHint()))
	screen = append(screen, m.renderBareStatusBar(bottom)...)

	view := newView(screen, m.renderWindowTitle())
	view.Cursor = cursor

	return view
}

// renderTitle 은 제목 띠다. 지금 보고 있는 자리를 백분율로 같이 보여준다.
//
// **몇 줄인지가 아니라 어디쯤인지다.** 도움말의 줄 번호는 아무 뜻이 없고, 긴 글에서 알고
// 싶은 것은 「끝이 얼마나 남았나」다.
func (m viewHelp) renderTitle() string {
	label := "도움말"
	if m.query != "" {
		label += fmt.Sprintf(" · %q 찾는 중", m.query)
	}

	label += fmt.Sprintf("  %d%%", m.progress())

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// progress 는 보고 있는 자리가 전체의 몇 퍼센트인지다. 마지막 줄이 100 이다.
func (m viewHelp) progress() int {
	last := m.buf.LineCount() - 1
	if last < 1 {
		return 100
	}

	return m.buf.Cursor.Line * 100 / last
}

func (m viewHelp) renderHint() string {
	find := "/ 찾기"
	if m.query != "" {
		find = "/ 찾기  n/N 다음·이전"
	}

	return " j/k 이동  ctrl+d/u 반쪽  g/G 처음·끝  " + find + "  q 닫기"
}

// renderBareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다(view-jobs.go 의 같은 이름).
func (m viewHelp) renderBareStatusBar(bottom string) []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	return bare.renderStatusBar("HELP", bottom)
}

// runHelp 는 팔레트의 「도움말」이다. `:help` 와 같은 길이다.
func runHelp(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return helpMode(e)
}
