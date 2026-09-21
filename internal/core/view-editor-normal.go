package core

import (
	tea "charm.land/bubbletea/v2"
)

// viewEditorNormal 은 normal mode 다. 커서가 글자 위에 있어서 줄 끝 다음 칸에 설 수 없다.
func normalMode(e *editor) (tea.Model, tea.Cmd) {
	// **하단 drawer 는 여기서 닫힌다.** 판을 연 mode 마다 나가는 길에서 지우면 한 곳은
	// 반드시 빠뜨리고, 그때 편집 영역이 줄어든 채로 굳는다 — statusBar 위에 빈 띠가 남고
	// 마우스 행 계산도 어긋난다. 아래의 `hasTab` 과 똑같은 까닭이라 같은 자리에 둔다
	// (ADR-0056, ADR-0064, ADR-0069).
	//
	// 판에서 판으로 넘어가는 길(정의 후보를 보다 사용처 답이 오는 자리) 은 여기를 지나지
	// 않는다 — 그쪽은 새 판이 자기 높이를 잡는다.
	if e.drawerHeight != 0 {
		e.setDrawerHeight(0)

		if e.hasTab() {
			e.scrollToCursor()
		}
	}

	// **볼 파일이 없으면 빈 화면이다.** 편집 화면으로 돌아오는 길이 백 곳 남짓인데 전부
	// 이 함수를 지나므로 여기 하나로 가른다 — 각자 고치면 한 곳은 반드시 빠뜨리고, 그
	// 한 곳이 tab 없이 normal 로 들어가 다음 프레임에 터진다(ADR-0064).
	if !e.hasTab() {
		return emptyMode(e)
	}

	// normal 에는 고른 범위가 없다. visual 을 떠나는 문이 여기와 insertMode 둘뿐이라
	// 놓는 자리도 그 둘이다 (ADR-0037).
	e.activeBuffer().ClearSelection()

	return viewEditorNormal{editor: e}, nil
}

// normalModeMessage 는 명령 결과를 아래 줄에 띄운 채로 normal 로 돌아간다.
// 알림은 mode 밖(editor)에 있다 — 백그라운드 작업의 실패가 어느 mode 에서든 도착한다.
func normalModeMessage(e *editor, message string) (tea.Model, tea.Cmd) {
	e.notify(message)

	return normalMode(e)
}

// normalModeError 는 실패를 알린 채로 normal 로 돌아간다.
//
// 예전에는 이것이 command mode 의 메서드(`fail`) 여서 팔레트·트리·검색이 쓸 수 없었고,
// 그쪽은 `normalModeMessage(e, errors.Cause(err).Error())` 를 손으로 되풀이했다. 그러면
// 기록에 남길 때 「이 알림이 오류인가」를 문구만 보고는 알 수 없다 — 갈래를 type 으로
// 가르려면 오류가 `error` 인 채로 여기까지 와야 한다(notice.go, ADR-0053).
func normalModeError(e *editor, err error) (tea.Model, tea.Cmd) {
	e.notifyError(err)

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
// 바뀌어도 다시 불리지 않는다.
//
// 무엇을 시작하는지는 editor 가 든다. 인자 없이 시작하면 첫 model 이 빈 화면이라 그쪽
// Init 도 같은 것을 내야 하기 때문이다(startInitialJobs, ADR-0064).
func (m viewEditorNormal) Init() tea.Cmd {
	return m.startInitialJobs()
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
		// 예약이 한 번에 하나뿐이고(scheduleEditTick) 보낼 것이 없으면 그때 아무것도
		// 보내지 않기 때문이다 — 「고치는 동작」 목록을 여기 또 두지 않는다(ADR-0051).
		return next, tea.Batch(cmd, m.scheduleEditTick())
	case tea.MouseClickMsg:
		// 왼쪽과 오른쪽만 본다. 가운데 버튼에 붙일 동작은 아직 정하지 않았다.
		//
		// 대기 중인 접두 키(m.state) 와 알림(m.message) 은 그대로 둔다 — 키 이야기다.
		switch mouse := msg.Mouse(); mouse.Button {
		case tea.MouseLeft:
			// **끌기 시작한 자리를 여기서 정한다.** tabline 에서 시작한 드래그만 tab 을
			// 옮긴다(ADR-0090). 누르기 없는 드래그가 없으므로 값이 늘 맞다.
			m.draggingTab = m.regionAt(mouse.X, mouse.Y) == regionTabline

			return m.click(mouse)
		case tea.MouseRight:
			// tabline 의 tab 을 닫는다. 다른 영역에서는 아무 일도 없다(ADR-0060).
			return rightClick(m, m.editor, mouse)
		}

		return m, nil
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}

		// 구분선에서 시작했으면 트리 폭을 끈다. 구분선이 한 칸이라 시작한 자리를 들고
		// 있어야 한다(ADR-0145).
		if m.draggingSidebar {
			m.resizeSidebarTo(mouse.X)

			return m, nil
		}

		// tabline 에서 시작했으면 tab 을 옮긴다. **지금 자리가 편집 영역이어도 범위를 고르지
		// 않는다** — tabline 이 한 행이라 가로로 끄는 손이 아래로 한 칸 새기 쉽다(ADR-0090).
		if m.draggingTab {
			m.dragTab(mouse.X, mouse.Y)

			return m, nil
		}

		// 버튼을 누른 채 움직이는 중이다. 누른 자리를 anchor 로 삼아 범위를 고르기 시작한다 —
		// 누른 자리는 MouseClickMsg 가 이미 커서로 만들어 두었다(ADR-0012, ADR-0037).
		//
		// 클릭만 하는 것은 지금처럼 커서 이동이다. vim 도 드래그부터 visual 이다.
		if m.regionAt(mouse.X, mouse.Y) == regionText {
			m.startSelection(false)
			m.dragTo(mouse.X, mouse.Y)
			m.activeBuffer().ExtendSelection()

			return visualMode(m.editor)
		}

		return m, nil
	case tea.MouseReleaseMsg:
		// 끄는 것이 끝났다. 지우지 않아도 다음 누르기가 값을 바로잡지만, 끈 자리가 남아 있으면
		// 다른 mode 에서 누른 뒤 normal 로 돌아온 첫 편집 영역 드래그가 한 번 먹히지 않는다.
		m.draggingTab = false
		m.draggingSidebar = false

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
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
	m.clearNotice()

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
		m.renderWithTip(m.noticeOr(m.renderPosition()), m.keyState().showcmd()))
}
