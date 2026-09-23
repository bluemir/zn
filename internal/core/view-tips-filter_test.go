package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// countTips 는 그 글이 든 문장 수다. 시험이 진짜 목록의 길이에 매달리지 않게 여기서 센다.
func countTips(needle string) int {
	n := 0
	for _, tip := range assets.Tips {
		if strings.Contains(strings.ToLower(tip), strings.ToLower(needle)) {
			n++
		}
	}

	return n
}

// `/` 로 거르는 화면에 들어간다. 지금 거르던 글이 채워진 채로 뜬다.
func TestTipsFilterOpensFilled(t *testing.T) {
	tips := openTips(t, 120, 20)
	tips.filter = "트리"

	model, _ := tips.Update(key("/"))

	filter, ok := model.(viewTipsFilter)
	require.True(t, ok, "거르는 화면으로 간다")
	assert.Equal(t, "트리", filter.input.text, "고치던 글이 채워져 있다")
}

// 치는 대로 목록이 좁아진다. 몇 글자에서 멈출지를 보면서 정한다.
func TestTipsFilterNarrowsWhileTyping(t *testing.T) {
	tips := openTips(t, 120, 20)
	want := countTips("트리")
	require.Positive(t, want, "트리 tip 이 있다")
	require.Less(t, want, len(assets.Tips), "전부는 아니다")

	model := send(tea.Model(tips), "/", "트", "리")

	filter, ok := model.(viewTipsFilter)
	require.True(t, ok, "치는 동안 거르는 화면에 머문다")
	assert.Len(t, filter.narrowed().rows(), want, "친 대로 좁아진다")
	assert.Contains(t, tipLinesOf(filter.View())[0], "트리", "제목줄이 무엇으로 걸렀는지 말한다")
}

// `enter` 는 거른 목록에 선다.
func TestTipsFilterAcceptsWithEnter(t *testing.T) {
	tips := openTips(t, 120, 20)

	model := send(tea.Model(tips), "/", "트", "리", "enter")

	next, ok := model.(viewTips)
	require.True(t, ok, "목록으로 돌아간다")
	assert.Equal(t, "트리", next.filter)
	assert.Len(t, next.rows(), countTips("트리"))
}

// `esc` 는 치던 것을 버린다. 거르던 글은 그대로다.
func TestTipsFilterEscapeKeepsOldFilter(t *testing.T) {
	tips := openTips(t, 120, 20)
	tips.filter = "트리"

	model := send(tea.Model(tips), "/", "x", "esc")

	next, ok := model.(viewTips)
	require.True(t, ok)
	assert.Equal(t, "트리", next.filter, "치던 것은 버린다")
}

// 다 지우면 거르던 것을 그만두고 전체 목록으로 돌아간다. 명령줄과 같은 규칙이다.
func TestTipsFilterBackspaceOnEmptyLeaves(t *testing.T) {
	tips := openTips(t, 120, 20)
	tips.filter = "트"

	model := send(tea.Model(tips), "/", "backspace", "backspace")

	next, ok := model.(viewTips)
	require.True(t, ok, "목록으로 돌아간다")
	assert.Empty(t, next.filter)
	assert.Len(t, next.rows(), len(assets.Tips))
}

// 거른 뒤에도 번호는 전체에서의 자리다. 그 문장이 어디쯤인지가 남는다.
func TestTipsFilterKeepsOriginalNumbers(t *testing.T) {
	tips := openTips(t, 120, 20)
	tips.filter = "트리"

	rows := tips.rows()
	require.NotEmpty(t, rows)
	assert.Equal(t, assets.Tips[rows[0].at-1], rows[0].text, "번호가 그 문장을 가리킨다")
	assert.Greater(t, rows[0].at, 1, "걸러낸 앞쪽만큼 번호가 밀려 있다")
}

// 고른 자리는 좁아진 목록 안으로 당겨진다. 셋만 남았는데 서른째 줄을 고르고 있을 수 없다.
func TestTipsFilterPullsSelectionIn(t *testing.T) {
	tips := openTips(t, 120, 20)
	tips.selected = len(assets.Tips) - 1

	model := send(tea.Model(tips), "/", "트", "리", "enter")

	next := model.(viewTips)
	assert.Less(t, next.selected, len(next.rows()))
	assert.GreaterOrEqual(t, next.selected, 0)
}

// 하나도 안 걸리면 까닭을 적는다. 빈 판만 남으면 화면이 고장난 것으로 보인다.
func TestTipsFilterSaysWhenNothingMatches(t *testing.T) {
	tips := openTips(t, 120, 20)
	tips.filter = "여기에는없을글자"

	assert.Empty(t, tips.rows())
	assert.Contains(t, strings.Join(tipLinesOf(tips.View()), "\n"), "든 문장이 없습니다")
}

// 거르는 글은 두벌식 자리로 되돌리지 않는다. 「트리」가 `xmfl` 가 되면 안 된다(ADR-0008).
func TestTipsFilterKeepsHangulAsTyped(t *testing.T) {
	tips := openTips(t, 120, 20)

	model := send(tea.Model(tips), "/", "ㅌ")

	filter := model.(viewTipsFilter)
	assert.Equal(t, "ㅌ", filter.input.text)
}
