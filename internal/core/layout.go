package core

import (
	"fmt"
	"strings"
)

// tablineHeight 는 편집 영역 위 tabline 이 차지하는 줄 수다(docs/spec.md).
const tablineHeight = 1

// statusBarHeight 는 화면 아래 statusBar 가 차지하는 줄 수다(docs/spec.md).
const statusBarHeight = 2

// textHeight 는 편집 내용을 그릴 수 있는 높이다.
// tabline 이 편집 영역 위를, statusBar 가 그 아래를 차지한다.
func (e editor) textHeight() int {
	return max(0, e.height-tablineHeight-statusBarHeight)
}

// sidebarHeight 는 sidebar 가 차지하는 높이다.
//
// tabline 은 편집 영역 위에만 있으므로 sidebar 가 그 옆줄까지 올라간다.
// 아래로는 statusBar 앞에서 멈춘다 — statusBar 는 화면 끝까지 이어지는 한 줄이고
// 글자만 편집 영역 아래에서 시작한다(ADR-0005).
//
// 트리 이동과 스크롤은 편집 영역이 아니라 이 높이를 기준으로 세야 맨 윗줄이 잘리지 않는다.
func (e editor) sidebarHeight() int {
	return tablineHeight + e.textHeight()
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
// 편집 영역에서 줄번호 칸을 뗀 나머지다.
//
// 줄바꿈·스크롤·커서 계산은 모두 이 값을 써야 한다. textWidth 를 쓰면 줄번호 칸만큼
// 넓게 잡아서 줄이 화면 오른쪽으로 삐져나간다.
func (e editor) contentWidth() int {
	return max(0, e.textWidth()-e.lineNumberWidth())
}

// contentLeft 는 파일 내용이 시작하는 화면 칸이다. 커서 좌표를 옮길 때 쓴다.
func (e editor) contentLeft() int {
	return e.sidebarLeft() + e.lineNumberWidth()
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

// lineNumberDigits 는 절대·상대 번호가 각각 쓰는 자릿수다.
//
// 절대번호는 전체 줄 수까지, 상대번호는 화면 높이까지만 커진다.
// 상대번호는 화면 밖으로 나가면 볼 수 없으므로 줄 수와 무관하다.
func (e editor) lineNumberDigits() (absolute, relative int) {
	return max(digits(len(e.buffers[e.active].lines)), minAbsoluteDigits),
		max(digits(e.textHeight()), minRelativeDigits)
}

// lineNumberWidth 는 줄번호 칸이 차지하는 폭이다. 안 그릴 때는 0 이다.
func (e editor) lineNumberWidth() int {
	absolute, relative := e.lineNumberDigits()

	// 번호 칸을 떼고 나면 본문이 남지 않는 좁은 화면에서는 그리지 않는다. sidebar 와 같은 규칙이다.
	width := absolute + 1 + relative + 1
	if e.textWidth()-width < minTextWidth {
		return 0
	}

	return width
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

// renderLineNumber 는 화면 행 앞에 붙는 줄번호 칸이다. `절대 상대 ` 순서다.
//
// wrap 되어 이어지는 행은 빈 칸이다. 번호가 있는 행이 곧 논리 줄의 시작이라
// 화면에서 줄을 셀 때 헷갈리지 않는다. vim 과 같다.
func (e editor) renderLineNumber(cursorLine int, row screenRow) string {
	width := e.lineNumberWidth()
	if width == 0 {
		return ""
	}
	if row.start != 0 {
		return strings.Repeat(" ", width)
	}

	absolute, relative := e.lineNumberDigits()

	// 커서 줄은 0 이다. 절대번호가 바로 옆에 있어서 거기에 또 찍을 이유가 없다.
	distance := row.line - cursorLine
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

	return styleLineNumberAbsolute.Render(fmt.Sprintf("%*d", absolute, row.line+1)) + " " +
		styleLineNumberRelative.Render(relativeNumber) + " "
}
