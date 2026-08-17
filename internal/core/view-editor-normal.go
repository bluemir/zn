package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// viewEditorNormal 은 normal mode 다. 커서가 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
func normalMode(e *editor) (tea.Model, tea.Cmd) {
	return viewEditorNormal{editor: e}, nil
}

// normalModeMessage 는 명령 결과를 아래 줄에 띄운 채로 normal 로 돌아간다.
// 알림은 mode 밖(editor)에 있다 — 백그라운드 작업의 실패가 어느 mode 에서든 도착한다.
func normalModeMessage(e *editor, message string) (tea.Model, tea.Cmd) {
	e.message = message

	return viewEditorNormal{editor: e}, nil
}

type viewEditorNormal struct {
	*editor

	// state 는 키 나열을 명령 하나로 만드는 상태다. 숫자 접두와 `g` 같은 접두 키가 여기 산다.
	// mode 안에서만 사는 상태라 editor 가 아니라 여기에 둔다(ADR-0002).
	state normalState
}

// keyState 는 지금 키 상태다. zero value(nil) 는 아무것도 먹지 않은 처음이다.
func (m viewEditorNormal) keyState() normalState {
	if m.state == nil {
		return normalStart{}
	}

	return m.state
}

// Init 은 프로그램이 시작할 때 처음 model 에게만 불린다(bubbletea). mode 를 오가며 model 이
// 바뀌어도 다시 불리지 않으므로, git 갱신 고리를 거는 자리가 여기 하나다(ADR-0030).
//
// 첫 표시도 이 작업이 채운다. 그전까지 statusBar 오른쪽은 비어 있다 — 큰 저장소에서
// `git status` 를 기다리느라 편집기가 늦게 뜨는 것보다 낫다.
func (m viewEditorNormal) Init() tea.Cmd {
	// 트리의 첫 읽기도 여기서 시작한다. core.Run 은 Program 이 뜨기 전이라 Cmd 를 낼 자리가
	// 없어서, git 첫 갱신과 같이 이 자리가 낸다(ADR-0030, ADR-0032).
	return tea.Batch(m.refreshGit(), tickGit(), m.startTree())
}

func (m viewEditorNormal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.ResumeMsg, tea.FocusMsg:
		// 내려가 있는 동안의 commit·checkout 은 여기서 읽지 않는다. 주기 갱신이 5 초 안에
		// 따라온다 (ADR-0023, ADR-0030).
		//
		// 보고 있는 파일은 밖에서 바뀌었을 수 있다. 셸에서 올라오는 길과 다른 창에서 돌아오는
		// 길이 같은 자리다 — 둘 다 "바깥을 만지고 왔다" 는 뜻이다 (ADR-0023, ADR-0031).
		// 알리기만 하고 buffer 는 건드리지 않는다 — 가져오는 것은 `:e` 다.
		//
		// 알릴 것이 있을 때만 덮어쓴다. 창을 오갈 때마다 아래 줄이 비면 방금 친 명령의 결과가
		// 창을 한 번 바꿨다는 이유로 사라진다.
		if message := m.noteOutsideChange(); message != "" {
			m.message = message
		}

		return m, nil
	case tea.KeyPressMsg:
		// 파서가 글자를 기다리는 중이면 그 한 키는 명령이 아니라 파일에 들어갈 글자다.
		// 두벌식 자리로 되돌리면 `한` 이 `g` `k` `s` 세 키로 풀려서 한글을 넣을 수 없다.
		// 어느 상태가 그런지는 파서가 안다 — 여기서 상태 型 을 알아보지 않는다(ADR-0018).
		if m.keyState().literalNext() {
			return m.press(msg.String())
		}

		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		// 음절 하나가 키 여럿으로 풀리므로 차례로 먹인다.
		var model tea.Model = m
		for _, key := range keys {
			normal, ok := model.(viewEditorNormal)
			if !ok {
				// 앞의 키에서 mode 가 바뀌었다. 남은 키는 버린다 — 한글 상태로 잘못 들어온
				// 입력인데 남은 자모가 insert mode 로 흘러가 글자로 꽂히면 안 된다.
				return model, nil
			}

			next, cmd := normal.press(key)

			// cmd 를 내는 명령(종료, 확인창) 에서 멈춘다. 뒤에 올 키가 그 결과를 뒤집으면 안 된다.
			if cmd != nil {
				return next, cmd
			}

			model = next
		}

		return model, nil
	case tea.MouseClickMsg:
		// 왼쪽 버튼만 본다. 가운데·오른쪽에 붙일 동작은 아직 정하지 않았다.
		//
		// 누를 때 반응하고 뗄 때는 보지 않는다. 드래그가 없으니 누른 자리가 곧 고른 자리다.
		// 대기 중인 접두 키(m.state) 와 알림(m.message) 은 그대로 둔다 — 키 이야기다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			return m.click(mouse)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg:
		// 백그라운드 작업의 진행도 git 갱신 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go).
		return m, m.handleJob(msg)
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다. 명령이 완성되면 실행한다.
func (m viewEditorNormal) press(key string) (tea.Model, tea.Cmd) {
	// 알림은 다음 키를 누르면 사라진다.
	m.message = ""

	// sidebar 가 안 보이면 ctrl+w 를 없는 키로 친다. 접두 키는 다음 키를 삼키는데
	// (ctrl+c 까지) ctrl+w 는 셸에서 단어 지우기 근육기억이라, 갈 곳도 없는데
	// 키를 먹으면 안 된다.
	if key == "ctrl+w" && !m.sidebarVisible() {
		return m, nil
	}

	command, state := m.keyState().press(key)
	m.state = state

	// 아직 다음 키를 기다리는 중이다. 화면은 showcmd 만 바뀐다.
	if command.name == "" {
		return m, nil
	}

	return m.run(command)
}

// run 은 완성된 명령 하나를 실행한다.
//
// 짝이 없는 접두 키 조합(`g x`) 은 여기서 모르는 이름이 되어 아무 일도 하지 않는다. vim 과 같다.
func (m viewEditorNormal) run(key normalKey) (tea.Model, tea.Cmd) {
	buf := m.buffer()
	width := m.contentWidth()

	// tab 을 옮기면 트리가 그 파일 자리를 따라간다. 아직 읽지 않은 디렉터리가 있으면 읽는
	// 작업이 시작되므로 그 Cmd 를 아래 끝까지 들고 나간다(ADR-0032).
	var reveal tea.Cmd

	// count 를 받지 않는 명령은 파서가 0 을 준다.
	n := max(key.count, 1)

	// `d` 는 뒤에 붙은 motion 이 지울 범위를 정한다. 이름이 `d w` 처럼 둘로 되어 있어서
	// 아래 switch 의 평평한 이름으로는 받을 수 없다. 모르는 motion 은 아무 일도 하지 않는다.
	if motion, found := strings.CutPrefix(key.name, "d "); found {
		if deleted, ok := buf.deleteByMotion(motion, key.count, width); ok {
			m.register = deleted
		}

		buf.scrollTo(width, m.textHeight())

		return m, nil
	}

	// `r` 은 뒤에 바꿔 넣을 글자 한 개가 붙는다. 이것도 이름이 둘로 되어 있다.
	// 글자가 아닌 키(`esc` 방향키 ...) 는 아무 일도 하지 않아서 잘못 누른 `r` 을 무르는 길이 된다.
	if char, found := strings.CutPrefix(key.name, "r "); found {
		if char == "enter" {
			buf.replaceWithNewline(key.count, width)
		} else if text, ok := replacementText(char); ok {
			buf.replaceChar(text, key.count, width)
		}

		buf.scrollTo(width, m.textHeight())

		return m, nil
	}

	// `c` 는 지우고 insert mode 로 들어간다. 모르는 motion 이면 mode 도 바뀌지 않는다 —
	// 손이 미끄러진 `c` 가 글자를 파일에 넣기 시작하면 무를 길이 없다.
	if motion, found := strings.CutPrefix(key.name, "c "); found {
		removed, ok := buf.changeByMotion(motion, key.count, width)
		if !ok {
			return m, nil
		}

		// 바꿀 것이 없었으면(빈 줄의 `cw`) register 는 그대로 둔다. vim 과 같다.
		if len(removed.lines) > 0 {
			m.register = removed
		}

		next, cmd := insertMode(m.editor)
		buf.scrollTo(width, m.textHeight())

		return next, cmd
	}

	// `y` 도 같은 모양이다. 범위 계산은 `d` 와 같은 것을 쓰고 파일은 건드리지 않는다(ADR-0017).
	if motion, found := strings.CutPrefix(key.name, "y "); found {
		if yanked, ok := buf.yankByMotion(motion, key.count, width); ok {
			m.register = yanked
		}

		buf.scrollTo(width, m.textHeight())

		return m, nil
	}

	switch key.name {
	case "ctrl+c":
		// :qa 와 같은 경로다. 어느 tab 이든 저장하지 않은 변경이 있으면 확인창이 뜬다.
		return quitAll(m, m.editor)
	case "ctrl+z":
		// 셸로 내려간다. vim 과 같고 종료가 아니라 멈춤이라 저장하지 않은 변경을 묻지 않는다.
		// `fg` 로 올라오면 ResumeMsg 가 이 mode 로 돌아온다 (ADR-0023).
		return m, tea.Suspend
	case ":":
		return commandMode(m.editor)
	case "ctrl+p":
		return paletteMode(m.editor)
	case "/":
		return searchMode(m.editor, searchForward)
	case "?":
		return searchMode(m.editor, searchBackward)
	case "n":
		// 마지막 검색을 같은 방향으로 되풀이한다. `?` 로 찾았으면 `n` 도 위로 간다.
		return m.jumpToMatch(m.search.direction, n)
	case "N":
		return m.jumpToMatch(m.search.direction.reverse(), n)
	case "*":
		return m.searchWord(searchForward, n)
	case "#":
		return m.searchWord(searchBackward, n)
	case "i":
		// 커서 앞에 넣는다. 커서는 그대로다.
		return insertMode(m.editor)
	case "a":
		// 커서 글자 뒤에 넣는다.
		// 줄 끝 다음 칸은 insert mode 에서만 갈 수 있어서 mode 를 먼저 바꾼다.
		next, cmd := insertMode(m.editor)
		buf.moveRight(1, width)
		buf.scrollTo(width, m.textHeight())

		return next, cmd
	case "o":
		// 아래에 빈 줄을 만들고 그 줄에서 넣는다. `a` 와 같은 이유로 mode 를 먼저 바꾼다.
		next, cmd := insertMode(m.editor)
		buf.openLineBelow(width)
		buf.scrollTo(width, m.textHeight())

		return next, cmd
	case "O":
		// 위에 빈 줄을 만들고 그 줄에서 넣는다.
		next, cmd := insertMode(m.editor)
		buf.openLineAbove(width)
		buf.scrollTo(width, m.textHeight())

		return next, cmd
	case "x":
		// `dl` 과 같다. 줄 끝을 넘지 않으므로 다음 줄이 끌려 올라오지 않는다. vim 과 같다.
		if deleted, ok := buf.deleteByMotion("l", key.count, width); ok {
			m.register = deleted
		}
	case "p":
		// 지우거나 복사한 것을 커서 뒤에 붙인다. 비어 있으면 아무 일도 하지 않는다.
		buf.pasteAfter(m.register, n, width)
	case "P":
		buf.pasteBefore(m.register, n, width)
	case "u":
		buf.applyUndo(width)
		buf.clampToNormal(width)
	case "ctrl+r":
		buf.applyRedo(width)
		buf.clampToNormal(width)
	case "h":
		buf.moveLeft(n, width)
	case "l":
		buf.moveRight(n, width)
		buf.clampToNormal(width)
	case "k":
		buf.moveUpLine(n)
		buf.clampToNormal(width)
	case "j":
		buf.moveDownLine(n)
		buf.clampToNormal(width)
	case "w":
		buf.moveWordForward(n, smallWord, width)
		buf.clampToNormal(width)
	case "W":
		buf.moveWordForward(n, bigWord, width)
		buf.clampToNormal(width)
	case "e":
		buf.moveWordEnd(n, smallWord, width)
		buf.clampToNormal(width)
	case "E":
		buf.moveWordEnd(n, bigWord, width)
		buf.clampToNormal(width)
	case "b":
		buf.moveWordBackward(n, smallWord, width)
	case "B":
		buf.moveWordBackward(n, bigWord, width)
	case "0":
		buf.moveLineStart(width)
	case "^":
		buf.moveLineFirstNonBlank(width)
		buf.clampToNormal(width)
	case "$":
		buf.moveLineEnd(n, width)
		buf.clampToNormal(width)
	case "g g":
		// count 가 있으면 그 줄, 없으면 첫 줄이다.
		buf.moveToLine(n-1, width)
		buf.clampToNormal(width)
	case "G":
		// count 가 있으면 그 줄, 없으면 마지막 줄이다.
		line := len(buf.lines) - 1
		if key.count > 0 {
			line = key.count - 1
		}
		buf.moveToLine(line, width)
		buf.clampToNormal(width)
	case "up":
		buf.moveUp(1, width)
		buf.clampToNormal(width)
	case "down":
		buf.moveDown(1, width)
		buf.clampToNormal(width)
	case "left":
		buf.moveLeft(1, width)
	case "right":
		buf.moveRight(1, width)
		buf.clampToNormal(width)
	case "g t":
		reveal = m.nextTab()
	case "g T":
		reveal = m.prevTab()
	case "ctrl+w ctrl+w", "ctrl+w w":
		// pane 이 둘뿐이라 순환이 곧 왕래다. vim 의 ctrl+w ctrl+w / ctrl+w w 와 같다.
		return sidebarMode(m.editor)
	default:
		return m, nil
	}

	// tab 을 옮겼으면 buf 가 옛 buffer 를 가리키므로 다시 받는다.
	// 옮겨 간 tab 은 이 크기의 화면을 처음 볼 수도 있다.
	m.buffer().scrollTo(width, m.textHeight())

	return m, reveal
}

func (m viewEditorNormal) View() tea.View {
	// 커서가 글자 위에 있으므로 블록이다.
	return m.render(tea.CursorBlock, "NORMAL",
		m.withShowcmd(m.messageOr(m.position()), m.keyState().showcmd()))
}
