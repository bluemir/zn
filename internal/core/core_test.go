package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

func TestOpenBuffersWithoutFiles(t *testing.T) {
	buffers, err := openBuffers(nil)
	require.NoError(t, err)

	assert.Empty(t, buffers, "인자가 없으면 tab 하나도 없이 시작한다 — 그 자리가 빈 화면이다")
}

// 같은 파일을 두 번 넘겨도 tab 은 하나여야 한다.
// Buffer 가 둘이면 한쪽에서 저장하는 순간 다른 쪽 편집이 사라진다.
func TestOpenBuffersSkipsDuplicates(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.txt")
	second := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(first, []byte("a\n"), 0644))
	require.NoError(t, os.WriteFile(second, []byte("b\n"), 0644))

	buffers, err := openBuffers([]string{first, second, first})
	require.NoError(t, err)

	require.Len(t, buffers, 2)
	assert.Equal(t, first, buffers[0].Path, "처음 나온 자리에 남는다")
	assert.Equal(t, second, buffers[1].Path)
}

// 표기가 달라도 같은 파일이면 한 번만 연다. tabOf 와 같은 기준이다.
func TestOpenBuffersSkipsSameFileWrittenDifferently(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("a\n"), 0644))

	// filepath.Join 은 경로를 정리해 버리므로 어긋난 표기를 직접 이어 붙인다.
	buffers, err := openBuffers([]string{path, dir + "/./a.txt"})
	require.NoError(t, err)

	require.Len(t, buffers, 1)
	assert.Equal(t, path, buffers[0].Path, "먼저 적힌 경로로 연다")
}

// 없는 파일도 새 파일로 열리므로 중복이면 마찬가지로 하나다.
func TestOpenBuffersSkipsDuplicateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-exist.txt")

	buffers, err := openBuffers([]string{path, path})
	require.NoError(t, err)

	assert.Len(t, buffers, 1)
}

// linesOf 는 buffer 의 줄들을 비교하기 쉽게 문자열로 바꾼다.
func linesOf(buf textarea.Viewport) []string {
	out := make([]string, buf.LineCount())
	for i, line := range buf.AllLines() {
		out[i] = string(line)
	}
	return out
}

// goSource 는 줄 수를 정해 만드는 Go 소스다. 화면보다 긴 파일이 필요한 캐시 시험에서 쓴다.
func goSource(lines int) string {
	out := strings.Builder{}
	out.WriteString("package main\n")
	for i := 1; i < lines; i++ {
		out.WriteString("var x int = 1\n")
	}

	return out.String()
}

// mdSource 는 코드펜스가 든 markdown 이다. 위임된 문맥이 캐시를 지나가는지 보는 데 쓴다.
func mdSource(lines int) string {
	out := strings.Builder{}
	out.WriteString("# 제목\n")
	out.WriteString("```go\n")
	for i := 2; i < lines-1; i++ {
		out.WriteString("var x int = 1\n")
	}
	out.WriteString("```\n")

	return out.String()
}

// toLines 는 시험에서 쓰는 줄 묶음이다.
func toLines(text string) [][]byte {
	out, _ := textarea.SplitLines([]byte(text))

	return out
}

// withEditorconfig 는 `.editorconfig` 와 파일 하나가 든 임시 디렉터리를 만들고 그 파일 경로를 준다.
//
// `root = true` 를 반드시 넣는다. 없으면 라이브러리가 위로 훑어 올라가다 이 저장소의
// `.editorconfig` 를 만나고, 그러면 시험이 자기가 적은 것 말고 다른 것에 매인다.
func withEditorconfig(t *testing.T, config, name, content string) string {
	t.Helper()

	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"),
		[]byte("root = true\n\n"+config), 0644))

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	return path
}

const wide = 1000

// 시작할 때 터미널에게 폭을 grapheme cluster 로 세라고 알린다(mode 2027).
//
// 우리가 이미 그렇게 세고 있다는 것을 알리는 것이다. 알리지 않으면 터미널만 코드포인트
// 단위로 세서 여러 rune 글자에서 어긋난다(ADR-0136).
func TestEnableGraphemeClusteringSendsMode2027(t *testing.T) {
	msg := enableGraphemeClustering()()

	raw, ok := msg.(tea.RawMsg)
	require.True(t, ok, "터미널에 그대로 나가는 것이어야 한다: %T", msg)
	assert.Equal(t, "\x1b[?2027h", raw.Msg)
}
