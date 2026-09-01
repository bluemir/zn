package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 제어문자를 두 글자로 보이게 하는 것과 두 칸으로 세는 것이 같은 자리에서 나와야 한다.
// 갈리면 화면에 그린 것과 커서가 어긋난다 (ADR-0118).
func TestControlText(t *testing.T) {
	tests := []struct {
		name string
		b    byte
		want string
	}{
		{name: "ESC", b: 0x1b, want: "^["},
		{name: "NUL", b: 0x00, want: "^@"},
		{name: "CR", b: 0x0d, want: "^M"},
		{name: "DEL", b: 0x7f, want: "^?"},
		{name: "tab 은 칸으로 펴는 쪽이다", b: '\t', want: ""},
		{name: "줄바꿈은 줄 안에 없다", b: '\n', want: ""},
		{name: "보통 글자", b: 'a', want: ""},
		{name: "UTF-8 이어지는 byte", b: 0x95, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, controlText(test.b))
		})
	}
}

func TestGlyphAtControl(t *testing.T) {
	size, width := glyphAt([]byte("\x1b"), 0, 0, 4)

	assert.Equal(t, 1, size, "파일에서는 1 byte 다")
	assert.Equal(t, 2, width, "화면에서는 `^[` 두 칸이다")
}

// screenColAt 은 byte offset 을 화면 칸으로 옮긴다. 글자 경계가 아니면 그 글자를 다 센다.
func TestColAt(t *testing.T) {
	line := []byte("한글\tx")

	tests := []struct {
		name   string
		offset int
		want   int
	}{
		{name: "줄 시작", offset: 0, want: 0},
		{name: "글자 가운데는 그 글자를 다 센다", offset: 1, want: 2},
		{name: "한 다음", offset: 3, want: 2},
		{name: "한글 다음", offset: 6, want: 4},
		{name: "tab 은 다음 tab stop 까지 민다", offset: 7, want: 8},
		{name: "줄 끝은 줄 전체 폭", offset: 8, want: 9},
		{name: "줄을 넘어가면 줄 전체 폭", offset: 100, want: 9},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, screenColAt(line, test.offset, 4))
		})
	}
}

// offsetAtScreenCol 은 그 반대다. 여러 칸을 쓰는 글자의 가운데면 그 글자의 시작으로 맞춘다.
func TestOffsetAtCol(t *testing.T) {
	line := []byte("한글")

	assert.Equal(t, 0, offsetAtScreenCol(line, 0, 4))
	assert.Equal(t, 0, offsetAtScreenCol(line, 1, 4), "「한」의 둘째 칸은 그 시작으로")
	assert.Equal(t, 3, offsetAtScreenCol(line, 2, 4))
	assert.Equal(t, len(line), offsetAtScreenCol(line, 100, 4), "줄을 넘으면 줄 끝")
}
