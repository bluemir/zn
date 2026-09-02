package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUnnamedEditor 는 `:tabnew` 로 만든 것과 같은 이름 없는 tab 하나짜리 화면이다.
// 쓸 곳을 만들 수 있도록 빈 디렉터리를 같이 준다.
func newUnnamedEditor(t *testing.T) (viewEditorNormal, string) {
	t.Helper()

	return viewEditorNormal{
		editor: &editor{
			buffers: []viewport{newEmptyBuffer("")},
			width:   60,
			height:  5 + tablineHeight + statusBarHeight,
		},
	}, t.TempDir()
}

// `:w <파일>` 은 사본을 쓴다. 보고 있는 파일도 tab 이름도 그대로다 (ADR-0024).
func TestWriteToOtherFileWritesCopy(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	start.activeBuffer().insert([]byte("X"))

	copyPath := filepath.Join(dir, "copy.txt")
	m := runCommand(start, "w "+copyPath)

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "사본을 씀")

	written, err := os.ReadFile(copyPath)
	require.NoError(t, err)
	assert.Equal(t, "Xa.txt\n", string(written))

	buf := bufferOf(t, m)
	assert.Equal(t, filepath.Join(dir, "a.txt"), buf.path, "보고 있는 파일은 그대로다")
	assert.True(t, buf.dirty, "원래 파일에는 아직 쓰지 않았으므로 변경 표시가 남는다")

	origin, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a.txt\n", string(origin), "원래 파일은 건드리지 않는다")
}

// 사본을 쓴 뒤 그냥 `:w` 를 치면 사본이 아니라 원래 파일에 쓴다. 이름이 옮겨가지 않았다.
func TestWriteAfterCopyStillSavesOriginal(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	start.activeBuffer().insert([]byte("X"))

	m := runCommand(start, "w "+filepath.Join(dir, "copy.txt"))
	m = runCommand(m, "w")

	require.IsType(t, viewEditorNormal{}, m)
	assert.False(t, bufferOf(t, m).dirty)

	origin, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Xa.txt\n", string(origin))
}

// 이미 있는 파일에는 쓰지 않는다. 읽은 적이 없는 파일이라 무엇을 덮어쓰는지 알 수 없다.
func TestWriteToExistingFileRefused(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt", "b.txt")

	m := runCommand(start, "w "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "이미 있습니다")

	kept, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "b.txt\n", string(kept), "덮어쓰지 않았다")
}

// `!` 는 그것을 알고도 덮어쓴다는 뜻이다.
func TestWriteForceToExistingFileOverwrites(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	other := filepath.Join(dir, "other.txt")
	require.NoError(t, os.WriteFile(other, []byte("남의 것\n"), 0644))

	m := runCommand(start, "w! "+other)

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "사본을 씀")

	written, err := os.ReadFile(other)
	require.NoError(t, err)
	assert.Equal(t, "a.txt\n", string(written))
}

// 이름 없는 buffer 는 이 저장으로 그 파일의 buffer 가 된다. `:tabnew` 로 만든 tab 을 저장하는 길이다.
func TestWriteNamesUnnamedBuffer(t *testing.T) {
	start, dir := newUnnamedEditor(t)

	var model tea.Model = start
	model = send(model, "i", "메", "모", "esc")

	path := filepath.Join(dir, "notes.txt")
	model = runCommand(model, "w "+path)

	require.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, barOf(t, model)[1], "저장함")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "메모\n", string(written))

	buf := bufferOf(t, model)
	assert.Equal(t, path, buf.path, "이름이 붙는다")
	assert.False(t, buf.dirty, "이제 이 파일의 buffer 라 변경 표시가 사라진다")

	tabline := model.(viewEditorNormal).renderTabline(60).line
	assert.Contains(t, tabline, "notes.txt", "tabline 도 [No Name] 이 아니라 새 이름이다")
}

// 이름이 붙은 뒤에는 그냥 `:w` 로 이어서 저장한다.
// 방금 쓴 것이 저장 기준이 되어야 자기가 쓴 것을 남의 변경으로 보지 않는다 (ADR-0015).
func TestWriteAgainAfterNaming(t *testing.T) {
	start, dir := newUnnamedEditor(t)

	path := filepath.Join(dir, "notes.txt")
	m := runCommand(start, "w "+path)

	m = send(m, "i", "X", "esc")
	m = runCommand(m, "w")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "저장함")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "X\n", string(written))
}

// 이름 붙이기는 「같은 파일은 한 tab」에 걸린다.
// 이름이 붙는 순간 같은 파일에 Buffer 가 둘이 되어 한쪽 저장이 다른 쪽 편집을 덮어쓴다.
func TestWriteRefusesNameOpenInAnotherTab(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	start.newTab()
	require.Equal(t, "", start.activeBuffer().path, "이름 없는 tab 으로 옮겨왔다")

	m := runCommand(start, "w! "+filepath.Join(dir, "a.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "다른 tab 에 열려 있습니다")
	assert.Equal(t, "", bufferOf(t, m).path, "이름이 붙지 않는다")

	kept, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a.txt\n", string(kept), "쓰지도 않는다")
}

// 이름 있는 buffer 는 사본만 쓰므로 대상이 다른 tab 에 열려 있어도 상관없다.
func TestWriteCopyAllowsFileOpenInAnotherTab(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt", "b.txt")

	m := runCommand(start, "w! "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "사본을 씀")
	assert.Equal(t, "b.txt", string(m.(viewEditorNormal).buffers[1].lines[0]),
		"그 tab 의 buffer 는 그대로다. 다시 읽는 것은 `:e` 다")
}

// `:w <보고 있는 파일>` 은 사본이 아니라 제자리 저장이다.
// 사본으로 보면 「이미 있습니다」로 막히고, `!` 를 붙여도 변경 표시가 남는다.
func TestWriteToActiveFileIsPlainSave(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	start.activeBuffer().insert([]byte("X"))

	// 표기가 달라도 같은 파일이면 제자리 저장이다. tabOf 와 같은 기준이다.
	m := runCommand(start, "w "+dir+"/./a.txt")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "저장함")
	assert.False(t, bufferOf(t, m).dirty)

	written, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Xa.txt\n", string(written))
}

// 이름 없는 buffer 에 인자 없이 `:w` 를 치면 쓸 곳이 없다. 이름을 주는 길을 알린다.
func TestWriteWithoutNameTellsHowToName(t *testing.T) {
	start, _ := newUnnamedEditor(t)

	m := runCommand(start, "w")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], ":w <파일>")
}

// 쓸 수 없는 자리면 이름도 붙지 않는다.
func TestWriteToUnwritablePathKeepsBufferUnnamed(t *testing.T) {
	start, dir := newUnnamedEditor(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0555))

	m := runCommand(start, "w "+filepath.Join(dir, "sub", "new.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.NotContains(t, barOf(t, m)[1], "저장함")
	assert.Equal(t, "", bufferOf(t, m).path)
}

// 공백이 든 파일 이름도 따옴표로 쓸 수 있다. tokenizer 가 이미 하는 일이다.
func TestWriteToQuotedPath(t *testing.T) {
	start, dir := newUnnamedEditor(t)

	path := filepath.Join(dir, "my notes.txt")
	m := runCommand(start, `w "`+path+`"`)

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, path, bufferOf(t, m).path)

	_, err := os.Stat(path)
	assert.NoError(t, err)
}
