package lsp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// 표로 적은 경우(document_test.go) 는 내가 떠올린 것만 본다. 어긋난 사본이 만드는 증상은
// 「정의가 엉뚱한 데로 뛴다」 라서 눈에 늦게 띄므로, 무작위 편집을 되풀이해 계약을 다시 잰다.
//
// 난수는 씨앗을 박은 것을 쓴다. 실패한 경우를 그대로 다시 돌릴 수 있어야 한다.
func TestDiffAppliesForRandomEdits(t *testing.T) {
	random := newLCG(20260822)

	lines := splitLines("package main\n\nfunc main() {\n\tprintln(\"가나다\")\n}")

	for round := range 2000 {
		next := randomEdit(random, lines)

		change, changed := diff(lines, next)
		if !changed {
			require.Equal(t, text(lines), text(next), "%d 번째: 같다고 했는데 다르다", round)

			lines = next

			continue
		}

		require.Equal(t, text(next), applyChange(lines, change),
			"%d 번째 편집에서 어긋났다\n옛것: %q\n지금: %q\n변경: %+v",
			round, text(lines), text(next), change)

		lines = next
	}
}

// randomEdit 은 줄 목록에 편집 하나를 얹은 새 목록이다. 제자리에서 고치지 않는다 —
// 편집기의 replaceLines 와 같은 손이다.
func randomEdit(random *lcg, lines [][]byte) [][]byte {
	next := copyLines(lines)

	switch random.next(4) {
	case 0: // 한 줄을 고친다
		at := random.next(len(next))
		next[at] = []byte(randomLine(random))
	case 1: // 줄을 끼운다
		at := random.next(len(next) + 1)
		next = append(next[:at:at], append([][]byte{[]byte(randomLine(random))}, next[at:]...)...)
	case 2: // 줄을 지운다. 마지막 한 줄은 남긴다 — 편집기의 buffer 도 줄이 없을 수 없다
		if len(next) <= 1 {
			return next
		}

		at := random.next(len(next))
		next = append(next[:at:at], next[at+1:]...)
	case 3: // 여러 줄을 한꺼번에 갈아치운다
		if len(next) <= 1 {
			return next
		}

		from := random.next(len(next))
		count := 1 + random.next(min(3, len(next)-from))
		with := [][]byte{}
		for range random.next(3) {
			with = append(with, []byte(randomLine(random)))
		}

		next = append(next[:from:from], append(with, next[from+count:]...)...)

		// 줄이 하나도 없는 buffer 는 편집기에 없다. 빈 파일도 빈 줄 하나다
		// (core/buffer.go 의 newEmptyBuffer). 그 규칙을 여기서도 지킨다.
		if len(next) == 0 {
			next = [][]byte{{}}
		}
	}

	return next
}

// randomLine 은 편집기에서 실제로 나오는 모양의 줄이다. 빈 줄과 한글과 이모지가 섞인다 —
// 마지막 줄의 끝 자리를 UTF-16 으로 세는 자리가 그것들에서 갈린다.
func randomLine(random *lcg) string {
	switch random.next(6) {
	case 0:
		return ""
	case 1:
		return "\tprintln(\"가나다\")"
	case 2:
		return "// 주석 🙂"
	case 3:
		return "}"
	case 4:
		return fmt.Sprintf("\tvalue%d := %d", random.next(100), random.next(1000))
	default:
		return "가나다라마바사"
	}
}

// lcg 는 씨앗을 박은 아주 작은 난수다. math/rand 를 쓰지 않는 것은 판마다 수열이 달라지면
// 실패한 경우를 다시 돌릴 수 없어서다.
type lcg struct{ state uint64 }

func newLCG(seed uint64) *lcg { return &lcg{state: seed} }

func (r *lcg) next(n int) int {
	if n <= 0 {
		return 0
	}

	r.state = r.state*6364136223846793005 + 1442695040888963407

	return int(r.state >> 33 % uint64(n))
}
