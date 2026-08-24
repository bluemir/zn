package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/lsp"
)

// insert 에서 뜨는 자동완성이다(ADR-0066).
//
// 하는 일은 넷이다 — 언제 물을지 고르기(completionTriggers), 묻기(startCompletion),
// 낡은 답 버리기(finishCompletion), 고른 것을 넣기(applyCompletion). 그리는 것은
// render-completion.go 에 있다.

// completionRows 는 목록 창에 한 번에 보이는 줄 수다.
//
// 여덟은 편집 화면을 절반 넘게 가리지 않으면서 후보를 훑기에 넉넉한 수다. 후보는 이보다
// 훨씬 많이 온다(잰 값으로 `rand.` 뒤 스물여덟) — 나머지는 밀어서 본다.
const completionRows = 8

// completion 은 떠 있는 목록이다. items 가 비어 있으면 닫힌 것이다.
type completion struct {
	items    []lsp.CompletionItem
	selected int
	top      int // 창의 첫 줄에 오는 항목. 팔레트의 밀기와 같다(view-palette.go)

	// line 은 물어본 줄이다. 커서가 그 줄을 떠나면 목록을 닫는다 — 후보는 그 자리에서만
	// 뜻이 있고, TextEdit 의 범위도 그 줄을 가리키고 있다.
	line int
}

// completionMsg 는 물어본 답이다.
//
// seq 를 싣고 온다. 답이 오는 사이에 글자가 더 들어왔으면 그 답은 낡은 것이다 —
// 이 편집기에서 「보내고 나중에 받는」 길 중 되받는 값이 화면에 남는 첫 자리라 이 표시가 있다
// (`\gd` 는 답이 오면 뛰고 끝나서 필요가 없었다, ADR-0051).
type completionMsg struct {
	seq   int
	items []lsp.CompletionItem
	err   error
}

// completionOpen 은 목록이 떠 있는지다.
func (e editor) completionOpen() bool {
	return len(e.completion.items) > 0
}

// closeCompletion 은 목록을 닫는다. 도는 요청이 있어도 그 답은 seq 로 버려진다.
//
// 기다리는 표시도 같이 내린다. 답은 insert 만 받는데 그 답이 오기 전에 mode 가 바뀌면
// (esc·팔레트) 아무도 받지 않아서, 내리지 않으면 그 뒤로 영영 묻지 못한다. 잃는 것은
// 지나간 요청 하나뿐이다 — 그 답은 어차피 seq 가 버린다.
func (e *editor) closeCompletion() {
	e.completion = completion{}
	e.completionSeq++
	e.completionAsking = false
}

// completionTriggers 는 방금 친 글자가 목록을 부를 만한 것인지다.
//
// **ASCII 만 부른다.** Go 의 식별자가 ASCII 이기도 하고, 한글을 치는 동안에는 요청이 아예
// 나가지 않아야 해서다 — 조합 중인 글자는 buffer 에 없고 터미널이 화면에만 그려 주므로,
// 답이 와서 다시 그리면 그 자리가 덮인다(ADR-0008, ADR-0061 §3).
func completionTriggers(text string) bool {
	if len(text) != 1 {
		return false
	}

	c := text[0]

	return c == '.' || c == '_' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// startCompletion 은 커서 자리에서 이어 칠 수 있는 것을 묻는다. 물을 자리가 아니면 nil 이다.
//
// **한 번에 하나만 묻는다.** 글자마다 묻는 자리라 그냥 두면 요청이 겹치고, 겹치면 전문 맞추기
// (SyncFull) 도 겹쳐서 서버가 마지막에 받는 것이 지금 화면이라는 보장이 없어진다.
// 도는 동안 더 친 글자는 답이 온 뒤 finishCompletion 이 한 번 더 걸어서 따라잡는다.
//
// gopls 를 여기서 띄우지 않는다. Go 파일을 열 때 이미 뜨고(ADR-0051), 글자마다 지나는
// 자리에서 남의 프로세스를 띄우는 일이 일어나서는 안 된다.
func (e *editor) startCompletion() tea.Cmd {
	if !e.hasTab() || e.completionAsking || e.gopls == nil {
		return nil
	}

	buf := e.activeBuffer()

	path, ok := goplsPath(buf.path)
	if !ok || !e.gopls.Tracks(path) {
		return nil
	}

	client := e.gopls
	lines := buf.lines
	position := lsp.Position{
		Line:      buf.cursorLine,
		Character: lsp.UTF16Column(buf.lines[buf.cursorLine], buf.cursorCol),
	}

	e.completionSeq++
	e.completionAsking = true
	seq := e.completionSeq

	return func() tea.Msg {
		// 묻기 직전에 전문으로 맞춘다. 250ms 예약(scheduleLspTick) 을 기다리면 방금 친 글자가
		// 아직 서버에 없어서 한 글자 뒤처진 후보가 온다(ADR-0051 의 정의 찾기와 같은 자리다).
		if err := client.SyncFull(path, lines); err != nil {
			return completionMsg{seq: seq, err: err}
		}

		items, err := client.Completion(path, position)

		return completionMsg{seq: seq, items: items, err: err}
	}
}

// finishCompletion 은 답을 받아 목록을 세운다.
//
// **낡은 답은 버린다.** 그 사이에 글자를 더 쳤으면 seq 가 어긋나고, 그때는 새로 물어야 한다 —
// 버리고 마는 것이 아니라 여기서 한 번 더 거는 것이 「도는 동안 친 글자」를 따라잡는 길이다.
//
// 오류는 조용히 닫는다. 글자마다 지나는 자리라 알림을 세우면 치는 동안 아래 줄이 번쩍인다 —
// 서버가 아직 훑는 중이거나 그 자리에 답이 없는 것이 흔하고, 둘 다 사람이 할 일이 없다.
func (e *editor) finishCompletion(msg completionMsg) tea.Cmd {
	e.completionAsking = false

	if msg.seq != e.completionSeq {
		if e.completionOpen() {
			return e.startCompletion()
		}

		return nil
	}

	if msg.err != nil || len(msg.items) == 0 {
		e.completion = completion{}

		return nil
	}

	e.completion = completion{items: msg.items, line: e.activeBuffer().cursorLine}

	return nil
}

// moveCompletion 은 고른 자리를 옮긴다. 위아래 화살표가 부른다.
//
// 끝에서 멈춘다. 감아 도는 것이 편할 자리도 있지만, 여기서는 손가락이 화살표에 올라간 채
// 목록 끝을 지나쳐 반대편으로 튀는 쪽이 더 놀랍다.
func (e *editor) moveCompletion(delta int) {
	c := &e.completion

	c.selected = min(max(c.selected+delta, 0), len(c.items)-1)

	// 창 안으로 끌어온다. 팔레트의 밀기와 같은 셈이다(view-palette.go).
	if c.selected < c.top {
		c.top = c.selected
	}
	if c.selected >= c.top+completionRows {
		c.top = c.selected - completionRows + 1
	}
}

// applyCompletion 은 고른 후보를 넣고 목록을 닫는다.
func (e *editor) applyCompletion() {
	if !e.completionOpen() {
		return
	}

	item := e.completion.items[e.completion.selected]
	e.activeBuffer().insertCompletion(item, e.contentWidth())
	e.closeCompletion()
	e.scrollToCursor()
}

// insertCompletion 은 후보 하나를 커서 자리에 넣는다.
//
// **서버가 준 범위(TextEdit) 를 그대로 쓴다.** 이미 친 접두를 그 범위가 덮고 있어서
// (`rand.IntN` 에서 `IntN` 넉 자였다) 우리가 접두를 셀 일이 없다. 범위가 없거나 여러 줄에
// 걸치면 커서 자리에 넣는다 — 여러 줄짜리는 snippet 쪽 이야기이고 우리는 그것을 켜지 않았다
// (lsp/client.go 의 initialize 가 능력을 비워 둔다).
//
// **되돌리기 구간을 닫지 않는다.** insert 에서 친 글자와 한 구간에 있어야 `u` 한 번으로
// 그 insert 가 통째로 돌아간다. vim 과 같다.
func (buf *Buffer) insertCompletion(item lsp.CompletionItem, width int) {
	line := buf.cursorLine
	start, end := buf.cursorCol, buf.cursorCol

	if edit := item.TextEdit; edit != nil &&
		edit.Range.Start.Line == line && edit.Range.End.Line == line {
		start = lsp.ByteColumn(buf.lines[line], edit.Range.Start.Character)
		end = lsp.ByteColumn(buf.lines[line], edit.Range.End.Character)
	}

	// 서버가 보던 판과 지금 판이 어긋났으면 범위가 줄 밖을 가리킬 수 있다.
	start = min(max(start, 0), len(buf.lines[line]))
	end = min(max(end, start), len(buf.lines[line]))

	text := []byte(item.Text())

	next := make([]byte, 0, len(buf.lines[line])-(end-start)+len(text))
	next = append(next, buf.lines[line][:start]...)
	next = append(next, text...)
	next = append(next, buf.lines[line][end:]...)

	buf.beginEdit(line, 1)
	buf.replaceLines(line, 1, [][]byte{next})

	buf.cursorCol = start + len(text)
	buf.updateDesiredCol(width)
}
