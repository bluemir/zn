package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// 서버가 없으면 묻지 않고 알린다. `\gr` 을 쳤을 때 아무 일도 안 나면 안 된다.
func TestStartReferencesWithoutServer(t *testing.T) {
	t.Run("Go 파일이 아니다", func(t *testing.T) {
		e := &editor{buffers: []viewport{newEmptyBuffer("README.md")}}

		assert.Nil(t, e.startReferences())
		assert.Equal(t, "언어 서버가 붙는 파일에서만 사용처를 찾습니다", e.notice)
	})

	t.Run("서버를 못 띄운 뒤", func(t *testing.T) {
		e := &editor{buffers: []viewport{newEmptyBuffer("main.go")}}
		e.serverState(lsp.ServerFor("main.go")).failed = true

		assert.Nil(t, e.startReferences())
		assert.Equal(t, "gopls 가 없어 사용처를 찾을 수 없습니다", e.notice)
	})

	t.Run("아직 뜨는 중이다", func(t *testing.T) {
		e := &editor{buffers: []viewport{newEmptyBuffer("main.go")}}
		e.serverState(lsp.ServerFor("main.go")).starting = true

		assert.Nil(t, e.startReferences())
		assert.Equal(t, "gopls 를 띄우는 중입니다. 잠시 뒤 다시 칩니다", e.notice)
	})
}

// 사용처가 하나면 목록을 거치지 않고 곧바로 뛴다(ADR-0068).
func TestFinishReferencesJumpsWhenSingle(t *testing.T) {
	dir := t.TempDir()

	here := filepath.Join(dir, "here.go")
	there := filepath.Join(dir, "there.go")
	require.NoError(t, os.WriteFile(here, []byte("package main\n"), 0644))
	require.NoError(t, os.WriteFile(there, []byte("package main\n\nfunc use() { target() }\n"), 0644))

	buf, err := OpenBuffer(here)
	require.NoError(t, err)

	e := &editor{buffers: []viewport{buf}, width: 80, height: 20}

	next, _ := e.finishReferences(referencesMsg{locations: []lsp.Location{{
		URI:   "file://" + there,
		Range: lsp.Range{Start: lsp.Position{Line: 2, Character: 12}},
	}}})

	assert.Nil(t, next, "하나면 mode 를 바꾸지 않는다")
	require.Len(t, e.buffers, 2)
	assert.Equal(t, there, e.activeBuffer().path)
	assert.Equal(t, 2, e.activeBuffer().cursor.line)
	assert.Equal(t, 12, e.activeBuffer().cursor.col)
}

// 여럿이면 고르는 화면이 열린다. 정의와 같은 화면이고 제목만 다르다.
func TestFinishReferencesOpensList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc target() {}\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{buffers: []viewport{buf}, width: 80, height: 20}

	next, _ := e.finishReferences(referencesMsg{locations: []lsp.Location{
		{URI: "file://" + path, Range: lsp.Range{Start: lsp.Position{Line: 2}}},
		{URI: "file://" + path, Range: lsp.Range{Start: lsp.Position{Line: 0}}},
	}})

	list, ok := next.(viewLocations)
	require.True(t, ok, "고르는 화면이 열려야 한다")
	assert.Equal(t, "사용처", list.title)
	assert.Len(t, list.locations, 2)
	assert.Equal(t, 0, e.activeBuffer().cursor.line, "고르기 전에는 커서가 움직이지 않는다")
}

// 아무도 쓰지 않는 이름이면 0 개로 온다. 선언 자리를 목록에서 뺐기 때문에 이 말이 맞다.
func TestFinishReferencesNothing(t *testing.T) {
	e := &editor{buffers: []viewport{newEmptyBuffer("main.go")}, width: 80, height: 20}

	next, cmd := e.finishReferences(referencesMsg{})
	assert.Nil(t, next)
	assert.Nil(t, cmd)
	assert.Equal(t, "사용처를 찾지 못했습니다", e.notice)
}

// 서버가 없으면 설치를 묻는다. 정의로 가기와 같은 확인창이다.
func TestGotoReferencesPromptsInstallWhenMissing(t *testing.T) {
	editor := newTestEditor("package main\nfunc main() {}\n", 80, 20)
	editor.editor.buffers[0].path = "main.go"
	editor.editor.serverState(lsp.ServerFor("main.go")).failed = true

	normal, _ := normalMode(editor.editor)
	model, cmd := gotoReferences(normal, editor.editor)

	require.IsType(t, viewServerInstallConfirm{}, model)
	assert.Nil(t, cmd)
}

func TestGotoReferencesWithoutServerForFileNotifies(t *testing.T) {
	editor := newTestEditor("# Hello\n", 80, 20)
	editor.editor.buffers[0].path = "README.md"

	normal, _ := normalMode(editor.editor)
	model, cmd := gotoReferences(normal, editor.editor)

	assert.Nil(t, model)
	assert.Nil(t, cmd)
	assert.Equal(t, "언어 서버가 붙는 파일에서만 사용처를 찾습니다", editor.editor.notice)
}

func TestActionGotoReferencesPromptsConfirm(t *testing.T) {
	editor := newTestEditor("package main\n", 80, 20)
	editor.editor.buffers[0].path = "main.go"
	editor.editor.serverState(lsp.ServerFor("main.go")).failed = true

	model, cmd := actionGotoReferences{}.run(editor.editor)
	require.IsType(t, viewServerInstallConfirm{}, model)
	assert.Nil(t, cmd)
}
