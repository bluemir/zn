package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorEmpty 는 tab 이 하나도 없을 때의 화면이다(ADR-0064).
//
// 인자 없이 시작했을 때와 마지막 tab 을 닫았을 때 여기로 온다. 볼 파일이 없으므로 커서도
// 편집도 없고, 편집 영역에는 로고와 버전이 선다(renderEmptyScreen).
//
// **mode 는 normal 이다.** statusBar 에도 그렇게 적는다 — 사람에게는 mode 가 바뀐 것이
// 아니라 편집할 것이 없는 것이다. model 을 따로 두는 것은 normal 이 activeBuffer 를 전제로
// 한 코드에 통째로 매달려 있어서, 거기 갈래를 넣으면 파서와 동작 전체에 「buffer 가 없을
// 때」 가 번지기 때문이다(ADR-0002).
func emptyMode(e *editor) (tea.Model, tea.Cmd) {
	return viewEditorEmpty{editor: e}, nil
}

type viewEditorEmpty struct {
	*editor

	// state 는 normal 과 같은 키 상태다. 숫자 접두와 `g`·`ctrl+w` 접두가 여기 산다.
	state normalState
}

func (m viewEditorEmpty) keyState() normalState {
	if m.state == nil {
		return normalStart{}
	}

	return m.state
}

// Init 은 프로그램이 시작할 때 처음 model 에게만 불린다(bubbletea). 인자 없이 시작하면
// 그 첫 model 이 이것이라, normal 과 같은 시작 작업을 여기서도 낸다(ADR-0064).
func (m viewEditorEmpty) Init() tea.Cmd {
	return m.startInitialJobs()
}

func (m viewEditorEmpty) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.ResumeMsg, tea.FocusMsg:
		// 맞춰 볼 파일이 없어도 검사는 시작한다. 시작하지 않으면 cooldown 고리가 그 자리에서
		// 멈춰서, 여기서 파일을 열어도 바깥 변경을 다시 보지 않는다(outside.go, ADR-0044).
		return m, m.startOutsideCheck()
	case shellDoneMsg:
		// `:!` 로 넘겼던 터미널이 돌아왔다. tab 이 없어도 셸은 쓸 수 있다(ADR-0045).
		return m, m.finishShell(msg.err)
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseClickMsg:
		// 왼쪽 버튼만 본다. 오른쪽 버튼은 tab 을 닫는 것이라 닫을 tab 이 없는 여기서는
		// 할 일이 없다(ADR-0060).
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			return m.click(mouse)
		}

		return m, nil
	case tea.MouseMotionMsg:
		// 트리 구분선을 끄는 것만 받는다(ADR-0145). 볼 파일이 없어도 트리는 있고,
		// 폭을 바꾸는 것은 편집할 것이 있는지와 상관이 없다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft && m.draggingSidebar {
			m.resizeSidebarTo(mouse.X)
		}

		return m, nil
	case tea.MouseReleaseMsg:
		m.draggingSidebar = false

		return m, nil
	case tea.MouseWheelMsg:
		// 굴릴 수 있는 것은 트리뿐이다. 본문은 regionAt 이 아무 자리도 아닌 것으로 돌려준다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 주기 tick 과 백그라운드 작업은 mode 와 무관하다. git 표시는 저장소 이야기라
		// 보고 있는 파일이 없어도 돈다(ADR-0009, job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다. 파서는 normal 과 같은 것을 쓰고 실행할 동작만 거른다.
func (m viewEditorEmpty) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	actions, state := m.keyState().press(key)
	m.state = state

	for _, act := range actions {
		if !allowedWithoutTab(act) {
			continue
		}

		next, cmd := act.run(m.editor)
		if next == nil && cmd == nil {
			continue
		}

		// mode 가 바뀌었거나 낼 것이 있으면 남은 동작은 버린다. normal 과 같은 규칙이다(ADR-0008).
		if next == nil {
			return m, cmd
		}

		return next, cmd
	}

	return m, nil
}

// allowedWithoutTab 은 tab 이 없어도 할 수 있는 동작인지다.
//
// **키를 미리 고르지 않고 실행할 때 거른다.** 파서를 normal 과 같이 쓰는 것은 숫자 접두와
// `g`·`ctrl+w` 접두, 한글 되돌림, showcmd 가 그 안에 있기 때문이다 — 여기에 키 문자열
// 목록을 두면 그 규칙을 두 벌로 적게 되고 한쪽만 고치는 날이 온다(ADR-0006, ADR-0008).
//
// 나머지는 **조용히** 버린다. 알리지 않는 것은 `g` 뒤에 짝이 없는 키가 조용한 것과 같은
// 자리다 — 편집 키를 눌러 볼 화면이 아니라는 것은 화면이 이미 말하고 있다(ADR-0003).
func allowedWithoutTab(act action) bool {
	switch act.(type) {
	case actionQuit, actionSuspend, actionOpenCommandLine, actionOpenPalette, actionFocusTree:
		return true
	}

	return false
}

// click 은 빈 화면에서 왼쪽 버튼을 먹는다. 누를 것이 트리와 그 구분선뿐이다.
func (m viewEditorEmpty) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	region := m.regionAt(mouse.X, mouse.Y)
	m.pressSidebarEdge(region)

	if region == regionSidebar {
		return m.clickSidebar(mouse.Y)
	}

	return m, nil
}

func (m viewEditorEmpty) View() tea.View {
	// 커서 모양은 얹을 커서가 없어서 쓰이지 않는다(editorView).
	//
	// 아래 줄에 tip 을 붙이지 않는다. 본문이 이미 그것을 보여주고 있어서 두 번 뜬다.
	// 치고 있는 접두 키는 그대로 오른쪽 끝에 세운다(ADR-0061).
	return m.editorView(tea.CursorBlock, "NORMAL",
		m.renderWithShowcmd(m.noticeOr(""), m.keyState().showcmd()))
}
