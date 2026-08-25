package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewGrepInput 은 여러 파일 검색의 패턴을 받는 자리다. `:grep`(패턴 없이) 와 팔레트의
// 「프로젝트 검색」이 여기로 온다.
//
// **`/` 검색창과 같은 틀이다**(view-editor-search.go). 아래 줄에 앞머리와 친 글자를 적고
// 커서를 그 끝에 둔다 — 패턴을 치는 자리가 화면에서 같은 곳이어야 손이 헷갈리지 않는다.
//
// **미리보기가 없다.** `/` 는 치는 동안 첫 매칭으로 화면이 따라가는데(incsearch) 여기서는
// 글자마다 저장소를 훑는 일이 된다. 그래서 Enter 로 한 번만 시작한다.
//
// 명령줄에 얹지 않고 mode 를 따로 둔 것은 패턴이 통째로 글자이기 때문이다 — 이 mode 가
// `msg.Text` 를 그대로 받는다. 트리의 이름 받기와 같은 까닭이다(ADR-0008, ADR-0054).
func grepInputMode(e *editor) (tea.Model, tea.Cmd) {
	// 지난 패턴을 채워 주지 않는다. 대개 다른 것을 찾으려고 여는 자리라, 채워 두면 늘
	// 먼저 지우게 된다 — 이름 바꾸기가 지금 이름을 채워 주는 것과 갈리는 자리다(ADR-0067).
	return viewGrepInput{editor: e}, nil
}

type viewGrepInput struct {
	*editor

	input string
}

func (m viewGrepInput) Init() tea.Cmd { return nil }

func (m viewGrepInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			// 명령줄·검색창과 같이 앞머리까지 지우면 나간다.
			if m.input == "" {
				return normalMode(m.editor)
			}
			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]

			return m, nil
		default:
			// Text 는 출력 가능한 글자에만 찬다. 특수 키와 modifier 조합은 비어 있다.
			if msg.Text == "" {
				return m, nil
			}
			m.input += msg.Text

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 치는 동안에도 화면은 둘러볼 수 있다. 명령줄과 같다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg:
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// run 은 친 패턴으로 검색을 시작하고 결과 판으로 넘어간다.
func (m viewGrepInput) run() (tea.Model, tea.Cmd) {
	if m.input == "" {
		return normalModeMessage(m.editor, "검색할 패턴이 없습니다")
	}

	return runGrep(m.editor, m.input)
}

// grepInputPrompt 는 아래 줄 맨 앞에 적는 앞머리다. 무엇을 치는 중인지가 이것으로 보인다.
const grepInputPrompt = "검색: "

func (m viewGrepInput) View() tea.View {
	line := grepInputPrompt + m.input
	view := m.editorView(tea.CursorBlock, "GREP", line)

	// 커서는 본문이 아니라 아래 줄 끝에 있어야 한다. 명령줄·검색창과 같은 자리다.
	view.Cursor = tea.NewCursor(screenWidthOf(line)+m.sidebarLeft(), m.height-1)

	return view
}

// grepInputMode 를 부르는 자리는 인자 없는 `:grep` 하나다. 팔레트는 박스에서 왔으므로
// 박스로 받는다(view-grep-modal.go, ADR-0078 §7).
