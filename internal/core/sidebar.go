package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
