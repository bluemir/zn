package core

import "github.com/charmbracelet/x/ansi"

// 줄을 화면에서 재는 자리다. Buffer 의 메서드가 아니라 `[]byte` 한 줄을 받는 함수들이고,
// 커서를 옮기는 쪽(buffer-move.go·buffer-screen.go) 과 그리는 쪽(render-row.go·tabline.go) 이
// 같이 쓴다. 그래서 buffer- 접두를 붙이지 않았다.
//
// 재는 단위가 rune 이 아니라 grapheme cluster 인 것이 이 파일의 요점이다 — 까닭은 clusterAt 에 있다.
//
// 줄 하나를 화면 행으로 끊는 자리(wrapOffsets·rowIndexAt·rowRange) 도 여기다.
// 「논리 줄」과 「화면 행」이 무엇인지는 buffer-move.go 머리글에 있다.
//
// **tab 폭을 마지막 인자로 받는 함수와 받지 않는 함수가 갈린다.** 받는 쪽은 파일 내용을 재는
// 자리이고 그 폭은 파일마다 다르다(`buf.tabWidth()`, ADR-0096). 받지 않는 쪽은 UI 문자열을
// 재는 자리다 — tabline 의 제목, statusBar, 팔레트 후보에는 tab 이 들어올 일이 없다.

// screenRow 는 화면 한 행에 그려지는 논리 줄의 일부다.
type screenRow struct {
	line  int // lines 의 index
	start int // 줄 안의 byte offset. 포함
	end   int // 줄 안의 byte offset. 제외
}

// defaultTabWidth 는 `.editorconfig` 가 아무 말도 하지 않을 때 tab 하나가 미는 칸 수다.
// 터미널 기본 tab stop 은 8 이지만, 8 칸은 깊게 들여쓴 코드를 화면 밖으로 밀어낸다.
//
// UI 문자열을 재는 함수들이 이것을 쓴다. 파일 내용은 `buf.tabWidth()` 가 준다(ADR-0096).
const defaultTabWidth = 4

// clusterAt 은 offset 에서 시작하는 grapheme cluster 의 byte 길이와 화면 폭을 돌려준다.
// col 은 그 글자가 시작하는 화면 칸이다. tab 이 시작 위치에 따라 폭이 달라서 필요하다.
// tab 은 tab 하나가 미는 칸 수이고 1 보다 작으면 안 된다 — 나머지 연산이 여기 있다.
//
// 커서 이동과 줄바꿈은 rune 이 아니라 이 단위로 해야 한다.
// NFD 한글(ᄒ+ᅡ+ᆫ), 결합 악센트(e+́), 이모지 조합(가족 이모지, 국기) 은 rune 여러 개가 한 글자다.
// 폭과 경계를 같은 함수에서 얻어야 "한 글자" 와 "그 폭" 이 어긋나지 않는다.
// lipgloss.Width 도 결국 이 경로를 쓴다.
func clusterAt(line []byte, offset, col, tab int) (size, width int) {
	// ansi 는 tab 을 폭 0 으로 본다. 그대로 두면 들여쓰기가 화면에서 사라진다.
	if line[offset] == '\t' {
		return 1, tab - col%tab
	}

	cluster, w := ansi.FirstGraphemeCluster(line[offset:], ansi.GraphemeWidth)

	// 깨진 UTF-8 에서 0 이 나오면 진행하지 못하고 무한 반복한다.
	if len(cluster) < 1 {
		return 1, 1
	}
	return len(cluster), w
}

// clusterSize 는 폭이 필요 없을 때 쓴다. byte 길이는 시작 칸과 무관하다.
//
// 그래서 tab 폭을 받지 않는다 — tab 은 어느 폭으로 그리든 1 byte 다. 넘기는 값은 나머지
// 연산이 0 으로 나누지 않게 하는 자리 채움이고 답에 닿지 않는다.
func clusterSize(line []byte, offset int) int {
	size, _ := clusterAt(line, offset, 0, defaultTabWidth)
	return size
}

// screenColAt 은 offset 까지의 화면 칸 수다.
// 한글은 두 칸, tab 은 다음 tab stop 까지라서 byte offset 과 다르다.
func screenColAt(line []byte, offset, tab int) int {
	col := 0
	for i := 0; i < offset && i < len(line); {
		size, w := clusterAt(line, i, col, tab)
		col += w
		i += size
	}
	return col
}

// screenWidthOf 는 그 글자가 통째로 차지하는 화면 칸 수다.
//
// **UI 문자열 전용이다.** 파일 내용을 재려면 `screenColAt(line, len(line), buf.tabWidth())` 를
// 쓴다 — 이 함수를 쓰면 그 파일의 tab 폭이 아니라 기본값으로 잰다(ADR-0096).
func screenWidthOf(text string) int {
	return screenColAt([]byte(text), len(text), defaultTabWidth)
}

// offsetAtScreenCol 은 화면 칸 col 에 해당하는 byte offset 을 찾는다.
// col 이 여러 칸을 쓰는 글자의 중간이면 그 글자의 시작으로 맞춘다.
func offsetAtScreenCol(line []byte, col, tab int) int {
	width := 0
	for offset := 0; offset < len(line); {
		size, w := clusterAt(line, offset, width, tab)
		if width+w > col {
			return offset
		}
		width += w
		offset += size
	}
	return len(line)
}

// escapeSizeAt 은 offset 에서 시작하는 ANSI escape 의 byte 길이다. escape 가 아니면 0 이다.
//
// **다 지은 화면 줄에만 쓴다.** 색을 입힌 글자는 escape 를 달고 오는데 그것은 화면에서 자리를
// 차지하지 않으므로, 재거나 자를 때 지나쳐야 한다(ADR-0061).
//
// buffer 안의 글자에는 쓰지 않는다 — 파일에 든 ESC byte 는 색이 아니라 내용이다.
func escapeSizeAt(line []byte, offset int) int {
	if line[offset] != 0x1b || offset+1 >= len(line) || line[offset+1] != '[' {
		return 0
	}

	// CSI 는 `@`~`~` 사이 글자에서 끝난다. lipgloss 가 내는 것은 색을 켜는 `\x1b[…m` 과 끄는 `\x1b[0m` 이다.
	for i := offset + 2; i < len(line); i++ {
		if line[i] >= '@' && line[i] <= '~' {
			return i - offset + 1
		}
	}

	// 끝을 못 찾으면 남은 것이 전부 escape 다. 반쪽짜리 escape 를 글자로 세면 폭이 늘어난다.
	return len(line) - offset
}

// screenWidthOfStyled 는 색을 입힌 줄이 차지하는 화면 칸 수다. escape 는 폭 0 이다.
//
// 다 지은 화면 줄을 재는 자리라 UI 쪽이다. screenWidthOf 와 같이 기본 폭으로 잰다.
func screenWidthOfStyled(text string) int {
	line := []byte(text)

	col := 0
	for offset := 0; offset < len(line); {
		if size := escapeSizeAt(line, offset); size > 0 {
			offset += size
			continue
		}

		size, width := clusterAt(line, offset, col, defaultTabWidth)
		col += width
		offset += size
	}

	return col
}

// prevClusterStart 는 offset 직전 글자의 시작을 돌려준다. from 은 글자 경계여야 한다.
//
// grapheme cluster 는 뒤에서 앞으로 읽을 수 없어서 알려진 경계에서부터 훑는다.
// 행 시작이 항상 경계이므로 훑는 범위는 화면 한 행으로 묶인다.
func prevClusterStart(line []byte, from, offset int) int {
	prev := from
	for i := from; i < offset && i < len(line); {
		prev = i
		i += clusterSize(line, i)
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
		size, w := clusterAt(line, offset, col, tab)

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

// rowBefore 는 화면 행 (line1,row1) 이 (line2,row2) 보다 위인지 본다.
func rowBefore(line1, row1, line2, row2 int) bool {
	return line1 < line2 || (line1 == line2 && row1 < row2)
}
