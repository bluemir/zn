package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// 고르기 규칙은 자기 짧은 목록으로 본다. 진짜 tips 의 길이에 검사가 매달리면
// 문장 하나를 고칠 때마다 여기가 깨진다.
var testTips = []string{"가나다라마", "짧다", "제법 긴 문장이 여기 있다"}

func TestFitTipSkipsTooLong(t *testing.T) {
	// 첫 문장은 한글 다섯 자라 10 칸이다. 8 칸에는 `짧다`(4 칸) 만 들어간다.
	assert.Equal(t, "짧다", fitTip(testTips, 0, 8))
}

func TestFitTipCountsHangulAsTwo(t *testing.T) {
	assert.Equal(t, "가나다라마", fitTip(testTips, 0, 10), "딱 맞으면 들어간다")
	assert.Equal(t, "짧다", fitTip(testTips, 0, 9), "한 칸 모자라면 건너뛴다")
}

func TestFitTipWrapsAround(t *testing.T) {
	assert.Equal(t, "가나다라마", fitTip(testTips, len(testTips)-1, 10),
		"마지막 다음은 처음이다")
}

func TestFitTipGivesUp(t *testing.T) {
	assert.Empty(t, fitTip(testTips, 0, 3), "하나도 안 들어가면 빈 문자열이다")
	assert.Empty(t, fitTip(nil, 0, 100), "목록이 비어도 나눗셈이 터지지 않는다")
}

// 목록 위생. 그리는 자리가 색도 줄바꿈도 다루지 않으므로 데이터가 지켜야 한다.
func TestTipsAreClean(t *testing.T) {
	require.NotEmpty(t, assets.Tips)

	for _, tip := range assets.Tips {
		assert.NotEmpty(t, strings.TrimSpace(tip), "빈 문장은 자리만 먹는다")
		assert.NotContains(t, tip, "\n", "아래 줄은 한 줄이다")
		assert.NotContains(t, tip, "\t", "tab 은 폭이 자리에 따라 달라진다")
		assert.NotContains(t, tip, "\x1b", "색을 넣지 않는다(truncateToWidth 가 ANSI 를 모른다)")
	}
}

// 80 칸에 트리를 열면 편집 영역이 48 칸이고 커서 위치를 빼면 35 칸쯤 남는다.
// 그 자리에 보일 문장이 하나도 없으면 이 기능이 트리를 쓰는 동안 없는 것과 같다.
func TestTipsFitNarrowScreen(t *testing.T) {
	fits := 0
	for _, tip := range assets.Tips {
		if screenWidthOf(tip) <= 35 {
			fits++
		}
	}

	assert.Greater(t, fits, len(assets.Tips)/2, "절반 이상은 좁은 화면에서도 보여야 한다")
}

func TestTipShowsAtBottomRight(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	bottom := barOf(t, m)[1]
	assert.True(t, strings.HasSuffix(bottom, assets.Tips[0]), "오른쪽 끝에 붙는다: %q", bottom)
	assert.True(t, strings.HasPrefix(bottom, "1:1"), "커서 위치는 그대로다: %q", bottom)
	assert.LessOrEqual(t, screenWidthOf(bottom), 120, "화면을 넘지 않는다")
}

// 접두 키를 치는 동안에는 그 키가 먹혔는지가 tip 보다 급하다.
func TestTipYieldsToShowcmd(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	m = send(m, "3")

	bottom := barOf(t, m)[1]
	assert.True(t, strings.HasSuffix(bottom, "3"), "showcmd 가 그 자리다: %q", bottom)
	assert.NotContains(t, bottom, assets.Tips[0])

	m = send(m, "esc")
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], assets.Tips[0]), "접두 키를 무르면 돌아온다")
}

// 좁으면 숨는다 — 아래 줄이 한 글자도 달라지지 않아야 한다.
func TestTipHiddenWhenNarrow(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 20, 3)

	assert.Equal(t, "1:1  (1 줄)", barOf(t, m)[1])
}

// 알림이 그 줄을 쓰는 동안에는 비켜서고, 걷히면 다음 문장이 드러난다.
func TestTipAdvancesAfterNotice(t *testing.T) {
	m := newTestEditor("abc\n", 120, 3)
	m.notify("떴다")

	var model tea.Model = m

	bottom := barOf(t, model)[1]
	assert.Contains(t, bottom, "떴다")
	assert.NotContains(t, bottom, assets.Tips[0], "알림과 배움이 한 줄에 같이 뜨지 않는다")
	assert.NotContains(t, bottom, assets.Tips[1])

	// 다음 키에 알림이 걷힌다. 그때 보이는 것은 다음 문장이다.
	model = send(model, "l")
	assert.True(t, strings.HasSuffix(barOf(t, model)[1], assets.Tips[1]), "%q", barOf(t, model)[1])
}

// 알림을 내는 실제 경로 하나로도 넘어가는지 본다.
func TestTipAdvancesOnCommandNotice(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	m = send(m, ":", "z", "z", "enter") // 알 수 없는 명령
	require.Contains(t, barOf(t, m)[1], "zz")

	m = send(m, "l")
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], assets.Tips[1]), "%q", barOf(t, m)[1])
}

func TestTipShowsInInsertAndVisual(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	m = send(m, "i")
	require.IsType(t, viewEditorInsert{}, m)
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], assets.Tips[0]), "insert 에서도 보인다")

	m = send(m, "esc", "v")
	require.IsType(t, viewEditorVisual{}, m)
	assert.True(t, strings.HasSuffix(barOf(t, m)[1], assets.Tips[0]), "visual 에서도 보인다")
}

func TestTipShowsInTree(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 200, 6)

	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, m)

	assert.True(t, strings.HasSuffix(barOf(t, m)[1], assets.Tips[0]), "%q", barOf(t, m)[1])
}

// 명령줄과 검색은 그 줄에 커서가 서 있다. tip 이 끼면 치는 글자와 부딪힌다.
func TestTipAbsentWhileTyping(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	assert.Equal(t, ":", barOf(t, send(m, ":"))[1])
	assert.Equal(t, "/", barOf(t, send(m, "/"))[1])
}
