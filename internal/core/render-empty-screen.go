package core

import (
	"slices"
	"strings"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/buildinfo"
)

// renderEmptyScreen 은 tab 이 하나도 없을 때 편집 영역에 오는 행들이다(ADR-0064).
//
// **본문 자리만 만든다.** 정확히 textHeight 행이고 각 행은 textWidth 안이다. tabline·
// sidebar·statusBar 와 맞물리는 것은 renderScreen 이 한다 — 이름이 비슷해서 여기 적어 둔다.
//
// 가운데에 로고와 버전과 tip 이 선다. 빈 판에 문구를 적는 것은 `:jobs` 의
// 「도는 작업이 없습니다」 와 같은 자리다 — 아무것도 없으면 편집기가 덜 뜬 것처럼 보인다.
func (e editor) renderEmptyScreen() []string {
	width, height := e.textWidth(), e.textHeight()
	if height < 1 {
		return nil
	}

	block := e.emptyScreenBlock(width, height)

	rows := make([]string, 0, height)

	// 세로로도 가운데다. 홀수로 남는 한 줄은 아래에 둔다.
	for range max(0, (height-len(block))/2) {
		rows = append(rows, "")
	}

	for _, line := range block {
		// 뒤는 채우지 않는다. sidebar 는 왼쪽에 붙고 statusBar 는 따로라 오른쪽 빈 칸이
		// 필요한 곳이 없다 — 편집 화면이 채움 행을 빈 문자열로 두는 것과 같다(renderScreen).
		rows = append(rows, strings.Repeat(" ", max(0, (width-widthOf(line))/2))+line)
	}

	for len(rows) < height {
		rows = append(rows, "")
	}

	return rows
}

// emptyScreenBlock 은 가운데에 놓을 줄들이다. 로고, 한 줄 띄우고 버전, 한 줄 띄우고 tip 이다.
//
// **자르지 않고 버린다.** 자리에 안 들어가는 것은 통째로 뺀다 — 반쯤 잘린 로고는 로고로
// 읽히지 않고, 뒤가 끊긴 tip 은 무엇을 누르라는 것인지 알려주지 못하면서 자리는 다
// 차지한다(fitTip 과 같은 태도, ADR-0061).
//
// 버리는 순서는 tip, 로고, 버전이다. 마지막까지 남는 것이 버전 줄인 것은 그것이 편집기가
// 떠 있다는 것을 가장 짧게 말해 주기 때문이다.
func (e editor) emptyScreenBlock(width, height int) []string {
	block := []string{}

	// 버전은 자르면 무엇으로 지은 것인지 알 수 없어져서 안 들어가면 뺀다.
	// 버그 신고에 그대로 붙이는 줄이라 `:version` 과 `--version` 과 같은 것을 쓴다.
	if version := buildinfo.Describe(); widthOf(version) <= width {
		block = append(block, version)
	}

	// 로고는 버전 위에 한 줄 띄우고 붙는다. 남은 행이 그만큼 없으면 fitLogo 가 알아서 내린다.
	room := height
	if len(block) > 0 {
		room = height - len(block) - 1
	}

	if logo := e.fitLogo(width, room); len(logo) > 0 {
		if len(block) > 0 {
			block = slices.Concat(logo, []string{""}, block)
		} else {
			block = slices.Clone(logo)
		}
	}

	// tip 만 색이 붙는다. 흐린 글씨라 로고·버전보다 뒤로 물러난다(style.go 의 styleTip).
	// 가운데로 미는 계산이 lines.WidthOfStyled 인 것이 이 줄 때문이다.
	if tip := fitTip(assets.Tips, e.tipIndex, width); tip != "" && len(block)+2 <= height {
		block = append(block, "", styleTip.Render(tip))
	}

	if len(block) > height {
		return block[:height]
	}

	return block
}

// fitLogo 는 이 자리에 그릴 로고다. 원소 칸이 안 들어가면 한 줄 이름으로 내리고,
// 그것도 안 들어가면 아무것도 그리지 않는다.
//
// **자르지 않는다.** 반쯤 잘린 아스키 아트는 로고로 읽히지 않으면서 자리는 다 차지한다.
// 폭과 높이 둘 다 본다 — 낮은 화면에서 위아래가 잘린 원소 칸은 테두리만 남는다.
//
// 박스와 블록 문자를 쓸지는 테두리에 되묻는다. boxChars 를 고르는 조건이 「Ambiguous 를 한
// 칸으로 그린다고 확인됐다」 하나뿐이라(core.Run) 둘은 같은 갈림이고, 그래서 잰 값을 따로
// 들고 다니지 않는다(ADR-0028, ADR-0072).
func (e editor) fitLogo(width, height int) []string {
	logo := assets.LogoASCII
	if e.boxChars == boxUnicode {
		logo = assets.Logo
	}

	fits := len(logo) <= height
	for _, line := range logo {
		if widthOf(line) > width {
			fits = false
		}
	}

	switch {
	case fits:
		return logo
	case height >= 1 && width >= widthOf(smallLogo):
		return []string{smallLogo}
	}

	return nil
}

// smallLogo 는 원소 칸이 들어가지 않는 좁은 화면에서 그 자리를 대신하는 이름이다.
const smallLogo = "zn"
