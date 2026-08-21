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
// # 어두운 배경에 맞춘 값이다
//
// 처음에는 밝은 테마와 어두운 테마 양쪽에서 읽히도록 밝기를 가운데로 모았다. 그것이 틀렸다 —
// 어두운 배경에서 본문 색 전부가 3.4~4.7:1 한 띠에 뭉쳐서 **계층이 사라졌다.** 제목이 본문보다
// 앞으로 나오지 않고, keyword 와 문자열이 가장 어두워서 Go 의 `import` 문이 안 보였다.
//
// 이제 `#1e1e1e` 기준으로 전부 **6.8:1 이상**이다. 밝은 배경에서는 1.5~2.5:1 로 약하다 —
// 그쪽으로 옮기면 이 표를 갈아야 한다(ADR-0041).
//
// # 두 가지 규칙으로 값을 골랐다
//
// **주석이 가장 흐리다.** 6.8:1 로 이 표에서 제일 낮다. 일부러 그렇다 — 주석은 물러나 있어야
// 코드를 읽는 데 끼어들지 않는다. 회색으로 두지 않은 것은 본문 영역에서 회색이 이미 두 번
// 쓰여서다(colorWhitespace 240, styleLineNumberRelative 244).
//
// **같이 나오는 갈래끼리 색상이 25 도 이상 갈린다.** 언어별로 한 화면에 나오는 조합을 전부
// 재서 맞췄다. 주석과 문자열만 20 도인데 밝기가 6.8 대 9.5 로 갈려서 그대로 두었다 —
// 흐린 초록과 선명한 초록이다.
//
// **같이 나오지 않는 갈래는 색을 나눠 쓴다.** 강조는 keyword 와, 링크는 부르는 이름과 같은
// 색이다. markdown 에 keyword·부르는 이름이 없고 코드에 제목·강조·링크가 없어서 한 화면에서
// 만나지 않는다. 그래서 갈래 열하나가 색 아홉으로 된다 — 갈래를 늘리는 것이 팔레트를 반드시
// 늘리지는 않는다.
//
// 화면에서 이미 쓰이는 값(220 208 238 240 3 244 117 81 150 179 214) 은 피했다.
var styleSyntax = map[syntax.Kind]lipgloss.Style{
	// 흐린 초록. 이 표에서 가장 낮은 6.8:1 이다 — 물러나 있어야 한다.
	syntax.KindComment: lipgloss.NewStyle().Foreground(lipgloss.Color("108")), // #87af87

	syntax.KindString:   lipgloss.NewStyle().Foreground(lipgloss.Color("113")), // 초록 #87d75f
	syntax.KindNumber:   lipgloss.NewStyle().Foreground(lipgloss.Color("215")), // 주황 #ffaf5f
	syntax.KindKeyword:  lipgloss.NewStyle().Foreground(lipgloss.Color("177")), // 자주 #d787ff
	syntax.KindType:     lipgloss.NewStyle().Foreground(lipgloss.Color("185")), // 노랑 #d7d75f
	syntax.KindFunction: lipgloss.NewStyle().Foreground(lipgloss.Color("111")), // 연한 파랑 #87afff
	syntax.KindConstant: lipgloss.NewStyle().Foreground(lipgloss.Color("210")), // 연어 #ff8787
	syntax.KindVariable: lipgloss.NewStyle().Foreground(lipgloss.Color("79")),  // 청록 #5fd7af

	// 링크는 부르는 이름과 같은 색이다. 둘이 한 화면에 나오지 않는다(위 규칙).
	//
	// 하늘 계열을 쓰지 못한다 — 제목이 거기 있고 markdown 에서 늘 같이 나온다. 재보니 하늘 쪽
	// 후보는 제목과 색상차가 5~20 도라 섞였고, 이 값만 30 도를 넘으면서 밝기도 살았다.
	syntax.KindLink: lipgloss.NewStyle().Foreground(lipgloss.Color("111")), // 연한 파랑 #87afff

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
	// **제목이 이 표에서 유일하게 자기 색을 혼자 쓴다.** 문서에서 가장 구조를 이루는 것이라
	// 본문보다 앞으로 나와야 하고, 나눠 쓸 자리가 없었다.
	syntax.KindHeading: lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true), // 밝은 하늘 #00d7ff

	// 강조와 굵게는 keyword 와 같은 색이고, 속성으로 갈린다 — 쓴 사람이 고른 표시(`*` 하나냐
	// 둘이냐) 를 화면이 그대로 따라간다.
	syntax.KindEmphasis: lipgloss.NewStyle().Foreground(lipgloss.Color("177")).Italic(true),
	syntax.KindStrong:   lipgloss.NewStyle().Foreground(lipgloss.Color("177")).Bold(true),
}
