package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// sidebarWidth 는 좌측 sidebar 가 차지하는 칸 수다(docs/spec.md).
const sidebarWidth = 24

// minTextWidth 는 sidebar 를 그리고도 남아 있어야 하는 편집 영역 너비다.
// 이보다 좁아지면 sidebar 가 켜져 있어도 그리지 않는다.
const minTextWidth = 20

// sidebar 는 좌측 파일 트리다.
//
// mode 가 바뀌어도 유지되어야 하므로 editor 가 값으로 들고 있다.
// tea.Model 이 아니라 평범한 struct 다 — 중첩 model 로 만들면 키가 어디서 처리되는지가
// 한 겹 숨는다. ADR-0002 가 型 교체를 고른 것은 pane 이 아니라 mode 에 대해서였다.
// 포커스가 여기 있는 상태만 mode(viewSidebar) 로 나타낸다.
//
// tree 가 포인터라서 editor 를 값으로 복사해도 트리는 공유된다.
// buffers 의 backing array 가 공유되는 것과 같고, 펼친 상태가 mode 를 넘어 남으려면 그래야 한다.
type sidebar struct {
	// open 은 사용자가 열어둔 상태인지다. 실제로 그리는지는 editor.sidebarVisible 이 정한다.
	open bool

	// root 는 트리의 뿌리다. os.Getwd() 로 채우지만 필드로 두어야 테스트가 t.TempDir() 를 넣는다.
	root string

	tree *treeNode

	// selected 는 보이는 행 중 몇 번째인지다. top 은 그중 화면 맨 위에 그릴 행이다.
	selected int
	top      int
}

// treeNode 는 트리의 항목 하나다.
//
// children 이 값 slice 가 아니라 포인터 slice 인 것이 중요하다. 자식을 다시 읽을 때 slice 가
// 재할당되는데, 값 slice 였다면 들고 있던 항목 포인터가 옛 배열을 가리키게 된다.
// editor.buffers 가 겪은 것과 같은 문제다.
type treeNode struct {
	name  string
	path  string // 절대 경로
	isDir bool

	// symlink 는 따라가지 않는다. 고리가 생길 수 있고 끊어진 링크도 있다.
	// 디렉터리를 가리키는 링크도 잎으로 두고 표시만 다르게 한다.
	symlink bool

	// ignored 는 git 이 무시하는 항목인지다. 흐리게 그린다.
	ignored bool

	expanded bool
	children []*treeNode
}

// openSidebar 는 root 를 뿌리로 트리를 열고 뿌리를 펼친 상태로 준다.
func openSidebar(root string) sidebar {
	s := sidebar{open: true, root: root}
	s.tree = &treeNode{
		name:  filepath.Base(root),
		path:  root,
		isDir: true,
	}
	s.tree.expand()

	return s
}

// expand 는 디렉터리를 펼친다. 펼칠 때마다 다시 읽으므로 접었다 펴는 것이 곧 새로고침이다.
// watcher 없이 이 정도면 충분하고, 화면을 그릴 때마다 syscall 을 하지 않아도 된다.
func (n *treeNode) expand() {
	if !n.isDir || n.symlink {
		return
	}

	n.children = readDir(n.path)
	markIgnored(n.path, n.children)
	n.expanded = true
}

// toggle 은 디렉터리를 펼치거나 접는다.
func (n *treeNode) toggle() {
	if n.expanded {
		n.expanded = false
		n.children = nil

		return
	}

	n.expand()
}

// readDir 은 디렉터리 하나를 읽어 자식 목록을 만든다. 디렉터리가 먼저 오고 그 안은 이름순이다.
//
// os.ReadDir 이 이미 이름순으로 주므로 디렉터리와 파일로 한 번 가르기만 하면 된다.
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
			name:    entry.Name(),
			path:    filepath.Join(dir, entry.Name()),
			isDir:   entry.IsDir(),
			symlink: entry.Type()&os.ModeSymlink != 0,
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
// 규칙을 직접 구현하지 않고 git 에게 묻는다. 중첩 .gitignore, `!` 부정, 전역 설정,
// .git/info/exclude 까지 전부 git 과 같은 답이 나온다.
// 디렉터리를 펼칠 때 그 디렉터리 항목을 한꺼번에 넘기므로 호출은 펼침당 한 번이다.
//
// git 이 없거나 저장소 밖이면 아무것도 표시하지 않는다. 흐린 것이 없을 뿐 틀리지는 않는다.
func markIgnored(dir string, nodes []*treeNode) {
	if len(nodes) == 0 {
		return
	}

	input := strings.Builder{}
	for _, node := range nodes {
		input.WriteString(node.path)
		input.WriteByte(0)
	}

	cmd := exec.Command("git", "check-ignore", "-z", "--stdin")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input.String())

	// 무시되는 것이 하나도 없으면 exit 1, 저장소가 아니면 exit 128 이다.
	// 둘 다 오류로 다루지 않고 출력이 있는 만큼만 읽는다.
	out, _ := cmd.Output()

	ignored := map[string]bool{}
	for _, path := range strings.Split(string(out), "\x00") {
		if path != "" {
			ignored[path] = true
		}
	}

	for _, node := range nodes {
		node.ignored = ignored[node.path]
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

func appendRows(rows []treeRow, node *treeNode, depth int) []treeRow {
	rows = append(rows, treeRow{depth: depth, node: node})

	if node.expanded {
		for _, child := range node.children {
			rows = appendRows(rows, child, depth+1)
		}
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

// selectedRow 는 고른 항목이 sidebar 안에서 몇 번째 화면 행인지다.
// 화면 밖이면 ok 가 false 다.
func (s sidebar) selectedRow(height int) (int, bool) {
	row := s.selected - s.top
	if row < 0 || row >= height || s.selectedNode() == nil {
		return 0, false
	}

	return row, true
}

// labelWidth 는 sidebar 24 칸 중 이름에 쓰는 칸이다. 나머지 둘은 구분선 `│` 과 그 뒤 빈 칸이다.
const labelWidth = sidebarWidth - 2

// 파일 종류별 글자색이다. 256 색 고정값이라 터미널 테마를 타지 않는다(ADR-0005).
var (
	styleTreeDir     = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	styleTreeGo      = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	styleTreeDoc     = lipgloss.NewStyle().Foreground(lipgloss.Color("150"))
	styleTreeWeb     = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	styleTreeIgnored = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// cells 는 sidebar 가 차지하는 화면 행들을 돌려준다.
// 트리가 짧아도 height 개를 채우고, 한 행은 언제나 정확히 sidebarWidth 칸이다.
func (s sidebar) cells(height int) []string {
	rows := s.rows()

	cells := make([]string, 0, max(0, height))
	for i := range height {
		index := s.top + i
		if index < 0 || index >= len(rows) {
			// 트리가 끝나도 구분선은 화면 아래까지 이어져야 한다.
			cells = append(cells, strings.Repeat(" ", labelWidth)+"│ ")
			continue
		}

		cells = append(cells, rows[index].cell())
	}

	return cells
}

// cell 은 행 하나를 정확히 sidebarWidth 칸으로 그린다.
//
// 자르는 것이 색을 입히는 것보다 먼저다. escape 가 섞이면 폭을 셀 수 없다.
// 두 칸짜리 글자가 경계에 걸치면 truncateToWidth 가 통째로 버리므로 남는 칸을 뒤에서 채운다.
func (r treeRow) cell() string {
	label := truncateToWidth(r.label(), labelWidth)
	pad := max(0, labelWidth-screenColAt([]byte(label), len(label)))

	// 빈 칸은 색 밖에 둔다. 글자색만 쓰므로 어차피 보이지 않지만 escape 를 덜 낸다.
	return r.style().Render(label) + strings.Repeat(" ", pad) + "│ "
}

// label 은 들여쓰기와 펼침 표시가 붙은 이름이다.
//
// 파일은 펼침 표시 자리에 빈 칸을 두어 같은 깊이의 디렉터리와 이름이 나란히 선다.
func (r treeRow) label() string {
	marker := "  "
	if r.node.isDir && !r.node.symlink {
		marker = "▸ "
		if r.node.expanded {
			marker = "▾ "
		}
	}

	name := sanitizeName(r.node.name)
	switch {
	case r.node.symlink:
		name += "@" // ls -F 와 같다. 따라가지 않는다는 표시다
	case r.node.isDir:
		name += "/"
	}

	return strings.Repeat("  ", r.depth) + marker + name
}

// style 은 파일 종류별 글자색이다. gitignore 된 것은 종류와 무관하게 흐리다.
func (r treeRow) style() lipgloss.Style {
	switch {
	case r.node.ignored:
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
