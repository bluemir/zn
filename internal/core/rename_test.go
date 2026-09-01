package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// edit 은 한 줄 안의 편집 하나다. 열은 UTF-16 이다(lsp/protocol.go).
func renameEdit(line, start, end int, text string) lsp.TextEdit {
	return lsp.TextEdit{
		NewText: text,
		Range: lsp.Range{
			Start: lsp.Position{Line: line, Character: start},
			End:   lsp.Position{Line: line, Character: end},
		},
	}
}

// 같은 줄에 편집이 여럿이면 뒤에서 앞으로 적용해야 한다. 앞부터 고치면 뒤 편집의 열이 밀린다.
func TestApplyEditsGoesBackward(t *testing.T) {
	lines := [][]byte{[]byte("aa bb aa")}

	next, done := applyEdits(lines, []lsp.TextEdit{
		renameEdit(0, 0, 2, "cccc"),
		renameEdit(0, 6, 8, "cccc"),
	})

	assert.Equal(t, 2, done)
	assert.Equal(t, "cccc bb cccc", string(next[0]))
	assert.Equal(t, "aa bb aa", string(lines[0]), "받은 줄은 제자리에서 바뀌지 않는다")
}

// 열이 UTF-16 이라 한글이 든 줄에서 byte 로 세면 어긋난다.
func TestApplyEditsCountsUTF16(t *testing.T) {
	lines := [][]byte{[]byte(`x := "한글" + name`)}

	// `name` 은 UTF-16 으로 12 번째 자리에서 시작한다. 한글 두 자가 byte 로는 여섯이라
	// byte 로 세면 이 자리가 어긋난다.
	next, done := applyEdits(lines, []lsp.TextEdit{renameEdit(0, 12, 16, "title")})

	assert.Equal(t, 1, done)
	assert.Equal(t, `x := "한글" + title`, string(next[0]))
}

// 이름은 줄을 넘지 않는다. 여러 줄짜리와 줄 밖은 건너뛴다.
func TestApplyEditsSkipsWhatItCannotDo(t *testing.T) {
	lines := [][]byte{[]byte("abc"), []byte("def")}

	next, done := applyEdits(lines, []lsp.TextEdit{
		{NewText: "x", Range: lsp.Range{
			Start: lsp.Position{Line: 0, Character: 0},
			End:   lsp.Position{Line: 1, Character: 1},
		}},
		renameEdit(9, 0, 1, "y"),
	})

	assert.Equal(t, 0, done)
	assert.Equal(t, "abc", string(next[0]))
	assert.Equal(t, "def", string(next[1]))
}

// 열려 있는 tab 은 buffer 에서 고치고 저장한다. 디스크에서 다시 읽으면 저장하지 않은 편집이 사라진다.
func TestRenameAppliesToOpenTab(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package p\n\nvar Greet = 1\n"), 0644))

	m := newTestEditorFile(path, "package p\n\nvar Greet = 1\n", 80, 6)

	done, err := m.editor.applyRenameTo(lsp.FileEdits{Path: path, Edits: []lsp.TextEdit{renameEdit(2, 4, 9, "Hello")}})
	require.NoError(t, err)
	assert.Equal(t, 1, done)

	assert.Equal(t, "var Hello = 1", string(m.editor.buffers[0].lines[2]), "buffer 가 바뀐다")
	assert.False(t, m.editor.buffers[0].dirty, "저장까지 한다")

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "package p\n\nvar Hello = 1\n", string(saved))
}

// 열려 있지 않은 파일은 그 자리에서 열어 고치고 쓴다. tab 으로 만들지 않는다.
func TestRenameWritesUnopenedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.go")
	require.NoError(t, os.WriteFile(path, []byte("package p\n\nvar _ = Greet\n"), 0644))

	e := &editor{}

	done, err := e.applyRenameTo(lsp.FileEdits{Path: path, Edits: []lsp.TextEdit{renameEdit(2, 8, 13, "Hello")}})
	require.NoError(t, err)
	assert.Equal(t, 1, done)

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "package p\n\nvar _ = Hello\n", string(saved))
	assert.Empty(t, e.buffers, "tab 이 생기지 않았다")
}

// 줄끝이 CRLF 인 파일은 CRLF 로 남는다. 여는 자리를 지나므로 공짜로 지켜진다(ADR-0052).
func TestRenameKeepsLineEnding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.go")
	require.NoError(t, os.WriteFile(path, []byte("package p\r\n\r\nvar _ = Greet\r\n"), 0644))

	e := &editor{}

	_, err := e.applyRenameTo(lsp.FileEdits{Path: path, Edits: []lsp.TextEdit{renameEdit(2, 8, 13, "Hello")}})
	require.NoError(t, err)

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "package p\r\n\r\nvar _ = Hello\r\n", string(saved))
}

// 서버가 보는 자리 밖이면 아예 하지 않는다. 반쪽짜리 rename 은 빌드를 조용히 깨뜨린다.
func TestRenameRefusesOutsideRoot(t *testing.T) {
	assert.True(t, underRoot("/work", "/work/pkg/a.go"))
	assert.True(t, underRoot("", "/anywhere/a.go"), "뿌리를 모르면 막지 않는다")
	assert.False(t, underRoot("/work", "/other/a.go"))
	assert.False(t, underRoot("/work", "/workshop/a.go"), "이름이 겹치는 옆 폴더도 밖이다")
}

// 답이 비면 조용히 알린다. 오류는 오류로 뜬다.
func TestFinishRenameMessages(t *testing.T) {
	e := &editor{}

	e.finishRename(renameMsg{newName: "Hello"})
	assert.Equal(t, "바꿀 자리가 없습니다", e.notice)
}

// 부르는 문 셋이 모두 커서 옆 창으로 온다. 창은 옛 이름으로 채워져 있다.
func TestRenameOpensPopupFromEveryDoor(t *testing.T) {
	for name, open := range map[string]func(e *editor) (tea.Model, tea.Cmd){
		"\\rn": func(e *editor) (tea.Model, tea.Cmd) {
			return send(tea.Model(viewEditorNormal{editor: e}), "\\", "r", "n"), nil
		},
		"팔레트": func(e *editor) (tea.Model, tea.Cmd) { return runRename(e) },
		":rename": func(e *editor) (tea.Model, tea.Cmd) {
			return send(tea.Model(viewEditorNormal{editor: e}), ":", "r", "e", "n", "a", "m", "e", "enter"), nil
		},
	} {
		e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor
		e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 4

		m, _ := open(e)

		require.IsType(t, viewRenameInput{}, m, name)
		assert.Equal(t, "Greet", m.(viewRenameInput).input.text, "%s: 옛 이름으로 채워진다", name)
		assert.Equal(t, "Greet", m.(viewRenameInput).old, name)
	}
}

// 이름을 대고 부르면 창을 거치지 않는다.
func TestRenameCommandWithNameSkipsPopup(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor

	m := send(tea.Model(viewEditorNormal{editor: e}), ":", "r", "e", "n", "a", "m", "e", " ", "H", "i", "enter")

	assert.IsType(t, viewEditorNormal{}, m, "창을 거치지 않고 곧바로 묻는 길로 간다")
	assert.Contains(t, e.notice, "gopls 를 띄우는 중입니다",
		"시험에는 서버가 없어서 startRename 이 여기서 멈춘다")
}

// 커서를 그 이름의 첫 글자로 옮긴다. 서버에 묻는 자리가 커서라 화면과 맞아야 한다.
func TestRenamePopupMovesCursorToWord(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\n\tvar Greet = 1\n", 80, 6).editor
	e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 0

	m, _ := renameInputMode(e)

	require.IsType(t, viewRenameInput{}, m)
	assert.Equal(t, len("\t"), e.buffers[0].cursorCol, "낱말 첫 글자로 옮겼다")
	assert.Equal(t, "var", m.(viewRenameInput).old,
		"커서가 들여쓰기 위면 그 줄의 첫 낱말이다 — vim 의 `*` 와 같다(search.go)")
}

// 바꿀 이름이 없는 줄에서는 열리지 않는다.
func TestRenamePopupNeedsWord(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\n\n", 80, 6).editor
	e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 0

	m, _ := renameInputMode(e)

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, "바꿀 이름이 없습니다", e.notice)
}

// 창에서 치고 지우고 그만둘 수 있다.
func TestRenamePopupTyping(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor
	e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 4

	m, _ := renameInputMode(e)

	m = send(m, "backspace", "backspace", "e", "d")
	assert.Equal(t, "Greed", m.(viewRenameInput).input.text)

	assert.IsType(t, viewEditorNormal{}, send(m, "esc"), "esc 는 그만두기다")
}

// **긴 이름을 쳐도 방금 친 글자가 보인다.** 오른쪽부터 자르면 치고 있는 뒤쪽이 사라지고,
// 커서가 창 밖으로 나가 화면 끝에서 다음 줄로 감긴다 — 실제로 그렇게 되었다.
func TestRenamePopupKeepsTailVisible(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor
	e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 4

	m, _ := renameInputMode(e)
	require.IsType(t, viewRenameInput{}, m)

	long := m.(viewRenameInput)
	long.input = newInputLine(strings.Repeat("Long", 20))

	text, cursor := long.renameLine()

	assert.True(t, strings.HasSuffix(text, "Long"), "방금 친 끝이 남는다: %q", text)
	assert.True(t, strings.HasPrefix(text, "…"), "접은 자리를 표시한다: %q", text)
	assert.LessOrEqual(t, widthOf(text), renameBoxInner-1, "테두리를 넘지 않는다")
	assert.Equal(t, widthOf(text), cursor, "커서는 그린 글자 뒤다")
}

// 짧은 이름은 접지 않는다. 옛 이름이 그대로 보인다.
func TestRenamePopupShowsBothNamesWhenShort(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor
	e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 4

	m, _ := renameInputMode(e)

	text, cursor := m.(viewRenameInput).renameLine()

	assert.Equal(t, " Greet → Greet", text)
	assert.Equal(t, widthOf(text), cursor)
}

// 옛 이름 그대로 enter 를 누르면 아무 일도 하지 않는다.
func TestRenamePopupKeepsSameName(t *testing.T) {
	e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor
	e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 4

	m, _ := renameInputMode(e)
	m = send(m, "enter")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Empty(t, e.notice, "묻지도 않았다")
}
