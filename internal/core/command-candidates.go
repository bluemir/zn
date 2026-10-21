package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// 명령줄의 후보 창에서 하나를 고르는 자리다(ADR-0158). 창을 그리는 것은
// render-command-complete.go 이고, 후보를 만드는 것은 complete-path.go 다.
//
// **insert 의 자동완성 창과 같은 손이다.** 창이 떠 있는 동안만 위·아래가 고르는 줄을 옮기고
// `tab`·`enter` 가 고른 것을 넣는다. 창이 없으면 그 키들은 원래 하던 일(이력 훑기·완성·실행)
// 을 한다(ADR-0066, ADR-0143).

// candidatesShown 은 후보 창이 화면에 떠 있는지다.
//
// 후보를 들고 있어도 화면이 낮아 한 줄도 못 넣으면 창은 뜨지 않는다. 그때 위·아래가 보이지
// 않는 줄을 옮기면 이력을 훑으려던 손이 아무 일도 안 한 것으로 보인다.
func (m viewEditorCommand) candidatesShown() bool {
	return m.candidateRows() > 0
}

// candidateKey 는 후보 창이 떠 있는 동안 창이 가로채는 키다. 가로챘으면 taken 이 참이다.
//
// `enter` 가 실행이 아니라 넣기인 것은 고른 줄을 넣어 보고 실행하기 위해서다. 넣자마자 실행하면
// 위·아래로 짚은 것이 무엇이었는지 명령줄에서 확인할 틈이 없다.
//
// `esc` 는 여기 없다. insert 처럼 창을 닫는 것과 mode 를 나가는 것을 한 키가 같이 한다.
func (m viewEditorCommand) candidateKey(key string) (viewEditorCommand, bool) {
	switch key {
	case "up":
		m.moveCandidate(-1)
	case "down":
		m.moveCandidate(1)
	case "tab", "enter":
		m.pickCandidate(m.selected)
	default:
		return m, false
	}

	return m, true
}

// complete 는 커서 앞의 마지막 조각을 채우고 남은 후보를 든다. `tab` 이 하는 일이다(ADR-0099).
//
// 고르는 줄은 늘 첫 후보에서 시작한다. 이전 목록의 자리를 이어받으면 다른 디렉터리의 몇째 줄이
// 짚여 있게 된다.
func (m *viewEditorCommand) complete() {
	filled, candidates := completeCommandLine(m.input.head())
	m.input.replaceHead(filled)
	m.candidates = candidates
	m.selected = 0
	m.candidateTop = 0
}

// pickCandidate 는 그 후보를 명령줄에 넣는다.
//
// **디렉터리면 그 안을 곧바로 다시 완성한다.** `tab` 을 한 번 더 친 것과 같다. 디렉터리를 고른
// 것은 더 들어가려는 것이라, 창이 닫히면 `tab` 을 또 쳐야 다음 목록이 보인다.
func (m *viewEditorCommand) pickCandidate(index int) {
	name := m.candidates[index]

	m.input.replaceHead(applyCandidate(m.input.head(), name))
	m.candidates = nil

	if strings.HasSuffix(name, "/") {
		m.complete()
	}
}

// moveCandidate 는 고른 줄을 옮긴다. 끝에서 더 가면 그 자리에 선다. insert 의 창과 같다.
//
// 창에 보이는 줄보다 후보가 많으면 고른 줄이 늘 창 안에 있도록 창을 민다.
func (m *viewEditorCommand) moveCandidate(delta int) {
	m.selected = min(max(m.selected+delta, 0), len(m.candidates)-1)

	rows := m.candidateRows()
	top := m.candidateStart()

	if m.selected < top {
		top = m.selected
	}
	if m.selected >= top+rows {
		top = m.selected - rows + 1
	}

	m.candidateTop = top
}

// scrollCandidates 는 휠로 창을 민다. 고른 줄은 창 밖으로 밀려나면 창 안으로 끌어온다.
//
// **고른 줄을 창 안에 둔다.** 보이지 않는 줄이 짚혀 있으면 `enter` 가 무엇을 넣을지 모른다.
func (m *viewEditorCommand) scrollCandidates(delta int) {
	rows := m.candidateRows()

	m.candidateTop = min(max(m.candidateStart()+delta, 0), len(m.candidates)-rows)
	m.selected = min(max(m.selected, m.candidateTop), m.candidateTop+rows-1)
}

// candidateStart 는 창의 첫 줄이 몇째 후보인지다.
//
// 들고 있는 값을 화면에 맞춰 다시 누른다. 창이 뜬 채로 화면이 낮아지면 보일 줄 수가 줄어서,
// 그대로 두면 창 아래쪽이 빈다.
func (m viewEditorCommand) candidateStart() int {
	return max(min(m.candidateTop, len(m.candidates)-m.candidateRows()), 0)
}

// candidateClick 은 왼쪽 클릭이다. 후보 줄을 누르면 그것을 넣는다.
//
// **창 밖의 클릭은 여전히 받지 않는다.** 치던 명령이 클릭 한 번에 조용히 사라지면 안 된다.
func (m viewEditorCommand) candidateClick(mouse tea.Mouse) viewEditorCommand {
	if mouse.Button != tea.MouseLeft {
		return m
	}

	if index, ok := m.candidateAt(mouse.X, mouse.Y); ok {
		m.pickCandidate(index)
	}

	return m
}

// candidateWheel 은 창 위의 휠이면 창을 민다. 창 밖이면 화면을 둘러보는 원래 휠이다.
//
// 한 번에 한 줄이다. 창은 여덟 줄이라 화면처럼 세 줄씩 밀면 짚던 이름이 금세 사라진다.
func (m viewEditorCommand) candidateWheel(mouse tea.Mouse) viewEditorCommand {
	if !m.overCandidateBox(mouse.X, mouse.Y) {
		m.wheel(mouse)

		return m
	}

	switch mouse.Button {
	case tea.MouseWheelUp:
		m.scrollCandidates(-1)
	case tea.MouseWheelDown:
		m.scrollCandidates(1)
	}

	return m
}

// candidateAt 은 그 화면 칸에 있는 후보가 몇째인지다. 후보 줄이 아니면 거짓이다.
//
// 테두리와 「몇/몇」 줄은 후보가 아니다.
func (m viewEditorCommand) candidateAt(x, y int) (int, bool) {
	if !m.overCandidateBox(x, y) {
		return 0, false
	}

	_, top := m.candidateBoxPos()

	row := y - top - 1
	if row < 0 || row >= m.candidateRows() {
		return 0, false
	}

	return m.candidateStart() + row, true
}

// overCandidateBox 는 그 화면 칸이 떠 있는 창 안인지다.
func (m viewEditorCommand) overCandidateBox(x, y int) bool {
	if !m.candidatesShown() {
		return false
	}

	left, top := m.candidateBoxPos()

	return x >= left && x < left+commandCompleteWidth && y >= top && y < top+m.candidateBoxHeight()
}
