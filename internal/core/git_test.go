package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tickOf 는 tick 하나를 먹이고 mode 가 돌려준 Cmd 를 준다.
func tickOf(m tea.Model) (tea.Model, tea.Cmd) {
	return m.Update(gitTickMsg(time.Now()))
}

// tick 을 받으면 갱신 작업이 시작된다. 읽는 것은 그 작업이고 Update 는 기다리지 않는다.
func TestGitTickStartsJob(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, cmd := tickOf(m)

	require.NotNil(t, cmd, "갱신 작업의 첫 조각을 받을 Cmd 가 나오지 않는다")
	assert.True(t, m.(viewEditorNormal).jobRunning(gitJobName, nil))
}

// 갱신이 아직 도는 중이면 다음 tick 은 새로 시작하지 않는다.
// 느린 저장소에서 갱신이 겹쳐 쌓이면 훑기가 끝없이 늘어난다.
func TestGitTickDoesNotStack(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = tickOf(m)
	m, _ = tickOf(m)

	assert.Len(t, m.(viewEditorNormal).jobs, 1)
}

// 다음 갱신은 앞 갱신이 *끝난 뒤* 부터 잰다. tick 을 받는 자리에서 걸면 고정 주기가 되어,
// 갱신이 주기보다 오래 걸리는 저장소에서 쉼 없이 도는 상태가 된다(ADR-0043).
func TestGitCooldownStartsWhenJobFinishes(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = tickOf(m)
	require.False(t, m.(viewEditorNormal).gitTickScheduled, "아직 갱신이 도는 중이다")

	m, cmd := m.Update(jobDoneMsg{name: gitJobName})

	assert.NotNil(t, cmd, "갱신이 끝났는데 다음 것을 예약하지 않는다")
	assert.True(t, m.(viewEditorNormal).gitTickScheduled)
}

// 예약된 tick 은 늘 하나다. `:w` 로 갱신이 한 번 더 돌면 그것이 끝날 때도 이 자리를 지나는데,
// 그때마다 새로 걸면 저장할 때마다 갱신이 두 배로 는다(ADR-0043).
func TestGitCooldownDoesNotSplit(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = tickOf(m)
	m, first := m.Update(jobDoneMsg{name: gitJobName})
	require.NotNil(t, first)

	_, second := m.Update(jobDoneMsg{name: gitJobName})

	assert.Nil(t, second, "이미 걸어둔 것이 있는데 하나 더 걸었다")
}

// git 이 아닌 작업이 끝나는 것은 cooldown 과 무관하다. 여기서 걸면 인덱싱 한 번에
// 갱신 고리가 하나 더 는다.
func TestOtherJobDoneDoesNotScheduleGitTick(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	_, cmd := m.Update(jobDoneMsg{name: "파일 인덱싱"})

	assert.Nil(t, cmd)
	assert.False(t, m.(viewEditorNormal).gitTickScheduled)
}

// 읽어온 값은 작업이 apply 로 넣는다. statusBar 는 들고 있는 것을 그리기만 한다.
func TestGitJobAppliesStatus(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	status := gitStatus{branch: "master", commit: "a1b2c3d", dirty: true}
	m = finishOf(t, m, gitJobName, jobProgress{
		done:    1,
		total:   1,
		summary: status.label(),
		apply:   func(e *editor) { e.git = status },
	})

	assert.Equal(t, status, m.(viewEditorNormal).git)
	assert.Contains(t, barOf(t, m)[0], "master(a1b2c3d*)")
}

// mode 를 오가는 동안에도 고리가 끊기지 않는다. 한 mode 라도 흘려보내면 갱신이 조용히 멈춘다.
//
// cooldown 에서는 볼 것이 둘이다 — tick 을 받아 갱신을 시작하는 것과, 갱신이 끝난 것을 받아
// 다음 cooldown 을 거는 것이다. 뒤쪽을 빠뜨리면 그 mode 에 머무는 동안 고리가 끊긴다(ADR-0043).
//
// mode 마다 편집기를 새로 만든다. `*editor` 를 나눠 쓰면 앞 subtest 가 시작한 갱신이 뒤까지
// 남아서, 뒤에서는 「이미 도는 중」이라 새로 시작하지 않는다.
func TestEveryModeKeepsGitTicking(t *testing.T) {
	modes := []struct {
		name  string
		enter func(t *testing.T) tea.Model
		want  tea.Model
	}{
		{"normal", func(*testing.T) tea.Model { return newTestEditor("abc\n", 80, 10) }, viewEditorNormal{}},
		{"insert", func(*testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), "i") }, viewEditorInsert{}},
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
			t.Run("tick 이 갱신을 시작한다", func(t *testing.T) {
				model := mode.enter(t)
				require.IsType(t, mode.want, model, "이 mode 로 들어가지 못했다")

				model, cmd := tickOf(model)

				assert.NotNil(t, cmd, "이 mode 에서 tick 이 끊긴다")
				assert.True(t, jobRunningIn(t, model, gitJobName), "갱신이 시작되지 않았다")
			})

			t.Run("갱신이 끝나면 다음 것을 예약한다", func(t *testing.T) {
				model := mode.enter(t)
				require.IsType(t, mode.want, model, "이 mode 로 들어가지 못했다")

				_, cmd := model.Update(jobDoneMsg{name: gitJobName})

				assert.NotNil(t, cmd, "이 mode 에서 cooldown 이 끊긴다")
			})
		})
	}
}

// 파일을 여는 길은 저마다 갱신을 같이 발행한다. openTab 이 대신 해주지 않으므로(ADR-0030)
// 여는 길이 새로 생기면 이 목록에도 한 줄이 는다. 빠뜨리면 그 길로 연 파일에서만
// 표시가 최대 5 초 낡은 채로 있는데, 화면에는 아무 표시도 나지 않는다.
func TestOpenPathsEmitGitRefresh(t *testing.T) {
	other := filepath.Join(t.TempDir(), "other.txt")
	require.NoError(t, os.WriteFile(other, []byte("다른 파일\n"), 0644))

	paths := map[string]func(*editor) (tea.Model, tea.Cmd){
		":tabnew <파일>": func(e *editor) (tea.Model, tea.Cmd) {
			return viewEditorCommand{editor: e, input: "tabnew " + other}.run()
		},
		":e <파일>": func(e *editor) (tea.Model, tea.Cmd) {
			return editFile(e, other)
		},
		"팔레트에서 고르기": func(e *editor) (tea.Model, tea.Cmd) {
			return viewPalette{editor: e}.openFile(other)
		},
	}

	for name, open := range paths {
		t.Run(name, func(t *testing.T) {
			m, _ := newFileEditor(t, "abc\n")

			_, cmd := open(m.editor)

			require.NotNil(t, cmd, "갱신 Cmd 가 밖으로 나오지 않는다")
			assert.True(t, m.jobRunning(gitJobName, nil))
		})
	}
}

// 트리에서 Enter 로 여는 길도 같다. 트리는 fixture 가 있어야 해서 따로 본다.
func TestTreeEnterEmitsGitRefresh(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	revealSyncIn(t, m.editor, filepath.Join(m.sidebar.root, "main.go"))
	require.Equal(t, "main.go", m.sidebar.selectedNode().name)

	_, cmd := viewSidebar{editor: m.editor}.enter()

	require.NotNil(t, cmd, "갱신 Cmd 가 밖으로 나오지 않는다")
	assert.True(t, m.jobRunning(gitJobName, nil))
}

// tab 을 오가는 것에는 git 이 붙지 않는다. `gt` 마다 프로세스가 뜨면 ADR-0009 의 출발점이 깨진다.
func TestSwitchingTabsDoesNotRefresh(t *testing.T) {
	m := newTestEditor("abc\n", 80, 10)
	m.buffers = append(m.buffers, newBuffer("other.txt", []byte("x\n")))

	model := send(m, "g", "t")

	require.IsType(t, viewEditorNormal{}, model)
	assert.False(t, model.(viewEditorNormal).jobRunning(gitJobName, nil))
}

// 끊긴 갱신이 읽어온 것은 "저장소가 아니다" 와 구별되지 않는다.
// 그대로 넣으면 편집기를 끝내는 길에 표시가 사라진다.
func TestCancelledGitJobKeepsStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	e := &editor{ctx: ctx}

	cmd := e.startGitRefresh()
	require.NotNil(t, cmd)
	cancel()

	msg := cmd()
	progress, ok := msg.(jobProgressMsg)
	require.True(t, ok)

	assert.Nil(t, progress.apply, "값을 넣지 않는다")
	assert.Error(t, progress.err)
}

// jobRunningIn 은 어느 mode 든 그 작업이 도는 중인지 본다. mode 마다 model 타입이 달라서
// editor 를 꺼내는 자리가 필요하다.
func jobRunningIn(t *testing.T, m tea.Model, name string) bool {
	t.Helper()

	switch v := m.(type) {
	case viewEditorNormal:
		return v.jobRunning(name, nil)
	case viewEditorInsert:
		return v.jobRunning(name, nil)
	case viewEditorCommand:
		return v.jobRunning(name, nil)
	case viewEditorSearch:
		return v.jobRunning(name, nil)
	case viewEditorVisual:
		return v.jobRunning(name, nil)
	case viewPalette:
		return v.jobRunning(name, nil)
	case viewJobs:
		return v.jobRunning(name, nil)
	case viewSidebar:
		return v.jobRunning(name, nil)
	case viewConfirmDiscard:
		return v.jobRunning(name, nil)
	default:
		t.Fatalf("editor 를 든 mode 가 아니다: %T", m)

		return false
	}
}
