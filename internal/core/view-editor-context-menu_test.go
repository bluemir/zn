package core

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// labelsOfContext 는 메뉴에 선 항목 이름과 비활성화 여부다.
func labelsOfContext(menu viewEditorContextMenu) []string {
	out := []string{}
	for _, item := range menu.items() {
		out = append(out, item.label)
	}

	return out
}

// contextItemPos 는 메뉴의 at 번째 항목을 누를 화면 좌표다.
func contextItemPos(menu viewEditorContextMenu, at int) (x, y int) {
	return menu.left + 2, menu.top + menuHeader + at
}

// 편집 영역의 단어를 우클릭하면 그 단어에 대한 컨텍스트 메뉴가 뜨고 커서가 그리로 이동한다.
func TestEditorContextMenuOpenOnRightClick(t *testing.T) {
	editor := newTestEditor("func helloWorld() {}\n", 80, 10)
	// tabline(0) 아래, 줄번호 오른쪽의 "helloWorld" 위치 (x=10, y=1)
	cellX := editor.contentLeft() + 7 // "helloWorld" 안쪽
	cellY := tablineHeight

	model, _ := editor.Update(rightClickAt(cellX, cellY))

	menu, ok := model.(viewEditorContextMenu)
	require.True(t, ok, "컨텍스트 메뉴가 뜬다")
	assert.Equal(t, "helloWorld", menu.target)
	assert.True(t, menu.hasWord)
	assert.Contains(t, menu.View().Content, "helloWorld")
}

// .go 파일에서는 LSP 항목(정의, 사용처, 이름 바꾸기)이 활성화된다.
func TestEditorContextMenuLSPEnabledForGo(t *testing.T) {
	editor := newTestEditor("package main\n\nfunc main() {}\n", 80, 10)
	editor.activeBuffer().Path = filepath.Join(t.TempDir(), "main.go")

	cellX := editor.contentLeft() + 6 // "main"
	cellY := tablineHeight + 2

	model, _ := editor.Update(rightClickAt(cellX, cellY))

	menu, ok := model.(viewEditorContextMenu)
	require.True(t, ok)
	assert.True(t, menu.hasLSP)
	assert.Equal(t, "main", menu.target)

	for _, item := range menu.items() {
		assert.False(t, item.disabled, "%s 항목이 활성화되어 있어야 한다", item.label)
	}
}

// .txt 파일처럼 LSP 가 없는 파일에서는 LSP 항목이 흐리게(disabled) 처리되고 검색과 붙여넣기는 활성화된다.
func TestEditorContextMenuLSPDisabledForTxt(t *testing.T) {
	editor := newTestEditor("some regular text here\n", 80, 10)
	editor.activeBuffer().Path = filepath.Join(t.TempDir(), "note.txt")

	cellX := editor.contentLeft() + 2 // "some"
	cellY := tablineHeight

	model, _ := editor.Update(rightClickAt(cellX, cellY))

	menu, ok := model.(viewEditorContextMenu)
	require.True(t, ok)
	assert.False(t, menu.hasLSP)
	assert.Equal(t, "some", menu.target)

	disabledMap := map[string]bool{}
	for _, item := range menu.items() {
		disabledMap[item.label] = item.disabled
	}

	assert.True(t, disabledMap["정의로 가기"], "LSP 미지원 파일에서 정의로 가기는 비활성화")
	assert.True(t, disabledMap["사용처로 가기"], "LSP 미지원 파일에서 사용처로 가기는 비활성화")
	assert.True(t, disabledMap["이름 바꾸기"], "LSP 미지원 파일에서 이름 바꾸기는 비활성화")
	assert.False(t, disabledMap["단어 검색"], "단어 검색은 활성화")
	assert.False(t, disabledMap["붙여넣기"], "붙여넣기는 활성화")
}

// 공백이나 빈 줄을 우클릭하면 대상이 "편집"이 되고 단어 관련 항목이 모두 비활성화된다.
func TestEditorContextMenuEmptyLine(t *testing.T) {
	editor := newTestEditor("\n\n\n", 80, 10)
	editor.activeBuffer().Path = filepath.Join(t.TempDir(), "empty.txt")

	cellX := editor.contentLeft() + 2
	cellY := tablineHeight + 1

	model, _ := editor.Update(rightClickAt(cellX, cellY))

	menu, ok := model.(viewEditorContextMenu)
	require.True(t, ok)
	assert.False(t, menu.hasWord)
	assert.Equal(t, "편집", menu.target)

	disabledMap := map[string]bool{}
	for _, item := range menu.items() {
		disabledMap[item.label] = item.disabled
	}

	assert.True(t, disabledMap["정의로 가기"])
	assert.True(t, disabledMap["사용처로 가기"])
	assert.True(t, disabledMap["이름 바꾸기"])
	assert.True(t, disabledMap["단어 검색"])
	assert.False(t, disabledMap["붙여넣기"])
}

// esc 나 ctrl+c, 바깥 클릭을 누르면 메뉴가 닫히고 부모 모드로 복귀한다.
func TestEditorContextMenuClose(t *testing.T) {
	editor := newTestEditor("hello world\n", 80, 10)
	cellX := editor.contentLeft() + 2
	cellY := tablineHeight

	// 1) esc 로 닫기
	model, _ := editor.Update(rightClickAt(cellX, cellY))
	_, isMenu := model.(viewEditorContextMenu)
	require.True(t, isMenu)

	closed, _ := model.Update(key("esc"))
	assert.IsType(t, viewEditorNormal{}, closed)

	// 2) 바깥 클릭으로 닫기
	model, _ = editor.Update(rightClickAt(cellX, cellY))
	menu := model.(viewEditorContextMenu)
	closed, _ = menu.Update(click(menu.left-2, menu.top-2))
	assert.IsType(t, viewEditorNormal{}, closed)
}

// 비활성화된 항목을 클릭하면 아무 동작도 하지 않고 메뉴에 머문다.
func TestEditorContextMenuClickDisabledDoesNothing(t *testing.T) {
	editor := newTestEditor("some regular text\n", 80, 10)
	editor.activeBuffer().Path = filepath.Join(t.TempDir(), "note.txt")

	cellX := editor.contentLeft() + 2
	cellY := tablineHeight

	model, _ := editor.Update(rightClickAt(cellX, cellY))
	menu := model.(viewEditorContextMenu)

	// 0번째 항목은 비활성화된 "정의로 가기"
	itemX, itemY := contextItemPos(menu, 0)
	next, cmd := menu.Update(click(itemX, itemY))

	assert.IsType(t, viewEditorContextMenu{}, next, "비활성화 항목 클릭 시 메뉴 유지")
	assert.Nil(t, cmd)
}

// 마우스가 활성 항목 위에 오면 hover 가 잡히고, 비활성 항목 위에 오면 잡히지 않는다.
func TestEditorContextMenuHover(t *testing.T) {
	editor := newTestEditor("some regular text\n", 80, 10)
	editor.activeBuffer().Path = filepath.Join(t.TempDir(), "note.txt")

	cellX := editor.contentLeft() + 2
	cellY := tablineHeight

	model, _ := editor.Update(rightClickAt(cellX, cellY))
	menu := model.(viewEditorContextMenu)

	// 비활성화된 0번째 항목 위로 마우스 이동
	itemX, itemY := contextItemPos(menu, 0)
	model, _ = menu.Update(tea.MouseMotionMsg{X: itemX, Y: itemY})
	assert.Equal(t, -1, model.(viewEditorContextMenu).hover, "비활성 항목은 hover 되지 않는다")

	// 활성화된 3번째 항목("단어 검색") 위로 마우스 이동
	itemX, itemY = contextItemPos(menu, 3)
	model, _ = menu.Update(tea.MouseMotionMsg{X: itemX, Y: itemY})
	assert.Equal(t, 3, model.(viewEditorContextMenu).hover, "활성 항목은 hover 된다")
}

// Visual 모드에서 우클릭하면 선택 영역 복사 메뉴가 뜨고 직전 선택 범위가 보존된다.
func TestEditorContextMenuVisualMode(t *testing.T) {
	editor := newTestEditor("first second third\n", 80, 10)
	// 'v' 로 visual 모드 진입 후 단어 선택
	vModel, _ := editor.Update(key("v"))
	vModel, _ = vModel.Update(key("w"))

	require.IsType(t, viewEditorVisual{}, vModel)

	// 우클릭
	cellX := editor.contentLeft() + 2
	cellY := tablineHeight
	model, _ := vModel.Update(rightClickAt(cellX, cellY))

	menu, ok := model.(viewEditorContextMenu)
	require.True(t, ok)
	assert.True(t, menu.isVisual)
	assert.Equal(t, "선택 영역", menu.target)
	assert.Equal(t, "선택 영역 복사", menu.items()[0].label)

	// esc 로 닫으면 visual 모드로 돌아온다
	closed, _ := menu.Update(key("esc"))
	assert.IsType(t, viewEditorVisual{}, closed)
}

// Insert 모드에서 우클릭하면 메뉴가 뜨고, esc 로 닫으면 insert 모드로 돌아온다.
func TestEditorContextMenuInsertMode(t *testing.T) {
	editor := newTestEditor("hello world\n", 80, 10)
	iModel, _ := editor.Update(key("i"))
	require.IsType(t, viewEditorInsert{}, iModel)

	cellX := editor.contentLeft() + 2
	cellY := tablineHeight
	model, _ := iModel.Update(rightClickAt(cellX, cellY))

	menu, ok := model.(viewEditorContextMenu)
	require.True(t, ok)

	closed, _ := menu.Update(key("esc"))
	assert.IsType(t, viewEditorInsert{}, closed)
}

// 단어 검색을 누르면 searchWord 가 실행되고 부모 모드로 돌아온다.
func TestEditorContextMenuSearchWord(t *testing.T) {
	editor := newTestEditor("word word word\n", 80, 10)
	cellX := editor.contentLeft() + 1
	cellY := tablineHeight

	model, _ := editor.Update(rightClickAt(cellX, cellY))
	menu := model.(viewEditorContextMenu)

	// 3번째 항목 "단어 검색" 클릭
	itemX, itemY := contextItemPos(menu, 3)
	closed, _ := menu.Update(click(itemX, itemY))

	assert.IsType(t, viewEditorNormal{}, closed)
	assert.True(t, editor.search.highlight)
	assert.True(t, strings.Contains(editor.search.input, "word"))
}
