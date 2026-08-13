package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenBuffersWithoutFiles(t *testing.T) {
	buffers, err := openBuffers(nil)
	require.NoError(t, err)

	require.Len(t, buffers, 1)
	assert.Equal(t, "", buffers[0].path, "이름 없는 빈 buffer 하나로 시작한다")
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
	assert.Equal(t, first, buffers[0].path, "처음 나온 자리에 남는다")
	assert.Equal(t, second, buffers[1].path)
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
	assert.Equal(t, path, buffers[0].path, "먼저 적힌 경로로 연다")
}

// 없는 파일도 새 파일로 열리므로 중복이면 마찬가지로 하나다.
func TestOpenBuffersSkipsDuplicateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-exist.txt")

	buffers, err := openBuffers([]string{path, path})
	require.NoError(t, err)

	assert.Len(t, buffers, 1)
}
