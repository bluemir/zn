package textarea

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
func saveBuffer(t *testing.T, buf *Viewport) error {
	t.Helper()

	_, err := buf.Save(nil)

	return err
}

func saveBufferForce(t *testing.T, buf *Viewport) error {
	t.Helper()

	_, err := buf.SaveForce(nil)

	return err
}

func TestNewBuffer(t *testing.T) {
	tests := []struct {
		name            string
		data            string
		Lines           []string
		lineEnding      lineEnding
		finalLineEnding bool
	}{
		{
			name:            "빈 파일은 빈 줄 하나다",
			data:            "",
			Lines:           []string{""},
			lineEnding:      lineEndingLF,
			finalLineEnding: false,
		},
		{
			name:            "줄끝 하나뿐인 파일도 빈 줄 하나다",
			data:            "\n",
			Lines:           []string{""},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
		{
			name:            "LF, 줄끝으로 끝남",
			data:            "a\nb\n",
			Lines:           []string{"a", "b"},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
		{
			name:            "LF, 줄끝 없이 끝남",
			data:            "a\nb",
			Lines:           []string{"a", "b"},
			lineEnding:      lineEndingLF,
			finalLineEnding: false,
		},
		{
			name:            "CRLF 는 줄에서 \\r 이 빠진다",
			data:            "a\r\nb\r\n",
			Lines:           []string{"a", "b"},
			lineEnding:      lineEndingCRLF,
			finalLineEnding: true,
		},
		{
			name:            "CRLF, 줄끝 없이 끝남",
			data:            "a\r\nb",
			Lines:           []string{"a", "b"},
			lineEnding:      lineEndingCRLF,
			finalLineEnding: false,
		},
		{
			name:            "빈 줄이 사이에 있어도 유지된다",
			data:            "a\n\nb\n",
			Lines:           []string{"a", "", "b"},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
		{
			name:            "한글은 그대로 담긴다",
			data:            "한글\n두번째 줄\n",
			Lines:           []string{"한글", "두번째 줄"},
			lineEnding:      lineEndingLF,
			finalLineEnding: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("test.txt", []byte(test.data))

			Lines := make([]string, len(buf.Lines))
			for i, Line := range buf.Lines {
				Lines[i] = string(Line)
			}

			assert.Equal(t, test.Lines, Lines)
			assert.Equal(t, test.lineEnding, buf.lineEnding)
			assert.Equal(t, test.finalLineEnding, buf.finalLineEnding)
		})
	}
}

// lines 는 data 를 가리키는 subslice 여야 한다. 복사하면 큰 파일에서 메모리가 4 배로 뛴다.
func TestNewBufferSharesData(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("hello\nworld\n"))
	require.Len(t, buf.Lines, 2)

	buf.data[0] = 'H'
	assert.Equal(t, "Hello", string(buf.Lines[0]), "lines 가 data 를 가리키지 않고 복사했다")
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
			Path := filepath.Join(t.TempDir(), "test.txt")
			require.NoError(t, os.WriteFile(Path, []byte(test.data), 0644))

			buf, err := OpenBuffer(Path)
			require.NoError(t, err)
			require.NoError(t, saveBuffer(t, &buf))

			saved, err := os.ReadFile(Path)
			require.NoError(t, err)
			assert.Equal(t, test.data, string(saved))
		})
	}
}

func TestOpenBufferMissingFile(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "not-exist.txt")

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	assert.Equal(t, Path, buf.Path)
	assert.Equal(t, []string{""}, []string{string(buf.Lines[0])})
	assert.Len(t, buf.Lines, 1)
}

// 저장할 때 이미 있는 파일의 권한을 떨어뜨리면 안 된다. 실행 스크립트를 열었다 저장하는 경우다.
func TestBufferSaveKeepsFileMode(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "script.sh")
	require.NoError(t, os.WriteFile(Path, []byte("#!/bin/sh\n"), 0755))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)
	require.NoError(t, saveBuffer(t, &buf))

	info, err := os.Stat(Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

// 읽은 뒤에 밖에서 바뀐 파일은 덮어쓰지 않는다. 남의 편집을 조용히 날리지 않기 위해서다.
func TestBufferSaveRefusesWhenFileChangedOutside(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.WriteFile(Path, []byte("남이 쓴 것\n"), 0644))

	err = saveBuffer(t, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "바뀌었습니다")
	assert.True(t, buf.Dirty, "저장되지 않았으므로 변경 표시가 남는다")

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "남이 쓴 것\n", string(after), "파일을 건드리지 않는다")
}

// `:w!` 는 알고도 덮어쓰겠다는 뜻이다.
func TestBufferSaveForceOverwritesChangedFile(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.WriteFile(Path, []byte("남이 쓴 것\n"), 0644))

	require.NoError(t, saveBufferForce(t, &buf))

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(after))
	assert.False(t, buf.Dirty)
}

// 내용이 같으면 밖에서 되쓰였어도 헛경고를 내지 않는다. mtime 이 아니라 내용을 보는 이유다.
func TestBufferSaveAllowsRewriteWithSameContent(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "Xabc\n", string(after))
}

// 저장한 뒤에는 방금 쓴 것이 기준이다. 이어지는 저장이 자기가 쓴 것을 남의 변경으로 보면 안 된다.
func TestBufferSaveTwice(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, saveBuffer(t, &buf))

	buf.Insert([]byte("Y"))
	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "XYabc\n", string(after))
}

// 열 때 없던 파일이 저장 시점에 생겨 있으면 남이 만든 것이다.
func TestBufferSaveRefusesWhenFileAppeared(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "new.txt")

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.WriteFile(Path, []byte("남이 만든 것\n"), 0644))

	err = saveBuffer(t, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "새로 생겼습니다")

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "남이 만든 것\n", string(after))
}

// 열 때도 없었고 지금도 없으면 그냥 새로 만든다.
func TestBufferSaveCreatesNewFile(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "new.txt")

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "X\n", string(after))
}

// 밖에서 지워진 파일도 알린다. 조용히 되살아나면 지운 쪽이 모른다.
func TestBufferSaveRefusesWhenFileRemoved(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.Remove(Path))

	err = saveBuffer(t, &buf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "사라졌습니다")
	assert.NoFileExists(t, Path)

	require.NoError(t, saveBufferForce(t, &buf), "`:w!` 로 다시 만들 수 있다")
	assert.FileExists(t, Path)
}

// 다시 읽으면 바깥 내용이 들어오고 저장하지 않은 변경과 undo 이력은 사라진다 (ADR-0016).
func TestBufferReloadTakesOutsideChange(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.WriteFile(Path, []byte("남이 쓴 것\n"), 0644))

	require.NoError(t, buf.Reload())

	assert.Equal(t, "남이 쓴 것", string(buf.Lines[0]))
	assert.False(t, buf.Dirty)
	assert.Empty(t, buf.undo, "이력은 버린다")
	assert.Empty(t, buf.redo)
}

// 다시 읽은 뒤 바로 저장해도 막히지 않는다. 방금 읽은 것이 새 기준이다.
func TestBufferReloadResetsDiskHash(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(Path, []byte("남이 쓴 것\n"), 0644))
	require.NoError(t, buf.Reload())

	buf.Insert([]byte("X"))
	require.NoError(t, saveBuffer(t, &buf))

	after, err := os.ReadFile(Path)
	require.NoError(t, err)
	assert.Equal(t, "X남이 쓴 것\n", string(after))
}

// 커서는 줄 번호와 화면 칸을 유지한다. byte offset 이 아니라 칸이라 두 칸 글자 중간에 서지 않는다.
func TestBufferReloadKeepsCursorColumn(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\ndef\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.MoveDownLine(1)
	buf.MoveRight(2)
	require.Equal(t, 2, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth))

	// 둘째 줄이 두 칸 글자로 바뀐다. 2 칸은 두 번째 글자의 시작이다.
	require.NoError(t, os.WriteFile(Path, []byte("abc\n한글\n"), 0644))
	require.NoError(t, buf.Reload())

	assert.Equal(t, 1, buf.Cursor.Line)
	assert.Equal(t, 2, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth))
	assert.Equal(t, "글", string(buf.Lines[1][buf.Cursor.Col:]), "글자 경계에 선다")
}

// **다시 읽어도 창이 받은 크기는 그대로다.**
//
// Reload 는 `newBuffer` 로 새 창을 지어 갈아끼우는데, 그 창은 editor 를 지나오지 않아서
// 자기 크기를 모른다. adopt 이 이어받지 않으면 폭이 0 이 되어 다시 읽은 순간 본문이 사라진다
// (ADR-0123).
func TestBufferReloadKeepsPane(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	setContentWidth(&buf, 40, 12)
	want := buf.Size

	require.NoError(t, os.WriteFile(Path, []byte("abcdef\n"), 0644))
	require.NoError(t, buf.Reload())

	assert.Equal(t, want, buf.Size)
	assert.Equal(t, 40, buf.ContentWidth(), "본문 폭이 살아 있다")
}

// 파일이 짧아졌으면 커서를 범위 안으로 끌어온다.
func TestBufferReloadClampsCursorToShorterFile(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("a\nb\nc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.MoveDownLine(2)
	require.Equal(t, 2, buf.Cursor.Line)

	require.NoError(t, os.WriteFile(Path, []byte("a\n"), 0644))
	require.NoError(t, buf.Reload())

	assert.Equal(t, 0, buf.Cursor.Line, "한 줄만 남았으므로 그 줄로 끌려온다")
	assert.Equal(t, 0, buf.Cursor.Col)
}

// 밖에서 지워진 파일은 다시 읽지 않는다. 손에 든 것이 마지막 사본이다.
func TestBufferReloadRefusesWhenFileRemoved(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(Path, []byte("abc\n"), 0644))

	buf, err := OpenBuffer(Path)
	require.NoError(t, err)

	buf.Insert([]byte("X"))
	require.NoError(t, os.Remove(Path))

	err = buf.Reload()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "파일이 없습니다")
	assert.Equal(t, "Xabc", string(buf.Lines[0]), "내용을 건드리지 않는다")
	assert.True(t, buf.Dirty)
}

// 이름 없는 buffer 는 다시 읽을 곳이 없다.
func TestBufferReloadRefusesWithoutPath(t *testing.T) {
	buf := NewEmptyBuffer("")

	err := buf.Reload()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "파일 이름이 없습니다")
}

func TestScreenCol(t *testing.T) {
	tests := []struct {
		Line   string
		offset int
		Col    int
	}{
		{Line: "abc", offset: 0, Col: 0},
		{Line: "abc", offset: 2, Col: 2},
		{Line: "한글", offset: 0, Col: 0},
		{Line: "한글", offset: 3, Col: 2}, // 한 글자 뒤 = 두 칸 뒤
		{Line: "한글", offset: 6, Col: 4},
		{Line: "한a글", offset: 4, Col: 3}, // 한(2) + a(1)
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/offset=%d", test.Line, test.offset), func(t *testing.T) {
			assert.Equal(t, test.Col, ScreenColAt([]byte(test.Line), test.offset, DefaultTabWidth))
		})
	}
}

func TestOffsetAtScreenCol(t *testing.T) {
	tests := []struct {
		name   string
		Line   string
		Col    int
		offset int
	}{
		{name: "ascii", Line: "abc", Col: 2, offset: 2},
		{name: "줄보다 큰 칸은 줄 끝으로", Line: "abc", Col: 99, offset: 3},
		{name: "한글 한 글자 뒤", Line: "한글", Col: 2, offset: 3},
		{name: "두 칸 글자 중간은 글자 시작으로", Line: "한글", Col: 1, offset: 0},
		{name: "한글 뒤 ascii", Line: "한a", Col: 2, offset: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.offset, OffsetAtScreenCol([]byte(test.Line), test.Col, DefaultTabWidth))
		})
	}
}

// 짧은 줄을 지나가도 원래 열로 돌아와야 한다.
func TestCursorKeepsDesiredCol(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("0123456789\nab\n0123456789\n"))

	// 첫 줄에서 오른쪽으로 5 칸
	for range 5 {
		buf.MoveRight(1)
	}
	require.Equal(t, 5, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth))

	// 짧은 줄로 내려가면 줄 끝까지만
	buf.MoveDownRow(1)
	assert.Equal(t, 1, buf.Cursor.Line)
	assert.Equal(t, 2, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth), "짧은 줄에서는 줄 끝")

	// 다시 긴 줄로 내려가면 원래 열로 복귀
	buf.MoveDownRow(1)
	assert.Equal(t, 2, buf.Cursor.Line)
	assert.Equal(t, 5, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth), "긴 줄로 돌아오면 원래 열")
}

func TestCursorMoveClamps(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\ncd\n"))

	buf.MoveUpRow(1)
	assert.Equal(t, 0, buf.Cursor.Line, "첫 줄 위로는 못 간다")

	buf.MoveLeft(1)
	assert.Equal(t, 0, buf.Cursor.Col, "줄 시작 왼쪽으로는 못 간다")

	buf.MoveDownRow(99)
	assert.Equal(t, 1, buf.Cursor.Line, "마지막 줄 아래로는 못 간다")

	buf.MoveRight(1)
	buf.MoveRight(1)
	buf.MoveRight(1)
	assert.Equal(t, 2, buf.Cursor.Col, "줄 끝 오른쪽으로는 못 간다")
}

// moveLeft, moveRight 는 count 만큼 움직이고 줄 양끝에서 멈춘다.
func TestCursorMoveCounted(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("0123456789\n"))

	buf.MoveRight(5)
	assert.Equal(t, 5, buf.Cursor.Col)

	buf.MoveLeft(3)
	assert.Equal(t, 2, buf.Cursor.Col)

	buf.MoveRight(99)
	assert.Equal(t, 10, buf.Cursor.Col, "줄 끝에서 멈춘다")

	buf.MoveLeft(99)
	assert.Equal(t, 0, buf.Cursor.Col, "줄 시작에서 멈춘다")
}

// moveUpLine·moveDownLine 은 논리 줄 단위다. wrap 된 줄도 한 번에 건넌다.
func TestCursorMoveByLine(t *testing.T) {
	// 폭 10 이라 첫 줄이 화면 행 세 개다.
	buf := NewBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\nsecond\nthird\n"))
	setContentWidth(&buf, 10, 10)

	buf.MoveDownLine(1)
	assert.Equal(t, 1, buf.Cursor.Line, "wrap 된 줄을 한 번에 건넌다")

	// 같은 자리에서 화면 행 단위로 내려가면 같은 줄 안에서 행만 옮긴다.
	other := NewBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\nsecond\nthird\n"))
	setContentWidth(&other, 10, 10)
	other.MoveDownRow(1)
	assert.Equal(t, 0, other.Cursor.Line, "moveDown 은 화면 행 단위라 같은 줄에 남는다")

	buf.MoveDownLine(99)
	assert.Equal(t, 2, buf.Cursor.Line, "마지막 줄 아래로는 못 간다")

	buf.MoveUpLine(99)
	assert.Equal(t, 0, buf.Cursor.Line, "첫 줄 위로는 못 간다")
}

// 줄 단위 이동도 짧은 줄을 지나가면 원래 칸으로 돌아온다.
func TestCursorMoveByLineKeepsDesiredCol(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("0123456789\nab\n0123456789\n"))

	buf.MoveRight(5)
	require.Equal(t, 5, buf.Cursor.Col)

	buf.MoveDownLine(1)
	buf.ClampToNormal()
	assert.Equal(t, 1, buf.Cursor.Col, "짧은 줄에서는 마지막 글자 위")

	buf.MoveDownLine(1)
	assert.Equal(t, 5, buf.Cursor.Col, "긴 줄로 돌아오면 원래 칸")
}

// 한글은 rune 하나에 두 칸이다.
func TestCursorMoveHangul(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("한글abc\n"))

	buf.MoveRight(1)
	assert.Equal(t, 3, buf.Cursor.Col, "byte offset 은 3")
	assert.Equal(t, 2, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth), "화면 칸은 2")

	buf.MoveRight(1)
	assert.Equal(t, 6, buf.Cursor.Col)
	assert.Equal(t, 4, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth))

	buf.MoveLeft(1)
	assert.Equal(t, 3, buf.Cursor.Col)
	assert.Equal(t, 2, ScreenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, DefaultTabWidth))
}

// 커서가 화면 안에 있으면 화면은 움직이지 않아야 한다.
func TestScrollTo(t *testing.T) {
	buf := NewBuffer("test.txt", []byte(strings.Repeat("line\n", 100)))
	Height := 10

	buf.MoveDownRow(5)
	buf.ScrollTo(Height)
	assert.Equal(t, 0, buf.Top.Line, "화면 안이면 안 움직인다")

	buf.MoveDownRow(5) // 10 번째 줄, 화면 아래로 한 줄 초과
	buf.ScrollTo(Height)
	assert.Equal(t, 1, buf.Top.Line, "아래로 벗어나면 한 줄만 밀린다")

	buf.MoveDownRow(50)
	buf.ScrollTo(Height)
	assert.Equal(t, 51, buf.Top.Line)

	buf.MoveUpRow(20)
	buf.ScrollTo(Height)
	assert.Equal(t, 40, buf.Top.Line, "위로 벗어나면 커서 줄이 최상단")
}

func TestVisibleRows(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))

	assert.Len(t, buf.VisibleRows(3), 3)
	assert.Len(t, buf.VisibleRows(99), 5, "줄 수보다 큰 화면은 있는 만큼만")

	buf.Top.Line = 3
	assert.Len(t, buf.VisibleRows(99), 2, "top 이후만")
}

func TestWrapOffsets(t *testing.T) {
	tests := []struct {
		name    string
		Line    string
		Width   int
		offsets []int
	}{
		{name: "화면에 들어가는 줄은 한 행", Line: "abcd", Width: 10, offsets: []int{0}},
		{name: "빈 줄도 한 행", Line: "", Width: 10, offsets: []int{0}},
		{name: "딱 맞는 줄은 한 행", Line: "abcd", Width: 4, offsets: []int{0}},
		{name: "넘치면 나뉜다", Line: "abcdef", Width: 4, offsets: []int{0, 4}},
		{name: "여러 번 나뉜다", Line: "abcdefghij", Width: 4, offsets: []int{0, 4, 8}},
		// 한글은 두 칸이므로 너비 5 에는 두 글자(4칸) 까지만 들어간다
		{name: "한글은 두 칸으로 센다", Line: "한글한글", Width: 5, offsets: []int{0, 6}},
		{name: "너비보다 넓은 글자는 한 행에 하나", Line: "한글", Width: 1, offsets: []int{0, 3}},
		{name: "너비 0 이면 나누지 않는다", Line: "abcdef", Width: 0, offsets: []int{0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.offsets, WrapOffsets([]byte(test.Line), test.Width, DefaultTabWidth))
		})
	}
}

// wrap 된 줄 안에서는 아래 이동이 같은 줄의 다음 행으로 가야 한다.
func TestCursorMovesByScreenRow(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abcdefghij\nnext\n"))
	setContentWidth(&buf, 4, 10)

	buf.MoveDownRow(1)
	assert.Equal(t, 0, buf.Cursor.Line, "아직 같은 줄")
	assert.Equal(t, 4, buf.Cursor.Col, "두 번째 행의 시작")

	buf.MoveDownRow(1)
	assert.Equal(t, 0, buf.Cursor.Line)
	assert.Equal(t, 8, buf.Cursor.Col, "세 번째 행의 시작")

	buf.MoveDownRow(1)
	assert.Equal(t, 1, buf.Cursor.Line, "행이 끝나면 다음 줄")
	assert.Equal(t, 0, buf.Cursor.Col)

	buf.MoveUpRow(1)
	assert.Equal(t, 0, buf.Cursor.Line, "위로도 행 단위")
	assert.Equal(t, 8, buf.Cursor.Col)
}

// wrap 된 줄에서 위아래로 움직여도 행 안에서의 칸이 유지되어야 한다.
func TestCursorKeepsDesiredColAcrossWrappedRows(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abcdefghij\n"))
	setContentWidth(&buf, 4, 10)

	buf.MoveRight(1)
	buf.MoveRight(1)
	require.Equal(t, 2, buf.Cursor.Col)
	require.Equal(t, 2, buf.desiredX, "행 안에서 2 칸")

	buf.MoveDownRow(1)
	assert.Equal(t, 6, buf.Cursor.Col, "두 번째 행의 2 칸 = offset 4+2")

	buf.MoveDownRow(1)
	assert.Equal(t, 10, buf.Cursor.Col, "세 번째 행의 2 칸. 줄이 짧아 끝")
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
	const Width = 20

	long := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMN" // 50 글자, 화면 행 셋
	buf := NewBuffer("test.txt", []byte(long+"\n"+long+"\n"))
	setContentWidth(&buf, Width, 10)

	// 첫 줄 offset 25 는 둘째 화면 행(행 시작 20) 의 6 번째 칸이다.
	buf.Cursor.Line = 0
	buf.Cursor.Col = 25
	buf.UpdateDesiredCol()
	require.Equal(t, 5, buf.desiredX, "행 시작을 뺀 칸이다")
	require.Equal(t, byte('z'), buf.Lines[0][25])

	buf.MoveDownLine(1)

	assert.Equal(t, 1, buf.Cursor.Line)
	assert.Equal(t, 5, buf.Cursor.Col, "줄 시작에서 5 칸. vim 은 25 로 간다")
	assert.Equal(t, byte('f'), buf.Lines[1][buf.Cursor.Col])

	offsets := WrapOffsets(buf.Lines[1], Width, buf.TabWidth())
	assert.Equal(t, 0, rowIndexAt(offsets, buf.Cursor.Col), "언제나 첫 화면 행이다")

	// 셋째 행에서 눌렀으면 두 행 몫이 당겨진다. 같은 규칙의 더 센 모습이다.
	buf.Cursor.Line = 0
	buf.Cursor.Col = 45
	buf.UpdateDesiredCol()
	require.Equal(t, 5, buf.desiredX, "셋째 행(행 시작 40) 의 6 번째 칸")

	buf.MoveDownLine(1)
	assert.Equal(t, 5, buf.Cursor.Col, "행이 달라져도 같은 자리로 온다")
}

// wrap 된 줄이 화면을 넘으면 그 줄 중간부터 그려야 한다.
func TestScrollToWithinWrappedLine(t *testing.T) {
	Width, Height := 4, 3
	buf := NewBuffer("test.txt", []byte(strings.Repeat("x", 40)+"\n"))
	require.Len(t, WrapOffsets(buf.Lines[0], Width, DefaultTabWidth), 10)
	setContentWidth(&buf, Width, Height)

	buf.MoveDownRow(2)
	buf.ScrollTo(Height)
	assert.Equal(t, 0, buf.Top.Row, "화면 안이면 안 움직인다")

	buf.MoveDownRow(1)
	buf.ScrollTo(Height)
	assert.Equal(t, 0, buf.Top.Line, "같은 줄이다")
	assert.Equal(t, 1, buf.Top.Row, "행 하나만 밀린다")

	buf.MoveDownRow(5)
	buf.ScrollTo(Height)
	assert.Equal(t, 6, buf.Top.Row)

	buf.MoveUpRow(4)
	buf.ScrollTo(Height)
	assert.Equal(t, 4, buf.Top.Row, "위로 벗어나면 커서 행이 최상단")
}

// 화면이 넓어지면 그 줄의 wrap 행 수가 줄어서 예전 topRow 가 없는 행을 가리키게 된다.
// 그대로 두면 visibleRows 가 그 줄을 통째로 건너뛰어서 화면이 조용히 밀린다.
//
// 커서가 top 보다 아래 줄에 있어야 재현된다. 커서가 top 보다 위면 scrollTo 의
// "위로 벗어났으면" 갈래가 top 을 커서 자리로 새로 잡아서 우연히 나아버린다.
func TestScrollToClampsTopRowWhenWidened(t *testing.T) {
	narrow, wide, Height := 4, 40, 3
	buf := NewBuffer("test.txt", []byte(strings.Repeat("x", 40)+"\na\nb\nc\n"))
	require.Len(t, WrapOffsets(buf.Lines[0], narrow, DefaultTabWidth), 10)
	require.Len(t, WrapOffsets(buf.Lines[0], wide, DefaultTabWidth), 1, "넓히면 한 행으로 준다")

	// 긴 줄 끝까지 내려가서 그 줄 깊숙이 스크롤한 뒤, 아래 줄들로 커서를 옮긴다.
	setContentWidth(&buf, narrow, Height)
	buf.MoveDownRow(9)
	buf.ScrollTo(Height)
	buf.MoveDownRow(1)
	buf.ScrollTo(Height)
	buf.MoveDownRow(1)
	buf.ScrollTo(Height)

	require.Equal(t, 2, buf.Cursor.Line, "커서는 top 보다 아래 줄")
	require.Equal(t, 0, buf.Top.Line)
	require.Equal(t, 9, buf.Top.Row, "긴 줄의 마지막 행부터 그리고 있다")

	// 창을 넓힌다. 앱에서는 resize 가 layoutViews 로 하는 일이다(ADR-0123).
	setContentWidth(&buf, wide, Height)
	buf.ScrollTo(Height)

	assert.Equal(t, 0, buf.Top.Line)
	assert.Equal(t, 0, buf.Top.Row, "넓어진 뒤에는 그 줄에 행이 하나뿐이다")

	rows := buf.VisibleRows(Height)
	require.NotEmpty(t, rows)
	assert.Equal(t, 0, rows[0].Line, "첫 줄이 통째로 사라지면 안 된다")
}

// 줄이 지워져서 top 이 파일 끝을 넘어가도 죽지 않아야 한다.
func TestScrollToClampsTopBeyondEnd(t *testing.T) {
	Height := 3
	buf := NewBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))

	buf.Top.Line, buf.Top.Row = 4, 0
	buf.Lines = buf.Lines[:2]
	buf.Cursor.Line, buf.Cursor.Col = 0, 0

	assert.NotPanics(t, func() { buf.ScrollTo(Height) })
	assert.Less(t, buf.Top.Line, len(buf.Lines))
}

func TestCursorScreenPos(t *testing.T) {
	Height := 5
	buf := NewBuffer("test.txt", []byte("abcdefgh\nnext\n"))
	setContentWidth(&buf, 4, Height)

	x, y, ok := buf.CursorScreenPos(Height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)

	// 두 번째 행 시작
	buf.MoveDownRow(1)
	x, y, ok = buf.CursorScreenPos(Height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 1, y, "wrap 된 행도 화면 행을 차지한다")

	// 다음 줄
	buf.MoveDownRow(1)
	x, y, ok = buf.CursorScreenPos(Height)
	require.True(t, ok)
	assert.Equal(t, 0, x)
	assert.Equal(t, 2, y)
}

// 결합 문자는 rune 여러 개가 한 글자다. 커서 이동과 줄바꿈은 이 단위여야 한다.
func TestClusterAt(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		Size  int
		Width int
	}{
		{name: "ascii", text: "a", Size: 1, Width: 1},
		{name: "NFC 한글", text: "한", Size: 3, Width: 2},
		{name: "NFD 한글", text: "한", Size: 9, Width: 2},
		{name: "e + 결합 악센트", text: "é", Size: 3, Width: 1},
		{name: "ZWJ 가족 이모지", text: "\U0001F468‍\U0001F469‍\U0001F466", Size: 18, Width: 2},
		{name: "국기 이모지", text: "\U0001F1F0\U0001F1F7", Size: 8, Width: 2},
		{name: "피부색 이모지", text: "\U0001F44D\U0001F3FD", Size: 8, Width: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			Size, Width := GlyphAt([]byte(test.text), 0, 0, DefaultTabWidth)

			assert.Equal(t, test.Size, Size, "글자 하나의 byte 길이")
			assert.Equal(t, test.Width, Width, "화면 폭")
			assert.Equal(t, test.Width, WidthOf(test.text))
		})
	}
}

// 깨진 UTF-8 에서도 진행해야 한다. 0 을 돌려주면 무한 반복이다.
func TestClusterAtInvalidUTF8(t *testing.T) {
	Size, _ := GlyphAt([]byte{0xff, 0xfe}, 0, 0, DefaultTabWidth)
	assert.Positive(t, Size)

	assert.NotPanics(t, func() {
		buf := NewBuffer("test.txt", []byte{0xff, 0xfe, '\n'})
		buf.MoveRight(1)
		buf.MoveRight(1)
		buf.MoveLeft(1)
	})
}

// 커서는 결합 문자를 한 번에 건너뛰어야 한다. rune 단위면 글자 중간에 선다.
func TestCursorMovesByCluster(t *testing.T) {
	tests := []struct {
		name string
		text string
		Size int
	}{
		{name: "NFD 한글", text: "한", Size: 9},
		{name: "e + 결합 악센트", text: "é", Size: 3},
		{name: "ZWJ 가족 이모지", text: "\U0001F468‍\U0001F469‍\U0001F466", Size: 18},
		{name: "국기 이모지", text: "\U0001F1F0\U0001F1F7", Size: 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("test.txt", []byte(test.text+"a\n"))

			buf.MoveRight(1)
			assert.Equal(t, test.Size, buf.Cursor.Col, "글자 하나를 통째로 건너뛴다")

			buf.MoveRight(1)
			assert.Equal(t, test.Size+1, buf.Cursor.Col)

			buf.MoveLeft(1)
			assert.Equal(t, test.Size, buf.Cursor.Col)

			buf.MoveLeft(1)
			assert.Equal(t, 0, buf.Cursor.Col, "글자 중간이 아니라 시작으로 돌아온다")
		})
	}
}

// 줄바꿈이 결합 문자 중간을 끊으면 화면에 깨진 글자가 나온다.
func TestWrapDoesNotSplitCluster(t *testing.T) {
	emoji := "\U0001F468‍\U0001F469‍\U0001F466" // 18 byte, 2 칸
	buf := NewBuffer("test.txt", []byte(strings.Repeat(emoji, 3)+"\n"))

	// 폭 5 면 두 글자(4 칸) 까지만 들어간다
	offsets := WrapOffsets(buf.Lines[0], 5, DefaultTabWidth)
	assert.Equal(t, []int{0, 36}, offsets, "글자 경계에서만 끊긴다")

	// 폭 6 이면 세 글자가 딱 맞는다
	assert.Equal(t, []int{0}, WrapOffsets(buf.Lines[0], 6, DefaultTabWidth))
}

// 행 경계를 넘어 왼쪽으로 갈 때도 앞 행의 마지막 글자 시작으로 가야 한다.
func TestMoveLeftAcrossWrappedRowWithClusters(t *testing.T) {
	emoji := "\U0001F468‍\U0001F469‍\U0001F466"
	buf := NewBuffer("test.txt", []byte(strings.Repeat(emoji, 3)+"\n"))
	Width := 5

	buf.Cursor.Col = 36 // 두 번째 행의 시작
	require.Equal(t, 1, rowIndexAt(WrapOffsets(buf.Lines[0], Width, DefaultTabWidth), buf.Cursor.Col))

	buf.MoveLeft(1)
	assert.Equal(t, 18, buf.Cursor.Col, "앞 행 마지막 글자의 시작")
}

// tab 은 다음 tab stop 까지 밀어내므로 시작 위치에 따라 폭이 다르다.
func TestClusterAtTab(t *testing.T) {
	tests := []struct {
		Col   int
		Width int
	}{
		{Col: 0, Width: 4},
		{Col: 1, Width: 3},
		{Col: 3, Width: 1},
		{Col: 4, Width: 4},
		{Col: 5, Width: 3},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("col=%d", test.Col), func(t *testing.T) {
			Size, Width := GlyphAt([]byte("\t"), 0, test.Col, DefaultTabWidth)

			assert.Equal(t, 1, Size, "tab 은 1 byte")
			assert.Equal(t, test.Width, Width)
		})
	}
}

func TestScreenColWithTab(t *testing.T) {
	tests := []struct {
		name   string
		Line   string
		offset int
		Col    int
	}{
		{name: "줄 앞 tab", Line: "\tab", offset: 1, Col: 4},
		{name: "tab 뒤 글자", Line: "\tab", offset: 2, Col: 5},
		{name: "tab 두 개", Line: "\t\ta", offset: 2, Col: 8},
		{name: "글자 뒤 tab 은 남은 칸만", Line: "ab\tc", offset: 3, Col: 4},
		{name: "3 칸 뒤 tab 은 1 칸", Line: "012\tx", offset: 4, Col: 4},
		{name: "4 칸 뒤 tab 은 4 칸", Line: "0123\tx", offset: 5, Col: 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.Col, ScreenColAt([]byte(test.Line), test.offset, DefaultTabWidth))
		})
	}
}

// 커서는 tab 을 한 번에 건너뛰고, 화면 칸은 tab stop 을 따라야 한다.
func TestCursorMoveOverTab(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("\tab\n"))

	buf.MoveRight(1)
	assert.Equal(t, 1, buf.Cursor.Col, "tab 은 1 byte")
	assert.Equal(t, 4, ScreenColAt(buf.Lines[0], buf.Cursor.Col, DefaultTabWidth), "화면 칸은 4")

	buf.MoveLeft(1)
	assert.Equal(t, 0, buf.Cursor.Col)
}

// tab 이 든 줄도 화면 너비 기준으로 나뉘어야 한다.
func TestWrapWithTab(t *testing.T) {
	// tab(4 칸) + "abcd" 를 너비 6 에 넣으면 ab 까지만 들어간다
	assert.Equal(t, []int{0, 3}, WrapOffsets([]byte("\tabcd"), 6, DefaultTabWidth))
}

// moveRowStart, moveRowEnd 는 **화면 행** 안에서 양끝으로 간다. `home` 과 `end` 다.
//
// 줄 단위(`0`·`$`) 와 갈린다 — `↑`·`↓` 가 화면 행을 세는 것과 같은 가름이다(ADR-0076).
func TestMoveRowStartEndStayInTheScreenRow(t *testing.T) {
	// 폭 10 이라 첫 줄이 화면 행 셋이다: 0-9, 10-19, 20-24.
	buf := NewBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\nsecond\n"))
	setContentWidth(&buf, 10, 10)

	buf.MoveTo(0, 14) // 가운데 행

	buf.MoveRowStart()
	assert.Equal(t, 0, buf.Cursor.Line)
	assert.Equal(t, 10, buf.Cursor.Col, "지금 행의 앞이다. 줄 맨 앞(0) 이 아니다")

	buf.MoveTo(0, 14)
	buf.MoveRowEnd()
	assert.Equal(t, 20, buf.Cursor.Col, "지금 행의 끝이다. 줄 맨 끝(25) 이 아니다")
}

// 마지막 행에서 `end` 는 줄 끝이다. 뒤에 이어지는 행이 없다.
func TestMoveRowEndOnLastRowIsLineEnd(t *testing.T) {
	buf := NewBuffer("test.txt", []byte(strings.Repeat("a", 25)+"\n"))
	setContentWidth(&buf, 10, 10)

	buf.MoveTo(0, 22)
	buf.MoveRowEnd()

	assert.Equal(t, 25, buf.Cursor.Col)
}

// 접히지 않은 줄에서는 `0`·`$` 와 같은 자리다.
func TestMoveRowStartEndMatchLineOnUnwrappedLine(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("hello world\n"))

	buf.MoveTo(0, 5)
	buf.MoveRowStart()
	assert.Equal(t, 0, buf.Cursor.Col)

	buf.MoveRowEnd()
	assert.Equal(t, len("hello world"), buf.Cursor.Col)
}

// deleteForward 는 커서 자리 글자를 지운다. insert mode 의 `delete` 다.
func TestDeleteForward(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("abc\n"))

	buf.DeleteForward()
	assert.Equal(t, "bc", string(buf.Lines[0]))
	assert.Equal(t, 0, buf.Cursor.Col, "커서는 제자리다")
}

// 한글·이모지도 한 글자로 지운다. lines.Size 가 글자 경계를 준다.
func TestDeleteForwardDeletesWholeCluster(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("한글x\n"))

	buf.DeleteForward()
	assert.Equal(t, "글x", string(buf.Lines[0]), "3 byte 를 한 번에 지운다")
}

// 줄 끝에서는 다음 줄을 끌어올려 붙인다. deleteBackward 가 앞 줄과 합치는 것의 거울이다.
func TestDeleteForwardJoinsNextLine(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\ncd\n"))

	buf.MoveLineEnd(1)
	buf.DeleteForward()

	require.Len(t, buf.Lines, 1)
	assert.Equal(t, "abcd", string(buf.Lines[0]))
	assert.Equal(t, 2, buf.Cursor.Col, "이은 자리가 곧 커서 자리다")
}

// 마지막 줄 끝에서는 끌어올 것이 없어서 아무 일도 하지 않는다.
func TestDeleteForwardAtEndOfBufferDoesNothing(t *testing.T) {
	buf := NewBuffer("test.txt", []byte("ab\n"))

	buf.MoveLineEnd(1)
	buf.DeleteForward()

	require.Len(t, buf.Lines, 1)
	assert.Equal(t, "ab", string(buf.Lines[0]))
	assert.False(t, buf.Dirty, "바꾼 것이 없으면 dirty 도 서지 않는다")
}

// setContentWidth 는 본문 폭이 want 가 되도록 창의 크기를 정한다. **시험 전용이다.**
//
// 앱에서는 editor 가 편집 영역을 통째로 배정하고 창이 거기서 본문 앞 칸을 뗀다(layoutViews).
// 그런데 창 하나만 세워서 이동과 줄바꿈을 보는 시험은 **본문 폭**을 정하고 싶어 하므로,
// 뗄 만큼을 도로 얹어 준다.
//
// 두 번 재는 것은 앞 칸의 폭이 자리에 따라 달라지기 때문이다 — 좁으면 0 이고 넓으면 아홉
// 칸이라, 얹고 나서 넓어지면 답이 달라진다(ADR-0086, ADR-0123).
func setContentWidth(buf *Viewport, want, height int) {
	buf.Size = ViewSize{Width: want, Height: height}
	buf.Size.Width = want + buf.GutterWidth()
	buf.Size.Width = want + buf.GutterWidth()
}
