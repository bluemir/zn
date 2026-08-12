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

	// 인자를 받는 명령은 아직 없다. 조용히 버리면 `:w foo` 가 foo 에 저장한 것처럼 보인다.
	if len(cmd.args) > 0 {
		return normalModeMessage(m.editor, "알 수 없는 명령: "+m.input)
	}

	switch cmd.name {
	case "":
		return normalMode(m.editor)
	case "w":
		if err := buf.Save(); err != nil {
			return m.fail(err)
		}

		return normalModeMessage(m.editor, "저장함: "+buf.path)
	case "tabnew":
		// 이름 없는 빈 tab 을 연다. 파일을 지정해서 여는 것은 아직 없다.
		m.newTab()

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
	case "wq", "x":
		if err := buf.Save(); err != nil {
			return m.fail(err)
		}

		return forceCloseTab(m.editor)
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
