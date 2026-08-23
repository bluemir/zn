package core

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	m.boxChars = boxASCII

	rows := strings.Split(m.renderBox(), "\n")
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
	s := openSidebarSync(t, newTreeFixture(t))

	for i, cell := range s.renderCells(10, "", boxASCII) {
		plain := ansi.Strip(cell)

		assert.Equal(t, sidebarWidth, screenColAt([]byte(plain), len(plain)), "행 %d: %q", i, plain)
		assert.True(t, strings.HasSuffix(plain, "| "), "행 %d: %q", i, plain)
	}
}

// tabline 구분자도 같이 내려간다.
func TestASCIITabline(t *testing.T) {
	m := newTestEditor("a\n", 80, 20)
	m.buffers = append(m.buffers, Buffer{path: "b.txt"})
	m.boxChars = boxASCII

	line := m.renderTabline(m.textWidth()).line

	assert.Contains(t, ansi.Strip(line), "|")
	assert.NotContains(t, ansi.Strip(line), "│")
}
