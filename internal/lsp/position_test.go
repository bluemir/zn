package lsp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// zn 은 줄 안의 자리를 byte 로 세고 LSP 는 UTF-16 으로 센다. 그 둘을 잇는 자리다(ADR-0051).
func TestUTF16Column(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		byteCol int
		want    int
	}{
		{name: "줄 머리", line: "func main() {}", byteCol: 0, want: 0},
		{name: "ASCII 는 byte 와 같다", line: "func main() {}", byteCol: 5, want: 5},
		{name: "한글 뒤는 byte 의 1/3", line: "가나다", byteCol: 9, want: 3},
		{name: "한글 한 글자 뒤", line: "가나다", byteCol: 3, want: 1},
		{name: "한글과 ASCII 가 섞인 줄", line: "// 가나다 openBuffers", byteCol: 20, want: 14},
		{name: "이모지는 두 단위다", line: "🙂ab", byteCol: 4, want: 2},
		{name: "이모지 뒤의 ASCII", line: "🙂ab", byteCol: 6, want: 4},
		{name: "줄보다 긴 자리는 줄 끝이다", line: "가나다", byteCol: 100, want: 3},
		{name: "빈 줄", line: "", byteCol: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UTF16Column([]byte(tt.line), tt.byteCol))
		})
	}
}

func TestByteColumn(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		column int
		want   int
	}{
		{name: "줄 머리", line: "func main() {}", column: 0, want: 0},
		{name: "ASCII 는 byte 와 같다", line: "func main() {}", column: 5, want: 5},
		{name: "한글 세 글자 뒤", line: "가나다", column: 3, want: 9},
		{name: "한글 한 글자 뒤", line: "가나다", column: 1, want: 3},
		{name: "한글과 ASCII 가 섞인 줄", line: "// 가나다 openBuffers", column: 14, want: 20},
		{name: "이모지 뒤", line: "🙂ab", column: 2, want: 4},
		{name: "줄보다 긴 자리는 줄 끝이다", line: "가나다", column: 100, want: 9},
		{name: "음수는 줄 머리다", line: "가나다", column: -1, want: 0},
		{name: "빈 줄", line: "", column: 3, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ByteColumn([]byte(tt.line), tt.column))
		})
	}
}

// 두 함수는 서로의 반대다. 글자 경계에서 물으면 왕복해도 같은 자리여야 한다.
func TestColumnRoundTrip(t *testing.T) {
	lines := []string{
		"func main() {}",
		"가나다라마",
		"// 가나다 openBuffers 를 부른다",
		"\t\t버퍼 := openBuffers(files)",
		"🙂 이모지가 든 줄",
	}

	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			bytes := []byte(line)

			// 글자 경계마다 왕복한다. 경계는 rune 을 따라가며 얻는다.
			for offset := range line {
				if offset > 0 && !isBoundary(line, offset) {
					continue
				}

				column := UTF16Column(bytes, offset)
				assert.Equal(t, offset, ByteColumn(bytes, column), "byte %d", offset)
			}
		})
	}
}

// isBoundary 는 그 byte 자리가 글자의 시작인지다.
func isBoundary(s string, offset int) bool {
	return s[offset]&0xC0 != 0x80
}

func TestUTF16Len(t *testing.T) {
	assert.Equal(t, 0, utf16Len([]byte("")))
	assert.Equal(t, 3, utf16Len([]byte("abc")))
	assert.Equal(t, 3, utf16Len([]byte("가나다")))
	assert.Equal(t, 2, utf16Len([]byte("🙂")))
	assert.Equal(t, 4, utf16Len([]byte("가🙂나")))
}
