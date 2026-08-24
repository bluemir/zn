package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoplsInstallConfirmRendersAsModalOverlay(t *testing.T) {
	editor := newTestEditor("hello world\nsecond line\n", 80, 20)
	model, cmd := goplsInstallConfirmMode(editor, editor.editor)
	require.Nil(t, cmd)
	require.IsType(t, viewGoplsInstallConfirm{}, model)

	confirm := model.(viewGoplsInstallConfirm)
	content := confirm.View().Content
	assert.Contains(t, content, "gopls 가 설치되어 있지 않습니다.")
	assert.Contains(t, content, "지금 설치하시겠습니까?")
	assert.Contains(t, content, "Yes")
	assert.Contains(t, content, "No")
	// 부모 화면의 텍스트도 배경에 남아 있어야 한다.
	assert.Contains(t, content, "hello world")
}

func TestGoplsInstallConfirmKeyNavigation(t *testing.T) {
	editor := newTestEditor("test\n", 80, 20)
	model, _ := goplsInstallConfirmMode(editor, editor.editor)

	// 초기 커서는 0(Yes)
	confirm := model.(viewGoplsInstallConfirm)
	assert.Equal(t, 0, confirm.cursor)

	// right 또는 n 누르면 1(No) 로 이동
	next, _ := confirm.press("right")
	assert.Equal(t, 1, next.(viewGoplsInstallConfirm).cursor)

	// left 또는 y 누르면 0(Yes) 로 이동
	next, _ = next.(viewGoplsInstallConfirm).press("left")
	assert.Equal(t, 0, next.(viewGoplsInstallConfirm).cursor)

	// esc 누르면 parent 로 복귀
	back, _ := next.(viewGoplsInstallConfirm).press("esc")
	assert.Equal(t, editor, back)
}

func TestGoplsInstallConfirmHangulKeys(t *testing.T) {
	editor := newTestEditor("test\n", 80, 20)
	model, _ := goplsInstallConfirmMode(editor, editor.editor)

	// 'ㅜ'(n) 누르면 No 선택
	model, _ = model.Update(key("ㅜ"))
	require.IsType(t, viewGoplsInstallConfirm{}, model)
	assert.Equal(t, 1, model.(viewGoplsInstallConfirm).cursor)

	// 'ㅛ'(y) 누르면 Yes 선택
	model, _ = model.Update(key("ㅛ"))
	require.IsType(t, viewGoplsInstallConfirm{}, model)
	assert.Equal(t, 0, model.(viewGoplsInstallConfirm).cursor)
}
