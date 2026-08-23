package core

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

func TestOpenBufferMarksReadOnly(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.go")
	require.NoError(t, os.WriteFile(locked, []byte("package main\n"), 0444))

	buf, err := OpenBuffer(locked)
	require.NoError(t, err)
	assert.True(t, buf.readOnly)

	writable := filepath.Join(dir, "writable.go")
	require.NoError(t, os.WriteFile(writable, []byte("package main\n"), 0644))

	buf, err = OpenBuffer(writable)
	require.NoError(t, err)
	assert.False(t, buf.readOnly)
}

// 읽기 전용 파일에서는 고치는 키가 듣지 않고 그 까닭이 아래 줄에 뜬다.
//
// 아무 일도 안 나는 것과 거절하는 것을 가르는 것이 요점이다 — 키가 조용히 먹지 않으면
// 편집기가 고장난 것으로 보인다.
func TestReadOnlyRefusesEditingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0444))

	keys := map[string][]string{
		"x(지우기)":      {"x"},
		"dd(줄 지우기)":   {"d", "d"},
		"cw(바꾸기)":     {"c", "w"},
		"i(넣기)":       {"i"},
		"a(뒤에 넣기)":    {"a"},
		"o(아래 줄)":     {"o"},
		"O(위 줄)":      {"O"},
		"p(붙여넣기)":     {"p"},
		"P(앞에 붙여넣기)":  {"P"},
		"r(글자 바꾸기)":   {"r", "z"},
		">>(들여쓰기)":    {">", ">"},
		"<<(내어쓰기)":    {"<", "<"},
		"==(다시 들여쓰기)": {"=", "="},
		"u(되돌리기)":     {"u"},
		"ctrl+r(다시)":  {"ctrl+r"},
	}

	for name, pressed := range keys {
		t.Run(name, func(t *testing.T) {
			buf, err := OpenBuffer(path)
			require.NoError(t, err)

			before := string(buf.contents())

			e := &editor{buffers: []Buffer{buf}, width: 80, height: 20}
			m := viewEditorNormal{editor: e}

			model := send(m, pressed...)

			assert.Equal(t, before, string(e.activeBuffer().contents()), "파일이 바뀌었다")
			assert.Equal(t, "읽기 전용 파일입니다", e.message)
			assert.False(t, e.activeBuffer().dirty)

			// mode 도 바뀌지 않는다. `i` 가 insert 로 들어가면 다음 글자가 파일로 간다.
			assert.IsType(t, viewEditorNormal{}, model)
		})
	}
}

// 고치지 않는 키는 그대로 듣는다. 읽기 전용은 보는 것을 막지 않는다.
func TestReadOnlyAllowsMovingAndYanking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0444))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{buffers: []Buffer{buf}, width: 80, height: 20}
	m := viewEditorNormal{editor: e}

	send(m, "j", "j")
	assert.Equal(t, 2, e.activeBuffer().cursorLine)
	assert.NotEqual(t, "읽기 전용 파일입니다", e.message)

	send(m, "y", "y")
	assert.Len(t, e.register.lines, 1)
	assert.NotEqual(t, "읽기 전용 파일입니다", e.message)
}

// statusBar 에 읽기 전용임이 보인다. 고치려 하기 전에 알 수 있어야 한다.
func TestReadOnlyShowsInStatusBar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0444))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	// 임시 디렉터리 경로가 길어서 좁은 화면에서는 표시가 잘린다. 여기서 보는 것은 표시가
	// 붙는지라 화면을 넉넉히 준다.
	e := editor{buffers: []Buffer{buf}, width: 300, height: 20}

	assert.Contains(t, e.renderStatusBar("NORMAL", "")[0], "[읽기 전용]")
}
