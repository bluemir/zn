package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFileEditor 는 진짜 파일을 연 편집 화면을 만든다. 저장을 확인하려면 파일이 있어야 한다.
func newFileEditor(t *testing.T, data string) (viewEditorNormal, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte(data), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	return viewEditorNormal{
		editor: &editor{
			buffers: []viewport{buf},
			width:   40,
			height:  5 + tablineHeight + statusBarHeight,
		},
	}, path
}

func TestEnterCommandMode(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":")

	assert.IsType(t, viewEditorCommand{}, m)
	assert.Contains(t, barOf(t, m)[1], ":", "명령줄이 아래 줄에 뜬다")
}

func TestCommandModeTyping(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "w", "q")

	assert.Equal(t, ":wq", barOf(t, m)[1])
}

func TestCommandModeBackspace(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "w", "q", "backspace")

	assert.Equal(t, ":w", barOf(t, m)[1])
}

// `:` 까지 지우면 명령줄에서 나간다. vim 과 같다.
func TestCommandModeBackspaceLeavesMode(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "backspace")

	assert.IsType(t, viewEditorNormal{}, m)
}

func TestCommandModeEscapeCancels(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "w", "esc")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], ":", "커서 위치로 돌아간다")
}

// 명령줄이 편집 내용을 밀어내면 안 된다. 아래 줄을 바꿔 쓰는 것이다.
func TestCommandModeKeepsTextHeight(t *testing.T) {
	var m tea.Model = newTestEditor("a\nb\nc\nd\ne\n", 40, 5)
	before := strings.Count(m.(viewEditorNormal).View().Content, "\n")

	m = send(m, ":")

	assert.Equal(t, before, strings.Count(m.(viewEditorCommand).View().Content, "\n"))
	assert.Equal(t, "a", strings.Split(textOf(t, m), "\n")[0])
}

func TestCommandWriteSavesFile(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")

	var model tea.Model = m
	model = send(model, "i", "X", "esc")
	model = send(model, ":", "w", "enter")

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(saved))

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, barOf(t, model)[1], "저장함")
	assert.False(t, bufferOf(t, model).Dirty, "저장하면 변경 표시가 사라진다")
}

// 읽은 뒤 밖에서 바뀐 파일은 `:w` 로 덮어쓰이지 않고 알림만 뜬다.
func TestCommandWriteRefusesChangedFile(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")

	var model tea.Model = m
	model = send(model, "i", "X", "esc")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = send(model, ":", "w", "enter")

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.True(t, bufferOf(t, model).Dirty, "저장되지 않았다")

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "남이 쓴 것\n", string(saved))
}

// `:w!` 는 그것을 알고도 덮어쓴다.
func TestCommandForceWriteOverwritesChangedFile(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")

	var model tea.Model = m
	model = send(model, "i", "X", "esc")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = send(model, ":", "w", "!", "enter")

	assert.Contains(t, barOf(t, model)[1], "저장함")

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(saved))
}

// :wq 도 같은 곳을 지나므로 저장이 막히면 tab 을 닫지 않는다.
func TestCommandWriteQuitRefusesChangedFile(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")

	var model tea.Model = m
	model = send(model, "i", "X", "esc")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = send(model, ":", "w", "q", "enter")

	require.IsType(t, viewEditorNormal{}, model, "실패하면 닫지 않는다")
	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
}

func TestCommandQuitWhenClean(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "q", "enter")

	assert.IsType(t, viewEditorEmpty{}, m, "바뀐 것이 없으면 묻지 않고 닫는다")
}

// 저장하지 않은 변경이 있으면 :q 가 확인창을 띄운다. 확인창 자체는 Ctrl+C 와 같은 것이다.
func TestCommandQuitConfirmsWhenDirty(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "enter")

	assert.IsType(t, viewConfirmDiscard{}, m, "묻고 나서 닫는다")
	assert.Contains(t, m.View().Content, "저장하지 않은 변경")
}

// 확인창에서 취소하면 명령줄이 아니라 normal 로 돌아간다.
func TestCommandQuitCancelReturnsToNormal(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "enter")
	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("esc"))

	assert.IsType(t, viewEditorNormal{}, m)
}

// 확인창에서 Yes 를 고르면 나간다.
func TestQuitConfirmYesExits(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "esc")
	m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.IsType(t, viewConfirmDiscard{}, m)

	m, _ = m.Update(key("enter"))

	assert.IsType(t, finalExit{}, m)
}

func TestCommandForceQuit(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, "i", "X", "esc")
	m = send(m, ":", "q", "!", "enter")

	assert.IsType(t, viewEditorEmpty{}, m, "묻지 않고 닫는다")
}

func TestCommandWriteQuit(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")

	var model tea.Model = m
	model = send(model, "i", "X", "esc")
	model = send(model, ":", "w", "q", "enter")

	assert.IsType(t, viewEditorEmpty{}, model, "저장하고 닫는다. 마지막 tab 이어도 종료가 아니다")

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(saved))
}

func TestCommandUnknown(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "z", "z", "enter")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "알 수 없는 명령")
}

// 인자를 받는 명령은 `:w` `:e` `:tabnew` 뿐이다.
// 나머지에 붙은 인자를 조용히 버리면 `:wq foo` 가 foo 에 저장한 것처럼 보인다.
func TestCommandRejectsUnexpectedArgument(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")

	var model tea.Model = m
	model = send(model, "i", "X", "esc")
	model = send(model, ":", "w", "q", " ", "f", "o", "o", "enter")

	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, barOf(t, model)[1], "알 수 없는 명령")
	assert.True(t, bufferOf(t, model).Dirty, "저장하지 않는다")

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "abc\n", string(saved), "원래 파일도 건드리지 않는다")
}

// 앞뒤 공백은 명령 이름으로 치지 않는다.
func TestCommandIgnoresSurroundingSpaces(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", " ", "q", " ", "enter")

	assert.IsType(t, viewEditorEmpty{}, m)
}

// 따옴표가 닫히지 않으면 실행하지 않고 알린다.
func TestCommandReportsParseError(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "w", " ", "\"", "f", "o", "o", "enter")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "따옴표가 닫히지 않았습니다")
}

func TestCommandEmptyReturnsToNormal(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "enter")

	assert.IsType(t, viewEditorNormal{}, m)
}

// 저장할 수 없으면 종료하지 않고 알린다.
func TestCommandWriteFailure(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0555))

	buf, err := OpenBuffer(filepath.Join(dir, "sub", "new.txt"))
	require.NoError(t, err)

	var m tea.Model = viewEditorNormal{
		editor: &editor{buffers: []viewport{buf}, width: 40, height: 5 + tablineHeight + statusBarHeight},
	}
	m = send(m, ":", "w", "enter")

	assert.IsType(t, viewEditorNormal{}, m, "실패하면 종료하지 않는다")
	assert.NotContains(t, barOf(t, m)[1], "저장함")
}

// 알림은 다음 키를 누르면 사라진다.
func TestMessageClearsOnNextKey(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":", "z", "enter")
	require.Contains(t, barOf(t, m)[1], "알 수 없는 명령")

	m = send(m, "right")
	assert.NotContains(t, barOf(t, m)[1], "알 수 없는 명령")
}

// statusBar 에 변경 표시가 뜬다.
func TestStatusBarShowsDirtyMark(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)
	require.NotContains(t, barOf(t, m)[0], "[+]")

	m = send(m, "i", "X")

	assert.Contains(t, barOf(t, m)[0], "[+]")
}

func TestCommandModeShowsModeName(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 40, 5)

	m = send(m, ":")

	assert.Contains(t, barOf(t, m)[0], "COMMAND")
}
