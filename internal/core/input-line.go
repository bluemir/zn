package core

import (
	"github.com/bluemir/zn/internal/textarea"
)

// 한 줄 입력의 글과 그 안의 커서다. `:`·`/`·이름 받기 셋·grep 셋이 이 하나를 나눠 쓴다.
//
// **처음에는 여덟 곳이 저마다 `input string` 을 들고 뒤에만 붙였다.** 커서가 없으니
// `←`·`→`·`home`·`end`·`delete` 가 그 자리에서 뜻을 가질 수 없었고, 오타 하나를 고치려면
// 뒤를 다 지워야 했다(ADR-0076 §5).
//
// 커서를 넣으면 넣기·지우기·그리기가 전부 커서를 봐야 한다. 그것을 여덟 벌 두는 대신
// 여기 한 벌만 둔다 — embed 가 아니라 필드다. `m.text` 는 무엇의 글인지 읽히지 않는데
// `m.input.text` 는 읽힌다.

// inputLine 은 치고 있는 글과 그 안의 커서 자리다.
type inputLine struct {
	text string

	// cursor 는 text 안의 byte offset 이다. **글자 경계에 있다** — grapheme cluster
	// 단위로만 옮기므로 한글이나 이모지 가운데에 서지 않는다(cluster.go).
	cursor int
}

// newInputLine 은 무언가 적힌 채로 여는 입력줄이다. 커서는 그 끝이다.
//
// 채워진 채로 여는 곳이 셋이다 — visual 의 `:`(`'<,'>`), 이름 바꾸기 둘(옛 이름).
func newInputLine(text string) inputLine {
	return inputLine{text: text, cursor: len(text)}
}

func (in inputLine) empty() bool {
	return in.text == ""
}

// head 는 커서 앞의 글, tail 은 커서 뒤의 글이다.
//
// 커서를 두고 갈리는 자리라 이름을 붙였다. 넣기·지우기·접기가 다 이 둘로 말한다.
func (in inputLine) head() string {
	return in.text[:in.cursor]
}

func (in inputLine) tail() string {
	return in.text[in.cursor:]
}

// insert 는 커서 자리에 글자를 넣고 커서를 그 뒤로 옮긴다.
func (in *inputLine) insert(text string) {
	in.text = in.head() + text + in.tail()
	in.cursor += len(text)
}

// replaceHead 는 커서 앞을 통째로 갈아끼운다. 커서는 갈아끼운 글 끝이다.
//
// 명령줄의 `tab` 자동완성이 쓴다. **줄 전체가 아니라 커서 앞만 채운다** — 커서 뒤에 친 것을
// 완성 대상으로 넘기면 `:e docs/a▮.md` 에서 `.md` 까지 경로 조각으로 읽힌다. 셸과 같은 손이다.
func (in *inputLine) replaceHead(text string) {
	in.text = text + in.tail()
	in.cursor = len(text)
}

// deleteBackward 는 커서 앞 한 글자를 지운다. `backspace` 다.
//
// **줄이 비었는지는 부르는 쪽이 먼저 본다.** 여덟 곳이 다 「다 지우면 이 mode 에서 나간다」를
// 들고 있는데 나가는 자리가 저마다 다르다. 커서가 맨 앞인데 뒤에 글이 남은 것은 그것과 다른
// 일이라 여기서 조용히 넘긴다.
func (in *inputLine) deleteBackward() {
	if in.cursor == 0 {
		return
	}

	start := textarea.PrevGlyphStart([]byte(in.text), 0, in.cursor)
	in.text = in.text[:start] + in.tail()
	in.cursor = start
}

// deleteForward 는 커서 뒤 한 글자를 지운다. `delete` 다. 커서는 제자리다.
func (in *inputLine) deleteForward() {
	if in.cursor >= len(in.text) {
		return
	}

	in.text = in.head() + in.text[in.cursor+textarea.GlyphSize([]byte(in.text), in.cursor):]
}

// move 는 커서를 옮긴다. 글은 바뀌지 않으므로 부르는 쪽이 거르기나 미리보기를 다시 돌릴
// 것이 없다.
//
// **키 이름을 그대로 받는다.** 부르는 여덟 곳이 전부 `msg.String()` 으로 갈리는 switch 안이라
// 거기서 네 case 를 한 줄로 묶을 수 있다. 넷을 따로 두면 같은 열여섯 줄이 여덟 벌 생긴다.
//
// `home`·`end` 는 화면 행이 아니라 이 줄의 양끝이다. 입력줄은 한 행이라 둘이 같다(ADR-0076 §1).
func (in *inputLine) move(key string) {
	switch key {
	case "left":
		in.cursor = textarea.PrevGlyphStart([]byte(in.text), 0, in.cursor)
	case "right":
		if in.cursor < len(in.text) {
			in.cursor += textarea.GlyphSize([]byte(in.text), in.cursor)
		}
	case "home":
		in.cursor = 0
	case "end":
		in.cursor = len(in.text)
	}
}

// screenCursor 는 커서 앞 글이 차지하는 화면 칸 수다. 커서가 설 칸이 곧 이것이다.
//
// 접히지 않는 자리(아래 줄에 그리는 넷) 가 쓴다. 접히는 자리는 visible 이 답한다.
func (in inputLine) screenCursor() int {
	return textarea.WidthOf(in.head())
}

// visible 은 width 칸 안에 커서가 들어오도록 접은 글과 그 안에서 커서가 설 칸이다.
// prefix 는 입력 앞에 늘 붙어 그려지는 글이다. 같이 접힌다.
//
// **커서 앞을 먼저 접는다.** 그렇게 해야 커서가 접혀 사라진 쪽으로 넘어가지 않는다 — 늘
// 오른쪽 끝을 보이던 때는 커서도 늘 오른쪽 끝이라 이것을 볼 일이 없었다.
//
// 창 안에 그리는 둘(이름 바꾸기 창·grep 검색창) 이 쓴다. **커서 한 칸은 부르는 쪽이 뺀
// width 로 남긴다** — 안 남기면 마지막 글자를 친 순간 커서가 테두리 위에 선다.
func (in inputLine) visible(prefix string, width int) (text string, cursorCol int) {
	head := trimLeftToWidth(prefix+in.head(), width)

	return truncateToWidth(head+in.tail(), width), textarea.WidthOf(head)
}
