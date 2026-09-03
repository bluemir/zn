package textarea

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 권한 비트가 닫힌 파일은 읽기 전용으로 연다. 정의로 뛰어 열리는 module cache 의 파일이
// 이 모양이다(ADR-0051).
func TestDetectReadOnly(t *testing.T) {
	dir := t.TempDir()

	writable := filepath.Join(dir, "writable.go")
	require.NoError(t, os.WriteFile(writable, []byte("package main\n"), 0644))
	assert.False(t, detectReadOnly(writable))

	locked := filepath.Join(dir, "locked.go")
	require.NoError(t, os.WriteFile(locked, []byte("package main\n"), 0444))
	assert.False(t, detectReadOnly(writable))
	assert.True(t, detectReadOnly(locked))

	// 없는 파일은 새로 만드는 것이라 읽기 전용이 아니다.
	assert.False(t, detectReadOnly(filepath.Join(dir, "없다.go")))

	// 이름 없는 buffer 도 아니다.
	assert.False(t, detectReadOnly(""))

	// 디렉터리는 buffer 가 되지 않지만, 되더라도 이 표시를 붙일 것은 아니다.
	assert.False(t, detectReadOnly(dir))
}

func TestSplitFormatted(t *testing.T) {
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitFormatted([]byte("a\nb\n")))
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitFormatted([]byte("a\nb")),
		"마지막 줄바꿈이 없어도 같다")
	assert.Equal(t, [][]byte{{}}, splitFormatted(nil), "buffer 에는 줄이 적어도 하나 있어야 한다")
}

func TestCountChangedLines(t *testing.T) {
	old := [][]byte{[]byte("a"), []byte("b")}

	assert.Equal(t, 0, countChangedLines(old, old))
	assert.Equal(t, 1, countChangedLines(old, [][]byte{[]byte("a"), []byte("B")}))
	assert.Equal(t, 1, countChangedLines(old, [][]byte{[]byte("a"), []byte("b"), []byte("c")}),
		"늘어난 줄도 달라진 것이다")
}
