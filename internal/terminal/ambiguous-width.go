// Package terminal 은 터미널에 직접 묻는 것들이다.
//
// **bubbletea 를 지나지 않는 자리라 따로 있다.** 편집기의 나머지는 화면과 키를 전부
// bubbletea 를 통해 다루는데, 여기 있는 것은 stdin 을 잠깐 raw 로 바꾸고 escape sequence 를
// 손으로 쏘고 답을 읽는다. 그 갈래를 core 안에 두면 「터미널을 직접 만지는 자리가 어디인가」가
// 파일 이름에만 남는다(ADR-0028).
//
// 여기 있는 것은 **tea.NewProgram 보다 먼저** 불러야 한다. bubbletea 가 stdin 을 읽기
// 시작하면 터미널의 답을 그쪽이 가져간다.
package terminal

import (
	"bytes"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
)

// probeChar 는 폭을 재는 데 쓰는 글자다. 우리가 실제로 그릴 글자로 재야 의미가 있다.
const probeChar = "│"

// probeTimeout 은 커서 위치 답을 기다리는 시간이다. 터미널이 곧바로 답하는 것이라
// 짧아도 되고, 답하지 않는 터미널에서 시작이 눈에 띄게 늦어지면 안 된다.
const probeTimeout = 100 * time.Millisecond

// ProbeAmbiguousWidth 는 이 터미널이 Ambiguous 글자를 몇 칸으로 그리는지 잰다.
// 재지 못하면 0 이다 — tty 가 아니거나, 터미널이 답하지 않거나, 답이 알아볼 수 없을 때다.
//
// 글자를 하나 찍고 커서가 몇 번째 칸에 있는지 물어보는 것이 전부다(DSR 6 / CPR).
// bubbletea 의 RequestCursorPosition 으로는 못 한다. 그것은 bubbletea 가 자기 프레임에서
// 커서를 놓은 자리를 되묻는 것이라 "방금 찍은 글자 뒤" 를 잴 수 없다.
//
// 반드시 tea.NewProgram 보다 먼저 불러야 한다(패키지 주석).
func ProbeAmbiguousWidth() int {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return 0
	}

	// 답은 사용자가 친 것이 아니라 터미널이 보내는 것이라 echo 와 줄 단위 입력을 꺼야 읽힌다.
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return 0
	}
	defer term.Restore(os.Stdin.Fd(), state)

	// 답하지 않는 터미널에서 읽기를 끊을 수 있어야 한다. 그냥 goroutine 으로 읽어두면
	// 그 읽기가 살아남아 나중에 사용자가 친 키를 한 개 집어삼킨다.
	reader, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return 0
	}
	defer reader.Close()

	// 줄 맨 앞으로 가서 글자 하나를 찍고 커서 자리를 묻는다.
	if _, err := os.Stdout.WriteString("\r" + probeChar + "\x1b[6n"); err != nil {
		return 0
	}
	// 답이 오든 말든 찍은 글자는 지운다. 이것은 alt screen 에 들어가기 전, 곧 셸이 쓰던
	// 화면에 찍는 것이라 지우지 않으면 편집기를 끝낸 뒤 그 줄이 그대로 남는다.
	defer os.Stdout.WriteString("\r\x1b[K")

	timer := time.AfterFunc(probeTimeout, func() { reader.Cancel() })
	defer timer.Stop()

	// 답이 한 번에 다 오지 않을 수 있어서 끝 글자 `R` 이 나올 때까지 모은다.
	answer := []byte{}
	buf := make([]byte, 32)
	for !bytes.ContainsRune(answer, 'R') {
		n, err := reader.Read(buf)
		if err != nil {
			return 0
		}
		answer = append(answer, buf[:n]...)

		// 엉뚱한 것이 끝없이 들어오면 그만둔다. 커서 위치 답은 열 몇 byte 면 끝난다.
		if len(answer) > 64 {
			return 0
		}
	}

	col := columnOf(answer)
	if col < 1 {
		return 0
	}

	// 1 번 칸에서 글자 하나를 찍었으니 커서가 간 자리에서 1 을 빼면 그 글자의 폭이다.
	return col - 1
}

// columnOf 는 커서 위치 답(`ESC [ 행 ; 열 R`) 에서 열을 뽑는다. 못 읽으면 0 이다.
//
// 앞에 사용자가 미리 쳐둔 키가 섞여 있을 수 있어서 마지막 `ESC [` 부터 본다.
func columnOf(answer []byte) int {
	start := bytes.LastIndex(answer, []byte("\x1b["))
	if start < 0 {
		return 0
	}

	body := answer[start+2:]
	end := bytes.IndexByte(body, 'R')
	if end < 0 {
		return 0
	}

	// 어떤 터미널은 DECXCPR 형태로 `?` 를 앞에 붙여 답한다.
	body = bytes.TrimPrefix(body[:end], []byte("?"))

	semicolon := bytes.IndexByte(body, ';')
	if semicolon < 0 {
		return 0
	}

	col, err := strconv.Atoi(string(body[semicolon+1:]))
	if err != nil {
		return 0
	}

	return col
}
