package core

import (
	"os"
	"path/filepath"
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

// 다른 창에서 고치고 돌아오면 그 자리에서 가져온다. 잃을 것이 없으면 묻지 않는다(ADR-0038).
func TestFocusReloadsOutsideChange(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ := m.Update(tea.FocusMsg{})

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
	assert.NotContains(t, barOf(t, model)[0], "[!]", "어긋난 것이 없어서 마커도 없다")
}

// 보고 있는 채로 밖에서 바뀌는 파일은 포커스가 오가지 않는다. 주기 tick 이 그것을 잡는다.
func TestTickReloadsOutsideChange(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, cmd := m.Update(fileTickMsg{})

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
	assert.NotNil(t, cmd, "다음 tick 을 예약한다")
}

// mode 를 오가는 동안에도 파일 검사 고리가 끊기지 않는다. 한 mode 라도 tick 을 흘려보내면
// 다음 tick 을 아무도 예약하지 않아 검사가 조용히 멈춘다 — git tick 과 같은 그물이다(ADR-0038).
func TestEveryModeKeepsFileTicking(t *testing.T) {
	base := newTestEditor("abc\n", 80, 10)

	modes := []struct {
		name  string
		model tea.Model
		want  tea.Model
	}{
		{"normal", base, viewEditorNormal{}},
		{"insert", send(base, "i"), viewEditorInsert{}},
		{"visual", send(base, "v"), viewEditorVisual{}},
		{"command", send(base, ":"), viewEditorCommand{}},
		{"search", send(base, "/"), viewEditorSearch{}},
		{"palette", send(base, "ctrl+p"), viewPalette{}},
		{"jobs", send(base, ":", "j", "o", "b", "s", "enter"), viewJobs{}},
		{"tree", send(newTreeEditor(t, 80, 10), "ctrl+w", "ctrl+w"), viewSidebar{}},
		{"confirm", send(send(base, "x"), ":", "q", "enter"), viewConfirmDiscard{}},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			require.IsType(t, mode.want, mode.model, "이 mode 로 들어가지 못했다")

			_, cmd := mode.model.Update(fileTickMsg{})

			assert.NotNil(t, cmd, "이 mode 에서 tick 이 끊긴다")
		})
	}
}

// tick 을 보지 않는 mode 를 지나도 buffer 는 그대로다. 치는 중에 내용이 갈아끼워지지 않는다.
func TestTickKeepsLoopAliveInInsertMode(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i")
	require.IsType(t, viewEditorInsert{}, model)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, cmd := model.Update(fileTickMsg{})

	assert.NotNil(t, cmd, "지나가는 mode 도 다음 tick 을 예약한다")
	assert.Equal(t, "abc", string(bufferOf(t, model).lines[0]), "치는 중에는 갈아끼우지 않는다")
}

// 커서는 줄 번호와 화면 칸을 이어받는다. 밖에서 포매터를 돌린 뒤 같은 자리에서 이어 본다.
func TestAutoReloadKeepsCursorLine(t *testing.T) {
	m, path := newWideFileEditor(t, "a\nb\nc\n")

	model := send(m, "j", "j")
	require.Equal(t, 2, bufferOf(t, model).cursorLine)

	require.NoError(t, os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0644))
	model, _ = model.Update(tea.FocusMsg{})

	assert.Equal(t, 2, bufferOf(t, model).cursorLine)
}

// 자동으로 읽으면 undo 이력이 사라진다. 저장한 뒤에도 `u` 로 돌아갈 것이 없다 —
// ADR-0038 이 감수한 값이다.
func TestAutoReloadDropsUndoHistory(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x", ":", "w", "enter")
	require.Equal(t, "bc", string(bufferOf(t, model).lines[0]))

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	model, _ = model.Update(tea.FocusMsg{})

	model = send(model, "u")

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]), "`u` 로 돌아갈 것이 없다")
}

// 저장하지 않은 변경이 있으면 읽지 않는다. 무엇을 남길지는 `:e` 의 확인창이 묻는다.
func TestFocusMarksOutsideChangeWhenDirty(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.True(t, bufferOf(t, model).dirty)

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	model, _ = model.Update(tea.FocusMsg{})

	assert.Equal(t, "bc", string(bufferOf(t, model).lines[0]), "손에 든 것을 버리지 않는다")
	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]", "마커가 붙는다")
}

// 없던 파일이 생긴 것은 자동으로 읽지 않는다. 새로 쓰려던 자리일 수 있어서 손으로 정한다.
func TestOutsideCreatedIsNotReadAutomatically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	m := viewEditorNormal{editor: &editor{
		buffers: []Buffer{buf},
		width:   200,
		height:  5 + tablineHeight + statusBarHeight,
	}}

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	model, _ := m.Update(tea.FocusMsg{})

	assert.Equal(t, "", string(bufferOf(t, model).lines[0]), "빈 buffer 가 조용히 채워지지 않는다")
	assert.Contains(t, barOf(t, model)[1], "새로 생겼습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 마커는 다음 키에 사라지지 않는다. 알림은 지나가고 상태는 남는다.
func TestOutsideMarkerOutlivesMessage(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})
	model = send(model, "j")

	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다", "알림은 다음 키에 사라진다")
	assert.Contains(t, barOf(t, model)[0], "[!]", "마커는 남는다")
}

// 이미 알린 것을 또 알리지 않는다. 밖에서 계속 바뀌는 파일을 띄워두면 아래 줄이 그 알림에 덮인다.
func TestOutsideChangeTellsOnce(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})
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

	model := send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})
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

	var model tea.Model = send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})
	require.Contains(t, barOf(t, model)[0], "[!]")

	model = send(model, ":", "w", "!", "enter")

	assert.NotContains(t, barOf(t, model)[0], "[!]")
	assert.Contains(t, barOf(t, model)[1], "저장함")
}

// 치던 중에 다른 창을 만지고 돌아오는 것이 흔하다. insert 는 알리기만 한다 —
// 커서 아래 내용이 갑자기 바뀌지 않는다(ADR-0038).
func TestFocusMarksOutsideChangeInInsertMode(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i")
	require.IsType(t, viewEditorInsert{}, model)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})

	assert.Equal(t, "abc", string(bufferOf(t, model).lines[0]), "치는 중에는 갈아끼우지 않는다")
	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 트리에 포커스가 있어도 본다. 편집 영역은 옆에 그려져 있으므로 가져온다(ADR-0031, ADR-0038).
func TestFocusReloadsOutsideChangeInSidebar(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	m.sidebar = openSidebarSync(t, newTreeFixture(t))

	var model tea.Model = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ = model.Update(tea.FocusMsg{})

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
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
