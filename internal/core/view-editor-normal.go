package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorNormal 은 normal mode 다. 커서가 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
func normalMode(e editor) (tea.Model, tea.Cmd) {
	return viewEditorNormal{editor: e}, nil
}

// normalModeMessage 는 명령 결과를 아래 줄에 띄운 채로 normal 로 돌아간다.
func normalModeMessage(e editor, message string) (tea.Model, tea.Cmd) {
	return viewEditorNormal{editor: e, message: message}, nil
}

type viewEditorNormal struct {
	editor

	// message 는 명령 결과나 오류다. 다음 키를 누르면 사라진다.
	message string

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

func (m viewEditorNormal) Init() tea.Cmd { return nil }

func (m viewEditorNormal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
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

	// count 를 받지 않는 명령은 파서가 0 을 준다.
	n := max(key.count, 1)

	switch key.name {
	case "ctrl+c":
		// :qa 와 같은 경로다. 어느 tab 이든 저장하지 않은 변경이 있으면 확인창이 뜬다.
		return quitAll(m, m.editor)
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
		m.nextTab()
	case "g T":
		m.prevTab()
	case "ctrl+w ctrl+w", "ctrl+w w":
		// pane 이 둘뿐이라 순환이 곧 왕래다. vim 의 ctrl+w ctrl+w / ctrl+w w 와 같다.
		return sidebarMode(m.editor)
	default:
		return m, nil
	}

	// tab 을 옮겼으면 buf 가 옛 buffer 를 가리키므로 다시 받는다.
	// 옮겨 간 tab 은 이 크기의 화면을 처음 볼 수도 있다.
	m.buffer().scrollTo(width, m.textHeight())

	return m, nil
}

func (m viewEditorNormal) View() tea.View {
	bottom := m.position()
	if m.message != "" {
		bottom = m.message
	}

	// 커서가 글자 위에 있으므로 블록이다.
	return m.render(tea.CursorBlock, "NORMAL", m.withShowcmd(bottom, m.keyState().showcmd()))
}
