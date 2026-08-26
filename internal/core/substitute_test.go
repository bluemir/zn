package core

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 아래 기대값은 vim 9.1 에서 같은 명령을 쳐서 확인한 것이다(ADR-0084).
func TestSubstitute(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		line  int
		input string
		want  []string
	}{
		{name: "커서 줄 하나", data: "aaa\naaa\n", input: "s/a/X/", want: []string{"Xaa", "aaa"}},
		{name: "범위를 치지 않으면 커서 줄", data: "aaa\naaa\n", line: 1, input: "s/a/X/", want: []string{"aaa", "Xaa"}},
		{name: "한 줄에 한 번", data: "aaa\n", input: "%s/a/X/", want: []string{"Xaa"}},
		{name: "`g` 는 그 줄 전부", data: "aaa\n", input: "%s/a/X/g", want: []string{"XXX"}},
		{name: "`%` 는 파일 전체", data: "a\nb\na\n", input: "%s/a/X/", want: []string{"X", "b", "X"}},
		{name: "줄 범위", data: "a\na\na\n", input: "1,2s/a/X/", want: []string{"X", "X", "a"}},
		{name: "마지막 구분자를 생략한다", data: "aaa\n", input: "%s/a/X", want: []string{"Xaa"}},
		{name: "바꿀 글이 비면 지운다", data: "abc\n", input: "%s/b//", want: []string{"ac"}},
		{name: "이름과 띄어 쓴다", data: "aaa\n", input: "s /a/X/", want: []string{"Xaa"}},
		{name: "온 이름", data: "aaa\n", input: "substitute/a/X/", want: []string{"Xaa"}},

		// 패턴은 `/` 검색과 같은 RE2 다(ADR-0010).
		{name: "정규식", data: "foo123bar\n", input: `%s/\d+/N/`, want: []string{"fooNbar"}},
		{name: "`i` flag", data: "FOO\n", input: "%s/foo/x/i", want: []string{"x"}},
		{name: "줄 앞은 한 번만 맞는다", data: "abc\n", input: "%s/^/> /g", want: []string{"> abc"}},
		{name: "빈 매칭도 자리마다 센다", data: "abc\n", input: "%s/x*/-/g", want: []string{"-a-b-c-"}},

		// 구분자는 아무 글자나 쓸 수 있다. 경로를 바꿀 때 `/` 를 막지 않는 것이 그 값이다.
		{name: "`#` 구분자", data: "internal/core\n", input: "%s#internal/core#internal/ui#", want: []string{"internal/ui"}},
		{name: "`,` 구분자", data: "a\n", input: "%s,a,X,", want: []string{"X"}},
		{name: "`!` 는 강제가 아니라 구분자다", data: "a\n", input: "%s!a!X!", want: []string{"X"}},
		{name: "막은 구분자는 글자다", data: "a/b\n", input: `%s/a\/b/X/`, want: []string{"X"}},

		// 바꿀 글의 문법이다. vim 의 `&`·`\1` 이고 Go 의 `${1}` 로 옮긴다.
		{name: "`&` 는 잡은 것 전체", data: "abc\n", input: "%s/b/[&]/", want: []string{"a[b]c"}},
		{name: "`\\0` 도 잡은 것 전체", data: "abc\n", input: `%s/b/[\0]/`, want: []string{"a[b]c"}},
		{name: "`\\1` 은 잡은 것", data: "fooBuffer\n", input: `%s/(\w+)Buffer/\1Screen/`, want: []string{"fooScreen"}},
		{name: "잡은 것 여럿", data: "a-b\n", input: `%s/(\w)-(\w)/\2-\1/`, want: []string{"b-a"}},
		{name: "`\\&` 는 `&` 한 글자", data: "a\n", input: `%s/a/\&/`, want: []string{"&"}},
		{name: "`\\\\` 는 `\\` 한 글자", data: "a\n", input: `%s/a/\\/`, want: []string{`\`}},
		{name: "`\\t` 는 tab", data: "a b\n", input: `%s/ /\t/`, want: []string{"a\tb"}},
		{name: "`$` 는 글자다", data: "a\n", input: "%s/a/$1/", want: []string{"$1"}},
		{name: "한글도 넣는다", data: "abc\n", input: "%s/b/한글/", want: []string{"a한글c"}},
		{name: "한글을 찾는다", data: "한글abc\n", input: "%s/한글/x/", want: []string{"xabc"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start := newTestEditor(test.data, 60, 8)
			start.buffers[0].cursorLine = test.line

			m := runCommand(start, test.input)

			require.IsType(t, viewEditorNormal{}, m, barOf(t, m)[1])
			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
		})
	}
}

// 알림 문구다. vim 의 `3 substitutions on 2 lines` 자리이고, 하나여도 그대로 적는다.
func TestSubstituteMessage(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		input string
		want  string
	}{
		{name: "한 곳", data: "a\nb\n", input: "%s/a/X/", want: "1 곳을 1 줄에서 바꿨습니다"},
		{name: "여러 곳 한 줄", data: "aaa\n", input: "%s/a/X/g", want: "3 곳을 1 줄에서 바꿨습니다"},
		{name: "여러 줄", data: "aa\naa\n", input: "%s/a/X/g", want: "4 곳을 2 줄에서 바꿨습니다"},
		{name: "못 찾음", data: "a\n", input: "%s/z/X/", want: "찾을 수 없음: z"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := runCommand(newTestEditor(test.data, 60, 8), test.input)

			assert.Contains(t, barOf(t, m)[1], test.want)
		})
	}
}

// 커서는 마지막으로 바꾼 줄의 첫 비공백이다. 줄로 뛰는 것은 `:5`·`G` 와 같은 규칙이다.
func TestSubstituteCursor(t *testing.T) {
	m := runCommand(newTestEditor("a\nb\n    a\nc\n", 60, 8), "%s/a/X/")

	buf := bufferOf(t, m)
	assert.Equal(t, 2, buf.cursorLine)
	assert.Equal(t, 4, buf.cursorCol)
}

// 되돌리기는 한 구간이다. 한 번의 `u` 로 범위 전체가 돌아온다.
func TestSubstituteUndoIsOneStep(t *testing.T) {
	m := runCommand(newTestEditor("a\na\na\n", 60, 8), "%s/a/X/")
	require.Equal(t, []string{"X", "X", "X"}, linesOf(bufferOf(t, m)))

	m = send(m, "u")

	assert.Equal(t, []string{"a", "a", "a"}, linesOf(bufferOf(t, m)))
}

// 앞의 타이핑에 섞이지 않는다. `u` 한 번에 남의 편집까지 딸려오면 안 된다.
func TestSubstituteIsItsOwnUndoStep(t *testing.T) {
	m := send(newTestEditor("a\n", 60, 8), "i", "z")
	m = runCommand(send(m, "esc"), "%s/a/X/")

	m = send(m, "u")

	assert.Equal(t, []string{"za"}, linesOf(bufferOf(t, m)), "치환만 돌아온다")
}

// 못 찾으면 파일을 건드리지 않는다. 고친 표시가 서지 않는다.
func TestSubstituteWithoutMatchLeavesFileAlone(t *testing.T) {
	m := runCommand(newTestEditor("a\n", 60, 8), "%s/zzz/X/")

	assert.Equal(t, []string{"a"}, linesOf(bufferOf(t, m)))
	assert.False(t, bufferOf(t, m).dirty)
}

// 되돌아갈 앞날(redo) 도 그대로 있어야 한다. 구간을 열기만 해도 그것이 날아간다.
func TestSubstituteWithoutMatchKeepsRedo(t *testing.T) {
	m := send(newTestEditor("a\n", 60, 8), "x", "u")
	require.Equal(t, []string{"a"}, linesOf(bufferOf(t, m)))

	m = runCommand(m, "%s/zzz/X/")
	m = send(m, "ctrl+r")

	assert.Equal(t, []string{""}, linesOf(bufferOf(t, m)))
}

func TestSubstituteErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "댄 것이 없다", input: "s", want: "바꿀 것을 대지 않았습니다"},
		{name: "공백뿐이다", input: "s ", want: "바꿀 것을 대지 않았습니다"},
		{name: "바꿀 글이 없다", input: "%s/a", want: "바꿀 글이 없습니다"},
		{name: "찾을 것이 없다", input: "%s//X/", want: "찾을 것이 없습니다"},
		{name: "잘못된 정규식", input: "%s/(/X/", want: "잘못된 패턴"},
		{name: "모르는 flag", input: "%s/a/X/z", want: "모르는 flag: z"},
		{name: "`\\` 뒤에 글자가 없다", input: `%s/a/X\`, want: "`\\` 뒤에 글자가 없습니다"},
		{name: "없는 줄", input: "9s/a/X/", want: "그런 줄이 없습니다: 9"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := runCommand(newTestEditor("a\nb\nc\n", 60, 8), test.input)

			require.IsType(t, viewEditorNormal{}, m)
			assert.Contains(t, barOf(t, m)[1], test.want)
			assert.Equal(t, []string{"a", "b", "c"}, linesOf(bufferOf(t, m)), "실패했으면 파일을 건드리지 않는다")
		})
	}
}

// 찾은 것이 마지막 검색이 된다. `n` 으로 남은 자리를 훑을 수 있다. vim 과 같다.
func TestSubstituteSetsLastSearch(t *testing.T) {
	var m tea.Model = newTestEditor("aXa\n", 60, 8)

	m = runCommand(m, "%s/a/b/")

	e := m.(viewEditorNormal).editor
	require.NotNil(t, e.search.pattern)
	assert.Equal(t, "a", e.search.input)
	assert.True(t, e.search.highlight)
}

// 읽기 전용 파일은 고치지 않는다. `:y` 와 달리 파일을 건드리는 명령이다(readonly.go).
func TestSubstituteRefusesReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0444))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	e := &editor{buffers: []Buffer{buf}, width: 80, height: 20}

	m := runCommand(viewEditorNormal{editor: e}, "%s/main/x/")

	assert.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, "읽기 전용 파일입니다", e.notice)
	assert.Equal(t, []string{"package main"}, linesOf(*e.activeBuffer()))
}
