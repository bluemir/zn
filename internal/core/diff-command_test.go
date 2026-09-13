package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// newDiffEditor 는 진짜 파일 하나를 연 편집 화면이다. `:diff` 는 경로를 정말로 본다.
func newDiffEditor(t *testing.T) (*editor, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("a\nb\n"), 0644))

	buf, err := textarea.OpenBuffer(path)
	require.NoError(t, err)

	return &editor{buffers: []textarea.Viewport{buf}, width: 60, height: 12}, dir
}

// 인자가 없으면 HEAD 와 지금 buffer 다.
func TestDiffRequestDefaultsToHead(t *testing.T) {
	e, dir := newDiffEditor(t)

	request, err := diffRequestFor(e, nil)
	require.NoError(t, err)

	assert.Equal(t, diffTargetCommit, request.left.kind)
	assert.Equal(t, "HEAD", request.left.rev)
	assert.Equal(t, diffTargetBuffer, request.right.kind)
	assert.Equal(t, filepath.Join(dir, "a.txt"), request.right.path)
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, request.right.lines,
		"저장하지 않은 것까지 지금 buffer 에서 떠 간다")
}

// 인자가 하나면 파일인지 커밋인지는 읽는 자리가 푼다.
func TestDiffRequestLeavesOneArgUnresolved(t *testing.T) {
	e, _ := newDiffEditor(t)

	request, err := diffRequestFor(e, []string{"master"})
	require.NoError(t, err)

	assert.Equal(t, diffTargetAuto, request.left.kind)
	assert.Equal(t, "master", request.left.arg)
	assert.Equal(t, diffTargetBuffer, request.right.kind)
}

// 인자가 둘이면 둘 다 경로다. 커밋과 섞지 않는다(ADR-0140 §3).
func TestDiffRequestTakesTwoPaths(t *testing.T) {
	e, _ := newDiffEditor(t)

	request, err := diffRequestFor(e, []string{"x.txt", "y.txt"})
	require.NoError(t, err)

	assert.Equal(t, diffTargetFile, request.left.kind)
	assert.Equal(t, "x.txt", request.left.path)
	assert.Equal(t, diffTargetFile, request.right.kind)
	assert.Equal(t, "y.txt", request.right.path)
}

// 셋 이상은 받지 않는다.
func TestDiffRequestRefusesThreeArgs(t *testing.T) {
	e, _ := newDiffEditor(t)

	_, err := diffRequestFor(e, []string{"a", "b", "c"})
	assert.Error(t, err)
}

// 볼 파일이 없으면 견줄 오른쪽이 없다.
func TestDiffRequestNeedsBuffer(t *testing.T) {
	_, err := diffRequestFor(&editor{width: 60, height: 12}, nil)
	assert.Error(t, err)
}

// 명령줄이 `:diff` 의 인자를 그대로 넘긴다.
//
// **관문이 셋이다.** 인자를 받는 명령인지, 인자가 둘까지인지, `~` 를 어디서 푸는지다. 셋 다
// 이름 목록이라 새 명령은 그 목록에 들어야 하는데, 실제로 하나도 못 들어가 있었다.
func TestDiffCommandPassesArgs(t *testing.T) {
	for _, line := range []string{"diff", "diff HEAD~1", "diff a.txt", "diff a.txt b.txt"} {
		cmd, err := parseCommand(line, "cur.go")

		require.NoError(t, err, line)
		assert.Equal(t, "diff", cmd.name, line)
	}
}

// 명령줄의 관문 셋이 `:diff` 를 막지 않는다.
//
// 막히면 알림줄에 그 까닭이 적히고 판이 열리지 않는다. 셋 다 이름 목록이라 새 명령은 그
// 목록에 들어야 하는데, 실제로 하나도 못 들어가 있었다.
func TestDiffCommandPassesGates(t *testing.T) {
	for _, line := range []string{"diff", "diff HEAD~1", "diff a.txt", "diff a.txt b.txt"} {
		e, dir := newDiffEditor(t)
		t.Chdir(dir)

		m := viewEditorCommand{editor: e}
		m.input.text = line

		m.run()

		assert.NotContains(t, e.notice, "알 수 없는 명령", line)
		assert.NotContains(t, e.notice, "파일은 하나만", line)
		assert.NotContains(t, e.notice, "줄 범위를 받지 않습니다", line)
	}
}

// 인자의 맨 앞 `~` 는 명령줄이 편다. 둘 다 편다 — `:diff` 가 둘을 받는 첫 명령이다.
//
// **가운데 `~` 는 건드리지 않는다.** `:diff HEAD~1` 이 그 자리다.
func TestDiffCommandExpandsHomeInBothArgs(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	e, dir := newDiffEditor(t)
	t.Chdir(dir)

	m := viewEditorCommand{editor: e}
	m.input.text = "diff ~/왼쪽 ~/오른쪽"

	// 판이 든 request 가 명령줄을 지나온 인자다. 읽기는 Cmd 안이라 아직 돌지 않았다.
	model, _ := m.run()

	panel, ok := model.(viewDiff)
	require.True(t, ok, "판이 열린다")

	assert.Equal(t, filepath.Join(home, "왼쪽"), panel.request.left.path)
	assert.Equal(t, filepath.Join(home, "오른쪽"), panel.request.right.path)
}

// 가운데 `~` 는 건드리지 않는다. `:diff HEAD~1` 이 그 자리다.
func TestDiffCommandKeepsRevisionTilde(t *testing.T) {
	e, dir := newDiffEditor(t)
	t.Chdir(dir)

	m := viewEditorCommand{editor: e}
	m.input.text = "diff HEAD~1"

	model, _ := m.run()

	panel, ok := model.(viewDiff)
	require.True(t, ok, "판이 열린다")

	assert.Equal(t, "HEAD~1", panel.request.left.arg)
}
