package core

import (
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// sidebarWidth 는 좌측 sidebar 가 차지하는 칸 수다(docs/spec.md).
const sidebarWidth = 32

// minTextWidth 는 sidebar 를 그리고도 남아 있어야 하는 편집 영역 너비다.
// 이보다 좁아지면 sidebar 가 켜져 있어도 그리지 않는다.
const minTextWidth = 20

// sidebar 는 좌측 파일 트리다.
//
// mode 가 바뀌어도 유지되어야 하므로 editor 가 들고 있다.
// tea.Model 이 아니라 평범한 struct 다 — 중첩 model 로 만들면 키가 어디서 처리되는지가
// 한 겹 숨는다. ADR-0002 가 type 교체를 고른 것은 pane 이 아니라 mode 에 대해서였다.
// 포커스가 여기 있는 상태만 mode(viewSidebar) 로 나타낸다.
//
// tree 는 포인터로 이은 항목들이다. editor 하나를 mode 마다 나눠 쓰므로 펼친 상태가 mode 를 넘어 남는다.
type sidebar struct {
	// open 은 사용자가 열어둔 상태인지다. 실제로 그리는지는 editor.sidebarVisible 이 정한다.
	open bool

	// root 는 트리의 뿌리다. os.Getwd() 로 채우지만 필드로 두어야 테스트가 t.TempDir() 를 넣는다.
	root string

	tree *treeNode

	// selected 는 보이는 행 중 몇 번째인지다. top 은 그중 화면 맨 위에 그릴 행이다.
	selected int
	top      int

	// pendingReveal 은 아직 자리를 잡지 못한 reveal 대상의 절대 경로다.
	//
	// 디렉터리 읽기가 비동기라 reveal 이 한 번에 끝나지 않는다. 도중에 아직 읽지 않은
	// 디렉터리를 만나면 그것을 읽는 작업을 시작하고 여기에 남겨 둔 뒤, 자식이 도착하면
	// 다음 층으로 나아간다(ADR-0032). 자리를 잡거나 못 찾으면 비워진다.
	pendingReveal string
}

// treeNode 는 트리의 항목 하나다.
//
// children 이 값 slice 가 아니라 포인터 slice 인 것이 중요하다. 자식을 다시 읽을 때 slice 가
// 재할당되는데, 값 slice 였다면 들고 있던 항목 포인터가 옛 배열을 가리키게 된다.
// editor.buffers 가 겪은 것과 같은 문제다.
type treeNode struct {
	name string
	path string // 절대 경로

	isDir        bool
	isSymlink    bool // symlink 여부
	isGitIgnored bool // ignored 는 git 이 무시하는 항목인지다. 흐리게 그린다.

	expanded bool
	children []*treeNode

	// loading 은 자식을 읽는 작업이 도는 중인지다. 도는 동안 자식 자리에 `… 읽는 중` 이 표시된다.
	loading bool

	// placeholder 는 `… 읽는 중` 처럼 파일이 아닌 안내 행인지다.
	// rows() 가 그릴 때마다 만들고 트리에는 남지 않는다 — children 은 실제 항목만 든다.
	placeholder bool
}

// openSidebar 는 root 를 뿌리로 트리를 만든다. 읽지는 않는다 —
// 뿌리를 펼치는 것도 백그라운드 작업이고, 그 Cmd 는 부르는 쪽이 발행한다(ADR-0032).
func openSidebar(root string) sidebar {
	return sidebar{
		open: true,
		root: root,
		tree: &treeNode{
			name:  filepath.Base(root),
			path:  root,
			isDir: true,
		},
	}
}

// child 는 이름이 name 인 자식이다. 없으면 nil 이다.
func (n *treeNode) child(name string) *treeNode {
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}

	return nil
}

// readDir 은 디렉터리 하나를 읽어 자식 목록을 만든다. 디렉터리가 먼저 오고 그 안은 이름순이다.
//
// os.ReadDir 이 이미 이름순으로 주므로 디렉터리와 파일로 한 번 체크한다
// 읽다가 실패해도 읽은 만큼은 쓴다. 권한이 없는 디렉터리는 빈 것으로 보이는데,
// 펼침 표시(▾)가 있으므로 정말 빈 디렉터리와 구분된다.
func readDir(dir string) []*treeNode {
	entries, _ := os.ReadDir(dir)

	dirs := []*treeNode{}
	files := []*treeNode{}
	for _, entry := range entries {
		// .git 은 어느 깊이에서든 감춘다. worktree 나 submodule 에서는 디렉터리가 아니라
		// 파일이므로 종류를 보지 않고 이름만 본다.
		if entry.Name() == ".git" {
			continue
		}

		node := &treeNode{
			name:      entry.Name(),
			path:      filepath.Join(dir, entry.Name()),
			isDir:     entry.IsDir(),
			isSymlink: entry.Type()&os.ModeSymlink != 0,
		}

		if node.isDir {
			dirs = append(dirs, node)
		} else {
			files = append(files, node)
		}
	}

	return append(dirs, files...)
}

// markIgnored 는 git 이 무시하는 항목을 표시한다.
//
// `git check-ignore` 를 부르지 않고 무시 규칙을 직접 읽어 견준다(ADR-0042). dir 이 든 저장소의
// 뿌리부터 dir 까지 층마다 `.gitignore` 를 얹으므로 값은 깊이만큼만 든다 — 트리를 훑지 않는다.
//
// 저장소가 아니면 아무것도 표시하지 않는다. 표시가 없을 뿐 틀리지는 않는다(ADR-0005).
func markIgnored(dir string, nodes []*treeNode) {
	if len(nodes) == 0 {
		return
	}

	_, root, rel, ok := openGitRepo(dir)
	if !ok {
		return
	}

	ignore := gitIgnoreAt(root, rel)

	for _, node := range nodes {
		childRel := node.name
		if rel != "" {
			childRel = rel + "/" + node.name
		}

		node.isGitIgnored = ignore.match(childRel, node.isDir)
	}
}

// treeRow 는 화면에 그릴 행 하나다. 펼쳐진 것만 위에서부터 늘어놓은 것이다.
type treeRow struct {
	depth int
	node  *treeNode
}

// rows 는 지금 보이는 행을 위에서부터 돌려준다. 렌더와 커서 이동이 모두 이 목록을 기준으로 센다.
func (s sidebar) rows() []treeRow {
	if s.tree == nil {
		return nil
	}

	return appendRows(nil, s.tree, 0)
}

// 읽는 중인 디렉터리는 자식 자리에 안내 행이 한 줄 선다. 그것이 없으면 빈 디렉터리로 읽힌다(ADR-0032).
func appendRows(rows []treeRow, node *treeNode, depth int) []treeRow {
	rows = append(rows, treeRow{depth: depth, node: node})

	if !node.expanded {
		return rows
	}

	for _, child := range node.children {
		rows = appendRows(rows, child, depth+1)
	}

	if node.loading {
		rows = append(rows, treeRow{depth: depth + 1, node: &treeNode{
			name:        "… 읽는 중",
			placeholder: true,
		}})
	}

	return rows
}

// selectedNode 는 지금 고른 항목이다. 트리가 비었으면 nil 이다.
func (s sidebar) selectedNode() *treeNode {
	rows := s.rows()
	if s.selected < 0 || s.selected >= len(rows) {
		return nil
	}

	return rows[s.selected].node
}

// selectedLabel 은 고른 항목을 statusBar 아래 줄에 보일 형태로 준다.
// project root 기준 상대 경로다.
func (s sidebar) selectedLabel() string {
	node := s.selectedNode()
	if node == nil {
		return ""
	}

	// 안내 행은 파일이 아니라 문구 자체가 뜻이다. 경로로 바꿀 것이 없다.
	if node.placeholder {
		return node.name
	}

	rel, err := filepath.Rel(s.root, node.path)
	if err != nil || rel == "." {
		return filepath.Base(s.root) + "/"
	}

	return rel
}

// scrollTo 는 고른 항목이 화면 안에 들어오도록 top 을 최소한으로 움직인다.
// 트리가 줄어들었을 수도 있으므로 selected 와 top 을 먼저 범위 안으로 당긴다.
func (s *sidebar) scrollTo(height int) {
	rows := len(s.rows())
	if rows == 0 || height < 1 {
		s.selected, s.top = 0, 0
		return
	}

	s.selected = max(0, min(s.selected, rows-1))
	s.top = max(0, min(s.top, rows-1))

	if s.selected < s.top {
		s.top = s.selected
	}
	if s.selected >= s.top+height {
		s.top = s.selected - height + 1
	}
}

// selectRow 는 sidebar 의 화면 행 y 에 있는 항목을 고른다.
// 트리가 끝난 아래 빈 행이면 아무것도 하지 않고 false 다.
//
// renderCells 는 트리가 짧아도 height 개를 채우므로 그 채움 행을 걸러야 한다.
func (s *sidebar) selectRow(y, height int) bool {
	index := s.top + y
	if index < 0 || index >= len(s.rows()) {
		return false
	}

	s.selected = index
	s.scrollTo(height)

	return true
}

// scrollBy 는 트리를 화면 행 n 개만큼 굴린다. 고른 항목은 건드리지 않는다.
//
// scrollTo 를 부르면 안 된다. 그것은 고른 항목을 화면에 넣으려고 top 을 되돌려서
// 굴린 것이 그대로 사라진다. 고른 항목이 화면 밖으로 나가면 selectedRow 가 false 를 주고
// 커서가 잠시 사라진다 — 다음 `j`/`k` 가 데려온다.
func (s *sidebar) scrollBy(n, height int) {
	rows := len(s.rows())
	if rows == 0 || height < 1 {
		return
	}

	s.top = max(0, min(s.top+n, rows-1))
}

// selectedRow 는 고른 항목이 sidebar 안에서 몇 번째 화면 행인지다.
// 화면 밖이면 ok 가 false 다.
func (s sidebar) selectedRow(height int) (int, bool) {
	row := s.selected - s.top
	if row < 0 || row >= height || s.selectedNode() == nil {
		return 0, false
	}

	return row, true
}

// labelWidth 는 sidebar 32 칸 중 이름에 쓰는 칸이다. 나머지 둘은 구분선과 그 뒤 빈 칸이다.
const labelWidth = sidebarWidth - 2

// renderCells 는 sidebar 가 차지하는 화면 행들을 돌려준다.
// 트리가 짧아도 height 개를 채우고, 한 행은 언제나 정확히 sidebarWidth 칸이다.
//
// activePath 는 지금 보고 있는 파일의 절대 경로다. 그 행만 굵게, 밑줄 그어 그린다(ADR-0022).
// 트리 커서는 터미널 커서라 포커스가 트리에 있을 때만 보이므로, 편집 중에 트리가
// 지금 자리를 나타내는 것은 이 표시뿐이다. 이름 없는 buffer 는 빈 문자열이라 어느 행과도 안 맞는다.
func (s sidebar) renderCells(height int, activePath string, box boxSet) []string {
	rows := s.rows()

	cells := make([]string, 0, max(0, height))
	for i := range height {
		index := s.top + i
		if index < 0 || index >= len(rows) {
			// 트리가 끝나도 구분선은 화면 아래까지 이어져야 한다.
			cells = append(cells, strings.Repeat(" ", labelWidth)+box.vertical+" ")
			continue
		}

		// `… 읽는 중` 은 어느 파일도 아니다. 이름 없는 buffer 는 activePath 가 빈 문자열이라
		// 그냥 두면 그 행이 "보고 있는 파일" 로 굵게 그려진다.
		node := rows[index].node
		cells = append(cells, rows[index].render(!node.placeholder && node.path == activePath, box))
	}

	return cells
}

// render 는 행 하나를 정확히 sidebarWidth 칸으로 그린다.
//
// 자르는 것이 색을 입히는 것보다 먼저다. escape 가 섞이면 폭을 셀 수 없다.
// 두 칸짜리 글자가 경계에 걸치면 truncateToWidth 가 통째로 버리므로 남는 칸을 뒤에서 채운다.
func (r treeRow) render(active bool, box boxSet) string {
	label := truncateToWidth(r.label(), labelWidth)
	pad := max(0, labelWidth-screenWidthOf(label))

	// 빈 칸은 색 밖에 둔다. 글자색만 쓰므로 어차피 보이지 않지만 escape 를 덜 낸다.
	style := r.style()
	if !active {
		return style.Render(label) + strings.Repeat(" ", pad) + box.vertical + " "
	}

	// 굵기와 밑줄은 종류별 색 위에 덧입힌다. 색은 그 파일이 무엇인지, 이 둘은 지금 보고 있는지다(ADR-0022).
	//
	// 이름에만 얹고 들여쓰기는 뗀다. 표시가 가리키는 것은 그 파일이므로 밑줄도 이름 아래에만
	// 있어야 한다 — 들여쓰기까지 이으면 깊은 자리의 파일에서 밑줄이 이름 앞의 빈 칸을 끌고 온다.
	//
	// 잘려서 이름이 남지 않으면 뒤 조각이 빈 문자열이라 앞부분만 그려진다.
	indent := min(len(r.indent()), len(label))

	return style.Render(label[:indent]) +
		style.Bold(true).Underline(true).Render(label[indent:]) +
		strings.Repeat(" ", pad) + box.vertical + " "
}

// label 은 들여쓰기와 펼침 표시가 붙은 이름이다.
func (r treeRow) label() string {
	return r.indent() + r.name()
}

// indent 는 이름 앞에 붙는 빈 칸과 펼침 표시다.
//
// 파일은 펼침 표시 자리에 빈 칸을 두어 같은 깊이의 디렉터리와 이름이 나란히 선다.
//
// 굵기·밑줄이 이름에만 얹혀야 하므로 이름과 나눠 둔다(ADR-0022).
func (r treeRow) indent() string {
	marker := "  "
	if r.node.isDir && !r.node.isSymlink {
		marker = "▸ "
		if r.node.expanded {
			marker = "▾ "
		}
	}

	return strings.Repeat("  ", r.depth) + marker
}

// name 은 트리에 찍히는 이름이다.
func (r treeRow) name() string {
	// 안내 행은 파일 이름이 아니라 문구다. `/`·`@` 를 붙이지 않는다.
	if r.node.placeholder {
		return r.node.name
	}

	name := sanitizeName(r.node.name)
	switch {
	case r.node.isSymlink:
		name += "@" // ls -F 와 같다. 따라가지 않는다는 표시다
	case r.node.isDir:
		name += "/"
	}

	return name
}

// style 은 파일 종류별 글자색이다. gitignore 된 것은 종류와 무관하게 흐리다.
func (r treeRow) style() lipgloss.Style {
	switch {
	case r.node.placeholder:
		// 안내 행은 파일 목록이 아니므로 gitignore 된 것과 같이 뒤로 물러나 있어야 한다.
		return styleTreeIgnored
	case r.node.isGitIgnored:
		return styleTreeIgnored
	case r.node.isDir:
		return styleTreeDir
	}

	switch filepath.Ext(r.node.name) {
	case ".go":
		return styleTreeGo
	case ".md", ".txt":
		return styleTreeDoc
	case ".html", ".css", ".js":
		return styleTreeWeb
	default:
		return lipgloss.NewStyle()
	}
}

// sanitizeName 은 파일 이름의 제어문자를 걷어낸다.
//
// unix 파일 이름에는 `\n` 이나 escape 가 들어갈 수 있다. `\n` 하나면 그 아래 화면이
// 통째로 밀리고, escape 는 폭 계산을 무너뜨린다. 화면에 내보내기 전에 없앤다.
func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, name)
}
