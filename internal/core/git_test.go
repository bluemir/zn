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

	require.NotNil(t, cmd, "다음 tick 을 예약하지 않으면 갱신이 여기서 멈춘다")
	assert.True(t, m.(viewEditorNormal).jobRunning(gitJobName))
}

// 갱신이 아직 도는 중이면 다음 tick 은 새로 시작하지 않는다.
// 느린 저장소에서 갱신이 겹쳐 쌓이면 프로세스가 끝없이 늘어난다.
func TestGitTickDoesNotStack(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 80, 10)

	m, _ = tickOf(m)
	m, _ = tickOf(m)

	assert.Len(t, m.(viewEditorNormal).jobs, 1)
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

// mode 를 오가는 동안에도 고리가 끊기지 않는다. 한 mode 라도 tick 을 흘려보내면
// 다음 tick 을 아무도 예약하지 않아 갱신이 조용히 멈춘다.
func TestEveryModeKeepsGitTicking(t *testing.T) {
	base := newTestEditor("abc\n", 80, 10)

	modes := []struct {
		name  string
		model tea.Model
		want  tea.Model
	}{
		{"normal", base, viewEditorNormal{}},
		{"insert", send(base, "i"), viewEditorInsert{}},
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

			_, cmd := tickOf(mode.model)

			assert.NotNil(t, cmd, "이 mode 에서 tick 이 끊긴다")
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
			assert.True(t, m.jobRunning(gitJobName))
		})
	}
}

// 트리에서 Enter 로 여는 길도 같다. 트리는 fixture 가 있어야 해서 따로 본다.
func TestTreeEnterEmitsGitRefresh(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	require.True(t, m.sidebar.reveal(filepath.Join(m.sidebar.root, "main.go")))

	_, cmd := viewSidebar{editor: m.editor}.enter()

	require.NotNil(t, cmd, "갱신 Cmd 가 밖으로 나오지 않는다")
	assert.True(t, m.jobRunning(gitJobName))
}

// tab 을 오가는 것에는 git 이 붙지 않는다. `gt` 마다 프로세스가 뜨면 ADR-0009 의 출발점이 깨진다.
func TestSwitchingTabsDoesNotRefresh(t *testing.T) {
	m := newTestEditor("abc\n", 80, 10)
	m.buffers = append(m.buffers, newBuffer("other.txt", []byte("x\n")))

	model := send(m, "g", "t")

	require.IsType(t, viewEditorNormal{}, model)
	assert.False(t, model.(viewEditorNormal).jobRunning(gitJobName))
}

// 끊긴 갱신이 읽어온 것은 "저장소가 아니다" 와 구별되지 않는다.
// 그대로 넣으면 편집기를 끝내는 길에 표시가 사라진다.
func TestCancelledGitJobKeepsStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	e := &editor{ctx: ctx}

	cmd := e.refreshGit()
	require.NotNil(t, cmd)
	cancel()

	msg := cmd()
	progress, ok := msg.(jobProgressMsg)
	require.True(t, ok)

	assert.Nil(t, progress.apply, "값을 넣지 않는다")
	assert.Error(t, progress.err)
}
