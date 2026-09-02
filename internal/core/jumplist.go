package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/scheme"
)

// 되돌아오기(jumplist) 다. `ctrl+o`(뒤로)·`ctrl+i`(앞으로) 가 이 이력을 오간다(ADR-0070).
//
// 필요는 정의로 가기에서 생겼다 — 다른 파일로 뛰는 첫 동작이었고, 그때까지 돌아오는 길은
// `gT` 와 tab 닫기뿐이었다(docs/tasks.md).

// jumpListMax 는 이력 최대 길이다. vim 과 같은 값이고, 넘치면 오래된 것부터 버린다.
const jumpListMax = 100

// jumpPlace 는 이력 한 자리다.
//
// **buffer 가 아니라 경로를 든다.** tab 을 닫아도 이력은 남아야 하고, 그때 되돌아가는 것은
// 그 파일을 다시 여는 일이다. 「정의로 뛰었다가 그 tab 을 닫고 돌아오기」가 그 길이다.
type jumpPlace struct {
	path string
	line int
	col  int
}

// sameLine 은 같은 파일의 같은 줄인지다. 칸은 보지 않는다 — 한 줄 안에서 옮겨 다닌 것을
// 저마다 담으면 `n` 을 몇 번 치는 사이에 이력이 그 줄로 채워진다.
func (p jumpPlace) sameLine(other jumpPlace) bool {
	return p.path == other.path && p.line == other.line
}

// jumpList 는 이력과 지금 자리다.
//
// **편집기 하나가 든다. tab 마다가 아니다.** 되돌아오는 것이 파일을 넘어야 뜻이 있어서다.
// vim 은 window 마다 드는데, zn 의 tab 은 window 보다 buffer 쪽에 가깝다(ADR-0070).
//
// **브라우저의 뒤로/앞으로와 같은 모양이다.** `at` 이 `len(places)` 면 이력 끝(가장 새 자리에
// 서 있는 것) 이고, 그보다 작으면 `ctrl+o` 로 되짚어 들어와 `places[at]` 에 서 있는 것이다.
// vim 은 앞쪽을 버리지 않고 지금 자리를 맨 뒤에 붙이는데, 그러면 두 키가 어디로 갈지
// 예측하기 어렵기로 유명하다. 여기서는 새로 뛰면 앞쪽을 버린다.
type jumpList struct {
	places []jumpPlace
	at     int
}

// here 는 지금 커서 자리다. 담을 자리가 아니면 false 다.
//
// 이름 없는 buffer(새 파일) 를 빼는 것은 되돌아갈 때 열 파일이 없어서다.
func (e *editor) here() (jumpPlace, bool) {
	if !e.hasTab() {
		return jumpPlace{}, false
	}

	buf := e.activeBuffer()
	if buf.Path == "" {
		return jumpPlace{}, false
	}

	return jumpPlace{path: buf.Path, line: buf.Cursor.Line, col: buf.Cursor.Col}, true
}

// jumpMotion 은 되돌아오기 이력에 담는 이동인지다.
//
// 자는 **한 번에 여러 줄을 건너뛰는가**이지 파일을 넘는가가 아니다 — vim 의 기준과 같다
// (`:help jump-motions`, ADR-0082). 그래서 `w`·`}` 같은 이동은 들지 않고, 화면을 굴리는
// `ctrl+d`·`ctrl+f` 도 들지 않는다(그쪽은 애초에 motion 이 아니다, ADR-0062).
func jumpMotion(mo moveMotion) bool {
	switch mo.(type) {
	case motionToLastLine, motionToFirstLine:
		return true
	}

	return false
}

// recordJump 는 지금 자리를 이력에 남긴다. **뛰기 직전에** 부른다.
//
// 담는 것은 「뛴 곳」이 아니라 「뛰기 전 자리」다. `ctrl+o` 가 데려다줄 곳이 그것이다.
//
// 부르는 자리는 넷이다 — 정의·사용처가 곧바로 뛸 때(language-server.go, references.go), `GOTO` 판에서
// 처음 뛸 때(view-locations.go), 검색이 옮길 때(view-editor-search.go 의 jumpToMatch).
// 되짚는 이동(goToPlace) 은 부르지 않는다.
//
// 파일 안에서 멀리 뛰는 것(`G`·`gg`·`:번호`) 은 이쪽이 아니라 recordJumpMove 로 간다.
func (e *editor) recordJump() {
	place, ok := e.here()
	if !ok {
		return
	}

	e.recordJumpFrom(place)
}

// recordJumpMove 는 **파일 안에서 멀리 뛴 것**을 담는다. `G`·`gg`·`:번호` 가 쓴다(ADR-0082).
// 다른 부르는 자리와 달리 **뛴 뒤에** 부르고, 떠난 자리를 받는다.
//
// 뒤에 부르는 것은 **아무 데도 가지 않았으면 담지 않기** 위해서다 — 파일 끝에서 `G` 를 또
// 치거나 이미 서 있는 줄에 `3G` 로 가는 것이 그런 자리인데, 그것을 담으면 `ctrl+o` 한 번이
// 제자리걸음이 된다. 파일을 넘는 뛰기는 커서가 반드시 움직여서 이 물음이 없었다.
//
// 닿은 자리는 방문 기록에도 남는다. 떠난 자리는 recordJumpFrom 이 남긴다(ADR-0074).
func (e *editor) recordJumpMove(from jumpPlace) {
	to, ok := e.here()
	if !ok || to.sameLine(from) {
		return
	}

	e.recordJumpFrom(from)
	e.recordVisit(to)
}

// recordJumpFrom 은 **적어 둔 자리**를 담는다. 둘러보는 판이 쓴다 — 확정할 때는 커서가 이미
// 둘러보던 곳에 가 있어서 지금 자리를 담으면 안 되고, 판을 열기 전 자리를 담아야 한다
// (ADR-0073 의 previewSession.origin).
func (e *editor) recordJumpFrom(place jumpPlace) {
	// 떠난 자리는 방문 기록에도 남는다. 「뛰기 직전」이 모이는 자리가 여기라 한 번만 건다
	// (jumplog.go, ADR-0074).
	e.recordVisit(place)

	// **앞쪽을 버린다.** 되짚어 들어와 있다가 새로 뛰면 `ctrl+i` 로 갈 자리가 사라진다.
	e.jumps.places = e.jumps.places[:e.jumps.at]

	// 같은 줄이 잇달아 오면 새로 담지 않는다. 자리만 이력 끝으로 옮긴다.
	if n := len(e.jumps.places); n > 0 && e.jumps.places[n-1].sameLine(place) {
		e.jumps.at = n

		return
	}

	e.jumps.places = append(e.jumps.places, place)
	if over := len(e.jumps.places) - jumpListMax; over > 0 {
		e.jumps.places = e.jumps.places[over:]
	}

	e.jumps.at = len(e.jumps.places)
}

// jumpBack 은 `ctrl+o` 다. 이력을 한 칸 뒤로 가고 그 자리로 옮긴다.
func (e *editor) jumpBack() tea.Cmd {
	if e.jumps.at == 0 {
		e.notify("되돌아갈 자리가 없습니다")

		return nil
	}

	// 이력 끝에 서 있으면 지금 자리를 먼저 담는다. 그래야 `ctrl+i` 로 돌아올 곳이 생긴다.
	// 담지 못하는 자리(이름 없는 buffer) 면 그냥 간다 — 못 담은 것이 되돌아가기를 막을 일은 아니다.
	if e.jumps.at == len(e.jumps.places) {
		if place, ok := e.here(); ok {
			e.jumps.places = append(e.jumps.places, place)
		}
	}

	e.jumps.at--

	cmd := e.goToPlace(e.jumps.places[e.jumps.at])
	e.arrive()

	return cmd
}

// jumpForward 는 `ctrl+i` 다. 터미널에서 그 키는 `tab` 과 같은 바이트(0x09) 로 와서
// 갈라 받을 수 없다 — 받는 이름이 `tab` 이고, 그래서 `tab` 키로도 앞으로 간다.
// normal mode 에 `tab` 이 비어 있어서 부딪히는 것은 없다. vim 도 이 둘이 같다.
func (e *editor) jumpForward() tea.Cmd {
	if e.jumps.at >= len(e.jumps.places)-1 {
		e.notify("앞으로 갈 자리가 없습니다")

		return nil
	}

	e.jumps.at++

	cmd := e.goToPlace(e.jumps.places[e.jumps.at])
	e.arrive()

	return cmd
}

// goToPlace 는 이력의 한 자리로 간다. 그 파일이 닫혀 있으면 다시 연다.
//
// **이 이동은 이력에 담지 않는다.** 되짚는 것이 새 jump 가 되면 되돌아갈 수 없다.
//
// 줄이 파일 밖을 가리키면 moveTo 가 안쪽으로 잡아 준다 — 담아 둔 뒤에 그 파일이 짧아진
// 경우이고, 그때는 엉뚱한 줄보다 파일 끝이 낫다(language-server.go 의 moveToLocation 과 같다).
func (e *editor) goToPlace(place jumpPlace) tea.Cmd {
	cmd, err := e.openTab(place.path)
	if err != nil {
		e.notifyError(err)

		return nil
	}

	buf := e.activeBuffer()
	buf.MoveTo(scheme.Cursor{Line: place.line, Col: place.col})
	buf.ClampToNormal()
	e.scrollToCursor()
	e.clearNotice()

	return cmd
}
