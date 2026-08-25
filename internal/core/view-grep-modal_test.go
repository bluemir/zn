package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 팔레트의 「프로젝트 검색」은 아래 줄이 아니라 박스로 받는다.
//
// 박스를 보고 있다가 고른 것이니 박스로 받는 것이 맥락이 이어진다(ADR-0078 §7).
func TestPaletteOpensGrepModal(t *testing.T) {
	m := newTestEditor("abc\n", 80, 12)

	model, _ := runGrepModal(m.editor)

	require.IsType(t, viewGrepModal{}, model, "아래 줄 입력창이 아니다")
}

// 인자 없는 `:grep` 은 아래 줄에서 받는다. 명령줄에서 왔고 `/` 검색과 같은 자리다.
func TestBareGrepCommandUsesBottomLine(t *testing.T) {
	m := typeInto(newTestEditor("abc\n", 80, 12), ":grep")

	assert.IsType(t, viewGrepInput{}, send(m, "enter"))
}

// 박스는 팔레트가 서던 자리·같은 폭이다. 고른 박스가 있던 곳에 그대로 뜬다.
func TestGrepModalSitsWherePaletteWas(t *testing.T) {
	e := newTestEditor("abc\n", 80, 12).editor

	palette, _ := paletteMode(e)
	modal, _ := grepModalMode(e)

	require.IsType(t, viewGrepModal{}, modal)

	assert.Equal(t, palette.(viewPalette).paletteLeft(), modal.(viewGrepModal).paletteLeft())
	assert.Equal(t, palette.(viewPalette).paletteWidth(), modal.(viewGrepModal).paletteWidth())

	// 위 테두리에 제목이 얹히고 안쪽은 입력줄 하나뿐이다 — 팔레트보다 두 줄 낮다.
	rows := strings.Split(ansi.Strip(modal.(viewGrepModal).renderBox()), "\n")
	require.Len(t, rows, 3)
	assert.Contains(t, rows[0], "프로젝트 검색")
}

// 친 글자가 박스 안에 보이고 커서가 그 뒤에 선다.
func TestGrepModalShowsInputAndCursor(t *testing.T) {
	m := newGrepModal(t, 80, 12)

	typed := send(m, "f", "u", "n", "c").(viewGrepModal)

	assert.Equal(t, "func", typed.input)
	assert.Contains(t, ansi.Strip(typed.renderBox()), "func")

	view := typed.View()
	require.NotNil(t, view.Cursor)
	assert.Equal(t, typed.paletteLeft()+2+len("func"), view.Cursor.X, "커서가 친 글자 뒤에 선다")
	assert.Equal(t, paletteTop+1, view.Cursor.Y, "박스 안 입력줄이다")
}

// 넘치면 왼쪽부터 접는다. 치고 있는 것은 뒤쪽이라 오른쪽부터 자르면 방금 친 글자가 사라진다.
func TestGrepModalTrimsInputFromLeft(t *testing.T) {
	m := newGrepModal(t, 80, 12)
	m.input = strings.Repeat("a", 200) + "END"

	text := m.inputText()

	assert.True(t, strings.HasPrefix(text, "…"), "글: %q", text)
	assert.True(t, strings.HasSuffix(text, "END"), "방금 친 글자가 남는다: %q", text)
	assert.LessOrEqual(t, screenWidthOf(text), m.paletteWidth()-4, "박스 안에 든다")
}

// `esc` 는 그만두고 `backspace` 로 다 지워도 나간다. 팔레트·명령줄과 같은 손이다.
func TestGrepModalLeavesOnEscAndEmptyBackspace(t *testing.T) {
	m := newGrepModal(t, 80, 12)

	assert.IsType(t, viewEditorNormal{}, send(m, "esc"))
	assert.IsType(t, viewEditorNormal{}, send(m, "backspace"))

	// 글자가 있으면 한 글자만 지운다.
	typed := send(send(m, "a", "b"), "backspace")
	require.IsType(t, viewGrepModal{}, typed)
	assert.Equal(t, "a", typed.(viewGrepModal).input)
}

// 빈 채로 enter 는 알리고 만다. 저장소 전체를 찾는 빈 패턴은 뜻이 없다.
func TestGrepModalRefusesEmptyPattern(t *testing.T) {
	m := newGrepModal(t, 80, 12)

	next := send(m, "enter")

	require.IsType(t, viewEditorNormal{}, next)
	assert.Contains(t, next.(viewEditorNormal).notice, "검색할 패턴이 없습니다")
}

// 화면이 좁아 팔레트를 못 열면 이쪽도 못 연다. 같은 자리를 쓰므로 문턱이 하나다.
func TestGrepModalRefusesWhenPaletteDoesNotFit(t *testing.T) {
	e := newTestEditor("abc\n", 20, 4).editor
	require.False(t, e.paletteFits())

	model, _ := grepModalMode(e)

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "화면이 좁아")
}

// newGrepModal 은 열린 검색 박스다.
func newGrepModal(t *testing.T, width, height int) viewGrepModal {
	t.Helper()

	model, _ := grepModalMode(newTestEditor("abc\n", width, height).editor)

	m, ok := model.(viewGrepModal)
	require.True(t, ok, "박스가 열려야 한다: %T", model)

	return m
}
