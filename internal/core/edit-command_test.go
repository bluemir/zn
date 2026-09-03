package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFilesEditor 는 진짜 파일 여러 개를 tab 으로 연 편집 화면이다.
// `:e` 와 `:tabnew` 는 파일을 정말로 읽으므로 없는 경로로는 볼 수 없다.
func newFilesEditor(t *testing.T, names ...string) (viewEditorNormal, string) {
	t.Helper()

	dir := t.TempDir()

	buffers := make([]viewport, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(name+"\n"), 0644))

		buf, err := OpenBuffer(path)
		require.NoError(t, err)

		buffers = append(buffers, buf)
	}

	return viewEditorNormal{
		editor: &editor{
			buffers: buffers,
			width:   60,
			height:  5 + tablineHeight + statusBarHeight,
		},
	}, dir
}

// runCommand 는 `:` 를 치고 명령 한 줄을 글자마다 넣은 뒤 enter 를 누른다.
// 파일 경로가 인자로 들어가면 키를 하나씩 나열하기에는 너무 길다.
func runCommand(m tea.Model, line string) tea.Model {
	keys := []string{":"}
	for _, ch := range line {
		keys = append(keys, string(ch))
	}

	return send(m, append(keys, "enter")...)
}

// `:e <파일>` 은 tab 을 늘리지 않고 보고 있는 tab 의 내용을 그 파일로 바꾼다 (ADR-0021).
func TestEditReplacesActiveTab(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb\n"), 0644))

	m := runCommand(start, "e "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1, "tab 은 늘지 않는다")
	assert.Equal(t, "bbb", string(bufferOf(t, m).Line(0)))
}

// 없는 파일도 연다. 그 이름의 빈 buffer 가 된다. CLI 인자와 같다.
func TestEditOpensMissingFileAsEmpty(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")

	path := filepath.Join(dir, "new.txt")
	m := runCommand(start, "e "+path)

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, path, bufferOf(t, m).Path)
	assert.Equal(t, [][]byte{{}}, bufferOf(t, m).AllLines())
}

// 이미 다른 tab 에 열려 있으면 갈아끼우지 않고 그 tab 으로 옮겨간다.
// 같은 파일에 Buffer 가 둘이면 한쪽 저장이 다른 쪽 편집을 덮어쓴다 (ADR-0021).
func TestEditMovesToTabAlreadyOpen(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt", "b.txt")

	m := runCommand(start, "e "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 2, "tab 은 늘지도 줄지도 않는다")
	assert.Equal(t, 1, activeOf(t, m))
	assert.Equal(t, filepath.Join(dir, "a.txt"), m.(viewEditorNormal).buffers[0].Path,
		"떠나온 tab 은 그대로다")
}

// 옮겨가기만 할 때는 잃을 것이 없으므로 지금 tab 이 dirty 여도 묻지 않는다.
func TestEditDoesNotAskWhenMovingToOpenTab(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt", "b.txt")
	start.activeBuffer().Insert([]byte("X"))

	m := runCommand(start, "e "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, 1, activeOf(t, m))
	assert.Equal(t, "Xa.txt", string(m.(viewEditorNormal).buffers[0].Line(0)),
		"떠나온 tab 의 편집이 남는다")
}

// 갈아끼우면 지금 tab 의 편집이 사라지므로 한 번 더 묻는다.
func TestEditAsksWhenDirty(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb\n"), 0644))
	start.activeBuffer().Insert([]byte("X"))

	confirm := runCommand(start, "e "+filepath.Join(dir, "b.txt"))
	require.IsType(t, viewConfirmDiscard{}, confirm)

	// No 로 취소하면 편집이 그대로 남고 명령줄이 아니라 normal 로 돌아간다.
	cancelled := send(confirm, "n", "enter")
	require.IsType(t, viewEditorNormal{}, cancelled)
	assert.Equal(t, "Xa.txt", string(bufferOf(t, cancelled).Line(0)))

	opened := send(confirm, "y", "enter")
	require.IsType(t, viewEditorNormal{}, opened)
	assert.Equal(t, "bbb", string(bufferOf(t, opened).Line(0)))
}

// `!` 는 묻지 말라는 뜻이다. 저장하지 않은 변경을 버리고 연다.
func TestEditForceDoesNotAsk(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb\n"), 0644))
	start.activeBuffer().Insert([]byte("X"))

	m := runCommand(start, "e! "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, "bbb", string(bufferOf(t, m).Line(0)))
}

// 인자가 없는 `:e` 는 다시 읽기다. 팔레트의 「파일 다시 읽기」와 같은 길이다 (ADR-0016).
func TestEditWithoutArgReloads(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("남이 쓴 것\n"), 0644))

	m := runCommand(start, "e")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, m).Line(0)))
	assert.Contains(t, barOf(t, m)[1], "다시 읽음")
}

// 저장하지 않은 변경이 있으면 다시 읽기도 묻는다. `:e!` 는 묻지 않는다.
func TestEditWithoutArgAsksWhenDirty(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	start.activeBuffer().Insert([]byte("X"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("남이 쓴 것\n"), 0644))

	confirm := runCommand(start, "e")
	require.IsType(t, viewConfirmDiscard{}, confirm)

	forced := runCommand(start, "e!")
	require.IsType(t, viewEditorNormal{}, forced)
	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, forced).Line(0)))
}

// `:tabnew <파일>` 은 보고 있던 tab 바로 뒤에 그 파일로 새 tab 을 연다.
func TestTabNewWithFileOpensTab(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb\n"), 0644))

	m := runCommand(start, "tabnew "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	require.Len(t, m.(viewEditorNormal).buffers, 2)
	assert.Equal(t, 1, activeOf(t, m))
	assert.Equal(t, "bbb", string(bufferOf(t, m).Line(0)))
}

// 이미 열려 있으면 새 tab 을 만들지 않고 그 tab 으로 옮겨간다.
func TestTabNewWithFileMovesToTabAlreadyOpen(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt", "b.txt")

	m := runCommand(start, "tabnew "+filepath.Join(dir, "b.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 2)
	assert.Equal(t, 1, activeOf(t, m))
}

// 인자를 받는 명령은 `:w` `:e` `:tabnew` 뿐이다.
// `:wq foo` 를 조용히 받으면 foo 에 저장한 것처럼 보인다.
func TestCommandWithArgIsRejected(t *testing.T) {
	start, _ := newFilesEditor(t, "a.txt")

	m := runCommand(start, "wq other.txt")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "알 수 없는 명령")
	assert.NotContains(t, barOf(t, m)[1], "저장함")
}

// 파일 이름은 하나만 받는다. 여럿을 tab 여러 개로 여는 것은 CLI 인자의 몫이다.
func TestEditTakesOnlyOneFile(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")

	m := runCommand(start, "e "+filepath.Join(dir, "a.txt")+" "+filepath.Join(dir, "a.txt"))

	require.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], "파일은 하나만")
}
