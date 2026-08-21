package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bluemir/zn/internal/buildinfo"
)

// `:version` 은 아래 줄에 찍고 normal 로 돌아간다. 찍는 줄은 CLI 의 `--version` 과 같다.
func TestCommandVersion(t *testing.T) {
	old := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = old })
	buildinfo.Version = "v0.3.1"

	var m tea.Model = newTestEditor("abc\n", 60, 5)

	m = typeInto(m, ":version")
	m = send(m, "enter")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Contains(t, barOf(t, m)[1], buildinfo.Describe())
}

// 인자를 받지 않는다. 조용히 버리면 인자가 무언가를 한 것처럼 보인다.
func TestCommandVersionRejectsArgs(t *testing.T) {
	var m tea.Model = newTestEditor("abc\n", 60, 5)

	m = typeInto(m, ":version foo")
	m = send(m, "enter")

	assert.Contains(t, barOf(t, m)[1], "알 수 없는 명령")
}

// 줄 범위도 받지 않는다.
func TestCommandVersionRejectsRange(t *testing.T) {
	var m tea.Model = newTestEditor("abc\ndef\n", 60, 5)

	m = typeInto(m, ":1,2version")
	m = send(m, "enter")

	assert.Contains(t, barOf(t, m)[1], "줄 범위를 받지 않습니다")
}
