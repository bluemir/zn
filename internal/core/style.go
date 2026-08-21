package core

import (
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/syntax"
)

// 화면에 쓰는 색이 전부 여기 있다.
//
// 갈래마다 그 기능 파일에 흩어져 있던 것을 모았다. 색을 고치려면 "그 색을 누가 쓰는가" 가
// 아니라 "화면에 어떤 색들이 있는가" 를 먼저 봐야 하는데, 흩어져 있으면 그것을 볼 자리가
// 없었다. 값이 겹치는지(240 과 244, 117 과 81) 도 한자리에 모여야 보인다.
//
// 그리는 코드는 여기 없다. 그래서 파일 이름에 `render-` 접두를 붙이지 않는다 — 그 접두는
// 그리는 코드만 든 파일의 것이다(ADR-0036). 이것은 그리는 코드가 읽는 표다.
//
// 색이 아닌 속성(`reverse` 의 반전, `styleMatch` 의 굵기) 도 여기 산다. 화면의 겉모습을
// 정하는 것이 한자리에 있어야 하고, 그래서 이름이 `color.go` 가 아니라 `style.go` 다.
//
// 여기 없는 것: `markerTab`·`markerSpace` 는 색이 아니라 글자라 `render-row.go` 에 남았고,
// `boxUnicode`·`boxASCII` 도 같은 이유로 `box.go` 에 남았다(ADR-0028).

// reverse 는 편집 내용과 구분되는 색이다. 색을 정하지 않고 터미널의 전경·배경을 뒤집기만 한다.
// 밝은 테마든 어두운 테마든 알아서 맞고 팔레트를 정할 필요가 없다(ADR-0004).
var reverse = lipgloss.NewStyle().Reverse(true)

// colorWhitespace 는 공백 마커의 색이다. 본문보다 흐려야 코드를 읽는 데 끼어들지 않는다.
// 검색 강조·sidebar·상대 줄번호와 같이 256색 고정값이다(ADR-0005, ADR-0007).
var colorWhitespace = lipgloss.Color("240")

// styleSearchMatch, styleSearchCurrent 는 찾은 자리의 색이다.
//
// 반전은 statusBar·tabline 이 이미 쓰고 있어서(ADR-0004) 본문에 쓰면 그 둘과 같은 모양이 된다.
// 그래서 여기만 256색 고정값을 쓴다. sidebar·상대 줄번호와 같은 방식이다(ADR-0005, ADR-0007).
//
// 커서가 선 매칭만 색이 다르다. `n` 으로 옮길 때 지금 어느 것 위에 있는지 보여야 한다.
var (
	styleSearchMatch   = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("16"))
	styleSearchCurrent = lipgloss.NewStyle().Background(lipgloss.Color("208")).Foreground(lipgloss.Color("16"))
)

// styleSelection 은 고른 범위의 색이다.
//
// 반전은 statusBar·tabline 이 이미 쓰고 있어서(ADR-0004) 본문에 쓰면 그 둘과 같은 모양이 된다.
// 검색 강조가 256색 고정값을 고른 것과 같은 자리다(ADR-0010).
//
// 글자색까지 고정하는 것도 검색과 같은 이유다 — 배경만 정하면 밝은 테마에서 읽히지 않는다.
var styleSelection = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("231"))

var (
	// styleLineNumberAbsolute 는 절대 줄번호 색이다. vim 의 `LineNr` 과 같은 노란색(ANSI 3)이라
	// 256 색 고정값과 달리 터미널 테마가 정한 노랑을 따른다(ADR-0007).
	styleLineNumberAbsolute = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	// styleLineNumberRelative 는 상대 줄번호 색이다. sidebar 의 흐린 색과 같은 값이다(ADR-0005).
	// 절대번호와 색이 달라, 둘이 나란히 있어도 어느 쪽이 무엇인지 색으로 갈린다.
	styleLineNumberRelative = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// 파일 종류별 글자색이다. 256 색 고정값이라 터미널 테마를 타지 않는다(ADR-0005).
//
// 굵기와 밑줄은 여기 없다. 그 둘은 "지금 보고 있는 파일" 한 뜻으로만 쓴다(ADR-0022).
// 디렉터리는 색과 `▸`/`▾` 표시와 `/` 접미로 이미 갈린다.
var (
	styleTreeDir     = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	styleTreeGo      = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	styleTreeDoc     = lipgloss.NewStyle().Foreground(lipgloss.Color("150"))
	styleTreeWeb     = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	styleTreeIgnored = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// styleMatch 는 fuzzy 매칭으로 맞은 글자다.
//
// 검색 강조(styleSearchMatch) 를 쓰지 않는다 — 그것은 "파일 안에서 찾은 것" 이라는 뜻이
// 이미 붙었다. 반전도 못 쓴다. statusBar·tabline(ADR-0004) 과 이 목록의 고른 행이 쓰고 있다.
var (
	styleMatch  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	styleDetail = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// styleSyntax 는 토큰 갈래별 색이다.
//
// **색은 core 가 정한다** — syntax 패키지는 갈래만 주고 색을 모른다(internal/syntax).
// 언어가 늘어도 이 표는 그대로다. 표에 없는 갈래는 색이 없다(render-row.go 의 appendSyntax).
//
// 배경은 정하지 않는다. 검색·선택이 배경을 쓰고 있어서(styleSearchMatch, styleSelection)
// 배경을 겹치면 위 층이 어디까지인지 보이지 않는다. 문법은 언제나 글자색뿐이다.
//
// 256색 고정값이다(ADR-0005, ADR-0007, ADR-0010). 화면에서 이미 쓰이는 값
// (220 208 238 240 3 244 117 81 150 179 214) 은 피했다.
//
// 대부분은 밝은 테마와 어두운 테마 양쪽에서 읽히도록 밝기를 가운데로 모았다. 제목만 예외로
// 어두운 배경에 맞춰 밝게 잡았다 — 아래 그 자리에 이유가 있다.
//
// 갈래가 늘어도 팔레트가 반드시 늘지는 않는다. emphasis 는 keyword 색을 나눠 쓴다.
//
// 주석이 회색이 아닌 이유: 본문 영역에서 회색은 이미 두 번 쓰인다(colorWhitespace 240,
// styleLineNumberRelative 244). 회색 주석은 "내용이 아님", 즉 줄번호 칸이 말하는 것과 같은
// 말이 된다.
var styleSyntax = map[syntax.Kind]lipgloss.Style{
	syntax.KindKeyword:  lipgloss.NewStyle().Foreground(lipgloss.Color("97")),  // 자주 #875faf
	syntax.KindString:   lipgloss.NewStyle().Foreground(lipgloss.Color("130")), // 주황빛 갈색 #af5f00
	syntax.KindComment:  lipgloss.NewStyle().Foreground(lipgloss.Color("65")),  // 흐린 초록 #5f875f
	syntax.KindNumber:   lipgloss.NewStyle().Foreground(lipgloss.Color("131")), // 벽돌 #af5f5f
	syntax.KindType:     lipgloss.NewStyle().Foreground(lipgloss.Color("30")),  // 청록 #008787
	syntax.KindFunction: lipgloss.NewStyle().Foreground(lipgloss.Color("67")),  // 강청 #5f87af
	syntax.KindConstant: lipgloss.NewStyle().Foreground(lipgloss.Color("168")), // 장미 #d75f87
	syntax.KindVariable: lipgloss.NewStyle().Foreground(lipgloss.Color("100")), // 올리브 #878700

	// 굵기·기울임은 색 위에 얹는다. **색 없이 속성만 쓰지 않는다** — 굵기를 흉내만 내고 밝은
	// 색으로 바꾸는 터미널이 있고 기울임을 아예 안 그리는 터미널도 있어서, 속성 하나에 뜻을
	// 걸면 그런 곳에서 표시가 사라진다. ADR-0022 가 트리에서 굵기에 밑줄을 갈아타지 않고
	// *더한* 것과 같은 판단이다.
	//
	// 밑줄은 쓰지 않는다. 같은 ADR 이 적어 둔 대로 lipgloss 가 글자마다 style 을 내서 본문에서는
	// escape 가 너무 길어진다.
	//
	// 굵기의 뜻이 트리와 본문에서 갈린다 — 트리에서는 "지금 보고 있는 파일"(ADR-0022) 이고
	// 여기서는 제목이다. 한 화면에 같이 있지만 트리와 본문은 칸이 갈려 있어 섞이지 않는다.
	//
	// **제목은 keyword 색을 나눠 쓰지 않는다.** 자주(97) 는 어두운 배경에서 3.6:1 이라 굵게
	// 해도 눈에 들어오지 않았다. 제목은 문서에서 가장 구조를 이루는 것이라 본문보다 앞으로
	// 나와야 하는데 그 색으로는 그것이 안 됐다. 밝은 하늘(45) 은 같은 자리에서 10:1 이다.
	//
	// 링크(33) 와 type(30) 도 푸른 쪽인데, 제목은 줄 전체가 굵어서 인라인인 그 둘과 섞이지
	// 않는다. 밝은 배경에서는 1.9:1 로 약하다 — 그때는 이 한 줄을 고친다.
	syntax.KindHeading: lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true), // 밝은 하늘 #00d7ff

	// emphasis 는 자주로 남는다. 인라인 장식이라 물러나 있어도 되고, 기울임이 이미 뜻을 나른다.
	syntax.KindEmphasis: lipgloss.NewStyle().Foreground(lipgloss.Color("97")).Italic(true),

	// 링크는 파랑이다. 흔히 링크에 쓰는 색이라 무엇인지 설명할 필요가 없다.
	// 부르는 이름(67, #5f87af) 과 type(30, #008787) 도 푸른 쪽인데, 이것이 훨씬 짙어서 갈린다.
	//
	// 밑줄을 얹지 않는다. 링크에 흔한 표시이긴 하지만 lipgloss 가 글자마다 style 을 내서
	// 본문에서는 escape 가 너무 길어진다(ADR-0022, ADR-0039).
	syntax.KindLink: lipgloss.NewStyle().Foreground(lipgloss.Color("33")), // 파랑 #0087ff
}
