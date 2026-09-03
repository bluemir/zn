package core

import (
	"context"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// dirJobName 은 디렉터리 읽기 작업의 이름이다. 여러 디렉터리를 읽어도 이름은 이것 하나다.
const dirJobName = "디렉터리 읽기"

// dirJobArgs 는 그 작업이 어느 디렉터리를 읽는지다. 이름과 합쳐 신원이 된다(job.go).
//
// 인자가 들어가야 디렉터리마다 따로 돌고(같은 신원은 한 번에 하나만 돈다) 접을 때 그것만 끊는다.
// 뿌리 기준 상대 경로라 `:jobs` 에서 어디를 읽는지가 읽힌다.
//
// 이것을 이름에 이어 붙이던 때는 목록에서 디렉터리마다 남남이었다 — 이름 칸을 넘겨 상태·시간
// 칸을 밀었고, 끝난 목록이 이름당 하나라는 규칙이 펼친 수만큼 줄을 쌓았다(ADR-0075).
func dirJobArgs(root, path string) []string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		rel = filepath.Base(path)
	}

	return []string{rel}
}

// expandNode 는 디렉터리를 펼치고 자식을 읽는 작업을 시작한다.
//
// 자식은 바로 오지 않는다. 펼침 표시(▾) 는 지금 서고 자식 자리에는 `… 읽는 중` 이 한 행 선다.
// 결과가 도착하면 조각의 apply 가 채운다(ADR-0025, ADR-0032).
//
// **펼치는 것은 자식을 버리고 시작한다.** 접혀 있던 자리라 버릴 것이 없고, `… 읽는 중` 이
// 서려면 자식 자리가 비어 있어야 한다. 이미 펼쳐 둔 것을 다시 읽는 길은 reloadNode 다
// (ADR-0134).
func (e *editor) expandNode(node *treeNode) tea.Cmd {
	if !node.isDir || node.isSymlink || node.loading {
		return nil
	}

	node.expanded = true
	node.loading = true
	node.children = nil

	return e.startDirRead(node.path)
}

// reloadNode 는 이미 펼쳐 둔 디렉터리를 자리를 지킨 채 다시 읽는다.
//
// **펼침 표시도 자식도 건드리지 않는다.** 지금 있는 것을 그대로 두고 읽어서, 도착한 목록과
// 견줘 사라진 것만 빼고 새로 온 것만 넣는다(mergeTreeChildren). 그래서 아래 펼쳐 둔 자리가
// 남고, 보일 자식이 있는 동안은 `… 읽는 중` 도 서지 않아 트리가 움찔하지 않는다(ADR-0134).
//
// **loading 은 켠다.** 화면에 보이지 않아도 읽는 중인 것은 사실이고, 그것을 보는 자리가 둘이다
// 읽기를 겹쳐 열지 않는 것과, reveal 이 「지금 오는 중이니 도착하면 이어 걷자」로 물러나는
// 것이다(continueReveal). 끄지 않으면 이름을 바꿔 옮긴 자리를 reveal 이 먼저 포기한다.
//
// 접힌 자리는 다시 읽지 않는다. 보이지 않는 것을 읽을 값이 없고, 펼칠 때 어차피 읽는다.
func (e *editor) reloadNode(node *treeNode) tea.Cmd {
	if !node.isDir || node.isSymlink || !node.expanded || node.loading {
		return nil
	}

	node.loading = true

	return e.startDirRead(node.path)
}

// startDirRead 는 디렉터리 하나를 읽는 작업을 연다. 펼치는 길과 다시 읽는 길이 나눠 쓴다.
//
// 노드가 아니라 경로를 받는다. 결과가 도착할 때 그 포인터는 이미 없을 수 있어서, 도착한
// 자리에서 경로로 다시 찾는다(readDirJob).
func (e *editor) startDirRead(dir string) tea.Cmd {
	return e.startJob(dirJobName, dirJobArgs(e.sidebar.root, dir), func(ctx context.Context) <-chan jobProgress {
		return readDirJob(ctx, dir)
	})
}

// collapseNode 는 디렉터리를 접는다. 읽는 중이었으면 그 작업도 끊는다.
func (e *editor) collapseNode(node *treeNode) {
	if node.loading {
		e.cancelJob(dirJobName, dirJobArgs(e.sidebar.root, node.path))
	}

	node.expanded = false
	node.loading = false
	node.children = nil

	// 접은 자리 안으로 향하던 reveal 은 그만둔다. 남겨 두면 다음 조각이 도착할 때
	// 도로 펼쳐져서 방금 접은 것이 되돌아온다.
	if strings.HasPrefix(e.sidebar.pendingReveal, node.path+string(filepath.Separator)) {
		e.sidebar.pendingReveal = ""
	}
}

// refreshDir 은 그 디렉터리를 다시 읽는다. 파일을 만들거나 지운 뒤 트리를 맞추는 길이다.
//
// **접혀 있으면 펼치고, 펼쳐져 있으면 자리를 지킨 채 다시 읽는다.** 접힌 자리를 펼치는 것은
// 방금 그 안에 만든 파일을 보러 가는 자리라 그것이 맞고, 펼쳐진 자리에서 아래 펼쳐 둔 것까지
// 접을 이유는 없다(ADR-0032, ADR-0134).
//
// 트리에 아직 없는 자리(뿌리 밖, 읽지 않은 층 아래) 면 아무 일도 하지 않는다.
// 그 자리는 나중에 펼칠 때 읽으므로 지금 맞출 것이 없다.
func (e *editor) refreshDir(dir string) tea.Cmd {
	node := e.sidebar.nodeAt(dir)
	if node == nil {
		return nil
	}
	if node.expanded {
		return e.reloadNode(node)
	}

	return e.expandNode(node)
}

// toggleNode 는 디렉터리를 펼치거나 접는다. 펼치는 쪽은 작업을 시작하므로 Cmd 가 나온다.
func (e *editor) toggleNode(node *treeNode) tea.Cmd {
	if node.expanded {
		e.collapseNode(node)

		return nil
	}

	return e.expandNode(node)
}

// readDirJob 은 디렉터리 하나를 끝까지 읽는 작업이다.
//
// 조각은 하나뿐이다 — 이름순으로 놓으려면 어차피 다 읽어야 해서 나눠 보낼 것이 없다.
// 다 읽은 뒤 항목을 만드는 것은 slice 하나를 넘기는 일이라 조각을 쪼개도 빨라지지 않는다.
// gitignore 표시도 같은 자리에서 묻는다. 둘 다 펼치는 길에 있던 동기 호출이었다.
func readDirJob(ctx context.Context, dir string) <-chan jobProgress {
	ch := make(chan jobProgress, 1)

	go func() {
		defer close(ch)

		children := readDir(dir)
		if ctx.Err() != nil {
			return
		}

		markIgnored(dir, children)

		// 끊긴 뒤에 채우면 방금 접은 디렉터리가 도로 펼쳐진다. 취소했다는 것은 실행기가
		// 이미 적어 두었다(job.go).
		if ctx.Err() != nil {
			return
		}

		ch <- jobProgress{
			done:    1,
			total:   1,
			summary: formatCount(len(children)) + " 개",
			apply: func(e *editor) {
				// 노드는 포인터로 잡아두지 않고 경로로 다시 찾는다. 그 사이 접혔거나
				// `:tree` 로 트리를 새로 열어서 그 포인터가 아무 화면에도 없을 수 있다.
				node := e.sidebar.nodeAt(dir)
				if node == nil || !node.expanded {
					return
				}

				// **고른 것은 행 번호가 아니라 그 파일이다.** 위쪽에 무엇이 생기면 행 번호가
				// 한 칸씩 밀리는데, 그때 고른 자리를 번호로 두면 남의 파일로 미끄러진다.
				// 아무 키도 누르지 않았는데 미끄러지고, 이어 치는 `md` 가 엉뚱한 것을
				// 지운다(ADR-0134).
				selected := ""
				if node := e.sidebar.selectedNode(); node != nil {
					selected = node.path
				}

				node.children = mergeTreeChildren(node.children, children)
				node.loading = false

				e.sidebar.selectByPath(selected)
				e.scrollSidebar()
			},
		}
	}()

	return ch
}

// mergeTreeChildren 은 다시 읽어온 목록에 지금 펼쳐 둔 상태를 물려준다.
//
// **목록은 새것이 정하고 펼침은 옛것이 정한다.** 사라진 것은 없어지고 새로 온 것은 접힌 채로
// 서고, 이름이 같은 디렉터리는 펼침과 그 아래 자식을 그대로 들고 있는다. 그래서 뿌리를 다시
// 읽어도 세 층 아래 펼쳐 둔 자리가 남는다(ADR-0134).
//
// 읽는 중(loading) 도 같이 물려준다. 그 작업의 결과는 경로로 노드를 다시 찾아 들어오므로
// 여기서 바꿔 놓은 새 노드에 제대로 앉는다 — 표시만 떨구면 자식은 오는데 `… 읽는 중` 이
// 먼저 사라진다.
//
// 처음 펼치는 길에서는 옛것이 비어 있어(expandNode 가 지운다) 새것이 그대로 남는다.
func mergeTreeChildren(was, now []*treeNode) []*treeNode {
	for _, node := range now {
		if !node.isDir || node.isSymlink {
			continue
		}

		old := treeChild(was, node.name)
		// 파일이 있던 자리에 같은 이름의 디렉터리가 생겼으면 물려받을 펼침이 없다.
		if old == nil || !old.isDir || old.isSymlink {
			continue
		}

		node.expanded = old.expanded
		node.children = old.children
		node.loading = old.loading
	}

	return now
}

// treeChild 는 목록에서 그 이름을 찾는다. treeNode.child 와 같은 일을 노드 없이 한다 —
// 병합은 아직 어느 노드에도 붙지 않은 목록 둘을 견주는 자리다.
func treeChild(nodes []*treeNode, name string) *treeNode {
	for _, node := range nodes {
		if node.name == name {
			return node
		}
	}

	return nil
}

// reloadTree 는 뿌리부터 펼쳐 둔 디렉터리를 전부 다시 읽는다. 트리의 `R` 이다.
//
// 접힌 자리는 건드리지 않는다. NERDTree 의 `R` 과 같은 자리이고, 펼친 것만 보는 것이라
// 값이 화면에 보이는 만큼만 든다(ADR-0134).
//
// 디렉터리마다 작업이 따로 열린다. 신원에 경로가 들어 있어서(dirJobArgs) 나란히 돌고,
// 도착 차례는 보지 않는다 — 각 조각이 경로로 자기 노드를 찾아 앉는다.
func (e *editor) reloadTree() tea.Cmd {
	if e.sidebar.tree == nil {
		return nil
	}

	nodes := appendExpandedDirs(nil, e.sidebar.tree)

	cmds := make([]tea.Cmd, 0, len(nodes))
	for _, node := range nodes {
		cmds = append(cmds, e.reloadNode(node))
	}

	return tea.Batch(cmds...)
}

// appendExpandedDirs 는 펼쳐진 디렉터리를 위에서부터 모은다. appendRows 와 같은 모양이다.
func appendExpandedDirs(nodes []*treeNode, node *treeNode) []*treeNode {
	if !node.expanded {
		return nodes
	}

	nodes = append(nodes, node)

	for _, child := range node.children {
		nodes = appendExpandedDirs(nodes, child)
	}

	return nodes
}

// nodeAt 은 그 경로의 항목을 트리에서 찾는다. 없으면 nil 이다.
// 찾기만 하고 펼치지는 않는다 — 이미 읽어 둔 자리만 본다.
func (s sidebar) nodeAt(path string) *treeNode {
	if s.tree == nil {
		return nil
	}

	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return nil
	}
	if rel == "." {
		return s.tree
	}

	node := s.tree
	for _, name := range strings.Split(rel, string(filepath.Separator)) {
		node = node.child(name)
		if node == nil {
			return nil
		}
	}

	return node
}

// setRevealTarget 은 트리가 향할 자리를 세운다. 갈 수 없는 자리면 세우지 않고 false 다.
//
// 뿌리 밖의 파일(트리는 cwd 가 뿌리다) 과 이름 없는 buffer(빈 경로) 가 그렇다.
func (s *sidebar) setRevealTarget(path string) bool {
	if s.tree == nil || path == "" {
		return false
	}

	// CLI 로 연 파일은 상대 경로이고 트리는 절대 경로다. tabOf 와 같은 이유로 맞춰 둔다.
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	s.pendingReveal = abs

	return true
}

// continueReveal 은 pendingReveal 을 향해 갈 수 있는 만큼 걸어간다.
// 보는 파일이 바뀌어도 트리가 따라오게 하는 길이다(ADR-0019).
//
// 아직 읽지 않은 디렉터리를 만나면 그것을 펼치는 작업을 시작하고 물러난다. 자식이 도착하면
// handleJob 이 여기를 다시 불러서 다음 층으로 나아간다 — 그렇게 한 층씩 내려간다(ADR-0032).
//
// 이미 펼쳐진 디렉터리는 다시 읽지 않는다. 다시 읽으면 그 아래 펼쳐 둔 것이 통째로 접힌다.
// 새로고침은 접었다 펴는 것이고 파일을 여는 것이 아니다.
//
// 도중에 항목을 찾지 못하면 조용히 그만둔다. 그 사이 지워진 파일과 symlink 디렉터리 안쪽이
// 그렇다. 그때까지 펼친 것은 되돌리지 않는다 — 펼친 것 자체는 틀린 상태가 아니다.
func (e *editor) continueReveal() tea.Cmd {
	if e.sidebar.pendingReveal == "" || e.sidebar.tree == nil {
		return nil
	}

	rel, err := filepath.Rel(e.sidebar.root, e.sidebar.pendingReveal)
	if err != nil {
		e.sidebar.pendingReveal = ""

		return nil
	}

	node := e.sidebar.tree
	for _, name := range strings.Split(rel, string(filepath.Separator)) {
		// 자식이 오는 중이다. 도착하면 이 자리부터 다시 걷는다.
		if node.loading {
			return nil
		}
		if !node.expanded {
			return e.expandNode(node)
		}

		child := node.child(name)
		if child == nil {
			e.sidebar.pendingReveal = ""

			return nil
		}

		node = child
	}

	e.sidebar.pendingReveal = ""

	// 고른 자리는 보이는 행 중 몇 번째인지로 들고 있으므로 펼친 뒤에 다시 센다.
	for i, row := range e.sidebar.rows() {
		if row.node == node {
			e.sidebar.selected = i
			e.scrollSidebar()

			break
		}
	}

	return nil
}

// startTree 는 트리의 첫 읽기를 시작한다.
//
// core.Run 은 Program 이 뜨기 전이라 Cmd 를 낼 자리가 없다. 그래서 normal mode 의 Init 이
// 이것을 낸다 — git 첫 갱신과 같은 자리다(ADR-0030).
func (e *editor) startTree() tea.Cmd {
	if e.sidebar.tree == nil {
		return nil
	}

	// 갈 자리가 정해져 있으면 그쪽이 뿌리부터 필요한 만큼 펼친다.
	if e.sidebar.pendingReveal != "" {
		return e.continueReveal()
	}

	return e.expandNode(e.sidebar.tree)
}
