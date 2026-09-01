package core

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// 줄을 화면에서 재고, 화면에 그릴 글자로 바꾸는 자리다.
//
// `Buffer` 의 메서드가 아니라 `[]byte` 한 줄을 받는 함수들이다. 커서를 옮기는 쪽
// (buffer-move.go·buffer-screen.go) 과 그리는 쪽(render-row.go·tabline.go), 그리고 파일이
// 아닌 글을 다루는 쪽(input-line.go·palette-match.go) 이 같이 쓴다.
//
// 하는 일이 셋이다.
//
//   - **좌표를 옮긴다.** screenColAt(offset→칸)·offsetAtScreenCol(칸→offset)·
//     rowIndexAt(offset→행)·rowRange(행→byte 범위)
//   - **경계를 찾는다.** glyphSize(다음 글자)·prevGlyphStart(이전 글자)·wrapOffsets(행 경계)
//   - **화면에 그릴 글자로 바꾼다.** screenText 가 tab 을 빈 칸으로, 제어문자를 `^[` 로
//     바꾼다(ADR-0118)
//
// **셋이 한 뿌리라 갈리지 않는다.** 전부 glyphAt 위에 서는데, 그것이 답하는 「이 자리 글자는
// 몇 byte 이고 몇 칸인가」가 곧 「화면에 어떻게 그리나」다. tab 을 몇 칸으로 펴고 제어문자를
// 몇 칸으로 볼지가 그 안에 들어 있다. **재는 일과 그리는 일이 한 덩어리여서 이것을 따로
// 패키지로 뺄 수 없다** (ADR-0119).
//
// 재는 단위가 rune 이 아니라 grapheme cluster 인 것이 이 파일의 요점이다 — 까닭은 glyphAt 에 있다.
// 「논리 줄」과 「화면 행」이 무엇인지는 buffer-move.go 머리글에 있다.
//
// **줄 시작만이 알려진 경계다.** grapheme cluster 는 뒤에서 앞으로 읽을 수 없고
// (prevGlyphStart), tab 이 미는 폭은 시작 칸을 알아야 정해진다. 그래서 여기 있는 것은 전부
// 줄 시작에서 출발해 앞으로만 간다. **그것이 함수들이 `line` 과 `offset` 을 같이 받는
// 까닭이다** — offset 은 그냥 위치가 아니라 줄 시작에서 걸어온 거리이고, 그 줄을 빼면 뜻이
// 없는 값이다.
//
// **tab 폭은 부르는 쪽이 늘 넘긴다.** 그 폭이 파일마다 다르기 때문이다(`buf.tabWidth()`,
// ADR-0096). 안 받는 것은 glyphSize(byte 길이는 폭과 무관하다) 와 rowIndexAt(끊어 놓은
// offset 만 본다) 둘뿐이고, 그 둘은 답이 폭에 닿지 않아서다.

// glyphAt 은 offset 에서 시작하는 grapheme cluster 의 byte 길이와 화면 폭을 돌려준다.
// col 은 그 글자가 시작하는 화면 칸이다. tab 이 시작 위치에 따라 폭이 달라서 필요하다.
// tab 은 tab 하나가 미는 칸 수이고 1 보다 작으면 안 된다 — 나머지 연산이 여기 있다.
//
// 커서 이동과 줄바꿈은 rune 이 아니라 이 단위로 해야 한다.
// NFD 한글(ᄒ+ᅡ+ᆫ), 결합 악센트(e+́), 이모지 조합(가족 이모지, 국기) 은 rune 여러 개가 한 글자다.
// 폭과 경계를 같은 함수에서 얻어야 "한 글자" 와 "그 폭" 이 어긋나지 않는다.
// lipgloss.Width 도 결국 이 경로를 쓴다.
func glyphAt(line []byte, offset, col, tab int) (size, width int) {
	// ansi 는 tab 을 폭 0 으로 본다. 그대로 두면 들여쓰기가 화면에서 사라진다.
	if line[offset] == '\t' {
		return 1, tab - col%tab
	}

	// 제어문자는 `^[` 두 글자로 보인다. 폭을 여기서 정하고 그 글자는 controlText 가 준다
	// (ADR-0118).
	if controlText(line[offset]) != "" {
		return 1, 2
	}

	cluster, w := ansi.FirstGraphemeCluster(line[offset:], ansi.GraphemeWidth)

	// 깨진 UTF-8 에서 0 이 나오면 진행하지 못하고 무한 반복한다.
	if len(cluster) < 1 {
		return 1, 1
	}
	return len(cluster), w
}

// controlText 는 제어문자를 화면에 보이는 두 글자로 바꾼다. 제어문자가 아니면 빈 글이다.
//
// **편집기는 터미널 흉내가 아니다.** 파일에 든 `\x1b` 는 색을 켜라는 명령이 아니라 고칠 수
// 있어야 하는 내용이라, vim 처럼 `^[` 로 보이게 그린다. 그대로 흘려보내면 터미널이 그것을
// 먹어서 화면에서 사라지고 커서 자리가 어긋난다 (ADR-0118).
//
// tab 은 여기 들지 않는다. 그것은 칸으로 펴는 쪽이다(glyphAt). 줄바꿈은 줄 안에 없다.
func controlText(b byte) string {
	switch {
	case b == '\t' || b == '\n':
		return ""
	case b < 0x20:
		// `^` 다음 글자는 제어문자에 0x40 을 더한 것이다. ESC(0x1b) 가 `^[`(0x5b) 다.
		return "^" + string(rune(b+0x40))
	case b == 0x7f:
		return "^?"
	}

	return ""
}

// screenText 는 offset 자리 글자를 화면에 그릴 때 대신 쓸 글자다. 그대로 그려도 되는
// 글자면 빈 글이다.
//
// **폭은 glyphAt 이 주는 것과 같다.** 둘이 갈리면 센 것과 그린 것이 어긋나 커서와 오른쪽
// 끝이 밀린다. 그래서 tab 의 빈 칸도 여기서 glyphAt 에게 물어 만든다 (ADR-0118).
//
// 그리는 자리가 둘이라(편집 영역의 expandRow, grep 목록) 규칙이 흩어지면 한쪽이 빠진다.
// 실제로 제어문자가 grep 쪽에서 빠져 있었다.
func screenText(line []byte, offset, col, tab int) string {
	if line[offset] == '\t' {
		_, width := glyphAt(line, offset, col, tab)

		return strings.Repeat(" ", width)
	}

	return controlText(line[offset])
}

// glyphSize 는 폭이 필요 없을 때 쓴다. byte 길이는 시작 칸과 무관하다.
//
// 그래서 tab 폭을 받지 않는다 — tab 은 어느 폭으로 그리든 1 byte 다. 넘기는 1 은 나머지
// 연산이 0 으로 나누지 않게 하는 자리 채움이고 답에 닿지 않는다.
func glyphSize(line []byte, offset int) int {
	size, _ := glyphAt(line, offset, 0, 1)
	return size
}

// screenColAt 은 offset 까지의 화면 칸 수다.
// 한글은 두 칸, tab 은 다음 tab stop 까지라서 byte offset 과 다르다.
func screenColAt(line []byte, offset, tab int) int {
	col := 0
	for i := 0; i < offset && i < len(line); {
		size, w := glyphAt(line, i, col, tab)
		col += w
		i += size
	}
	return col
}

// offsetAtScreenCol 은 화면 칸 col 에 해당하는 byte offset 을 찾는다.
// col 이 여러 칸을 쓰는 글자의 중간이면 그 글자의 시작으로 맞춘다.
func offsetAtScreenCol(line []byte, col, tab int) int {
	width := 0
	for offset := 0; offset < len(line); {
		size, w := glyphAt(line, offset, width, tab)
		if width+w > col {
			return offset
		}
		width += w
		offset += size
	}
	return len(line)
}

// prevGlyphStart 는 offset 직전 글자의 시작을 돌려준다. from 은 글자 경계여야 한다.
//
// grapheme cluster 는 뒤에서 앞으로 읽을 수 없어서 알려진 경계에서부터 훑는다.
// 행 시작이 항상 경계이므로 훑는 범위는 화면 한 행으로 묶인다.
func prevGlyphStart(line []byte, from, offset int) int {
	prev := from
	for i := from; i < offset && i < len(line); {
		prev = i
		i += glyphSize(line, i)
	}
	return prev
}

// wrapOffsets 는 줄이 width 칸에서 끊기는 지점, 즉 각 화면 행의 시작 byte offset 을 돌려준다.
// 가로 스크롤 대신 줄바꿈으로 보여주므로 화면보다 긴 줄은 화면 행 여러 개가 된다.
// 빈 줄도 행 하나를 차지하므로 항상 최소 하나를 돌려준다.
func wrapOffsets(line []byte, width, tab int) []int {
	if width < 1 {
		return []int{0}
	}

	offsets := []int{0}
	col := 0
	for offset := 0; offset < len(line); {
		// col 은 행이 바뀔 때 0 으로 돌아간다. 화면 행이 왼쪽 끝에서 시작하므로 tab stop 도 거기부터다.
		size, w := glyphAt(line, offset, col, tab)

		// col > 0 조건이 없으면 width 보다 넓은 글자에서 같은 지점을 계속 끊는다.
		if col > 0 && col+w > width {
			offsets = append(offsets, offset)
			col = 0
		}

		col += w
		offset += size
	}

	return offsets
}

// rowIndexAt 은 offset 이 속한 화면 행의 index 를 돌려준다.
func rowIndexAt(offsets []int, offset int) int {
	for i := len(offsets) - 1; i > 0; i-- {
		if offset >= offsets[i] {
			return i
		}
	}
	return 0
}

// rowRange 는 화면 행 하나가 담는 byte 범위를 돌려준다.
func rowRange(line []byte, offsets []int, row int) (int, int) {
	start := offsets[row]
	if row+1 < len(offsets) {
		return start, offsets[row+1]
	}
	return start, len(line)
}
