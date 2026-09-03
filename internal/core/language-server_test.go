package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"

	"github.com/bluemir/zn/internal/textarea"
)

// 어느 파일에 어느 서버가 붙는지는 lsp 의 표가 정한다(lsp/server.go).
func TestServerForFile(t *testing.T) {
	assert.Equal(t, "gopls", lsp.ServerFor("internal/core/core.go").Name)
	assert.Equal(t, "gopls", lsp.ServerFor("main.go").Name)
	assert.Equal(t, "pyright", lsp.ServerFor("app.py").Name)
	assert.Equal(t, "pyright", lsp.ServerFor("/tmp/pkg/App.PY").Name, "확장자는 내려 견준다")

	assert.Nil(t, lsp.ServerFor("go.mod"))
	assert.Nil(t, lsp.ServerFor("README.md"))
	assert.Nil(t, lsp.ServerFor("gopher.gop"))
	assert.Nil(t, lsp.ServerFor(""))
}

// 서버에 알리는 경로는 늘 절대 경로다. 이름 없는 buffer 와 붙는 서버가 없는 파일은
// 알리지 않는다.
func TestServerPath(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	server, got, ok := serverPath("core.go")
	require.True(t, ok)
	assert.Equal(t, "gopls", server.Name)
	assert.Equal(t, filepath.Join(cwd, "core.go"), got)

	server, got, ok = serverPath(filepath.Join(cwd, "core.go"))
	require.True(t, ok)
	assert.Equal(t, "gopls", server.Name)
	assert.Equal(t, filepath.Join(cwd, "core.go"), got)

	// python 파일은 python 서버로 간다. 같은 자리가 언어를 가른다.
	server, got, ok = serverPath("app.py")
	require.True(t, ok)
	assert.Equal(t, "pyright", server.Name)
	assert.Equal(t, filepath.Join(cwd, "app.py"), got)

	_, _, ok = serverPath("")
	assert.False(t, ok)

	_, _, ok = serverPath("README.md")
	assert.False(t, ok)
}

// 서버가 없으면 묻지 않고 알린다. `\gd` 를 쳤을 때 아무 일도 안 나면 안 된다.
func TestStartDefinitionWithoutServer(t *testing.T) {
	t.Run("붙는 서버가 없다", func(t *testing.T) {
		e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("README.md")}}

		assert.Nil(t, e.startDefinition())
		assert.Equal(t, "언어 서버가 붙는 파일에서만 정의를 찾습니다", e.notice)
	})

	t.Run("서버를 못 띄운 뒤", func(t *testing.T) {
		e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("main.go")}}
		e.serverState(lsp.ServerFor("main.go")).failed = true

		assert.Nil(t, e.startDefinition())
		assert.Equal(t, "gopls 가 없어 정의를 찾을 수 없습니다", e.notice)
	})

	// **서버 이름이 문구에 든다.** python 파일에서 gopls 를 말하면 안 된다(ADR-0107).
	t.Run("python 파일은 python 서버를 말한다", func(t *testing.T) {
		e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("app.py")}}
		e.serverState(lsp.ServerFor("app.py")).failed = true

		assert.Nil(t, e.startDefinition())
		assert.Equal(t, "pyright 가 없어 정의를 찾을 수 없습니다", e.notice)
	})

	t.Run("아직 뜨는 중이다", func(t *testing.T) {
		e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("main.go")}}
		e.serverState(lsp.ServerFor("main.go")).starting = true

		// 뜨는 중이면 새로 걸지 않는다. 알림만 남는다.
		assert.Nil(t, e.startDefinition())
		assert.Equal(t, "gopls 를 띄우는 중입니다. 잠시 뒤 다시 칩니다", e.notice)
	})
}

// 서버가 답한 자리로 커서가 간다. 열은 UTF-16 이라 한글이 든 줄에서 byte 로 바뀌어야 한다.
func TestFinishDefinitionMovesCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.go")
	require.NoError(t, os.WriteFile(path, []byte(
		"package main\n\n// 가나다 부른다\nfunc target() {}\n"), 0644))

	buf, err := textarea.OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{
		buffers: []textarea.Viewport{buf},
		width:   80,
		height:  20,
	}

	// 3 번째 줄(0 부터 세어 2) 의 UTF-16 열 7 은 "// 가나다 " 다음이라 byte 열 13 이다.
	next, cmd := e.finishDefinition(definitionMsg{locations: []lsp.Location{{
		URI: "file://" + path,
		Range: lsp.Range{
			Start: lsp.Position{Line: 2, Character: 7},
		},
	}}})

	// 후보가 하나면 mode 를 바꾸지 않고 그 자리로 뛴다.
	assert.Nil(t, next)

	// 열린 파일이 Go 파일이라 서버를 띄우는 Cmd 가 같이 나온다. 진짜로 쓸 때는 이미 떠 있어서
	// nil 이다 — 답을 받았다는 것이 곧 서버가 있다는 뜻이다.
	assert.NotNil(t, cmd)

	assert.Equal(t, 2, e.activeBuffer().Cursor.Line)
	assert.Equal(t, 13, e.activeBuffer().Cursor.Col)
	assert.Empty(t, e.notice)
}

// 다른 파일이면 새 tab 으로 열린다.
func TestFinishDefinitionOpensNewTab(t *testing.T) {
	dir := t.TempDir()

	here := filepath.Join(dir, "here.go")
	there := filepath.Join(dir, "there.go")
	require.NoError(t, os.WriteFile(here, []byte("package main\n"), 0644))
	require.NoError(t, os.WriteFile(there, []byte("package main\n\nfunc there() {}\n"), 0644))

	buf, err := textarea.OpenBuffer(here)
	require.NoError(t, err)

	e := &editor{buffers: []textarea.Viewport{buf}, width: 80, height: 20}

	e.finishDefinition(definitionMsg{locations: []lsp.Location{{
		URI:   "file://" + there,
		Range: lsp.Range{Start: lsp.Position{Line: 2, Character: 5}},
	}}})

	require.Len(t, e.buffers, 2)
	assert.Equal(t, there, e.activeBuffer().Path)
	assert.Equal(t, 2, e.activeBuffer().Cursor.Line)
	assert.Equal(t, 5, e.activeBuffer().Cursor.Col)
}

// 후보가 여럿이면 고르는 화면이 열린다. 커서는 물어본 자리에 그대로 있다(ADR-0051).
func TestFinishDefinitionOpensList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc target() {}\n"), 0644))

	buf, err := textarea.OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{buffers: []textarea.Viewport{buf}, width: 80, height: 20}

	next, _ := e.finishDefinition(definitionMsg{locations: []lsp.Location{
		{URI: "file://" + path, Range: lsp.Range{Start: lsp.Position{Line: 2}}},
		{URI: "file://" + path, Range: lsp.Range{Start: lsp.Position{Line: 0}}},
		{URI: "file://" + path, Range: lsp.Range{Start: lsp.Position{Line: 1}}},
	}})

	list, ok := next.(viewLocations)
	require.True(t, ok, "고르는 화면이 열려야 한다")
	assert.Len(t, list.locations, 3)
	assert.Equal(t, 0, list.selected)
	assert.Equal(t, 0, e.activeBuffer().Cursor.Line, "고르기 전에는 커서가 움직이지 않는다")
	assert.Empty(t, e.notice)
}

// 못 찾았거나 실패했으면 그것을 알린다. 커서는 그대로다.
func TestFinishDefinitionNothing(t *testing.T) {
	e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("main.go")}, width: 80, height: 20}

	next, cmd := e.finishDefinition(definitionMsg{})
	assert.Nil(t, next)
	assert.Nil(t, cmd)
	assert.Equal(t, "정의를 찾지 못했습니다", e.notice)
}

// 서버가 없으면 맞출 것도 없다. tick 이 와도 아무 일도 하지 않아야 한다.
func TestSyncServersWithoutServer(t *testing.T) {
	e := &editor{buffers: []textarea.Viewport{textarea.NewEmptyBuffer("main.go")}}

	assert.Nil(t, e.syncServers())
	assert.Nil(t, e.scheduleEditTick())
	assert.False(t, e.editTickScheduled)
}

// tick 이 도는 자리를 진짜 서버로 재 본다.
//
// 보고 있는 tab 만이 아니라 **열려 있는 Go 파일 전부**를 맞추는지가 요점이다. 묻는 자리는
// 활성 buffer 를 전문으로 보내므로(startDefinition) 그쪽 길만 보면 다른 tab 이 서버에
// 알려지는지 알 수 없다.
//
// **남의 언어 파일도 같이 열어 둔다.** 서버가 여럿이 되면서 「자기 몫만 받는다」가 새 규칙이
// 되었고, python 파일이 gopls 에 알려지지 않는 것이 그 규칙의 겉모습이다(ADR-0107).
func TestSyncServersReconcilesOpenTabs(t *testing.T) {
	if testing.Short() {
		t.Skip("gopls 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	goServer := lsp.ServerFor("main.go")
	require.NotNil(t, goServer)

	client, err := lsp.Start(ctx, root, goServer)
	if err != nil {
		t.Skip("gopls 를 띄우지 못했다: " + err.Error())
	}
	t.Cleanup(client.Shutdown)

	first := filepath.Join(root, "internal/core/core.go")
	second := filepath.Join(root, "internal/core/editor.go")
	notGo := filepath.Join(root, "docs/tasks.md")
	python := filepath.Join(root, "scripts/notaserver.py")

	buffers := []textarea.Viewport{}
	for _, path := range []string{first, second, notGo} {
		buf, err := textarea.OpenBuffer(path)
		require.NoError(t, err)

		buffers = append(buffers, buf)
	}

	// 디스크에 없어도 된다. 여기서 재는 것은 「gopls 에 알려지는가」이고 그것은 이름으로
	// 갈린다(lsp.ServerFor).
	buffers = append(buffers, textarea.NewEmptyBuffer(python))

	e := &editor{buffers: buffers, width: 80, height: 20}
	e.serverState(goServer).client = client

	// tick 이 내는 Cmd 를 그대로 돌린다. bubbletea 가 하는 일이다.
	cmd := e.syncServers()
	require.NotNil(t, cmd)
	assert.Nil(t, cmd(), "알릴 것이 없어야 한다")

	assert.True(t, client.Tracks(first), "보고 있는 tab")
	assert.True(t, client.Tracks(second), "보고 있지 않은 tab 도 알려야 한다")
	assert.False(t, client.Tracks(notGo), "붙는 서버가 없는 것은 알리지 않는다")
	assert.False(t, client.Tracks(python), "남의 언어 파일은 이 서버에 알리지 않는다")

	// 고친 것이 다음 tick 에 간다. 사본이 갱신되어 그다음 tick 에는 보낼 것이 없다.
	e.buffers[1].Insert([]byte("// 끼운 줄\n"))

	cmd = e.syncServers()
	require.NotNil(t, cmd)
	assert.Nil(t, cmd())

	// tab 을 닫으면 서버도 잊는다. 닫는 자리에 갈고리를 걸지 않고 목록을 견주어 안다.
	e.active = 1
	require.True(t, e.closeTab())

	cmd = e.syncServers()
	require.NotNil(t, cmd)
	assert.Nil(t, cmd())

	assert.True(t, client.Tracks(first))
	assert.False(t, client.Tracks(second), "닫은 tab 은 서버도 잊어야 한다")
}

func TestGotoDefinitionPromptsInstallWhenMissing(t *testing.T) {
	editor := newTestEditor("package main\nfunc main() {}\n", 80, 20)
	editor.editor.buffers[0].Path = "main.go"
	editor.editor.serverState(lsp.ServerFor("main.go")).failed = true

	normal, _ := normalMode(editor.editor)
	model, cmd := gotoDefinition(normal, editor.editor)

	require.IsType(t, viewServerInstallConfirm{}, model)
	assert.Nil(t, cmd)

	confirm := model.(viewServerInstallConfirm)
	assert.Equal(t, 0, confirm.cursor)
	assert.Equal(t, "gopls", confirm.server.Name)
}

// python 파일에서는 python 서버를 깔라고 묻는다. 창이 어느 서버인지 들고 있는 값이 이것이다.
func TestGotoDefinitionPromptsPyrightForPython(t *testing.T) {
	editor := newTestEditor("def f():\n    pass\n", 80, 20)
	editor.editor.buffers[0].Path = "app.py"
	editor.editor.serverState(lsp.ServerFor("app.py")).failed = true

	normal, _ := normalMode(editor.editor)
	model, _ := gotoDefinition(normal, editor.editor)

	require.IsType(t, viewServerInstallConfirm{}, model)

	confirm := model.(viewServerInstallConfirm)
	assert.Equal(t, "pyright", confirm.server.Name)
	assert.Contains(t, confirm.server.InstallHint(), "uv tool install")
}

func TestGotoDefinitionWithoutServerForFileNotifies(t *testing.T) {
	editor := newTestEditor("# Hello\n", 80, 20)
	editor.editor.buffers[0].Path = "README.md"

	normal, _ := normalMode(editor.editor)
	model, cmd := gotoDefinition(normal, editor.editor)

	assert.Nil(t, model)
	assert.Nil(t, cmd)
	assert.Equal(t, "언어 서버가 붙는 파일에서만 정의를 찾습니다", editor.editor.notice)
}

func TestGotoDefinitionWhileInstallingNotifies(t *testing.T) {
	editor := newTestEditor("package main\n", 80, 20)
	editor.editor.buffers[0].Path = "main.go"
	editor.editor.putJob(job{name: serverJobName(lsp.ServerFor("main.go"))})

	normal, _ := normalMode(editor.editor)
	model, cmd := gotoDefinition(normal, editor.editor)

	assert.Nil(t, model)
	assert.Nil(t, cmd)
	assert.Equal(t, "gopls 를 설치하는 중입니다", editor.editor.notice)
}

func TestActionGotoDefinitionPromptsConfirm(t *testing.T) {
	editor := newTestEditor("package main\n", 80, 20)
	editor.editor.buffers[0].Path = "main.go"
	editor.editor.serverState(lsp.ServerFor("main.go")).failed = true

	model, cmd := actionGotoDefinition{}.run(editor.editor)
	require.IsType(t, viewServerInstallConfirm{}, model)
	assert.Nil(t, cmd)
}

// 설치가 끝나면 그 서버를 띄운다. 작업 이름이 서버마다 다르므로 이름으로 되짚는다.
func TestServerInstallJobDoneStartsThatServer(t *testing.T) {
	server := lsp.ServerFor("main.go")

	editor := newTestEditor("package main\n", 80, 20)
	editor.editor.buffers[0].Path = "main.go"

	_, cmd := editor.editor.handleJob(jobDoneMsg{name: serverJobName(server)})
	assert.NotNil(t, cmd)
	assert.True(t, editor.editor.serverState(server).starting)
}

// python 서버를 깐 것이 끝나면 python 서버가 뜬다. gopls 는 건드리지 않는다.
func TestPyrightInstallJobDoneStartsOnlyPyright(t *testing.T) {
	goServer := lsp.ServerFor("main.go")
	pyServer := lsp.ServerFor("app.py")

	editor := newTestEditor("def f():\n    pass\n", 80, 20)
	editor.editor.buffers[0].Path = "app.py"

	_, cmd := editor.editor.handleJob(jobDoneMsg{name: serverJobName(pyServer)})
	assert.NotNil(t, cmd)
	assert.True(t, editor.editor.serverState(pyServer).starting)
	assert.False(t, editor.editor.serverState(goServer).starting, "남의 서버는 그대로다")
}

// 남의 작업 이름은 서버로 되짚히지 않는다. `goimports 설치` 도 「이름 + 설치」 꼴이다.
func TestFormatterJobDoneIsNotAServer(t *testing.T) {
	assert.Nil(t, serverForJobName("goimports 설치"))
	assert.Nil(t, serverForJobName("git 상태"))
	assert.NotNil(t, serverForJobName("gopls 설치"))
	assert.NotNil(t, serverForJobName("pyright 설치"))
}
