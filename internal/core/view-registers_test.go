package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// newRegistersView 는 register 를 몇 개 채운 뒤 판을 연 상태다.
//
// `yy` 로 `"0` 을, `dd` 둘로 `"1` `"2` 를 채운다. 무명은 마지막으로 지운 것이다.
func newRegistersView(t *testing.T, width, height int) viewRegisters {
	t.Helper()

	m := send(newTestEditor("one\ntwo\nthree\nfour", width, height),
		"y", "y", "d", "d", "d", "d")

	opened := send(m, ":", "r", "e", "g", "enter")

	here, ok := opened.(viewRegisters)
	require.True(t, ok, "`:reg` 로 판이 열려야 한다")

	return here
}

// registerRowsOf 는 화면에서 판의 행만 떼어낸다. 특수문자 판의 것과 같은 길이다.
func registerRowsOf(t *testing.T, m viewRegisters) []string {
	t.Helper()

	left, width := m.sidebarLeft(), m.textWidth()

	rows := []string{}
	for _, row := range strings.Split(m.View().Content, "\n") {
		plain := ansi.Strip(row)

		cut := ansi.Truncate(ansi.TruncateLeft(plain, left, ""), width, "")
		if strings.HasPrefix(cut, "┌") || strings.HasPrefix(cut, "│") || strings.HasPrefix(cut, "└") {
			rows = append(rows, cut)
		}
	}

	return rows
}

// 담긴 것만 보인다. 빈 자리를 늘 그리면 채워진 둘을 찾는 데 눈이 걸린다.
func TestRegistersShowsOnlyFilled(t *testing.T) {
	m := newRegistersView(t, 40, 20)

	rows := registerRowsOf(t, m)
	require.Len(t, rows, 4+registersFrame, "테두리 둘과 담긴 넷이다")

	assert.Contains(t, rows[1], `"" 줄   two⏎`)
	assert.Contains(t, rows[2], `"0 줄   one⏎`)
	assert.Contains(t, rows[3], `"1 줄   two⏎`)
	assert.Contains(t, rows[4], `"2 줄   one⏎`)
}

// 갈래를 글자로 적는다. 줄 단위인지가 `p` 를 어디에 붙일지를 정하는 값이다(ADR-0017).
func TestRegistersShowsKind(t *testing.T) {
	m := send(newTestEditor("foo bar", 40, 20), "d", "w")

	opened := send(m, ":", "r", "e", "g", "i", "s", "t", "e", "r", "s", "enter")

	here, ok := opened.(viewRegisters)
	require.True(t, ok)

	rows := registerRowsOf(t, here)
	require.Len(t, rows, 2+registersFrame)

	assert.Contains(t, rows[1], `"" 글자 foo `)
	assert.Contains(t, rows[2], `"1 글자 foo `)
}

// 하나도 없으면 그 사실을 한 줄로 알린다. 빈 상자는 「못 그렸나」로 읽힌다.
func TestRegistersEmpty(t *testing.T) {
	opened := send(newTestEditor("one\ntwo", 40, 20), ":", "r", "e", "g", "enter")

	here, ok := opened.(viewRegisters)
	require.True(t, ok)

	rows := registerRowsOf(t, here)
	require.Len(t, rows, 1+registersFrame)
	assert.Contains(t, rows[1], "담긴 register 가 없습니다")
}

// 판은 편집 영역의 행을 가져간다. 얹으면 커서가 그 밑에 가려진다(ADR-0056).
func TestRegistersTakesRowsFromText(t *testing.T) {
	m := send(newTestEditor("one\ntwo\nthree\nfour", 40, 20), "d", "d")

	before := m.(viewEditorNormal).textHeight()

	opened := send(m, ":", "r", "e", "g", "enter")

	here, ok := opened.(viewRegisters)
	require.True(t, ok)

	// 담긴 둘에 테두리 둘이다.
	assert.Equal(t, 2+registersFrame, here.drawerHeight)
	assert.Equal(t, before-here.drawerHeight, here.textHeight())
}

// 나가면 편집 영역이 온전히 돌아온다. drawerHeight 를 남기고 나가면 줄어든 채로 굳는다.
func TestRegistersLeaveRestoresText(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		t.Run(key, func(t *testing.T) {
			m := newRegistersView(t, 40, 20)
			before := m.paneHeight()

			next := send(m, key)

			require.IsType(t, viewEditorNormal{}, next)
			assert.Zero(t, next.(viewEditorNormal).drawerHeight, "판이 닫혀야 한다")
			assert.Equal(t, before, next.(viewEditorNormal).textHeight())
		})
	}
}

// 다른 화면으로 넘어가도 판이 닫힌다. 특수문자 판과 같은 자리다(view-symbol.go).
func TestRegistersClosesWhenJobChangesMode(t *testing.T) {
	m := newRegistersView(t, 80, 20)
	require.NotZero(t, m.drawerHeight)

	before := m.paneHeight()

	next, _ := m.Update(definitionMsg{locations: []lsp.Location{
		{URI: "file:///a.go"},
		{URI: "file:///b.go"},
	}})

	require.IsType(t, viewLocations{}, next, "다른 화면으로 넘어가야 하는 시험이다")

	assert.Zero(t, m.drawerHeight, "판이 닫혀야 한다")
	assert.Equal(t, before, m.textHeight(), "편집 영역이 돌아와야 한다")
}

// 화면이 낮으면 들어가는 만큼만 그리고 `j`/`k` 로 훑는다. 조용히 감추지 않는다.
func TestRegistersScrollsWhenShort(t *testing.T) {
	// 편집 내용 여섯 줄이면 paneHeight 가 6 이라 목록에 한 줄만 남는다.
	m := newRegistersView(t, 40, 6)
	require.Equal(t, 1, m.registersRows())

	rows := registerRowsOf(t, m)
	require.Len(t, rows, 1+registersFrame)
	assert.Contains(t, rows[1], `"" 줄   two⏎`)

	scrolled := send(m, "j", "j")

	here, ok := scrolled.(viewRegisters)
	require.True(t, ok)
	assert.Contains(t, registerRowsOf(t, here)[1], `"1 줄   two⏎`)

	// 양끝에서 멈춘다. 둘러 가면 목록의 끝이 어디인지 알 수 없다.
	end := send(here, "j", "j", "j", "j")
	assert.Contains(t, registerRowsOf(t, end.(viewRegisters))[1], `"2 줄   one⏎`)
}

// 판의 행은 저마다 정확히 편집 영역 폭이다. 한 칸이라도 넘치면 터미널이 접어서 아래가 밀린다.
func TestRegistersRowWidth(t *testing.T) {
	for _, width := range []int{20, 33, 40, 80} {
		m := newRegistersView(t, width, 20)

		// 합성하기 전의 판을 그대로 잰다. 화면에서 떼어 오면 넘친 부분이 이미 잘려 있다.
		for i, row := range strings.Split(m.renderDrawer(), "\n") {
			assert.Equal(t, m.textWidth(), ansi.StringWidth(row), "폭 %d 의 %d 행: %q", width, i, row)
		}
	}
}

// 한글을 되돌린다. 입력줄이 없는 판이라 `ㅓ` 가 `j` 여야 한다(ADR-0008).
func TestRegistersHangulKeys(t *testing.T) {
	m := newRegistersView(t, 40, 6)

	scrolled := send(m, "ㅓ")

	here, ok := scrolled.(viewRegisters)
	require.True(t, ok)
	assert.Contains(t, registerRowsOf(t, here)[1], `"0 줄   one⏎`)
}

// 팔레트의 「register 목록」도 같은 길이다.
func TestRegistersFromPalette(t *testing.T) {
	m := newTestEditor("one\ntwo", 40, 20)

	var opened tea.Model = m
	opened = send(opened, "ctrl+p", ">", "r", "e", "g", "i", "s", "t", "e", "r", "s", "enter")

	assert.IsType(t, viewRegisters{}, opened)
}

// 문자 register 도 목록에 보인다. 무명·숫자·문자 순이다(ADR-0058).
func TestRegistersShowsNamed(t *testing.T) {
	// `dd` 로 숫자 링을, `"byy` 로 문자를 채운다.
	m := send(newTestEditor("one\ntwo\nthree", 40, 20), "d", "d", `"`, "b", "y", "y")

	opened := send(m, ":", "r", "e", "g", "enter")

	here, ok := opened.(viewRegisters)
	require.True(t, ok)

	rows := registerRowsOf(t, here)
	require.Len(t, rows, 3+registersFrame)

	assert.Contains(t, rows[1], `"" 줄   two⏎`)
	assert.Contains(t, rows[2], `"1 줄   one⏎`)
	assert.Contains(t, rows[3], `"b 줄   two⏎`)
}

// 대문자 자리는 없다. `"A` 는 `"a` 와 같은 줄에 모인다(register.go 의 storeNamed).
func TestRegistersHasNoUppercaseRow(t *testing.T) {
	m := send(newTestEditor("one\ntwo", 40, 20), `"`, "a", "y", "y", "j", `"`, "A", "y", "y")

	opened := send(m, ":", "r", "e", "g", "enter")

	here, ok := opened.(viewRegisters)
	require.True(t, ok)

	rows := registerRowsOf(t, here)
	require.Len(t, rows, 2+registersFrame, "무명과 `\"a` 둘이다")
	assert.Contains(t, rows[2], `"a 줄   one⏎two⏎`)
}
