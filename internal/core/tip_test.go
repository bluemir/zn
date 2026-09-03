package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"

	"github.com/bluemir/zn/internal/textarea"
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
		assert.NotContains(t, tip, "\x1b", "색은 그리는 쪽이 입힌다(styleTip). 문장은 글자만 든다")

		// 한 줄에 한 문장뿐이라 끝을 알릴 것이 없고, 좁은 화면에서는 그 한 칸이
		// 문장이 보이거나 마는 것을 가른다(assets.Tips 의 규칙).
		assert.NotEqual(t, ".", tip[len(tip)-1:], "마침표를 붙이지 않는다: %q", tip)
	}
}

// 좁은 화면에서 긴 문장이 그냥 지나가는 것은 값으로 치지 않는다 — 요즘 터미널은 대개
// 160 칸을 넘고 tip 은 보조 수단이다. 그래도 **어디에도 못 뜨는 문장은 없어야 한다.**
//
// 160 칸에 트리(32) 를 열면 편집 영역이 128 칸이고 커서 위치와 띄우는 칸을 빼면 110 칸쯤
// 남는다. 100 을 상한으로 두면 그 화면에는 하나도 빠짐없이 뜬다.
func TestTipsFitWideScreen(t *testing.T) {
	for _, tip := range assets.Tips {
		assert.LessOrEqual(t, textarea.WidthOf(tip), 100, "어느 화면에도 못 뜬다: %q", tip)
	}
}

// 다음 자리는 한 칸 뒤가 아니라 무작위로 건너뛴 자리다(ADR-0061 §4).
//
// **적어도 한 칸은 간다.** 그래서 같은 문장이 잇달아 두 번 서지 않는다.
func TestNextTipJumpsForward(t *testing.T) {
	e := newTestEditor("abc\n", 120, 3).editor

	steps := map[int]int{}
	for range 500 {
		before := e.tipIndex
		e.nextTip()

		step := e.tipIndex - before
		assert.GreaterOrEqual(t, step, 1, "제자리에 서지 않는다")
		assert.LessOrEqual(t, step, tipStride, "목록 밖으로 튀지 않는다")
		steps[step]++
	}

	assert.Greater(t, len(steps), 1, "늘 같은 칸만큼 가면 순서가 그대로 드러난다")
}

// 오래 켜 두어도 목록 전체가 돈다. 한 바퀴 안에 다 나오지는 않지만 못 나오는 문장은 없다.
func TestNextTipReachesEveryTip(t *testing.T) {
	e := newTestEditor("abc\n", 120, 3).editor

	seen := map[string]bool{}
	for range 100 * len(assets.Tips) {
		seen[assets.Tips[e.tipIndex%len(assets.Tips)]] = true
		e.nextTip()
	}

	assert.Len(t, seen, len(assets.Tips), "안 나온 문장이 있다")
}

func TestTipShowsAtBottomRight(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	bottom := barOf(t, m)[1]
	assert.True(t, strings.HasSuffix(bottom, assets.Tips[0]), "오른쪽 끝에 붙는다: %q", bottom)
	assert.True(t, strings.HasPrefix(bottom, "1:1"), "커서 위치는 그대로다: %q", bottom)
	assert.LessOrEqual(t, textarea.WidthOf(bottom), 120, "화면을 넘지 않는다")
}

// tip 은 흐린 글씨다. 커서 위치와 같은 밝기로 서면 읽던 것을 끊는다(ADR-0061 §6).
func TestTipIsDim(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	raw := rawBarOf(t, m)[1]
	assert.True(t, strings.HasSuffix(raw, styleTip.Render(assets.Tips[0])), "%q", raw)

	// 색을 입혀도 줄은 그만큼 어긋나지 않는다. escape 가 폭으로 세어지면 여기가 깨진다.
	assert.Equal(t, textarea.WidthOf(ansi.Strip(raw)), textarea.WidthOf(raw))
	assert.LessOrEqual(t, textarea.WidthOf(raw), 120)
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
//
// 다음 자리가 무작위라(nextTip) 어느 문장인지는 대지 않는다. 볼 것은 「갈렸다」다.
func TestTipAdvancesAfterNotice(t *testing.T) {
	m := newTestEditor("abc\n", 120, 3)
	m.notify("떴다")

	var model tea.Model = m

	bottom := barOf(t, model)[1]
	assert.Contains(t, bottom, "떴다")
	assert.NotContains(t, bottom, assets.Tips[0], "알림과 배움이 한 줄에 같이 뜨지 않는다")

	// 다음 키에 알림이 걷힌다. 그때 보이는 것은 다른 문장이다.
	model = send(model, "l")
	next := barOf(t, model)[1]
	assert.Contains(t, assets.Tips, tipOf(t, next), "목록에 있는 문장이다")
	assert.NotEqual(t, assets.Tips[0], tipOf(t, next), "알림 하나에 한 번 넘어간다")
}

// 알림을 내는 실제 경로 하나로도 넘어가는지 본다.
func TestTipAdvancesOnCommandNotice(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 120, 3)

	m = send(m, ":", "z", "z", "enter") // 알 수 없는 명령
	require.Contains(t, barOf(t, m)[1], "zz")

	m = send(m, "l")
	assert.NotEqual(t, assets.Tips[0], tipOf(t, barOf(t, m)[1]))
}

// tipOf 는 아래 줄 오른쪽 끝에 붙은 tip 만 떼어낸다. 왼쪽은 커서 위치와 줄 수다.
func tipOf(t *testing.T, bottom string) string {
	t.Helper()

	for _, tip := range assets.Tips {
		if strings.HasSuffix(bottom, tip) {
			return tip
		}
	}

	return ""
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
