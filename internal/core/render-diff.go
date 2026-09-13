package core

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/syntax"
	"github.com/bluemir/zn/internal/textarea"
)

// diff 판의 한 행을 그리는 자리다(ADR-0140 §6, §7).
//
// **글자색은 문법이 들고 배경색은 diff 가 든다.** 들어낸 줄도 문법 강조를 그대로 받는다.
// 이 판에서 읽는 것이 결국 코드라서, `+`·`-` 만 보려고 여는 것이 아니라 「무엇이 어떻게
// 되었나」를 읽으려고 연다. 그러면 갈래가 설 자리가 배경뿐이다.
//
// 본문의 renderRow 를 나눠 쓰지 않는다. 그쪽은 검색·선택이 **글자색과 배경을 함께** 정하는
// 층 구조라(render-row.go 의 segments) 바탕을 아래에 깔 자리가 없다. 여기서 필요한 것은
// 반대로 「바탕은 줄이 정하고 글자색은 토큰이 정한다」는 한 겹이다.

// diffPaint 는 한 줄을 어떤 바탕으로 그릴지다.
type diffPaint struct {
	// base 는 줄 전체의 바탕이다.
	base color.Color

	// strong 은 줄 안에서 달라진 글자의 바탕이다. 한 단계 진하다(ADR-0140 §7).
	strong color.Color
}

// on 은 그 바탕 위에 문법 색을 얹은 style 이다.
//
// **문법 style 에 배경을 더한다.** 반대로 바탕에 글자색만 옮겨 담으면 굵기와 기울임을
// 잃는다 — markdown 의 `**굵게**` 와 `*기울임*` 이 거기 걸려 있다(style.go 의 styleSyntax).
func (paint diffPaint) on(kind syntax.Kind, strong bool) lipgloss.Style {
	background := paint.base
	if strong {
		background = paint.strong
	}

	if painted, ok := styleSyntax[kind]; ok {
		return painted.Background(background)
	}

	return lipgloss.NewStyle().Background(background)
}

// renderDiffText 는 줄 하나를 width 칸에 꽉 채워 그린다.
//
// **줄 끝까지 칠한다**(ADR-0140 §6). 글자 있는 데서 끊으면 빈 줄을 넣거나 지운 것이 보이지
// 않고, side-by-side 에서는 두 열의 경계와 filler 자리가 사라진다.
//
// 넘치면 자른다. 좁은 화면에서 side-by-side 를 unified 로 되돌리지 않기로 했으므로
// (ADR-0140 §8) 자르는 자리가 여기다.
func renderDiffText(text []byte, tokens []syntax.Token, spans []diffSpan, paint diffPaint, tab, width int) string {
	if width < 1 {
		return ""
	}

	out := strings.Builder{}
	mark := markWhitespace(text)

	col, offset := 0, 0

	for _, cut := range diffTextCuts(len(text), tokens, spans) {
		if col >= width {
			break
		}

		kind, _ := diffTokenAt(tokens, offset)
		style := paint.on(kind, inDiffSpans(spans, offset))

		parts, next := expandRow(text, offset, cut, col, mark, tab)
		out.WriteString(renderParts(parts, style))
		col, offset = next, cut
	}

	line := out.String()

	// 넘쳤으면 자른다. truncateToWidth 가 색을 켠 채로 끊기지 않게 리셋을 붙인다.
	if col > width {
		line = truncateToWidth(line, width)
		col = textarea.WidthOf(line)
	}

	if col < width {
		line += lipgloss.NewStyle().Background(paint.base).Render(strings.Repeat(" ", width-col))
	}

	return line
}

// diffTextCuts 는 색이 갈리는 byte 자리들이다. 마지막은 줄 끝이다.
//
// 문법 토큰의 경계와 달라진 구간의 경계를 합친 것이다. 둘은 서로를 모르고 자유롭게 겹치므로
// (한 토큰 가운데서 글자가 바뀌는 일이 흔하다) 경계를 모아 놓고 잘라야 어긋나지 않는다.
//
// 줄 밖을 가리키는 자리는 버린다. lexer 가 어긋난 답을 줘도 그리는 쪽이 없는 byte 를 집지
// 않는다(render-row.go 의 appendSyntax 와 같은 자리다).
func diffTextCuts(size int, tokens []syntax.Token, spans []diffSpan) []int {
	cuts := make([]int, 0, len(tokens)*2+len(spans)*2+1)

	for _, token := range tokens {
		cuts = append(cuts, token.Start, token.End)
	}

	cuts = append(cuts, diffSpanBounds(spans)...)
	cuts = append(cuts, size)

	slices.Sort(cuts)

	kept := cuts[:0]
	for _, cut := range cuts {
		if cut <= 0 || cut > size {
			continue
		}

		if len(kept) > 0 && kept[len(kept)-1] == cut {
			continue
		}

		kept = append(kept, cut)
	}

	return kept
}

// diffTokenAt 은 그 byte 자리를 덮는 토큰의 갈래다. 덮는 것이 없으면 KindPlain 이다.
//
// 토큰은 앞에서부터 겹치지 않게 늘어서므로 자리를 지나친 순간 끝낸다.
func diffTokenAt(tokens []syntax.Token, offset int) (syntax.Kind, bool) {
	for _, token := range tokens {
		if offset < token.Start {
			return syntax.KindPlain, false
		}

		if offset < token.End {
			return token.Kind, true
		}
	}

	return syntax.KindPlain, false
}

// renderDiffNumber 는 줄번호 칸이다. 그쪽에 줄이 없으면 빈 칸이다.
//
// **바탕을 그대로 이어받는다.** 번호 칸만 색이 끊기면 줄을 가로지르는 띠가 두 동강 난다.
//
// 색은 상대 줄번호와 같은 흐린 회색이다. 이 저장소가 「물러나 있는 글씨」로 이미 정해 둔
// 값이라 새 회색을 하나 더 만들지 않는다(ADR-0005, ADR-0007).
func renderDiffNumber(line, digits int, background color.Color) string {
	style := lipgloss.NewStyle().Background(background)

	if line < 0 {
		return style.Render(strings.Repeat(" ", digits))
	}

	text := strconv.Itoa(line + 1)

	// 자릿수를 넘으면 칸을 늘리지 않고 뒷자리를 남긴다. 한 칸이라도 넘치면 그 아래 행이
	// 통째로 밀린다(layout.go 의 renderGutter 가 상대번호에서 겪는 자리와 같다).
	if len(text) > digits {
		text = text[len(text)-digits:]
	}

	return style.Foreground(colorDiffNumber).
		Render(strings.Repeat(" ", digits-len(text)) + text)
}

// diffDigits 는 줄번호 칸의 폭이다. 줄이 없으면 한 칸이다.
func diffDigits(lines int) int {
	return max(len(strconv.Itoa(lines)), 1)
}
