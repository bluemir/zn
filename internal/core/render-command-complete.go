package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// 명령줄에서 `tab` 을 친 뒤 후보를 얹는 자리다(ADR-0099).
//
// **insert 의 자동완성 창과 같은 손이다.** 후보를 늘어놓는 그림이 이미 있고(render-completion.go)
// 자리를 잡는 `popupPos` 도 「아래에 못 넣으면 위로 뒤집고 statusBar 는 덮지 않는다」를 이미
// 한다. 명령줄은 화면 맨 아래라 창은 늘 위로 뒤집힌다.
//
// **고르는 화면이기도 하다.** 짚어 둔 줄을 반전으로 그리고, 위·아래·`tab`·`enter`·클릭으로
// 고른다(command-candidates.go, ADR-0158). 그래도 mode 를 만들지 않았다. 창이 떠 있는 동안
// 이어 치는 글자는 그대로 명령줄에 들어가야 해서다.

// commandCompleteRows 는 창에 보일 후보 줄 수의 상한이다. 자동완성 창과 같은 값이다.
const commandCompleteRows = completionRows

// commandCompleteFrame 은 후보 말고 창이 먹는 행이다. 테두리 둘과 「몇/몇」 한 줄이다.
const commandCompleteFrame = 3

// candidateRows 는 이 화면에서 실제로 보일 후보 줄 수다. 하나도 못 넣으면 0 이다.
//
// **상한만 보고 그리면 낮은 화면에서 창이 넘친다.** popupPos 는 자리를 잡아 줄 뿐 창이
// 화면보다 높을 때를 막지 않아서(top 을 0 으로 눌러도 아래로 넘친다) statusBar 와 명령줄이
// 창에 덮였다. 그리는 쪽에서 줄 수를 화면에 맞춘다.
//
// 쓸 수 있는 것은 tabline 아래부터 statusBar 위까지다. 그 둘은 덮지 않는다.
func (m viewEditorCommand) candidateRows() int {
	room := m.height - statusBarHeight - tablineHeight - commandCompleteFrame

	return max(min(commandCompleteRows, len(m.candidates), room), 0)
}

// commandCompleteWidth 는 창의 폭이다.
//
// 자동완성 창(48+2) 보다 넓다. 여기 오는 것은 경로 조각이 아니라 **파일 이름**이라 길고,
// 한 줄에 하나씩 서므로 폭이 곧 읽을 수 있는 이름 길이다.
const commandCompleteWidth = 40

// overlayCandidates 는 명령줄 위에 후보 창을 얹는다. 후보가 없으면 화면을 그대로 준다.
//
// **커서는 건드리지 않는다.** 명령줄 끝에 그대로 서 있어야 창이 뜬 채로 이어 친다.
func (m viewEditorCommand) overlayCandidates(view tea.View) tea.View {
	// 후보가 없거나 화면이 낮아 한 줄도 못 넣으면 얹지 않는다. 반쪽짜리 창이 명령줄을
	// 덮는 것보다 안 뜨는 것이 낫다.
	if !m.candidatesShown() {
		return view
	}

	left, top := m.candidateBoxPos()

	// 받은 view 를 그대로 쓰고 내용만 갈아끼운다. 새 view 를 만들면 터미널 설정을 손으로
	// 베껴야 한다(ADR-0110).
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderCandidateBox()).X(left).Y(top).Z(1),
	).Render()

	return view
}

// candidateBoxHeight 는 창이 먹는 행 수다. renderCandidateBox 가 짓는 줄 수와 같아야 한다.
//
// 그리는 자리와 클릭을 받는 자리가 같은 값으로 창을 놓아야 해서 따로 셌다. 창을 그려 보고
// 줄을 세면 클릭 한 번에 창을 통째로 짓게 된다.
func (m viewEditorCommand) candidateBoxHeight() int {
	rows := m.candidateRows() + 2
	if len(m.candidates) > m.candidateRows() {
		rows++
	}

	return rows
}

// candidateBoxPos 는 창의 왼쪽 위 칸이다.
//
// **기준은 명령줄이 아니라 statusBar 의 첫 줄이다.** popupPos 는 「기준 한 줄 위」에
// 창의 끝을 놓는데, 명령줄을 기준으로 주면 그 한 줄 위가 statusBar 윗줄이라 창이 그것을
// 덮는다. statusBar 두 줄을 통째로 비켜서야 한다.
//
// x 를 커서에 맞추지 않는 것은 명령줄 커서가 오른쪽으로 계속 밀려서 창이 화면 밖으로
// 새기 때문이다. 편집 영역 왼쪽 끝에 붙인다.
func (m viewEditorCommand) candidateBoxPos() (left, top int) {
	return popupPos(m.sidebarLeft(), m.height-statusBarHeight, commandCompleteWidth, m.candidateBoxHeight(), m.width, m.height)
}

// renderCandidateBox 는 후보 창이다.
func (m viewEditorCommand) renderCandidateBox() string {
	chars := m.boxChars
	inner := commandCompleteWidth - 2

	line := strings.Repeat(chars.horizontal, inner)
	rows := []string{chars.topLeft + line + chars.topRight}

	start := m.candidateStart()
	end := start + m.candidateRows()
	for i := start; i < end; i++ {
		// 이름이 길면 **왼쪽부터** 접는다. 확장자와 뒷부분이 남아야 무엇인지 갈린다 —
		// `GOTO` 목록의 경로와 같은 자리다(ADR-0069).
		text := padTo(trimLeftToWidth(m.candidates[i], inner), inner)

		// 고른 줄은 반전이다. insert 의 자동완성 창과 같은 표시다(ADR-0066).
		if i == m.selected {
			text = reverse.Render(text)
		}

		rows = append(rows, chars.vertical+text+chars.vertical)
	}

	// 창에 다 못 담으면 몇째인지와 모두 몇인지 적는다. 조용히 자르면 「이게 전부」로 읽힌다.
	if len(m.candidates) > m.candidateRows() {
		rows = append(rows,
			chars.leftTee+padTo(truncateToWidth(candidateCount(m.selected, len(m.candidates)), inner), inner)+chars.rightTee)
	}

	return strings.Join(append(rows, chars.bottomLeft+line+chars.bottomRight), "\n")
}

// candidateCount 는 창 아래에 적는 「지금 몇째/모두 몇」이다. insert 의 창과 같은 꼴이다.
func candidateCount(selected, total int) string {
	return fmt.Sprintf(" %d/%d", selected+1, total)
}
