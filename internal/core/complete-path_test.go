package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// completeDir 은 완성 시험이 쓸 디렉터리를 만들고 그 안으로 들어간다.
//
// 지금 자리를 옮기는 것은 `:e foo` 처럼 **상대 경로**를 치는 것이 이 기능의 손이기
// 때문이다. `t.Chdir` 이 시험이 끝나면 되돌려 준다.
func completeDir(t *testing.T, names ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range names {
		full := filepath.Join(dir, name)

		if after, ok := strings.CutSuffix(name, "/"); ok {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, after), 0o755))

			continue
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, nil, 0o644))
	}

	t.Chdir(dir)

	return dir
}

func TestCompletePath(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		want     string
		wantList []string
	}{
		{name: "하나면 통째로 채운다", fragment: "buf", want: "buffer.go"},
		{name: "디렉터리면 `/` 를 붙인다", fragment: "int", want: "internal/"},
		{name: "여럿이면 공통 앞부분까지", fragment: "d", want: "doc",
			wantList: []string{"docs/", "docx.md"}},
		{name: "맞는 것이 없으면 그대로", fragment: "zzz", want: "zzz"},
		{name: "빈 조각은 전부가 후보다", fragment: "", want: "",
			wantList: []string{"buffer.go", "docs/", "docx.md", "internal/"}},
		{name: "디렉터리 안으로 들어간다", fragment: "internal/c", want: "internal/core/"},
		{name: "없는 디렉터리는 그대로", fragment: "nope/x", want: "nope/x"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			completeDir(t, "buffer.go", "docs/", "docx.md", "internal/core/", ".hidden")

			filled, list := completePath(test.fragment)

			assert.Equal(t, test.want, filled)
			assert.Equal(t, test.wantList, list)
		})
	}
}

// 숨김 파일은 `.` 을 쳤을 때만 나온다. 셸과 같은 손이다.
func TestCompletePathHidesDotFiles(t *testing.T) {
	completeDir(t, "a.go", "ab.go", ".hidden", ".hidey")

	filled, list := completePath("")
	assert.Equal(t, "a", filled, "숨김을 뺀 둘의 공통 앞부분이다")
	assert.Equal(t, []string{"a.go", "ab.go"}, list, "`.` 을 안 쳤으면 숨김은 없다")

	filled, list = completePath(".")
	assert.Equal(t, ".hid", filled, "`.` 을 쳤으면 그 둘의 공통 앞부분까지 채운다")
	assert.Equal(t, []string{".hidden", ".hidey"}, list)
}

// 한글 이름에서 공통 앞부분이 글자 가운데를 자르지 않는다.
func TestCompletePathKeepsClusters(t *testing.T) {
	completeDir(t, "한글가.md", "한글나.md")

	filled, list := completePath("한")

	assert.Equal(t, "한글", filled, "가·나 앞에서 끊긴다")
	assert.Equal(t, []string{"한글가.md", "한글나.md"}, list)
}

func TestCompleteCommandLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantList []string
	}{
		{name: "tabnew", input: ":tabnew buf", want: ":tabnew buffer.go"},
		{name: "e 도 된다", input: ":e int", want: ":e internal/"},
		{name: "w 도 된다", input: ":w buf", want: ":w buffer.go"},
		{name: "`!` 가 붙어도 된다", input: ":w! buf", want: ":w! buffer.go"},
		{name: "이름 자리는 완성하지 않는다", input: ":tabn", want: ":tabn"},
		{name: "경로를 안 받는 명령은 그대로", input: ":grep buf", want: ":grep buf"},
		{name: "정규식을 받는 명령도 그대로", input: ":s/buf/x/", want: ":s/buf/x/"},
		{name: "따옴표가 있으면 건드리지 않는다", input: `:e "a b`, want: `:e "a b`},
		{name: "여럿이면 공통 앞부분까지", input: ":e d", want: ":e doc",
			wantList: []string{"docs/", "docx.md"}},
		{name: "빈 칸 뒤는 그 디렉터리 전부", input: ":tabnew internal/", want: ":tabnew internal/core/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			completeDir(t, "buffer.go", "docs/", "docx.md", "internal/core/")

			// 명령줄이 드는 것은 `:` 다음부터다. 시험은 읽기 좋게 `:` 를 붙여 적는다.
			filled, list := completeCommandLine(test.input[1:])

			assert.Equal(t, test.want[1:], filled)
			assert.Equal(t, test.wantList, list)
		})
	}
}

// commandAfter 는 명령줄에 키를 넣고 그 model 을 돌려준다.
func commandAfter(t *testing.T, keys ...string) viewEditorCommand {
	t.Helper()

	var m tea.Model = newTestEditor("abc\n", 60, 8)
	m = send(m, keys...)

	here, ok := m.(viewEditorCommand)
	require.True(t, ok, "명령줄에 남아 있어야 한다")

	return here
}

// `tab` 이 명령줄의 마지막 조각을 채운다(ADR-0099).
func TestCommandTabCompletes(t *testing.T) {
	completeDir(t, "buffer.go", "docs/", "docx.md")

	assert.Equal(t, "tabnew buffer.go",
		commandAfter(t, ":", "t", "a", "b", "n", "e", "w", " ", "b", "u", "tab").input.text)
}

// 여럿이면 공통 앞부분까지만 채우고 후보를 든다.
func TestCommandTabShowsCandidates(t *testing.T) {
	completeDir(t, "docs/", "docx.md")

	m := commandAfter(t, ":", "e", " ", "d", "tab")

	assert.Equal(t, "e doc", m.input.text)
	assert.Equal(t, []string{"docs/", "docx.md"}, m.candidates)
	assert.Contains(t, m.renderCandidateBox(), "docx.md", "창에 후보가 선다")
}

// 다음 키에 후보가 사라진다. 글자를 치면 조각이 달라지므로 남아 있으면 어긋난다.
func TestCommandCandidatesClearOnNextKey(t *testing.T) {
	completeDir(t, "docs/", "docx.md")

	typed := commandAfter(t, ":", "e", " ", "d", "tab", "s")
	assert.Empty(t, typed.candidates, "글자를 치면 사라진다")

	erased := commandAfter(t, ":", "e", " ", "d", "tab", "backspace")
	assert.Empty(t, erased.candidates, "지워도 사라진다")
}

// 경로를 안 받는 명령에서 `tab` 은 아무 일도 하지 않는다.
func TestCommandTabIgnoredElsewhere(t *testing.T) {
	completeDir(t, "docs/", "docx.md")

	m := commandAfter(t, ":", "g", "r", "e", "p", " ", "d", "tab")

	assert.Equal(t, "grep d", m.input.text, "정규식을 경로로 맞추지 않는다")
	assert.Empty(t, m.candidates)
}

// 후보 창은 명령줄 위에 뜨고 statusBar 를 덮지 않는다.
func TestCommandCandidateBoxSitsAboveTheLine(t *testing.T) {
	completeDir(t, "docs/", "docx.md")

	m := commandAfter(t, ":", "e", " ", "d", "tab")

	rows := strings.Split(m.View().Content, "\n")
	require.GreaterOrEqual(t, len(rows), 3)

	assert.Contains(t, rows[len(rows)-1], ":e doc", "맨 아래는 명령줄이다")
	assert.NotContains(t, rows[len(rows)-1], m.boxChars.vertical, "명령줄을 덮지 않는다")
	assert.Contains(t, strings.Join(rows, "\n"), "docx.md", "창이 화면에 있다")
}

// 낮은 화면에서 창이 statusBar 와 명령줄을 덮지 않는다.
//
// popupPos 는 자리를 잡아 줄 뿐 창이 화면보다 높을 때를 막지 않는다 — 줄 수를 여기서 맞춘다.
func TestCommandCandidateBoxFitsShortScreen(t *testing.T) {
	completeDir(t, "d1", "d2", "d3", "d4", "d5", "d6", "d7", "d8", "d9")

	tests := []struct {
		height int
		want   int
	}{
		{height: 20, want: 8}, // 자리가 넉넉하면 상한까지
		{height: 10, want: 4},
		{height: 8, want: 2},
		{height: 6, want: 0}, // 한 줄도 못 넣으면 안 띄운다
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("height=%d", test.height), func(t *testing.T) {
			var m tea.Model = newTestEditor("abc\n", 60, test.height-tablineHeight-statusBarHeight)
			m = send(m, ":", "e", " ", "d", "tab")

			here := m.(viewEditorCommand)
			require.Len(t, here.candidates, 9)
			assert.Equal(t, test.want, here.candidateRows())

			rows := strings.Split(here.View().Content, "\n")
			assert.Contains(t, rows[len(rows)-1], ":e d", "명령줄은 늘 살아 있다")
			assert.NotContains(t, rows[len(rows)-1], here.boxChars.vertical)

			// 창이 화면보다 높으면 아래로 넘쳐 statusBar 를 덮는다.
			assert.LessOrEqual(t, len(rows), test.height, "화면 행 수를 넘지 않는다")

			// statusBar 두 줄을 통째로 비켜야 한다. 아래 줄이 명령줄이고 위 줄은 mode 와
			// 파일 경로다 — 창이 그 위에 앉으면 무엇을 치고 있었는지가 가려진다.
			assert.NotContains(t, rows[len(rows)-2], here.boxChars.bottomLeft,
				"statusBar 윗줄을 덮지 않는다")
		})
	}
}

// 창을 얹은 화면의 모든 행이 화면 폭 그대로여야 한다.
//
// 합성기가 만든 층이 한 칸이라도 어긋나면 그 행부터 오른쪽이 밀린다. 터미널 캡처는 빈 칸을
// tab 으로 압축해서 이것을 잴 수 없으므로(run-zn skill 의 Gotchas) 여기서 잰다.
func TestCommandCandidateBoxKeepsScreenWidth(t *testing.T) {
	completeDir(t, "buffer-case.go", "buffer-cat.go", "buffer-change.go", "buffer-clip.go")

	var m tea.Model = newTestEditor("abc\n", 60, 12)
	m = send(m, ":", "e", " ", "b", "tab")

	here := m.(viewEditorCommand)
	require.NotEmpty(t, here.candidates)

	for i, row := range strings.Split(here.View().Content, "\n") {
		plain := ansi.Strip(row)
		assert.Equal(t, here.width, screenColAt([]byte(plain), len(plain), defaultTabWidth),
			"행 %d: %q", i, plain)
	}
}
