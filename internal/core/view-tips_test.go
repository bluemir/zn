package core

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// openTips 는 `:tips` 로 tip 목록을 연 화면이다.
func openTips(t *testing.T, width, height int) viewTips {
	t.Helper()

	m := newTestEditor("hello\nworld\n", width, height)

	model := send(tea.Model(m), ":", "t", "i", "p", "s", "enter")

	tips, ok := model.(viewTips)
	require.True(t, ok, "tip 목록이 열린다: %T", model)

	return tips
}

// tipLinesOf 는 화면을 색 없는 줄로 쪼갠다.
func tipLinesOf(view tea.View) []string {
	out := []string{}
	for _, line := range strings.Split(view.Content, "\n") {
		out = append(out, ansi.Strip(line))
	}

	return out
}

// `:tips` 는 맨 위에서 연다. 제목줄이 전부 몇 개인지 말한다.
func TestTipsOpensAtTop(t *testing.T) {
	tips := openTips(t, 120, 20)

	assert.Equal(t, 0, tips.selected)
	assert.Len(t, tips.rows(), len(assets.Tips))

	lines := tipLinesOf(tips.View())
	assert.Contains(t, lines[0], fmt.Sprintf("tip  %d 개", len(assets.Tips)))
	assert.Contains(t, lines[1], assets.Tips[0], "첫 문장이 첫 줄이다")
	assert.Contains(t, lines[1], "▸", "고른 줄에 표시가 선다")
}

// 팔레트로 열어도 같은 화면이다.
func TestTipsOpensFromPalette(t *testing.T) {
	m := newTestEditor("hello\n", 120, 20)

	model, _ := runTips(m.editor)

	assert.IsType(t, viewTips{}, model)
}

// 번호는 assets.Tips 안의 자리다. 1 부터 센다.
func TestTipsNumbersRowsFromOne(t *testing.T) {
	tips := openTips(t, 120, 20)

	rows := tips.rows()
	require.NotEmpty(t, rows)
	assert.Equal(t, 1, rows[0].at)
	assert.Equal(t, len(assets.Tips), rows[len(rows)-1].at)
}

// j/k 로 오르내리고 g/G 로 양끝에 선다. 다른 목록 화면들과 같은 키다.
func TestTipsMovesWithKeys(t *testing.T) {
	tips := openTips(t, 120, 20)

	model := send(tea.Model(tips), "j", "j")
	assert.Equal(t, 2, model.(viewTips).selected)

	model = send(model, "k")
	assert.Equal(t, 1, model.(viewTips).selected)

	model = send(model, "G")
	assert.Equal(t, len(assets.Tips)-1, model.(viewTips).selected, "끝으로 간다")

	model = send(model, "g")
	assert.Equal(t, 0, model.(viewTips).selected, "처음으로 돌아온다")
}

// 한글 입력 상태의 키도 두벌식 자리로 되돌려 받는다. 입력줄이 없는 화면이다(ADR-0008).
func TestTipsAcceptsHangulKeys(t *testing.T) {
	tips := openTips(t, 120, 20)

	model := send(tea.Model(tips), "ㅓ")

	assert.Equal(t, 1, model.(viewTips).selected, "`ㅓ` 가 `j` 다")
}

// `q` 와 `esc` 로 닫는다.
func TestTipsClosesWithQ(t *testing.T) {
	tips := openTips(t, 120, 20)

	assert.IsType(t, viewEditorNormal{}, send(tea.Model(tips), "q"))
	assert.IsType(t, viewEditorNormal{}, send(tea.Model(tips), "esc"))
}

// 휠은 목록을 오르내린다. 화면을 통째로 쓰므로 굴릴 다른 영역이 없다.
func TestTipsWheelMovesList(t *testing.T) {
	tips := openTips(t, 120, 20)

	model, _ := tips.Update(wheel(10, 5, false))
	assert.Equal(t, wheelRows, model.(viewTips).selected)

	model, _ = model.Update(wheel(10, 5, true))
	assert.Equal(t, 0, model.(viewTips).selected)
}

// 좁은 화면에서는 문장이 오른쪽부터 줄어든다. 줄이 화면보다 넓어지면 아래가 통째로 밀린다.
func TestTipsTrimsLongLines(t *testing.T) {
	tips := openTips(t, 40, 12)

	for at, line := range tipLinesOf(tips.View()) {
		assert.LessOrEqual(t, len([]rune(line)), 40*2, "행 %d", at)
	}
}
