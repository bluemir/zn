package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 재보지 않은 편집기는 지금까지와 똑같이 그린다. 테스트가 전부 이 자리에 있다.
func TestBoxCharsDefaultToUnicode(t *testing.T) {
	assert.Equal(t, boxUnicode, editor{}.boxChars())
	assert.Equal(t, boxASCII, editor{asciiBox: true}.boxChars())
}

// ASCII 로 내려도 박스 글자는 전부 한 칸이어야 한다. 물러선 이유가 그것뿐이다.
func TestASCIIBoxCharsAreSingleWidth(t *testing.T) {
	for _, char := range []string{
		boxASCII.vertical, boxASCII.horizontal,
		boxASCII.topLeft, boxASCII.topRight, boxASCII.bottomLeft, boxASCII.bottomRight,
		boxASCII.leftTee, boxASCII.rightTee,
	} {
		assert.Equal(t, 1, ansi.StringWidth(char), "%q", char)
	}
}

// ASCII 모드에서도 팔레트 테두리가 유니코드일 때와 정확히 같은 자리에 있어야 한다.
func TestASCIIPaletteBox(t *testing.T) {
	m := newPaletteView(t, 80, 20, "internal/core/edit.go", "main.go")
	m.asciiBox = true

	rows := strings.Split(m.box(), "\n")
	require.NotEmpty(t, rows)

	assert.True(t, strings.HasPrefix(rows[0], "+-"), "위 모서리: %q", rows[0])
	assert.True(t, strings.HasSuffix(rows[0], "-+"), "위 모서리: %q", rows[0])
	assert.True(t, strings.HasPrefix(rows[len(rows)-1], "+-"), "아래 모서리: %q", rows[len(rows)-1])

	for i, row := range rows {
		plain := ansi.Strip(row)

		assert.Equal(t, m.paletteWidth(), ansi.StringWidth(plain), "행 %d: %q", i, plain)
		assert.NotContains(t, plain, "│", "행 %d 에 유니코드 박스 글자가 남았다", i)
		assert.NotContains(t, plain, "─", "행 %d 에 유니코드 박스 글자가 남았다", i)
	}
}

// sidebar 구분선도 같이 내려간다. 한 행이 sidebarWidth 칸이라는 것은 그대로다.
func TestASCIISidebarCells(t *testing.T) {
	s := openSidebar(newTreeFixture(t))

	for i, cell := range s.cells(10, "", boxASCII) {
		plain := ansi.Strip(cell)

		assert.Equal(t, sidebarWidth, screenColAt([]byte(plain), len(plain)), "행 %d: %q", i, plain)
		assert.True(t, strings.HasSuffix(plain, "| "), "행 %d: %q", i, plain)
	}
}

// tabline 구분자도 같이 내려간다.
func TestASCIITabline(t *testing.T) {
	m := newTestEditor("a\n", 80, 20)
	m.buffers = append(m.buffers, Buffer{path: "b.txt"})
	m.asciiBox = true

	line := m.tabline(m.textWidth()).line

	assert.Contains(t, ansi.Strip(line), "|")
	assert.NotContains(t, ansi.Strip(line), "│")
}

// columnOf 는 커서 위치 답에서 열만 뽑는다. 앞에 사용자가 미리 쳐둔 키가 섞일 수 있다.
func TestColumnOf(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		want   int
	}{
		{name: "한 칸", answer: "\x1b[12;2R", want: 2},
		{name: "두 칸", answer: "\x1b[12;3R", want: 3},
		{name: "DECXCPR 형태", answer: "\x1b[?12;3R", want: 3},
		{name: "앞에 친 키가 섞임", answer: "jj\x1b[7;2R", want: 2},
		{name: "답이 없음", answer: "", want: 0},
		{name: "끝나지 않음", answer: "\x1b[12;2", want: 0},
		{name: "열이 없음", answer: "\x1b[12R", want: 0},
		{name: "숫자가 아님", answer: "\x1b[12;xR", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, columnOf([]byte(test.answer)))
		})
	}
}

// tty 가 아니면 재지 않고 곧바로 0 이다. 파이프로 돌리는 테스트·CI 가 그렇다.
func TestProbeWithoutTerminal(t *testing.T) {
	assert.Equal(t, 0, probeAmbiguousWidth())
}
