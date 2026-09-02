package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// afterFocus·afterFileTick 은 msg 를 먹이고 검사 작업이 끝난 뒤까지 몬다.
//
// 검사가 백그라운드로 내려가서(ADR-0044) 시험도 그 길을 그대로 지나야 한다. `Update` 가
// 돌려주는 것이 둘이라 한 줄로 쓸 수 없어서 이 자리에 모아 두었다.
func afterFocus(t *testing.T, m tea.Model) tea.Model {
	t.Helper()

	next, cmd := m.Update(tea.FocusMsg{})
	require.NotNil(t, cmd, "검사 작업이 시작되지 않았다")

	return settle(t, next, cmd)
}

func afterFileTick(t *testing.T, m tea.Model) tea.Model {
	t.Helper()

	next, cmd := m.Update(fileTickMsg{})
	require.NotNil(t, cmd, "검사 작업이 시작되지 않았다")

	return settle(t, next, cmd)
}

// 다른 창에서 고치고 돌아오면 그 자리에서 가져온다. 잃을 것이 없으면 묻지 않는다(ADR-0038).
func TestFocusReloadsOutsideChange(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model := afterFocus(t, m)

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
	assert.NotContains(t, barOf(t, model)[0], "[!]", "어긋난 것이 없어서 마커도 없다")
}

// 보고 있는 채로 밖에서 바뀌는 파일은 포커스가 오가지 않는다. 주기 tick 이 그것을 잡는다.
func TestTickReloadsOutsideChange(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model := afterFileTick(t, m)

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
}

// mode 를 오가는 동안에도 파일 검사 고리가 끊기지 않는다. 한 mode 라도 흘려보내면 검사가
// 조용히 멈춘다 — git tick 과 같은 그물이다 (ADR-0038, ADR-0043).
//
// 볼 것이 둘이다. tick 을 받아 검사를 시작하는 것과, 검사가 끝난 것을 받아 다음 cooldown 을
// 거는 것이다. 뒤쪽을 빠뜨리면 그 mode 에 머무는 동안 고리가 끊긴다.
//
// mode 마다 편집기를 새로 만든다. `*editor` 를 나눠 쓰면 앞 subtest 가 시작한 검사가 뒤까지
// 남아서, 뒤에서는 「이미 도는 중」이라 새로 시작하지 않는다.
func TestEveryModeKeepsFileTicking(t *testing.T) {
	modes := []struct {
		name  string
		enter func(t *testing.T) tea.Model
		want  tea.Model
	}{
		{"normal", func(*testing.T) tea.Model { return newTestEditor("abc\n", 80, 10) }, viewEditorNormal{}},
		{"insert", func(*testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), "i") }, viewEditorInsert{}},
		{"visual", func(*testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), "v") }, viewEditorVisual{}},
		{"command", func(*testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), ":") }, viewEditorCommand{}},
		{"search", func(*testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), "/") }, viewEditorSearch{}},
		{"palette", func(*testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), "ctrl+p") }, viewPalette{}},
		{"jobs", func(*testing.T) tea.Model {
			return send(newTestEditor("abc\n", 80, 10), ":", "j", "o", "b", "s", "enter")
		}, viewJobs{}},
		{"tree", func(t *testing.T) tea.Model {
			return send(newTreeEditor(t, 80, 10), "ctrl+w", "ctrl+w")
		}, viewSidebar{}},
		{"confirm", func(*testing.T) tea.Model {
			return send(send(newTestEditor("abc\n", 80, 10), "x"), ":", "q", "enter")
		}, viewConfirmDiscard{}},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Run("tick 이 검사를 시작한다", func(t *testing.T) {
				model := mode.enter(t)
				require.IsType(t, mode.want, model, "이 mode 로 들어가지 못했다")

				model, cmd := model.Update(fileTickMsg{})

				assert.NotNil(t, cmd, "이 mode 에서 tick 이 끊긴다")
				assert.True(t, jobRunningIn(t, model, fileJobName), "검사가 시작되지 않았다")
			})

			t.Run("검사가 끝나면 다음 것을 예약한다", func(t *testing.T) {
				model := mode.enter(t)
				require.IsType(t, mode.want, model, "이 mode 로 들어가지 못했다")

				_, cmd := model.Update(jobDoneMsg{name: fileJobName})

				assert.NotNil(t, cmd, "이 mode 에서 cooldown 이 끊긴다")
			})
		})
	}
}

// insert 에서도 아직 아무것도 치지 않았으면 가져온다.
//
// 검사가 작업으로 내려가며 「어느 mode 인가」가 판정에서 빠졌다. 결과가 돌아올 때 mode 를 알
// 수 없고(ADR-0002 에서 mode 는 model 타입이다) 알 필요도 없다 — 잃을 것이 있는지, 곧 `dirty`
// 인지만 본다. 예전에는 insert 를 mode 로 걸러서 마커만 붙였다 (ADR-0038, ADR-0044).
func TestTickReadsInInsertModeWhenClean(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i")
	require.IsType(t, viewEditorInsert{}, model)
	require.False(t, bufferOf(t, model).dirty, "아직 아무것도 치지 않았다")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFileTick(t, model)

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
}

// 치기 시작했으면 갈아끼우지 않는다. 손에 든 것을 버리는 일은 사람이 정한다.
func TestTickKeepsBufferInInsertModeWhenDirty(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i", "z")
	require.IsType(t, viewEditorInsert{}, model)
	require.True(t, bufferOf(t, model).dirty)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFileTick(t, model)

	assert.Equal(t, "zabc", string(bufferOf(t, model).lines[0]), "치는 중에는 갈아끼우지 않는다")
	assert.Contains(t, barOf(t, model)[0], "[!]", "마커는 붙는다")
}

// 커서는 줄 번호와 화면 칸을 이어받는다. 밖에서 포매터를 돌린 뒤 같은 자리에서 이어 본다.
func TestAutoReloadKeepsCursorLine(t *testing.T) {
	m, path := newWideFileEditor(t, "a\nb\nc\n")

	model := send(m, "j", "j")
	require.Equal(t, 2, bufferOf(t, model).cursor.line)

	require.NoError(t, os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0644))
	model = afterFocus(t, model)

	assert.Equal(t, 2, bufferOf(t, model).cursor.line)
}

// 자동으로 읽으면 undo 이력이 사라진다. 저장한 뒤에도 `u` 로 돌아갈 것이 없다 —
// ADR-0038 이 감수한 값이다.
func TestAutoReloadDropsUndoHistory(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x", ":", "w", "enter")
	require.Equal(t, "bc", string(bufferOf(t, model).lines[0]))

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	model = afterFocus(t, model)

	model = send(model, "u")

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]), "`u` 로 돌아갈 것이 없다")
}

// 저장하지 않은 변경이 있으면 읽지 않는다. 무엇을 남길지는 `:e` 의 확인창이 묻는다.
func TestFocusMarksOutsideChangeWhenDirty(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.True(t, bufferOf(t, model).dirty)

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	model = afterFocus(t, model)

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
		buffers: []viewport{buf},
		width:   200,
		height:  5 + tablineHeight + statusBarHeight,
	}}

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	model := afterFocus(t, m)

	assert.Equal(t, "", string(bufferOf(t, model).lines[0]), "빈 buffer 가 조용히 채워지지 않는다")
	assert.Contains(t, barOf(t, model)[1], "새로 생겼습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 마커는 다음 키에 사라지지 않는다. 알림은 지나가고 상태는 남는다.
func TestOutsideMarkerOutlivesMessage(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFocus(t, model)
	model = send(model, "j")

	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다", "알림은 다음 키에 사라진다")
	assert.Contains(t, barOf(t, model)[0], "[!]", "마커는 남는다")
}

// 이미 알린 것을 또 알리지 않는다. 밖에서 계속 바뀌는 파일을 띄워두면 아래 줄이 그 알림에 덮인다.
func TestOutsideChangeTellsOnce(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFocus(t, model)
	require.Contains(t, barOf(t, model)[1], "바뀌었습니다")

	model = send(model, "j")
	require.NoError(t, os.WriteFile(path, []byte("또 남이 쓴 것\n"), 0644))
	model = afterFocus(t, model)

	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Contains(t, barOf(t, model)[0], "[!]")
}

// 밖의 변경이 되돌아가면 마커도 사라진다. 되돌아간 것은 알리지 않는다 — 할 일이 없다.
func TestOutsideMarkerClearsWhenSameAgain(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	model := send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFocus(t, model)
	require.Contains(t, barOf(t, model)[0], "[!]")

	// 알림은 다음 키에 사라진다. 그것까지 지나야 이번 복귀가 조용한지 볼 수 있다.
	model = send(model, "j")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))
	model = afterFocus(t, model)

	assert.NotContains(t, barOf(t, model)[0], "[!]")
	assert.NotContains(t, barOf(t, model)[1], "바뀌었습니다", "되돌아간 것은 알리지 않는다")
}

// `:w!` 로 덮어쓰면 어긋난 것이 없어진다. 마커도 그 자리에서 사라진다.
func TestForceSaveClearsOutsideMarker(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "x")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFocus(t, model)
	require.Contains(t, barOf(t, model)[0], "[!]")

	model = send(model, ":", "w", "!", "enter")

	assert.NotContains(t, barOf(t, model)[0], "[!]")
	assert.Contains(t, barOf(t, model)[1], "저장함")
}

// 치던 중에 다른 창을 만지고 돌아오는 것이 흔하다. 손에 든 것이 있으면 알리기만 한다 —
// 커서 아래 내용이 갑자기 바뀌지 않는다 (ADR-0038, ADR-0044).
func TestFocusMarksOutsideChangeInInsertMode(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i", "z")
	require.IsType(t, viewEditorInsert{}, model)
	require.True(t, bufferOf(t, model).dirty)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model = afterFocus(t, model)

	assert.Equal(t, "zabc", string(bufferOf(t, model).lines[0]), "치는 중에는 갈아끼우지 않는다")
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

	model = afterFocus(t, model)

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
}

// 달라진 것이 없으면 조용하다. 창을 오갈 때마다 아래 줄이 흔들리면 안 된다.
func TestFocusKeepsMessageWhenUnchanged(t *testing.T) {
	m, _ := newWideFileEditor(t, "abc\n")

	var model tea.Model = send(m, ":", "w", "enter")
	require.Contains(t, barOf(t, model)[1], "저장함")

	model = afterFocus(t, model)

	assert.Contains(t, barOf(t, model)[1], "저장함", "알릴 것이 없으면 덮어쓰지 않는다")
	assert.NotContains(t, barOf(t, model)[0], "[!]")
}

// 앞잡이가 맞으면 읽지 않는다.
//
// 읽지 않았다는 것은 「내용이 달라져 있어도 그대로라고 답한다」 로 드러난다. mtime 을 그대로
// 두고 기준 해시만 엉뚱하게 주면, 읽었다면 `outsideModified` 가 나와야 한다 (ADR-0044).
func TestOutsideCheckSkipsReadWhenStatMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	info, err := os.Lstat(path)
	require.NoError(t, err)

	result := checkOutsideFile(path, []byte("맞지 않는 기준"), info.Size(), info.ModTime())

	assert.Equal(t, outsideSame, result.change)
	assert.Nil(t, result.next, "읽지 않았으므로 갈아끼울 것도 없다")
}

// 앞잡이가 어긋나면 읽는다. 갈아끼울 내용까지 여기서 만들어 둔다.
func TestOutsideCheckReadsWhenStatDiffers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	info, err := os.Lstat(path)
	require.NoError(t, err)

	result := checkOutsideFile(path, []byte("맞지 않는 기준"), info.Size(), info.ModTime().Add(-time.Second))

	assert.Equal(t, outsideModified, result.change)
	require.NotNil(t, result.next, "갈아끼울 내용을 백그라운드에서 만들어 와야 한다")
	assert.Equal(t, "abc", string(result.next.lines[0]))
}

// 열 때 없던 파일에는 앞잡이를 쓰지 않는다.
//
// 견줄 기준 해시가 없어서 stat 이 같다는 것이 「내용이 같다」 를 뜻하지 않는다. 앞잡이를 쓰면
// 「새로 생겼습니다」 를 한 번 알린 뒤 다음 검사에서 마커가 조용히 지워진다.
func TestOutsideCheckDoesNotTrustStatWithoutBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	info, err := os.Lstat(path)
	require.NoError(t, err)

	result := checkOutsideFile(path, nil, info.Size(), info.ModTime())

	assert.Equal(t, outsideCreated, result.change)
}

// 검사가 본 stat 은 buffer 에 남는다. 남지 않으면 다음 검사가 또 파일 전체를 읽는다.
func TestOutsideCheckRecordsStat(t *testing.T) {
	m, _ := newWideFileEditor(t, "abc\n")
	require.True(t, m.activeBuffer().disk.mtime.IsZero(), "아직 앞잡이가 없다")

	model := afterFileTick(t, m)

	assert.False(t, bufferOf(t, model).disk.mtime.IsZero(), "앞잡이가 남지 않으면 매번 읽는다")
}

// 검사하는 동안 기준이 달라졌으면 그 결과는 낡은 것이다. 넣지 않고 물러난다.
//
// 저장하거나 `:e` 로 다시 읽으면 `diskHash` 가 바뀐다. 그때 도착한 결과를 넣으면 방금 한 일을
// 덮는다 — 다음 tick 이 새 기준으로 다시 본다 (ADR-0044).
func TestOutsideResultDroppedWhenBaselineMoved(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")

	next := newBuffer(path, []byte("남이 쓴 것\n"))
	m.applyOutsideResult(path, []byte("검사를 시작할 때의 기준"), outsideResult{
		change: outsideModified,
		next:   &next,
	})

	assert.Equal(t, "abc", string(m.activeBuffer().lines[0]), "낡은 결과를 넣었다")
	assert.Equal(t, outsideSame, m.activeBuffer().disk.outside, "마커도 붙이지 않는다")
}

// tab 이 없어도 검사 작업은 시작한다. 시작하지 않으면 끝나지도 않아서 cooldown 고리가
// 그 자리에서 멈추고, 그러면 빈 화면에서 파일을 열어도 바깥 변경을 다시 보지 않는다
// (ADR-0044, ADR-0064).
func TestOutsideCheckRunsWithoutTab(t *testing.T) {
	e := newEmptyEditor(80, 20).editor

	assert.NotNil(t, e.startOutsideCheck(), "고리가 여기서 끊기면 안 된다")
}

// 주기 검사가 열려 있는 tab 전부를 본다.
//
// **보고 있지 않은 tab 이 여기서 풀린다.** 예전에는 활성 buffer 하나만 봐서, 다른 tab 의
// 파일이 밖에서 바뀌면 그 낡은 내용이 tab 을 옮길 때까지 남았고 gopls 에는 그 낡은 overlay
// 가 계속 실려 갔다. ADR-0092 가 「알리지 않는 것보다 나쁜 자리」로 적어 둔 것이다(ADR-0093).
func TestTickChecksEveryOpenTab(t *testing.T) {
	m, _ := newWideFileEditor(t, "첫째\n")

	second := filepath.Join(t.TempDir(), "second.txt")
	require.NoError(t, os.WriteFile(second, []byte("둘째\n"), 0644))

	buf, err := OpenBuffer(second)
	require.NoError(t, err)

	m.buffers = append(m.buffers, buf)

	// 보고 있지 않은 쪽을 밖에서 고친다. 활성 tab 은 첫째 그대로다.
	require.NoError(t, os.WriteFile(second, []byte("남이 쓴 것\n"), 0644))

	model := afterFileTick(t, m)

	editor := model.(viewEditorNormal).editor
	require.Equal(t, 0, editor.active, "활성 tab 이 옮겨졌다")

	assert.Equal(t, "첫째", string(editor.buffers[0].lines[0]), "보고 있는 tab 이 건드려졌다")
	assert.Equal(t, "남이 쓴 것", string(editor.buffers[1].lines[0]), "보고 있지 않은 tab 을 다시 읽지 않았다")
}

// tab 이 하나도 없어도 검사 작업은 돌고 끝난다.
//
// 시작하지 않으면 끝나지도 않아서 cooldown 고리가 그 자리에서 멈추고, 그러면 빈 화면에서
// 파일을 열어도 검사가 다시 돌지 않는다(ADR-0044, ADR-0064).
func TestTickRunsWithoutTabs(t *testing.T) {
	m := newTestEditor("abc\n", 80, 10)
	m.buffers = nil
	m.active = -1

	_, cmd := m.Update(fileTickMsg{})

	assert.NotNil(t, cmd, "tab 이 없다고 고리가 끊긴다")
}
