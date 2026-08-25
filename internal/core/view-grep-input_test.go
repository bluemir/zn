package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// **패턴을 받는 자리는 하나다.** 팔레트의 「프로젝트 검색」과 인자 없는 `:grep` 이 같은
// 박스로 온다 — 어느 길로 왔는지는 친 사람이 알고 무엇을 치는지는 같다(ADR-0078 §7).
func TestBothPathsOpenTheSameGrepInput(t *testing.T) {
	viaPalette, _ := runGrepInput(newTestEditor("abc\n", 80, 12).editor)
	require.IsType(t, viewGrepInput{}, viaPalette)

	viaCommand := send(typeInto(newTestEditor("abc\n", 80, 12), ":grep"), "enter")
	require.IsType(t, viewGrepInput{}, viaCommand)
}

// 박스는 팔레트가 서던 자리·같은 폭이다. 고른 박스가 있던 곳에 그대로 뜬다.
func TestGrepInputSitsWherePaletteWas(t *testing.T) {
	e := newTestEditor("abc\n", 80, 12).editor

	palette, _ := paletteMode(e)
	modal, _ := grepInputMode(e)

	require.IsType(t, viewGrepInput{}, modal)

	assert.Equal(t, palette.(viewPalette).paletteLeft(), modal.(viewGrepInput).paletteLeft())
	assert.Equal(t, palette.(viewPalette).paletteWidth(), modal.(viewGrepInput).paletteWidth())

	// 위 테두리에 제목이 얹히고 안쪽은 입력줄 하나뿐이다 — 팔레트보다 두 줄 낮다.
	rows := strings.Split(ansi.Strip(modal.(viewGrepInput).renderBox()), "\n")
	require.Len(t, rows, 3)
	assert.Contains(t, rows[0], "프로젝트 검색")
}

// 친 글자가 박스 안에 보이고 커서가 그 뒤에 선다.
func TestGrepInputShowsInputAndCursor(t *testing.T) {
	m := newGrepInput(t, 80, 12)

	typed := send(m, "f", "u", "n", "c").(viewGrepInput)

	assert.Equal(t, "func", typed.input)
	assert.Contains(t, ansi.Strip(typed.renderBox()), "func")

	view := typed.View()
	require.NotNil(t, view.Cursor)
	assert.Equal(t, typed.paletteLeft()+2+len("func"), view.Cursor.X, "커서가 친 글자 뒤에 선다")
	assert.Equal(t, paletteTop+1, view.Cursor.Y, "박스 안 입력줄이다")
}

// 넘치면 왼쪽부터 접는다. 치고 있는 것은 뒤쪽이라 오른쪽부터 자르면 방금 친 글자가 사라진다.
func TestGrepInputTrimsInputFromLeft(t *testing.T) {
	m := newGrepInput(t, 80, 12)
	m.input = strings.Repeat("a", 200) + "END"

	text := m.inputText()

	assert.True(t, strings.HasPrefix(text, "…"), "글: %q", text)
	assert.True(t, strings.HasSuffix(text, "END"), "방금 친 글자가 남는다: %q", text)
	assert.LessOrEqual(t, screenWidthOf(text), m.paletteWidth()-4, "박스 안에 든다")
}

// `esc` 는 그만두고 `backspace` 로 다 지워도 나간다. 팔레트·명령줄과 같은 손이다.
func TestGrepInputLeavesOnEscAndEmptyBackspace(t *testing.T) {
	m := newGrepInput(t, 80, 12)

	assert.IsType(t, viewEditorNormal{}, send(m, "esc"))
	assert.IsType(t, viewEditorNormal{}, send(m, "backspace"))

	// 글자가 있으면 한 글자만 지운다.
	typed := send(send(m, "a", "b"), "backspace")
	require.IsType(t, viewGrepInput{}, typed)
	assert.Equal(t, "a", typed.(viewGrepInput).input)
}

// 빈 채로 enter 는 알리고 만다. 저장소 전체를 찾는 빈 패턴은 뜻이 없다.
func TestGrepInputRefusesEmptyPattern(t *testing.T) {
	m := newGrepInput(t, 80, 12)

	next := send(m, "enter")

	require.IsType(t, viewEditorNormal{}, next)
	assert.Contains(t, next.(viewEditorNormal).notice, "검색할 패턴이 없습니다")
}

// 폭이 좁으면 못 연다. 폭은 팔레트와 같은 자를 쓴다 — 같은 자리에 같은 폭으로 떠야 한다.
func TestGrepInputRefusesWhenTooNarrow(t *testing.T) {
	e := newTestEditor("abc\n", 20, 12).editor
	require.Less(t, e.paletteWidth(), paletteMinWidth)

	model, _ := grepInputMode(e)

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "화면이 좁아")
}

// **높이 문턱은 팔레트보다 낮다.** 팔레트는 목록 열 줄이 들어가는지를 재는데 이 박스에는
// 목록이 없어서, 그 자로 재면 그릴 수 있는데도 거절한다(ADR-0078 §7).
func TestGrepInputFitsWherePaletteDoesNot(t *testing.T) {
	e := newTestEditor("abc\n", 80, 4).editor

	require.False(t, e.paletteFits(), "팔레트는 목록 자리가 없어 못 열린다")
	require.True(t, e.grepInputFits(), "박스는 세 줄이라 들어간다")

	model, _ := grepInputMode(e)
	require.IsType(t, viewGrepInput{}, model)

	rows := strings.Split(ansi.Strip(model.(viewGrepInput).renderBox()), "\n")
	assert.Len(t, rows, grepInputRows)
}

// 높이가 정말 모자라면 거절한다.
func TestGrepInputRefusesWhenTooShort(t *testing.T) {
	e := newTestEditor("abc\n", 80, 1).editor
	require.False(t, e.grepInputFits())

	model, _ := grepInputMode(e)

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "화면이 좁아")
}

// newGrepInput 은 열린 검색 박스다.
func newGrepInput(t *testing.T, width, height int) viewGrepInput {
	t.Helper()

	model, _ := grepInputMode(newTestEditor("abc\n", width, height).editor)

	m, ok := model.(viewGrepInput)
	require.True(t, ok, "박스가 열려야 한다: %T", model)

	return m
}
