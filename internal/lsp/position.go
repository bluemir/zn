package lsp

import "unicode/utf8"

// 여기 둘이 zn 의 셈과 LSP 의 셈을 잇는 전부다.
//
// zn 은 줄 안의 자리를 byte 로 센다(core/buffer.go 의 cursorCol). LSP 는 UTF-16 코드 단위로
// 센다. 한글 한 글자가 byte 로는 3, UTF-16 으로는 1 이라 한글이 든 줄에서 이 둘이 갈린다 —
// 바꾸지 않고 보내면 gopls 가 `column is beyond end of line` 로 거절한다(ADR-0051).
//
// 이 변환이 필요한 자리는 두 곳뿐이다. 묻는 자리(커서) 와 답을 받는 자리(정의의 위치) 다.
// 동기화는 줄 단위로만 보내서 열이 늘 0 이라 지나가지 않는다(document.go 의 diff).

// UTF16Column 은 줄 안의 byte 자리를 UTF-16 자리로 바꾼다.
//
// byteCol 이 줄보다 길면 줄 끝이다 — 커서가 줄 끝 다음 칸에 서는 mode 가 있어서(insert)
// 그 자리가 실제로 온다.
func UTF16Column(line []byte, byteCol int) int {
	if byteCol > len(line) {
		byteCol = len(line)
	}

	column := 0
	for i := 0; i < byteCol; {
		r, size := utf8.DecodeRune(line[i:])
		if size == 0 {
			break
		}

		// 잘못된 byte 는 그 자체로 한 단위다. utf8.DecodeRune 이 RuneError 를 size 1 로 준다.
		column++
		if r > 0xFFFF {
			column++ // BMP 밖은 두 단위(surrogate pair) 다
		}

		i += size
	}

	return column
}

// ByteColumn 은 UTF-16 자리를 줄 안의 byte 자리로 바꾼다. UTF16Column 의 반대다.
//
// 자리가 글자 가운데를 가리키면(surrogate pair 의 뒤쪽 단위) 그 글자의 시작으로 본다.
// 그런 자리를 서버가 줄 이유는 없지만, 받으면 줄 안의 엉뚱한 byte 를 가리키는 것보다 낫다.
func ByteColumn(line []byte, column int) int {
	if column <= 0 {
		return 0
	}

	units := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		if size == 0 {
			break
		}

		if units >= column {
			return i
		}

		units++
		if r > 0xFFFF {
			units++
		}

		i += size
	}

	return len(line)
}
