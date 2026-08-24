package core

import (
	tea "charm.land/bubbletea/v2"
)

// action 은 완성된 normal mode 동작이다. **자기가 어떻게 실행되는지 안다.**
//
// vim 은 이것도 「Normal mode command」 라 부르지만, 우리는 `:` 로 치는 것을 명령이라 부르므로
// (`command`, command-parser.go) 이쪽은 동작이다.
//
// 파서가 키에서 곧바로 이 type 을 만든다. 예전에는 파서가 `"d w"` 같은 이름으로 정규화하고
// 실행하는 쪽이 그것을 `CutPrefix` 로 다시 뜯었다 — 파싱이 세 겹이었고 둘 사이의 계약이
// 약속된 문자열이라 어긋나도 컴파일이 아니라 런타임에 조용히 아무 일도 하지 않는 것이 됐다(ADR-0034).
//
// **받는 것은 editor 이지 화면이 아니다.** 동작이 건드리는 것은 buffer·register·tab 이고
// 그것은 전부 editor 에 있다. 화면 model 을 받으면 mode 를 바꾸지 않는 동작까지 화면을 알게 된다.
//
// mode 를 바꾸는 동작(`i` `:` `ctrl+p`)은 새 model 을 돌려주고, 바꾸지 않는 동작은 nil 을
// 돌려준다 — 부르는 쪽이 지금 mode 를 그대로 쓴다.
type action interface {
	run(e *editor) (tea.Model, tea.Cmd)
}

// ── 이동 ──

// actionMove 는 커서를 옮긴다. motion 이 어디로 갈지 안다.
type actionMove struct {
	motion moveMotion
	count  int
}

func (c actionMove) run(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	c.motion.move(buf, c.count, e.contentWidth())

	// normal mode 의 커서는 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
	// 왼쪽으로 가는 이동에는 걸릴 것이 없지만 나누어 둘 이유도 없다.
	buf.clampToNormal(e.contentWidth())
	e.scrollToCursor()

	return nil, nil
}

// actionPage 는 `ctrl+d`·`ctrl+u`(반 화면) 와 `ctrl+f`·`ctrl+b`(한 화면) 다. 화면과 커서를
// 같이 옮긴다.
//
// **motion 이 아니라 홀로 서는 동작이다.** motion 은 buffer 와 폭만 받아서(motion.go) 화면
// 높이를 모르는데 화면 단위 이동은 높이가 있어야 정해진다. operator 뒤에 올 수 없는 것도
// 그래서고, vim 에서도 `d ctrl+d` 는 지우지 않는다(ADR-0062, ADR-0063).
type actionPage struct {
	direction pageDirection
	span      pageSpan
	count     int
}

func (c actionPage) run(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	buf.movePage(c.direction, c.span, c.count, e.contentWidth(), e.textHeight())

	// 커서가 글자 위에 있어야 한다. actionMove 와 같은 자리다.
	buf.clampToNormal(e.contentWidth())

	// scrollToCursor 는 부르지 않는다. movePage 가 화면을 이미 옮겼는데 그것이 커서를 좇아
	// top 을 다시 최소한으로 당기면, 한 화면 굴린 것이 한 행 굴린 것이 된다.

	return nil, nil
}

// ── operator ──

// actionDelete 는 motion 이 잡은 범위를 지운다. `x` 는 motion 이 `l` 인 것이다.
type actionDelete struct {
	motion motion
	count  int
	reg    string // `"` 로 고른 register 이름. "" 면 무명과 숫자 링이다(ADR-0058)
}

func (c actionDelete) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	if deleted, ok := e.activeBuffer().deleteByMotion(c.motion, c.count, e.contentWidth()); ok {
		e.storeDelete(deleted, c.reg)
	}
	e.scrollToCursor()

	return nil, nil
}

// actionYank 는 motion 이 잡은 범위를 register 에 담는다. 파일은 건드리지 않는다(ADR-0017).
type actionYank struct {
	motion motion
	count  int
	reg    string
}

func (c actionYank) run(e *editor) (tea.Model, tea.Cmd) {
	if yanked, ok := e.activeBuffer().yankByMotion(c.motion, c.count, e.contentWidth()); ok {
		e.storeYank(yanked, c.reg)
		e.notify(yanked.copiedMessage())
	}
	e.scrollToCursor()

	return nil, nil
}

// actionChange 는 지우고 insert mode 로 들어간다.
//
// 모르는 범위면 mode 도 바꾸지 않는다 — 손이 미끄러진 `c` 가 글자를 파일에 넣기 시작하면
// 무를 길이 없다(ADR-0033).
type actionChange struct {
	motion motion
	count  int
	reg    string
}

func (c actionChange) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	removed, ok := e.activeBuffer().changeByMotion(c.motion, c.count, e.contentWidth())
	if !ok {
		return nil, nil
	}

	// 바꿀 것이 없었으면(빈 줄의 `cw`) register 는 그대로 둔다. vim 과 같다.
	// 숫자 링도 밀지 않는다 — 담기지 않은 것이 링을 흔들면 `"1` 이 뜻을 잃는다(ADR-0058).
	if len(removed.lines) > 0 {
		e.storeDelete(removed, c.reg)
	}

	next, cmd := insertMode(e)
	e.scrollToCursor()

	return next, cmd
}

// actionIndent 는 `>` 와 `<` 다. motion 이 잡은 범위를 한 단계 밀거나 당긴다.
//
// 범위가 글자 단위여도(`>w`) 걸친 줄 전체가 움직인다. 들여쓰기는 줄의 성질이라 반쪽을
// 밀 수 없다. vim 과 같다.
type actionIndent struct {
	motion    motion
	count     int
	direction indentDirection
}

func (c actionIndent) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf := e.activeBuffer()

	if area, ok := c.motion.span(*buf, c.count, e.contentWidth()); ok {
		buf.shiftLines(area.startLine, area.endLine, c.direction, e.contentWidth())
	}
	e.scrollToCursor()

	return nil, nil
}

// actionReindent 는 `=` 다. motion 이 잡은 범위를 언어 규칙이 정한 자리로 다시 들여쓴다.
type actionReindent struct {
	motion motion
	count  int
}

func (c actionReindent) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf := e.activeBuffer()

	if area, ok := c.motion.span(*buf, c.count, e.contentWidth()); ok {
		buf.reindentLines(area.startLine, area.endLine, e.contentWidth())
	}
	e.scrollToCursor()

	return nil, nil
}

// actionReplaceChar 는 커서 자리 글자를 바꿔 넣는다. `r` 뒤의 한 키가 넣을 글자다.
//
// 글자가 아닌 키(`esc` 방향키 …) 는 아무 일도 하지 않아서 잘못 누른 `r` 을 무르는 길이 된다.
type actionReplaceChar struct {
	key   string
	count int
}

func (c actionReplaceChar) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf, width := e.activeBuffer(), e.contentWidth()

	if c.key == "enter" {
		buf.replaceWithNewline(c.count, width)
	} else if text, ok := replacementText(c.key); ok {
		buf.replaceChar(text, c.count, width)
	}
	e.scrollToCursor()

	return nil, nil
}

// ── visual ──
//
// visual 의 동작은 operator 와 달리 뒤에 motion 을 기다리지 않는다. 고른 범위가 이미 있어서
// 그 자리에서 끝난다. 범위는 실행할 때 읽는다 — 한글로 온 키 하나가 「위로 → 지우기」처럼
// 동작 여럿이 될 수 있어서, 지을 때 담아 두면 앞선 이동을 놓친다(ADR-0008, ADR-0037).

// actionVisualStart 는 normal 의 `v` 와 `V` 다. 커서 자리를 anchor 로 삼아 범위를 연다.
type actionVisualStart struct{ linewise bool }

func (c actionVisualStart) run(e *editor) (tea.Model, tea.Cmd) {
	e.startSelection(c.linewise)

	return visualMode(e)
}

// actionVisualSwitch 는 visual 안에서의 `v` 와 `V` 다.
// 같은 키를 다시 치면 나가고, 다른 키면 갈래만 바꾼다. vim 과 같다.
type actionVisualSwitch struct{ linewise bool }

func (c actionVisualSwitch) run(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()
	if buf.selection.linewise == c.linewise {
		return normalMode(e)
	}

	buf.selection.linewise = c.linewise

	return visualMode(e)
}

// actionVisualLeave 는 `esc` 다. 고른 것을 버리고 normal 로 돌아간다.
type actionVisualLeave struct{}

func (actionVisualLeave) run(e *editor) (tea.Model, tea.Cmd) {
	return normalMode(e)
}

// actionVisualDelete 는 visual 의 `d` 와 `x` 다. `x` 가 같은 것은 지울 범위가 이미 정해져
// 있어서다 — normal 의 `x` 가 `dl` 인 것과 달리 여기서는 고른 것이 전부다.
type actionVisualDelete struct{ reg string }

func (c actionVisualDelete) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf := e.activeBuffer()

	if area, ok := buf.selectionRange(); ok {
		if deleted, cut := buf.deleteRange(area, e.contentWidth()); cut {
			e.storeDelete(deleted, c.reg)
		}
	}
	e.scrollToCursor()

	return normalMode(e)
}

// actionVisualYank 는 visual 의 `y` 다. 파일을 건드리지 않는다(ADR-0017).
type actionVisualYank struct{ reg string }

func (c actionVisualYank) run(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	if area, ok := buf.selectionRange(); ok {
		if yanked, copied := buf.yankRange(area, e.contentWidth()); copied {
			e.storeYank(yanked, c.reg)
			e.notify(yanked.copiedMessage())
		}
	}
	e.scrollToCursor()

	return normalMode(e)
}

// actionVisualChange 는 visual 의 `c` 다. 지우고 insert mode 로 들어간다.
//
// 줄 단위면 줄을 없애지 않고 첫 줄의 들여쓰기만 남긴다. `cc` 와 같은 자리다(ADR-0033).
type actionVisualChange struct{ reg string }

func (c actionVisualChange) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf := e.activeBuffer()

	area, ok := buf.selectionRange()
	if !ok {
		return normalMode(e)
	}

	// 지운 것과 이어 친 글자가 한 번의 `u` 로 함께 돌아간다. changeRange 가 구간을 열어 둔다.
	if removed, changed := buf.changeRange(area, e.contentWidth()); changed {
		if len(removed.lines) > 0 {
			e.storeDelete(removed, c.reg)
		}
	}

	next, cmd := insertMode(e)
	e.scrollToCursor()

	return next, cmd
}

// actionVisualIndent 는 visual 의 `>` 와 `<` 다.
type actionVisualIndent struct{ direction indentDirection }

func (c actionVisualIndent) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf := e.activeBuffer()

	if area, ok := buf.selectionRange(); ok {
		buf.shiftLines(area.startLine, area.endLine, c.direction, e.contentWidth())
	}
	e.scrollToCursor()

	return normalMode(e)
}

// actionVisualReindent 는 visual 의 `=` 다.
type actionVisualReindent struct{}

func (actionVisualReindent) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf := e.activeBuffer()

	if area, ok := buf.selectionRange(); ok {
		buf.reindentLines(area.startLine, area.endLine, e.contentWidth())
	}
	e.scrollToCursor()

	return normalMode(e)
}

// ── 붙여넣기와 되돌리기 ──

// actionPasteAfter 는 `p` 다. 비어 있으면 아무 일도 하지 않는다.
//
// reg 는 `"` 로 고른 register 이름이다. 비어 있으면 무명이다(ADR-0058).
type actionPasteAfter struct {
	count int
	reg   string
}

func (c actionPasteAfter) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	e.activeBuffer().pasteAfter(e.registerNamed(c.reg), max(c.count, 1), e.contentWidth())
	e.scrollToCursor()

	return nil, nil
}

// actionPasteBefore 는 `P` 다.
type actionPasteBefore struct {
	count int
	reg   string
}

func (c actionPasteBefore) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	e.activeBuffer().pasteBefore(e.registerNamed(c.reg), max(c.count, 1), e.contentWidth())
	e.scrollToCursor()

	return nil, nil
}

type actionUndo struct{}

func (actionUndo) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf, width := e.activeBuffer(), e.contentWidth()

	buf.applyUndo(width)
	buf.clampToNormal(width)
	e.scrollToCursor()

	return nil, nil
}

type actionRedo struct{}

func (actionRedo) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	buf, width := e.activeBuffer(), e.contentWidth()

	buf.applyRedo(width)
	buf.clampToNormal(width)
	e.scrollToCursor()

	return nil, nil
}

// ── insert mode 로 들어가는 것들 ──

// actionInsert 는 `i` 다. 커서 앞에 넣는다. 커서는 그대로다.
type actionInsert struct{}

func (actionInsert) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	return insertMode(e)
}

// actionAppend 는 `a` 다. 커서 글자 뒤에 넣는다.
// 줄 끝 다음 칸은 insert mode 에서만 갈 수 있어서 mode 를 먼저 바꾼다.
type actionAppend struct{}

func (actionAppend) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	next, cmd := insertMode(e)

	e.activeBuffer().moveRight(1, e.contentWidth())
	e.scrollToCursor()

	return next, cmd
}

// actionOpenBelow 는 `o` 다. 아래에 빈 줄을 만들고 그 줄에서 넣는다.
type actionOpenBelow struct{}

func (actionOpenBelow) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	next, cmd := insertMode(e)

	e.activeBuffer().openLineBelow(e.contentWidth())
	e.scrollToCursor()

	return next, cmd
}

// actionOpenAbove 는 `O` 다.
type actionOpenAbove struct{}

func (actionOpenAbove) run(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return nil, nil
	}

	next, cmd := insertMode(e)

	e.activeBuffer().openLineAbove(e.contentWidth())
	e.scrollToCursor()

	return next, cmd
}

// ── 검색 ──

// actionSearch 는 `/` 와 `?` 다. 명령줄로 들어간다.
type actionSearch struct{ direction searchDirection }

func (c actionSearch) run(e *editor) (tea.Model, tea.Cmd) {
	return searchMode(e, c.direction)
}

// actionNextMatch 는 `n` 이다. 마지막 검색을 같은 방향으로 되풀이한다.
// `?` 로 찾았으면 `n` 도 위로 간다. 방향은 실행할 때 알 수 있어서 여기 담기지 않는다.
type actionNextMatch struct{ count int }

func (c actionNextMatch) run(e *editor) (tea.Model, tea.Cmd) {
	e.jumpToMatch(e.search.direction, max(c.count, 1))

	return nil, nil
}

// actionPrevMatch 는 `N` 이다. 마지막 검색을 거꾸로 되풀이한다.
type actionPrevMatch struct{ count int }

func (c actionPrevMatch) run(e *editor) (tea.Model, tea.Cmd) {
	e.jumpToMatch(e.search.direction.reverse(), max(c.count, 1))

	return nil, nil
}

// actionSearchWord 는 `*` 와 `#` 이다. 커서가 선 단어를 찾는다.
type actionSearchWord struct {
	direction searchDirection
	count     int
}

func (c actionSearchWord) run(e *editor) (tea.Model, tea.Cmd) {
	e.searchWord(c.direction, max(c.count, 1))

	return nil, nil
}

// ── mode 와 화면 ──

// actionGotoDefinition 은 커서 자리의 정의로 간다. `\gd` 다(ADR-0051).
//
// 답을 기다리지 않는다 — 물어보는 Cmd 를 내고 돌아온다. 첫 요청은 서버가 모듈을 훑는 동안
// 1 초 남짓 걸려서, 기다리면 그동안 편집기가 멈춘다. 답이 오면 그때 tab 을 열고 뛴다
// (gopls.go 의 finishDefinition).
type actionGotoDefinition struct{}

func (c actionGotoDefinition) run(e *editor) (tea.Model, tea.Cmd) {
	back, _ := normalMode(e)

	return gotoDefinition(back, e)
}

// actionQuit 는 `ctrl+c` 다. `:qa` 와 같은 경로라 저장하지 않은 변경이 있으면 확인창이 뜬다.
type actionQuit struct{}

func (actionQuit) run(e *editor) (tea.Model, tea.Cmd) {
	// 확인창에서 취소하면 돌아갈 화면이다. `ctrl+c` 가 완성된 시점이라 키 상태는 비어 있어서
	// 새로 만든 것과 지금 것이 같다.
	back, _ := normalMode(e)

	return quitAll(back, e)
}

// actionSuspend 는 `ctrl+z` 다. 종료가 아니라 멈춤이라 저장하지 않은 변경을 묻지 않는다(ADR-0023).
type actionSuspend struct{}

func (actionSuspend) run(e *editor) (tea.Model, tea.Cmd) {
	return nil, tea.Suspend
}

// actionOpenCommandLine 는 `:` 다.
type actionOpenCommandLine struct{}

func (actionOpenCommandLine) run(e *editor) (tea.Model, tea.Cmd) {
	return commandMode(e)
}

// actionOpenPalette 는 `ctrl+p` 다.
type actionOpenPalette struct{}

func (actionOpenPalette) run(e *editor) (tea.Model, tea.Cmd) {
	return paletteMode(e)
}

// actionFocusTree 는 `ctrl+w ctrl+w` 와 `ctrl+w w` 다.
// pane 이 둘뿐이라 순환이 곧 왕래다. vim 과 같다.
type actionFocusTree struct{}

func (actionFocusTree) run(e *editor) (tea.Model, tea.Cmd) {
	return sidebarMode(e)
}

// ── tab ──

// actionNextTab, actionPrevTab 는 `gt` 와 `gT` 다.
//
// 트리가 그 파일 자리를 따라간다. 아직 읽지 않은 디렉터리가 있으면 읽는 작업이 시작되므로
// 그 Cmd 를 들고 나간다(ADR-0032).
type actionNextTab struct{}

func (actionNextTab) run(e *editor) (tea.Model, tea.Cmd) {
	reveal := e.nextTab()

	// 옮겨 간 tab 은 이 크기의 화면을 처음 볼 수도 있다.
	e.scrollToCursor()

	return nil, reveal
}

type actionPrevTab struct{}

func (actionPrevTab) run(e *editor) (tea.Model, tea.Cmd) {
	reveal := e.prevTab()

	e.scrollToCursor()

	return nil, reveal
}
