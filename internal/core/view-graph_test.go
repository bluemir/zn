package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// newGraphView 는 커밋 넷이 담긴 목록 화면이다.
//
// 저장소를 열지 않고 담긴 것만 세운다. 훑기 자체는 git-graph_test.go 가 진짜 git 과
// 견주고 있어서, 여기서 볼 것은 그 결과를 화면이 어떻게 다루는가다.
func newGraphView(t *testing.T, rows []graphRow) viewGraph {
	t.Helper()

	e := &editor{
		boxChars: boxUnicode,
		width:    100,
		height:   20,
		graph:    graphList{rows: rows, done: true},
	}

	return viewGraph{editor: e}
}

// graphFixtureRows 는 갈래가 있는 저장소를 훑은 결과다.
func graphFixtureRows(t *testing.T) []graphRow {
	t.Helper()

	return walkAll(t, newGraphFixture(t), 100)
}

// pressKeys 는 키를 차례로 넣고 마지막 model 을 준다.
func pressKeys(model tea.Model, keys ...string) tea.Model {
	for _, key := range keys {
		next, _ := model.(interface {
			press(string) (tea.Model, tea.Cmd)
		}).press(key)

		model = next
	}

	return model
}

// 커밋 하나가 적어도 두 행이다. 열을 옮기는 행이 붙으면 그만큼 는다(ADR-0141).
//
// **git 이 한 행으로 내는 커밋도 있다.** 우리 형식은 두 줄이라 제목이 설 자리를 그리는 쪽이
// 채운다. 그래서 높이의 바닥이 둘이다.
func TestGraphCommitHeight(t *testing.T) {
	rows := graphFixtureRows(t)
	require.Len(t, rows, 4)

	for i, row := range rows {
		assert.GreaterOrEqual(t, graphHeightOf(row), graphRowsPerCommit, "%d 번째", i)
		assert.GreaterOrEqual(t, graphHeightOf(row), len(row.graph), "%d 번째", i)
	}

	m := newGraphView(t, rows)

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "커밋  4 개")
	assert.NotContains(t, content, "더 읽는 중", "다 읽었으면 적지 않는다")
}

// 담기는 수는 커밋마다의 높이를 실제로 세어 나온다. 반쪽 커밋은 그리지 않는다.
func TestGraphRowsCountsRealHeights(t *testing.T) {
	rows := graphFixtureRows(t)
	m := newGraphView(t, rows)

	// 앞 둘이 차지하는 만큼만 준다. 셋째는 들어가지 못한다.
	m.height = m.graphSpanOf(0, 2) + jobsTitleHeight + jobsHintHeight + statusBarHeight
	assert.Equal(t, 2, m.graphRows())

	// 셋째가 들어갈 만큼 넓히면 셋이 담긴다.
	m.height += graphHeightOf(rows[2])
	assert.Equal(t, 3, m.graphRows())
}

// j/k 로 고른 자리가 움직이고 양끝에서 멈춘다. 다른 목록 판과 같은 손이다(ADR-0076).
func TestGraphMoves(t *testing.T) {
	m := newGraphView(t, graphFixtureRows(t))

	m.move(1)
	assert.Equal(t, 1, m.selected)

	m.move(-5)
	assert.Equal(t, 0, m.selected, "맨 위에서 멈춘다")

	m.move(99)
	assert.Equal(t, 3, m.selected, "맨 아래에서 멈춘다")
}

// 한글 상태로 친 `ㅓ`(=j)·`ㅏ`(=k) 도 먹는다(ADR-0008).
func TestGraphTakesHangulKeys(t *testing.T) {
	m := newGraphView(t, graphFixtureRows(t))

	after, ok := pressKeys(m, "ㅓ", "ㅓ").(viewGraph)
	require.True(t, ok)
	assert.Equal(t, 2, after.selected)

	back, ok := pressKeys(after, "ㅏ").(viewGraph)
	require.True(t, ok)
	assert.Equal(t, 1, back.selected)
}

// `q` 는 닫고 normal 로 간다. 볼 파일이 없으면 빈 화면이다(ADR-0064).
func TestGraphCloses(t *testing.T) {
	m := newGraphView(t, graphFixtureRows(t))

	next, _ := m.run("q")
	assert.IsType(t, viewEditorEmpty{}, next)
}

// `enter` 는 고른 커밋의 상세로 가고, 그 화면은 목록을 그대로 들고 간다.
func TestGraphOpensDetail(t *testing.T) {
	rows := graphFixtureRows(t)
	m := newGraphView(t, rows)
	m.move(2)

	next, cmd := m.run("enter")

	detail, ok := next.(viewCommit)
	require.True(t, ok, "상세 화면으로 간다")
	assert.Equal(t, rows[2].commit.hash, detail.commit.hash)
	assert.NotNil(t, cmd, "읽어 오는 Cmd 가 딸려 나온다")

	back, _ := detail.run("q")

	list, ok := back.(viewGraph)
	require.True(t, ok, "목록으로 돌아온다")
	assert.Equal(t, 2, list.selected, "고른 자리가 그대로다")
}

// 담긴 것이 없어도 열린다. 「없다」를 보여주는 것이 아무 일도 안 나는 것보다 낫다.
func TestGraphOpensWhenEmpty(t *testing.T) {
	m := newGraphView(t, nil)

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "커밋이 없습니다")

	next, _ := m.run("enter")
	assert.Equal(t, m, next, "고를 것이 없으면 아무 일도 하지 않는다")
}

// 다 읽지 않았으면 제목이 그것을 말한다.
func TestGraphTitleWhileReading(t *testing.T) {
	m := newGraphView(t, graphFixtureRows(t))
	m.graph.done = false
	m.graph.reading = true

	assert.Contains(t, ansi.Strip(m.renderTitle()), "커밋  4 개 · 더 읽는 중")
}

// 끝이 가까울 때만 다음 조각을 읽는다. 조각마다 이어 걸면 여는 순간 저장소를 통째로 읽는다.
func TestGraphPrefetchLooksAtPosition(t *testing.T) {
	rows := make([]graphRow, graphPrefetch*3)
	m := newGraphView(t, rows)
	m.graph.done, m.graph.walk = false, &graphWalk{}

	assert.Nil(t, m.prefetch(), "맨 위에서는 읽지 않는다")

	m.move(len(rows))
	assert.NotNil(t, m.prefetch(), "끝에 닿으면 읽는다")
}

// 커밋 한 줄은 `git graph` 의 두 줄 형식 그대로다. 아랫줄의 제목이 윗줄의 날짜 아래에 선다.
func TestGraphRendersTwoLines(t *testing.T) {
	rows := graphFixtureRows(t)
	m := newGraphView(t, rows)

	rendered := m.renderCommit(rows[0], false)
	require.Len(t, rendered, 2)

	head, subject := ansi.Strip(rendered[0]), ansi.Strip(rendered[1])

	assert.Contains(t, head, rows[0].commit.short+" - ")
	assert.Contains(t, head, "(HEAD → master)")
	assert.Contains(t, subject, "merge topic - test")

	// 화면 칸으로 견준다. 그래프 글자가 여러 byte 라 byte 자리로는 맞출 수 없다.
	assert.Equal(t,
		textarea.WidthOf(head[:strings.Index(head, " - ")+3]),
		textarea.WidthOf(subject[:strings.Index(subject, "merge topic")]),
		"제목이 날짜와 같은 칸에서 시작한다")
}

// 고른 커밋은 두 행 다 반전한다. 한 항목이 두 행이라 한 행만 칠하면 경계가 갈리지 않는다.
func TestGraphMarksSelected(t *testing.T) {
	rows := graphFixtureRows(t)
	m := newGraphView(t, rows)

	lines := m.renderCommit(rows[0], true)

	assert.True(t, strings.HasPrefix(ansi.Strip(lines[0]), "▸ "), "첫 행에 표시가 붙는다")
	for i, line := range lines {
		assert.NotEqual(t, line, ansi.Strip(line), "%d 번째 행이 반전된다", i)
	}
}
