package core

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCommitView 는 fixture 저장소의 커밋 하나를 연 상세 화면이다.
//
// at 은 목록에서 몇 번째냐다. 0 이 merge 이고 3 이 뿌리다(newGraphFixture).
func newCommitView(t *testing.T, at int) viewCommit {
	t.Helper()

	root := newGraphFixture(t)
	t.Chdir(root)

	rows := walkAll(t, root, 100)
	require.Greater(t, len(rows), at)

	list := newGraphView(t, rows)
	list.selected = at

	model, cmd := list.run("enter")
	require.NotNil(t, cmd)

	m, ok := model.(viewCommit)
	require.True(t, ok)

	// 읽어 온 것을 그 자리에서 넣는다. Cmd 를 돌리는 고리는 시험에 없다.
	detail, err := readCommitDetail(context.Background(), root, m.commit.hash)
	require.NoError(t, err)

	m.detail, m.ready = detail, true

	return m
}

// 상세는 `git show --format=fuller` 와 같은 차례로 선다.
func TestCommitViewLines(t *testing.T) {
	m := newCommitView(t, 3)

	content := ansi.Strip(strings.Join(m.lines(), "\n"))

	assert.Contains(t, content, "commit "+m.commit.hash.String())
	assert.Contains(t, content, "(tag: v0.1)", "ref 이름이 해시 옆에 붙는다")
	assert.Contains(t, content, "Author: test <test@example.com>")
	assert.Contains(t, content, "    a.txt", "메시지는 네 칸 들여쓴다")
	assert.Contains(t, content, "A  a.txt")
	assert.NotContains(t, content, "Merge:", "부모가 하나면 서지 않는다")
}

// merge 는 `Merge:` 줄에 부모 둘을 적는다.
func TestCommitViewShowsMergeParents(t *testing.T) {
	m := newCommitView(t, 0)

	content := ansi.Strip(strings.Join(m.lines(), "\n"))

	assert.Regexp(t, `Merge: [0-9a-f]{7} [0-9a-f]{7}`, content)
	assert.Contains(t, content, "A  c.txt", "첫 부모와 견줘 갈래가 가져온 것을 보인다")
}

// 읽어 오기 전에는 그렇다고 적는다. 빈 화면을 두면 커밋이 비어 있는 것처럼 보인다.
func TestCommitViewWhileReading(t *testing.T) {
	m := newCommitView(t, 0)
	m.ready, m.detail = false, commitDetail{}

	assert.Contains(t, ansi.Strip(strings.Join(m.lines(), "\n")), "읽는 중입니다")
}

// 늦게 온 답은 버린다. 목록으로 돌아갔다 다른 커밋을 여는 사이에 도착할 수 있다.
func TestCommitViewDropsLateAnswer(t *testing.T) {
	m := newCommitView(t, 0)
	m.ready = false

	next, _ := m.Update(commitMsg{hash: plumbing.NewHash(strings.Repeat("a", 40))})

	after, ok := next.(viewCommit)
	require.True(t, ok)
	assert.False(t, after.ready, "남의 답으로는 채우지 않는다")
}

// 건드린 파일이 있으면 `j`·`k` 가 그 목록을 고른다(ADR-0140 §3).
func TestCommitViewSelectsFiles(t *testing.T) {
	m := newCommitView(t, 0)

	// 담긴 것이 넘치도록 화면을 낮춘다. 다 들어가면 굴릴 것이 없어 시험이 아무것도 재지 않는다.
	m.height = 8
	require.Greater(t, len(m.lines()), m.listHeight())
	require.NotEmpty(t, m.detail.files)

	m.move(-5)
	assert.Equal(t, 0, m.selected, "맨 위에서 멈춘다")

	m.move(9999)
	assert.Equal(t, len(m.detail.files)-1, m.selected, "맨 아래에서 멈춘다")
	assert.LessOrEqual(t, m.top, m.fileTop()+m.selected, "고른 파일이 화면 안에 있다")
	assert.Less(t, m.fileTop()+m.selected, m.top+m.listHeight())
}

// 건드린 파일이 없으면 고를 것이 없어서 화면만 굴린다.
//
// 커서를 세울 데가 없는 화면에서 고르는 손을 흉내 내면 키가 안 먹은 것으로 읽힌다.
func TestCommitViewScrollsWithoutFiles(t *testing.T) {
	m := newCommitView(t, 0)
	m.height, m.detail.files = 8, nil

	require.Greater(t, len(m.lines()), m.listHeight())

	m.move(-5)
	assert.Equal(t, 0, m.top, "맨 위에서 멈춘다")

	m.move(9999)
	assert.Equal(t, max(len(m.lines())-m.listHeight(), 0), m.top, "맨 아래에서 멈춘다")
}

// 파일 줄을 누르면 그것을 고른다. 열지는 않는다(ADR-0012).
func TestCommitViewClickSelectsFile(t *testing.T) {
	m := newCommitView(t, 0)
	m.detail.files = append(m.detail.files, commitFile{action: "M", path: "d.txt"})
	m.scrollTo()

	next, _ := m.Update(click(0, m.fileTop()+1-m.top+jobsTitleHeight))

	after, ok := next.(viewCommit)
	require.True(t, ok, "화면에 그대로 있는다")
	assert.Equal(t, 1, after.selected)
}

// 전문 쪽을 누르면 고른 것이 움직이지 않는다. 거기에는 고를 자리가 없다.
func TestCommitViewClickOnMessageDoesNothing(t *testing.T) {
	m := newCommitView(t, 0)
	m.detail.files = append(m.detail.files, commitFile{action: "M", path: "d.txt"})
	m.selected = 1
	m.scrollTo()

	next, _ := m.Update(click(0, jobsTitleHeight))

	after, ok := next.(viewCommit)
	require.True(t, ok)
	assert.Equal(t, 1, after.selected)
}

// 한글 상태로 친 키도 먹는다(ADR-0008).
func TestCommitViewTakesHangulKeys(t *testing.T) {
	m := newCommitView(t, 0)
	m.height = 8

	// fixture 의 커밋은 파일을 하나씩만 건드린다. 하나면 고른 자리가 움직일 데가 없어서
	// 키가 먹었는지 안 먹었는지가 갈리지 않는다.
	m.detail.files = append(m.detail.files, commitFile{action: "M", path: "d.txt"})

	after, ok := pressKeys(m, "ㅓ").(viewCommit)
	require.True(t, ok)
	assert.Equal(t, 1, after.selected)
}

// 건드린 파일은 갈래마다 색이 갈린다. git 마커와 같은 색이다(ADR-0094).
func TestCommitFileStyles(t *testing.T) {
	assert.Equal(t, styleGitAdded, styleCommitFile("A"))
	assert.Equal(t, styleGitModified, styleCommitFile("M"))
	assert.Equal(t, styleGitRemoved, styleCommitFile("D"))
	assert.Equal(t, styleDetail, styleCommitFile("R"))
}
