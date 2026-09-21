package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// stickyTreeHeight 는 머리줄 시험이 쓰는 트리 높이다. 트리가 16 행이라 굴릴 자리가 남는다.
const stickyTreeHeight = 10

// newStickyTree 는 깊이가 있는 트리를 끝까지 펼쳐 둔 것이다. 머리줄은 깊이가 있어야 볼 것이 생긴다.
//
//	 0 root/
//	 1   a/
//	 2     b/
//	 3       c/
//	 4         f1.go
//	 5         f2.go
//	 6         f3.go
//	 7         f4.go
//	 8         f5.go
//	 9       sibling.go
//	10   z1.go
//	 …
//	15   z6.go
func newStickyTree(t *testing.T) (sidebar, string) {
	t.Helper()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0755))

	files := []string{"a/b/sibling.go"}
	for i := 1; i <= 5; i++ {
		files = append(files, fmt.Sprintf("a/b/c/f%d.go", i))
	}
	for i := 1; i <= 6; i++ {
		files = append(files, fmt.Sprintf("z%d.go", i))
	}
	for _, file := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte("x\n"), 0644))
	}

	s := openSidebarSync(t, root)
	toggleSync(t, s.rows()[1].node) // a/
	toggleSync(t, s.rows()[2].node) // a/b/
	toggleSync(t, s.rows()[3].node) // a/b/c/

	require.Equal(t, []string{
		"▾ " + filepath.Base(root) + "/",
		"  ▾ a/",
		"    ▾ b/",
		"      ▾ c/",
		"          f1.go",
		"          f2.go",
		"          f3.go",
		"          f4.go",
		"          f5.go",
		"        sibling.go",
		"    z1.go", "    z2.go", "    z3.go", "    z4.go", "    z5.go", "    z6.go",
	}, treeLabels(s))

	return s, root
}

// treeLabels 는 지금 보이는 행들의 이름이다.
func treeLabels(s sidebar) []string {
	out := []string{}
	for _, row := range s.rows() {
		out = append(out, row.label())
	}

	return out
}

// cellLabels 는 그린 행에서 이름 칸만 떼어낸 것이다. 오른쪽 끝의 마커 칸과 구분선을 버린다.
func cellLabels(cells []string) []string {
	out := []string{}
	for _, cell := range cells {
		plain := strings.TrimSuffix(strings.TrimRight(ansi.Strip(cell), " "), "│")
		out = append(out, strings.TrimRight(plain, " "))
	}

	return out
}

// 트리를 굴리지 않았으면 머리줄이 없다. 맨 위가 뿌리라 덮을 것이 없다.
func TestSidebarStickyEmptyAtTop(t *testing.T) {
	s, _ := newStickyTree(t)

	assert.Empty(t, s.stickyRows(stickyTreeHeight))
}

// 굴린 만큼 상위 디렉터리가 선다. 뿌리부터 바로 위 폴더까지 전부다.
func TestSidebarStickyShowsAncestors(t *testing.T) {
	s, _ := newStickyTree(t)

	s.top = 4 // f1.go 다. 깊이 4 라 조상이 넷이다
	assert.Equal(t, []int{0, 1, 2, 3}, s.stickyRows(stickyTreeHeight), "뿌리·a·b·c 가 선다")
}

// 상위 폴더 안이 다 덮이면 그 폴더도 머리줄에서 빠진다.
func TestSidebarStickyDropsFinishedBranch(t *testing.T) {
	s, _ := newStickyTree(t)

	// 6 이면 머리줄 셋 아래 첫 행이 sibling.go 다. c/ 안은 다 덮여서 c/ 가 감쌀 것이 없다.
	s.top = 6
	assert.Equal(t, []int{0, 1, 2}, s.stickyRows(stickyTreeHeight), "c/ 가 빠지고 뿌리·a·b 만 남는다")
}

// 머리줄은 화면 절반을 넘지 않는다. 넘치면 바깥쪽부터 버린다.
func TestSidebarStickyStopsAtHalfScreen(t *testing.T) {
	s, _ := newStickyTree(t)

	s.top = 4
	assert.Equal(t, []int{2, 3}, s.stickyRows(4), "안쪽 둘만 남는다")
	assert.Empty(t, s.stickyRows(1), "그릴 자리가 없으면 하나도 안 붙는다")
}

// 트리 끝까지 굴려도 머리줄이 트리를 통째로 덮으면 안 된다.
func TestSidebarStickyLeavesOneRow(t *testing.T) {
	s, _ := newStickyTree(t)

	s.top = len(s.rows()) - 1
	assert.Empty(t, s.stickyRows(stickyTreeHeight), "마지막 한 행은 머리줄에 내주지 않는다")
}

// 머리줄이 덮은 자리에 고른 항목이 숨으면 안 된다.
func TestSidebarStickyKeepsSelectionBelow(t *testing.T) {
	s, _ := newStickyTree(t)

	s.selected, s.top = 8, 8 // f5.go 를 맨 윗 행에 둔 자리다
	s.scrollTo(stickyTreeHeight)

	row, ok := s.selectedRow(stickyTreeHeight)
	require.True(t, ok)
	assert.Equal(t, 4, len(s.stickyRows(stickyTreeHeight)))
	assert.Equal(t, 4, row, "고른 항목이 머리줄 바로 아래다")
	assert.Equal(t, 4, s.top, "머리줄 넷이 설 만큼만 위로 민다")
}

// 머리줄이 덮은 행은 화면에 그 조상으로 그려진다.
func TestSidebarStickyRendersOverTree(t *testing.T) {
	s, root := newStickyTree(t)

	s.top = 4

	assert.Equal(t, []string{
		"▾ " + filepath.Base(root) + "/",
		"  ▾ a/",
		"    ▾ b/",
		"      ▾ c/",
		"          f5.go",
		"        sibling.go",
		"    z1.go", "    z2.go", "    z3.go", "    z4.go",
	}, cellLabels(s.renderCells(stickyTreeHeight, "", nil, boxUnicode)),
		"머리줄 넷이 f1 부터 넷을 덮는다")
}

// 머리줄을 누르면 그 디렉터리가 골라진다. 덮인 항목이 아니다.
func TestSidebarStickyClickSelectsAncestor(t *testing.T) {
	s, root := newStickyTree(t)

	s.top = 4
	require.Equal(t, []int{0, 1, 2, 3}, s.stickyRows(stickyTreeHeight))

	require.True(t, s.selectRow(2, stickyTreeHeight))
	assert.Equal(t, filepath.Join(root, "a", "b"), s.selectedNode().path)
}

// 머리줄이 붙어도 한 행은 언제나 정확히 sidebarDefaultWidth 칸이다.
func TestSidebarStickyCellsAreExactlyWide(t *testing.T) {
	s, _ := newStickyTree(t)
	s.top = 4

	for i, cell := range s.renderCells(stickyTreeHeight, "", nil, boxUnicode) {
		plain := ansi.Strip(cell)
		assert.Equal(t, sidebarDefaultWidth, textarea.WidthOf(plain), "행 %d: %q", i, plain)
	}
}
