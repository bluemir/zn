package lsp

import (
	"bytes"
	"strings"
)

// document 는 서버가 들고 있다고 우리가 믿는 파일 하나다.
//
// **lines 는 마지막으로 보낸 것의 사본이다.** 지금 편집 중인 것이 아니다. 이 둘의 차이가
// 다음에 보낼 것이라서, 사본을 들고 있는 것이 이 구조의 전부다.
//
// 증분을 이렇게 만드는 것이 요점이다(ADR-0051). 편집하는 자리마다 "무엇을 바꿨다" 고
// 적어 보내면 그 적는 자리가 하나라도 틀리면 서버의 사본이 조용히 어긋난 채 남는다.
// 여기서는 보낼 것이 편집 기록이 아니라 **두 사본의 차이에서 파생**되므로, 어긋날 자리가 없다.
type document struct {
	version int
	lines   [][]byte
}

// text 는 서버에 보내는 파일 전문이다. 줄을 `\n` 으로 잇고 끝에도 하나 붙인다.
//
// 줄끝이 CRLF 인 파일도 LF 로 보낸다. Buffer 의 줄 내용에는 `\r` 이 없으므로(buffer.go 의
// newBuffer) 이렇게 하면 우리가 세는 열과 서버가 세는 열이 같아진다.
//
// 파일이 줄끝으로 끝나지 않았어도 하나 붙는다. 없는 빈 줄이 하나 생기지만 이미 있는 줄들의
// 자리는 그대로라 정의를 찾는 데 달라지는 것이 없다.
func text(lines [][]byte) string {
	var out strings.Builder

	size := 0
	for _, line := range lines {
		size += len(line) + 1
	}
	out.Grow(size)

	for _, line := range lines {
		out.Write(line)
		out.WriteByte('\n')
	}

	return out.String()
}

// diff 는 사본에서 지금으로 가는 변경 하나다. 달라진 것이 없으면 false 다.
//
// 앞뒤로 같은 줄을 깎고 남은 구간을 통째로 갈아끼운다. 편집은 거의 언제나 한 자리에서
// 일어나므로 이 한 조각이면 실제로 바뀐 줄만 나간다.
//
// 범위의 열은 늘 0(줄 머리) 이다 — 마지막 줄까지 닿는 한 경우만 줄 끝을 써야 하고,
// 그래서 UTF-16 로 세는 자리가 이 함수에 하나뿐이다.
//
// 양쪽 모두 줄이 하나 이상이라고 본다. 편집기의 buffer 는 빈 파일도 빈 줄 하나라서
// 줄이 없는 상태가 없다(core/buffer.go 의 newEmptyBuffer).
func diff(old, now [][]byte) (contentChange, bool) {
	head := commonHead(old, now)
	if head == len(old) && head == len(now) {
		return contentChange{}, false // 같다
	}

	tail := commonTail(old, now, head)

	// 붙이기만 한 경우(old 가 now 의 앞부분) 시작 자리가 마지막 줄 다음이 된다. 그런 자리는
	// 없으므로 한 줄 물러서서 마지막 줄을 갈아끼우는 것으로 만든다.
	if head >= len(old) {
		head = len(old) - 1
	}

	oldEnd := len(old) - tail
	segment := now[head : len(now)-tail]

	if tail > 0 {
		// 갈아끼울 구간 뒤에 줄이 남아 있다. 줄 머리에서 줄 머리까지를 줄 단위로 바꾼다.
		return contentChange{
			Range: &Range{
				Start: Position{Line: head},
				End:   Position{Line: oldEnd},
			},
			Text: text(segment),
		}, true
	}

	// 파일 끝까지 바뀌었다. 끝 자리는 옛 마지막 줄의 끝이고, 넣는 글에는 끝 줄바꿈이 없다.
	last := len(old) - 1
	start := Position{Line: head}

	// 넣을 줄이 하나도 없으면 지우기만 하는 것이다. 그때는 앞 줄을 끝내는 줄바꿈까지 걷어야
	// 한다 — 줄 머리부터 지우면 그 줄바꿈이 남아서 빈 줄 하나가 파일 끝에 생긴다.
	if len(segment) == 0 && head > 0 {
		start = Position{Line: head - 1, Character: utf16Len(old[head-1])}
	}

	return contentChange{
		Range: &Range{
			Start: start,
			End:   Position{Line: last, Character: utf16Len(old[last])},
		},
		Text: strings.Join(linesToStrings(segment), "\n"),
	}, true
}

// commonHead 는 앞에서부터 같은 줄의 수다.
func commonHead(old, now [][]byte) int {
	n := min(len(old), len(now))
	for i := range n {
		if !bytes.Equal(old[i], now[i]) {
			return i
		}
	}

	return n
}

// commonTail 은 뒤에서부터 같은 줄의 수다. 앞에서 이미 센 만큼은 넘지 않는다 —
// 넘으면 같은 줄을 두 번 세어 범위가 뒤집힌다.
func commonTail(old, now [][]byte, head int) int {
	limit := min(len(old), len(now)) - head

	n := 0
	for n < limit && bytes.Equal(old[len(old)-1-n], now[len(now)-1-n]) {
		n++
	}

	return n
}

func linesToStrings(lines [][]byte) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = string(line)
	}

	return out
}

// copyLines 는 사본을 뜬다. Buffer 의 줄을 그대로 들고 있으면 다음 편집이 우리 사본까지
// 바꿔 버려서 차이가 사라진다.
//
// 줄 자체는 갈아끼우기만 하고 제자리에서 고치지 않으므로(edit.go 의 replaceLines) 겉껍데기만
// 새로 만들면 된다.
func copyLines(lines [][]byte) [][]byte {
	return append([][]byte(nil), lines...)
}
