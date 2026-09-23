package core

import (
	"bytes"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// 시스템 클립보드와 잇는 규칙이다. `"+` 를 이름으로 댄 것만 터미널을 거친다(ADR-0148).
//
// **터미널에게 부탁하는 것이지 OS 를 부르는 것이 아니다.** vim 의 `"+` 는 클립보드 API 를
// 직접 부르지만 여기서 쓸 수 있는 것은 OSC 52 뿐이라, 보낸 것이 닿았는지 앱이 알 길이 없고
// 읽기는 답이 나중에 온다. 그 둘이 아래 규칙의 대부분을 정한다.
//
// `"*` 는 열지 않았다. X11 밖에서는 `"+` 와 같은 것을 가리키는 이름이라 갈래가 없는 자리에
// 이름만 둘이 된다.

// clipboardRegister 는 시스템 클립보드를 가리키는 이름이다. vim 의 `"+` 와 같은 자리다.
const clipboardRegister = "+"

// clipboardSelectionSystem 은 OSC 52 가 시스템 클립보드를 가리키는 글자다. 나머지 하나는
// primary selection 인 `p` 이고, 그쪽은 묻지 않는다.
const clipboardSelectionSystem = 'c'

// clipboardWait 는 `"+p` 가 터미널의 답을 기다리는 시간이다.
//
// 터미널 왕복은 밀리초 아래이고 tmux 를 거쳐도 짧다. 이 값은 답이 오는 데 걸리는 시간이
// 아니라 **답하지 않는 터미널을 알아보는 데** 드는 시간이라, 사람이 「반응이 없다」고
// 느끼기 전에 끝나면 된다.
const clipboardWait = 500 * time.Millisecond

// clipboardPending 은 `"+p` 가 터미널의 답을 기다리는 상태다. seq 가 0 이면 기다리지 않는다.
//
// mode 안이 아니라 editor 에 사는 것은 답이 키와 무관하게 오기 때문이다. `"+p` 를 치고
// 바로 `i` 를 누른 손은 insert mode 에 있고, 그 사이에 도착한 답은 받을 자리가 없다.
type clipboardPending struct {
	// seq 는 물어본 차례다. 기다림을 끝낼 때마다 editor 쪽 값이 올라가서, 늦게 온 시간초과가
	// 이미 붙인 것을 두고 「답이 없다」고 말하지 않는다. completion 이 쓰는 손과 같다.
	seq int

	// before 는 `P` 로 물었는가다. `p` 면 false 다.
	before bool

	count int
}

// clipboardTimeoutMsg 는 기다리던 시간이 지났다는 알림이다. seq 가 그때 물어본 차례다.
type clipboardTimeoutMsg struct{ seq int }

// clipboardStore 는 `"+` 로 담은 것을 터미널에 보낸다. 다른 이름이면 아무 일도 하지 않는다.
//
// 담는 동작 여섯(`y` `d` `c` 와 visual 의 셋) 이 전부 이것을 부른다. 이름을 보는 자리가
// 하나여야 `"+dd` 만 조용히 빠지는 일이 없다(ADR-0148).
func clipboardStore(name string, block textarea.TextBlock) tea.Cmd {
	if name != clipboardRegister {
		return nil
	}

	return tea.SetClipboard(clipboardText(block))
}

// clipboardNote 는 알림 뒤에 붙는 말이다. `"+` 로 복사했을 때만 붙는다.
//
// **보냈다는 뜻이지 닿았다는 뜻이 아니다.** OSC 52 는 터미널이 받았는지 답하지 않아서 앱이
// 아는 것이 여기까지다. 그래서 「담았습니다」가 아니라 자리만 밝힌다(ADR-0148).
func clipboardNote(name string) string {
	if name != clipboardRegister {
		return ""
	}

	return " (클립보드)"
}

// clipboardText 는 클립보드로 나갈 글이다.
//
// **줄 단위면 끝에 개행을 붙인다.** OSC 52 에는 「줄 단위인가」를 실을 자리가 없어서 그것을
// 개행 하나로 나타낸다. 읽어 올 때 같은 것을 단서로 삼으므로(clipboardBlock) zn 끼리는
// 왕복이 맞고, 다른 앱으로 보낼 때도 줄을 통째로 복사한 것이 줄로 붙는다(ADR-0148).
func clipboardText(block textarea.TextBlock) string {
	text := string(bytes.Join(block.Lines, []byte("\n")))

	if block.Linewise {
		text += "\n"
	}

	return text
}

// clipboardBlock 은 터미널이 준 글을 붙일 수 있는 덩이로 바꾼다.
//
// **`\n` 으로 끝나면 줄 단위다.** 실려 온 갈래가 없으니 글 자체에서 읽는다. 줄을 통째로
// 긁어 온 것은 대개 끝에 개행이 있고 낱말 하나를 고른 것에는 없다(ADR-0148).
//
// **`\r\n` 은 `\n` 으로 고친다.** 브라우저나 윈도우 쪽에서 온 글이 그 모양이고, 그대로
// 두면 줄 끝마다 보이지 않는 글자가 하나씩 박힌다.
//
// 빈 글은 빈 덩이다. 붙여넣기가 조용히 아무 일도 하지 않는 자리로 그대로 간다(ADR-0017).
func clipboardBlock(text string) textarea.TextBlock {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return textarea.TextBlock{}
	}

	linewise := strings.HasSuffix(text, "\n")
	if linewise {
		text = strings.TrimSuffix(text, "\n")
	}

	parts := strings.Split(text, "\n")

	lines := make([][]byte, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, []byte(part))
	}

	return textarea.TextBlock{Lines: lines, Linewise: linewise}
}

// askClipboard 는 터미널에 클립보드를 묻고 기다림을 세운다. `"+p` 와 `"+P` 가 부른다.
//
// 기다리는 동안 키를 막지 않는다. 막으면 답하지 않는 터미널에서 편집기가 멈춘 것처럼
// 보이고, 그 상태를 푸는 길을 또 만들어야 한다(ADR-0148).
func (e *editor) askClipboard(before bool, count int) tea.Cmd {
	e.clipboardSeq++
	e.clipboard = clipboardPending{seq: e.clipboardSeq, before: before, count: count}

	seq := e.clipboardSeq

	return tea.Batch(
		tea.ReadClipboard,
		tea.Tick(clipboardWait, func(time.Time) tea.Msg { return clipboardTimeoutMsg{seq: seq} }),
	)
}

// pasteClipboard 는 터미널이 준 글을 붙인다. 기다리던 답이 아니면 버린다.
//
// **묻지 않았는데 온 답은 버린다.** OSC 52 응답은 우리 물음에 차례를 실어 주지 않아서
// 「기다리는 중인가」만 본다. 다른 까닭으로 온 것에 글이 끼어들면 손이 치지 않은 편집이 된다.
func (e *editor) pasteClipboard(text string) {
	if e.clipboard.seq == 0 {
		return
	}

	pending := e.clipboard
	e.clipboard = clipboardPending{}
	e.clipboardSeq++

	// 답이 오는 사이에 파일을 닫았거나 읽기 전용이 되었을 수 있다. 물을 때 본 것을 붙일
	// 때 다시 본다.
	if !e.hasTab() || e.refuseReadOnly() {
		return
	}

	block := clipboardBlock(text)
	if !block.Filled() {
		return
	}

	buf := e.activeBuffer()

	if pending.before {
		buf.PasteBefore(block, max(pending.count, 1))
	} else {
		buf.PasteAfter(block, max(pending.count, 1))
	}

	e.scrollToCursor()
}

// clipboardTimedOut 은 답이 오지 않은 것을 알린다.
//
// **여기만 소리를 낸다.** 빈 register 로 `p` 를 친 것이 조용한 것과 달리(ADR-0017) 이쪽은
// 손이 부탁한 것이 닿지도 않은 것이라, 아무 일도 안 일어난 까닭을 볼 길이 없다.
func (e *editor) clipboardTimedOut(seq int) {
	if e.clipboard.seq != seq {
		return
	}

	e.clipboard = clipboardPending{}
	e.notify("터미널이 클립보드를 주지 않습니다")
}
