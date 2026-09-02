package core

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShiftLinesRight(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "a\n\tb\n\nc\n")

	m = send(m, "V", "j", "j", "j", ">")

	assert.Equal(t, []string{"\ta", "\t\tb", "", "\tc"}, linesOf(bufferOf(t, m)),
		"빈 줄은 건드리지 않는다")
}

func TestShiftLinesLeft(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "\ta\n\t\tb\nc\n")

	m = send(m, "V", "j", "j", "<")

	assert.Equal(t, []string{"a", "\tb", "c"}, linesOf(bufferOf(t, m)),
		"들여쓰기가 없는 줄은 그대로다")
}

func TestShiftLinesLeftWithPartialIndent(t *testing.T) {
	// space 두 칸짜리 파일의 세 칸 줄. 한 단계로 딱 떨어지지 않아도 한 단계만큼 뗀다.
	m := newIndentEditor(t, "a.js", "indent_style = space\nindent_size = 2", "   a\n b\n")

	m = send(m, "V", "j", "<")

	assert.Equal(t, []string{" a", "b"}, linesOf(bufferOf(t, m)))
}

func TestShiftWithMotionAndCount(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []string
	}{
		{name: "`>>` 는 이 줄", keys: []string{">", ">"}, want: []string{"\ta", "b", "c", "d"}},
		{name: "`3>>` 는 세 줄", keys: []string{"3", ">", ">"},
			want: []string{"\ta", "\tb", "\tc", "d"}},
		{name: "`>j` 는 이 줄과 다음 줄", keys: []string{">", "j"},
			want: []string{"\ta", "\tb", "c", "d"}},
		{name: "`>G` 는 끝까지", keys: []string{">", "G"},
			want: []string{"\ta", "\tb", "\tc", "\td"}},
		{name: "`>` 뒤에 이동이 아닌 키면 아무 일도 없다", keys: []string{">", "q"},
			want: []string{"a", "b", "c", "d"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newIndentEditor(t, "a.go", "indent_style = tab", "a\nb\nc\nd\n")

			m = send(m, test.keys...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

func TestShiftPutsCursorOnFirstNonBlank(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "a\nb\n")

	m = send(m, ">", ">")

	buf := bufferOf(t, m)
	assert.Equal(t, 0, buf.cursor.line)
	assert.Equal(t, 1, buf.cursor.col, "들여쓰기 다음")
}

func TestShiftIsOneUndo(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "a\nb\nc\n")

	m = send(m, "3", ">", ">", "u")

	assert.Equal(t, []string{"a", "b", "c"}, linesOf(bufferOf(t, m)))
}

func TestReindentGo(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab",
		"func f() {\nif x {\nfoo()\n}\nbar()\n}\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{
		"func f() {",
		"\tif x {",
		"\t\tfoo()",
		"\t}",
		"\tbar()",
		"}",
	}, linesOf(bufferOf(t, m)))
}

func TestReindentKeepsTheLineAboveTheRange(t *testing.T) {
	// 범위 밖의 줄이 기준이다. 그 줄은 손대지 않는다.
	m := newIndentEditor(t, "a.go", "indent_style = tab",
		"\t\tfunc f() {\nfoo()\n}\n")

	m = send(m, "j", "V", "j", "=")

	assert.Equal(t, []string{"\t\tfunc f() {", "\t\t\tfoo()", "\t\t}"},
		linesOf(bufferOf(t, m)))
}

func TestReindentSwitchCase(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab",
		"switch x {\ncase 1:\nfoo()\ncase 2:\nbar()\n}\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{
		"switch x {",
		"case 1:",
		"\tfoo()",
		"case 2:",
		"\tbar()",
		"}",
	}, linesOf(bufferOf(t, m)), "gofmt 는 case 를 switch 와 나란히 둔다")
}

func TestReindentIgnoresBracketsInStringsAndComments(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab",
		"func f() {\ns := \"{\"\n// {\nfoo()\n}\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"func f() {", "\ts := \"{\"", "\t// {", "\tfoo()", "}"},
		linesOf(bufferOf(t, m)))
}

func TestReindentEmptiesBlankLinesAndCarriesTheBlockAcross(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab",
		"func f() {\nfoo()\n   \nbar()\n}\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"func f() {", "\tfoo()", "", "\tbar()", "}"},
		linesOf(bufferOf(t, m)), "빈 줄이 블록을 끊지 않는다")
}

func TestReindentPython(t *testing.T) {
	m := newIndentEditor(t, "a.py", "indent_style = space\nindent_size = 4",
		"def f():\nif x:\nfoo()\nelse:\nbar()\nreturn 1\nbaz()\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{
		"def f():",
		"    if x:",
		"        foo()",
		"    else:",
		"        bar()",
		"        return 1",
		"    baz()",
	}, linesOf(bufferOf(t, m)))
}

func TestReindentShell(t *testing.T) {
	m := newIndentEditor(t, "a.sh", "indent_style = space\nindent_size = 2",
		"if [ -f x ]; then\nfoo\nfi\nbar\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"if [ -f x ]; then", "  foo", "fi", "bar"},
		linesOf(bufferOf(t, m)))
}

func TestReindentHTML(t *testing.T) {
	m := newIndentEditor(t, "a.html", "indent_style = space\nindent_size = 2",
		"<div>\n<p>글</p>\n<br>\n</div>\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"<div>", "  <p>글</p>", "  <br>", "</div>"},
		linesOf(bufferOf(t, m)))
}

func TestReindentMakefile(t *testing.T) {
	m := newIndentEditor(t, "Makefile", "indent_style = space\nindent_size = 2",
		"build:\ngo build\ngo vet\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"build:", "\tgo build", "\tgo vet"},
		linesOf(bufferOf(t, m)), "조리법은 `.editorconfig` 가 space 라도 tab 이다")
}

func TestReindentDoesNothingForMarkdown(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"- 첫째\n\t- 깊은 것\n    코드 블록\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"- 첫째", "\t- 깊은 것", "    코드 블록"},
		linesOf(bufferOf(t, m)), "목록 깊이와 코드 블록은 앞 줄에서 되짚을 수 없다")
}

// markdown 문서에서 `=` 는 산문을 그대로 두고 코드펜스 안만 정리한다.
//
// 판정이 파일 하나가 아니라 줄마다다(ADR-0102). 산문 줄은 markdown 규칙이 「되짚을 수 없다」고
// 답하고, 펜스 안 줄은 go 규칙이 답한다.
func TestReindentFencedCodeOnly(t *testing.T) {
	m := newIndentEditor(t, "a.md", "indent_style = space\nindent_size = 2",
		"- 첫째\n\t- 깊은 것\n\n```go\nfunc f() {\nfoo()\n}\n```\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{
		"- 첫째",
		"\t- 깊은 것",
		"",
		"```go",
		"func f() {",
		"  foo()",
		"}",
		"```",
	}, linesOf(bufferOf(t, m)), "목록은 그대로, 펜스 안만 정리된다")
}

func TestReindentDoesNothingForUnknownLanguage(t *testing.T) {
	m := newIndentEditor(t, "a.txt", "indent_style = tab", "a\n\t\tb\nc\n")

	m = send(m, "=", "G")

	assert.Equal(t, []string{"a", "\t\tb", "c"}, linesOf(bufferOf(t, m)))
}

func TestReindentIsOneUndo(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\nfoo()\n}\n")

	m = send(m, "=", "G", "u")

	assert.Equal(t, []string{"func f() {", "foo()", "}"}, linesOf(bufferOf(t, m)))
}

func TestReindentDoesNotDirtyWhenNothingChanges(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "func f() {\n\tfoo()\n}\n")
	require.False(t, bufferOf(t, m).dirty)

	m = send(m, "=", "G")

	assert.False(t, bufferOf(t, m).dirty, "이미 맞는 파일에 `=` 를 쳐도 고친 것이 아니다")
}

func TestVisualIndentLeavesVisualMode(t *testing.T) {
	m := newIndentEditor(t, "a.go", "indent_style = tab", "a\nb\n")

	m = send(m, "V", "j", ">")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, []string{"\ta", "\tb"}, linesOf(bufferOf(t, m)))
}

func TestIndentSpanCoversWholeLinesOfACharwiseMotion(t *testing.T) {
	// 범위가 글자 단위여도 걸친 줄 전체가 움직인다. 들여쓰기는 줄의 성질이다.
	m := newIndentEditor(t, "a.go", "indent_style = tab", "one two\nthree\n")

	m = send(m, ">", "w")

	assert.Equal(t, []string{"\tone two", "three"}, linesOf(bufferOf(t, m)))
}

// 자르는 자리는 언제나 줄 앞 공백 안이다. 공백은 ' ' 와 '\t' 뿐이라 글자 가운데를 가를 수 없다.
func TestIndentNeverCutsAMultibyteCharacter(t *testing.T) {
	tests := []struct {
		name  string
		style string
		data  string
		keys  []string
		want  []string
	}{
		{name: "tab 파일에서 space 로 들여쓴 한글 줄을 당긴다",
			style: "indent_style = tab", data: " 한글 이모지 🙂\n",
			keys: []string{"V", "<"}, want: []string{"한글 이모지 🙂"}},
		{name: "공백보다 한 단계가 길어도 글자는 안 건드린다",
			style: "indent_style = space\nindent_size = 4", data: " 한글\n",
			keys: []string{"V", "<"}, want: []string{"한글"}},
		{name: "한글 줄을 민다",
			style: "indent_style = tab", data: "한글 🙂\n",
			keys: []string{"V", ">"}, want: []string{"\t한글 🙂"}},
		{name: "한글이 든 Go 줄을 다시 들여쓴다",
			style: "indent_style = tab", data: "func f() {\nfoo(\"한글 🙂\")\n}\n",
			keys: []string{"=", "G"}, want: []string{"func f() {", "\tfoo(\"한글 🙂\")", "}"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newIndentEditor(t, "a.go", test.style, test.data)

			m = send(m, test.keys...)

			buf := bufferOf(t, m)
			assert.Equal(t, test.want, linesOf(buf))
			for _, line := range buf.lines {
				assert.True(t, utf8.Valid(line), "%q 가 깨졌다", line)
			}
		})
	}
}
