package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// saveWithNote 는 저장하고 맞춘 문구까지 준다. 이 파일의 시험들이 보는 것이 그 문구다.
func saveWithNote(t *testing.T, path string) (string, string) {
	t.Helper()

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	note, err := buf.Save(nil)
	require.NoError(t, err)

	saved, err := os.ReadFile(path)
	require.NoError(t, err)

	return note, string(saved)
}

func TestSaveTrimsTrailingWhitespace(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\n",
		"a.txt", "첫 줄   \n두 번째\t\n셋째\n")

	note, saved := saveWithNote(t, path)

	assert.Equal(t, "첫 줄\n두 번째\n셋째\n", saved)
	assert.Equal(t, ".editorconfig: 줄끝 공백 2 줄 지움", note)
}

// 적히지 않았거나 거짓이면 그대로 둔다. 「열었을 때 모습을 지킨다」가 여기서는 그대로다.
func TestSaveKeepsTrailingWhitespaceWhenNotAsked(t *testing.T) {
	for _, config := range []string{
		"[*]\ntrim_trailing_whitespace = false\n",
		"[*]\nindent_style = tab\n",
	} {
		t.Run(config, func(t *testing.T) {
			path := withEditorconfig(t, config, "a.txt", "첫 줄   \n")

			note, saved := saveWithNote(t, path)

			assert.Equal(t, "첫 줄   \n", saved)
			assert.Empty(t, note)
		})
	}
}

// 다른 글로브의 규칙은 이 파일에 닿지 않는다. 라이브러리가 고르는 것을 우리가 다시 세지 않는다.
func TestSaveHonoursGlobs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"),
		[]byte("root = true\n\n[*.md]\ntrim_trailing_whitespace = true\n"), 0644))

	md := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(md, []byte("md 줄   \n"), 0644))

	txt := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(txt, []byte("txt 줄   \n"), 0644))

	_, saved := saveWithNote(t, md)
	assert.Equal(t, "md 줄\n", saved)

	note, saved := saveWithNote(t, txt)
	assert.Equal(t, "txt 줄   \n", saved, "글로브가 다르면 닿지 않는다")
	assert.Empty(t, note)
}

func TestSaveInsertsFinalNewline(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ninsert_final_newline = true\n",
		"a.txt", "줄끝이 없다")

	note, saved := saveWithNote(t, path)

	assert.Equal(t, "줄끝이 없다\n", saved)
	assert.Equal(t, ".editorconfig: 마지막 줄바꿈 넣음", note)
}

func TestSaveRemovesFinalNewline(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ninsert_final_newline = false\n",
		"a.txt", "줄끝이 있다\n")

	note, saved := saveWithNote(t, path)

	assert.Equal(t, "줄끝이 있다", saved)
	assert.Equal(t, ".editorconfig: 마지막 줄바꿈 뗌", note)
}

// 이미 그 모습이면 아무 말도 하지 않는다. 저장할 때마다 문구가 뜨면 알림이 뜻을 잃는다.
func TestSaveSaysNothingWhenAlreadyMatching(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\ninsert_final_newline = true\nend_of_line = lf\n",
		"a.txt", "깨끗한 줄\n")

	note, saved := saveWithNote(t, path)

	assert.Equal(t, "깨끗한 줄\n", saved)
	assert.Empty(t, note)
}

func TestSaveConvertsLineEnding(t *testing.T) {
	t.Run("lf 로", func(t *testing.T) {
		path := withEditorconfig(t,
			"[*]\nend_of_line = lf\n",
			"a.txt", "한\r\n둘\r\n")

		note, saved := saveWithNote(t, path)

		assert.Equal(t, "한\n둘\n", saved)
		assert.Equal(t, ".editorconfig: 줄끝 LF 로 맞춤", note)
	})

	t.Run("crlf 로", func(t *testing.T) {
		path := withEditorconfig(t,
			"[*]\nend_of_line = crlf\n",
			"a.txt", "한\n둘\n")

		note, saved := saveWithNote(t, path)

		assert.Equal(t, "한\r\n둘\r\n", saved)
		assert.Equal(t, ".editorconfig: 줄끝 CRLF 로 맞춤", note)
	})

	// `cr` 은 zn 의 줄끝에 없다. 못 맞추는 것을 조용히 LF 로 바꿔 쓰지 않는다.
	t.Run("cr 은 건드리지 않는다", func(t *testing.T) {
		path := withEditorconfig(t,
			"[*]\nend_of_line = cr\n",
			"a.txt", "한\r\n둘\r\n")

		note, saved := saveWithNote(t, path)

		assert.Equal(t, "한\r\n둘\r\n", saved)
		assert.Empty(t, note)
	})
}

// 여럿을 맞췄으면 한 줄에 이어 적는다.
func TestSaveNoteJoinsChanges(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\ninsert_final_newline = true\nend_of_line = crlf\n",
		"a.txt", "한 줄  \n둘")

	note, saved := saveWithNote(t, path)

	assert.Equal(t, "한 줄\r\n둘\r\n", saved)
	assert.Equal(t,
		".editorconfig: 줄끝 공백 1 줄 지움 · 마지막 줄바꿈 넣음 · 줄끝 CRLF 로 맞춤", note)
}

// 저장이 막히면 buffer 를 건드리지 않는다. 「저장하지 못했는데 파일이 달라졌다」가 되면
// 무엇을 잃었는지 셀 수 없다.
func TestSaveRefusedDoesNotFormat(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\n",
		"a.txt", "첫 줄   \n")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	buf.insert([]byte("X"))

	// 밖에서 바뀌었다. `:w` 는 막힌다(ADR-0015).
	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	note, err := buf.Save(nil)
	require.Error(t, err)
	assert.Empty(t, note)

	assert.Equal(t, "X첫 줄   ", string(buf.lines[0]), "줄이 다듬어지지 않았다")
	assert.True(t, buf.dirty)
}

// `.editorconfig` 가 없으면 지금까지와 같다. 읽은 대로 되돌린다.
func TestSaveWithoutEditorconfigKeepsShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("공백이 있다   \r\n줄끝 없음"), 0644))

	note, saved := saveWithNote(t, path)

	assert.Equal(t, "공백이 있다   \r\n줄끝 없음", saved)
	assert.Empty(t, note)
}

// 다듬은 뒤 커서가 줄 밖에 서 있으면 안 된다. trimTrailingSpace 가 당겨 두는 자리다.
func TestSaveTrimPullsCursorIn(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\n",
		"a.txt", "가나다      \n")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	// 공백 위에 커서를 둔다.
	buf.moveTo(0, 12)
	require.Equal(t, 12, buf.cursor.Col)

	_, err = buf.Save(nil)
	require.NoError(t, err)

	assert.Equal(t, len("가나다"), buf.cursor.Col, "잘려나간 자리에 남지 않는다")
}

// 다듬은 것은 `u` 로 되돌아온다. 줄을 고치는 일이라 되돌리기 구간에 들어간다.
func TestSaveTrimIsUndoable(t *testing.T) {
	path := withEditorconfig(t,
		"[*]\ntrim_trailing_whitespace = true\n",
		"a.txt", "가나다   \n")

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	_, err = buf.Save(nil)
	require.NoError(t, err)
	require.Equal(t, "가나다", string(buf.lines[0]))

	buf.applyUndo()

	assert.Equal(t, "가나다   ", string(buf.lines[0]))
}

func TestLineEndingNamed(t *testing.T) {
	got, ok := lineEndingNamed("lf")
	assert.True(t, ok)
	assert.Equal(t, lineEndingLF, got)

	got, ok = lineEndingNamed("CRLF")
	assert.True(t, ok, "값은 대소문자를 가리지 않는다")
	assert.Equal(t, lineEndingCRLF, got)

	_, ok = lineEndingNamed("cr")
	assert.False(t, ok)

	_, ok = lineEndingNamed("")
	assert.False(t, ok)
}

func TestLineEndingName(t *testing.T) {
	assert.Equal(t, "LF", lineEndingLF.name())
	assert.Equal(t, "CRLF", lineEndingCRLF.name())
}

// 읽기 전용 파일은 다듬지 않는다. 쓰기가 실패할 것인데 buffer 만 바뀌면 되돌릴 길이 없다 —
// `u` 도 읽기 전용에서는 거절된다(readonly.go).
func TestReadOnlyFileIsNotFormatted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"),
		[]byte("root = true\n\n[*]\ntrim_trailing_whitespace = true\n"), 0644))

	path := filepath.Join(dir, "locked.txt")
	require.NoError(t, os.WriteFile(path, []byte("가나다   \n"), 0444))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)
	require.True(t, buf.readOnly)

	note, err := buf.Save(nil)

	// 쓰기 자체는 권한 때문에 실패한다. 그 전에 줄이 다듬어지지 않았는지가 요점이다.
	require.Error(t, err)
	assert.Empty(t, note)
	assert.Equal(t, "가나다   ", string(buf.lines[0]))
}
