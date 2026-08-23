package core

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

// viewSidebar 는 포커스가 좌측 파일 트리에 있는 상태다.
//
// 키 해석이 통째로 다르다 — 위아래가 커서 이동이 아니라 트리 이동이다.
// 그래서 editor 에 focus 필드를 두고 mode 마다 분기하는 대신 type 을 따로 뒀다(ADR-0002).
// 트리 자체(펼친 상태, 고른 항목)는 mode 를 넘어 살아야 하므로 editor.sidebar 에 있다.
func sidebarMode(e *editor) (tea.Model, tea.Cmd) {
	// **안 보이는 pane 에는 포커스를 놓지 않는다.** 화면이 좁아져 sidebar 가 숨을 때 여기서
	// 튕겨내는 것(Update) 과 같은 규칙이고, 들어오는 자리에서도 그것을 지킨다.
	// paletteMode 가 `paletteFits` 로 하는 것과 같은 종류의 방어다.
	//
	// 왜 못 갔는지 아래 줄에 알린다. 아무 일도 안 나면 키가 먹었는지 알 수 없다 — 트리가
	// 없는 것과 `ctrl+w` 가 죽은 것을 화면만 보고 가릴 수 없다.
	if !e.sidebar.open {
		return normalModeMessage(e, "트리가 닫혀 있습니다")
	}
	if !e.sidebarVisible() {
		return normalModeMessage(e, "화면이 좁아 트리를 열 수 없습니다")
	}

	e.sidebar.scrollTo(e.sidebarHeight())

	return viewSidebar{editor: e}, nil
}

// sidebarModeMessage 는 알림을 띄운 채 트리로 돌아간다. normalModeMessage 와 같은 자리다.
//
// 트리에서 시작한 일의 결과는 트리에서 본다 — 파일을 만들고 지우는 것은 몇 번을 이어서 하는
// 일이라, 거절이나 결과마다 편집 영역으로 내보내면 그때마다 `ctrl+w` 로 돌아와야 한다.
func sidebarModeMessage(e *editor, message string) (tea.Model, tea.Cmd) {
	e.notify(message)

	return sidebarMode(e)
}

// sidebarModeError 는 실패를 알린 채 트리로 돌아간다. normalModeError 와 같은 짝이다.
func sidebarModeError(e *editor, err error) (tea.Model, tea.Cmd) {
	e.notifyError(err)

	return sidebarMode(e)
}

type viewSidebar struct {
	*editor

	// state 는 키 나열을 동작 하나로 만드는 상태다. `ctrl+w` 같은 접두 키가 여기 산다.
	// mode 안에서만 사는 상태라 editor 가 아니라 여기에 둔다(ADR-0002).
	state sidebarState
}

// keyState 는 지금 키 상태다. zero value(nil) 는 아무것도 먹지 않은 처음이다.
func (m viewSidebar) keyState() sidebarState {
	if m.state == nil {
		return sidebarStart{}
	}

	return m.state
}

func (m viewSidebar) Init() tea.Cmd { return nil }

func (m viewSidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져서 sidebar 가 자동으로 숨으면 포커스가 안 보이는 곳에 남는다.
		// 다른 mode 의 크기 처리와 달리 여기만 한 줄로 끝나지 않는다.
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}
		m.sidebar.scrollTo(m.sidebarHeight())

		return m, nil
	case tea.FocusMsg:
		// 트리에 포커스가 있어도 본다. ADR-0023 이 셸 복귀에서 이것을 뺀 것은 알림을 놓을
		// 자리가 마땅치 않아서였는데, 이제 statusBar 의 `[!]` 가 mode 와 무관하게 그 자리다
		// (ADR-0023, ADR-0031).
		//
		// 트리를 보고 있어도 편집 영역은 옆에 그려져 있다. 잃을 것이 없으면 가져와서 그
		// 화면이 파일과 어긋나 있지 않게 한다 (ADR-0038).
		return m, m.startOutsideCheck()
	case tea.KeyPressMsg:
		// 한글 되돌림은 파서가 한다. 여기는 키를 그대로 넘긴다(ADR-0008).
		return m.press(msg.String())
	case tea.MouseClickMsg:
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			return m.click(mouse)
		}

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg:
		// 백그라운드 작업의 진행도 git 갱신 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go).
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
// normal mode 와 같은 나눔이고, 도중에 포커스가 옮겨가면 남은 동작은 버린다(ADR-0008).
func (m viewSidebar) press(key string) (tea.Model, tea.Cmd) {
	// 알림은 다음 키를 누르면 사라진다. normal 과 같다.
	m.clearNotice()

	// 한글은 파서가 받아서 푼다. 여기는 키를 그대로 넘기고 나온 동작을 실행하기만 한다.
	names, state := m.keyState().press(key)
	m.state = state

	var model tea.Model = m
	for _, name := range names {
		tree, ok := model.(viewSidebar)
		if !ok {
			return model, nil
		}

		next, cmd := tree.run(name)
		if cmd != nil {
			return next, cmd
		}

		model = next
	}

	return model, nil
}

// run 은 완성된 동작 하나를 실행한다.
//
// 짝이 없는 접두 키 조합(`ctrl+w esc`) 은 여기서 모르는 이름이 되어 아무 일도 하지 않는다.
// 접두 키가 `esc`·`ctrl+c` 를 삼키는 것이 이 규칙이다 — 잘못 누른 `ctrl+w` 를 무르는 것이지
// 트리를 나가거나 편집기를 끄는 것이 아니다. normal mode 의 `g` 와 같다.
func (m viewSidebar) run(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "ctrl+c":
		// 다른 mode 와 같은 경로다. 확인창에서 취소하면 여기로 돌아온다.
		return quitAll(m, m.editor)
	case "ctrl+z":
		// 편집 화면과 같이 셸로 내려간다. 포커스는 트리에 그대로 두고 올라온다.
		// 파일이 밖에서 바뀌었는지는 보지 않는다 — 지금 보고 있는 것은 파일 내용이 아니다(ADR-0023).
		return m, tea.Suspend
	case "ctrl+p":
		// 트리를 뒤지다 이름으로 건너뛰는 길이다. 팔레트가 끝나면 편집 영역으로 나온다.
		return paletteMode(m.editor)
	case "esc":
		return normalMode(m.editor)
	case "ctrl+w ctrl+w", "ctrl+w w":
		// pane 이 둘뿐이라 순환이 곧 왕래다. normal 의 같은 키와 짝이다.
		return normalMode(m.editor)
	case "up", "k":
		m.sidebar.selected--
	case "down", "j":
		m.sidebar.selected++
	case "enter":
		return m.enter()
	case "m c":
		return m.createFile()
	case "m d":
		return m.deleteFile()
	case "m m":
		return m.renameFile()
	default:
		return m, nil
	}

	m.sidebar.scrollTo(m.sidebarHeight())

	return m, nil
}

// createFile 은 `mc` 다. 이름을 받는 화면으로 넘긴다.
//
// 만들 자리는 고른 항목 기준이다 — 디렉터리를 골랐으면 그 안, 파일을 골랐으면 그 파일이 있는
// 디렉터리다. 눈으로 고른 자리에 생기므로 어디에 만들지를 따로 치지 않는다(ADR-0054).
//
// 디렉터리를 가리키는 symlink 는 파일 쪽이다. 트리가 그것을 따라가지 않으므로(ADR-0005)
// 그 안에 만든 것은 트리에 보이지 않는다.
func (m viewSidebar) createFile() (tea.Model, tea.Cmd) {
	node := m.sidebar.selectedNode()
	if node == nil {
		return m, nil
	}

	// `… 읽는 중` 은 파일이 아니라 안내다. 경로가 빈 문자열이라 만들 자리를 짚을 수 없다.
	if node.placeholder {
		return sidebarModeMessage(m.editor, "읽는 중입니다")
	}

	dir := node.path
	if !node.isDir || node.isSymlink {
		dir = filepath.Dir(node.path)
	}

	return createFileMode(m.editor, dir)
}

// deleteFile 은 `md` 다. 지울 수 있는 자리인지 보고 파일은 곧바로 지운다.
//
// 여기서 걸러내는 셋은 물어 봐도 답이 하나뿐인 것들이다. 그 셋을 지나면 파일은 묻지 않고
// 지우고 디렉터리만 묻는 화면으로 넘긴다(ADR-0057).
func (m viewSidebar) deleteFile() (tea.Model, tea.Cmd) {
	node := m.sidebar.selectedNode()
	if node == nil {
		return m, nil
	}
	if node.placeholder {
		return sidebarModeMessage(m.editor, "읽는 중입니다")
	}

	// 뿌리는 트리 자체다. 지우면 트리가 가리킬 자리도 cwd 도 없어진다.
	if node == m.sidebar.tree {
		return sidebarModeMessage(m.editor, "뿌리는 지울 수 없습니다")
	}

	// 열려 있는 파일은 지우지 않는다. buffer 와 디스크가 어긋난 채로 남기지 않고 먼저 닫게 한다.
	if open, ok := m.openTabUnder(node.path, node.isDir && !node.isSymlink); ok {
		return sidebarModeMessage(m.editor, "tab 에 열려 있습니다. 먼저 닫아 주세요: "+m.sidebar.relLabel(open))
	}

	// symlink 는 가리키는 것이 디렉터리여도 링크만 지우므로 파일 쪽이다.
	if !node.isDir || node.isSymlink {
		return removeTreeEntry(m.editor, node.path, false)
	}

	return deleteDirMode(m.editor, node)
}

// renameFile 은 `mm` 이다. 지금 경로가 채워진 화면으로 넘긴다.
//
// 지우기와 달리 tab 에 열려 있어도 막지 않는다. 지우기는 그 buffer 가 갈 곳이 없어서 막는
// 것이고, 이름 바꾸기는 따라갈 곳이 있다 — 경로만 새 이름으로 맞춘다(ADR-0054).
func (m viewSidebar) renameFile() (tea.Model, tea.Cmd) {
	node := m.sidebar.selectedNode()
	if node == nil {
		return m, nil
	}
	if node.placeholder {
		return sidebarModeMessage(m.editor, "읽는 중입니다")
	}

	// 뿌리는 트리 자체다. 이름을 바꾸면 트리의 뿌리도 cwd 도 그 자리에 없다.
	if node == m.sidebar.tree {
		return sidebarModeMessage(m.editor, "뿌리는 이름을 바꿀 수 없습니다")
	}

	return renameFileMode(m.editor, node)
}

// enter 는 고른 항목을 연다. 디렉터리면 펼치거나 접고, 파일이면 tab 으로 연다.
func (m viewSidebar) enter() (tea.Model, tea.Cmd) {
	node := m.sidebar.selectedNode()
	if node == nil {
		return m, nil
	}

	// `… 읽는 중` 은 파일이 아니라 안내다. 열 것이 없다 —
	// 그냥 두면 아래에서 `os.Stat("")` 이 실패해서 오류 문구가 뜬다.
	if node.placeholder {
		return m, nil
	}

	if node.isDir && !node.isSymlink {
		// 펼치는 쪽은 읽는 작업을 시작한다. 자식은 그 결과가 도착할 때 찬다(ADR-0032).
		load := m.toggleNode(node)
		m.sidebar.scrollTo(m.sidebarHeight())

		return m, load
	}

	// 열기 전에 지금 무엇인지 다시 본다. 트리는 펼칠 때 읽은 것이라 그 사이에 지워졌을 수 있다.
	//
	// Stat 은 symlink 를 따라가므로 링크가 가리키는 것이 무엇인지로 판단한다.
	// 일반 파일이 아니면 열지 않는다 — 디렉터리를 가리키는 링크는 ReadFile 이 EISDIR 을 내고,
	// FIFO 나 소켓은 ReadFile 이 영영 돌아오지 않아서 편집기가 통째로 멈춘다.
	info, err := os.Stat(node.path)
	if err != nil {
		return normalModeError(m.editor, err)
	}
	if !info.Mode().IsRegular() {
		return normalModeMessage(m.editor, "일반 파일이 아닙니다: "+node.name)
	}

	reveal, err := m.openTab(node.path)
	if err != nil {
		return normalModeError(m.editor, err)
	}
	m.scrollToCursor()

	// 연 파일을 보러 왔으므로 포커스도 편집 영역으로 간다. 돌아올 때는 ctrl+w ctrl+w 다.
	model, cmd := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(cmd, m.startGitRefresh(), reveal)
}

func (m viewSidebar) View() tea.View {
	// 고른 항목을 아래 줄에 보여준다. 편집 중인 파일의 커서 위치는 지금 볼 것이 아니다.
	// 접두 키를 기다리는 중이면 오른쪽 끝에 그것도 같이 보여준다.
	view := m.editorView(tea.CursorBlock, "TREE",
		m.renderWithShowcmd(m.noticeOr(m.sidebar.selectedLabel()), m.keyState().showcmd()))

	// 커서는 편집 내용이 아니라 고른 트리 항목 위에 있어야 한다.
	// 동작줄 mode 가 하는 것과 같은 방식이다. 이 커서가 곧 포커스 표시다.
	//
	// sidebar 는 화면 맨 윗줄부터 시작하므로 트리 행 번호가 곧 화면 행이다.
	if row, ok := m.sidebar.selectedRow(m.sidebarHeight()); ok {
		view.Cursor = tea.NewCursor(0, row)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}
