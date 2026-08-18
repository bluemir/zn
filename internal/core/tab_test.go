package core

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTabsEditor 는 파일 여러 개를 연 편집기다. CLI 인자로 여러 파일을 준 것과 같다.
func newTabsEditor(paths ...string) viewEditorNormal {
	buffers := make([]Buffer, 0, len(paths))
	for _, path := range paths {
		buffers = append(buffers, newBuffer(path, []byte("a\nb\nc\n")))
	}

	return viewEditorNormal{
		editor: &editor{
			boxChars: boxUnicode,
			buffers:  buffers,
			width:    40,
			height:   5 + tablineHeight + statusBarHeight,
		},
	}
}

// splitByReverse 는 반전 색이 입혀진 구간과 그렇지 않은 구간을 갈라서 이어 붙인다.
// 어디에 색이 갔는지를 보는 검사에 쓴다. 색 자체는 lipgloss 가 `ESC[7m` 으로 낸다.
func splitByReverse(raw string) (plain, reversed string) {
	rest := raw
	for {
		before, after, found := strings.Cut(rest, "\x1b[7m")
		plain += before
		if !found {
			return plain, reversed
		}

		inner, tail, _ := strings.Cut(after, "\x1b[m")
		reversed += inner
		rest = tail
	}
}

// activeOf 는 지금 보고 있는 tab 의 index 다.
func activeOf(t *testing.T, m tea.Model) int {
	t.Helper()

	v, ok := m.(viewEditorNormal)
	require.True(t, ok, "normal mode 가 아니다: %T", m)

	return v.active
}

func TestTabNextWraps(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt")

	m = send(m, "g", "t")
	assert.Equal(t, 1, activeOf(t, m))

	m = send(m, "g", "t")
	assert.Equal(t, 2, activeOf(t, m))

	m = send(m, "g", "t")
	assert.Equal(t, 0, activeOf(t, m), "마지막에서 처음으로 둘러 간다")
}

func TestTabPrevWraps(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt")

	m = send(m, "g", "T")
	assert.Equal(t, 2, activeOf(t, m), "처음에서 마지막으로 둘러 간다")

	m = send(m, "g", "T")
	assert.Equal(t, 1, activeOf(t, m))
}

// tab 하나뿐이면 gt 는 제자리다.
func TestTabSwitchWithSingleTab(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "g", "t")

	assert.Equal(t, 0, activeOf(t, m))
}

// tab 전환의 핵심이다. 파일마다 보던 자리가 그대로 남아야 한다.
func TestTabSwitchKeepsCursorPerFile(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	// 첫 tab 에서 두 줄 내려간다.
	m = send(m, "down", "down")
	require.Equal(t, 2, bufferOf(t, m).cursorLine)

	m = send(m, "g", "t")
	require.Equal(t, 1, activeOf(t, m))
	assert.Equal(t, 0, bufferOf(t, m).cursorLine, "새 tab 은 자기 자리에서 시작한다")

	m = send(m, "down")
	require.Equal(t, 1, bufferOf(t, m).cursorLine)

	m = send(m, "g", "T")
	assert.Equal(t, 2, bufferOf(t, m).cursorLine, "돌아오면 보던 자리 그대로다")
}

// 접두 키 뒤에 짝이 없는 키가 오면 아무 일도 없고 접두 키는 풀린다.
func TestPendingPrefixDiscardsUnknownKey(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "g", "z")
	assert.Equal(t, 0, activeOf(t, m), "짝이 없는 조합은 버린다")

	m = send(m, "g", "t")
	assert.Equal(t, 1, activeOf(t, m), "접두 키가 남아 있지 않다")
}

// 접두 키를 기다리는 동안 눌린 esc 는 접두 키를 무를 뿐이다.
func TestPendingPrefixCancelledByEscape(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "g", "esc")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 0, activeOf(t, m))
}

// g 뒤에 손이 미끄러져서 편집기가 꺼지면 안 된다.
func TestPendingPrefixSwallowsCtrlC(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "g")
	m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	assert.IsType(t, viewEditorNormal{}, m)
}

func TestTablineShowsAllFiles(t *testing.T) {
	m := newTabsEditor("dir/a.txt", "dir/b.txt")

	line := tablineOf(t, m.View())

	assert.Equal(t, " 1 a.txt │ 2 b.txt", line, "경로가 아니라 파일 이름만 쓴다")
}

// 보고 있는 tab 만 편집 내용과 같은 색이고 나머지는 반전이다.
// 활성 tab 이 아래 내용과 이어져 보이는 것이 tab 이라는 비유 자체다.
func TestTablineHighlightMovesWithActiveTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	plain, reversed := splitByReverse(rawTablineOf(t, m.View()))
	assert.Equal(t, " 1 a.txt ", plain, "활성 tab 만 색이 없다")
	assert.Contains(t, reversed, " 2 b.txt ")

	m = send(m, "g", "t")

	plain, reversed = splitByReverse(rawTablineOf(t, m.View()))
	assert.Equal(t, " 2 b.txt ", plain, "색이 옮겨 간 tab 을 따라간다")
	assert.Contains(t, reversed, " 1 a.txt ")
}

// 남은 칸까지 반전이어야 줄 전체가 한 덩어리로 보인다.
func TestTablineReverseSpansFullWidth(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")

	plain, reversed := splitByReverse(rawTablineOf(t, m.View()))

	assert.Equal(t, m.width, screenColAt([]byte(plain+reversed), len(plain+reversed)))
	assert.True(t, strings.HasSuffix(reversed, "  "), "빈 칸도 칠해진다")
}

func TestTablineShowsDirtyMark(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")
	require.NotContains(t, tablineOf(t, m.View()), "+")

	m = send(m, "i", "X", "esc")

	assert.Equal(t, " 1 a.txt+ │ 2 b.txt", tablineOf(t, m.View()))
}

// 다른 tab 의 변경도 tabline 에 보여야 한다. 그것 때문에 종료가 막히기 때문이다.
func TestTablineShowsDirtyMarkOfInactiveTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, "g", "t")

	assert.Equal(t, " 1 a.txt+ │ 2 b.txt", tablineOf(t, m.View()))
}

func TestTablineShowsNoNameForEmptyPath(t *testing.T) {
	m := viewEditorNormal{
		editor: &editor{
			buffers: []Buffer{newEmptyBuffer("")},
			width:   40,
			height:  3 + tablineHeight + statusBarHeight,
		},
	}

	assert.Equal(t, " 1 [No Name]", tablineOf(t, m.View()))
}

// tabline 이 화면 너비를 넘으면 터미널이 줄바꿈해서 화면이 밀린다.
func TestTablineTruncatesToWidth(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.width = 12

	line := tablineOf(t, m.View())

	assert.LessOrEqual(t, screenColAt([]byte(line), len(line)), 12)
}

// 넘치는 tab 은 양끝 표시가 알린다. `<n` 은 왼쪽으로, `n>` 는 오른쪽으로 그만큼 더 있다는 뜻이다.
func TestTablineShowsHiddenCount(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.width = 30

	assert.Equal(t, " 1 a.txt │ 2 b.txt │3>", tablineOf(t, m.View()))
}

// 활성 tab 이 오른쪽 끝에 있으면 안 보이던 것을 민다.
// 한 번에 한 tab 씩만 밀어야 tabline 이 덜 흔들린다.
func TestTablineScrollsToActiveTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.(viewEditorNormal).editor.width = 30

	m = send(m, "g", "t")
	assert.Equal(t, " 1 a.txt │ 2 b.txt │3>", tablineOf(t, m.(viewEditorNormal).View()),
		"보이는 자리로 옮겼으면 밀지 않는다")

	m = send(m, "g", "t")
	assert.Equal(t, "<1│ 2 b.txt │ 3 c.txt │2>", tablineOf(t, m.(viewEditorNormal).View()),
		"화면 밖으로 나간 만큼만 민다")

	m = send(m, "g", "t")
	assert.Equal(t, "<2│ 3 c.txt │ 4 d.txt │1>", tablineOf(t, m.(viewEditorNormal).View()))

	m = send(m, "g", "t")
	assert.Equal(t, "<3│ 4 d.txt │ 5 e.txt", tablineOf(t, m.(viewEditorNormal).View()),
		"마지막 tab 까지 왔으면 오른쪽 표시가 없다")

	// 처음으로 둘러 가면 스크롤도 처음으로 돌아온다.
	m = send(m, "g", "t")
	assert.Equal(t, " 1 a.txt │ 2 b.txt │3>", tablineOf(t, m.(viewEditorNormal).View()))
}

// 반쯤 걸친 tab 은 그리지 않는다. 잘린 이름은 어느 파일인지 알려주지 못한다.
func TestTablineDoesNotDrawPartialTab(t *testing.T) {
	for width := 10; width <= 40; width++ {
		m := newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
		m.width = width
		m.active = 2
		m.scrollTabsTo()

		line := tablineOf(t, m.View())

		assert.LessOrEqual(t, screenWidthOf(line), width, "width=%d", width)
		assert.Contains(t, line, " 3 c.txt", "활성 tab 은 온전히 보인다: width=%d", width)

		// 줄 끝 빈 칸은 tablineOf 가 떼므로 마지막 tab 은 뒷 칸이 없다.
		for _, piece := range strings.Split(line, "│") {
			if strings.HasPrefix(piece, " ") {
				assert.Regexp(t, `^ \d [a-e]\.txt ?$`, piece, "이름이 잘린 tab 이 있다: %q", line)
			}
		}
	}
}

// tab 을 닫아 오른쪽에 자리가 남으면 왼쪽에 가려둔 것을 도로 보여준다.
func TestTablineScrollsBackWhenRoomAppears(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.(viewEditorNormal).editor.width = 30

	m = send(m, "g", "t", "g", "t", "g", "t")
	require.Equal(t, "<2│ 3 c.txt │ 4 d.txt │1>", tablineOf(t, m.(viewEditorNormal).View()))

	m = send(m, ":", "q", "enter") // 4 d.txt 를 닫는다
	m = send(m, ":", "q", "enter") // 그 자리에 드러난 5 e.txt 를 닫는다

	assert.Equal(t, " 1 a.txt │ 2 b.txt │ 3 c.txt", tablineOf(t, m.(viewEditorNormal).View()),
		"셋만 남아 다 들어가므로 가려짐 표시도 없다")
}

// 화면이 넓어지면 밀어둔 것이 도로 보인다.
func TestTablineScrollsBackOnResize(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt", "d.txt", "e.txt")
	m.(viewEditorNormal).editor.width = 30

	m = send(m, "g", "t", "g", "t", "g", "t")
	require.Equal(t, "<2│ 3 c.txt │ 4 d.txt │1>", tablineOf(t, m.(viewEditorNormal).View()))

	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})

	assert.Equal(t, " 1 a.txt │ 2 b.txt │ 3 c.txt │ 4 d.txt │ 5 e.txt",
		tablineOf(t, m.(viewEditorNormal).View()))
}

// 화면이 tabline 과 statusBar 를 합친 것보다 작아도 죽지 않아야 한다.
func TestTinyScreenWithTablineDoesNotPanic(t *testing.T) {
	for _, height := range []int{0, 1, 2, 3, 4} {
		m := newTabsEditor("a.txt", "b.txt")
		m.height = height

		assert.NotPanics(t, func() { m.View() }, "height=%d", height)
	}
}

// :tabnew 는 이름 없는 빈 tab 을 열고 그리로 옮긴다.
func TestNewTabOpensEmptyBuffer(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 2)
	assert.Equal(t, 1, activeOf(t, m), "새로 만든 tab 을 본다")
	assert.Equal(t, " 1 a.txt │ 2 [No Name]", tablineOf(t, m.(viewEditorNormal).View()))

	buf := bufferOf(t, m)
	assert.Equal(t, [][]byte{{}}, buf.lines, "빈 줄 하나로 시작한다")
	assert.False(t, buf.dirty, "만들기만 한 것은 변경이 아니다")
}

// 맨 뒤가 아니라 보고 있던 tab 바로 뒤에 생긴다. vim 과 같다.
func TestNewTabIsInsertedAfterActive(t *testing.T) {
	wide := newTabsEditor("a.txt", "b.txt", "c.txt")
	wide.width = 60 // tab 넷이 잘리지 않을 만큼

	var m tea.Model = wide

	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")

	assert.Equal(t, 1, activeOf(t, m))
	assert.Equal(t, " 1 a.txt │ 2 [No Name] │ 3 b.txt │ 4 c.txt",
		tablineOf(t, m.(viewEditorNormal).View()))
}

// 새 tab 은 다른 tab 의 커서를 건드리지 않는다.
func TestNewTabKeepsOtherCursors(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "down", "down")
	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")
	require.Equal(t, 0, bufferOf(t, m).cursorLine, "새 tab 은 맨 위에서 시작한다")

	m = send(m, "g", "T")

	assert.Equal(t, 2, bufferOf(t, m).cursorLine)
}

// 이름이 없으면 쓸 곳이 없다. vim 의 E32 와 같다.
func TestWriteWithoutNameFails(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")
	m = send(m, "i", "X", "esc")
	m = send(m, ":", "w", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "파일 이름이 없습니다")
	assert.True(t, bufferOf(t, m).dirty, "저장되지 않았다")
}

// :wq 도 같은 곳을 지나므로 저장에 실패하면 tab 을 닫지 않는다.
func TestWriteQuitWithoutNameKeepsTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")
	m = send(m, "i", "X", "esc")
	m = send(m, ":", "w", "q", "enter")

	require.IsType(t, viewEditorNormal{}, m, "실패하면 닫지 않는다")
	assert.Len(t, m.(viewEditorNormal).buffers, 2)
	assert.Contains(t, barOf(t, m)[1], "파일 이름이 없습니다")
}

// 저장할 수 없어도 잃을 것이 있으면 닫을 때 물어야 한다.
func TestCloseDirtyUnnamedTabConfirms(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")
	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "enter")

	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("enter"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
}

// 손대지 않은 빈 tab 은 잃을 것이 없으므로 묻지 않고 닫힌다.
func TestCloseCleanUnnamedTabDoesNotConfirm(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "t", "a", "b", "n", "e", "w", "enter")
	m = send(m, ":", "q", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, " 1 a.txt", tablineOf(t, m.(viewEditorNormal).View()))
}

func TestCloseTabKeepsOtherTabs(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt")

	m = send(m, "g", "t")
	m = send(m, ":", "q", "enter")

	require.IsType(t, viewEditorNormal{}, m, "다른 tab 이 남아 있으면 종료하지 않는다")

	v := m.(viewEditorNormal)
	assert.Len(t, v.buffers, 2)
	assert.Equal(t, " 1 a.txt │ 2 c.txt", tablineOf(t, v.View()), "닫은 자리의 다음 tab 을 본다")
}

// 마지막 tab 을 닫으면 왼쪽으로 간다.
func TestCloseLastTabMovesActiveLeft(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "g", "t")
	m = send(m, ":", "q", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 0, activeOf(t, m))
	assert.Equal(t, " 1 a.txt", tablineOf(t, m.View()))
}

// tab 이 하나뿐이면 :q 는 종료다.
func TestCloseOnlyTabExits(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = send(m, ":", "q", "enter")

	assert.IsType(t, finalExit{}, m)
}

// :q 는 지금 보고 있는 tab 의 변경만 묻는다. 다른 tab 의 변경은 남으므로 묻지 않는다.
func TestCloseTabConfirmsWhenDirty(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "enter")

	require.IsType(t, viewConfirmDiscard{}, m)
	assert.Contains(t, m.View().Content, "이 tab 을 닫으시겠습니까?")

	m, _ = m.Update(key("enter"))

	require.IsType(t, viewEditorNormal{}, m, "종료가 아니라 tab 만 닫는다")
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
}

// 확인창에서 취소하면 tab 이 그대로 남아 있어야 한다.
func TestCloseTabCancelKeepsTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "enter")
	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("esc"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 2)
	assert.True(t, bufferOf(t, m).dirty)
}

// 다른 tab 이 깨끗하면 :q 는 묻지 않고 닫는다.
func TestCloseCleanTabDoesNotConfirm(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "g", "t")
	m = send(m, ":", "q", "enter")

	assert.IsType(t, viewEditorNormal{}, m)
}

// :q! 는 묻지 않고 tab 을 닫는다. 마지막 tab 이 아니면 종료가 아니다.
func TestForceCloseTabKeepsOtherTabs(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "!", "enter")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
}

// pickCloseOtherTabs 는 팔레트에서 「다른 tab 모두 닫기」를 골라 실행한다.
// 팔레트를 여는 길은 view-palette_test.go 가 보고, 여기는 고른 뒤에 tab 이 어떻게 되는지만 본다.
func pickCloseOtherTabs(t *testing.T, m tea.Model) tea.Model {
	t.Helper()

	v, ok := m.(viewEditorNormal)
	require.True(t, ok, "normal mode 가 아니다: %T", m)

	palette := viewPalette{editor: v.editor, input: "> close other"}
	palette.filter()

	require.Equal(t, "다른 tab 모두 닫기", paletteCommands[palette.hits[palette.selected].index].name)

	return send(palette, "enter")
}

// 「다른 tab 모두 닫기」는 보고 있는 tab 만 남긴다.
func TestCloseOtherTabsKeepsActiveTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt", "c.txt")

	m = send(m, "g", "t")
	m = pickCloseOtherTabs(t, m)

	require.IsType(t, viewEditorNormal{}, m)
	v := m.(viewEditorNormal)
	assert.Equal(t, 0, v.active)
	assert.Equal(t, " 1 b.txt", tablineOf(t, v.View()), "보고 있던 tab 이 남는다")
	assert.Equal(t, "2 개의 tab 을 닫았습니다", v.message)
}

// tab 이 하나뿐이면 닫을 것이 없다. 확인창도 뜨지 않는다.
func TestCloseOtherTabsWithSingleTab(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt")

	m = pickCloseOtherTabs(t, m)

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
	assert.Equal(t, "닫을 다른 tab 이 없습니다", m.(viewEditorNormal).message)
}

// 보고 있지 않은 tab 의 변경을 잃게 되므로 묻는다.
func TestCloseOtherTabsConfirmsWhenAnotherTabIsDirty(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, "g", "t")
	require.False(t, bufferOf(t, m).dirty, "지금 보고 있는 tab 은 깨끗하다")

	m = pickCloseOtherTabs(t, m)

	require.IsType(t, viewConfirmDiscard{}, m)
	assert.Contains(t, m.View().Content, "다른 tab 을 모두 닫으시겠습니까?")

	m, _ = m.Update(key("enter"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
	assert.Equal(t, "b.txt", bufferOf(t, m).path)
}

// 확인창에서 취소하면 tab 이 그대로 남는다. 팔레트가 아니라 normal 로 돌아간다.
func TestCloseOtherTabsCancelKeepsTabs(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, "g", "t")
	m = pickCloseOtherTabs(t, m)
	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("esc"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 2)
}

// 보고 있는 tab 의 변경은 잃지 않으므로 묻지 않는다.
func TestCloseOtherTabsDoesNotConfirmForActiveDirty(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = pickCloseOtherTabs(t, m)

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1)
	assert.True(t, bufferOf(t, m).dirty, "편집 중인 내용은 그대로다")
}

// Ctrl+C 는 :qa 다. 보고 있지 않은 tab 의 변경도 같이 잃으므로 그것까지 봐야 한다.
func TestCtrlCConfirmsWhenAnotherTabIsDirty(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, "g", "t")
	require.False(t, bufferOf(t, m).dirty, "지금 보고 있는 tab 은 깨끗하다")

	m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	require.IsType(t, viewConfirmDiscard{}, m)
	assert.Contains(t, m.View().Content, "정말 종료 하시겠습니까?")
}

func TestCommandQuitAllConfirmsWhenAnotherTabIsDirty(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, "g", "t")
	m = send(m, ":", "q", "a", "enter")

	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("enter"))
	assert.IsType(t, finalExit{}, m, "Yes 는 tab 을 닫는 것이 아니라 종료다")
}

func TestCommandQuitAllWhenClean(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, ":", "q", "a", "enter")

	assert.IsType(t, finalExit{}, m)
}

func TestCommandForceQuitAll(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "a", "!", "enter")

	assert.IsType(t, finalExit{}, m)
}

// 다른 tab 의 편집이 남아 있으면 확인창에서 취소했을 때 그 편집이 살아 있어야 한다.
func TestQuitAllCancelKeepsAllBuffers(t *testing.T) {
	var m tea.Model = newTabsEditor("a.txt", "b.txt")

	m = send(m, "i", "X", "esc")
	m = send(m, "g", "t")
	m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("esc"))

	require.IsType(t, viewEditorNormal{}, m)
	v := m.(viewEditorNormal)
	require.Len(t, v.buffers, 2)
	assert.True(t, v.buffers[0].dirty)
	assert.Equal(t, "Xa", string(v.buffers[0].lines[0]))
}

func TestCloseTabAfterWriteQuit(t *testing.T) {
	first, path := newFileEditor(t, "abc\n")
	second := newBuffer("other.txt", []byte("x\n"))

	var m tea.Model = viewEditorNormal{
		editor: &editor{
			buffers: append(first.buffers, second),
			width:   40,
			height:  5 + tablineHeight + statusBarHeight,
		},
	}

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "w", "q", "enter")

	require.IsType(t, viewEditorNormal{}, m, "다른 tab 이 남아 있으면 종료하지 않는다")
	assert.Len(t, m.(viewEditorNormal).buffers, 1)

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(saved))
}

// 편집 영역은 tabline 만큼 줄어들지만 statusBar 는 화면 맨 아래에 붙어 있어야 한다.
func TestTablineDoesNotPushOutStatusBar(t *testing.T) {
	m := newTabsEditor("a.txt", "b.txt")

	rows := strings.Split(m.View().Content, "\n")

	require.Len(t, rows, m.height)
	assert.Equal(t, " 1 a.txt │ 2 b.txt", strings.TrimRight(ansi.Strip(rows[0]), " "))
	assert.Contains(t, rows[len(rows)-statusBarHeight], "NORMAL")
}
