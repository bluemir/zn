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

// colorControl 은 제어문자를 보이게 한 `^[` 의 색이다(ADR-0118).
//
// **공백 마커와 반대로 눈에 띄어야 한다.** 마커는 늘 있는 것이라 물러나 있어야 하지만
// 제어문자는 드물게 나타나 「여기 글자가 아닌 것이 있다」를 알리는 자리다. 흐리게 두면
// `^[` 가 진짜 두 글자인 것과 구별되지 않는다.
//
// 분홍이다. 진단의 빨강(1)·노랑(11) 을 피했다 — 제어문자가 든 것은 오류가 아니라 사실이다.
// 문법 색 아홉과도 겹치지 않는다(styleSyntax 아래의 목록).
var colorControl = lipgloss.Color("205") // #ff5faf

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

// styleNoticeFailed 는 실패한 알림의 색이다(ADR-0053).
//
// 256 색 고정값이 아니라 ANSI 1 이다 — 절대 줄번호가 ANSI 3(노랑) 을 쓰는 것과 같은 자리다.
// 「빨강」은 터미널 테마마다 다르게 정해 두는 색이고, 여기서 필요한 것은 특정 빨강이 아니라
// **그 테마가 실패라고 부르는 색**이다(ADR-0007, ADR-0041).
//
// 색만으로 갈래를 나타내지는 않는다. 목록이 오류 줄에 `!` 도 같이 찍는다(view-messages.go).
var styleNoticeFailed = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

// styleDiagnosticError, styleDiagnosticWarning 은 진단 마커의 색이다(ADR-0086).
//
// 256 색 고정값이 아니라 ANSI 색이다. 실패한 알림이 ANSI 1 을 쓰는 것과 같은 자리다 —
// 여기서 필요한 것은 특정 빨강이 아니라 **그 테마가 실패라고 부르는 색**이다(styleNoticeFailed).
//
// 경고가 ANSI 3 이 아니라 11(밝은 노랑) 인 것은 바로 오른쪽 절대 줄번호가 3 을 쓰고 있어서다.
// 같은 값이면 `⚠ 12` 가 한 덩이로 붙어 보인다(styleLineNumberAbsolute, ADR-0007).
//
// 색만으로 갈래를 나타내지 않는다. 마커 글자가 `✖`·`⚠` 로 이미 갈리고, statusBar 아래 줄에도
// 같은 글자가 붙는다(diagnostics.go).
var (
	styleDiagnosticError   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleDiagnosticWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

// styleGitAdded, styleGitModified, styleGitRemoved 는 git 마커의 색이다(ADR-0094).
//
// 진단과 같이 ANSI 색이다. 그 테마가 부르는 초록·파랑·빨강이면 되고 특정 값이 필요하지 않다.
//
// **밝은 쪽(9~15) 을 쓴다.** 이 칸은 진단 칸(ANSI 1) 과 절대 줄번호(ANSI 3) 사이에 끼어
// 있어서, 어두운 쪽을 쓰면 옆 칸과 한 덩이로 보인다. 경고를 3 이 아니라 11 로 둔 것과 같은
// 자리다(styleDiagnosticWarning).
//
// 트리의 파일 마커도 이 색을 그대로 쓴다. 왼쪽 칸의 `~` 와 트리의 `M` 이 같은 것을 가리키고
// 있다는 것이 색으로 이어진다(sidebar.go).
var (
	styleGitAdded    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleGitModified = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	styleGitRemoved  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// diff 판의 바탕색이다(ADR-0140 §6).
//
// **이 판에서만 배경이 갈래를 든다.** 글자색을 문법 강조에 내주기로 했으므로 넣은 줄과
// 들어낸 줄이 설 자리가 배경뿐이다. ADR-0094 가 마커 칸에서 배경색을 기각한 근거 셋 중
// 둘이 여기서는 성립하지 않는다. 들어낸 줄이 실제로 그 자리에 그려지므로 배경이 칠한 그 줄
// 자신이 맞고, 이 판에는 배경을 이미 쓰고 있는 visual 선택도 검색 매칭도 없다.
//
// # 256 색 고정값이 아니라 참색이다
//
// 이 파일의 다른 값들과 갈린다. **256 색 큐브가 이 자리에 쓸 만큼 어둡지 않아서다** —
// 큐브의 색 단계가 `00 5f 87 af d7 ff` 뿐이라 초록은 검정 다음이 바로 `#005f00` 인데,
// 거기서는 가장 흐린 문법 색(주석 108) 의 대비가 3.2:1 로 떨어진다. 본문 문법 색이 지켜 온
// 바닥이 6.8:1 이다(ADR-0041).
//
// 참색이라고 테마를 타지는 않는다. 256 색 고정값을 쓴 까닭이 「터미널 테마가 이 색을 바꾸지
// 못하게」인데 그 성질은 그대로다. 참색을 못 내는 터미널에서는 lipgloss 가 가장 가까운
// 256 색으로 내려 준다.
//
// # 값은 대비가 골랐다
//
// `#1e1e1e` 위의 문법 색 여덟을 이 바탕 위에서 다시 쟀다. 바탕에서 가장 낮은 것이 넣은 줄
// 6.1:1, 들어낸 줄 6.5:1 이라 본문의 6.8:1 에 거의 닿는다. 글자 구간(strong) 은 4.7:1 과
// 5.1:1 로 내려가는데, **그 바탕이 덮는 것은 달라진 글자 몇 개뿐**이고 그 자리가 바로
// 눈이 가 있는 곳이다.
//
// 바탕끼리는 밝기가 아니라 색상으로 갈린다. `#1e1e1e` 와의 밝기 비가 1.04~1.10 이라 옅은데,
// 줄을 가로지르는 넓은 띠라 색상 차이로 읽힌다. 진하게 하면 그만큼 문법 색이 죽는다.
var (
	colorDiffAdded         = lipgloss.Color("#122b18") // 넣은 줄
	colorDiffAddedStrong   = lipgloss.Color("#1a4025") // 그중 달라진 글자
	colorDiffRemoved       = lipgloss.Color("#331a1e") // 들어낸 줄
	colorDiffRemovedStrong = lipgloss.Color("#57232c") // 그중 달라진 글자

	// colorDiffSame 은 문맥 줄이다. 바탕을 칠하지 않는 것과 같아 보이되, 줄 끝까지 칠해야
	// side-by-side 에서 두 열의 경계가 선다.
	colorDiffSame = lipgloss.Color("#1e1e1e")

	// colorDiffFiller 는 한쪽에 줄이 없는 자리다. 넣은 것도 들어낸 것도 아니라 색이 없고,
	// 「여기에는 아무것도 없다」만 말한다.
	colorDiffFiller = lipgloss.Color("#2a2a2a")

	// colorDiffNumber 는 줄번호 칸의 글자색이다. 상대 줄번호·sidebar 와 같은 244 다.
	colorDiffNumber = lipgloss.Color("244")
)

// styleDiffHeader 는 조각 머리줄(`@@ -47,2 +47,4 @@`) 의 색이다.
//
// git 이 청록으로 찍는다. 커밋 날짜와 같은 ANSI 14 다 — 그 테마가 부르는 청록이면 되고
// 특정 값이 필요하지 않다(styleCommitDate, ADR-0115 §6).
var styleDiffHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

// styleTip 은 짧은 안내 문장(tip) 의 색이다. statusBar 아래 줄 오른쪽과 빈 화면 가운데에 선다.
//
// **눈에 덜 띄어야 한다.** tip 은 지금 하는 일이 아니라 다음에 해 볼 것이라서, 커서 위치나
// 알림과 같은 밝기로 서면 읽던 것을 끊는다(ADR-0061).
//
// 상대 줄번호·sidebar 무시된 파일·팔레트 부가정보와 같은 244 다. 이 저장소가 「흐린 글씨」로
// 이미 정해 둔 값이라, 새 회색을 하나 더 만들지 않는다(ADR-0005, ADR-0007).
var styleTip = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

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
// 만나지 않는다. 그래서 갈래 열셋이 색 아홉으로 된다 — 갈래를 늘리는 것이 팔레트를 반드시
// 늘리지는 않는다.
//
// **키와 값 참조는 같이 나오면서도 같은 색이다.** 위 규칙의 예외이고 아래에 그 까닭을 적었다.
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

	// 키는 값을 가리키는 자리와 같은 청록이다. **둘은 한 화면에 나온다** — css 의
	// `--x: red` 와 `var(--x)` 가 그렇고 yaml 의 키와 anchor 도 그렇다. 그래도 색을 나누지
	// 않은 것은 자리가 이미 갈라 주기 때문이다: 키는 줄 앞에 서고 참조는 값 자리에 선다.
	// 나눠야 할 날이 오면 이 한 줄만 고친다(syntax.go 의 KindKey).
	syntax.KindKey: lipgloss.NewStyle().Foreground(lipgloss.Color("79")), // 청록 #5fd7af

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

	// 강조와 굵게와 취소선은 keyword 와 같은 색이고, 속성으로 갈린다 — 쓴 사람이 고른 표시
	// (`*` 하나냐 둘이냐, `~~` 냐) 를 화면이 그대로 따라간다.
	//
	// 취소선은 색을 흐리게 내린다. `~~` 로 그은 것은 「이제 아닌 것」이라 눈에서 물러나야
	// 하는데, 그은 줄만으로는 셋 중 어느 것인지 훑어보아 알기 어렵다.
	//
	// **이 갈래만 escape 가 네 배다.** lipgloss 가 취소선과 밑줄을 글자마다 감싼다
	// (`StrikethroughSpaces` 를 위해서다). 재보니 같은 줄이 굵기는 escape 6 개 98 byte,
	// 취소선은 46 개 418 byte 다. 손으로 `\x1b[9m` 을 붙이면 한 번이면 되는데 renderParts 에
	// 갈래가 하나 늘어서, 거슬리면 그때 본다(docs/tasks.md).
	syntax.KindEmphasis:      lipgloss.NewStyle().Foreground(lipgloss.Color("177")).Italic(true),
	syntax.KindStrong:        lipgloss.NewStyle().Foreground(lipgloss.Color("177")).Bold(true),
	syntax.KindStrikethrough: lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Strikethrough(true),
}

// styleCommitHash, styleCommitDate, styleCommitAge, styleCommitRefs 는 커밋 기록의 색이다(ADR-0115).
//
// **사용자의 `git graph` alias 가 쓰던 색을 그대로 옮겼다.** 해시가 파랑, 날짜가 청록,
// 얼마나 전인지가 초록, ref 이름이 노랑이다. 옆 터미널의 `git log` 와 같은 그림이라야
// 눈이 어느 칸을 볼지 다시 익히지 않는다.
//
// 256 색 고정값이 아니라 ANSI 밝은 색이다. git 마커·진단과 같은 자리다 — 여기서 필요한 것은
// 특정 파랑이 아니라 **그 테마가 부르는 파랑**이다(styleGitAdded, ADR-0007, ADR-0041).
//
// **그래프 선과 점에는 색을 두지 않는다.** 갈래를 가르는 것은 자리이지 색이 아니고,
// 색을 넣으면 그 색이 뜻하는 바를 또 익혀야 한다.
//
// 제목과 글쓴이도 여기 없다. 제목은 기본색 그대로이고 글쓴이는 흐린 회색(styleDetail) 이다 —
// 한 줄에서 가장 오래 읽는 것이 제목이라 그것만 색이 없는 편이 오히려 앞에 선다.
var (
	styleCommitHash = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleCommitDate = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	styleCommitAge  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleCommitRefs = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
)
