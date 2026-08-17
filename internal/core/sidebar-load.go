package core

import (
	"context"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// dirJobName 은 디렉터리 읽기 작업의 이름이자 신원이다.
//
// 경로가 들어가야 디렉터리마다 따로 돌고(같은 이름은 한 번에 하나만 돈다) 접을 때 그것만 끊는다.
// 뿌리 기준 상대 경로라 `:jobs` 에서 어디를 읽는지가 읽힌다.
func dirJobName(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		rel = filepath.Base(path)
	}

	return "디렉터리 읽기 " + rel
}

// expandNode 는 디렉터리를 펼치고 자식을 읽는 작업을 시작한다.
//
// 자식은 바로 오지 않는다. 펼침 표시(▾) 는 지금 서고 자식 자리에는 `… 읽는 중` 이 한 행 선다.
// 결과가 도착하면 조각의 apply 가 채운다(ADR-0025, ADR-0032).
//
// 펼칠 때마다 다시 읽으므로 접었다 펴는 것이 곧 새로고침이다. watcher 없이 이 정도면 충분하고,
// 화면을 그릴 때마다 syscall 을 하지 않아도 된다.
func (e *editor) expandNode(node *treeNode) tea.Cmd {
	if !node.isDir || node.isSymlink || node.loading {
		return nil
	}

	node.expanded = true
	node.loading = true
	node.children = nil

	// 노드가 아니라 경로를 넘긴다. 결과가 도착할 때 이 포인터는 이미 없을 수 있다.
	dir := node.path

	return e.startJob(dirJobName(e.sidebar.root, dir), func(ctx context.Context) <-chan jobProgress {
		return readDirJob(ctx, dir)
	})
}

// collapseNode 는 디렉터리를 접는다. 읽는 중이었으면 그 작업도 끊는다.
func (e *editor) collapseNode(node *treeNode) {
	if node.loading {
		e.cancelJob(dirJobName(e.sidebar.root, node.path))
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

		markIgnored(ctx, dir, children)

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

				node.children = children
				node.loading = false

				e.scrollSidebar()
			},
		}
	}()

	return ch
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
