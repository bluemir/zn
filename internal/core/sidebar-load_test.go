package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// newBigDirFixture 는 항목이 많은 디렉터리를 만든다. 이름은 `f00000` 부터 이름순이다.
func newBigDirFixture(t *testing.T, count int) string {
	t.Helper()

	root := t.TempDir()
	for i := range count {
		require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("f%05d", i)), nil, 0644))
	}

	return root
}

// newUnreadTreeEditor 는 그 뿌리로 sidebar 를 연 편집기다. 아직 아무것도 읽지 않았다 —
// 편집기가 뜨는 길과 같다(ADR-0032).
func newUnreadTreeEditor(t *testing.T, root string) viewEditorNormal {
	t.Helper()

	m := viewEditorNormal{
		editor: &editor{
			buffers: []textarea.Viewport{textarea.NewEmptyBuffer("")},
			width:   80,
			height:  10 + tablineHeight + statusBarHeight,
		},
	}
	m.sidebar = openSidebar(root)

	return m
}

// 항목이 많아도 끝까지 읽어 다 항목으로 만든다. 표시 상한은 두지 않는다(ADR-0032).
func TestTreeReadsEveryChild(t *testing.T) {
	const count = 5_000

	m := newUnreadTreeEditor(t, newBigDirFixture(t, count))
	settle(t, m, m.startTree())

	rows := m.sidebar.rows()
	require.Len(t, rows, 1+count, "뿌리 + 자식 전부")
	assert.Equal(t, "f00000", rows[1].node.name)
	assert.Equal(t, fmt.Sprintf("f%05d", count-1), rows[count].node.name, "마지막 항목까지 있다")

	for _, row := range rows {
		assert.False(t, row.node.placeholder, "다 읽은 뒤에는 안내 행이 없다: %s", row.node.name)
	}
}

// 다 읽은 개수는 작업 목록에 남는다.
func TestTreeLoadRecordsCount(t *testing.T) {
	const count = 1_200

	m := newUnreadTreeEditor(t, newBigDirFixture(t, count))
	settle(t, m, m.startTree())

	require.Len(t, m.finished, 1)
	assert.Equal(t, "1,200 개", m.finished[0].summary)
}

// 펼치기는 작업이다. 표시는 지금 서고 자식은 결과가 도착할 때 찬다.
func TestTreeExpandIsAsync(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	docs := m.sidebar.rows()[2].node
	require.Equal(t, "docs", docs.name)

	cmd := m.expandNode(docs)
	require.NotNil(t, cmd, "읽기 작업이 시작된다")

	assert.True(t, docs.expanded, "펼침 표시는 기다리지 않는다")
	assert.True(t, docs.loading)
	assert.Contains(t, names(m.sidebar.rows()), "2:… 읽는 중")
	assert.True(t, m.jobRunning(dirJobName, dirJobArgs(m.sidebar.root, docs.path)))

	settle(t, m, cmd)

	assert.False(t, docs.loading)
	assert.Contains(t, names(m.sidebar.rows()), "2:spec.md")
	assert.NotContains(t, names(m.sidebar.rows()), "2:… 읽는 중")
}

// 읽는 중에 접으면 작업을 끊는다. 늦게 도착한 결과는 접힌 것을 도로 펼치지 않는다.
func TestTreeCollapseCancelsLoad(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	docs := m.sidebar.rows()[2].node
	cmd := m.expandNode(docs)
	require.NotNil(t, cmd)

	args := dirJobArgs(m.sidebar.root, docs.path)
	m.collapseNode(docs)

	settle(t, m, cmd)

	assert.False(t, docs.expanded, "늦게 온 결과가 도로 펼치지 않는다")
	assert.Empty(t, docs.children)

	assert.False(t, m.jobRunning(dirJobName, args))
	require.Len(t, m.finished, 1)
	assert.Equal(t, dirJobName, m.finished[0].name)
	assert.Equal(t, args, m.finished[0].args)
	assert.ErrorIs(t, m.finished[0].err, context.Canceled)
}

// reveal 은 아직 읽지 않은 디렉터리를 만나면 그것을 읽고, 자식이 오면 다음 층으로 내려간다.
func TestRevealWalksUnreadDirs(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(deep, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "c.go"), []byte("x\n"), 0644))

	m := newUnreadTreeEditor(t, root)
	require.True(t, m.sidebar.setRevealTarget(filepath.Join(deep, "c.go")))

	settle(t, m, m.startTree())

	assert.Equal(t, []string{"0:" + filepath.Base(root), "1:a", "2:b", "3:c.go"}, names(m.sidebar.rows()))
	require.NotNil(t, m.sidebar.selectedNode())
	assert.Equal(t, "c.go", m.sidebar.selectedNode().name)
	assert.Empty(t, m.sidebar.pendingReveal, "자리를 잡으면 비워진다")
}

// 갈 자리가 없으면 뿌리만 읽는다. 이름 없는 buffer 로 시작하는 길이다.
func TestStartTreeReadsRootWithoutTarget(t *testing.T) {
	m := newUnreadTreeEditor(t, newTreeFixture(t))

	settle(t, m, m.startTree())

	assert.True(t, m.sidebar.tree.expanded)
	assert.Contains(t, names(m.sidebar.rows()), "1:main.go")
}

// 항목이 많은 디렉터리에서도 맨 끝의 파일을 찾아 고른다. 가려지는 자리가 없다.
func TestRevealFindsLastChildOfBigDir(t *testing.T) {
	const count = 3_000

	root := newBigDirFixture(t, count)
	last := fmt.Sprintf("f%05d", count-1)

	m := newUnreadTreeEditor(t, root)
	require.True(t, m.sidebar.setRevealTarget(filepath.Join(root, last)))

	settle(t, m, m.startTree())

	assert.Empty(t, m.sidebar.pendingReveal)
	require.NotNil(t, m.sidebar.selectedNode())
	assert.Equal(t, last, m.sidebar.selectedNode().name)
}

// 트리에 없는 자리를 가리키면 조용히 그만두고 고른 자리를 건드리지 않는다.
func TestRevealStopsAtMissingChild(t *testing.T) {
	root := newTreeFixture(t)

	m := newUnreadTreeEditor(t, root)
	require.True(t, m.sidebar.setRevealTarget(filepath.Join(root, "docs", "gone.md")))

	settle(t, m, m.startTree())

	assert.Empty(t, m.sidebar.pendingReveal, "찾지 못하면 그만둔다")
	assert.Equal(t, 0, m.sidebar.selected, "고른 자리는 뿌리 그대로다")
}

// 사용자가 직접 접었으면 그 안으로 향하던 reveal 도 그만둔다.
// 남겨 두면 다음 조각이 도착할 때 방금 접은 것이 도로 펼쳐진다.
func TestCollapseDropsPendingReveal(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	docs := m.sidebar.rows()[2].node
	require.Equal(t, "docs", docs.name)

	require.NotNil(t, m.revealInSidebar(filepath.Join(root, "docs", "spec.md")))
	require.NotEmpty(t, m.sidebar.pendingReveal)

	m.collapseNode(docs)

	assert.Empty(t, m.sidebar.pendingReveal)
}

// `… 읽는 중` 은 파일이 아니다. 열 것도 없고 오류도 아니다.
func TestTreeNoticeRowIsInert(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	docs := m.sidebar.rows()[2].node
	require.NotNil(t, m.expandNode(docs))

	// 안내 행은 그 디렉터리 자리 바로 아래다.
	for i, row := range m.sidebar.rows() {
		if row.node.placeholder {
			m.sidebar.selected = i
		}
	}
	require.True(t, m.sidebar.selectedNode().placeholder)

	next, cmd := viewSidebar{editor: m.editor}.enter()

	assert.Nil(t, cmd, "열 것이 없다")
	require.IsType(t, viewSidebar{}, next, "편집 화면으로 나가지 않는다")
	assert.Len(t, next.(viewSidebar).buffers, 1, "tab 을 만들지 않는다")
	assert.Empty(t, next.(viewSidebar).notice, "오류도 알리지 않는다")
}

// `… 읽는 중` 은 "지금 보고 있는 파일" 표시를 받지 않는다.
// 이름 없는 buffer 는 activePath 가 빈 문자열이라 경로만 견주면 이 행이 걸린다.
func TestTreeNoticeRowIsNotMarkedActive(t *testing.T) {
	m := newUnreadTreeEditor(t, newTreeFixture(t))
	require.NotNil(t, m.startTree())

	require.Equal(t, "", m.activePath())
	cells := m.sidebar.renderCells(m.sidebarHeight(), m.activePath(), nil, boxUnicode)
	require.Contains(t, cells[1], "읽는 중")
	assert.Empty(t, activeNames(cells))
}

// 작업 이름이 신원이라 디렉터리마다 달라야 한다. 목록에서 어디를 읽는지도 이것으로 읽힌다.
func TestDirJobName(t *testing.T) {
	root := filepath.Join("/tmp", "zn")

	assert.Equal(t, []string{"internal/core"}, dirJobArgs(root, filepath.Join(root, "internal", "core")))
	assert.Equal(t, []string{"zn"}, dirJobArgs(root, root), "뿌리는 그 이름으로 적는다")
}

// 트리를 다시 열면(`:tree`) 앞서 읽던 것이 도착해도 새 트리를 망가뜨리지 않는다.
// 결과는 노드 포인터가 아니라 경로로 자리를 찾는다.
func TestLateLoadFindsNodeByPath(t *testing.T) {
	root := newTreeFixture(t)
	t.Chdir(root)

	m := newUnreadTreeEditor(t, root)
	cmd := m.startTree()
	require.NotNil(t, cmd)

	// 읽는 중에 트리를 버리고 새로 연다. 앞서 잡아둔 포인터는 이제 어느 화면에도 없다.
	old := m.sidebar.tree
	m.sidebar = openSidebar(root)
	require.NotSame(t, old, m.sidebar.tree)
	m.sidebar.tree.expanded = true
	m.sidebar.tree.loading = true

	settle(t, m, cmd)

	assert.False(t, m.sidebar.tree.loading, "새 트리가 그 결과를 받는다")
	assert.Contains(t, names(m.sidebar.rows()), "1:main.go")
}

// tea.Batch 로 여럿이 실려 와도 settle 이 다 돌려야 한다. helper 자체를 한 번 본다.
func TestSettleRunsBatchedCmds(t *testing.T) {
	m := newTreeEditor(t, 80, 10)

	docs := m.sidebar.rows()[2].node
	build := m.sidebar.rows()[1].node
	require.Equal(t, "build", build.name)

	settle(t, m, tea.Batch(m.expandNode(docs), m.expandNode(build)))

	assert.Contains(t, names(m.sidebar.rows()), "2:spec.md")
	assert.Contains(t, names(m.sidebar.rows()), "2:out")
}
