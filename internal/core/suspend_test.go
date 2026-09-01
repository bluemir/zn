package core

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ctrl+z 는 셸로 내려간다. 종료가 아니라 멈춤이라 저장하지 않은 변경을 묻지 않는다(ADR-0023).
func TestSuspendFromNormalMode(t *testing.T) {
	m, _ := newFileEditor(t, "abc\n")

	var model tea.Model = send(m, "i", "X", "esc")
	require.True(t, bufferOf(t, model).dirty)

	next, cmd := model.Update(key("ctrl+z"))

	require.NotNil(t, cmd, "ctrl+z 는 멈추라는 cmd 를 낸다")
	assert.IsType(t, tea.SuspendMsg{}, cmd())
	assert.IsType(t, viewEditorNormal{}, next, "mode 는 그대로다")
	assert.True(t, bufferOf(t, next).dirty, "변경은 그대로 남는다")
}

// 트리에 포커스가 있을 때도 내려간다. 올라오면 포커스가 트리에 그대로 있다.
func TestSuspendFromSidebar(t *testing.T) {
	var model tea.Model = newTreeEditor(t, 80, 6)
	model = send(model, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)

	next, cmd := model.Update(key("ctrl+z"))

	require.NotNil(t, cmd)
	assert.IsType(t, tea.SuspendMsg{}, cmd())
	assert.IsType(t, viewSidebar{}, next)
}

// visual 에서도 내려간다. 올라오면 고른 것이 그대로다 — 종료가 아니라 멈춤이다.
func TestSuspendFromVisualMode(t *testing.T) {
	var model tea.Model = send(newTestEditor("abc\ndef\n", 40, 5), "V", "j")
	require.IsType(t, viewEditorVisual{}, model)

	next, cmd := model.Update(key("ctrl+z"))

	require.NotNil(t, cmd, "ctrl+z 는 멈추라는 cmd 를 낸다")
	assert.IsType(t, tea.SuspendMsg{}, cmd())
	assert.IsType(t, viewEditorVisual{}, next, "mode 는 그대로다")
	assert.True(t, bufferOf(t, next).selection.active, "고른 것도 그대로다")
}

// insert mode 에서는 내려가지 않는다. 글자로 들어가지도 않는다.
func TestSuspendIgnoredInInsertMode(t *testing.T) {
	var model tea.Model = send(newTestEditor("abc\n", 40, 5), "i")
	require.IsType(t, viewEditorInsert{}, model)

	next, cmd := model.Update(key("ctrl+z"))

	assert.Nil(t, cmd)
	assert.Equal(t, "abc", string(bufferOf(t, next).lines[0]))
}

// `fg` 로 올라오면 보고 있는 파일이 밖에서 바뀌었는지 본다. 잃을 것이 없으면 가져온다(ADR-0038).
func TestResumeReloadsOutsideChange(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model := afterResume(t, m)

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, model).lines[0]))
	assert.Contains(t, barOf(t, model)[1], "다시 읽었습니다")
}

// 올라오는 자리에서는 git 을 읽지 않는다. 주기 갱신이 5 초 안에 따라온다(ADR-0030).
// 여기서 읽으면 셸에서 돌아오는 길에 프로세스 세 개가 동기로 붙는다.
func TestResumeDoesNotReadGitStatus(t *testing.T) {
	m, _ := newFileEditor(t, "abc\n")
	m.git = gitStatus{branch: "낡은-branch", commit: "0000000"}

	model, _ := m.Update(tea.ResumeMsg{})

	require.IsType(t, viewEditorNormal{}, model)
	assert.Equal(t, "낡은-branch(0000000)", model.(viewEditorNormal).git.label())
}

// 밖에서 지워진 파일은 가져올 것이 없어서 사실만 알린다.
func TestResumeReportsRemovedFile(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")
	require.NoError(t, os.Remove(path))

	model := afterResume(t, m)

	assert.Contains(t, barOf(t, model)[1], "사라졌습니다")
}

// 파일이 그대로면 조용하다. 아래 줄은 커서 위치 그대로다.
func TestResumeQuietWhenFileUnchanged(t *testing.T) {
	m, _ := newFileEditor(t, "abc\n")

	model, _ := m.Update(tea.ResumeMsg{})

	assert.NotContains(t, barOf(t, model)[1], "밖에서")
	assert.Contains(t, barOf(t, model)[1], "1:1")
}

// 이름 없는 buffer 는 맞춰 볼 파일이 없다.
func TestResumeQuietWithoutFileName(t *testing.T) {
	m := viewEditorNormal{
		editor: &editor{
			buffers: []viewport{newEmptyBuffer("")},
			width:   40,
			height:  5 + tablineHeight + statusBarHeight,
		},
	}

	model, _ := m.Update(tea.ResumeMsg{})

	assert.NotContains(t, barOf(t, model)[1], "밖에서")
}

// 트리에 포커스가 있으면 올라올 때 파일을 보지 않는다. 지금 보고 있는 것이 파일 내용이 아니다.
// newTreeEditor 의 buffer(`main.go`) 는 실제로 없는 파일이라 검사했다면 「사라졌습니다」가 뜬다.
func TestResumeInSidebarIsQuiet(t *testing.T) {
	var model tea.Model = newTreeEditor(t, 80, 6)
	model = send(model, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, model)

	model, _ = model.Update(tea.ResumeMsg{})

	require.IsType(t, viewSidebar{}, model)
	assert.NotContains(t, barOf(t, model)[1], "밖에서")
}

// afterResume 은 셸에서 올라온 것을 먹이고 검사 작업이 끝난 뒤까지 몬다(ADR-0044).
func afterResume(t *testing.T, m tea.Model) tea.Model {
	t.Helper()

	next, cmd := m.Update(tea.ResumeMsg{})
	require.NotNil(t, cmd, "검사 작업이 시작되지 않았다")

	return settle(t, next, cmd)
}
