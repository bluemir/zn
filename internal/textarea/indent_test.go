package textarea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/syntax"
)

// writeEditorconfig 는 새 디렉터리에 `.editorconfig` 를 놓고 그 안의 파일 경로를 준다.
//
// `root = true` 를 언제나 붙인다. 붙이지 않으면 라이브러리가 상위로 계속 올라가서, 테스트를
// 돌리는 사람의 홈에 있는 `.editorconfig` 가 답을 바꾼다.
func writeEditorconfig(t *testing.T, body, name string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"),
		[]byte("root = true\n\n"+body), 0o644))

	return filepath.Join(dir, name)
}

func TestEditorconfigDecidesTheUnit(t *testing.T) {
	tests := []struct {
		name string
		body string
		file string
		want string
	}{
		{name: "tab 이라고 적혀 있다", body: "[*]\nindent_style = tab\n",
			file: "a.py", want: "\t"},
		{name: "space 두 칸", body: "[*]\nindent_style = space\nindent_size = 2\n",
			file: "a.go", want: "  "},
		{name: "확장자별로 갈린다", body: "[*]\nindent_style = tab\n" +
			"[*.{js,css}]\nindent_style = space\nindent_size = 4\n",
			file: "a.js", want: "    "},
		{name: "indent_size 가 tab 이면 tab_width 다",
			body: "[*]\nindent_style = space\nindent_size = tab\ntab_width = 3\n",
			file: "a.go", want: "   "},
		{name: "다른 확장자에는 안 걸린다", body: "[*.md]\nindent_style = space\nindent_size = 2\n",
			file: "a.go", want: "\t"},
		{name: "들여쓰기 키가 없으면 물러난다", body: "[*]\ncharset = utf-8\n",
			file: "a.py", want: "    "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeEditorconfig(t, test.body, test.file)

			// 파일 안은 비워 둔다. 잴 것이 없어야 `.editorconfig` 와 언어 기본값이 갈린다.
			assert.Equal(t, test.want, string(resolveIndentUnit(path, syntax.LanguageFor(path), [][]byte{{}})))
		})
	}
}

func TestEditorconfigDecidesTabWidth(t *testing.T) {
	tests := []struct {
		name string
		body string
		file string
		want int
	}{
		{name: "적힌 그대로", body: "[*]\ntab_width = 8\n",
			file: "a.go", want: 8},
		{name: "indent_size 만 적어도 그 값이다", body: "[*]\nindent_style = space\nindent_size = 2\n",
			file: "a.go", want: 2},
		{name: "tab_width 가 indent_size 를 이긴다",
			body: "[*]\nindent_style = space\nindent_size = 2\ntab_width = 8\n",
			file: "a.go", want: 8},
		{name: "확장자별로 갈린다", body: "[*]\ntab_width = 8\n[*.md]\ntab_width = 2\n",
			file: "a.md", want: 2},
		{name: "적힌 것이 없으면 기본값", body: "[*]\ncharset = utf-8\n",
			file: "a.go", want: DefaultTabWidth},
		{name: "0 은 받지 않는다", body: "[*]\ntab_width = 0\n",
			file: "a.go", want: DefaultTabWidth},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, resolveTabWidth(writeEditorconfig(t, test.body, test.file)))
		})
	}
}

// 한 단계를 tab 몇 개로 채우는지가 그 파일의 폭을 따른다.
func TestMakeBlankFollowsTabWidth(t *testing.T) {
	assert.Equal(t, "\t  ", string(makeBlank(10, true, 8)))
	assert.Equal(t, "\t\t  ", string(makeBlank(10, true, 4)))
	assert.Equal(t, "          ", string(makeBlank(10, false, 8)), "space 로 채울 때는 폭과 무관하다")
}

// 이름 없이 열었다가 `:w foo.md` 로 이름이 붙으면 폭도 그 경로가 정한 것으로 간다.
// language 를 다시 고르는 자리와 같은 곳이다(buffer-save.go).
func TestTabWidthFollowsNewName(t *testing.T) {
	dir := filepath.Dir(writeEditorconfig(t, "[*]\ntab_width = 8\n[*.md]\ntab_width = 2\n", "a.go"))

	buf := NewEmptyBuffer("")
	require.Equal(t, DefaultTabWidth, buf.TabWidth(), "이름이 없으면 적힌 것을 찾을 자리가 없다")

	require.NoError(t, buf.SaveTo(filepath.Join(dir, "a.md")))
	assert.Equal(t, 2, buf.TabWidth())
}

// 손으로 지은 Buffer 는 이 칸이 비어 있다. 그대로 넘기면 나머지 연산이 죽는다.
func TestTabWidthFallsBackWhenUnset(t *testing.T) {
	assert.Equal(t, DefaultTabWidth, Buffer{Path: "b.txt"}.TabWidth())
}

func TestMeasureIndentUnit(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "tab 으로 들여쓴 파일", data: "func f() {\n\ta()\n\tb()\n}\n", want: "\t"},
		{name: "space 두 칸", data: "function f() {\n  a();\n  if (x) {\n    b();\n  }\n}\n", want: "  "},
		{name: "space 네 칸", data: "def f():\n    a()\n    if x:\n        b()\n", want: "    "},
		{name: "섞이면 많은 쪽", data: "a\n\tb\n\tc\n  d\n", want: "\t"},
		{name: "들여쓴 줄이 없으면 모른다", data: "a\nb\nc\n", want: ""},
		{name: "공백뿐인 줄은 들여쓰기가 아니다", data: "a\n    \nb\n", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("x", []byte(test.data))
			assert.Equal(t, test.want, string(measureIndentUnit(buf.Lines)))
		})
	}
}

func TestIndentUnitFallsBackToTheLanguage(t *testing.T) {
	// `.editorconfig` 도 없고 잴 것도 없는 자리다. 상위에 남의 `.editorconfig` 가 있으면
	// 답이 달라지므로 빈 것을 하나 놓아 막는다.
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "golang 은 tab", file: "a.go", want: "\t"},
		{name: "makefile 은 tab", file: "Makefile", want: "\t"},
		{name: "python 은 space 네 칸", file: "a.py", want: "    "},
		{name: "js 는 space 두 칸", file: "a.js", want: "  "},
		{name: "모르는 확장자는 tab", file: "a.txt", want: "\t"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeEditorconfig(t, "", test.file)
			assert.Equal(t, test.want, string(resolveIndentUnit(path, syntax.LanguageFor(path), [][]byte{{}})))
		})
	}
}

func TestIndentTextIsDecidedOncePerPath(t *testing.T) {
	// 빈 `.editorconfig` 를 놓아 적힌 것이 없게 만든다. 그래야 재는 쪽이 시험된다.
	path := writeEditorconfig(t, "", "a.js")
	buf := NewBuffer(path, []byte("function f() {\n  a()\n}\n"))

	assert.Equal(t, "  ", string(buf.indentText()), "파일이 space 두 칸이다")

	// 이름이 그대로면 다시 재지 않는다. 줄이 바뀌어도 담아둔 것을 그대로 쓴다.
	buf.Lines = [][]byte{[]byte("x"), []byte("\ty")}
	assert.Equal(t, "  ", string(buf.indentText()))

	// 이름이 바뀌면(`:w foo.go`) 다시 정한다. 지금 줄은 tab 이다.
	buf.Path = filepath.Join(filepath.Dir(path), "b.js")
	assert.Equal(t, "\t", string(buf.indentText()))
}

func TestEditorconfigBeatsMeasuring(t *testing.T) {
	// 이 저장소의 `.editorconfig` 가 `[*] indent_style = tab` 이다. 파일이 space 로 쓰였어도
	// 적힌 것이 이긴다.
	path := writeEditorconfig(t, "[*]\nindent_style = tab\n", "a.js")
	buf := NewBuffer(path, []byte("function f() {\n  a()\n}\n"))

	assert.Equal(t, "\t", string(buf.indentText()))
}

func TestMeasureIndentUnitStopsAtTheLimit(t *testing.T) {
	// 앞쪽 500 줄만 본다. 뒤에 아무리 space 가 많아도 답이 바뀌지 않는다.
	data := strings.Repeat("a\n\tb\n", measureIndentUnitLimit) + strings.Repeat("a\n  b\n", 1000)
	buf := NewBuffer("x", []byte(data))

	assert.Equal(t, "\t", string(measureIndentUnit(buf.Lines)))
}
