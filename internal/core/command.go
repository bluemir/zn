package core

import (
	tea "charm.land/bubbletea/v2"
)

// normalCommand 는 완성된 normal mode 명령이다. **자기가 어떻게 실행되는지 안다.**
//
// 파서가 키에서 곧바로 이 type 을 만든다. 예전에는 파서가 `"d w"` 같은 이름으로 정규화하고
// 실행하는 쪽이 그것을 `CutPrefix` 로 다시 뜯었다 — 파싱이 세 겹이었고 둘 사이의 계약이
// 약속된 문자열이라 어긋나도 컴파일이 아니라 런타임에 조용한 무동작이 됐다(ADR-0034).
//
// **받는 것은 editor 이지 화면이 아니다.** 명령이 건드리는 것은 buffer·register·tab 이고
// 그것은 전부 editor 에 있다. 화면 model 을 받으면 mode 를 바꾸지 않는 명령까지 화면을 알게 된다.
//
// mode 를 바꾸는 명령(`i` `:` `ctrl+p`)은 새 model 을 돌려주고, 바꾸지 않는 명령은 nil 을
// 돌려준다 — 부르는 쪽이 지금 mode 를 그대로 쓴다.
type normalCommand interface {
	run(e *editor) (tea.Model, tea.Cmd)
}

// scrollToCursor 는 커서가 화면 안에 들어오도록 맞춘다. 편집·이동 명령이 끝에 이것을 한다.
func (e *editor) scrollToCursor() {
	e.activeBuffer().scrollTo(e.contentWidth(), e.textHeight())
}

// ── 이동 ──

// moveCommand 는 커서를 옮긴다. motion 이 어디로 갈지 안다.
type moveCommand struct {
	motion moveMotion
	count  int
}

func (c moveCommand) run(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	c.motion.move(buf, c.count, e.contentWidth())

	// normal mode 의 커서는 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
	// 왼쪽으로 가는 이동에는 걸릴 것이 없지만 나누어 둘 이유도 없다.
	buf.clampToNormal(e.contentWidth())
	e.scrollToCursor()

	return nil, nil
}

// ── operator ──

// deleteCommand 는 motion 이 잡은 범위를 지운다. `x` 는 motion 이 `l` 인 것이다.
type deleteCommand struct {
	motion motion
	count  int
}

func (c deleteCommand) run(e *editor) (tea.Model, tea.Cmd) {
	if deleted, ok := e.activeBuffer().deleteByMotion(c.motion, c.count, e.contentWidth()); ok {
		e.register = deleted
	}
	e.scrollToCursor()

	return nil, nil
}

// yankCommand 는 motion 이 잡은 범위를 register 에 담는다. 파일은 건드리지 않는다(ADR-0017).
type yankCommand struct {
	motion motion
	count  int
}

func (c yankCommand) run(e *editor) (tea.Model, tea.Cmd) {
	if yanked, ok := e.activeBuffer().yankByMotion(c.motion, c.count, e.contentWidth()); ok {
		e.register = yanked
	}
	e.scrollToCursor()

	return nil, nil
}

// changeCommand 는 지우고 insert mode 로 들어간다.
//
// 모르는 범위면 mode 도 바꾸지 않는다 — 손이 미끄러진 `c` 가 글자를 파일에 넣기 시작하면
// 무를 길이 없다(ADR-0033).
type changeCommand struct {
	motion motion
	count  int
}

func (c changeCommand) run(e *editor) (tea.Model, tea.Cmd) {
	removed, ok := e.activeBuffer().changeByMotion(c.motion, c.count, e.contentWidth())
	if !ok {
		return nil, nil
	}

	// 바꿀 것이 없었으면(빈 줄의 `cw`) register 는 그대로 둔다. vim 과 같다.
	if len(removed.lines) > 0 {
		e.register = removed
	}

	next, cmd := insertMode(e)
	e.scrollToCursor()

	return next, cmd
}

// replaceCharCommand 는 커서 자리 글자를 바꿔 넣는다. `r` 뒤의 한 키가 넣을 글자다.
//
// 글자가 아닌 키(`esc` 방향키 …) 는 아무 일도 하지 않아서 잘못 누른 `r` 을 무르는 길이 된다.
type replaceCharCommand struct {
	key   string
	count int
}

func (c replaceCharCommand) run(e *editor) (tea.Model, tea.Cmd) {
	buf, width := e.activeBuffer(), e.contentWidth()

	if c.key == "enter" {
		buf.replaceWithNewline(c.count, width)
	} else if text, ok := replacementText(c.key); ok {
		buf.replaceChar(text, c.count, width)
	}
	e.scrollToCursor()

	return nil, nil
}

// ── 붙여넣기와 되돌리기 ──

// pasteAfterCommand 는 `p` 다. 비어 있으면 아무 일도 하지 않는다.
type pasteAfterCommand struct{ count int }

func (c pasteAfterCommand) run(e *editor) (tea.Model, tea.Cmd) {
	e.activeBuffer().pasteAfter(e.register, max(c.count, 1), e.contentWidth())
	e.scrollToCursor()

	return nil, nil
}

// pasteBeforeCommand 는 `P` 다.
type pasteBeforeCommand struct{ count int }

func (c pasteBeforeCommand) run(e *editor) (tea.Model, tea.Cmd) {
	e.activeBuffer().pasteBefore(e.register, max(c.count, 1), e.contentWidth())
	e.scrollToCursor()

	return nil, nil
}

type undoCommand struct{}

func (undoCommand) run(e *editor) (tea.Model, tea.Cmd) {
	buf, width := e.activeBuffer(), e.contentWidth()

	buf.applyUndo(width)
	buf.clampToNormal(width)
	e.scrollToCursor()

	return nil, nil
}

type redoCommand struct{}

func (redoCommand) run(e *editor) (tea.Model, tea.Cmd) {
	buf, width := e.activeBuffer(), e.contentWidth()

	buf.applyRedo(width)
	buf.clampToNormal(width)
	e.scrollToCursor()

	return nil, nil
}

// ── insert mode 로 들어가는 것들 ──

// insertCommand 는 `i` 다. 커서 앞에 넣는다. 커서는 그대로다.
type insertCommand struct{}

func (insertCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return insertMode(e)
}

// appendCommand 는 `a` 다. 커서 글자 뒤에 넣는다.
// 줄 끝 다음 칸은 insert mode 에서만 갈 수 있어서 mode 를 먼저 바꾼다.
type appendCommand struct{}

func (appendCommand) run(e *editor) (tea.Model, tea.Cmd) {
	next, cmd := insertMode(e)

	e.activeBuffer().moveRight(1, e.contentWidth())
	e.scrollToCursor()

	return next, cmd
}

// openBelowCommand 는 `o` 다. 아래에 빈 줄을 만들고 그 줄에서 넣는다.
type openBelowCommand struct{}

func (openBelowCommand) run(e *editor) (tea.Model, tea.Cmd) {
	next, cmd := insertMode(e)

	e.activeBuffer().openLineBelow(e.contentWidth())
	e.scrollToCursor()

	return next, cmd
}

// openAboveCommand 는 `O` 다.
type openAboveCommand struct{}

func (openAboveCommand) run(e *editor) (tea.Model, tea.Cmd) {
	next, cmd := insertMode(e)

	e.activeBuffer().openLineAbove(e.contentWidth())
	e.scrollToCursor()

	return next, cmd
}

// ── 검색 ──

// searchCommand 는 `/` 와 `?` 다. 명령줄로 들어간다.
type searchCommand struct{ direction searchDirection }

func (c searchCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return searchMode(e, c.direction)
}

// nextMatchCommand 는 `n` 이다. 마지막 검색을 같은 방향으로 되풀이한다.
// `?` 로 찾았으면 `n` 도 위로 간다. 방향은 실행할 때 알 수 있어서 여기 담기지 않는다.
type nextMatchCommand struct{ count int }

func (c nextMatchCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return e.jumpToMatch(e.search.direction, max(c.count, 1))
}

// prevMatchCommand 는 `N` 이다. 마지막 검색을 거꾸로 되풀이한다.
type prevMatchCommand struct{ count int }

func (c prevMatchCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return e.jumpToMatch(e.search.direction.reverse(), max(c.count, 1))
}

// searchWordCommand 는 `*` 와 `#` 이다. 커서가 선 단어를 찾는다.
type searchWordCommand struct {
	direction searchDirection
	count     int
}

func (c searchWordCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return e.searchWord(c.direction, max(c.count, 1))
}

// ── mode 와 화면 ──

// quitCommand 는 `ctrl+c` 다. `:qa` 와 같은 경로라 저장하지 않은 변경이 있으면 확인창이 뜬다.
type quitCommand struct{}

func (quitCommand) run(e *editor) (tea.Model, tea.Cmd) {
	// 확인창에서 취소하면 돌아갈 화면이다. `ctrl+c` 가 완성된 시점이라 키 상태는 비어 있어서
	// 새로 만든 것과 지금 것이 같다.
	back, _ := normalMode(e)

	return quitAll(back, e)
}

// suspendCommand 는 `ctrl+z` 다. 종료가 아니라 멈춤이라 저장하지 않은 변경을 묻지 않는다(ADR-0023).
type suspendCommand struct{}

func (suspendCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return nil, tea.Suspend
}

// commandLineCommand 는 `:` 다.
type commandLineCommand struct{}

func (commandLineCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return commandMode(e)
}

// openPaletteCommand 는 `ctrl+p` 다.
type openPaletteCommand struct{}

func (openPaletteCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return paletteMode(e)
}

// focusTreeCommand 는 `ctrl+w ctrl+w` 와 `ctrl+w w` 다.
// pane 이 둘뿐이라 순환이 곧 왕래다. vim 과 같다.
type focusTreeCommand struct{}

func (focusTreeCommand) run(e *editor) (tea.Model, tea.Cmd) {
	return sidebarMode(e)
}

// ── tab ──

// nextTabCommand, prevTabCommand 는 `gt` 와 `gT` 다.
//
// 트리가 그 파일 자리를 따라간다. 아직 읽지 않은 디렉터리가 있으면 읽는 작업이 시작되므로
// 그 Cmd 를 들고 나간다(ADR-0032).
type nextTabCommand struct{}

func (nextTabCommand) run(e *editor) (tea.Model, tea.Cmd) {
	reveal := e.nextTab()

	// 옮겨 간 tab 은 이 크기의 화면을 처음 볼 수도 있다.
	e.scrollToCursor()

	return nil, reveal
}

type prevTabCommand struct{}

func (prevTabCommand) run(e *editor) (tea.Model, tea.Cmd) {
	reveal := e.prevTab()

	e.scrollToCursor()

	return nil, reveal
}
