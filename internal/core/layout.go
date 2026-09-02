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

// contentWidth 는 활성 창이 파일 내용을 그릴 너비다. 편집 영역에서 gutter 를 뗀 나머지다.
//
// **재는 것은 창이 한다**(viewport.contentWidth). 여기는 화면 밖에서 그 값이 필요한
// 자리(커서 좌표를 옮기고 sticky 머리줄을 그리는 곳) 를 위해 남겨 둔 문이다. tab 이 없으면
// 물어볼 창이 없어서 편집 영역을 통째로 쓴다(ADR-0064).
func (e editor) contentWidth() int {
	if !e.hasTab() {
		return e.textWidth()
	}

	return e.buffers[e.active].ContentWidth()
}

// contentLeft 는 파일 내용이 시작하는 화면 칸이다. 커서 좌표를 옮길 때 쓴다.
func (e editor) contentLeft() int {
	return e.sidebarLeft() + e.gutterWidth()
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

	// **재는 것은 창이 한다**(viewport.GutterWidth). 창은 자기가 받은 크기를 이미 알고 있다.
	return e.buffers[e.active].GutterWidth()
}

// layoutViews 는 지금 화면에서 창들이 받는 크기를 다시 배정한다.
//
// **크기가 바뀌는 자리마다 이것을 부른다.** 화면 크기(resize)·판 높이(setDrawerHeight)·
// sidebar 여닫기(toggleTree), 그리고 창을 새로 끼우는 자리(newTab·openTab·replaceTab)
// 여섯이다. 크기가 필드가 되면서 「누가 언제 그것을 넣는가」가 새로 생긴 짐인데, 그 짐을
// 여기 모았다 (ADR-0123).
//
// 일곱 번째는 이 길로 안 온다. 다시 읽기가 창을 새로 지어 갈아끼우므로 `adopt` 이 직접
// 이어받는다(viewport-reload.go).
//
// **활성 창만이 아니라 전부에 넣는다.** 이름 바꾸기와 여러 파일 치환이 보고 있지 않은 창을
// 직접 고치고(rename.go·replace.go), 그때 그 창도 자기 폭으로 줄을 다시 접어야 한다.
// 지금은 tab 이 화면을 나눠 쓰지 않아서 전부 같은 크기를 받는다.
//
// **줄 수는 여기서 안 본다.** 자릿수가 늘어 본문이 좁아지는 것은 창이 스스로 재므로
// (viewport.contentWidth) 편집할 때마다 다시 부를 일이 없다.
func (e *editor) layoutViews() {
	size := viewSize{Width: e.textWidth(), Height: e.textHeight()}

	for i := range e.buffers {
		e.buffers[i].Size = size
	}
}

// setDrawerHeight 는 아래 판이 가져가는 높이를 정한다. 0 이면 판이 닫힌 것이다.
//
// **대입 대신 이것을 쓴다.** 판이 열리고 닫히고 목록 길이가 바뀔 때마다 편집 영역 높이가
// 달라지는데, 그 자리가 열세 곳이라 하나만 빠뜨려도 창이 낡은 크기를 든 채로 남는다.
// 판들이 전부 `*editor` 를 embed 하므로 `m.setDrawerHeight(...)` 로 그대로 선다 (ADR-0123).
func (e *editor) setDrawerHeight(height int) {
	e.drawerHeight = height
	e.layoutViews()
}

// renderGutter 는 화면 행 앞에 붙는 칸이다. `마커 절대 상대 ` 순서다.
//
// wrap 되어 이어지는 행은 전부 빈 칸이다. 번호가 있는 행이 곧 논리 줄의 시작이라
// 화면에서 줄을 셀 때 헷갈리지 않는다. vim 과 같다. 마커도 같은 규칙이다 — 한 줄이 세 행이
// 되었을 때 마커가 세 번 서면 오류가 셋인 것처럼 보인다(ADR-0086).
func (e editor) renderGutter(buf *viewport, row screenRow) string {
	// **그리는 그 창에게 묻는다.** editor 를 거치면 활성 창의 값이 오는데, 그리는 것은 인자로
	// 받은 창이다. 지금은 같지만 화면 분할이 오면 갈린다(ADR-0123).
	width := buf.GutterWidth()
	if width == 0 {
		return ""
	}
	if row.Start != 0 {
		return strings.Repeat(" ", width)
	}

	absolute, relative := buf.LineNumberDigits()

	// 커서 줄은 0 이다. 절대번호가 바로 옆에 있어서 거기에 또 찍을 이유가 없다.
	distance := row.Line - buf.Cursor.Line
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
	return renderDiagnosticMarker(buf.DiagnosticAt(row.Line)) +
		renderGitMarker(buf.GitMarkAt(row.Line)) +
		styleLineNumberAbsolute.Render(fmt.Sprintf("%*d", absolute, row.Line+1)) + " " +
		styleLineNumberRelative.Render(relativeNumber) + " "
}
