package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 시험용 포매터는 셸 한 줄이다. goimports 를 부르면 그것이 깔려 있는지에 시험이 매달린다 —
// 여기서 볼 것은 「명령이 낸 글을 어떻게 buffer 에 들이는가」이지 goimports 가 아니다.
func shellHook(script string) *saveHook {
	return &saveHook{name: "시험", dir: ".", path: "/bin/sh", args: []string{"-c", script}}
}

// 표에 없는 확장자는 아무것도 돌리지 않는다. 그것은 까닭이 붙는 일이 아니다.
func TestSaveHookTableSkipsOtherFiles(t *testing.T) {
	e := &editor{}

	for _, path := range []string{"a.txt", "a.md", ""} {
		hook, err := e.saveHookFor(path)

		assert.Nil(t, hook, path)
		assert.NoError(t, err, "%q 는 「돌릴 것이 없다」이지 「없어서 못 돌린다」가 아니다", path)
	}
}

// 포매터와 폴더 짝마다 한 번만 찾는다. 담아 둔 것이 있으면 그것을 그대로 쓴다.
func TestSaveHookCachesPerDirectory(t *testing.T) {
	hook := shellHook("cat")
	e := &editor{saveHooks: map[saveHookKey]*saveHook{
		{tool: "goimports", dir: "pkg"}: hook,
	}}

	found, err := e.saveHookFor(filepath.Join("pkg", "a.go"))
	require.NoError(t, err)
	assert.Same(t, hook, found)

	// 「찾아봤는데 없다」도 담긴다. nil 이 담겨 있으면 다시 찾지 않는다.
	e.saveHooks[saveHookKey{tool: "goimports", dir: "none"}] = nil

	found, err = e.saveHookFor(filepath.Join("none", "a.go"))
	assert.Nil(t, found)
	assert.ErrorIs(t, err, errHookNotInstalled, "표에는 있는데 깔려 있지 않다")
	assert.Len(t, e.saveHooks, 2, "찾는 일이 새로 일어나지 않았다")
}

func TestSaveHookReplacesBuffer(t *testing.T) {
	buf := newBuffer("a.go", []byte("가나\nabc\n"))

	note := buf.applySaveHook(shellHook("tr a-z A-Z"), wide)

	assert.Equal(t, "가나\nABC", strings.Join(linesOf(buf), "\n"))
	assert.Equal(t, "시험: 1 줄 맞춤", note)
	assert.True(t, buf.dirty)
}

// 줄 수가 달라지면 얼마나 늘고 줄었는지도 적는다.
func TestSaveHookNoteCountsLines(t *testing.T) {
	grown := newBuffer("a.go", []byte("한 줄\n"))
	assert.Equal(t, "시험: 1 줄 맞춤, 1 줄 늘어남", grown.applySaveHook(shellHook("cat; echo 뒤에"), wide))

	shrunk := newBuffer("a.go", []byte("첫 줄\n둘째 줄\n"))
	assert.Equal(t, "시험: 1 줄 맞춤, 1 줄 줄어듦", shrunk.applySaveHook(shellHook("head -1"), wide))
}

// 바뀐 것이 없으면 아무 말도 하지 않고 buffer 도 건드리지 않는다.
func TestSaveHookQuietWhenNothingChanged(t *testing.T) {
	buf := newBuffer("a.go", []byte("그대로\n"))

	assert.Empty(t, buf.applySaveHook(shellHook("cat"), wide))
	assert.False(t, buf.dirty, "dirty 가 서면 저장할 것이 없는데 있다고 보인다")
	assert.Empty(t, buf.undo, "되돌릴 것도 생기지 않는다")
}

// 포매터가 고친 것은 한 동작이라 `u` 한 번에 통째로 돌아간다.
func TestSaveHookUndoesInOneStep(t *testing.T) {
	buf := newBuffer("a.go", []byte("첫 줄\n둘째\n셋째\n"))

	buf.applySaveHook(shellHook("tr 줄 칸"), wide)
	require.NotEqual(t, "첫 줄", string(buf.lines[0]))

	require.True(t, buf.applyUndo(wide))
	assert.Equal(t, "첫 줄\n둘째\n셋째", strings.Join(linesOf(buf), "\n"))
}

// 앞의 타이핑과 한 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
func TestSaveHookUndoDoesNotSwallowTyping(t *testing.T) {
	buf := newBuffer("a.go", []byte("abc\n"))
	buf.insert([]byte("XY"), wide)

	buf.applySaveHook(shellHook("tr a-z A-Z"), wide)

	require.True(t, buf.applyUndo(wide), "포매터가 한 것부터 돌아온다")
	assert.Equal(t, "XYabc", string(buf.lines[0]))

	require.True(t, buf.applyUndo(wide), "그 앞의 타이핑은 따로 남아 있다")
	assert.Equal(t, "abc", string(buf.lines[0]))
}

// 파일이 짧아져도 커서는 범위 안에 남는다.
func TestSaveHookKeepsCursorInRange(t *testing.T) {
	buf := newBuffer("a.go", []byte("첫 줄\n둘째 줄\n셋째 줄\n"))
	buf.cursor.Line, buf.cursor.Col = 2, 6

	buf.applySaveHook(shellHook("head -1"), wide)

	assert.Equal(t, 0, buf.cursor.Line)
	assert.LessOrEqual(t, buf.cursor.Col, len(buf.lines[0]))
}

// 실패하면 buffer 를 건드리지 않는다. 저장은 그대로 가고 까닭만 아래 줄에 뜬다.
func TestSaveHookFailureKeepsBuffer(t *testing.T) {
	buf := newBuffer("a.go", []byte("고치다 만 글\n"))

	note := buf.applySaveHook(shellHook("echo '<standard input>:1:1: expected declaration' >&2; exit 2"), wide)

	assert.Equal(t, "시험: <standard input>:1:1: expected declaration", note)
	assert.Equal(t, "고치다 만 글", string(buf.lines[0]))
	assert.False(t, buf.dirty)
}

// 읽기 전용 파일은 손대지 않는다. 쓰기가 어차피 실패하는데 buffer 만 바뀌면 되돌릴 길이 없다.
func TestSaveHookSkipsReadOnly(t *testing.T) {
	buf := newBuffer("a.go", []byte("abc\n"))
	buf.readOnly = true

	assert.Empty(t, buf.applySaveHook(shellHook("tr a-z A-Z"), wide))
	assert.Equal(t, "abc", string(buf.lines[0]))
}

// 마지막 줄바꿈은 포매터가 정하지 않는다. 그것은 `.editorconfig` 의 몫이다(ADR-0052).
func TestSaveHookKeepsFinalNewlineFact(t *testing.T) {
	buf := newBuffer("a.go", []byte("abc"))
	require.False(t, buf.finalLineEnding)

	buf.applySaveHook(shellHook("tr a-z A-Z"), wide)

	assert.Equal(t, "ABC", string(buf.lines[0]))
	assert.False(t, buf.finalLineEnding, "포매터가 붙인 줄바꿈이 사실을 뒤집지 않는다")
}

// **바깥 변경 검사가 맞추기보다 먼저다.** 막힐 저장이면 buffer 를 건드려선 안 된다(ADR-0052).
func TestSaveHookNotRunWhenOutsideChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	// 읽은 뒤에 밖에서 바뀌었다.
	require.NoError(t, os.WriteFile(path, []byte("남이 고친 글\n"), 0644))

	_, err = buf.Save(wide, shellHook("tr a-z A-Z"))

	require.Error(t, err)
	assert.Equal(t, "abc", string(buf.lines[0]), "저장이 막혔으면 buffer 도 그대로다")
}

// 포매터가 먼저고 `.editorconfig` 가 뒤다. 문구는 둘 다 남는다.
func TestSaveHookRunsBeforeEditorconfig(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\n",
		"a.go", "abc\n")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	// 포매터가 줄 끝에 공백을 남기면 `.editorconfig` 가 그것을 지운다.
	note, err := buf.Save(wide, shellHook("sed 's/$/  /'"))
	require.NoError(t, err)

	saved, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "abc\n", string(saved), "뒤에 선 것이 이긴다")
	assert.Equal(t, "시험: 1 줄 맞춤  .editorconfig: 줄끝 공백 1 줄 지움", note)
}

// goimports 를 찾지 못했으면 `:w` 가 설치를 한 번 묻는다. 저장은 이미 끝나 있다.
func TestSaveAsksToInstallGoimports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	m := newTestEditorFile(path, "package a\n", 80, 6)
	// 「찾아봤는데 없다」를 담아 둔다. 시험이 이 판에 goimports 가 깔려 있는지에 매이지 않는다.
	m.editor.saveHooks = map[saveHookKey]*saveHook{{tool: "goimports", dir: dir}: nil}

	var model tea.Model = m
	model = send(model, ":", "w", "enter")

	require.IsType(t, viewFormatterInstallConfirm{}, model)

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "package a\n", string(saved), "묻기 전에 저장은 끝났다")

	// 거절하면 그 뒤로는 묻지 않는다.
	back, _ := model.(viewFormatterInstallConfirm).press("esc")
	assert.True(t, back.(viewEditorNormal).formattersDeclined["goimports"])

	model = send(back, ":", "w", "enter")
	assert.IsType(t, viewEditorNormal{}, model, "두 번째 저장은 조용하다")
}

// 나가는 길에서는 묻지 않는다. tab 을 닫는 것과 창이 겹친다.
func TestSaveAndQuitDoesNotAsk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	m := newTestEditorFile(path, "package a\n", 80, 6)
	m.editor.saveHooks = map[saveHookKey]*saveHook{{tool: "goimports", dir: dir}: nil}

	model := send(tea.Model(m), ":", "w", "q", "enter")

	assert.NotEqual(t, "viewFormatterInstallConfirm", fmt.Sprintf("%T", model), "묻지 않는다")
	assert.False(t, m.editor.formattersDeclined["goimports"], "거절한 적도 없다 — 다음 `:w` 가 묻는다")
}

// `go env GOBIN GOPATH` 는 GOBIN 이 비면 **첫 줄을 빈 줄로** 준다. 통째로 다듬고 나누면
// 그 줄이 사라져 GOPATH 를 GOBIN 으로 읽는다 — 이 기계에서 실제로 그렇게 어긋났다.
func TestGoInstallDirsWhenGobinEmpty(t *testing.T) {
	dirs := goInstallDirs()

	require.NotEmpty(t, dirs, "go 가 있는 판에서는 적어도 하나는 나온다")
	for _, dir := range dirs {
		assert.True(t, strings.HasSuffix(dir, "bin"), "실행 파일이 들어가는 자리다: %q", dir)
	}
}

func TestSplitFormatted(t *testing.T) {
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitFormatted([]byte("a\nb\n")))
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitFormatted([]byte("a\nb")),
		"마지막 줄바꿈이 없어도 같다")
	assert.Equal(t, [][]byte{{}}, splitFormatted(nil), "buffer 에는 줄이 적어도 하나 있어야 한다")
}

func TestCountChangedLines(t *testing.T) {
	old := [][]byte{[]byte("a"), []byte("b")}

	assert.Equal(t, 0, countChangedLines(old, old))
	assert.Equal(t, 1, countChangedLines(old, [][]byte{[]byte("a"), []byte("B")}))
	assert.Equal(t, 1, countChangedLines(old, [][]byte{[]byte("a"), []byte("b"), []byte("c")}),
		"늘어난 줄도 달라진 것이다")
}

// python 파일은 ruff 를 통과한다. 표에 줄이 하나 는 것이 python 지원의 이 몫이다(ADR-0107).
func TestFormatterTableHasPython(t *testing.T) {
	spec := formatterFor("app.py")
	require.NotNil(t, spec)

	assert.Equal(t, "ruff", spec.name)
	assert.Equal(t, "uv tool install ruff", spec.InstallHint())

	assert.Equal(t, "goimports", formatterFor("main.go").name)
	assert.Nil(t, formatterFor("notes.md"))
	assert.Nil(t, formatterFor(""))
}

// **한 폴더에 `.go` 와 `.py` 가 같이 있으면 서로 덮지 않는다.** 담아 두는 자리의 키가
// 폴더만이던 때에는 먼저 저장한 쪽의 답이 다른 언어에 실려 갔다(ADR-0107).
func TestSaveHookCacheDoesNotMixLanguages(t *testing.T) {
	goHook := shellHook("cat")
	pyHook := shellHook("tr a-z A-Z")

	e := &editor{saveHooks: map[saveHookKey]*saveHook{
		{tool: "goimports", dir: "pkg"}: goHook,
		{tool: "ruff", dir: "pkg"}:      pyHook,
	}}

	found, err := e.saveHookFor(filepath.Join("pkg", "a.go"))
	require.NoError(t, err)
	assert.Same(t, goHook, found, "Go 파일은 Go 쪽 답을 받는다")

	found, err = e.saveHookFor(filepath.Join("pkg", "a.py"))
	require.NoError(t, err)
	assert.Same(t, pyHook, found, "python 파일은 python 쪽 답을 받는다")
}

// 거절은 포매터마다 따로 적힌다. 한쪽을 거절한 것이 다른 언어의 물음을 삼키지 않는다.
func TestFormatterDeclineIsPerFormatter(t *testing.T) {
	e := &editor{}

	goimports := formatterFor("main.go")
	ruff := formatterFor("app.py")

	require.True(t, e.askFormatter(goimports))
	require.True(t, e.askFormatter(ruff))

	e.declineFormatter(goimports)

	assert.False(t, e.askFormatter(goimports), "거절한 뒤로는 묻지 않는다")
	assert.True(t, e.askFormatter(ruff), "남의 거절이 옮지 않는다")
}

// 설치 작업이 도는 중에는 묻지 않는다. 포매터마다 따로 본다.
func TestFormatterAskWhileInstalling(t *testing.T) {
	e := &editor{}

	ruff := formatterFor("app.py")
	e.putJob(job{name: formatterJobName(ruff)})

	assert.False(t, e.askFormatter(ruff))
	assert.True(t, e.askFormatter(formatterFor("main.go")), "남의 작업은 이 물음을 막지 않는다")
}
