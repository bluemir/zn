package core

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/lsp"

	"github.com/bluemir/zn/internal/textarea"
)

// 자동완성 목록을 편집 화면 위에 얹는 자리다(ADR-0066). 무엇을 띄울지는 completion.go 가 정한다.

// completionMaxWidth 는 목록 창의 최대 폭이다.
//
// 후보에 붙는 타입(`func(n int) int`) 이 길어서 그대로 두면 창이 화면을 가로지른다. 잘리는
// 것은 곁들이는 설명 쪽이고 이름은 그 앞에 온다 — 이름이 안 보이면 고를 수가 없다.
const completionMaxWidth = 48

// overlayCompletion 은 편집 화면 위에 목록 창을 얹는다. 떠 있지 않으면 화면을 그대로 준다.
//
// **커서는 건드리지 않는다.** 치던 자리에 그대로 서 있어야 목록이 뜬 채로도 글을 이어 친다 —
// 목록은 고르는 화면이 아니라 곁들여 뜬 것이라, mode 를 만들지 않은 것과 같은 뜻이다.
func (m viewEditorInsert) overlayCompletion(view tea.View) tea.View {
	if !m.completionOpen() {
		return view
	}

	buf := m.activeBuffer()

	cursor, ok := buf.CursorScreenPos(m.textHeight())
	if !ok {
		return view
	}

	box := m.renderCompletionBox()
	rows := strings.Split(box, "\n")

	left, top := popupPos(cursor.X+m.contentLeft(), cursor.Y+tablineHeight, completionBoxWidth, len(rows), m.width, m.height)

	// **받은 view 를 그대로 쓰고 내용만 갈아끼운다.** 새 view 를 만들면 터미널 설정을 손으로
	// 베껴야 하고, 베낄 목록이 늘 때 이 자리가 조용히 뒤처진다 — AltScreen·MouseMode 만 베끼던
	// 때에 ReportFocus 와 키 확장이 실제로 꺼졌다(ADR-0110).
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(box).X(left).Y(top).Z(1),
	).Render()

	return view
}

// completionBoxWidth 는 창의 폭이다. renderCompletionBox 가 이 값으로 짓는다.
//
// 고정값이다 — 후보에 따라 폭이 달라지면 글자를 칠 때마다 창이 늘었다 줄었다 한다.
const completionBoxWidth = completionMaxWidth + 2

// renderCompletionBox 는 목록 창이다.
func (m viewEditorInsert) renderCompletionBox() string {
	c := m.completion
	chars := m.boxChars
	inner := completionBoxWidth - 2

	line := strings.Repeat(chars.horizontal, inner)

	rows := []string{chars.topLeft + line + chars.topRight}

	end := min(c.top+completionRows, len(c.items))
	for i := c.top; i < end; i++ {
		text := padTo(truncateToWidth(completionLabel(c.items[i], inner), inner), inner)

		// 고른 줄은 반전이다. 팔레트·기호 서랍이 목록에서 쓰는 그 표시다(ADR-0011).
		if i == c.selected {
			text = reverse.Render(text)
		}

		rows = append(rows, chars.vertical+text+chars.vertical)
	}

	// 아래에 몇 개가 더 있는지 적는다. 창에 여덟 줄만 보이므로 그것이 전부인지 아닌지가
	// 보이지 않으면 화살표를 더 눌러 볼 까닭을 모른다.
	if len(c.items) > completionRows {
		rows = append(rows,
			chars.leftTee+padTo(truncateToWidth(completionCount(c), inner), inner)+chars.rightTee)
	}

	return strings.Join(append(rows, chars.bottomLeft+line+chars.bottomRight), "\n")
}

// completionLabel 은 후보 한 줄이다. 이름이 앞이고 곁들이는 타입이 뒤다.
//
// 타입은 자리가 남을 때만 붙인다. 이름이 잘리면 고를 수가 없으므로 잘리는 쪽은 늘 뒤다.
func completionLabel(item lsp.CompletionItem, width int) string {
	if item.Detail == "" {
		return item.Label
	}

	rest := width - textarea.WidthOf(item.Label) - 2
	if rest < 4 {
		return item.Label
	}

	return item.Label + "  " + truncateToWidth(item.Detail, rest)
}

// completionCount 는 창 아래에 적는 「지금 몇째/모두 몇」이다.
func completionCount(c completion) string {
	return fmt.Sprintf(" %d/%d", c.selected+1, len(c.items))
}
