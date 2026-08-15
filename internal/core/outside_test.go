package core

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWideFileEditor 는 statusBar 가 잘리지 않을 만큼 넓은 편집 화면이다.
// 임시 파일 경로가 길어서 기본 폭(40 칸) 에서는 경로 뒤의 마커가 통째로 잘려 나간다.
func newWideFileEditor(t *testing.T, data string) (viewEditorNormal, string) {
	t.Helper()

	m, path := newFileEditor(t, data)
	m.width = 200

	return m, path
}

// 다른 창에서 고치고 돌아오면 그 자리에서 안다. 저장하려다 막혀서야 알게 되지 않는다.
func TestFocusFindsOutsideChange(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ := m.Update(tea.FocusMsg{})

	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]", "마커가 붙는다")
	assert.Equal(t, "abc", string(bufferOf(t, model).lines[0]), "다시 읽지는 않는다")
}

// 마커는 다음 키에 사라지지 않는다. 알림은 지나가고 상태는 남는다.
func TestOutsideMarkerOutlivesMessage(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ := m.Update(tea.FocusMsg{})
	model = send(model, "j")

	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다", "알림은 다음 키에 사라진다")
	assert.Contains(t, barOf(t, model)[0], "[!]", "마커는 남는다")
}

// 이미 알린 것을 또 알리지 않는다. 밖에서 계속 바뀌는 파일을 띄워두면 아래 줄이 그 알림에 덮인다.
func TestOutsideChangeTellsOnce(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ := m.Update(tea.FocusMsg{})
	require.Contains(t, barOf(t, model)[1], "바뀌었습니다")

	model = send(model, "j")
	require.NoError(t, os.WriteFile(path, []byte("또 남이 쓴 것\n"), 0644))
	model, _ = model.Update(tea.FocusMsg{})

	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 밖의 변경이 되돌아가면 마커도 사라진다. 되돌아간 것은 알리지 않는다 — 할 일이 없다.
func TestOutsideMarkerClearsWhenSameAgain(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ := m.Update(tea.FocusMsg{})
	require.Contains(t, barOf(t, model)[0], "[!]")

	// 알림은 다음 키에 사라진다. 그것까지 지나야 이번 복귀가 조용한지 볼 수 있다.
	model = send(model, "j")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))
	model, _ = model.Update(tea.FocusMsg{})

	assert.NotContains(t, barOf(t, model)[0], "[!]")
	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다", "되돌아간 것은 알리지 않는다")
}

// `:w!` 로 덮어쓰면 어긋난 것이 없어진다. 마커도 그 자리에서 사라진다.
func TestForceSaveClearsOutsideMarker(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	var model tea.Model = m
	model, _ = model.Update(tea.FocusMsg{})
	require.Contains(t, barOf(t, model)[0], "[!]")

	model = send(model, ":", "w", "!", "enter")

	assert.NotContains(t, barOf(t, model)[0], "[!]")
	assert.Contains(t, barOf(t, model)[1], "저장함")
}

// 치던 중에 다른 창을 만지고 돌아오는 것이 흔하다. insert mode 에서도 본다.
func TestFocusFindsOutsideChangeInInsertMode(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i")
	require.IsType(t, viewEditorInsert{}, model)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})

	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 트리에 포커스가 있어도 본다. 마커가 mode 와 무관한 자리라 알릴 곳이 있다(ADR-0031).
func TestFocusFindsOutsideChangeInSidebar(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	m.sidebar = openSidebarSync(t, newTreeFixture(t))

	var model tea.Model = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})

	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 달라진 것이 없으면 조용하다. 창을 오갈 때마다 아래 줄이 흔들리면 안 된다.
func TestFocusKeepsMessageWhenUnchanged(t *testing.T) {
	m, _ := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, ":", "w", "enter")
	require.Contains(t, barOf(t, model)[1], "저장함")

	model, _ = model.Update(tea.FocusMsg{})

	assert.Contains(t, barOf(t, model)[1], "저장함", "알릴 것이 없으면 덮어쓰지 않는다")
	assert.NotContains(t, barOf(t, model)[0], "[!]")
}
