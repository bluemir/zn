package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTreeFixture 는 트리 시험용 디렉터리를 만든다.
//
//	root/
//	  .git/          (감춰져야 한다)
//	  .gitignore     (숨김 파일이지만 보여야 한다)
//	  build/
//	    out
//	  docs/
//	    spec.md
//	  main.go
//	  README.md
func newTreeFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{".git", "build", "docs"} {
		require.NoError(t, os.Mkdir(filepath.Join(root, dir), 0755))
	}
	for _, file := range []string{".gitignore", "main.go", "README.md", "build/out", "docs/spec.md", ".git/HEAD"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte("x\n"), 0644))
	}

	return root
}

// openSidebarSync 는 뿌리를 다 읽어 둔 트리다.
//
// 편집기는 디렉터리 읽기를 백그라운드 작업으로 돌리지만(ADR-0032) 트리 자체를 보는 시험이
// 작업 실행기를 거칠 이유는 없다. 작업이 부르는 것과 같은 함수를 그 자리에서 부른다.
func openSidebarSync(t *testing.T, root string) sidebar {
	t.Helper()

	s := openSidebar(root)
	loadNodeSync(t, s.tree)

	return s
}

// loadNodeSync 는 디렉터리 하나를 그 자리에서 읽어 채운다.
func loadNodeSync(t *testing.T, node *treeNode) {
	t.Helper()

	require.True(t, node.isDir, "%s 는 디렉터리가 아니다", node.name)

	node.children = readDir(node.path)
	markIgnored(node.path, node.children)
	node.expanded = true
	node.loading = false
}

// settle 은 Cmd 가 낸 msg 를 model 에 다시 먹이는 것을 더 나올 것이 없을 때까지 되풀이한다.
// 백그라운드 작업이 끝나고 그 결과가 화면 상태에 들어온 뒤를 보는 시험이 쓴다.
//
// `tea.Batch` 는 msg 하나에 Cmd 여럿을 실어 오므로 풀어서 차례로 돌린다.
// Init 은 넘기지 않는다 — 5 초짜리 git tick 을 물고 있어서 그 자리에서 멈춘다.
//
// 작업이 끝나며 나오는 Cmd 도 같은 이유로 물지 않는다. 주기 작업은 끝나는 자리에서 cooldown
// 을 걸므로(ADR-0043, ADR-0044) 그것을 부르면 5 초를 기다린다 — 시험이 보려는 것은 결과다.
func settle(t *testing.T, model tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()

	queue := []tea.Cmd{cmd}
	for step := 0; len(queue) > 0; step++ {
		require.Less(t, step, 100, "작업이 끝나지 않는다")

		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}

		switch msg := next().(type) {
		case nil:
			// msg 를 내지 않는 Cmd 다.
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			var cmd tea.Cmd
			model, cmd = model.Update(msg)

			if _, done := msg.(jobDoneMsg); done {
				continue
			}

			// 서버가 떴으면 거기서 멈춘다. 그 자리에서 나오는 Cmd 에 진단을 기다리는 것이
			// 들어 있고, 그것은 종이 울릴 때까지 막힌다(diagnostics.go 의 waitDiagnostics).
			// bubbletea 는 Cmd 를 goroutine 에서 돌려 막히지 않는데 여기서는 이 goroutine 이
			// 그대로 돈다. jobDoneMsg 에서 고리를 끊는 것과 같은 자리다(ADR-0086).
			if _, ready := msg.(serverReadyMsg); ready {
				continue
			}

			queue = append(queue, cmd)
		}
	}

	return model
}

// toggleSync 는 디렉터리를 그 자리에서 여닫는다. expandNode·collapseNode 와 같은 일이다.
func toggleSync(t *testing.T, node *treeNode) {
	t.Helper()

	if node.expanded {
		node.expanded = false
		node.children = nil

		return
	}

	// 파일과 symlink 는 펼치지 않는다. 편집기의 expandNode 와 같다.
	if !node.isDir || node.isSymlink {
		return
	}

	loadNodeSync(t, node)
}

// revealSyncIn 은 편집기 안의 트리를 그 파일 자리로 데려가고 가는 길의 읽기가 끝나기를 기다린다.
// 실제 경로(revealInSidebar → 작업 → handleJob → 다음 층) 를 그대로 지나간다(ADR-0032).
func revealSyncIn(t *testing.T, e *editor, path string) {
	t.Helper()

	settle(t, viewEditorNormal{editor: e}, e.revealInSidebar(path))
}

// revealSync 는 편집기 없이 트리만 든 시험용이다. 트리 하나를 임시 편집기에 얹어 데려간다.
func revealSync(t *testing.T, s sidebar, path string) sidebar {
	t.Helper()

	e := &editor{
		buffers: []viewport{newEmptyBuffer("")},
		sidebar: s,
		width:   80,
		height:  10 + tablineHeight + statusBarHeight,
	}
	revealSyncIn(t, e, path)

	return e.sidebar
}

// sendSync 는 키를 먹이고 그 키가 낸 작업까지 돌린다. send 는 Cmd 를 버린다.
func sendSync(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()

	for _, k := range keys {
		next, cmd := m.Update(key(k))
		m = settle(t, next, cmd)
	}

	return m
}

// names 는 행 목록을 "깊이:이름" 으로 펴서 비교하기 쉽게 만든다.
func names(rows []treeRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, string(rune('0'+row.depth))+":"+row.node.name)
	}

	return out
}

func TestSidebarOpensWithRootExpanded(t *testing.T) {
	root := newTreeFixture(t)

	s := openSidebarSync(t, root)

	require.True(t, s.open)
	require.NotNil(t, s.tree)
	assert.Equal(t, filepath.Base(root), s.tree.name)
	assert.True(t, s.tree.expanded, "뿌리는 펼친 채로 시작한다")
}

// 디렉터리가 먼저, 그 안은 이름순이다. os.ReadDir 이 이미 이름순으로 준다.
func TestSidebarSortsDirsFirst(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	assert.Equal(t, []string{
		"1:build", "1:docs",
		"1:.gitignore", "1:README.md", "1:main.go",
	}, names(s.rows())[1:], "디렉터리 먼저, 그 안은 이름순")
}

// .git 은 감추고 나머지 숨김 파일은 보인다.
func TestSidebarHidesOnlyDotGit(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	shown := names(s.rows())
	assert.NotContains(t, shown, "1:.git")
	assert.Contains(t, shown, "1:.gitignore", "숨김 파일이어도 고치는 파일은 보인다")
}

// worktree·submodule 에서는 .git 이 디렉터리가 아니라 파일이다. 종류를 보지 않고 이름으로 거른다.
func TestSidebarHidesDotGitFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x\n"), 0644))

	s := openSidebarSync(t, root)

	assert.Equal(t, []string{"1:main.go"}, names(s.rows())[1:])
}

func TestSidebarExpandAndCollapse(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	docs := s.rows()[2].node
	require.Equal(t, "docs", docs.name)
	require.False(t, docs.expanded)

	toggleSync(t, docs)

	assert.Contains(t, names(s.rows()), "2:spec.md", "펼치면 자식이 목록에 들어온다")

	toggleSync(t, docs)

	assert.NotContains(t, names(s.rows()), "2:spec.md")
}

// 접었다 펴는 것이 곧 새로고침이다. watcher 없이 이걸로 충분하다.
func TestSidebarRereadsOnExpand(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebarSync(t, root)

	docs := s.rows()[2].node
	toggleSync(t, docs)
	require.Contains(t, names(s.rows()), "2:spec.md")

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "new.md"), []byte("x\n"), 0644))
	toggleSync(t, docs)
	toggleSync(t, docs)

	assert.Contains(t, names(s.rows()), "2:new.md", "다시 펼치면 새 파일이 보인다")
}

// symlink 는 따라가지 않는다. 디렉터리를 가리켜도 잎이다.
func TestSidebarDoesNotFollowSymlink(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")))

	s := openSidebarSync(t, root)

	var link *treeNode
	for _, row := range s.rows() {
		if row.node.name == "link" {
			link = row.node
		}
	}
	require.NotNil(t, link)

	assert.True(t, link.isSymlink)
	assert.False(t, link.isDir, "디렉터리를 가리켜도 잎으로 둔다")

	toggleSync(t, link)
	assert.False(t, link.expanded, "펼쳐지지 않는다")
}

// 끊어진 symlink 도 죽지 않아야 한다.
func TestSidebarHandlesBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(root, "nope"), filepath.Join(root, "dangling")))

	assert.NotPanics(t, func() {
		s := openSidebarSync(t, root)
		assert.Len(t, s.rows(), 2)
	})
}

// 권한이 없는 디렉터리는 빈 것으로 보인다. 펼침 표시가 있으므로 정말 빈 것과 구분된다.
func TestSidebarUnreadableDirIsEmpty(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(locked, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "secret"), []byte("x\n"), 0644))
	require.NoError(t, os.Chmod(locked, 0000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })

	s := openSidebarSync(t, root)
	node := s.rows()[1].node
	require.Equal(t, "locked", node.name)

	assert.NotPanics(t, func() { toggleSync(t, node) })
	assert.True(t, node.expanded)
	assert.Empty(t, node.children)
}

func TestSidebarEmptyDir(t *testing.T) {
	s := openSidebarSync(t, t.TempDir())

	assert.Len(t, s.rows(), 1, "뿌리 한 줄만")
	assert.Equal(t, s.tree, s.selectedNode())
}

func TestSidebarSelectedNode(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	assert.Equal(t, s.tree, s.selectedNode(), "처음에는 뿌리")

	s.selected = 3
	require.NotNil(t, s.selectedNode())
	assert.Equal(t, ".gitignore", s.selectedNode().name)

	s.selected = 999
	assert.Nil(t, s.selectedNode(), "범위를 벗어나면 nil")
}

// git 이 무시하는 항목은 표시된다. 규칙은 git 에게 묻는다.
func TestSidebarMarksGitIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root := newTreeFixture(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n"), 0644))
	require.NoError(t, exec.Command("git", "-C", root, "init").Run())

	s := openSidebarSync(t, root)

	byName := map[string]*treeNode{}
	for _, row := range s.rows() {
		byName[row.node.name] = row.node
	}

	assert.True(t, byName["build"].isGitIgnored, "gitignore 된 디렉터리")
	assert.False(t, byName["main.go"].isGitIgnored)
	assert.False(t, byName["README.md"].isGitIgnored)
}

// 저장소가 아니면 아무것도 흐리게 하지 않는다. 흐린 것이 없을 뿐 틀리지 않는다.
func TestSidebarNoRepoMarksNothing(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	for _, row := range s.rows() {
		assert.False(t, row.node.isGitIgnored, "%s", row.node.name)
	}
}

// newTreeEditor 는 sidebar 를 연 편집기다. 뿌리는 t.TempDir() 라 cwd 를 건드리지 않는다.
func newTreeEditor(t *testing.T, width, height int) viewEditorNormal {
	t.Helper()

	m := viewEditorNormal{
		editor: &editor{
			boxChars: boxUnicode,
			buffers:  []viewport{newBuffer("main.go", []byte("a\nb\n"))},
			width:    width,
			height:   height + tablineHeight + statusBarHeight,
		},
	}
	m.sidebar = openSidebarSync(t, newTreeFixture(t))
	m.sidebar.scrollTo(m.sidebarHeight())

	return m
}

// sidebarCellsOf 는 화면에서 sidebar 가 차지하는 왼쪽 칸만 떼어낸다.
// sidebar 는 tabline 옆줄부터 statusBar 앞줄까지라 아래 두 줄만 뗀다.
func sidebarCellsOf(t *testing.T, view tea.View) []string {
	t.Helper()

	rows := strings.Split(view.Content, "\n")
	require.Greater(t, len(rows), statusBarHeight)

	out := []string{}
	for _, row := range rows[:len(rows)-statusBarHeight] {
		plain := []byte(ansi.Strip(row))
		out = append(out, string(plain[:offsetAtScreenCol(plain, sidebarWidth, defaultTabWidth)]))
	}

	return out
}

// sidebar 는 한 행이 정확히 sidebarWidth 칸이어야 한다. 어긋나면 편집 내용이 통째로 밀린다.
func TestSidebarCellsAreExactlyWide(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	for i, cell := range s.renderCells(10, "", nil, boxUnicode) {
		plain := ansi.Strip(cell)
		assert.Equal(t, sidebarWidth, widthOf(plain), "행 %d: %q", i, plain)
	}
}

// 트리가 화면보다 짧아도 구분선은 화면 아래까지 이어져야 한다.
func TestSidebarCellsFillHeight(t *testing.T) {
	s := openSidebarSync(t, t.TempDir())

	cells := s.renderCells(6, "", nil, boxUnicode)

	require.Len(t, cells, 6)
	for _, cell := range cells[1:] {
		assert.Equal(t, strings.Repeat(" ", labelWidth+1)+"│ ", ansi.Strip(cell))
	}
}

// 한글 파일 이름이 경계에 걸쳐도 칸이 어긋나면 안 된다.
func TestSidebarCellsWithWideChars(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "아주아주긴한글파일이름입니다.md"), []byte("x\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "이모지🇰🇷파일.txt"), []byte("x\n"), 0644))

	s := openSidebarSync(t, root)

	for i, cell := range s.renderCells(4, "", nil, boxUnicode) {
		plain := ansi.Strip(cell)
		assert.Equal(t, sidebarWidth, widthOf(plain), "행 %d: %q", i, plain)
	}
}

// 파일 이름의 제어문자는 화면에 나가면 안 된다. \n 하나면 그 아래가 통째로 밀린다.
func TestSidebarSanitizesControlChars(t *testing.T) {
	assert.Equal(t, "a?b", sanitizeName("a\nb"))
	assert.Equal(t, "a?b", sanitizeName("a\tb"))
	assert.Equal(t, "?[7mfake", sanitizeName("\x1b[7mfake"))
	assert.Equal(t, "보통이름.go", sanitizeName("보통이름.go"))
}

func TestSidebarLabels(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")))

	s := openSidebarSync(t, root)
	toggleSync(t, s.rows()[1].node) // build/ 를 펼친다

	labels := []string{}
	for _, row := range s.rows() {
		labels = append(labels, row.label())
	}

	assert.Contains(t, labels, "  ▾ build/")
	assert.Contains(t, labels, "      out", "깊이 2 는 들여쓰기 4 칸 + 표시 자리 2 칸")
	assert.Contains(t, labels, "  ▸ docs/")
	assert.Contains(t, labels, "    main.go")
	assert.Contains(t, labels, "    link@", "symlink 은 따라가지 않는다는 표시")
}

// 트리가 화면보다 길면 고른 항목을 화면 안에 유지한다.
func TestSidebarScrollKeepsSelectionVisible(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0644))
	}

	s := openSidebarSync(t, root)
	require.Len(t, s.rows(), 8)

	s.selected = 7
	s.scrollTo(3)
	assert.Equal(t, 5, s.top, "아래로 벗어나면 최소한만 민다")
	row, ok := s.selectedRow(3)
	require.True(t, ok)
	assert.Equal(t, 2, row)

	s.selected = 1
	s.scrollTo(3)
	assert.Equal(t, 1, s.top, "위로 벗어나면 고른 것이 맨 위")
}

// 트리가 줄어들면 selected 와 top 을 범위 안으로 당긴다.
func TestSidebarScrollClampsAfterCollapse(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))
	docs := s.rows()[2].node
	toggleSync(t, docs)

	s.selected = len(s.rows()) - 1
	s.scrollTo(10)

	toggleSync(t, docs)
	s.scrollTo(10)

	assert.Less(t, s.selected, len(s.rows()))
	assert.NotNil(t, s.selectedNode())
}

// sidebar 가 열리면 편집 영역이 좁아지고 커서가 그만큼 오른쪽으로 간다.
func TestSidebarShiftsTextAndCursor(t *testing.T) {
	m := newTreeEditor(t, 80, 5)

	assert.Equal(t, 80-sidebarWidth, m.textWidth())
	assert.Equal(t, sidebarWidth, m.sidebarLeft())
	assert.Equal(t, tea.Position{X: sidebarWidth + m.gutterWidth(), Y: tablineHeight}, m.View().Cursor.Position,
		"sidebar 와 줄번호 칸을 지난 자리다")
}

// 편집 내용이 짧아도 sidebar 는 tabline 옆줄부터 statusBar 앞줄까지 이어져야 한다.
func TestSidebarRendersFullHeightBesideShortFile(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	cells := sidebarCellsOf(t, m.View())

	require.Len(t, cells, m.sidebarHeight())
	for i, cell := range cells {
		assert.Equal(t, sidebarWidth, widthOf(cell), "행 %d", i)
	}
	assert.Contains(t, cells[0], "▾ ", "뿌리가 tabline 옆줄에 온다")
	assert.Contains(t, cells[len(cells)-1], "│", "파일은 2 줄뿐이지만 구분선은 statusBar 앞까지 간다")
}

// tabline 은 sidebar 옆에서 끊기고, statusBar 는 sidebar 아래까지 이어진다.
// 아래까지 이어지는 자리에는 트리가 아니라 mode 가 온다.
func TestTablineStopsAtSidebarButStatusBarRunsUnder(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	rows := strings.Split(m.View().Content, "\n")

	// split 은 한 행을 sidebar 왼쪽 칸과 그 오른쪽으로 가른다.
	split := func(row string) (string, string) {
		plain := []byte(ansi.Strip(row))
		cut := offsetAtScreenCol(plain, sidebarWidth, defaultTabWidth)

		return string(plain[:cut]), string(plain[cut:])
	}

	cell, tabline := split(rows[0])
	assert.Contains(t, cell, "▾ ", "맨 윗줄 왼쪽은 트리다")
	assert.Contains(t, tabline, "1 main.go", "tab 목록은 그 오른쪽에 있다")

	left, path := split(rows[len(rows)-statusBarHeight])
	assert.NotContains(t, left, "▾", "statusBar 아래에는 트리가 없다")
	assert.Equal(t, "NORMAL", strings.TrimSpace(left), "그 자리는 mode 다")
	assert.True(t, strings.HasPrefix(path, "main.go"), "경로는 편집 영역 왼쪽 끝에서 시작한다: %q", path)

	left, command := split(rows[len(rows)-1])
	assert.Equal(t, strings.Repeat(" ", sidebarWidth), left, "명령줄도 마찬가지다")
	assert.True(t, strings.HasPrefix(command, "1:1"), "커서 위치가 같은 자리에서 시작한다: %q", command)

	// 반전으로 칠하는 두 줄은 화면 끝까지 이어져야 한다.
	// statusBar 는 sidebar 아래까지 한 덩어리라 왼쪽 끝부터 칠해진다.
	for _, i := range []int{0, len(rows) - statusBarHeight} {
		plain := ansi.Strip(rows[i])
		assert.Equal(t, 80, widthOf(plain), "행 %d", i)
	}
	assert.True(t, strings.HasPrefix(rows[len(rows)-statusBarHeight], "\x1b["),
		"sidebar 아래 빈 칸도 반전 안에 있어야 색이 끊기지 않는다")
}

// 화면이 좁으면 sidebar 를 켜뒀어도 그리지 않는다. 안 그러면 편집할 자리가 없다.
func TestSidebarAutoHidesOnNarrowScreen(t *testing.T) {
	m := newTreeEditor(t, sidebarWidth+minTextWidth, 5)
	require.True(t, m.sidebarVisible())

	m.width = sidebarWidth + minTextWidth - 1

	assert.True(t, m.sidebar.open, "사용자 의도는 그대로다")
	assert.False(t, m.sidebarVisible(), "그리지는 않는다")
	assert.Equal(t, m.width, m.textWidth())
	assert.Equal(t, 0, m.sidebarLeft())
}

// 아주 좁거나 낮은 화면에서도 죽지 않아야 한다. 음수 폭이 여기서 잡힌다.
func TestSidebarTinyScreenDoesNotPanic(t *testing.T) {
	// 임계값 언저리를 상수로 잡는다. 숫자를 박아두면 너비를 바꿀 때 조용히 낡는다.
	widths := []int{0, 1, 10, sidebarWidth - 1, sidebarWidth, sidebarWidth + 1, sidebarWidth + minTextWidth}
	for _, width := range widths {
		for _, height := range []int{0, 1, 3, 5} {
			m := newTreeEditor(t, width, 0)
			m.height = height

			assert.NotPanics(t, func() { m.View() }, "width=%d height=%d", width, height)
		}
	}
}

// ctrl+w ctrl+w 와 ctrl+w w 로 편집 영역과 트리를 오간다. pane 이 둘뿐이라 순환이 곧 왕래다.
func TestSidebarFocusCycles(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)

	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, m)

	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewEditorNormal{}, m)

	m = send(m, "ctrl+w", "w")
	assert.IsType(t, viewSidebar{}, m, "ctrl+w w 도 같다")
}

func TestSidebarEscapeLeaves(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)

	m = send(m, "ctrl+w", "ctrl+w", "esc")

	assert.IsType(t, viewEditorNormal{}, m)
}

// 트리가 닫혀 있으면 `ctrl+w w` 는 편집 영역에 머물면서 왜 못 갔는지 알린다.
// 아무 일도 안 나면 키가 먹었는지 알 수 없다.
func TestCtrlWTellsWhenTreeClosed(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 5)

	m = send(m, "ctrl+w", "w")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "트리가 닫혀 있습니다")
}

// 화면이 좁아 트리를 감춘 동안에도 같다. 알리는 문구만 다르다 —
// 닫아 둔 것과 화면이 좁은 것은 사용자가 할 일이 다르다.
func TestCtrlWTellsWhenSidebarHidden(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m = send(m, "ctrl+w", "w")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "화면이 좁아")
}

// 접두 키를 기다리는 동안의 esc 는 접두 키만 무른다. sidebar 를 나가면 안 된다.
func TestSidebarPendingEscapeStays(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)

	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, m)

	m = send(m, "ctrl+w", "esc")

	assert.IsType(t, viewSidebar{}, m)
}

func TestSidebarMovesSelection(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	m = send(m, "down", "down")
	assert.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name)

	m = send(m, "up")
	assert.Equal(t, "build", m.(viewSidebar).sidebar.selectedNode().name)

	// 맨 위에서 더 올라가지 않는다.
	m = send(m, "up", "up", "up")
	assert.Equal(t, 0, m.(viewSidebar).sidebar.selected)
}

// j k 는 ↓ ↑ 와 같은 트리 이동이다. sidebar 에는 삽입이 없으므로 글자 키를 이동에 쓸 수 있다.
func TestSidebarMovesSelectionWithJK(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	m = send(m, "j", "j")
	assert.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name)

	m = send(m, "k")
	assert.Equal(t, "build", m.(viewSidebar).sidebar.selectedNode().name)

	m = send(m, "k", "k", "k")
	assert.Equal(t, 0, m.(viewSidebar).sidebar.selected)
}

// 숫자를 대면 그만큼 뛴다. `20j` 는 트리를 넘어가지 않고 마지막 행에 선다(ADR-0059).
func TestSidebarCountMovesManyRows(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	// 뿌리와 그 아래 다섯이라 여섯 행이다.
	require.Len(t, m.(viewSidebar).sidebar.rows(), 6)

	m = send(m, "3", "j")
	assert.Equal(t, ".gitignore", m.(viewSidebar).sidebar.selectedNode().name)

	// 넘어가지 않는다. 양끝에서 멈추는 것이 한 행씩 갈 때와 같다.
	m = send(m, "2", "0", "j")
	assert.Equal(t, 5, m.(viewSidebar).sidebar.selected, "마지막 행에 선다")

	m = send(m, "2", "0", "k")
	assert.Equal(t, 0, m.(viewSidebar).sidebar.selected, "뿌리에 선다")
}

// `G` 는 마지막 행, `gg` 는 뿌리다. 숫자를 대면 둘 다 그 행으로 간다 — normal 과 같다.
func TestSidebarGotoFirstAndLastRow(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	m = send(m, "G")
	assert.Equal(t, "main.go", m.(viewSidebar).sidebar.selectedNode().name)

	m = send(m, "g", "g")
	assert.Equal(t, 0, m.(viewSidebar).sidebar.selected, "뿌리로 돌아온다")

	// 행은 1 부터 센다. 셋째 행이 `docs` 다.
	m = send(m, "3", "G")
	assert.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name)

	m = send(m, "g", "g")
	m = send(m, "3", "g", "g")
	assert.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name, "`20gg` 도 행 번호다")

	// 트리보다 큰 번호는 마지막 행이다. `G` 가 숫자 없이 가는 자리와 같다.
	m = send(m, "9", "9", "G")
	assert.Equal(t, 5, m.(viewSidebar).sidebar.selected)
}

// 뛴 자리가 화면 밖이면 트리가 따라 굴러야 한다. 한 행씩 갈 때와 같은 scrollTo 다.
func TestSidebarCountScrollsIntoView(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 3)
	m = send(m, "ctrl+w", "ctrl+w")

	// 트리는 tabline 옆줄까지 쓰므로 편집 영역보다 한 줄 높다(layout.go).
	require.Equal(t, 4, m.(viewSidebar).sidebarHeight())
	require.Len(t, m.(viewSidebar).sidebar.rows(), 6)

	m = send(m, "G")

	tree := m.(viewSidebar).sidebar
	assert.Equal(t, 5, tree.selected)
	assert.Equal(t, 2, tree.top, "마지막 행이 화면 맨 아래에 들어온다")

	m = send(m, "g", "g")
	assert.Equal(t, 0, m.(viewSidebar).sidebar.top, "처음으로 돌아오면 화면도 위로 온다")
}

// 숫자를 보는 것은 이동뿐이다. 파일 동작 앞의 숫자는 버려진다 — `3md` 가 `md` 다.
func TestSidebarCountIgnoredByFileAction(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = sendSync(t, model, "3", "m", "d")

	require.IsType(t, viewSidebar{}, model)
	_, err := os.Stat(filepath.Join(root, "README.md"))
	assert.Error(t, err, "숫자가 붙어도 그 한 파일을 지운다")
}

func TestSidebarEnterTogglesDirectory(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 10)
	m = send(m, "ctrl+w", "ctrl+w", "down", "down")
	require.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name)

	// 펼치기는 작업이라 그것이 끝난 뒤에 자식이 찬다.
	m = sendSync(t, m, "enter")
	assert.Contains(t, names(m.(viewSidebar).sidebar.rows()), "2:spec.md")

	m = sendSync(t, m, "enter")
	assert.NotContains(t, names(m.(viewSidebar).sidebar.rows()), "2:spec.md")
}

// 명령줄도 편집 영역 아래에 있으므로 커서가 sidebar 만큼 오른쪽에서 시작해야 한다.
func TestSidebarShiftsCommandLineCursor(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)

	m = send(m, ":", "w")

	require.IsType(t, viewEditorCommand{}, m)
	assert.Equal(t, sidebarWidth+len(":w"), m.View().Cursor.Position.X)
	assert.Equal(t, ":w", barOf(t, m)[1])
}

// 커서가 고른 항목 위에 있어야 한다. 이 커서가 곧 포커스 표시다.
func TestSidebarCursorFollowsSelection(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	// sidebar 가 화면 맨 윗줄부터라 뿌리를 고르면 커서도 맨 윗줄이다.
	assert.Equal(t, tea.Position{X: 0, Y: 0}, m.(viewSidebar).View().Cursor.Position)

	m = send(m, "down", "down")

	assert.Equal(t, tea.Position{X: 0, Y: 2}, m.(viewSidebar).View().Cursor.Position)
}

func TestSidebarShowsModeAndPath(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	assert.Equal(t, "TREE", modeOf(t, m))

	// 아래 줄 오른쪽 끝은 tip 자리라(tip.go) 통째로 비교하지 않는다. 여기서 보는 것은 왼쪽이다.
	assert.True(t, strings.HasPrefix(barOf(t, m)[1], filepath.Base(m.(viewSidebar).sidebar.root)+"/"),
		"뿌리는 이름만: %q", barOf(t, m)[1])

	m = send(m, "down", "down")
	assert.True(t, strings.HasPrefix(barOf(t, m)[1], "docs"),
		"뿌리 기준 상대 경로라 절대 경로처럼 잘리지 않는다: %q", barOf(t, m)[1])
}

// 화면이 좁아져서 sidebar 가 숨으면 포커스가 안 보이는 곳에 남으면 안 된다.
func TestSidebarFocusEscapesOnAutoHide(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, m)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})

	assert.IsType(t, viewEditorNormal{}, m, "안 보이는 pane 에 포커스를 남기지 않는다")
}

// ctrl+c 는 다른 mode 와 같은 경로다. 취소하면 sidebar 로 돌아온다.
func TestSidebarCtrlCQuits(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	m = send(m, "i", "X")
	require.IsType(t, viewSidebar{}, m, "sidebar 에서 i 는 아무 일도 하지 않는다")

	m, _ = m.Update(key("ctrl+c"))
	assert.IsType(t, finalExit{}, m, "변경이 없으면 그냥 종료")
}

func TestSidebarCtrlCConfirmsAndReturns(t *testing.T) {
	m := newTreeEditor(t, 80, 6)
	m.buffers[0].dirty = true

	var model tea.Model = m
	model = send(model, "ctrl+w", "ctrl+w")
	model, _ = model.Update(key("ctrl+c"))
	require.IsType(t, viewConfirmDiscard{}, model)

	model, _ = model.Update(key("esc"))
	assert.IsType(t, viewSidebar{}, model, "취소하면 sidebar 로 돌아온다")
}

// 트리가 화면보다 길면 아래로 내려갈 때 스크롤한다.
// sidebar 가 화면 전체 높이를 쓰므로 편집 내용 높이를 1 로 두어야 트리가 화면보다 길어진다.
func TestSidebarScrollsWhenSelectionLeavesView(t *testing.T) {
	m := newTreeEditor(t, 80, 1)

	var model tea.Model = m
	model = send(model, "ctrl+w", "ctrl+w")
	require.Equal(t, 0, model.(viewSidebar).sidebar.top)

	model = send(model, "down", "down", "down", "down")

	v := model.(viewSidebar)
	assert.Greater(t, v.sidebar.top, 0, "화면 밖으로 나가면 민다")
	row, ok := v.sidebar.selectedRow(v.sidebarHeight())
	require.True(t, ok, "고른 것은 언제나 화면 안에 있다")
	assert.Less(t, row, v.sidebarHeight())
}

// selectTree 는 sidebar 에 포커스를 두고 이름이 name 인 항목까지 내려간다.
func selectTree(t *testing.T, m tea.Model, name string) tea.Model {
	t.Helper()

	m = send(m, "ctrl+w", "ctrl+w")
	for range 20 {
		v, ok := m.(viewSidebar)
		require.True(t, ok)
		if node := v.sidebar.selectedNode(); node != nil && node.name == name {
			return m
		}
		m = send(m, "down")
	}

	t.Fatalf("%q 를 찾지 못했다", name)
	return nil
}

func TestSidebarEnterOpensFileInNewTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "enter")

	require.IsType(t, viewEditorNormal{}, model, "연 파일을 보러 편집 영역으로 간다")

	v := model.(viewEditorNormal)
	require.Len(t, v.buffers, 2)
	assert.Equal(t, filepath.Join(root, "README.md"), v.buffers[v.active].path)
	assert.True(t, v.sidebar.open, "sidebar 는 열린 채로 남는다")
}

// 이미 열려 있으면 새 tab 을 만들지 않고 그 tab 으로 옮긴다.
// 같은 파일을 두 Buffer 로 열면 한쪽 저장이 다른 쪽 편집을 조용히 덮어쓴다.
func TestSidebarEnterSwitchesToOpenTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "enter")
	require.Len(t, model.(viewEditorNormal).buffers, 2)

	model = selectTree(t, model, "README.md")
	model = send(model, "enter")

	v := model.(viewEditorNormal)
	assert.Len(t, v.buffers, 2, "tab 이 늘지 않는다")
	assert.Equal(t, "README.md", filepath.Base(v.buffers[v.active].path))
}

// CLI 로 상대 경로로 연 파일과 트리의 절대 경로가 같은 파일임을 알아봐야 한다.
// 글자 그대로 비교하면 여기서 중복 Buffer 가 생긴다.
func TestSidebarEnterMatchesRelativePath(t *testing.T) {
	root := newTreeFixture(t)
	t.Chdir(root)

	m := viewEditorNormal{
		editor: &editor{
			// CLI 로 상대 경로로 연 것과 같은 모양이다.
			buffers: []viewport{newBuffer("README.md", []byte("x\n"))},
			width:   80,
			height:  10 + tablineHeight + statusBarHeight,
		},
	}
	m.sidebar = openSidebarSync(t, root)

	model := selectTree(t, tea.Model(m), "README.md")
	model = send(model, "enter")

	v := model.(viewEditorNormal)
	assert.Len(t, v.buffers, 1, "상대 경로와 절대 경로가 같은 파일이다")
}

// 일반 파일이 아니면 열지 않는다. FIFO 를 ReadFile 하면 편집기가 영영 멈춘다.
func TestSidebarEnterRefusesNonRegularFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, exec.Command("mkfifo", filepath.Join(root, "pipe")).Run())

	m := viewEditorNormal{
		editor: &editor{
			buffers: []viewport{newBuffer("main.go", []byte("a\n"))},
			width:   80,
			height:  10 + tablineHeight + statusBarHeight,
		},
	}
	m.sidebar = openSidebarSync(t, root)

	model := selectTree(t, tea.Model(m), "pipe")
	model = send(model, "enter")

	require.IsType(t, viewEditorNormal{}, model)
	assert.Len(t, model.(viewEditorNormal).buffers, 1, "열지 않는다")
	assert.Contains(t, barOf(t, model)[1], "일반 파일이 아닙니다")
}

// 디렉터리를 가리키는 symlink 는 잎으로 보이지만 열면 안 된다.
func TestSidebarEnterRefusesSymlinkToDir(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")))

	m := newTreeEditor(t, 80, 10)
	m.sidebar = openSidebarSync(t, root)

	model := selectTree(t, tea.Model(m), "link")
	model = send(model, "enter")

	assert.Len(t, model.(viewEditorNormal).buffers, 1)
	assert.Contains(t, barOf(t, model)[1], "일반 파일이 아닙니다")
}

// 트리를 읽은 뒤에 지워진 파일을 고르면 빈 buffer 를 만들지 말고 알려야 한다.
func TestSidebarEnterRefusesDeletedFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	model := selectTree(t, tea.Model(m), "README.md")
	require.NoError(t, os.Remove(filepath.Join(root, "README.md")))

	model = send(model, "enter")

	require.IsType(t, viewEditorNormal{}, model)
	assert.Len(t, model.(viewEditorNormal).buffers, 1, "빈 buffer 를 만들지 않는다")
}

// reveal 은 파일이 있는 자리까지 펼치고 그 항목을 고른다(ADR-0019).
func TestSidebarRevealExpandsAndSelects(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebarSync(t, root)

	s = revealSync(t, s, filepath.Join(root, "docs", "spec.md"))

	assert.Contains(t, names(s.rows()), "2:spec.md", "가는 길의 디렉터리가 펼쳐진다")
	require.NotNil(t, s.selectedNode())
	assert.Equal(t, "spec.md", s.selectedNode().name, "고른 항목이 그 파일이다")
}

// 뿌리 바로 아래의 파일도 고른다. 펼칠 것이 없는 짧은 경로다.
func TestSidebarRevealSelectsFileAtRoot(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebarSync(t, root)

	s = revealSync(t, s, filepath.Join(root, "README.md"))

	assert.Equal(t, "README.md", s.selectedNode().name)
}

// CLI 로 상대 경로로 연 파일도 절대 경로인 트리에서 찾아야 한다. tabOf 와 같은 문제다.
func TestSidebarRevealMatchesRelativePath(t *testing.T) {
	root := newTreeFixture(t)
	t.Chdir(root)

	s := openSidebarSync(t, root)

	s = revealSync(t, s, filepath.Join("docs", "spec.md"))
	assert.Equal(t, "spec.md", s.selectedNode().name)
}

// 이미 펼쳐진 디렉터리는 다시 읽지 않는다. 다시 읽으면 자식이 새로 만들어져서
// 그 아래 펼쳐 둔 것이 통째로 접힌다.
func TestSidebarRevealDoesNotRereadExpandedDir(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebarSync(t, root)

	docs := s.rows()[2].node
	require.Equal(t, "docs", docs.name)
	toggleSync(t, docs)

	before := docs.children[0]
	require.Equal(t, "spec.md", before.name)

	s = revealSync(t, s, filepath.Join(root, "docs", "spec.md"))

	assert.Same(t, before, s.selectedNode(), "자식을 새로 만들지 않는다")
}

// 뿌리 밖의 파일은 트리에 자리가 없다. 고른 자리를 건드리지 않는다.
func TestSidebarRevealIgnoresOutsideRoot(t *testing.T) {
	root := newTreeFixture(t)
	outside := filepath.Join(t.TempDir(), "other.go")
	require.NoError(t, os.WriteFile(outside, []byte("x\n"), 0644))

	s := openSidebarSync(t, root)
	s.selected = 2

	s = revealSync(t, s, outside)
	assert.Equal(t, 2, s.selected, "고른 자리가 그대로다")
}

// 이름 없는 buffer 는 경로가 빈 문자열이다.
func TestSidebarRevealIgnoresEmptyPath(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))
	s.selected = 2

	s = revealSync(t, s, "")
	assert.Equal(t, 2, s.selected)
}

// symlink 디렉터리는 따라가지 않으므로 그 안쪽은 펼칠 자식이 없다.
func TestSidebarRevealStopsAtSymlinkDir(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")))

	s := openSidebarSync(t, root)
	s.selected = 1

	s = revealSync(t, s, filepath.Join(root, "link", "spec.md"))
	assert.Equal(t, 1, s.selected)
}

// 팔레트나 트리에서 파일을 열면(openTab) 트리가 그 자리를 펼치고 고른다.
func TestOpenTabRevealsInSidebar(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	reveal, err := m.openTab(filepath.Join(root, "docs", "spec.md"))
	require.NoError(t, err)
	settle(t, m, reveal)

	assert.Contains(t, names(m.sidebar.rows()), "2:spec.md")
	assert.Equal(t, "spec.md", m.sidebar.selectedNode().name)
	row, ok := m.sidebar.selectedRow(m.sidebarHeight())
	assert.True(t, ok, "고른 자리가 화면 안으로 들어온다")
	assert.Less(t, row, m.sidebarHeight())
}

// tab 을 옮기면 트리도 그 파일 자리로 따라간다.
func TestTabSwitchRevealsInSidebar(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root
	m.buffers = []viewport{
		newBuffer(filepath.Join(root, "main.go"), []byte("a\n")),
		newBuffer(filepath.Join(root, "docs", "spec.md"), []byte("b\n")),
	}

	model := sendSync(t, tea.Model(m), "g", "t")

	v := model.(viewEditorNormal)
	require.Equal(t, "spec.md", filepath.Base(v.activeBuffer().path))
	assert.Equal(t, "spec.md", v.sidebar.selectedNode().name)

	model = sendSync(t, model, "g", "T")

	v = model.(viewEditorNormal)
	assert.Equal(t, "main.go", v.sidebar.selectedNode().name)
}

// 이름 없는 tab 으로 옮기면 고른 자리는 그대로 남는다.
func TestTabSwitchToUnnamedKeepsSelection(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root
	m.buffers = []viewport{
		newBuffer(filepath.Join(root, "main.go"), []byte("a\n")),
		newEmptyBuffer(""),
	}
	revealSyncIn(t, m.editor, m.activeBuffer().path)

	model := sendSync(t, tea.Model(m), "g", "t")

	v := model.(viewEditorNormal)
	assert.Equal(t, "main.go", v.sidebar.selectedNode().name, "이름이 없으면 트리는 움직이지 않는다")
}

// `:tree` 로 닫으면 트리를 버리므로, 다시 열 때 보고 있는 파일 자리를 다시 펼친다.
func TestToggleTreeRevealsCurrentFile(t *testing.T) {
	root := newTreeFixture(t)
	t.Chdir(root)

	e := editor{
		buffers: []viewport{newBuffer(filepath.Join("docs", "spec.md"), []byte("a\n"))},
		width:   80,
		height:  10 + tablineHeight + statusBarHeight,
	}
	e.sidebar = openSidebarSync(t, root)

	_, err := e.toggleTree()
	require.NoError(t, err)
	require.False(t, e.sidebar.open)

	// 다시 여는 쪽은 뿌리부터 읽는 작업이라 그것이 끝나기를 기다린다.
	load, err := e.toggleTree()
	require.NoError(t, err)
	settle(t, viewEditorNormal{editor: &e}, load)

	require.NotNil(t, e.sidebar.selectedNode())
	assert.Equal(t, "spec.md", e.sidebar.selectedNode().name)
}

// tab 을 닫으면 그 자리에 드러난 파일 자리로 트리가 옮겨간다.
func TestCloseTabRevealsRemainingFile(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root
	m.buffers = []viewport{
		newBuffer(filepath.Join(root, "main.go"), []byte("a\n")),
		newBuffer(filepath.Join(root, "docs", "spec.md"), []byte("b\n")),
	}
	m.active = 1
	revealSyncIn(t, m.editor, m.activeBuffer().path)
	require.Equal(t, "spec.md", m.sidebar.selectedNode().name)

	model, _ := forceCloseTab(m.editor)
	require.IsType(t, viewEditorNormal{}, model)

	require.Equal(t, "main.go", filepath.Base(m.activeBuffer().path))
	assert.Equal(t, "main.go", m.sidebar.selectedNode().name, "닫은 파일이 아니라 남은 파일이다")
}

// sgrPattern 은 SGR sequence 와 그 뒤에 딸린 글자다. lipgloss 는 밑줄이 있으면 글자마다
// style 을 내므로 한 이름이 조각 여럿으로 쪼개진다.
var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m([^\x1b]*)`)

// activeText 는 "지금 보고 있는 파일" 표시가 걸린 글자만 이어 붙인 것이다.
//
// 굵기(1) 와 밑줄(4) 이 한 SGR 안에 같이 있는 조각을 찾는다. lipgloss 가 속성을 어떤 순서로
// 묶어 내든 상관없다. 색 매개변수(`38;5;81`) 가 섞여 들지만 트리 색에 1 과 4 는 없다.
func activeText(cell string) string {
	out := strings.Builder{}

	for _, match := range sgrPattern.FindAllStringSubmatch(cell, -1) {
		params := strings.Split(match[1], ";")
		if !slices.Contains(params, "1") || !slices.Contains(params, "4") {
			continue
		}

		out.WriteString(match[2])
	}

	return out.String()
}

// activeNames 는 그 표시가 걸린 행의 이름이다.
func activeNames(cells []string) []string {
	out := []string{}
	for _, cell := range cells {
		if activeText(cell) == "" {
			continue
		}

		plain := strings.TrimSpace(ansi.Strip(cell))
		out = append(out, strings.TrimSpace(strings.TrimSuffix(plain, "│")))
	}

	return out
}

// 지금 보고 있는 파일만 굵고 밑줄이 있다. 트리 커서는 포커스가 트리에 있을 때만 보이므로,
// 편집 중에 트리가 지금 자리를 나타내는 것은 이 표시뿐이다 (ADR-0022).
func TestSidebarMarksActiveFile(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebarSync(t, root)

	cells := s.renderCells(10, filepath.Join(root, "main.go"), nil, boxUnicode)

	assert.Equal(t, []string{"main.go"}, activeNames(cells))
}

// 표시는 이름에만 걸린다. 들여쓰기까지 이으면 깊은 자리의 파일에서 밑줄이 이름 앞 빈 칸을 끌고 온다.
func TestSidebarMarksNameWithoutIndent(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebarSync(t, root)

	// 한 칸 들여쓰인 자리라야 밑줄이 앞 빈 칸을 끌고 오는지가 드러난다.
	spec := filepath.Join(root, "docs", "spec.md")
	s = revealSync(t, s, spec)

	for _, cell := range s.renderCells(10, spec, nil, boxUnicode) {
		if text := activeText(cell); text != "" {
			assert.Equal(t, "spec.md", text)
			return
		}
	}

	t.Fatal("표시가 걸린 행이 없다")
}

// 디렉터리는 표시가 없다. 굵기가 뜻 둘을 가지면 읽는 규칙이 흐려진다 (ADR-0022).
// 디렉터리는 색과 `▸`/`▾` 표시와 `/` 접미로 이미 갈린다.
func TestSidebarDirIsNotMarked(t *testing.T) {
	s := openSidebarSync(t, newTreeFixture(t))

	assert.Empty(t, activeNames(s.renderCells(10, "", nil, boxUnicode)))
}

// 이름 없는 buffer 는 어느 행과도 맞지 않는다. `:tabnew` 로 만든 tab 이 그렇다.
func TestSidebarMarksNothingWithoutName(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	m.buffers = []viewport{newEmptyBuffer("")}

	assert.Empty(t, activeNames(m.sidebar.renderCells(m.sidebarHeight(), m.activePath(), nil, m.boxChars)))
}

// tab 을 옮기면 표시도 따라간다. 트리가 그 자리를 펼치는 것(reveal) 과 짝이다.
func TestSidebarMarkFollowsActiveTab(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root
	m.buffers = []viewport{
		newBuffer(filepath.Join(root, "main.go"), []byte("a\n")),
		newBuffer(filepath.Join(root, "docs", "spec.md"), []byte("b\n")),
	}
	require.Equal(t, []string{"main.go"}, activeNames(m.sidebar.renderCells(m.sidebarHeight(), m.activePath(), nil, m.boxChars)))

	next := sendSync(t, tea.Model(m), "g", "t").(viewEditorNormal)

	assert.Equal(t, []string{"spec.md"},
		activeNames(next.sidebar.renderCells(next.sidebarHeight(), next.activePath(), nil, next.boxChars)))
}
