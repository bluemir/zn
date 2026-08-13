package core

import (
	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// viewEditorCommand 는 vim 의 command-line mode 다. normal 에서 `:` 로 들어간다.
//
// 치고 있는 명령 문자열은 이 mode 에만 있는 상태다.
// mode 를 model 로 나눈 덕에 다른 mode 가 이 필드를 이고 다니지 않는다(ADR-0002).
func commandMode(e editor) (tea.Model, tea.Cmd) {
	return viewEditorCommand{editor: e}, nil
}

type viewEditorCommand struct {
	editor

	input string // `:` 뒤에 친 것
}

func (m viewEditorCommand) Init() tea.Cmd { return nil }

func (m viewEditorCommand) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return normalMode(m.editor)
		case "enter":
			return m.run()
		case "backspace":
			// vim 처럼 `:` 까지 지우면 명령줄에서 나간다.
			if m.input == "" {
				return normalMode(m.editor)
			}
			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]

			return m, nil
		default:
			if msg.Text == "" {
				return m, nil
			}
			m.input += msg.Text

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 명령을 치는 동안에도 화면은 둘러볼 수 있다.
		// 클릭은 받지 않는다 — 치던 명령이 클릭 한 번에 조용히 사라지면 안 된다.
		m.wheel(msg.Mouse())

		return m, nil
	default:
		return m, nil
	}
}

// run 은 친 명령을 실행한다.
func (m viewEditorCommand) run() (tea.Model, tea.Cmd) {
	buf := m.buffer()

	cmd, err := parseCommand(m.input)
	if err != nil {
		return m.fail(err)
	}

	// 인자를 받는 명령은 `:e` 와 `:tabnew` 뿐이다. 나머지에 붙은 인자를 조용히 버리면
	// `:w foo` 가 foo 에 저장한 것처럼 보인다.
	if len(cmd.args) > 0 && cmd.name != "e" && cmd.name != "tabnew" {
		return normalModeMessage(m.editor, "알 수 없는 명령: "+m.input)
	}
	// 파일 이름 하나만 받는다. 여럿을 tab 여러 개로 여는 것은 CLI 인자의 몫이다.
	if len(cmd.args) > 1 {
		return normalModeMessage(m.editor, "파일은 하나만 쓸 수 있습니다")
	}

	switch cmd.name {
	case "":
		return normalMode(m.editor)
	case "w":
		// `!` 는 읽은 뒤 밖에서 바뀐 파일도 덮어쓴다는 뜻이다 (ADR-0015).
		var err error
		if cmd.force {
			err = buf.SaveForce()
		} else {
			err = buf.Save()
		}
		if err != nil {
			return m.fail(err)
		}
		// 저장하면 저장소가 dirty 가 된다. statusBar 의 git 표시를 여기서 맞춘다(ADR-0009).
		m.git = readGitStatus()

		return normalModeMessage(m.editor, "저장함: "+buf.path)
	case "wq", "x":
		var err error
		if cmd.force {
			err = buf.SaveForce()
		} else {
			err = buf.Save()
		}
		if err != nil {
			return m.fail(err)
		}
		m.git = readGitStatus()

		return forceCloseTab(m.editor)
	case "e":
		return m.edit(cmd)
	case "tabnew":
		// 인자가 없으면 이름 없는 빈 tab 이다.
		if len(cmd.args) == 0 {
			m.newTab()

			return normalMode(m.editor)
		}

		// 이미 열려 있으면 새 tab 을 만들지 않고 그 tab 으로 옮겨간다(ADR-0021).
		if err := m.openTab(cmd.args[0]); err != nil {
			return m.fail(err)
		}

		return normalMode(m.editor)
	case "noh", "nohlsearch":
		// 강조만 끈다. 마지막 검색은 남아서 `n` 이 계속 먹는다. vim 과 같다.
		m.search.highlight = false

		return normalMode(m.editor)
	case "tree":
		// `!` 는 이 명령에서 뜻이 없다. 그냥 여닫는다.
		if err := m.toggleTree(); err != nil {
			return m.fail(err)
		}

		return normalMode(m.editor)
	case "q":
		// 지금 보고 있는 tab 만 닫는다. 마지막 tab 이면 종료가 된다.
		// `!` 는 묻지 않고 닫는다. 그냥 `:q` 는 저장하지 않은 변경이 있으면 확인창을 띄우고,
		// 취소하면 명령줄이 아니라 normal 로 돌아간다.
		if cmd.force {
			return forceCloseTab(m.editor)
		}
		back, _ := normalMode(m.editor)

		return closeTab(back, m.editor)
	case "qa":
		// 전체 종료다. `!` 는 묻지 않고, 그냥 `:qa` 는 어느 tab 이든 변경이 남아 있으면 묻는다.
		if cmd.force {
			return Exit()
		}
		back, _ := normalMode(m.editor)

		return quitAll(back, m.editor)
	default:
		return normalModeMessage(m.editor, "알 수 없는 명령: "+m.input)
	}
}

// edit 은 `:e` 다. 인자가 있으면 파일을 열고, 없으면 보고 있는 파일을 다시 읽는다(ADR-0021).
//
//	:e            다시 읽는다. 저장하지 않은 변경이 있으면 확인창을 띄운다
//	:e!           묻지 않고 다시 읽는다
//	:e <파일>      활성 tab 을 그 파일로 갈아끼운다. 잃을 것이 있으면 확인창을 띄운다
//	:e! <파일>     묻지 않고 갈아끼운다
func (m viewEditorCommand) edit(cmd command) (tea.Model, tea.Cmd) {
	if len(cmd.args) == 0 {
		// 팔레트의 「파일 다시 읽기」와 같은 길이다. `!` 가 확인창 자리를 대신한다(ADR-0016).
		if cmd.force {
			return reloadFile(m.editor)
		}

		return runReloadFile(m.editor)
	}

	path := cmd.args[0]

	// 지금 tab 의 편집이 사라지는 것은 갈아끼울 때뿐이다. 이미 다른 tab 에 열려 있으면
	// replaceTab 이 그리로 옮겨가기만 하므로 잃을 것이 없다.
	_, opened := m.tabOf(path)
	if m.buffer().dirty && !cmd.force && !opened {
		// 취소하면 명령줄이 아니라 normal 로 돌아간다. `:q` 의 확인창과 같다.
		back, _ := normalMode(m.editor)

		return ConfirmDiscard(back, "이 tab 에 다른 파일을 여시겠습니까?", func() (tea.Model, tea.Cmd) {
			return editFile(m.editor, path)
		}), nil
	}

	return editFile(m.editor, path)
}

// editFile 은 묻지 않고 연다. 확인창의 Yes 와 잃을 것이 없을 때가 쓴다.
// reloadFile 과 같은 짝이다.
func editFile(e editor, path string) (tea.Model, tea.Cmd) {
	if err := e.replaceTab(path); err != nil {
		return normalModeMessage(e, errors.Cause(err).Error())
	}

	// 갈아끼운 buffer 는 맨 위에서 시작하지만, 옮겨간 tab 은 보던 자리를 그대로 이어받는다.
	// 어느 쪽이든 지금 폭에 맞춰 둔다 — sidebar 를 여닫은 뒤라면 폭이 달라져 있다.
	e.buffer().scrollTo(e.contentWidth(), e.textHeight())

	return normalMode(e)
}

// fail 은 명령이 실패했음을 아래 줄에 알리고 normal 로 돌아간다.
func (m viewEditorCommand) fail(err error) (tea.Model, tea.Cmd) {
	return normalModeMessage(m.editor, errors.Cause(err).Error())
}

func (m viewEditorCommand) View() tea.View {
	line := ":" + m.input
	view := m.render(tea.CursorBlock, "COMMAND", line)

	// 커서는 본문이 아니라 명령줄 끝에 있어야 한다.
	// 명령줄도 편집 영역 아래에 있으므로 sidebar 만큼 오른쪽으로 옮긴다.
	view.Cursor = tea.NewCursor(screenColAt([]byte(line), len(line))+m.sidebarLeft(), m.height-1)

	return view
}
