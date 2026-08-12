package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wide 는 wrap 이 일어나지 않을 만큼 넓은 화면이다.
const wide = 1000

func TestNewBuffer(t *testing.T) {
	tests := []struct {
		name            string
		data            string
		lines           []string
		lineEnding      lineEnding
		finalLineEnding bool
	}{
		{
			name:            "빈 파일은 빈 줄 하나다",
			data:            "",
			lines:           []string{""},
			lineEnding:      lineEndingLF,
			finalLineEnding: false,
		},
		{
			name:            "줄끝 하나뿐인 파일도 빈 줄 하나다",
			data:            "\n",
			lines:           []string{""},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
		{
			name:            "LF, 줄끝으로 끝남",
			data:            "a\nb\n",
			lines:           []string{"a", "b"},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
		{
			name:            "LF, 줄끝 없이 끝남",
			data:            "a\nb",
			lines:           []string{"a", "b"},
			lineEnding:      lineEndingLF,
			finalLineEnding: false,
		},
		{
			name:            "CRLF 는 줄에서 \\r 이 빠진다",
			data:            "a\r\nb\r\n",
			lines:           []string{"a", "b"},
			lineEnding:      lineEndingCRLF,
			finalLineEnding: true,
		},
		{
			name:            "CRLF, 줄끝 없이 끝남",
			data:            "a\r\nb",
			lines:           []string{"a", "b"},
			lineEnding:      lineEndingCRLF,
			finalLineEnding: false,
		},
		{
			name:            "빈 줄이 사이에 있어도 유지된다",
			data:            "a\n\nb\n",
			lines:           []string{"a", "", "b"},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
		{
			name:            "한글은 그대로 담긴다",
			data:            "한글\n두번째 줄\n",
			lines:           []string{"한글", "두번째 줄"},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.data))

			lines := make([]string, len(buf.lines))
			for i, line := range buf.lines {
				lines[i] = string(line)
			}

			assert.Equal(t, test.lines, lines)
			assert.Equal(t, test.lineEnding, buf.lineEnding)
			assert.Equal(t, test.finalLineEnding, buf.finalLineEnding)
		})
	}
}

// lines 는 data 를 가리키는 subslice 여야 한다. 복사하면 큰 파일에서 메모리가 4 배로 뛴다.
func TestNewBufferSharesData(t *testing.T) {
	buf := newBuffer("test.txt", []byte("hello\nworld\n"))
	require.Len(t, buf.lines, 2)

	buf.data[0] = 'H'
	assert.Equal(t, "Hello", string(buf.lines[0]), "lines 가 data 를 가리키지 않고 복사했다")
}

// 읽어서 그대로 저장하면 파일이 바이트 단위로 같아야 한다.
func TestBufferSaveRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "LF", data: "a\nb\n"},
		{name: "LF, 줄끝 없이 끝남", data: "a\nb"},
		{name: "CRLF", data: "a\r\nb\r\n"},
		{name: "CRLF, 줄끝 없이 끝남", data: "a\r\nb"},
		{name: "빈 파일", data: ""},
		{name: "빈 줄 포함", data: "a\n\nb\n"},
		{name: "한글", data: "한글\n두번째 줄\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.txt")
			require.NoError(t, os.WriteFile(path, []byte(test.data), 0644))

			buf, err := OpenBuffer(path)
			require.NoError(t, err)
			require.NoError(t, buf.Save())

			saved, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, test.data, string(saved))
		})
	}
}

func TestOpenBufferMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-exist.txt")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	assert.Equal(t, path, buf.path)
	assert.Equal(t, []string{""}, []string{string(buf.lines[0])})
	assert.Len(t, buf.lines, 1)
}

// 저장할 때 이미 있는 파일의 권한을 떨어뜨리면 안 된다. 실행 스크립트를 열었다 저장하는 경우다.
func TestBufferSaveKeepsFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0755))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)
	require.NoError(t, buf.Save())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestScreenCol(t *testing.T) {
	tests := []struct {
		line   string
		offset int
		col    int
	}{
		{line: "abc", offset: 0, col: 0},
		{line: "abc", offset: 2, col: 2},
		{line: "한글", offset: 0, col: 0},
		{line: "한글", offset: 3, col: 2}, // 한 글자 뒤 = 두 칸 뒤
		{line: "한글", offset: 6, col: 4},
		{line: "한a글", offset: 4, col: 3}, // 한(2) + a(1)
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/offset=%d", test.line, test.offset), func(t *testing.T) {
			assert.Equal(t, test.col, screenColAt([]byte(test.line), test.offset))
		})
	}
}

func TestOffsetAtScreenCol(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		col    int
		offset int
	}{
		{name: "ascii", line: "abc", col: 2, offset: 2},
		{name: "줄보다 큰 칸은 줄 끝으로", line: "abc", col: 99, offset: 3},
		{name: "한글 한 글자 뒤", line: "한글", col: 2, offset: 3},
		{name: "두 칸 글자 중간은 글자 시작으로", line: "한글", col: 1, offset: 0},
		{name: "한글 뒤 ascii", line: "한a", col: 2, offset: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.offset, offsetAtScreenCol([]byte(test.line), test.col))
		})
	}
}

// 짧은 줄을 지나가도 원래 열로 돌아와야 한다.
func TestCursorKeepsDesiredCol(t *testing.T) {
	buf := newBuffer("test.txt", []byte("0123456789\nab\n0123456789\n"))

	// 첫 줄에서 오른쪽으로 5 칸
	for range 5 {
		buf.moveRight(wide)
	}
	require.Equal(t, 5, screenColAt(buf.lines[buf.cursorLine], buf.cursorCol))

	// 짧은 줄로 내려가면 줄 끝까지만
	buf.moveDown(1, wide)
	assert.Equal(t, 1, buf.cursorLine)
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursorLine], buf.cursorCol), "짧은 줄에서는 줄 끝")

	// 다시 긴 줄로 내려가면 원래 열로 복귀
	buf.moveDown(1, wide)
	assert.Equal(t, 2, buf.cursorLine)
	assert.Equal(t, 5, screenColAt(buf.lines[buf.cursorLine], buf.cursorCol), "긴 줄로 돌아오면 원래 열")
}

func TestCursorMoveClamps(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\ncd\n"))

	buf.moveUp(1, wide)
	assert.Equal(t, 0, buf.cursorLine, "첫 줄 위로는 못 간다")

	buf.moveLeft(wide)
	assert.Equal(t, 0, buf.cursorCol, "줄 시작 왼쪽으로는 못 간다")

	buf.moveDown(99, wide)
	assert.Equal(t, 1, buf.cursorLine, "마지막 줄 아래로는 못 간다")

	buf.moveRight(wide)
	buf.moveRight(wide)
	buf.moveRight(wide)
	assert.Equal(t, 2, buf.cursorCol, "줄 끝 오른쪽으로는 못 간다")
}

// 한글은 rune 하나에 두 칸이다.
func TestCursorMoveHangul(t *testing.T) {
	buf := newBuffer("test.txt", []byte("한글abc\n"))

	buf.moveRight(wide)
	assert.Equal(t, 3, buf.cursorCol, "byte offset 은 3")
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursorLine], buf.cursorCol), "화면 칸은 2")

	buf.moveRight(wide)
	assert.Equal(t, 6, buf.cursorCol)
	assert.Equal(t, 4, screenColAt(buf.lines[buf.cursorLine], buf.cursorCol))

	buf.moveLeft(wide)
	assert.Equal(t, 3, buf.cursorCol)
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursorLine], buf.cursorCol))
}

// 커서가 화면 안에 있으면 화면은 움직이지 않아야 한다.
func TestScrollTo(t *testing.T) {
	buf := newBuffer("test.txt", []byte(strings.Repeat("line\n", 100)))
	height := 10

	buf.moveDown(5, wide)
	buf.scrollTo(wide, height)
	assert.Equal(t, 0, buf.top, "화면 안이면 안 움직인다")

	buf.moveDown(5, wide) // 10 번째 줄, 화면 아래로 한 줄 초과
	buf.scrollTo(wide, height)
	assert.Equal(t, 1, buf.top, "아래로 벗어나면 한 줄만 밀린다")

	buf.moveDown(50, wide)
	buf.scrollTo(wide, height)
	assert.Equal(t, 51, buf.top)

	buf.moveUp(20, wide)
	buf.scrollTo(wide, height)
	assert.Equal(t, 40, buf.top, "위로 벗어나면 커서 줄이 최상단")
}

func TestVisibleRows(t *testing.T) {
	buf := newBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))

	assert.Len(t, buf.visibleRows(wide, 3), 3)
	assert.Len(t, buf.visibleRows(wide, 99), 5, "줄 수보다 큰 화면은 있는 만큼만")

	buf.top = 3
	assert.Len(t, buf.visibleRows(wide, 99), 2, "top 이후만")
}

func TestWrapOffsets(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		width   int
		offsets []int
	}{
		{name: "화면에 들어가는 줄은 한 행", line: "abcd", width: 10, offsets: []int{0}},
		{name: "빈 줄도 한 행", line: "", width: 10, offsets: []int{0}},
		{name: "딱 맞는 줄은 한 행", line: "abcd", width: 4, offsets: []int{0}},
		{name: "넘치면 나뉜다", line: "abcdef", width: 4, offsets: []int{0, 4}},
		{name: "여러 번 나뉜다", line: "abcdefghij", width: 4, offsets: []int{0, 4, 8}},
		// 한글은 두 칸이므로 너비 5 에는 두 글자(4칸) 까지만 들어간다
		{name: "한글은 두 칸으로 센다", line: "한글한글", width: 5, offsets: []int{0, 6}},
		{name: "너비보다 넓은 글자는 한 행에 하나", line: "한글", width: 1, offsets: []int{0, 3}},
		{name: "너비 0 이면 나누지 않는다", line: "abcdef", width: 0, offsets: []int{0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.offsets, wrapOffsets([]byte(test.line), test.width))
		})
	}
}

// wrap 된 줄 안에서는 아래 이동이 같은 줄의 다음 행으로 가야 한다.
func TestCursorMovesByScreenRow(t *testing.T) {
	width := 4
	buf := newBuffer("test.txt", []byte("abcdefghij\nnext\n"))

	buf.moveDown(1, width)
	assert.Equal(t, 0, buf.cursorLine, "아직 같은 줄")
	assert.Equal(t, 4, buf.cursorCol, "두 번째 행의 시작")

	buf.moveDown(1, width)
	assert.Equal(t, 0, buf.cursorLine)
	assert.Equal(t, 8, buf.cursorCol, "세 번째 행의 시작")

	buf.moveDown(1, width)
	assert.Equal(t, 1, buf.cursorLine, "행이 끝나면 다음 줄")
	assert.Equal(t, 0, buf.cursorCol)

	buf.moveUp(1, width)
	assert.Equal(t, 0, buf.cursorLine, "위로도 행 단위")
	assert.Equal(t, 8, buf.cursorCol)
}

// wrap 된 줄에서 위아래로 움직여도 행 안에서의 칸이 유지되어야 한다.
func TestCursorKeepsDesiredColAcrossWrappedRows(t *testing.T) {
	width := 4
	buf := newBuffer("test.txt", []byte("abcdefghij\n"))

	buf.moveRight(width)
	buf.moveRight(width)
	require.Equal(t, 2, buf.cursorCol)
	require.Equal(t, 2, buf.desiredCol, "행 안에서 2 칸")

	buf.moveDown(1, width)
	assert.Equal(t, 6, buf.cursorCol, "두 번째 행의 2 칸 = offset 4+2")

	buf.moveDown(1, width)
	assert.Equal(t, 10, buf.cursorCol, "세 번째 행의 2 칸. 줄이 짧아 끝")
}

// wrap 된 줄이 화면을 넘으면 그 줄 중간부터 그려야 한다.
func TestScrollToWithinWrappedLine(t *testing.T) {
	width, height := 4, 3
	buf := newBuffer("test.txt", []byte(strings.Repeat("x", 40)+"\n"))
	require.Len(t, wrapOffsets(buf.lines[0], width), 10)

	buf.moveDown(2, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 0, buf.topRow, "화면 안이면 안 움직인다")

	buf.moveDown(1, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 0, buf.top, "같은 줄이다")
	assert.Equal(t, 1, buf.topRow, "행 하나만 밀린다")

	buf.moveDown(5, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 6, buf.topRow)

	buf.moveUp(4, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 4, buf.topRow, "위로 벗어나면 커서 행이 최상단")
}

func TestCursorScreenPos(t *testing.T) {
	width, height := 4, 5
	buf := newBuffer("test.txt", []byte("abcdefgh\nnext\n"))

	x, y, ok := buf.cursorScreenPos(width, height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)

	// 두 번째 행 시작
	buf.moveDown(1, width)
	x, y, ok = buf.cursorScreenPos(width, height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 1, y, "wrap 된 행도 화면 행을 차지한다")

	// 다음 줄
	buf.moveDown(1, width)
	x, y, ok = buf.cursorScreenPos(width, height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 2, y)
}

// 결합 문자는 rune 여러 개가 한 글자다. 커서 이동과 줄바꿈은 이 단위여야 한다.
func TestClusterAt(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		size  int
		width int
	}{
		{name: "ascii", text: "a", size: 1, width: 1},
		{name: "NFC 한글", text: "한", size: 3, width: 2},
		{name: "NFD 한글", text: "한", size: 9, width: 2},
		{name: "e + 결합 악센트", text: "é", size: 3, width: 1},
		{name: "ZWJ 가족 이모지", text: "\U0001F468‍\U0001F469‍\U0001F466", size: 18, width: 2},
		{name: "국기 이모지", text: "\U0001F1F0\U0001F1F7", size: 8, width: 2},
		{name: "피부색 이모지", text: "\U0001F44D\U0001F3FD", size: 8, width: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			size, width := clusterAt([]byte(test.text), 0, 0)

			assert.Equal(t, test.size, size, "글자 하나의 byte 길이")
			assert.Equal(t, test.width, width, "화면 폭")
			assert.Equal(t, test.width, screenColAt([]byte(test.text), len(test.text)))
		})
	}
}

// 깨진 UTF-8 에서도 진행해야 한다. 0 을 돌려주면 무한 반복이다.
func TestClusterAtInvalidUTF8(t *testing.T) {
	size, _ := clusterAt([]byte{0xff, 0xfe}, 0, 0)
	assert.Positive(t, size)

	assert.NotPanics(t, func() {
		buf := newBuffer("test.txt", []byte{0xff, 0xfe, '\n'})
		buf.moveRight(wide)
		buf.moveRight(wide)
		buf.moveLeft(wide)
	})
}

// 커서는 결합 문자를 한 번에 건너뛰어야 한다. rune 단위면 글자 중간에 선다.
func TestCursorMovesByCluster(t *testing.T) {
	tests := []struct {
		name string
		text string
		size int
	}{
		{name: "NFD 한글", text: "한", size: 9},
		{name: "e + 결합 악센트", text: "é", size: 3},
		{name: "ZWJ 가족 이모지", text: "\U0001F468‍\U0001F469‍\U0001F466", size: 18},
		{name: "국기 이모지", text: "\U0001F1F0\U0001F1F7", size: 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.text+"a\n"))

			buf.moveRight(wide)
			assert.Equal(t, test.size, buf.cursorCol, "글자 하나를 통째로 건너뛴다")

			buf.moveRight(wide)
			assert.Equal(t, test.size+1, buf.cursorCol)

			buf.moveLeft(wide)
			assert.Equal(t, test.size, buf.cursorCol)

			buf.moveLeft(wide)
			assert.Equal(t, 0, buf.cursorCol, "글자 중간이 아니라 시작으로 돌아온다")
		})
	}
}

// 줄바꿈이 결합 문자 중간을 끊으면 화면에 깨진 글자가 나온다.
func TestWrapDoesNotSplitCluster(t *testing.T) {
	emoji := "\U0001F468‍\U0001F469‍\U0001F466" // 18 byte, 2 칸
	buf := newBuffer("test.txt", []byte(strings.Repeat(emoji, 3)+"\n"))

	// 폭 5 면 두 글자(4 칸) 까지만 들어간다
	offsets := wrapOffsets(buf.lines[0], 5)
	assert.Equal(t, []int{0, 36}, offsets, "글자 경계에서만 끊긴다")

	// 폭 6 이면 세 글자가 딱 맞는다
	assert.Equal(t, []int{0}, wrapOffsets(buf.lines[0], 6))
}

// 행 경계를 넘어 왼쪽으로 갈 때도 앞 행의 마지막 글자 시작으로 가야 한다.
func TestMoveLeftAcrossWrappedRowWithClusters(t *testing.T) {
	emoji := "\U0001F468‍\U0001F469‍\U0001F466"
	buf := newBuffer("test.txt", []byte(strings.Repeat(emoji, 3)+"\n"))
	width := 5

	buf.cursorCol = 36 // 두 번째 행의 시작
	require.Equal(t, 1, rowIndexAt(wrapOffsets(buf.lines[0], width), buf.cursorCol))

	buf.moveLeft(width)
	assert.Equal(t, 18, buf.cursorCol, "앞 행 마지막 글자의 시작")
}

// tab 은 다음 tab stop 까지 밀어내므로 시작 위치에 따라 폭이 다르다.
func TestClusterAtTab(t *testing.T) {
	tests := []struct {
		col   int
		width int
	}{
		{col: 0, width: 8},
		{col: 1, width: 7},
		{col: 7, width: 1},
		{col: 8, width: 8},
		{col: 9, width: 7},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("col=%d", test.col), func(t *testing.T) {
			size, width := clusterAt([]byte("\t"), 0, test.col)

			assert.Equal(t, 1, size, "tab 은 1 byte")
			assert.Equal(t, test.width, width)
		})
	}
}

func TestScreenColWithTab(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		offset int
		col    int
	}{
		{name: "줄 앞 tab", line: "\tab", offset: 1, col: 8},
		{name: "tab 뒤 글자", line: "\tab", offset: 2, col: 9},
		{name: "tab 두 개", line: "\t\ta", offset: 2, col: 16},
		{name: "글자 뒤 tab 은 남은 칸만", line: "ab\tc", offset: 3, col: 8},
		{name: "7 칸 뒤 tab 은 1 칸", line: "0123456\tx", offset: 8, col: 8},
		{name: "8 칸 뒤 tab 은 8 칸", line: "01234567\tx", offset: 9, col: 16},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.col, screenColAt([]byte(test.line), test.offset))
		})
	}
}

// 커서는 tab 을 한 번에 건너뛰고, 화면 칸은 tab stop 을 따라야 한다.
func TestCursorMoveOverTab(t *testing.T) {
	buf := newBuffer("test.txt", []byte("\tab\n"))

	buf.moveRight(wide)
	assert.Equal(t, 1, buf.cursorCol, "tab 은 1 byte")
	assert.Equal(t, 8, screenColAt(buf.lines[0], buf.cursorCol), "화면 칸은 8")

	buf.moveLeft(wide)
	assert.Equal(t, 0, buf.cursorCol)
}

// tab 이 든 줄도 화면 너비 기준으로 나뉘어야 한다.
func TestWrapWithTab(t *testing.T) {
	// tab(8 칸) + "abcd" 를 너비 10 에 넣으면 ab 까지만 들어간다
	assert.Equal(t, []int{0, 3}, wrapOffsets([]byte("\tabcd"), 10))
}
