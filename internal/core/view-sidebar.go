package core

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/textarea"
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
		switch mouse := msg.Mouse(); mouse.Button {
		case tea.MouseLeft:
			return m.click(mouse)
		case tea.MouseRight:
			// tabline 의 tab 을 닫는다. 보고 있던 tab 이었으면 normal 로 나간다(ADR-0060).
			return rightClick(m, m.editor, mouse)
		}

		return m, nil
	case tea.MouseMotionMsg:
		// 트리 구분선을 끄는 것만 받는다(ADR-0145). 트리에는 끌어서 고를 범위가 없다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft && m.draggingSidebar {
			m.resizeSidebarTo(mouse.X)
		}

		return m, nil
	case tea.MouseReleaseMsg:
		m.draggingSidebar = false

		return m, nil
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
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
	actions, state := m.keyState().press(key)
	m.state = state

	var model tea.Model = m
	for _, act := range actions {
		tree, ok := model.(viewSidebar)
		if !ok {
			return model, nil
		}

		next, cmd := tree.run(act)
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
//
// **숫자를 보는 것은 이동뿐이다.** 화면 단위 이동까지 여덟이고 나머지는 act.count 를 읽지 않아
// 그냥 무시한다 —
// `3md` 가 `md` 인 것이 normal 에서 `3i` 가 `i` 인 것과 같은 자리다(ADR-0059).
func (m viewSidebar) run(act sidebarAction) (tea.Model, tea.Cmd) {
	switch act.name {
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
		// 숫자를 대지 않으면 한 행이다. 양끝은 아래 scrollTo 가 당긴다 — 스무 행을 뛰다
		// 뿌리를 지나쳐도 뿌리에 선다.
		m.sidebar.selected -= max(act.count, 1)
	case "down", "j":
		m.sidebar.selected += max(act.count, 1)
	case "g g":
		// 숫자를 대면 그 행이다 — `20gg` 는 보이는 행 스무 번째다. 행은 1 부터 세고 자리는
		// 0 부터라 하나를 뺀다. normal 의 `20gg` 와 같은 자리다(ADR-0059).
		m.sidebar.selected = 0
		if act.count > 0 {
			m.sidebar.selected = act.count - 1
		}
	case "G":
		// 숫자를 대지 않으면 마지막 행이고, 대면 `gg` 와 똑같이 그 행이다. normal 의 `G` 와
		// 같다 — 그래서 파서가 count 를 1 로 메우지 않고 0 을 그대로 준다.
		m.sidebar.selected = len(m.sidebar.rows()) - 1
		if act.count > 0 {
			m.sidebar.selected = act.count - 1
		}
	case "home":
		// 숫자를 보지 않는다. `gg`·`G` 는 숫자를 행 번호로 받는데(`20G`) 이 키에는 숫자를
		// 붙여 치는 손버릇이 없고, 받으면 같은 일을 하는 길이 두 벌이 된다.
		m.sidebar.selected = 0
	case "end":
		m.sidebar.selected = len(m.sidebar.rows()) - 1
	case "ctrl+d":
		// 반 화면·한 화면 이동이다. 고른 항목과 화면이 같이 내려가므로 커서가 트리 안 같은
		// 자리에 남는다 — 휠이 화면만 굴리는 것과 갈린다(ADR-0062, ADR-0063).
		m.sidebar.movePage(textarea.PageDown, textarea.PageHalf, act.count, m.sidebarHeight())
	case "ctrl+u":
		m.sidebar.movePage(textarea.PageUp, textarea.PageHalf, act.count, m.sidebarHeight())
	case "ctrl+f", "pgdown":
		m.sidebar.movePage(textarea.PageDown, textarea.PageFull, act.count, m.sidebarHeight())
	case "ctrl+b", "pgup":
		m.sidebar.movePage(textarea.PageUp, textarea.PageFull, act.count, m.sidebarHeight())
	case "enter":
		return m.enter()
	case "R":
		// 뿌리부터 펼쳐 둔 디렉터리를 전부 다시 읽는다. NERDTree 의 `R` 과 같은 자리다.
		//
		// 감시가 붙어 있어서 대개 손을 쓸 일이 없다(ADR-0093, ADR-0134). 남는 자리는 감시가
		// 못 보는 곳이다 — 네트워크 파일 시스템, 감시 개수 상한에 걸린 큰 트리, cwd 위쪽.
		//
		// 고른 자리도 화면도 건드리지 않는다. 다시 읽는 것은 목록을 맞추는 일이고 어디를
		// 보고 있는지를 옮기는 일이 아니다.
		return m, m.reloadTree()
	case "m c", "m a":
		// `ma` 도 같은 자리다. NERDTree 의 파일 메뉴가 `a`(add) 로 만드는데(ADR-0054 가
		// 그 메뉴를 이 자리에 두었다) 그 손버릇으로 온 손이 `mc` 를 찾지 않아도 되게 한다.
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
// 여기서 걸러내는 둘은 물어 봐도 답이 하나뿐인 것들이다. 그 둘을 지나면 파일은 묻지 않고
// 지우고 디렉터리만 묻는 화면으로 넘긴다(ADR-0057).
//
// **tab 에 열려 있는 것도 지운다.** 지우면서 그 tab 을 같이 닫는다(ADR-0130). 저장하지 않은
// 변경이 있는 것만 물어 본다 — 묻지 않고 지우는 근거가 「잃을 것이 없다」였다.
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

	// symlink 는 가리키는 것이 디렉터리여도 링크만 지우므로 파일 쪽이다.
	isDir := node.isDir && !node.isSymlink
	dirty := m.dirtyTabsUnder(node.path, isDir)

	// 물을 일이 있으면 창 하나로 보낸다. 디렉터리는 안의 것이 화면에 드러나지 않아서고,
	// 저장하지 않은 tab 은 그 편집이 갈 곳이 없어서다(ADR-0057, ADR-0130).
	if isDir || dirty > 0 {
		return deleteConfirmMode(m, m.editor, node.path, isDir, dirty)
	}

	// 깨끗한 파일은 묻지 않는다. 트리의 그 행이 이름을 다 보여주고 있어서 물음이 더해 주는
	// 것이 없다. 무엇이 사라졌는지는 아래 줄의 알림이 말한다(ADR-0057).
	return removeTreeEntry(m.editor, node.path, false)
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
	m.arrive()

	// 연 파일을 보러 왔으므로 포커스도 편집 영역으로 간다. 돌아올 때는 ctrl+w ctrl+w 다.
	model, cmd := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(cmd, m.startGitRefresh(), reveal)
}

// fileMenuHint 는 `m` 을 먹고 다음 키를 기다리는 동안 아래 줄 왼쪽에 서는 안내다.
// 기다리는 중이 아니면 빈 문자열이다.
//
// **`m` 만 안내한다.** `ctrl+w`·`g` 는 짝이 normal mode 와 같은 손버릇이라 트리에서 새로
// 배울 것이 없고, 이 자리에 셋을 다 늘어놓으면 정작 트리에만 있는 파일 메뉴가 묻힌다.
//
// `a` 도 같이 적는다. NERDTree 손버릇으로 온 손이 헛치지 않게 둔 별칭인데(run 의 `m a`,
// ADR-0054) 안내가 `c` 만 세우면 그 손은 자기가 아는 키가 죽은 줄로 읽는다.
//
// 말은 들어가는 화면의 것을 그대로 쓴다. `mc` 와 `mm` 은 곧 `새 파일: `·`새 이름: ` 이 뜨는
// 자리고(view-sidebar-create.go, view-sidebar-rename.go), 여기서 다른 말로 부르면 같은 일을
// 두 이름으로 배우게 된다. 화면이 없는 지우기만 알림의 「지웠습니다」 를 따른다.
//
// 좁으면 오른쪽부터 잘린다. 아래 줄을 자르는 것은 renderStatusBar 하나가 하고 여기는
// 문구만 준다 — 앞의 `a,c 새 파일` 만 남아도 첫 항목은 읽힌다.
func (m viewSidebar) fileMenuHint() string {
	pending, ok := m.keyState().(sidebarPending)
	if !ok || pending.prefix != "m" {
		return ""
	}

	return "a,c 새 파일  d 지우기  m 새 이름"
}

func (m viewSidebar) View() tea.View {
	// 고른 항목을 아래 줄에 보여준다. 편집 중인 파일의 커서 위치는 지금 볼 것이 아니다.
	// 접두 키를 기다리는 중이면 오른쪽 끝에 그것도 같이 보여준다.
	//
	// `m` 을 먹은 동안만 그 자리를 파일 메뉴 안내가 쓴다. 고른 항목 이름은 접두 키를
	// 치기 전에 이미 읽은 것이고, 지금 급한 것은 다음에 무엇을 누를 수 있는지다.
	bottom := m.sidebar.selectedLabel()
	if hint := m.fileMenuHint(); hint != "" {
		bottom = hint
	}

	view := m.editorView(tea.CursorBlock, "TREE",
		m.renderWithTip(m.noticeOr(bottom), m.keyState().showcmd()))

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
