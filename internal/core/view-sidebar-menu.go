package core

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// menuHeader 는 항목 앞에 오는 행 수다. 위 테두리, 대상 이름, 가름줄이다.
// menuFrame 은 거기에 아래 테두리를 더한 것이라 상자 높이가 `menuFrame + 항목 수` 다.
const (
	menuHeader = 3
	menuFrame  = menuHeader + 1
)

// viewSidebarMenu 는 트리를 우클릭했을 때 그 자리에 뜨는 메뉴다(ADR-0146).
//
// **부모의 view 를 그대로 쓰고 내용만 갈아끼운다**(ADR-0110). 새 view 를 만들면 터미널
// 설정 넷을 손으로 베끼는 자리가 하나 더 생긴다.
//
// 노드 포인터가 아니라 경로를 든다. 메뉴가 떠 있는 사이에 디렉터리 읽기가 끝나면 그 포인터는
// 이미 어느 화면에도 없을 수 있다 — 지우기 확인창과 같은 까닭이다(view-sidebar-delete.go).
//
// **고르는 것은 마우스뿐이다.** 키로 오르내리지 않는다. 우클릭으로 연 것을 키로 고르게 하면
// 손이 마우스에서 자판으로 한 번 건너가야 하고, 트리에는 이미 키로 가는 길(`m` 접두) 이 있다.
type viewSidebarMenu struct {
	*editor

	parent tea.Model // 닫으면 돌아갈 화면. 우클릭 직전의 mode 그대로다
	path   string    // 대상의 절대 경로
	isDir  bool      // symlink 는 따라가지 않으므로 파일 쪽이다
	isRoot bool      // 뿌리다. 지울 수도 이름을 바꿀 수도 없다

	left, top int // 상자의 화면 좌표

	// hover 는 포인터가 얹힌 항목 자리다. 얹힌 것이 없으면 -1 이다.
	//
	// 열릴 때는 늘 -1 이다. 상자가 누른 칸 **아래**(또는 위) 에 서므로 그 순간 포인터는
	// 어느 항목 위에도 없다.
	hover int
}

// menuItem 은 메뉴 한 줄이다.
type menuItem struct {
	label string
	run   func() (tea.Model, tea.Cmd)
}

// sidebarMenuMode 는 누른 트리 행의 메뉴를 연다.
//
// 누른 행을 고른 자리로 삼는다. 메뉴가 그 행에 대한 것이므로 트리 커서도 거기 서야 하고,
// 메뉴를 닫은 뒤 `ctrl+w` 로 트리에 가면 그 자리에서 이어 하게 된다.
//
// 항목이 없는 아래쪽을 눌렀으면 뿌리 메뉴다. 빈 곳에도 「거기에 만든다」는 뜻이 있다.
func sidebarMenuMode(parent tea.Model, e *editor, x, y int) (tea.Model, tea.Cmd) {
	node := e.sidebar.tree
	if e.sidebar.selectRow(y, e.sidebarHeight()) {
		node = e.sidebar.selectedNode()
	}
	if node == nil {
		return parent, nil
	}

	// `… 읽는 중` 은 파일이 아니라 안내다. 경로가 빈 문자열이라 짚을 자리가 없다 —
	// `mc`·`md`·`mm` 셋이 이미 같은 자리에서 같은 말을 한다.
	//
	// 알림만 내고 포커스는 그대로 둔다. 편집 영역에서 누른 것이 트리로 건너가면 안 된다.
	if node.placeholder {
		e.notify("읽는 중입니다")

		return parent, nil
	}

	menu := viewSidebarMenu{
		editor: e,
		parent: parent,
		path:   node.path,
		isDir:  node.isDir && !node.isSymlink,
		isRoot: node == e.sidebar.tree,
		hover:  -1,
	}

	// 누른 자리 바로 아래가 제자리다. 아래에 자리가 없으면 위로 뒤집고, 어느 쪽이든
	// statusBar 두 줄은 덮지 않는다. 자동완성·이름 바꾸기 창과 같은 함수다.
	menu.left, menu.top = popupPos(x, y, menu.boxWidth(), len(menu.items())+menuFrame, e.width, e.height)

	return menu, nil
}

func (m viewSidebarMenu) Init() tea.Cmd { return nil }

func (m viewSidebarMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 폭이 바뀌면 트리 행도 상자 자리도 어긋난다. 메뉴는 누른 자리에 대한 것이라
		// 그 자리가 움직이면 남겨 둘 것이 없다.
		return m.parent, nil
	case tea.KeyPressMsg:
		// **`esc` 만 받는다.** 고르는 것은 마우스이고 키는 닫는 길 하나다.
		// `ctrl+c` 가 종료가 아니라 취소인 것은 지우기 확인창과 같다(ADR-0054).
		switch msg.String() {
		case "esc", "ctrl+c":
			return m.parent, nil
		default:
			return m, nil
		}
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m.parent, nil
		}

		// **바깥 클릭은 닫기만 한다.** 그 자리의 동작까지 하면 메뉴를 닫으려고 누른 것이
		// 파일을 열고, 잃는 것이 화면 이동으로 끝나지 않는다.
		at := m.itemAt(mouse.X, mouse.Y)
		if at < 0 {
			return m.parent, nil
		}

		return m.items()[at].run()
	case tea.MouseMotionMsg:
		// **포인터가 얹힌 항목을 따라간다.** 버튼을 누르지 않은 이동은 AllMotion 에서만
		// 오고(View 에서 그동안만 올린다) 상자 밖으로 나가면 itemAt 이 -1 을 준다.
		//
		// 끄는 중(버튼을 누른 채) 에 오는 것도 같이 받는다. 우클릭을 누른 채 손이 흔들린
		// 것이라 갈라 둘 까닭이 없다.
		m.hover = m.itemAt(msg.X, msg.Y)

		return m, nil
	case tea.MouseWheelMsg:
		// 굴리지 않는다. 트리가 밀리면 메뉴가 가리키는 행이 화면과 어긋나고,
		// 상자에 가려 아래를 볼 수도 없다 — 팔레트가 좌표를 보지 않는 것과 같은 자리다.
		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 흘려보내면 다음 조각을
		// 받을 Cmd 를 아무도 내지 않아 작업이 영영 멈춘다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// items 는 대상에 따라 낼 항목이다.
//
// **목록도 고르는 것도 그리는 것도 이 하나를 지난다.** 셋이 다른 목록을 보면 누른 자리와
// 실행되는 것이 어긋난다 — 팔레트의 `commands` 가 같은 이유로 한 자리다.
//
// 뿌리는 「새 파일」 하나다. 지우면 트리가 가리킬 자리도 cwd 도 없어지고, 이름을 바꿔도
// 마찬가지다 — 키로 가는 길(`md`·`mm`) 이 이미 같은 말로 막고 있다.
func (m viewSidebarMenu) items() []menuItem {
	create := menuItem{label: "새 파일", run: m.openCreate}
	if m.isRoot {
		return []menuItem{create}
	}

	return []menuItem{
		create,
		{label: "새 이름", run: m.openRename},
		{label: "지우기", run: m.openDelete},
	}
}

// openCreate 는 이름을 받는 상자로 간다. 만들 자리는 대상 기준이다 — 디렉터리를 눌렀으면
// 그 안, 파일을 눌렀으면 그 파일이 있는 디렉터리다(ADR-0054).
func (m viewSidebarMenu) openCreate() (tea.Model, tea.Cmd) {
	dir := m.path
	if !m.isDir {
		dir = filepath.Dir(m.path)
	}

	return sidebarInputMode(m, "새 파일", "", func(name string) (tea.Model, tea.Cmd) {
		return createEntry(m.editor, dir, name)
	})
}

// openRename 은 지금 경로가 채워진 상자로 간다. 치는 것이 이름이 아니라 뿌리 기준 경로라
// 앞을 고치면 자리를 옮기는 것이 된다(ADR-0054).
func (m viewSidebarMenu) openRename() (tea.Model, tea.Cmd) {
	return sidebarInputMode(m, "새 이름", m.sidebar.relLabel(m.path), func(name string) (tea.Model, tea.Cmd) {
		return renameEntry(m.editor, m.path, m.isDir, name)
	})
}

// openDelete 는 확인창으로 간다.
//
// **메뉴에서는 깨끗한 파일도 묻는다.** `md` 는 묻지 않는데(ADR-0057) 여기서 갈리는 것은
// 클릭이 미끄러지기 때문이다(ADR-0060). 키로 지우는 것은 `m` 과 `d` 를 이어 쳐야 하지만
// 메뉴는 누른 자리 바로 아래에 항목이 서므로, 메뉴를 닫으려던 손이 그 자리를 누를 수 있다.
func (m viewSidebarMenu) openDelete() (tea.Model, tea.Cmd) {
	return deleteConfirmMode(m.parent, m.editor, m.path, m.isDir, m.dirtyTabsUnder(m.path, m.isDir))
}

// itemAt 은 좌표가 가리키는 항목 자리다. 상자 밖이거나 항목 행이 아니면 -1 이다.
//
// **상자 자리를 아는 곳과 클릭을 먹는 곳이 같은 타입이다.** ADR-0012 와 ADR-0060 이
// 「박스의 자리를 `regionAt` 이 알아야 한다」고 적어 둔 것을 여기서 뒤집는다 — `regionAt` 은
// editor 의 고정 배치를 재는 자리이고, 이 상자는 이 mode 가 사는 동안만 있는 것이라
// 거기에 영역을 더하면 메뉴가 닫힌 뒤에도 그 좌표가 editor 에 남는다(ADR-0146).
func (m viewSidebarMenu) itemAt(x, y int) int {
	if x < m.left || x >= m.left+m.boxWidth() {
		return -1
	}

	at := y - m.top - menuHeader
	if at < 0 || at >= len(m.items()) {
		return -1
	}

	return at
}

// title 은 상자 첫 줄에 서는 대상 이름이다.
//
// **이름을 적는다.** 트리에서 고른 행을 나타내는 것은 터미널 커서 하나인데(ADR-0005)
// 편집 영역에 포커스가 있으면 그 커서가 트리에 없다. 무엇에 대한 메뉴인지 말할 자리가
// 이 줄뿐이다 — tabline 우클릭의 확인창이 tab 이름을 적는 것과 같다(ADR-0060).
func (m viewSidebarMenu) title() string {
	label := m.sidebar.relLabel(m.path)

	// 뿌리는 relLabel 이 이미 `zn/` 처럼 자국을 붙여 준다. 그것을 보지 않고 더하면 `zn//` 가
	// 된다. 만들 자리를 적는 자리도 같은 것을 본다(createDirLabel).
	if !m.isDir || strings.HasSuffix(label, "/") {
		return label
	}

	return label + "/"
}

// boxWidth 는 상자의 칸 수다. 테두리 둘과 좌우 빈 칸 하나씩을 더한다.
//
// 화면보다 넓어지지 않게 가둔다. 한 칸이라도 나가면 캔버스가 그만큼 커져 화면이 밀린다(ADR-0011).
func (m viewSidebarMenu) boxWidth() int {
	inner := textarea.WidthOf(m.title())
	for _, item := range m.items() {
		if w := textarea.WidthOf(item.label); w > inner {
			inner = w
		}
	}

	wanted := inner + 4
	if m.width < wanted {
		return max(m.width, 4)
	}

	return wanted
}

func (m viewSidebarMenu) renderBox() string {
	width := m.boxWidth()
	inner := width - 4 // 좌우 테두리 2 + 좌우 1칸 공백
	chars := m.boxChars
	edge := strings.Repeat(chars.horizontal, width-2)

	// 바탕은 테두리 안쪽 전부다. 좌우 빈 칸까지 칠해야 띠가 끊기지 않는다.
	// **칠하는 것은 폭을 채운 뒤다** — padTo 는 escape 를 칸으로 세지 않는다.
	row := func(text string, hover bool) string {
		body := " " + padTo(text, inner) + " "
		if hover {
			body = styleMenuHover.Render(body)
		}

		return chars.vertical + body + chars.vertical
	}

	// 이름은 넘치면 **왼쪽부터** 접는다. 오른쪽부터 자르면 파일 이름이 사라지고 폴더만 남는다.
	rows := []string{
		chars.topLeft + edge + chars.topRight,
		row(trimLeftToWidth(m.title(), inner), false),
		chars.leftTee + edge + chars.rightTee,
	}

	for at, item := range m.items() {
		rows = append(rows, row(truncateToWidth(item.label, inner), at == m.hover))
	}

	return strings.Join(append(rows, chars.bottomLeft+edge+chars.bottomRight), "\n")
}

func (m viewSidebarMenu) View() tea.View {
	view := m.parent.View()

	// **메뉴가 떠 있는 동안만 mouse 를 AllMotion 으로 올린다.** 버튼을 누르지 않은 이동은
	// CellMotion 에서 오지 않아서, 그대로는 포인터가 어느 항목에 얹혔는지 알 길이 없다
	// (ADR-0012, ADR-0060).
	//
	// 늘 올려 두지 않는 까닭은 ADR-0012 가 적어 둔 그대로다. 포인터가 움직이는 내내
	// 이벤트가 쏟아지고, 얻는 것은 이 강조 하나다. 메뉴는 잠깐 떴다 지는 것이라 그동안만
	// 치르는 값이다.
	//
	// 프레임마다 escape 가 오가지는 않는다. bubbletea 는 **값이 바뀔 때만** 모드 전환을
	// 쓴다(`cursed_renderer.go`). 메뉴를 열 때 한 번, 닫으면서 부모의 CellMotion 으로
	// 돌아갈 때 한 번이다.
	//
	// 부모의 view 에서 갈아끼우는 칸이 이것 하나다. 나머지 셋은 그대로 물려받는다(ADR-0110).
	view.MouseMode = tea.MouseModeAllMotion

	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderBox()).X(m.left).Y(m.top).Z(1),
	).Render()

	return view
}
