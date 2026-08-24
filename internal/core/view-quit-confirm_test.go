package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 종료 확인창은 부모 화면 위에 뜨는 것이라 터미널 상태를 부모와 맞춰야 한다.
// AltScreen 이 꺼지면 창을 띄울 때마다 셸 화면이 번쩍이고 편집 내용이 사라진다.
func TestQuitConfirmKeepsParentAltScreen(t *testing.T) {
	editor := newTestEditor("a\nb\n", 40, 5)
	require.True(t, editor.View().AltScreen, "편집 화면은 대체 화면이다")

	confirm := ConfirmDiscard(editor, editor.editor, "정말 종료 하시겠습니까?", Exit)

	assert.True(t, confirm.View().AltScreen, "종료 확인창도 대체 화면을 유지한다")
}

func TestQuitConfirmEscapeReturnsToParent(t *testing.T) {
	editor := newTestEditor("a\nb\n", 40, 5)

	confirm := ConfirmDiscard(editor, editor.editor, "정말 종료 하시겠습니까?", Exit)
	back, _ := confirm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.IsType(t, viewEditorNormal{}, back, "Escape 는 편집 화면으로 돌아간다")
}

// 확인창은 부모 화면 위에 overlay 로 얹힌다. 부모의 내용과 확인창 상자가 함께 있어야 한다.
func TestConfirmDiscardRendersAsModalOverlay(t *testing.T) {
	editor := newTestEditor("hello world\nsecond line\n", 80, 20)
	confirm := ConfirmDiscard(editor, editor.editor, "정말 종료 하시겠습니까?", Exit)

	content := confirm.View().Content
	assert.Contains(t, content, "저장하지 않은 변경이 있습니다.")
	assert.Contains(t, content, "정말 종료 하시겠습니까?")
	assert.Contains(t, content, "Yes")
	assert.Contains(t, content, "No")
	// 부모 화면의 텍스트도 배경에 남아 있어야 한다.
	assert.Contains(t, content, "hello world")
}
