package core

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// sentClipboard 는 cmd 가 터미널로 보낸 글이다. 보내지 않았으면 두 번째 값이 false 다.
//
// bubbletea 가 내는 msg 가 package 안에만 있는 type 이라 밖에서 이름으로 가를 수 없다.
// `%v` 는 밑에 깔린 문자열을 그대로 보여주므로 그것으로 본다. 잴 수 있는 것을 재는 자리다.
func sentClipboard(t *testing.T, cmd tea.Cmd) (string, bool) {
	t.Helper()

	if cmd == nil {
		return "", false
	}

	msg := cmd()
	if msg == nil {
		return "", false
	}

	return fmt.Sprintf("%v", msg), true
}

// yankWith 는 키를 넣고 마지막 model 과 cmd 를 같이 준다. send 는 cmd 를 버린다.
func yankWith(m tea.Model, keys ...string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(key(k))
	}

	return m, cmd
}

// ── 나가는 글 ──

// 줄 단위면 끝에 개행이 붙는다. OSC 52 에 줄 단위인지를 실을 자리가 없어서 그것이 단서다.
func TestClipboardTextMarksLinewiseWithNewline(t *testing.T) {
	linewise := textarea.TextBlock{Lines: [][]byte{[]byte("foo"), []byte("bar")}, Linewise: true}
	charwise := textarea.TextBlock{Lines: [][]byte{[]byte("foo"), []byte("bar")}}

	assert.Equal(t, "foo\nbar\n", clipboardText(linewise))
	assert.Equal(t, "foo\nbar", clipboardText(charwise))
}

// 보낸 것을 그대로 읽으면 같은 덩이가 나온다. 내는 규칙과 읽는 규칙이 한 짝이어야 한다.
func TestClipboardRoundTrip(t *testing.T) {
	blocks := []textarea.TextBlock{
		{Lines: [][]byte{[]byte("foo")}, Linewise: true},
		{Lines: [][]byte{[]byte("foo"), []byte("bar")}, Linewise: true},
		{Lines: [][]byte{[]byte("foo")}},
		{Lines: [][]byte{[]byte("foo"), []byte("bar")}},
		{Lines: [][]byte{[]byte("")}, Linewise: true},
		{Lines: [][]byte{[]byte("한글 줄")}, Linewise: true},
	}

	for _, block := range blocks {
		assert.Equal(t, block, clipboardBlock(clipboardText(block)), "%q", clipboardText(block))
	}
}

// ── 들어오는 글 ──

// `\n` 으로 끝나면 줄 단위다. 밖에서 줄을 통째로 긁어 온 것이 그 모양이다.
func TestClipboardBlockReadsLinewiseFromTrailingNewline(t *testing.T) {
	assert.Equal(t,
		textarea.TextBlock{Lines: [][]byte{[]byte("foo")}, Linewise: true},
		clipboardBlock("foo\n"))

	assert.Equal(t,
		textarea.TextBlock{Lines: [][]byte{[]byte("foo")}},
		clipboardBlock("foo"))
}

// `\r\n` 은 `\n` 으로 고친다. 브라우저에서 온 글이 그 모양이라 그대로 두면 줄 끝마다
// 보이지 않는 글자가 박힌다.
func TestClipboardBlockDropsCarriageReturn(t *testing.T) {
	assert.Equal(t,
		textarea.TextBlock{Lines: [][]byte{[]byte("foo"), []byte("bar")}, Linewise: true},
		clipboardBlock("foo\r\nbar\r\n"))
}

// 빈 글은 빈 덩이다. 붙여넣기가 조용히 아무 일도 하지 않는 자리로 간다(ADR-0017).
func TestClipboardBlockEmpty(t *testing.T) {
	assert.False(t, clipboardBlock("").Filled())
}

// ── `"+` 로 담기 ──

// `"+yy` 는 클립보드로 보내고 알림에 그 자리를 밝힌다.
func TestClipboardYankSendsAndNotes(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	after, cmd := yankWith(m, `"`, "+", "y", "y")

	sent, ok := sentClipboard(t, cmd)
	require.True(t, ok, "클립보드로 보내는 cmd 가 나와야 한다")
	assert.Equal(t, "foo bar\n", sent)

	assert.Equal(t, "1 줄 복사되었습니다 (클립보드)", after.(viewEditorNormal).notice)
}

// **`"+` 는 우리 칸에 담기지 않는다.** 담기는 자리가 터미널이다. 무명에는 담겨서 곧바로
// 친 `p` 가 같은 것을 붙인다.
func TestClipboardYankKeepsUnnamedButNotPlus(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	after := send(m, `"`, "+", "y", "y")

	assert.Equal(t, map[string]string{`""`: "foo bar⏎"}, registersOf(t, after))
	assert.False(t, after.(viewEditorNormal).registers.byName(clipboardRegister).Filled())
}

// 지우는 것도 이름을 대면 나간다. `"+dd` 다.
func TestClipboardDeleteSends(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	_, cmd := yankWith(m, `"`, "+", "d", "d")

	sent, ok := sentClipboard(t, cmd)
	require.True(t, ok)
	assert.Equal(t, "foo bar\n", sent)
}

// visual 에서 고른 것도 같다.
func TestClipboardVisualYankSends(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	_, cmd := yankWith(m, "v", "l", "l", `"`, "+", "y")

	sent, ok := sentClipboard(t, cmd)
	require.True(t, ok)
	assert.Equal(t, "foo", sent)
}

// 이름을 대지 않은 `y` 는 나가지 않는다. 밖으로 보내는 것은 손이 대야 한다.
func TestClipboardPlainYankStaysInside(t *testing.T) {
	m := newTestEditor("foo bar\nbaz", 80, 20)

	after, cmd := yankWith(m, "y", "y")

	_, ok := sentClipboard(t, cmd)
	assert.False(t, ok, "이름을 대지 않은 `y` 는 터미널을 거치지 않는다")
	assert.Equal(t, "1 줄 복사되었습니다", after.(viewEditorNormal).notice)
}

// ── `"+p` 로 읽기 ──

// 답이 오면 그때 붙는다. `"+p` 는 그 자리에서 아무것도 바꾸지 않는다.
func TestClipboardPasteWaitsForAnswer(t *testing.T) {
	m := newTestEditor("foo\n", 80, 20)

	asked, _ := yankWith(m, `"`, "+", "p")
	assert.Equal(t, []string{"foo"}, linesOf(bufferOf(t, asked)), "답이 오기 전에는 그대로다")

	pasted, _ := asked.Update(tea.ClipboardMsg{Content: "bar\n", Selection: clipboardSelectionSystem})

	assert.Equal(t, []string{"foo", "bar"}, linesOf(bufferOf(t, pasted)))
}

// `"+P` 는 위에 붙인다. 기다림이 어느 쪽으로 물었는지를 들고 있다.
func TestClipboardPasteBefore(t *testing.T) {
	m := newTestEditor("foo\n", 80, 20)

	asked, _ := yankWith(m, `"`, "+", "P")
	pasted, _ := asked.Update(tea.ClipboardMsg{Content: "bar\n", Selection: clipboardSelectionSystem})

	assert.Equal(t, []string{"bar", "foo"}, linesOf(bufferOf(t, pasted)))
}

// 묻지 않았는데 온 답은 버린다. 손이 치지 않은 편집이 되면 안 된다.
func TestClipboardAnswerWithoutAskIsDropped(t *testing.T) {
	m := newTestEditor("foo\n", 80, 20)

	after, _ := m.Update(tea.ClipboardMsg{Content: "bar\n", Selection: clipboardSelectionSystem})

	assert.Equal(t, []string{"foo"}, linesOf(bufferOf(t, after)))
}

// primary selection 의 답도 버린다. `"*` 를 열지 않았다.
func TestClipboardPrimaryAnswerIsDropped(t *testing.T) {
	m := newTestEditor("foo\n", 80, 20)

	asked, _ := yankWith(m, `"`, "+", "p")
	after, _ := asked.Update(tea.ClipboardMsg{Content: "bar\n", Selection: 'p'})

	assert.Equal(t, []string{"foo"}, linesOf(bufferOf(t, after)))
}

// 답이 오지 않으면 알린다. 빈 register 의 `p` 와 달리 여기는 부탁이 닿지도 않은 것이다.
func TestClipboardTimeoutNotifies(t *testing.T) {
	e := &editor{}
	e.askClipboard(false, 0)

	e.clipboardTimedOut(e.clipboard.seq)

	assert.Equal(t, "터미널이 클립보드를 주지 않습니다", e.notice)
}

// 답이 온 뒤에 도착한 시간초과는 조용하다. 이미 붙인 것을 두고 「답이 없다」고 말하지 않는다.
func TestClipboardTimeoutAfterAnswerIsQuiet(t *testing.T) {
	m := newTestEditor("foo\n", 80, 20)

	asked, _ := yankWith(m, `"`, "+", "p")
	stale := asked.(viewEditorNormal).clipboard.seq

	pasted, _ := asked.Update(tea.ClipboardMsg{Content: "bar\n", Selection: clipboardSelectionSystem})
	after, _ := pasted.Update(clipboardTimeoutMsg{seq: stale})

	assert.Equal(t, []string{"foo", "bar"}, linesOf(bufferOf(t, after)))
	assert.Empty(t, after.(viewEditorNormal).notice)
}
