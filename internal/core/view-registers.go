package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// registersMinTextHeight 는 판을 열고도 편집 영역에 남아야 할 행 수다.
// 특수문자 판과 같은 값이다 — 판 하나가 편집 화면을 다 먹지 않는다는 규칙은 같아야 한다.
const registersMinTextHeight = symbolMinTextHeight

// registersFrame 은 판이 목록 말고 쓰는 행 수다. 테두리 둘이다.
// 특수문자 판과 달리 입력줄도 가름줄도 없다 — 치는 것이 없고 보여주는 것만 있다.
const registersFrame = 2

// registerNames 는 목록에 보일 순서다. `""` 는 무명, 그다음이 숫자, 그다음이 문자다.
//
// 무명이 맨 위다. 이름을 대지 않은 `p` 가 붙이는 것이 그것이라, 「지금 `p` 를 치면 무엇이
// 나오나」가 첫 줄이어야 한다. 숫자가 문자보다 앞인 것은 저절로 채워지는 자리라서다 —
// 무엇이 방금 지워졌는지가 손으로 모아 둔 것보다 급하다.
//
// 대문자는 없다. `"A` 는 `"a` 와 같은 자리이고 담는 법만 다르다(register.go 의 storeNamed).
var registerNames = registerNameOrder()

func registerNameOrder() []string {
	names := []string{""}
	for digit := range 10 {
		names = append(names, string(rune('0'+digit)))
	}
	for letter := 'a'; letter <= 'z'; letter++ {
		names = append(names, string(letter))
	}

	return names
}

// registersMode 는 `:registers` 로 여는 하단 drawer 다(ADR-0058).
//
// 특수문자 판(view-symbol.go) 과 같이 편집 영역의 행을 가져간다. 화면을 통째로 쓰는
// `:jobs`·`:messages` 와 다른 것은, 이 목록을 보는 까닭이 대개 「어느 것을 붙일까」라서
// 붙일 자리가 같이 보여야 하기 때문이다.
//
// **판을 그리는 코드는 특수문자 판과 나누지 않았다.** 공유하는 것은 편집 영역에서 몇 행을
// 가져갔는지(`editor.drawerHeight`) 와 그것을 뺀 높이(`textHeight`) 뿐이다. 안쪽이 완전히
// 달라서(저쪽은 입력줄과 격자, 이쪽은 목록 한 종류) 겹치는 것이 테두리 붙이는 몇 줄이고,
// 그것을 장치로 가르려면 안쪽 내용을 넘겨받는 자리가 생긴다. 세 번째 쓰임이 올 때 가른다(ADR-0058).
func registersMode(e *editor) (tea.Model, tea.Cmd) {
	// 붙일 자리가 없으면 열지 않는다. drawer 는 편집 맥락에 딸린 자리라, 볼 파일이 없는
	// 화면에 띄우면 그 자리 설명이 무너진다. 특수문자 판과 같다(ADR-0064).
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	m := viewRegisters{editor: e}

	e.setDrawerHeight(m.registersDrawerHeight())
	e.scrollToCursor()

	return m, nil
}

type viewRegisters struct {
	*editor

	top int // 목록의 첫 행
}

// registerRow 는 목록 한 줄에 필요한 것이다.
type registerRow struct {
	name string // 무명은 ""
	reg  register
}

// rows 는 담긴 것만 골라낸 목록이다. vim 의 `:registers` 와 같이 빈 것은 빼놓는다 —
// 열한 줄을 늘 그리면 채워진 둘을 찾는 데 눈이 걸린다.
func (m viewRegisters) rows() []registerRow {
	rows := make([]registerRow, 0, len(registerNames))
	for _, name := range registerNames {
		if reg := m.registers.byName(name); reg.Filled() {
			rows = append(rows, registerRow{name: name, reg: reg})
		}
	}

	return rows
}

// registersRows 는 목록에 쓸 수 있는 행 수다. 편집 영역에 남길 몫을 뺀 나머지다.
//
// 담긴 것이 열한 개라도 화면이 낮으면 다 못 그린다. 그때는 거절하지 않고 들어가는 만큼
// 그리고 `j`/`k` 로 훑는다 — 목록이 최대 열한 줄이라 훑을 일이 드물지만, 조용히 감추면
// 「그 register 는 비었나」로 읽힌다.
func (m viewRegisters) registersRows() int {
	room := m.textAndDrawerHeight() - registersFrame - registersMinTextHeight

	return min(max(len(m.rows()), 1), max(room, 1))
}

// registersDrawerHeight 는 판이 편집 영역에서 가져갈 행 수다.
func (m viewRegisters) registersDrawerHeight() int {
	return m.registersRows() + registersFrame
}

func (m viewRegisters) Init() tea.Cmd { return nil }

func (m viewRegisters) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		m.setDrawerHeight(m.registersDrawerHeight())
		m.scrollToCursor()
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseWheelMsg:
		// 판이 화면 아래에 붙어 있어서 포인터가 어디 있든 목록을 오르내린다.
		// 특수문자 판과 같은 규칙이다.
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
			// 다른 화면으로 넘어간다. **판 높이는 여기서 지우지 않는다** — 넘어가는 곳이
			// 또 판이면(정의 후보·사용처 목록) 그쪽이 방금 잡은 높이를 우리가 지우게 된다.
			// 판이 아닌 곳으로 가는 길은 normalMode 가 지운다(ADR-0069).
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다.
//
// 한글은 되돌린다. 입력줄이 없는 판이라 `ㅓ` 를 `j` 로 읽어야 한다(ADR-0008).
// 알림 목록(view-messages.go) 과 같은 이유이고, 받는 키가 전부 한 개짜리라 상태 기계는 없다.
func (m viewRegisters) press(key string) (tea.Model, tea.Cmd) {
	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewRegisters)
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
// 담긴 것을 지우거나 고치는 키는 없다. 읽는 판이다 — 무엇을 붙일지 보고 나가서 `"1p` 를 친다.
func (m viewRegisters) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return m.leave()
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
		m.move(pageRows(pageFull, m.registersRows()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.registersRows()))

		return m, nil
	case "g", "home":
		m.top = 0

		return m, nil
	case "G", "end":
		m.top = max(len(m.rows())-m.registersRows(), 0)

		return m, nil
	default:
		return m, nil
	}
}

// leave 는 판을 닫고 normal 로 돌아간다.
//
// 닫는 일 자체는 normalMode 가 한다(ADR-0069). 기호 판과 달리 여기서 할 것이 남지 않는다 —
// 판을 여는 동안 편집도 이동도 하지 않았다.
func (m viewRegisters) leave() (tea.Model, tea.Cmd) {
	return normalMode(m.editor)
}

// move 는 훑는 자리를 옮긴다. 양끝에서 멈춘다.
//
// 고른 자리를 두지 않는다. 읽는 판이라 고른 것으로 할 일이 없고, 그러면 `scrollTo` 가
// 「고른 것을 보이게」가 아니라 첫 행을 옮기는 것 하나가 된다.
func (m *viewRegisters) move(delta int) {
	m.top += delta
	m.scrollTo()
}

// scrollTo 는 첫 행을 목록 안으로 되돌린다.
func (m *viewRegisters) scrollTo() {
	m.top = min(max(m.top, 0), max(len(m.rows())-m.registersRows(), 0))
}

func (m viewRegisters) View() tea.View {
	// 커서는 편집 내용에 그대로 둔다. 붙일 자리가 보이는 편이 낫다.
	view := m.editorView(tea.CursorBlock, "REGISTERS",
		m.noticeOr("j/k 이동  q 닫기"))

	// 편집 화면을 다 그린 뒤 그 아래 빈 자리에 판을 얹는다. textHeight 가 이미 그만큼
	// 줄어 있어서 덮는 것이 없다(view-symbol.go).
	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	return view
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
//
// 테두리는 특수문자 판과 같이 손으로 붙인다. lipgloss 의 Border 는 안쪽 내용의 폭을 스스로
// 재는데 강조 escape 가 이미 섞여 있어서 그 계산을 믿을 수 없다.
func (m viewRegisters) renderDrawer() string {
	width := m.textWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{chars.topLeft + line + chars.topRight}
	rows = append(rows, m.renderListRows(inner)...)
	rows = append(rows, chars.bottomLeft+line+chars.bottomRight)

	return strings.Join(rows, "\n")
}

// 앞머리가 쓰는 폭이다. 이름은 `""` `"1` 둘 다 두 칸이고, 갈래는 `글자` 가 네 칸이라
// `줄` 이 두 칸을 채워 따라간다. 세로 줄이 맞아야 내용이 어디서 시작하는지 눈이 안 헤맨다.
const (
	registerNameWidth = 2
	registerKindWidth = 4
	registerMarkWidth = registerNameWidth + 1 + registerKindWidth + 1
)

// renderListRows 는 목록 행들이다. 담긴 것이 없으면 그 사실을 한 줄로 알린다.
func (m viewRegisters) renderListRows(inner int) []string {
	side := m.boxChars.vertical
	height := m.registersRows()

	all := m.rows()

	body := make([]string, 0, height)
	for at := m.top; at < len(all) && len(body) < height; at++ {
		body = append(body, m.renderRow(all[at], inner))
	}

	if len(all) == 0 && height > 0 {
		// padTo 는 채우기만 하고 자르지 않는다. 좁은 편집 영역에서는 이 문구가 상자보다
		// 넓어서 오른쪽 테두리를 밀어낸다 — 먼저 자른다(view-symbol.go).
		empty := padTo(truncateToWidth("담긴 register 가 없습니다", inner), inner)
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

// renderRow 는 register 한 줄이다.
//
//	""  줄   func main() {⏎…
//	"1  글자 foo bar
//
// **갈래를 색으로만 나타내지 않는다.** 줄 단위인지 글자 단위인지가 `p` 를 어디에 붙일지를
// 정하는 것이라(ADR-0017) 이 판에서 가장 중요한 값인데, 화면을 글자로 떠서 보는 길이
// 있어서(.claude/skills/run-zn) 색으로만 갈리면 그 길에서 사라진다(view-messages.go).
func (m viewRegisters) renderRow(row registerRow, inner int) string {
	kind := "글자"
	if row.reg.Linewise {
		kind = "줄"
	}

	// 강조를 입힌 뒤에는 폭을 잴 수 없다(lines.WidthOf 가 escape 까지 센다). 칸을 먼저
	// 채우고 그다음에 색을 입힌다 — 특수문자 판이 셀마다 하는 것과 같은 순서다.
	body := max(inner-registerMarkWidth, 0)
	text := padTo(trimTextRight(previewOf(row.reg), body), body)

	return padTo(registerLabel(row.name), registerNameWidth) + " " +
		styleDetail.Render(padTo(kind, registerKindWidth)) + " " + text
}

// registerLabel 은 이름을 화면에 적는 글자다. 무명은 vim 처럼 `""` 다.
func registerLabel(name string) string {
	if name == "" {
		return `""`
	}

	return `"` + name
}

// previewOf 는 register 내용을 한 줄로 편 것이다.
//
// 줄바꿈을 `⏎` 로 잇는다. 여러 줄이 담긴 것을 한 줄에 보이려면 어디가 줄 끝인지가 보여야
// 하고, 줄 단위 register 는 마지막 줄에도 그것이 붙는다 — 그것이 `p` 가 줄로 끼워 넣는
// 까닭이다. tab 은 그대로 둔다. 넓이는 trimTextRight 가 잰다.
func previewOf(reg register) string {
	text := strings.Join(linesToStrings(reg.Lines), "⏎")
	if reg.Linewise {
		text += "⏎"
	}

	return text
}

// linesToStrings 는 줄들을 문자열로 바꾼다.
func linesToStrings(lines [][]byte) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, string(line))
	}

	return out
}

// runRegisters 는 팔레트의 「register 목록」이다. `:registers` 와 같은 길이다.
func runRegisters(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return registersMode(e)
}
