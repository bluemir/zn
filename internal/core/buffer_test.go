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

// saveBuffer, saveBufferForce 는 시험이 저장을 부르는 자리다.
//
// 저장이 `.editorconfig` 를 따라 파일을 맞추게 되면서 문구가 하나 더 나오는데(ADR-0052),
// 여기 시험들이 보는 것은 그것이 아니라 「쓰였는가」다. 맞추기만 보는 시험은 따로 있다
// (editorconfig_test.go).
func saveBuffer(t *testing.T, buf *viewport) error {
	t.Helper()

	_, err := buf.Save(wide, nil)

	return err
}

func saveBufferForce(t *testing.T, buf *viewport) error {
	t.Helper()

	_, err := buf.SaveForce(wide, nil)

	return err
}

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
			require.NoError(t, saveBuffer(t, &buf))

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
	require.NoError(t, saveBuffer(t, &buf))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

// 읽은 뒤에 밖에서 바뀐 파일은 덮어쓰지 않는다. 남의 편집을 조용히 날리지 않기 위해서다.
func TestBufferSaveRefusesWhenFileChangedOutside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	err = saveBuffer(t, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "바뀌었습니다")
	assert.True(t, buf.dirty, "저장되지 않았으므로 변경 표시가 남는다")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "남이 쓴 것\n", string(after), "파일을 건드리지 않는다")
}

// `:w!` 는 알고도 덮어쓰겠다는 뜻이다.
func TestBufferSaveForceOverwritesChangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	require.NoError(t, saveBufferForce(t, &buf))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(after))
	assert.False(t, buf.dirty)
}

// 내용이 같으면 밖에서 되쓰였어도 헛경고를 내지 않는다. mtime 이 아니라 내용을 보는 이유다.
func TestBufferSaveAllowsRewriteWithSameContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(after))
}

// 저장한 뒤에는 방금 쓴 것이 기준이다. 이어지는 저장이 자기가 쓴 것을 남의 변경으로 보면 안 된다.
func TestBufferSaveTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, saveBuffer(t, &buf))

	buf.insert([]byte("Y"), wide)
	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "XYabc\n", string(after))
}

// 열 때 없던 파일이 저장 시점에 생겨 있으면 남이 만든 것이다.
func TestBufferSaveRefusesWhenFileAppeared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.WriteFile(path, []byte("남이 만든 것\n"), 0644))

	err = saveBuffer(t, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "새로 생겼습니다")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "남이 만든 것\n", string(after))
}

// 열 때도 없었고 지금도 없으면 그냥 새로 만든다.
func TestBufferSaveCreatesNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "X\n", string(after))
}

// 밖에서 지워진 파일도 알린다. 조용히 되살아나면 지운 쪽이 모른다.
func TestBufferSaveRefusesWhenFileRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.Remove(path))

	err = saveBuffer(t, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "사라졌습니다")
	assert.NoFileExists(t, path)

	require.NoError(t, saveBufferForce(t, &buf), "`:w!` 로 다시 만들 수 있다")
	assert.FileExists(t, path)
}

// 다시 읽으면 바깥 내용이 들어오고 저장하지 않은 변경과 undo 이력은 사라진다 (ADR-0016).
func TestBufferReloadTakesOutsideChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	require.NoError(t, buf.Reload())

	assert.Equal(t, "남이 쓴 것", string(buf.lines[0]))
	assert.False(t, buf.dirty)
	assert.Empty(t, buf.undo, "이력은 버린다")
	assert.Empty(t, buf.redo)
}

// 다시 읽은 뒤 바로 저장해도 막히지 않는다. 방금 읽은 것이 새 기준이다.
func TestBufferReloadResetsDiskHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))
	require.NoError(t, buf.Reload())

	buf.insert([]byte("X"), wide)
	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "X남이 쓴 것\n", string(after))
}

// 커서는 줄 번호와 화면 칸을 유지한다. byte offset 이 아니라 칸이라 두 칸 글자 중간에 서지 않는다.
func TestBufferReloadKeepsCursorColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\ndef\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.moveDownLine(1)
	buf.moveRight(2, wide)
	require.Equal(t, 2, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth))

	// 둘째 줄이 두 칸 글자로 바뀐다. 2 칸은 두 번째 글자의 시작이다.
	require.NoError(t, os.WriteFile(path, []byte("abc\n한글\n"), 0644))
	require.NoError(t, buf.Reload())

	assert.Equal(t, 1, buf.cursor.Line)
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth))
	assert.Equal(t, "글", string(buf.lines[1][buf.cursor.Col:]), "글자 경계에 선다")
}

// **다시 읽어도 창이 받은 크기는 그대로다.**
//
// Reload 는 `newBuffer` 로 새 창을 지어 갈아끼우는데, 그 창은 editor 를 지나오지 않아서
// 자기 크기를 모른다. adopt 이 이어받지 않으면 폭이 0 이 되어 다시 읽은 순간 본문이 사라진다
// (ADR-0123).
func TestBufferReloadKeepsPane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	setContentWidth(&buf, 40, 12)
	want := buf.size

	require.NoError(t, os.WriteFile(path, []byte("abcdef\n"), 0644))
	require.NoError(t, buf.Reload())

	assert.Equal(t, want, buf.size)
	assert.Equal(t, 40, buf.contentWidth(), "본문 폭이 살아 있다")
}

// 파일이 짧아졌으면 커서를 범위 안으로 끌어온다.
func TestBufferReloadClampsCursorToShorterFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("a\nb\nc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.moveDownLine(2)
	require.Equal(t, 2, buf.cursor.Line)

	require.NoError(t, os.WriteFile(path, []byte("a\n"), 0644))
	require.NoError(t, buf.Reload())

	assert.Equal(t, 0, buf.cursor.Line, "한 줄만 남았으므로 그 줄로 끌려온다")
	assert.Equal(t, 0, buf.cursor.Col)
}

// 밖에서 지워진 파일은 다시 읽지 않는다. 손에 든 것이 마지막 사본이다.
func TestBufferReloadRefusesWhenFileRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"), wide)
	require.NoError(t, os.Remove(path))

	err = buf.Reload()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "파일이 없습니다")
	assert.Equal(t, "Xabc", string(buf.lines[0]), "내용을 건드리지 않는다")
	assert.True(t, buf.dirty)
}

// 이름 없는 buffer 는 다시 읽을 곳이 없다.
func TestBufferReloadRefusesWithoutPath(t *testing.T) {
	buf := newEmptyBuffer("")

	err := buf.Reload()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "파일 이름이 없습니다")
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
			assert.Equal(t, test.col, screenColAt([]byte(test.line), test.offset, defaultTabWidth))
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
			assert.Equal(t, test.offset, offsetAtScreenCol([]byte(test.line), test.col, defaultTabWidth))
		})
	}
}

// 짧은 줄을 지나가도 원래 열로 돌아와야 한다.
func TestCursorKeepsDesiredCol(t *testing.T) {
	buf := newBuffer("test.txt", []byte("0123456789\nab\n0123456789\n"))

	// 첫 줄에서 오른쪽으로 5 칸
	for range 5 {
		buf.moveRight(1, wide)
	}
	require.Equal(t, 5, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth))

	// 짧은 줄로 내려가면 줄 끝까지만
	buf.moveDownRow(1, wide)
	assert.Equal(t, 1, buf.cursor.Line)
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth), "짧은 줄에서는 줄 끝")

	// 다시 긴 줄로 내려가면 원래 열로 복귀
	buf.moveDownRow(1, wide)
	assert.Equal(t, 2, buf.cursor.Line)
	assert.Equal(t, 5, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth), "긴 줄로 돌아오면 원래 열")
}

func TestCursorMoveClamps(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\ncd\n"))

	buf.moveUpRow(1, wide)
	assert.Equal(t, 0, buf.cursor.Line, "첫 줄 위로는 못 간다")

	buf.moveLeft(1, wide)
	assert.Equal(t, 0, buf.cursor.Col, "줄 시작 왼쪽으로는 못 간다")

	buf.moveDownRow(99, wide)
	assert.Equal(t, 1, buf.cursor.Line, "마지막 줄 아래로는 못 간다")

	buf.moveRight(1, wide)
	buf.moveRight(1, wide)
	buf.moveRight(1, wide)
	assert.Equal(t, 2, buf.cursor.Col, "줄 끝 오른쪽으로는 못 간다")
}

// moveLeft, moveRight 는 count 만큼 움직이고 줄 양끝에서 멈춘다.
func TestCursorMoveCounted(t *testing.T) {
	buf := newBuffer("test.txt", []byte("0123456789\n"))

	buf.moveRight(5, wide)
	assert.Equal(t, 5, buf.cursor.Col)

	buf.moveLeft(3, wide)
	assert.Equal(t, 2, buf.cursor.Col)

	buf.moveRight(99, wide)
	assert.Equal(t, 10, buf.cursor.Col, "줄 끝에서 멈춘다")

	buf.moveLeft(99, wide)
	assert.Equal(t, 0, buf.cursor.Col, "줄 시작에서 멈춘다")
}

// moveUpLine·moveDownLine 은 논리 줄 단위다. wrap 된 줄도 한 번에 건넌다.
func TestCursorMoveByLine(t *testing.T) {
	// 폭 10 이라 첫 줄이 화면 행 세 개다.
	buf := newBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\nsecond\nthird\n"))
	const width = 10

	buf.moveDownLine(1)
	assert.Equal(t, 1, buf.cursor.Line, "wrap 된 줄을 한 번에 건넌다")

	// 같은 자리에서 화면 행 단위로 내려가면 같은 줄 안에서 행만 옮긴다.
	other := newBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\nsecond\nthird\n"))
	other.moveDownRow(1, width)
	assert.Equal(t, 0, other.cursor.Line, "moveDown 은 화면 행 단위라 같은 줄에 남는다")

	buf.moveDownLine(99)
	assert.Equal(t, 2, buf.cursor.Line, "마지막 줄 아래로는 못 간다")

	buf.moveUpLine(99)
	assert.Equal(t, 0, buf.cursor.Line, "첫 줄 위로는 못 간다")
}

// 줄 단위 이동도 짧은 줄을 지나가면 원래 칸으로 돌아온다.
func TestCursorMoveByLineKeepsDesiredCol(t *testing.T) {
	buf := newBuffer("test.txt", []byte("0123456789\nab\n0123456789\n"))

	buf.moveRight(5, wide)
	require.Equal(t, 5, buf.cursor.Col)

	buf.moveDownLine(1)
	buf.clampToNormal(wide)
	assert.Equal(t, 1, buf.cursor.Col, "짧은 줄에서는 마지막 글자 위")

	buf.moveDownLine(1)
	assert.Equal(t, 5, buf.cursor.Col, "긴 줄로 돌아오면 원래 칸")
}

// 한글은 rune 하나에 두 칸이다.
func TestCursorMoveHangul(t *testing.T) {
	buf := newBuffer("test.txt", []byte("한글abc\n"))

	buf.moveRight(1, wide)
	assert.Equal(t, 3, buf.cursor.Col, "byte offset 은 3")
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth), "화면 칸은 2")

	buf.moveRight(1, wide)
	assert.Equal(t, 6, buf.cursor.Col)
	assert.Equal(t, 4, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth))

	buf.moveLeft(1, wide)
	assert.Equal(t, 3, buf.cursor.Col)
	assert.Equal(t, 2, screenColAt(buf.lines[buf.cursor.Line], buf.cursor.Col, defaultTabWidth))
}

// 커서가 화면 안에 있으면 화면은 움직이지 않아야 한다.
func TestScrollTo(t *testing.T) {
	buf := newBuffer("test.txt", []byte(strings.Repeat("line\n", 100)))
	height := 10

	buf.moveDownRow(5, wide)
	buf.scrollTo(wide, height)
	assert.Equal(t, 0, buf.top.line, "화면 안이면 안 움직인다")

	buf.moveDownRow(5, wide) // 10 번째 줄, 화면 아래로 한 줄 초과
	buf.scrollTo(wide, height)
	assert.Equal(t, 1, buf.top.line, "아래로 벗어나면 한 줄만 밀린다")

	buf.moveDownRow(50, wide)
	buf.scrollTo(wide, height)
	assert.Equal(t, 51, buf.top.line)

	buf.moveUpRow(20, wide)
	buf.scrollTo(wide, height)
	assert.Equal(t, 40, buf.top.line, "위로 벗어나면 커서 줄이 최상단")
}

func TestVisibleRows(t *testing.T) {
	buf := newBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))

	assert.Len(t, buf.visibleRows(wide, 3), 3)
	assert.Len(t, buf.visibleRows(wide, 99), 5, "줄 수보다 큰 화면은 있는 만큼만")

	buf.top.line = 3
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
			assert.Equal(t, test.offsets, wrapOffsets([]byte(test.line), test.width, defaultTabWidth))
		})
	}
}

// wrap 된 줄 안에서는 아래 이동이 같은 줄의 다음 행으로 가야 한다.
func TestCursorMovesByScreenRow(t *testing.T) {
	width := 4
	buf := newBuffer("test.txt", []byte("abcdefghij\nnext\n"))

	buf.moveDownRow(1, width)
	assert.Equal(t, 0, buf.cursor.Line, "아직 같은 줄")
	assert.Equal(t, 4, buf.cursor.Col, "두 번째 행의 시작")

	buf.moveDownRow(1, width)
	assert.Equal(t, 0, buf.cursor.Line)
	assert.Equal(t, 8, buf.cursor.Col, "세 번째 행의 시작")

	buf.moveDownRow(1, width)
	assert.Equal(t, 1, buf.cursor.Line, "행이 끝나면 다음 줄")
	assert.Equal(t, 0, buf.cursor.Col)

	buf.moveUpRow(1, width)
	assert.Equal(t, 0, buf.cursor.Line, "위로도 행 단위")
	assert.Equal(t, 8, buf.cursor.Col)
}

// wrap 된 줄에서 위아래로 움직여도 행 안에서의 칸이 유지되어야 한다.
func TestCursorKeepsDesiredColAcrossWrappedRows(t *testing.T) {
	width := 4
	buf := newBuffer("test.txt", []byte("abcdefghij\n"))

	buf.moveRight(1, width)
	buf.moveRight(1, width)
	require.Equal(t, 2, buf.cursor.Col)
	require.Equal(t, 2, buf.desiredX, "행 안에서 2 칸")

	buf.moveDownRow(1, width)
	assert.Equal(t, 6, buf.cursor.Col, "두 번째 행의 2 칸 = offset 4+2")

	buf.moveDownRow(1, width)
	assert.Equal(t, 10, buf.cursor.Col, "세 번째 행의 2 칸. 줄이 짧아 끝")
}

// **wrap 된 줄의 둘째 행에서 `j` 를 누르면 다음 줄의 첫 화면 행에 선다**(ADR-0108).
//
// desiredX 가 화면 행 안에서 센 칸이라 늘 width 보다 작고, `j` 는 그것을 줄 시작에서 센
// 칸으로 읽는다. vim 은 이 칸을 줄 시작에서 세므로 여기서 결과가 갈린다. 잰 값으로 vim 9.1
// 은 offset 25 에 서고 우리는 offset 5 에 선다.
//
// **그대로 두기로 정한 것을 여기서 못 박는다.** 「vim 과 다르다」가 아니라 「이 편집기는
// 이렇게 움직인다」라서, 표현을 바꾸면 이 시험이 먼저 걸려야 한다.
func TestDesiredColIsRowRelativeAcrossLines(t *testing.T) {
	const width = 20

	long := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMN" // 50 글자, 화면 행 셋
	buf := newBuffer("test.txt", []byte(long+"\n"+long+"\n"))

	// 첫 줄 offset 25 는 둘째 화면 행(행 시작 20) 의 6 번째 칸이다.
	buf.cursor.Line = 0
	buf.cursor.Col = 25
	buf.updateDesiredCol(width)
	require.Equal(t, 5, buf.desiredX, "행 시작을 뺀 칸이다")
	require.Equal(t, byte('z'), buf.lines[0][25])

	buf.moveDownLine(1)

	assert.Equal(t, 1, buf.cursor.Line)
	assert.Equal(t, 5, buf.cursor.Col, "줄 시작에서 5 칸. vim 은 25 로 간다")
	assert.Equal(t, byte('f'), buf.lines[1][buf.cursor.Col])

	offsets := wrapOffsets(buf.lines[1], width, buf.tabWidth())
	assert.Equal(t, 0, rowIndexAt(offsets, buf.cursor.Col), "언제나 첫 화면 행이다")

	// 셋째 행에서 눌렀으면 두 행 몫이 당겨진다. 같은 규칙의 더 센 모습이다.
	buf.cursor.Line = 0
	buf.cursor.Col = 45
	buf.updateDesiredCol(width)
	require.Equal(t, 5, buf.desiredX, "셋째 행(행 시작 40) 의 6 번째 칸")

	buf.moveDownLine(1)
	assert.Equal(t, 5, buf.cursor.Col, "행이 달라져도 같은 자리로 온다")
}

// wrap 된 줄이 화면을 넘으면 그 줄 중간부터 그려야 한다.
func TestScrollToWithinWrappedLine(t *testing.T) {
	width, height := 4, 3
	buf := newBuffer("test.txt", []byte(strings.Repeat("x", 40)+"\n"))
	require.Len(t, wrapOffsets(buf.lines[0], width, defaultTabWidth), 10)

	buf.moveDownRow(2, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 0, buf.top.row, "화면 안이면 안 움직인다")

	buf.moveDownRow(1, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 0, buf.top.line, "같은 줄이다")
	assert.Equal(t, 1, buf.top.row, "행 하나만 밀린다")

	buf.moveDownRow(5, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 6, buf.top.row)

	buf.moveUpRow(4, width)
	buf.scrollTo(width, height)
	assert.Equal(t, 4, buf.top.row, "위로 벗어나면 커서 행이 최상단")
}

// 화면이 넓어지면 그 줄의 wrap 행 수가 줄어서 예전 topRow 가 없는 행을 가리키게 된다.
// 그대로 두면 visibleRows 가 그 줄을 통째로 건너뛰어서 화면이 조용히 밀린다.
//
// 커서가 top 보다 아래 줄에 있어야 재현된다. 커서가 top 보다 위면 scrollTo 의
// "위로 벗어났으면" 갈래가 top 을 커서 자리로 새로 잡아서 우연히 나아버린다.
func TestScrollToClampsTopRowWhenWidened(t *testing.T) {
	narrow, wide, height := 4, 40, 3
	buf := newBuffer("test.txt", []byte(strings.Repeat("x", 40)+"\na\nb\nc\n"))
	require.Len(t, wrapOffsets(buf.lines[0], narrow, defaultTabWidth), 10)
	require.Len(t, wrapOffsets(buf.lines[0], wide, defaultTabWidth), 1, "넓히면 한 행으로 준다")

	// 긴 줄 끝까지 내려가서 그 줄 깊숙이 스크롤한 뒤, 아래 줄들로 커서를 옮긴다.
	buf.moveDownRow(9, narrow)
	buf.scrollTo(narrow, height)
	buf.moveDownRow(1, narrow)
	buf.scrollTo(narrow, height)
	buf.moveDownRow(1, narrow)
	buf.scrollTo(narrow, height)

	require.Equal(t, 2, buf.cursor.Line, "커서는 top 보다 아래 줄")
	require.Equal(t, 0, buf.top.line)
	require.Equal(t, 9, buf.top.row, "긴 줄의 마지막 행부터 그리고 있다")

	buf.scrollTo(wide, height)

	assert.Equal(t, 0, buf.top.line)
	assert.Equal(t, 0, buf.top.row, "넓어진 뒤에는 그 줄에 행이 하나뿐이다")

	rows := buf.visibleRows(wide, height)
	require.NotEmpty(t, rows)
	assert.Equal(t, 0, rows[0].line, "첫 줄이 통째로 사라지면 안 된다")
}

// 줄이 지워져서 top 이 파일 끝을 넘어가도 죽지 않아야 한다.
func TestScrollToClampsTopBeyondEnd(t *testing.T) {
	width, height := 10, 3
	buf := newBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))

	buf.top.line, buf.top.row = 4, 0
	buf.lines = buf.lines[:2]
	buf.cursor.Line, buf.cursor.Col = 0, 0

	assert.NotPanics(t, func() { buf.scrollTo(width, height) })
	assert.Less(t, buf.top.line, len(buf.lines))
}

func TestCursorScreenPos(t *testing.T) {
	width, height := 4, 5
	buf := newBuffer("test.txt", []byte("abcdefgh\nnext\n"))

	x, y, ok := buf.cursorScreenPos(width, height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)

	// 두 번째 행 시작
	buf.moveDownRow(1, width)
	x, y, ok = buf.cursorScreenPos(width, height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 1, y, "wrap 된 행도 화면 행을 차지한다")

	// 다음 줄
	buf.moveDownRow(1, width)
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
			size, width := glyphAt([]byte(test.text), 0, 0, defaultTabWidth)

			assert.Equal(t, test.size, size, "글자 하나의 byte 길이")
			assert.Equal(t, test.width, width, "화면 폭")
			assert.Equal(t, test.width, widthOf(test.text))
		})
	}
}

// 깨진 UTF-8 에서도 진행해야 한다. 0 을 돌려주면 무한 반복이다.
func TestClusterAtInvalidUTF8(t *testing.T) {
	size, _ := glyphAt([]byte{0xff, 0xfe}, 0, 0, defaultTabWidth)
	assert.Positive(t, size)

	assert.NotPanics(t, func() {
		buf := newBuffer("test.txt", []byte{0xff, 0xfe, '\n'})
		buf.moveRight(1, wide)
		buf.moveRight(1, wide)
		buf.moveLeft(1, wide)
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

			buf.moveRight(1, wide)
			assert.Equal(t, test.size, buf.cursor.Col, "글자 하나를 통째로 건너뛴다")

			buf.moveRight(1, wide)
			assert.Equal(t, test.size+1, buf.cursor.Col)

			buf.moveLeft(1, wide)
			assert.Equal(t, test.size, buf.cursor.Col)

			buf.moveLeft(1, wide)
			assert.Equal(t, 0, buf.cursor.Col, "글자 중간이 아니라 시작으로 돌아온다")
		})
	}
}

// 줄바꿈이 결합 문자 중간을 끊으면 화면에 깨진 글자가 나온다.
func TestWrapDoesNotSplitCluster(t *testing.T) {
	emoji := "\U0001F468‍\U0001F469‍\U0001F466" // 18 byte, 2 칸
	buf := newBuffer("test.txt", []byte(strings.Repeat(emoji, 3)+"\n"))

	// 폭 5 면 두 글자(4 칸) 까지만 들어간다
	offsets := wrapOffsets(buf.lines[0], 5, defaultTabWidth)
	assert.Equal(t, []int{0, 36}, offsets, "글자 경계에서만 끊긴다")

	// 폭 6 이면 세 글자가 딱 맞는다
	assert.Equal(t, []int{0}, wrapOffsets(buf.lines[0], 6, defaultTabWidth))
}

// 행 경계를 넘어 왼쪽으로 갈 때도 앞 행의 마지막 글자 시작으로 가야 한다.
func TestMoveLeftAcrossWrappedRowWithClusters(t *testing.T) {
	emoji := "\U0001F468‍\U0001F469‍\U0001F466"
	buf := newBuffer("test.txt", []byte(strings.Repeat(emoji, 3)+"\n"))
	width := 5

	buf.cursor.Col = 36 // 두 번째 행의 시작
	require.Equal(t, 1, rowIndexAt(wrapOffsets(buf.lines[0], width, defaultTabWidth), buf.cursor.Col))

	buf.moveLeft(1, width)
	assert.Equal(t, 18, buf.cursor.Col, "앞 행 마지막 글자의 시작")
}

// tab 은 다음 tab stop 까지 밀어내므로 시작 위치에 따라 폭이 다르다.
func TestClusterAtTab(t *testing.T) {
	tests := []struct {
		col   int
		width int
	}{
		{col: 0, width: 4},
		{col: 1, width: 3},
		{col: 3, width: 1},
		{col: 4, width: 4},
		{col: 5, width: 3},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("col=%d", test.col), func(t *testing.T) {
			size, width := glyphAt([]byte("\t"), 0, test.col, defaultTabWidth)

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
		{name: "줄 앞 tab", line: "\tab", offset: 1, col: 4},
		{name: "tab 뒤 글자", line: "\tab", offset: 2, col: 5},
		{name: "tab 두 개", line: "\t\ta", offset: 2, col: 8},
		{name: "글자 뒤 tab 은 남은 칸만", line: "ab\tc", offset: 3, col: 4},
		{name: "3 칸 뒤 tab 은 1 칸", line: "012\tx", offset: 4, col: 4},
		{name: "4 칸 뒤 tab 은 4 칸", line: "0123\tx", offset: 5, col: 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.col, screenColAt([]byte(test.line), test.offset, defaultTabWidth))
		})
	}
}

// 커서는 tab 을 한 번에 건너뛰고, 화면 칸은 tab stop 을 따라야 한다.
func TestCursorMoveOverTab(t *testing.T) {
	buf := newBuffer("test.txt", []byte("\tab\n"))

	buf.moveRight(1, wide)
	assert.Equal(t, 1, buf.cursor.Col, "tab 은 1 byte")
	assert.Equal(t, 4, screenColAt(buf.lines[0], buf.cursor.Col, defaultTabWidth), "화면 칸은 4")

	buf.moveLeft(1, wide)
	assert.Equal(t, 0, buf.cursor.Col)
}

// tab 이 든 줄도 화면 너비 기준으로 나뉘어야 한다.
func TestWrapWithTab(t *testing.T) {
	// tab(4 칸) + "abcd" 를 너비 6 에 넣으면 ab 까지만 들어간다
	assert.Equal(t, []int{0, 3}, wrapOffsets([]byte("\tabcd"), 6, defaultTabWidth))
}

// moveRowStart, moveRowEnd 는 **화면 행** 안에서 양끝으로 간다. `home` 과 `end` 다.
//
// 줄 단위(`0`·`$`) 와 갈린다 — `↑`·`↓` 가 화면 행을 세는 것과 같은 가름이다(ADR-0076).
func TestMoveRowStartEndStayInTheScreenRow(t *testing.T) {
	// 폭 10 이라 첫 줄이 화면 행 셋이다: 0-9, 10-19, 20-24.
	buf := newBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\nsecond\n"))
	const width = 10

	buf.moveTo(0, 14, width) // 가운데 행

	buf.moveRowStart(width)
	assert.Equal(t, 0, buf.cursor.Line)
	assert.Equal(t, 10, buf.cursor.Col, "지금 행의 앞이다. 줄 맨 앞(0) 이 아니다")

	buf.moveTo(0, 14, width)
	buf.moveRowEnd(width)
	assert.Equal(t, 20, buf.cursor.Col, "지금 행의 끝이다. 줄 맨 끝(25) 이 아니다")
}

// 마지막 행에서 `end` 는 줄 끝이다. 뒤에 이어지는 행이 없다.
func TestMoveRowEndOnLastRowIsLineEnd(t *testing.T) {
	buf := newBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\n"))
	const width = 10

	buf.moveTo(0, 22, width)
	buf.moveRowEnd(width)

	assert.Equal(t, 25, buf.cursor.Col)
}

// 접히지 않은 줄에서는 `0`·`$` 와 같은 자리다.
func TestMoveRowStartEndMatchLineOnUnwrappedLine(t *testing.T) {
	buf := newBuffer("test.txt", []byte("hello world\n"))

	buf.moveTo(0, 5, wide)
	buf.moveRowStart(wide)
	assert.Equal(t, 0, buf.cursor.Col)

	buf.moveRowEnd(wide)
	assert.Equal(t, len("hello world"), buf.cursor.Col)
}

// deleteForward 는 커서 자리 글자를 지운다. insert mode 의 `delete` 다.
func TestDeleteForward(t *testing.T) {
	buf := newBuffer("test.txt", []byte("abc\n"))

	buf.deleteForward(wide)
	assert.Equal(t, "bc", string(buf.lines[0]))
	assert.Equal(t, 0, buf.cursor.Col, "커서는 제자리다")
}

// 한글·이모지도 한 글자로 지운다. lines.Size 가 글자 경계를 준다.
func TestDeleteForwardDeletesWholeCluster(t *testing.T) {
	buf := newBuffer("test.txt", []byte("한글x\n"))

	buf.deleteForward(wide)
	assert.Equal(t, "글x", string(buf.lines[0]), "3 byte 를 한 번에 지운다")
}

// 줄 끝에서는 다음 줄을 끌어올려 붙인다. deleteBackward 가 앞 줄과 합치는 것의 거울이다.
func TestDeleteForwardJoinsNextLine(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\ncd\n"))

	buf.moveLineEnd(1, wide)
	buf.deleteForward(wide)

	require.Len(t, buf.lines, 1)
	assert.Equal(t, "abcd", string(buf.lines[0]))
	assert.Equal(t, 2, buf.cursor.Col, "이은 자리가 곧 커서 자리다")
}

// 마지막 줄 끝에서는 끌어올 것이 없어서 아무 일도 하지 않는다.
func TestDeleteForwardAtEndOfBufferDoesNothing(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\n"))

	buf.moveLineEnd(1, wide)
	buf.deleteForward(wide)

	require.Len(t, buf.lines, 1)
	assert.Equal(t, "ab", string(buf.lines[0]))
	assert.False(t, buf.dirty, "바꾼 것이 없으면 dirty 도 서지 않는다")
}
