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

// **겹쳐 그리는 화면은 터미널 설정을 하나도 잃지 않는다**(ADR-0110).
//
// AltScreen 만 재던 때에는 `ReportFocus` 와 키 확장이 꺼지는 것을 못 잡았다. 새 view 를
// 만들고 두 칸만 베끼고 있었는데 `newView` 가 켜 두는 것은 넷이었다. 칸 이름을 하나씩
// 세지 않고 **부모 것과 통째로 견준다** — 다음에 칸이 늘어도 이 시험이 먼저 걸린다.
func TestOverlaysKeepEveryTerminalSetting(t *testing.T) {
	settings := func(v tea.View) [4]any {
		return [4]any{v.AltScreen, v.MouseMode, v.ReportFocus, v.KeyboardEnhancements}
	}

	m := newTestEditor("hello\nworld\n", 80, 12)
	want := settings(m.View())

	back, _ := normalMode(m.editor)

	overlays := map[string]tea.Model{
		"종료 확인창":   ConfirmDiscard(back, m.editor, "물음", func() (tea.Model, tea.Cmd) { return nil, nil }),
		"이름 바꾸기 창": must(renameInputMode(m.editor)),
		"팔레트":      must(paletteMode(m.editor)),
	}

	for name, overlay := range overlays {
		assert.Equal(t, want, settings(overlay.View()), "%s 이 설정을 잃었다", name)
	}
}

// must 는 mode 를 여는 함수의 model 만 집는다. 여는 Cmd 는 이 시험이 볼 것이 아니다.
func must(model tea.Model, _ tea.Cmd) tea.Model {
	return model
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
