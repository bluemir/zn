package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorNormal 은 normal mode 다. 커서가 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
func normalMode(e *editor) (tea.Model, tea.Cmd) {
	// normal 에는 고른 범위가 없다. visual 을 떠나는 문이 여기와 insertMode 둘뿐이라
	// 놓는 자리도 그 둘이다 (ADR-0037).
	e.activeBuffer().selection = selection{}

	return viewEditorNormal{editor: e}, nil
}

// normalModeMessage 는 명령 결과를 아래 줄에 띄운 채로 normal 로 돌아간다.
// 알림은 mode 밖(editor)에 있다 — 백그라운드 작업의 실패가 어느 mode 에서든 도착한다.
func normalModeMessage(e *editor, message string) (tea.Model, tea.Cmd) {
	e.message = message

	return normalMode(e)
}

type viewEditorNormal struct {
	*editor

	// state 는 키 나열을 동작 하나로 만드는 상태다. 숫자 접두와 `g` 같은 접두 키가 여기 산다.
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
	// CLI 인자로 Go 파일을 열고 시작하는 길이 여기다. 언어 서버는 파일을 열 때 띄우는데
	// (ADR-0051) 시작할 때 이미 열려 있는 것은 openTab 을 지나지 않는다.
	return tea.Batch(m.startGitRefresh(), m.startOutsideCheck(), m.startTree(), m.startGoplsForOpenBuffers())
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
		// 잃을 것이 없으면 가져온다 (ADR-0038).
		//
		// 검사는 작업이 한다. 큰 파일에서 창을 오갈 때마다 편집기가 멈추지 않는다(ADR-0044).
		return m, m.startOutsideCheck()
	case shellDoneMsg:
		// `:!` 로 넘겼던 터미널이 돌아왔다. 이 msg 를 다른 mode 에서 받을 일은 없다 —
		// runShell 이 언제나 normal 을 돌려주고, 터미널이 돌아오기 전에는 키가 처리되지
		// 않아서 그 사이에 mode 가 바뀔 수 없다 (ADR-0045).
		return m, m.finishShell(msg.err)
	case tea.KeyPressMsg:
		// 한글 되돌림은 파서가 한다. 여기는 키를 그대로 넘긴다(ADR-0008).
		next, cmd := m.press(msg.String())

		// 키가 파일을 고쳤으면 언어 서버와 맞출 때를 예약한다. 고쳤는지 보지 않는 것은
		// 예약이 한 번에 하나뿐이고(scheduleLspTick) 보낼 것이 없으면 그때 아무것도
		// 보내지 않기 때문이다 — 「고치는 동작」 목록을 여기 또 두지 않는다(ADR-0051).
		return next, tea.Batch(cmd, m.scheduleLspTick())
	case tea.MouseClickMsg:
		// 왼쪽 버튼만 본다. 가운데·오른쪽에 붙일 동작은 아직 정하지 않았다.
		//
		// 누를 때 반응하고 뗄 때는 보지 않는다. 드래그가 없으니 누른 자리가 곧 고른 자리다.
		// 대기 중인 접두 키(m.state) 와 알림(m.message) 은 그대로 둔다 — 키 이야기다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			return m.click(mouse)
		}

		return m, nil
	case tea.MouseMotionMsg:
		// 버튼을 누른 채 움직이는 중이다. 누른 자리를 anchor 로 삼아 범위를 고르기 시작한다 —
		// 누른 자리는 MouseClickMsg 가 이미 커서로 만들어 두었다(ADR-0012, ADR-0037).
		//
		// 클릭만 하는 것은 지금처럼 커서 이동이다. vim 도 드래그부터 visual 이다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft && m.regionAt(mouse.X, mouse.Y) == regionText {
			m.startSelection(false)
			m.dragTo(mouse.X, mouse.Y)

			return visualMode(m.editor)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go).
		//
		// 파일 검사도 여기로 온다. 예전에는 normal·트리가 자기 case 에서 직접 보았는데,
		// 이제 결과가 `dirty` 만 보고 갈리므로 mode 를 가릴 이유가 없다(ADR-0044).
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 차례로 실행한다.
//
// 동작이 여럿인 것은 한글 때문이다. 파서가 `ㅘ` 를 `h` `k` 로 풀어 왼쪽·위 두 동작을 준다
// (normal-key-parser.go). 동작이 하나도 완성되지 않았으면 화면은 showcmd 만 바뀐다.
//
// **도중에 mode 가 바뀌면 남은 동작은 버린다.** `마` 는 `a` `k` 인데 `a` 에서 insert mode 로
// 들어가므로, 버리지 않으면 남은 `k` 가 파일에 글자로 꽂힌다. 한글 상태로 normal mode 에 온 것
// 자체가 사고이므로 사고가 편집을 일으키는 것보다 아무 일도 안 나는 것이 낫다(ADR-0008).
func (m viewEditorNormal) press(key string) (tea.Model, tea.Cmd) {
	// 알림은 다음 키를 누르면 사라진다.
	m.message = ""

	// 한글은 파서가 받아서 푼다. 여기는 키를 그대로 넘기고 나온 동작을 실행하기만 한다.
	actions, state := m.keyState().press(key)
	m.state = state

	for _, act := range actions {
		// 동작은 editor 만 받는다. editor 는 포인터라 여기서 고친 것이 다음 동작에도 보인다(ADR-0026).
		next, cmd := act.run(m.editor)

		// 둘 다 없으면 mode 도 그대로고 낼 것도 없다. 다음 동작으로 간다.
		if next == nil && cmd == nil {
			continue
		}

		// mode 를 바꾸는 동작이거나 cmd 를 내는 동작(종료, 확인창) 이다. 남은 동작은 버린다 —
		// 뒤에 올 것이 그 결과를 뒤집으면 안 된다.
		if next == nil { // 현재 모델 유지
			return m, cmd
		}

		return next, cmd
	}

	return m, nil
}

func (m viewEditorNormal) View() tea.View {
	// 커서가 글자 위에 있으므로 블록이다.
	return m.editorView(tea.CursorBlock, "NORMAL",
		m.renderWithShowcmd(m.messageOr(m.renderPosition()), m.keyState().showcmd()))
}
