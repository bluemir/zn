package core

import (
	"fmt"
	"strings"
)

// tablineHeight 는 편집 영역 위 tabline 이 차지하는 줄 수다(docs/spec.md).
const tablineHeight = 1

// statusBarHeight 는 화면 아래 statusBar 가 차지하는 줄 수다(docs/spec.md).
const statusBarHeight = 2

// textAndDrawerHeight 는 tabline 과 statusBar 를 뺀 높이다.
// 편집 영역과 하단 판(drawer) 이 이것을 나눠 쓴다(docs/spec.md).
//
// **전에는 `paneHeight` 였다.** 그 이름이 「판의 높이」로 읽히는데 실제로는 판을 아직 안고
// 있는 높이라, 판이 열리면 그만큼 어긋난다. 닫혀 있는 동안은 우연히 편집 영역과 같아서
// 그 어긋남이 안 보인다. 나눠 쓰는 둘을 이름에 다 적어 그 물음을 없앴다 (ADR-0123).
func (e editor) textAndDrawerHeight() int {
	return max(0, e.height-tablineHeight-statusBarHeight)
}

// textHeight 는 편집 내용을 그릴 수 있는 높이다.
// tabline 이 편집 영역 위를, statusBar 가 그 아래를 차지한다.
//
// drawer 는 편집 화면에 얹지 않고 이 높이를 가져간다. 얹으면 넣는 자리인 커서가 그 밑에
// 가려질 수 있고, 여기서 빼두면 scrollToCursor 가 그대로 커서를 살려 준다(ADR-0056).
func (e editor) textHeight() int {
	return max(0, e.textAndDrawerHeight()-e.drawerHeight)
}

// sidebarHeight 는 sidebar 가 차지하는 높이다.
//
// tabline 은 편집 영역 위에만 있으므로 sidebar 가 그 옆줄까지 올라간다.
// 아래로는 statusBar 앞에서 멈춘다 — statusBar 는 화면 끝까지 이어지는 한 줄이고
// 글자만 편집 영역 아래에서 시작한다(ADR-0005).
//
// drawer 는 여기서 빼지 않는다. 폭이 편집 영역과 같아서 sidebar 옆을 지나가지 않는다
// (docs/spec.md). 그래서 drawer 를 열어도 트리 높이는 그대로다.
//
// 트리 이동과 스크롤은 편집 영역이 아니라 이 높이를 기준으로 세야 맨 윗줄이 잘리지 않는다.
func (e editor) sidebarHeight() int {
	return tablineHeight + e.textAndDrawerHeight()
}

// sidebarVisible 은 sidebar 가 실제로 그려지는지다.
//
// sidebar.open 은 사용자의 의도일 뿐이라 그것만 보면 안 된다. 화면이 좁으면 켜져 있어도
// 그리지 않는다. 32 칸을 떼고 나면 편집할 자리가 없고, textWidth 가 음수가 되면
// 줄바꿈 계산과 빈 칸 채우기가 무너진다.
//
// 폭과 관련된 모든 곳이 open 이 아니라 이것 하나만 봐야 한다. 한 군데라도 어긋나면
// 화면 절반만 밀린 상태가 된다.
func (e editor) sidebarVisible() bool {
	return e.sidebar.open && e.width >= sidebarWidth+minTextWidth
}

// sidebarLeft 는 편집 영역이 시작하는 화면 칸이다. 커서 좌표를 옮길 때 쓴다.
func (e editor) sidebarLeft() int {
	if e.sidebarVisible() {
		return sidebarWidth
	}
	return 0
}

// textWidth 는 편집 영역 너비다. tabline 과 statusBar 의 글자가 이 너비 안에 든다.
// e.width 는 statusBar 처럼 화면 끝까지 칠하는 것과 sidebar 를 그릴지 말지를 정할 때만 쓴다.
func (e editor) textWidth() int {
	return max(0, e.width-e.sidebarLeft())
}

// contentWidth 는 파일 내용을 그릴 너비다. 줄을 어디서 접을지가 이 값으로 정해진다.
// 편집 영역에서 gutter 를 뗀 나머지다.
//
// 줄바꿈·스크롤·커서 계산은 모두 이 값을 써야 한다. textWidth 를 쓰면 gutter 만큼
// 넓게 잡아서 줄이 화면 오른쪽으로 삐져나간다.
func (e editor) contentWidth() int {
	return max(0, e.textWidth()-e.gutterWidth())
}

// contentLeft 는 파일 내용이 시작하는 화면 칸이다. 커서 좌표를 옮길 때 쓴다.
func (e editor) contentLeft() int {
	return e.sidebarLeft() + e.gutterWidth()
}

// 줄번호 칸의 최소 자릿수다. 파일이 짧아도 이만큼은 잡아서 줄을 오갈 때 본문이 흔들리지 않는다.
//
// minAbsoluteDigits 3 은 vim 의 `numberwidth` 기본값 4 와 같은 자리다. vim 은 뒤 공백까지
// 포함한 총 폭이고 여기서는 자릿수라 하나 작다. 그래서 vim 과 같이 999 줄까지는 안 흔들리고
// 1000 줄에서 한 칸 늘어난다(ADR-0007). 이 값을 올리면 그 지점이 vim 과 갈린다.
const (
	minAbsoluteDigits = 3
	minRelativeDigits = 2
)

// markerWidth 는 줄번호 왼쪽 마커 칸의 폭이다. 진단 마커와 git 마커가 각각 한 칸씩 선다
// (ADR-0086, ADR-0094).
//
// **아무것도 없어도 늘 잡는다.** 있을 때만 잡으면 첫 오류가 뜨는 순간 본문 전체가 한 칸
// 밀리고 줄바꿈 자리가 통째로 달라진다. minAbsoluteDigits 를 3 으로 잡아 999 줄까지
// 번호 칸이 흔들리지 않게 한 것과 같은 손이다(ADR-0007).
//
// Go 파일이 아니어도, 저장소가 아니어도 잡는다. 「표시가 오는 파일」과 「칸이 있는 파일」이
// 갈리면 tab 을 옮길 때마다 본문이 좌우로 흔들린다.
//
// **둘을 한 칸에 겹치지 않는다.** 오류가 있는 줄은 대개 방금 고친 줄이라, 한 칸을 나눠 쓰면
// 정작 보고 싶을 때 git 표시가 가려진다(ADR-0094 §3).
const markerWidth = 2

// lineNumberDigits 는 활성 창의 번호 자릿수다. 창이 없으면 최소값이다.
//
// **세는 것은 창이 한다**(viewport.lineNumberDigits). 줄 수를 아는 것이 그쪽이라, 여기는
// 「어느 창인가」와 「높이가 얼마인가」만 정해 넘긴다.
func (e editor) lineNumberDigits() (absolute, relative int) {
	if !e.hasTab() {
		return minAbsoluteDigits, max(digits(e.textHeight()), minRelativeDigits)
	}

	return e.buffers[e.active].lineNumberDigits(e.textHeight())
}

// gutterWidth 는 본문 앞에 붙는 칸이 차지하는 폭이다. 마커 칸과 줄번호 칸을 합친 것이고,
// 안 그릴 때는 0 이다.
//
// **폭이 한 항이다.** 마커 칸과 번호 칸을 따로 재면 좁은 화면에서 한쪽만 사라질 수 있고,
// 폭을 보는 자리(contentWidth·contentLeft·sticky·시험의 gutterWidthOf) 가 둘을 각각
// 더해야 한다. 한 군데라도 어긋나면 화면 절반만 밀린 상태가 된다(ADR-0086).
func (e editor) gutterWidth() int {
	// tab 이 없으면 번호를 붙일 줄이 없다. 빈 화면은 편집 영역을 통째로 쓴다(ADR-0064).
	if !e.hasTab() {
		return 0
	}

	// **재는 것은 창이 한다**(viewport.gutterWidth). 여기는 창에 준 칸만 알려 준다.
	return e.buffers[e.active].gutterWidth(e.textWidth(), e.textHeight())
}

// digits 는 십진수 자릿수다.
func digits(n int) int {
	count := 1
	for n >= 10 {
		n /= 10
		count++
	}

	return count
}

// renderGutter 는 화면 행 앞에 붙는 칸이다. `마커 절대 상대 ` 순서다.
//
// wrap 되어 이어지는 행은 전부 빈 칸이다. 번호가 있는 행이 곧 논리 줄의 시작이라
// 화면에서 줄을 셀 때 헷갈리지 않는다. vim 과 같다. 마커도 같은 규칙이다 — 한 줄이 세 행이
// 되었을 때 마커가 세 번 서면 오류가 셋인 것처럼 보인다(ADR-0086).
func (e editor) renderGutter(buf *viewport, row screenRow) string {
	width := e.gutterWidth()
	if width == 0 {
		return ""
	}
	if row.start != 0 {
		return strings.Repeat(" ", width)
	}

	absolute, relative := e.lineNumberDigits()

	// 커서 줄은 0 이다. 절대번호가 바로 옆에 있어서 거기에 또 찍을 이유가 없다.
	distance := row.line - buf.cursor.Line
	if distance < 0 {
		distance = -distance
	}

	// 상대번호 칸은 화면 높이까지만 잡는다(lineNumberDigits). 그리는 행이 모두 화면 안이라
	// 거리가 그 칸을 넘지 않는다는 약속 위에 서 있는데, sticky 머리줄은 화면 밖에서 오므로
	// 그 약속을 깬다(ADR-0049).
	//
	// `%*d` 의 폭은 **최소**라 넘치면 그대로 늘어난다. 한 칸이라도 넘치면 행이 편집 영역보다
	// 넓어져서 터미널이 접고, 그 아래가 통째로 밀리면서 sidebar 칸까지 어긋난다.
	//
	// 넘치면 비운다. `k` 로 세어 갈 수 있는 수가 아니라서 잘못 자른 숫자보다 없는 편이 낫다.
	relativeNumber := fmt.Sprintf("%*d", relative, distance)
	if len(relativeNumber) > relative {
		relativeNumber = strings.Repeat(" ", relative)
	}

	// 진단이 왼쪽 끝이고 git 이 번호 옆이다. 진단은 있다가 없어지는 것이고 git 표시는 commit
	// 할 때까지 그 줄에 남아 있어서, 본문에 가까운 쪽을 오래 서 있는 것에 준다(ADR-0094 §3).
	return renderDiagnosticMarker(buf.diagnosticAt(row.line)) +
		renderGitMarker(buf.git.marks[row.line]) +
		styleLineNumberAbsolute.Render(fmt.Sprintf("%*d", absolute, row.line+1)) + " " +
		styleLineNumberRelative.Render(relativeNumber) + " "
}
