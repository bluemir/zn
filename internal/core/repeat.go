package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// lastChange 는 `.` 이 되풀이할 마지막 고치기다(ADR-0153).
//
// **동작 하나와 그 뒤 insert 에서 친 것들이다.** `ciw` 뒤에 `foo` 를 쳤으면 동작은 `ciw` 이고
// 친 것은 `f`·`o`·`o` 세 단계다. insert 로 들어가지 않는 동작(`dw`·`x`·`p`) 은 친 것이 비어 있다.
//
// editor 에 둔다. vim 과 같이 tab 을 넘어 되풀이한다 — 한 파일에서 고친 것을 옆 파일에 그대로
// 하는 것이 이 키를 가장 많이 쓰는 자리다.
type lastChange struct {
	act   action       // nil 이면 아직 고친 것이 없다
	typed []insertStep // insert 에서 친 것. 친 차례대로다

	// recording 은 insert 에서 친 것을 아직 담는 중인가다. 동작이 insert 로 들어가며 켜지고
	// `esc` 나 커서 이동이 끈다. 꺼진 뒤에 친 것은 담지 않는다 — undo 구간이 거기서 끊기는 것과
	// 같은 자리다(ADR-0153).
	recording bool
}

// insertStep 은 insert 에서 글을 고친 키 하나다.
//
// **키를 담되 자동완성은 결과를 담는다.** 되풀이할 때 같은 편집 함수를 다시 지나게 해서
// 들여쓰기가 새 자리의 규칙을 따른다. 자동완성은 다시 물으면 다른 답이 올 수 있어서, 그때
// 넣은 글과 갈아끼운 폭을 그대로 담는다.
type insertStep struct {
	// key 는 insert 의 편집 키 이름(`enter`·`backspace`·…) 이다. 글자를 친 것이면 비어 있고
	// 그 글자가 text 다. 키가 아닌 둘은 이름을 따로 둔다(stepPaste·stepCompletion).
	key  string
	text string

	// before·after 는 자동완성이 커서 앞뒤로 갈아끼운 byte 수다. 다른 단계에는 쓰지 않는다.
	before, after int
}

const (
	// stepPaste 는 터미널이 붙여넣은 것이다. 키가 아니라 글 덩이로 와서 들여쓰기를 손대지 않는다.
	stepPaste = "paste"
	// stepCompletion 은 자동완성 목록에서 고른 것이다.
	stepCompletion = "completion"
)

// repeatable 은 `.` 이 되풀이할 동작인가다. 파일을 고치는 normal 동작만 든다.
//
// `y` 는 고치지 않으므로 빠진다. vim 과 같다. visual 에서 한 동작도 빠진다 — 그쪽은 normal 의
// 동작이 아니라서 여기를 지나지 않는다(ADR-0153).
//
// 표 맞추기(`\mt`) 는 파일 전체를 맞추는 것이라 되풀이해도 바뀔 것이 없다.
func repeatable(act action) bool {
	switch act.(type) {
	case actionDelete, actionChange, actionIndent, actionReindent, actionReplaceChar,
		actionChangeCase, actionJoin, actionPasteAfter, actionPasteBefore,
		actionInsert, actionAppend, actionOpenBelow, actionOpenAbove:
		return true
	}

	return false
}

// withCount 는 숫자만 갈아 낀 동작이다. `2dw` 뒤의 `3.` 은 `3dw` 다. vim 과 같다.
//
// 숫자를 쓰지 않는 동작(`i`·`o`) 은 그대로다. `3i` 가 `i` 인 것과 같은 자리다.
func withCount(act action, count int) action {
	switch act := act.(type) {
	case actionDelete:
		act.count = count
		return act
	case actionChange:
		act.count = count
		return act
	case actionIndent:
		act.count = count
		return act
	case actionReindent:
		act.count = count
		return act
	case actionReplaceChar:
		act.count = count
		return act
	case actionChangeCase:
		act.count = count
		return act
	case actionJoin:
		act.count = count
		return act
	case actionPasteAfter:
		act.count = count
		return act
	case actionPasteBefore:
		act.count = count
		return act
	}

	return act
}

// actionRepeat 는 `.` 이다. 마지막 고치기를 지금 커서 자리에서 다시 한다.
//
// **한 번의 `u` 로 통째로 돌아간다.** 동작과 친 것을 같은 undo 구간 안에서 잇고 마지막에 닫는다.
// `c` 가 구간을 열어 둔 채 insert 로 넘기는 것을 그대로 쓴다(ADR-0033).
type actionRepeat struct{ count int }

func (c actionRepeat) run(e *editor) (tea.Model, tea.Cmd) {
	if e.repeat.act == nil {
		e.notify("되풀이할 것이 없습니다")

		return nil, nil
	}

	// 숫자를 댔으면 그 숫자가 다음 `.` 에도 남는다. vim 과 같다.
	if c.count > 0 {
		e.repeat.act = withCount(e.repeat.act, c.count)
	}

	next, cmd := e.repeat.act.run(e)

	// insert 로 들어가지 않는 동작은 그것으로 끝이다.
	if _, inserting := next.(viewEditorInsert); !inserting {
		return next, cmd
	}

	buf := e.activeBuffer()
	for _, step := range e.repeat.typed {
		replayInsertStep(buf, step)
	}

	// insert 의 `esc` 와 같다. 구간을 닫고 커서를 왼쪽 글자 위로 둔다.
	buf.EndEdit()
	buf.MoveLeft(1)
	e.scrollToCursor()

	model, _ := normalMode(e)

	return model, cmd
}

// startRepeat 는 동작 하나를 마지막 고치기로 적는다. insert 로 들어갔으면 친 것을 담기 시작한다.
func (e *editor) startRepeat(act action, next tea.Model) {
	_, inserting := next.(viewEditorInsert)

	e.repeat = lastChange{act: act, recording: inserting}
}

// recordInsert 는 insert 에서 친 것 하나를 담는다. 담는 중이 아니면 버린다.
func (e *editor) recordInsert(step insertStep) {
	if !e.repeat.recording {
		return
	}

	e.repeat.typed = append(e.repeat.typed, step)
}

// stopRecording 은 친 것을 그만 담는다. 이미 담은 것은 남는다.
func (e *editor) stopRecording() {
	e.repeat.recording = false
}

// applyInsertKey 는 insert 에서 글을 고치는 키 하나를 buffer 에 건다.
//
// **손으로 친 것과 `.` 이 되풀이하는 것이 이 하나를 지난다.** 두 벌이면 되풀이한 들여쓰기가
// 친 것과 갈린다.
func applyInsertKey(buf *textarea.Viewport, key, text string) {
	switch key {
	case "enter":
		// 낱말로 블록을 닫는 언어(shell 의 `fi`) 는 줄을 떠나는 이 순간에 낱말이 끝난다.
		buf.ReindentClosing([]byte{'\n'})
		buf.InsertNewLine()
	case "backspace":
		// 커서 앞이 들여쓰기뿐이면 한 칸이 아니라 앞 단위 경계까지 지운다(indent.go).
		if !buf.DeleteIndentBackward() {
			buf.DeleteBackward()
		}
	case "delete":
		// 커서 자리 글자를 지운다. 들여쓰기 단위로 묶지 않는다 — 뒤쪽 공백은
		// 들여쓰기가 아니라 줄 끝 공백이고, 그것을 한 번에 지우는 것은 별개 결정이다.
		buf.DeleteForward()
	case "tab":
		buf.InsertIndent()
	case "shift+tab":
		buf.OutdentLine()
	default:
		// 줄 앞이 닫는 표시가 되면 그 줄이 한 단계 당겨진다. 넣기 전에 자리를 잡아야
		// 커서가 옮겨진 자리에서 글자를 받는다(indent.go).
		buf.ReindentClosing([]byte(text))
		buf.Insert([]byte(text))
	}
}

// replayInsertStep 은 담아 둔 단계 하나를 지금 커서 자리에 다시 건다.
func replayInsertStep(buf *textarea.Viewport, step insertStep) {
	switch step.key {
	case stepPaste:
		buf.Insert([]byte(step.text))
	case stepCompletion:
		// 범위는 커서 기준으로 담았다. 줄 밖을 가리키면 InsertCompletion 이 줄 안으로 가둔다.
		col := buf.Cursor.Col
		buf.InsertCompletion(col-step.before, col+step.after, []byte(step.text))
	default:
		applyInsertKey(buf, step.key, step.text)
	}
}
