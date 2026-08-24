package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// item 은 시험용 후보다. 서버가 늘 주는 모양(TextEdit 이 접두를 덮는 것) 을 흉내 낸다.
func item(label string, line, start, end int) lsp.CompletionItem {
	return lsp.CompletionItem{
		Label: label,
		TextEdit: &lsp.TextEdit{
			NewText: label,
			Range: lsp.Range{
				Start: lsp.Position{Line: line, Character: start},
				End:   lsp.Position{Line: line, Character: end},
			},
		},
	}
}

// 한글은 부르지 않는다. 조합 중에 답이 와서 다시 그리면 그 자리가 덮인다(ADR-0008).
func TestCompletionTriggers(t *testing.T) {
	for _, text := range []string{".", "a", "Z", "_", "0"} {
		assert.True(t, completionTriggers(text), "%q", text)
	}

	for _, text := range []string{"", " ", "(", "가", "ㄱ", "ab"} {
		assert.False(t, completionTriggers(text), "%q", text)
	}
}

// 서버가 준 범위가 이미 친 접두를 덮는다. 우리가 접두를 세지 않는다.
func TestInsertCompletionReplacesPrefix(t *testing.T) {
	buf := newBuffer("a.go", []byte("x := strings.Con\n"))
	buf.cursorLine, buf.cursorCol = 0, len("x := strings.Con")

	buf.insertCompletion(item("Contains", 0, 13, 16), wide)

	assert.Equal(t, "x := strings.Contains", string(buf.lines[0]))
	assert.Equal(t, len("x := strings.Contains"), buf.cursorCol, "커서가 넣은 글자 뒤에 선다")
}

// 범위가 없으면 커서 자리에 넣는다.
func TestInsertCompletionWithoutRange(t *testing.T) {
	buf := newBuffer("a.go", []byte("ab\n"))
	buf.cursorLine, buf.cursorCol = 0, 2

	buf.insertCompletion(lsp.CompletionItem{Label: "cd"}, wide)

	assert.Equal(t, "abcd", string(buf.lines[0]))
}

// 서버가 보던 판과 어긋나 범위가 줄 밖을 가리켜도 죽지 않는다.
func TestInsertCompletionClampsRange(t *testing.T) {
	buf := newBuffer("a.go", []byte("ab\n"))
	buf.cursorLine, buf.cursorCol = 0, 2

	buf.insertCompletion(item("Z", 0, 100, 200), wide)

	assert.Equal(t, "abZ", string(buf.lines[0]))
}

// 넣은 것은 치던 글자와 한 구간이다. `u` 한 번에 그 insert 가 통째로 돌아간다(vim 과 같다).
func TestInsertCompletionKeepsUndoChunkOpen(t *testing.T) {
	buf := newBuffer("a.go", []byte("\n"))
	buf.insert([]byte("st"), wide)

	buf.insertCompletion(item("strings", 0, 0, 2), wide)
	buf.insert([]byte("."), wide)

	require.True(t, buf.applyUndo(wide))
	assert.Equal(t, "", string(buf.lines[0]), "친 것과 넣은 것이 한 번에 돌아간다")
}

// 여러 줄에 걸친 범위는 따르지 않는다. snippet 은 켜지 않았다.
func TestInsertCompletionIgnoresMultilineRange(t *testing.T) {
	buf := newBuffer("a.go", []byte("ab\ncd\n"))
	buf.cursorLine, buf.cursorCol = 0, 1

	buf.insertCompletion(lsp.CompletionItem{
		Label: "Z",
		TextEdit: &lsp.TextEdit{NewText: "Z", Range: lsp.Range{
			Start: lsp.Position{Line: 0, Character: 0},
			End:   lsp.Position{Line: 1, Character: 1},
		}},
	}, wide)

	assert.Equal(t, "aZb", string(buf.lines[0]), "커서 자리에 넣는다")
	assert.Equal(t, "cd", string(buf.lines[1]), "아래 줄은 그대로다")
}

func TestMoveCompletionStopsAtEnds(t *testing.T) {
	e := &editor{completion: completion{items: []lsp.CompletionItem{item("a", 0, 0, 0), item("b", 0, 0, 0)}}}

	e.moveCompletion(-1)
	assert.Equal(t, 0, e.completion.selected, "위로는 첫 줄에서 멈춘다")

	e.moveCompletion(5)
	assert.Equal(t, 1, e.completion.selected, "아래로는 마지막에서 멈춘다")
}

// 창에 여덟 줄만 보이므로 고른 것을 따라 민다.
func TestMoveCompletionScrolls(t *testing.T) {
	items := make([]lsp.CompletionItem, 20)
	for i := range items {
		items[i] = item("x", 0, 0, 0)
	}

	e := &editor{completion: completion{items: items}}

	for range completionRows {
		e.moveCompletion(1)
	}

	assert.Equal(t, completionRows, e.completion.selected)
	assert.Equal(t, 1, e.completion.top, "한 줄만 밀린다")
}

// 답이 오는 사이에 글자를 더 쳤으면 그 답은 낡은 것이다.
func TestFinishCompletionDropsStaleAnswer(t *testing.T) {
	e := &editor{completionSeq: 7, completionAsking: true}

	cmd := e.finishCompletion(completionMsg{seq: 6, items: []lsp.CompletionItem{item("a", 0, 0, 0)}})

	assert.False(t, e.completionOpen(), "낡은 답으로 목록을 세우지 않는다")
	assert.False(t, e.completionAsking, "기다리는 표시는 내린다")
	assert.Nil(t, cmd, "닫혀 있었으면 다시 묻지도 않는다")
}

// 후보가 없거나 오류면 조용히 닫는다. 글자마다 지나는 자리라 알림을 세우지 않는다.
func TestFinishCompletionQuietWhenEmpty(t *testing.T) {
	e := &editor{completionSeq: 1}

	e.finishCompletion(completionMsg{seq: 1})

	assert.False(t, e.completionOpen())
	assert.Empty(t, e.notice)
}

// 목록이 떠 있는 동안에만 키가 목록의 것이 된다.
func TestCompletionKeysApplyAndClose(t *testing.T) {
	m := insertWithCompletion(t, []lsp.CompletionItem{item("Alpha", 0, 0, 0), item("Beta", 0, 0, 0)})

	next, _ := send2(m, "down")
	assert.Equal(t, 1, next.(viewEditorInsert).completion.selected)

	next, _ = send2(next, "enter")
	confirm := next.(viewEditorInsert)

	assert.Equal(t, "Beta", string(confirm.activeBuffer().lines[0]), "enter 는 넣기다")
	assert.False(t, confirm.completionOpen(), "넣고 나면 닫힌다")
	assert.Len(t, confirm.activeBuffer().lines, 1, "줄바꿈이 되지 않았다")
}

// 첫 esc 는 목록만 닫는다. insert 에서 나가려면 한 번 더다.
func TestCompletionEscClosesListFirst(t *testing.T) {
	m := insertWithCompletion(t, []lsp.CompletionItem{item("Alpha", 0, 0, 0)})

	next, _ := send2(m, "esc")

	require.IsType(t, viewEditorInsert{}, next)
	assert.False(t, next.(viewEditorInsert).completionOpen())

	next, _ = send2(next, "esc")
	assert.IsType(t, viewEditorNormal{}, next)
}

// 목록이 없으면 이 키들은 여느 때와 같다. insert 의 규칙이 그대로 남는다.
func TestCompletionKeysUntouchedWhenClosed(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "", 80, 6)

	m = send(m, "i", "enter")

	assert.Len(t, m.(viewEditorInsert).activeBuffer().lines, 2, "enter 는 줄바꿈이다")
}

// 부를 만한 글자가 아니면 닫는다.
func TestCompletionClosesOnOtherKeys(t *testing.T) {
	m := insertWithCompletion(t, []lsp.CompletionItem{item("Alpha", 0, 0, 0)})

	next, _ := send2(m, " ")

	assert.False(t, next.(viewEditorInsert).completionOpen())
}

// 커서 아래가 제자리고, 아래에 자리가 없으면 위로 올린다.
func TestCompletionBoxPos(t *testing.T) {
	left, top := completionBoxPos(10, 3, 100, 20, 6)
	assert.Equal(t, 10, left)
	assert.Equal(t, 4, top, "커서 바로 아래다")

	_, top = completionBoxPos(10, 15, 100, 20, 6)
	assert.Equal(t, 9, top, "아래에 자리가 없으면 커서 위다")

	left, _ = completionBoxPos(95, 3, 100, 20, 6)
	assert.Equal(t, 100-completionBoxWidth, left, "오른쪽 끝에서는 화면 안으로 민다")

	left, _ = completionBoxPos(0, 3, 10, 20, 6)
	assert.Equal(t, 0, left, "화면이 창보다 좁아도 음수로 가지 않는다")
}

// 이름이 잘리면 고를 수가 없다. 잘리는 쪽은 늘 곁들이는 타입이다.
func TestCompletionLabelKeepsName(t *testing.T) {
	long := lsp.CompletionItem{Label: "Contains", Detail: "func(s string, substr string) bool"}

	assert.Equal(t, "Contains", completionLabel(long, 10), "자리가 없으면 이름만")
	assert.Contains(t, completionLabel(long, 40), "Contains  func(s string")
}

// insertWithCompletion 은 목록이 떠 있는 insert 화면이다.
func insertWithCompletion(t *testing.T, items []lsp.CompletionItem) tea.Model {
	t.Helper()

	m := send(newTestEditorFile("a.go", "", 80, 6), "i")
	require.IsType(t, viewEditorInsert{}, m)

	m.(viewEditorInsert).editor.completion = completion{items: items}

	return m
}

// send2 는 키 하나를 보내고 model 과 cmd 를 같이 준다. send 는 model 만 준다(mode_test.go).
func send2(m tea.Model, name string) (tea.Model, tea.Cmd) {
	return m.Update(key(name))
}
