package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

func TestServerInstallConfirmRendersAsModalOverlay(t *testing.T) {
	editor := newTestEditor("hello world\nsecond line\n", 80, 20)
	model, cmd := serverInstallConfirmMode(editor, editor.editor, lsp.ServerFor("main.go"))
	require.Nil(t, cmd)
	require.IsType(t, viewServerInstallConfirm{}, model)

	confirm := model.(viewServerInstallConfirm)
	content := confirm.View().Content
	assert.Contains(t, content, "gopls 가 설치되어 있지 않습니다.")
	assert.Contains(t, content, "지금 설치하시겠습니까?")
	assert.Contains(t, content, "go install")
	assert.Contains(t, content, "Yes")
	assert.Contains(t, content, "No")
	// 부모 화면의 텍스트도 배경에 남아 있어야 한다.
	assert.Contains(t, content, "hello world")
}

func TestServerInstallConfirmKeyNavigation(t *testing.T) {
	editor := newTestEditor("test\n", 80, 20)
	model, _ := serverInstallConfirmMode(editor, editor.editor, lsp.ServerFor("main.go"))

	// 초기 커서는 0(Yes)
	confirm := model.(viewServerInstallConfirm)
	assert.Equal(t, 0, confirm.cursor)

	// right 또는 n 누르면 1(No) 로 이동
	next, _ := confirm.press("right")
	assert.Equal(t, 1, next.(viewServerInstallConfirm).cursor)

	// left 또는 y 누르면 0(Yes) 로 이동
	next, _ = next.(viewServerInstallConfirm).press("left")
	assert.Equal(t, 0, next.(viewServerInstallConfirm).cursor)

	// esc 누르면 parent 로 복귀
	back, _ := next.(viewServerInstallConfirm).press("esc")
	assert.Equal(t, editor, back)
}

func TestServerInstallConfirmHangulKeys(t *testing.T) {
	editor := newTestEditor("test\n", 80, 20)
	model, _ := serverInstallConfirmMode(editor, editor.editor, lsp.ServerFor("main.go"))

	// 'ㅜ'(n) 누르면 No 선택
	model, _ = model.Update(key("ㅜ"))
	require.IsType(t, viewServerInstallConfirm{}, model)
	assert.Equal(t, 1, model.(viewServerInstallConfirm).cursor)

	// 'ㅛ'(y) 누르면 Yes 선택
	model, _ = model.Update(key("ㅛ"))
	require.IsType(t, viewServerInstallConfirm{}, model)
	assert.Equal(t, 0, model.(viewServerInstallConfirm).cursor)
}

// python 서버를 물을 때는 그 이름과 그 설치 명령이 뜬다. 창이 표의 줄을 읽는 값이다(ADR-0107).
func TestServerInstallConfirmRendersPyright(t *testing.T) {
	editor := newTestEditor("def f():\n    pass\n", 80, 20)
	model, _ := serverInstallConfirmMode(editor, editor.editor, lsp.ServerFor("app.py"))

	content := model.(viewServerInstallConfirm).View().Content
	assert.Contains(t, content, "pyright 가 설치되어 있지 않습니다.")
	assert.Contains(t, content, "uv tool install")

	// 대괄호는 따옴표로 감싸 적는다. 옮겨 친 사람이 zsh 의 짝맞추기를 만나지 않게 한다.
	assert.Contains(t, content, "'pyright[nodejs]'")
	assert.NotContains(t, content, "gopls")
}
