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

// insert mode 에서는 내려가지 않는다. 글자로 들어가지도 않는다.
func TestSuspendIgnoredInInsertMode(t *testing.T) {
	var model tea.Model = send(newTestEditor("abc\n", 40, 5), "i")
	require.IsType(t, viewEditorInsert{}, model)

	next, cmd := model.Update(key("ctrl+z"))

	assert.Nil(t, cmd)
	assert.Equal(t, "abc", string(bufferOf(t, next).lines[0]))
}

// `fg` 로 올라오면 보고 있는 파일이 밖에서 바뀌었는지 본다. 알리기만 하고 buffer 는 그대로다.
func TestResumeReportsOutsideChange(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	model, _ := m.Update(tea.ResumeMsg{})

	assert.Contains(t, barOf(t, model)[1], "바뀌었습니다")
	assert.Equal(t, "abc", string(bufferOf(t, model).lines[0]), "다시 읽지는 않는다")
}

// 올라올 때 git 표시도 다시 읽는다. 내려가 있는 동안 commit·checkout 을 하는 것이 흔하다.
func TestResumeRereadsGitStatus(t *testing.T) {
	m, _ := newFileEditor(t, "abc\n")
	m.git = gitStatus{branch: "낡은-branch", commit: "0000000"}

	model, _ := m.Update(tea.ResumeMsg{})

	require.IsType(t, viewEditorNormal{}, model)
	assert.Equal(t, readGitStatus(), model.(viewEditorNormal).git)
}

// 트리에 포커스가 있어도 git 표시는 다시 읽는다. statusBar 는 mode 와 무관하게 보인다.
func TestResumeRereadsGitStatusInSidebar(t *testing.T) {
	var model tea.Model = newTreeEditor(t, 80, 6)
	model = send(model, "ctrl+w", "ctrl+w")

	tree, ok := model.(viewSidebar)
	require.True(t, ok)
	tree.git = gitStatus{branch: "낡은-branch", commit: "0000000"}

	model, _ = tree.Update(tea.ResumeMsg{})

	require.IsType(t, viewSidebar{}, model)
	assert.Equal(t, readGitStatus(), model.(viewSidebar).git)
}

// 밖에서 지워진 파일은 가져올 것이 없어서 사실만 알린다.
func TestResumeReportsRemovedFile(t *testing.T) {
	m, path := newFileEditor(t, "abc\n")
	require.NoError(t, os.Remove(path))

	model, _ := m.Update(tea.ResumeMsg{})

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
			buffers: []Buffer{newEmptyBuffer("")},
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
