package textarea

// 다 지은 화면 글을 재는 자리다.
//
// **cluster.go 의 반대편이다.** 그쪽은 파일에서 온 줄을 받아 「이 글을 화면에 그리면 몇
// 칸인가」를 답하고, 여기는 이미 화면용으로 지어진 글을 받아 「이 글이 몇 칸인가」를 답한다.
// **변환 전과 후라 같은 `\x1b` 를 다르게 읽는다** — 저쪽에서는 `^[` 로 보일 내용이고
// 여기서는 lipgloss 가 붙인 색이다 (ADR-0118).
//
// 그래서 파일이 갈렸다. 같은 파일에 두면 어느 쪽 글을 받는 함수인지가 이름에만 남는다.

// widthOption 은 widthOf 가 재는 방식을 바꾼다. 지금은 tab 폭 하나뿐이다.
type WidthOption func(*int)

// withTabSize 는 tab 하나가 미는 칸 수를 정한다.
//
// 파일에서 온 글을 잴 때 그 파일의 폭을 넘긴다(`buf.TabWidth()`). 그 폭이 파일마다 다르기
// 때문이다 (ADR-0096).
func WithTabSize(tab int) WidthOption {
	return func(width *int) { *width = tab }
}

// widthOf 는 그 글이 차지하는 화면 칸 수다. ANSI escape 는 폭 0 이다.
//
// **색을 입힌 글과 안 입힌 글이 갈리지 않는다.** escape 가 없으면 건너뛸 것이 없을 뿐이라
// 한 함수가 둘 다 맞는다. 거꾸로 escape 를 안 보는 함수를 따로 두면 색이 들어왔을 때 그것을
// 글자로 세어 조용히 틀리고, 부르는 쪽이 매번 「이 글에 색이 붙었나」를 맞혀야 한다.
//
// **tab 은 적지 않으면 기본 폭이다.** 화면에 지어 붙인 글(tabline 의 제목, statusBar, 팔레트
// 후보) 에는 tab 이 들어올 일이 없어서 그 값이 답에 닿지 않는다. 파일에서 온 글을 잴 때는
// 그 파일의 폭을 넘긴다.
//
//	widthOf(title)                              // 지어 붙인 글
//	widthOf(text, withTabSize(buf.TabWidth()))  // 파일에서 온 글
func WidthOf(text string, opts ...WidthOption) int {
	tab := DefaultTabWidth
	for _, opt := range opts {
		opt(&tab)
	}

	line := []byte(text)

	col := 0
	for offset := 0; offset < len(line); {
		if size := EscapeSizeAt(line, offset); size > 0 {
			offset += size

			continue
		}

		size, width := GlyphAt(line, offset, col, tab)
		col += width
		offset += size
	}

	return col
}

// escapeSizeAt 은 offset 에서 시작하는 ANSI escape 의 byte 길이다. escape 가 아니면 0 이다.
//
// **다 지은 화면 줄에만 쓴다.** 색을 입힌 글자는 escape 를 달고 오는데 그것은 화면에서 자리를
// 차지하지 않으므로, 재거나 자를 때 지나쳐야 한다(ADR-0061).
//
// buffer 안의 글자에는 쓰지 않는다 — 파일에 든 ESC byte 는 색이 아니라 내용이다. 그것이
// 이 둘이 cluster.go 가 아니라 여기 사는 까닭이다.
func EscapeSizeAt(line []byte, offset int) int {
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
