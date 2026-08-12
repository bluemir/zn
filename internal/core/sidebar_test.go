package core

import (
	"os"
	"os/exec"
	"path/filepath"
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

	s := openSidebar(root)

	require.True(t, s.open)
	require.NotNil(t, s.tree)
	assert.Equal(t, filepath.Base(root), s.tree.name)
	assert.True(t, s.tree.expanded, "뿌리는 펼친 채로 시작한다")
}

// 디렉터리가 먼저, 그 안은 이름순이다. os.ReadDir 이 이미 이름순으로 준다.
func TestSidebarSortsDirsFirst(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	assert.Equal(t, []string{
		"1:build", "1:docs",
		"1:.gitignore", "1:README.md", "1:main.go",
	}, names(s.rows())[1:], "디렉터리 먼저, 그 안은 이름순")
}

// .git 은 감추고 나머지 숨김 파일은 보인다.
func TestSidebarHidesOnlyDotGit(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	shown := names(s.rows())
	assert.NotContains(t, shown, "1:.git")
	assert.Contains(t, shown, "1:.gitignore", "숨김 파일이어도 고치는 파일은 보인다")
}

// worktree·submodule 에서는 .git 이 디렉터리가 아니라 파일이다. 종류를 보지 않고 이름으로 거른다.
func TestSidebarHidesDotGitFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x\n"), 0644))

	s := openSidebar(root)

	assert.Equal(t, []string{"1:main.go"}, names(s.rows())[1:])
}

func TestSidebarExpandAndCollapse(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	docs := s.rows()[2].node
	require.Equal(t, "docs", docs.name)
	require.False(t, docs.expanded)

	docs.toggle()

	assert.Contains(t, names(s.rows()), "2:spec.md", "펼치면 자식이 목록에 들어온다")

	docs.toggle()

	assert.NotContains(t, names(s.rows()), "2:spec.md")
}

// 접었다 펴는 것이 곧 새로고침이다. watcher 없이 이걸로 충분하다.
func TestSidebarRereadsOnExpand(t *testing.T) {
	root := newTreeFixture(t)
	s := openSidebar(root)

	docs := s.rows()[2].node
	docs.toggle()
	require.Contains(t, names(s.rows()), "2:spec.md")

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "new.md"), []byte("x\n"), 0644))
	docs.toggle()
	docs.toggle()

	assert.Contains(t, names(s.rows()), "2:new.md", "다시 펼치면 새 파일이 보인다")
}

// symlink 는 따라가지 않는다. 디렉터리를 가리켜도 잎이다.
func TestSidebarDoesNotFollowSymlink(t *testing.T) {
	root := newTreeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")))

	s := openSidebar(root)

	var link *treeNode
	for _, row := range s.rows() {
		if row.node.name == "link" {
			link = row.node
		}
	}
	require.NotNil(t, link)

	assert.True(t, link.symlink)
	assert.False(t, link.isDir, "디렉터리를 가리켜도 잎으로 둔다")

	link.toggle()
	assert.False(t, link.expanded, "펼쳐지지 않는다")
}

// 끊어진 symlink 도 죽지 않아야 한다.
func TestSidebarHandlesBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(root, "nope"), filepath.Join(root, "dangling")))

	assert.NotPanics(t, func() {
		s := openSidebar(root)
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

	s := openSidebar(root)
	node := s.rows()[1].node
	require.Equal(t, "locked", node.name)

	assert.NotPanics(t, func() { node.toggle() })
	assert.True(t, node.expanded)
	assert.Empty(t, node.children)
}

func TestSidebarEmptyDir(t *testing.T) {
	s := openSidebar(t.TempDir())

	assert.Len(t, s.rows(), 1, "뿌리 한 줄만")
	assert.Equal(t, s.tree, s.selectedNode())
}

func TestSidebarSelectedNode(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

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

	s := openSidebar(root)

	byName := map[string]*treeNode{}
	for _, row := range s.rows() {
		byName[row.node.name] = row.node
	}

	assert.True(t, byName["build"].ignored, "gitignore 된 디렉터리")
	assert.False(t, byName["main.go"].ignored)
	assert.False(t, byName["README.md"].ignored)
}

// 저장소가 아니면 아무것도 흐리게 하지 않는다. 흐린 것이 없을 뿐 틀리지 않는다.
func TestSidebarNoRepoMarksNothing(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	for _, row := range s.rows() {
		assert.False(t, row.node.ignored, "%s", row.node.name)
	}
}

// newTreeEditor 는 sidebar 를 연 편집기다. 뿌리는 t.TempDir() 라 cwd 를 건드리지 않는다.
func newTreeEditor(t *testing.T, width, height int) viewEditorNormal {
	t.Helper()

	m := viewEditorNormal{
		editor: editor{
			buffers: []Buffer{newBuffer("main.go", []byte("a\nb\n"))},
			width:   width,
			height:  height + tablineHeight + statusBarHeight,
		},
	}
	m.sidebar = openSidebar(newTreeFixture(t))
	m.sidebar.scrollTo(m.textHeight())

	return m
}

// sidebarCellsOf 는 화면에서 sidebar 가 차지하는 왼쪽 칸만 떼어낸다.
func sidebarCellsOf(t *testing.T, view tea.View) []string {
	t.Helper()

	rows := strings.Split(view.Content, "\n")
	require.Greater(t, len(rows), tablineHeight+statusBarHeight)

	out := []string{}
	for _, row := range rows[tablineHeight : len(rows)-statusBarHeight] {
		plain := []byte(ansi.Strip(row))
		out = append(out, string(plain[:offsetAtScreenCol(plain, sidebarWidth)]))
	}

	return out
}

// sidebar 는 한 행이 정확히 24 칸이어야 한다. 어긋나면 편집 내용이 통째로 밀린다.
func TestSidebarCellsAreExactlyWide(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	for i, cell := range s.cells(10) {
		plain := ansi.Strip(cell)
		assert.Equal(t, sidebarWidth, screenColAt([]byte(plain), len(plain)), "행 %d: %q", i, plain)
	}
}

// 트리가 화면보다 짧아도 구분선은 화면 아래까지 이어져야 한다.
func TestSidebarCellsFillHeight(t *testing.T) {
	s := openSidebar(t.TempDir())

	cells := s.cells(6)

	require.Len(t, cells, 6)
	for _, cell := range cells[1:] {
		assert.Equal(t, strings.Repeat(" ", labelWidth)+"│ ", ansi.Strip(cell))
	}
}

// 한글 파일 이름이 경계에 걸쳐도 칸이 어긋나면 안 된다.
func TestSidebarCellsWithWideChars(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "아주아주긴한글파일이름입니다.md"), []byte("x\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "이모지🇰🇷파일.txt"), []byte("x\n"), 0644))

	s := openSidebar(root)

	for i, cell := range s.cells(4) {
		plain := ansi.Strip(cell)
		assert.Equal(t, sidebarWidth, screenColAt([]byte(plain), len(plain)), "행 %d: %q", i, plain)
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

	s := openSidebar(root)
	s.rows()[1].node.toggle() // build/ 를 펼친다

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

	s := openSidebar(root)
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
	s := openSidebar(newTreeFixture(t))
	docs := s.rows()[2].node
	docs.toggle()

	s.selected = len(s.rows()) - 1
	s.scrollTo(10)

	docs.toggle()
	s.scrollTo(10)

	assert.Less(t, s.selected, len(s.rows()))
	assert.NotNil(t, s.selectedNode())
}

// sidebar 가 열리면 편집 영역이 좁아지고 커서가 그만큼 오른쪽으로 간다.
func TestSidebarShiftsTextAndCursor(t *testing.T) {
	m := newTreeEditor(t, 80, 5)

	assert.Equal(t, 80-sidebarWidth, m.textWidth())
	assert.Equal(t, sidebarWidth, m.sidebarLeft())
	assert.Equal(t, tea.Position{X: sidebarWidth, Y: tablineHeight}, m.View().Cursor.Position)
}

// 편집 내용이 짧아도 sidebar 는 화면 아래까지 이어져야 한다.
func TestSidebarRendersFullHeightBesideShortFile(t *testing.T) {
	m := newTreeEditor(t, 80, 6)

	cells := sidebarCellsOf(t, m.View())

	require.Len(t, cells, 6)
	for i, cell := range cells {
		assert.Equal(t, sidebarWidth, screenColAt([]byte(cell), len(cell)), "행 %d", i)
	}
	assert.Contains(t, cells[0], "▾ ")
	assert.Contains(t, cells[5], "│", "파일은 2 줄뿐이지만 구분선은 아래까지 간다")
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
	for _, width := range []int{0, 1, 10, 23, 24, 25, 44} {
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

// sidebar 가 안 보이면 ctrl+w 가 접두 키를 세우지 않는다.
// 접두 키는 다음 키를 삼키는데, 갈 곳도 없이 키를 먹으면 안 된다.
func TestCtrlWDoesNotSwallowWhenSidebarHidden(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 5)

	m = send(m, "ctrl+w", "i")

	assert.IsType(t, viewEditorInsert{}, m, "ctrl+w 가 다음 키를 먹지 않는다")
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

func TestSidebarEnterTogglesDirectory(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 10)
	m = send(m, "ctrl+w", "ctrl+w", "down", "down")
	require.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name)

	m = send(m, "enter")
	assert.Contains(t, names(m.(viewSidebar).sidebar.rows()), "2:spec.md")

	m = send(m, "enter")
	assert.NotContains(t, names(m.(viewSidebar).sidebar.rows()), "2:spec.md")
}

// 커서가 고른 항목 위에 있어야 한다. 이 커서가 곧 포커스 표시다.
func TestSidebarCursorFollowsSelection(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	assert.Equal(t, tea.Position{X: 0, Y: tablineHeight}, m.(viewSidebar).View().Cursor.Position)

	m = send(m, "down", "down")

	assert.Equal(t, tea.Position{X: 0, Y: tablineHeight + 2}, m.(viewSidebar).View().Cursor.Position)
}

func TestSidebarShowsModeAndPath(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")

	assert.Contains(t, barOf(t, m)[0], "TREE")
	assert.Equal(t, filepath.Base(m.(viewSidebar).sidebar.root)+"/", barOf(t, m)[1], "뿌리는 이름만")

	m = send(m, "down", "down")
	assert.Equal(t, "docs", barOf(t, m)[1], "뿌리 기준 상대 경로라 절대 경로처럼 잘리지 않는다")
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
	require.IsType(t, viewQuitConfirm{}, model)

	model, _ = model.Update(key("esc"))
	assert.IsType(t, viewSidebar{}, model, "취소하면 sidebar 로 돌아온다")
}

// 트리가 화면보다 길면 아래로 내려갈 때 스크롤한다.
func TestSidebarScrollsWhenSelectionLeavesView(t *testing.T) {
	m := newTreeEditor(t, 80, 3)

	var model tea.Model = m
	model = send(model, "ctrl+w", "ctrl+w")
	require.Equal(t, 0, model.(viewSidebar).sidebar.top)

	model = send(model, "down", "down", "down", "down")

	v := model.(viewSidebar)
	assert.Greater(t, v.sidebar.top, 0, "화면 밖으로 나가면 민다")
	row, ok := v.sidebar.selectedRow(v.textHeight())
	require.True(t, ok, "고른 것은 언제나 화면 안에 있다")
	assert.Less(t, row, v.textHeight())
}
