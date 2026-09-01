package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 커서 자리에 들어간다. 뒤에만 붙던 때와 갈리는 것이 이것 하나다.
func TestInputLineInsertsAtCursor(t *testing.T) {
	in := newInputLine("abc")
	in.move("left")
	in.move("left")
	in.insert("X")

	assert.Equal(t, "aXbc", in.text)
	assert.Equal(t, 2, in.cursor, "커서는 넣은 글 뒤다")
}

// `backspace` 는 커서 앞을, `delete` 는 커서 뒤를 지운다. 커서는 지운 자리에 남는다.
func TestInputLineDeletesBothWays(t *testing.T) {
	in := newInputLine("abcd")
	in.move("left")

	in.deleteBackward()
	assert.Equal(t, "abd", in.text)
	assert.Equal(t, 2, in.cursor)

	in.deleteForward()
	assert.Equal(t, "ab", in.text)
	assert.Equal(t, 2, in.cursor, "지운 뒤에도 제자리다")
}

// **글자 단위로 옮긴다.** 한글은 3 byte 이고 이모지는 그보다 길다. byte 로 세면 커서가
// 글자 가운데에 서고, 거기서 지우면 깨진 UTF-8 이 남는다(cluster.go).
func TestInputLineMovesByCluster(t *testing.T) {
	in := newInputLine("한글")
	in.move("left")
	assert.Equal(t, 3, in.cursor)

	in.insert("x")
	assert.Equal(t, "한x글", in.text)

	emoji := newInputLine("a👨‍👩‍👧b")
	emoji.move("left")
	emoji.move("left")
	emoji.deleteForward()

	assert.Equal(t, "ab", emoji.text, "가족 이모지가 통째로 지워진다")
}

// 양끝을 넘어가지 않는다. 맨 앞의 `backspace` 와 맨 뒤의 `delete` 는 아무 일도 하지 않는다.
func TestInputLineStopsAtEnds(t *testing.T) {
	in := newInputLine("ab")

	in.move("home")
	in.move("left")
	assert.Zero(t, in.cursor)

	in.deleteBackward()
	assert.Equal(t, "ab", in.text)

	in.move("end")
	in.move("right")
	assert.Equal(t, 2, in.cursor)

	in.deleteForward()
	assert.Equal(t, "ab", in.text)
}

// 자동완성은 커서 앞만 갈아끼우고 뒤는 그대로 둔다(view-editor-command.go 의 `tab`).
func TestInputLineReplaceHeadKeepsTail(t *testing.T) {
	in := newInputLine("e docs/a.md")
	in.move("left")
	in.move("left")
	in.move("left")

	in.replaceHead("e docs/adr/")

	assert.Equal(t, "e docs/adr/.md", in.text)
	assert.Equal(t, len("e docs/adr/"), in.cursor, "커서는 채워진 끝이다")
}

// **접어도 커서는 보이는 안에 있다.** 늘 오른쪽 끝을 보이던 때는 커서도 늘 끝이라 이것을
// 볼 일이 없었다. 커서를 앞으로 옮기면 접는 자리가 같이 따라와야 한다.
func TestInputLineVisibleFollowsCursor(t *testing.T) {
	in := newInputLine(strings.Repeat("a", 50) + "END")

	text, cursor := in.visible("", 10)
	assert.True(t, strings.HasSuffix(text, "END"), "끝에 있으면 끝이 보인다: %q", text)
	assert.LessOrEqual(t, cursor, 10)

	in.move("home")

	text, cursor = in.visible("", 10)
	assert.True(t, strings.HasPrefix(text, "aaa"), "맨 앞으로 가면 앞이 보인다: %q", text)
	assert.Zero(t, cursor)
	assert.LessOrEqual(t, screenWidthOf(text), 10, "폭을 넘지 않는다")
}

// prefix 는 입력 앞에 늘 붙는 글이라 같이 접히고, 커서 칸도 그만큼 밀린다.
func TestInputLineVisibleCountsPrefix(t *testing.T) {
	in := newInputLine("ab")

	text, cursor := in.visible("옛 → ", 20)

	assert.Equal(t, "옛 → ab", text)
	assert.Equal(t, screenWidthOf("옛 → ab"), cursor)
}

// 한 줄 입력 열이 같은 손을 가진다. 어느 창에서든 `home` 으로 앞에 가서 넣으면 앞에
// 들어가고 `delete` 로 뒤를 지운다.
//
// **열을 한 표로 묶어 본다.** 저마다 자기 Update 를 가지고 있어서 한 곳만 고치고 나머지를
// 잊는 것이 이 과제에서 가장 있을 법한 일이다(ADR-0076 §5 가 「일곱 곳의 그리기와 지우기가
// 다 커서를 봐야 한다」고 미뤄 둔 까닭이 그것이다).
//
// `←`·`→` 는 여기서 보지 않는다. 특수문자 격자만 그 둘을 격자에 남기므로 열이 같지 않다
// (ADR-0113 §2). 그것은 아래 TestSymbolKeepsArrowsForTheGrid 가 따로 본다.
func TestEveryLineInputTakesCursorKeys(t *testing.T) {
	cases := []struct {
		name string
		open func(t *testing.T) tea.Model
		text func(tea.Model) string
	}{
		{
			name: ":",
			open: func(t *testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), ":") },
			text: func(m tea.Model) string { return m.(viewEditorCommand).input.text },
		},
		{
			name: "/",
			open: func(t *testing.T) tea.Model { return send(newTestEditor("abc\n", 80, 10), "/") },
			text: func(m tea.Model) string { return m.(viewEditorSearch).input.text },
		},
		{
			name: "이름 바꾸기",
			open: func(t *testing.T) tea.Model {
				e := newTestEditorFile("a.go", "package p\n\nvar Greet = 1\n", 80, 6).editor
				e.buffers[0].cursorLine, e.buffers[0].cursorCol = 2, 4

				// 옛 이름이 채워진 채로 열린다. 빈 칸에서 시작하도록 지운다.
				m, _ := renameInputMode(e)

				return send(m, "backspace", "backspace", "backspace", "backspace", "backspace")
			},
			text: func(m tea.Model) string { return m.(viewRenameInput).input.text },
		},
		{
			name: "트리 만들기",
			open: func(t *testing.T) tea.Model {
				m := selectTree(t, tea.Model(newTreeEditor(t, 80, 10)), "docs")

				return send(m, "m", "c")
			},
			text: func(m tea.Model) string { return m.(viewSidebarCreate).input.text },
		},
		{
			name: "트리 이름 고치기",
			open: func(t *testing.T) tea.Model {
				m := selectTree(t, tea.Model(newTreeEditor(t, 80, 10)), "README.md")
				m = send(m, "m", "m")

				// 지금 경로가 채워진 채로 열린다(ADR-0054). 빈 칸으로 만든다.
				rename := m.(viewSidebarRename)
				rename.input = inputLine{}

				return rename
			},
			text: func(m tea.Model) string { return m.(viewSidebarRename).input.text },
		},
		{
			name: "grep 검색창",
			open: func(t *testing.T) tea.Model { return newGrepInput(t, 80, 12) },
			text: func(m tea.Model) string { return m.(viewGrepInput).input.text },
		},
		{
			name: "grep 거르기",
			open: func(t *testing.T) tea.Model {
				return send(newGrepView(t, "x", grepHit{path: "a.go", text: "x"}), "/")
			},
			text: func(m tea.Model) string { return m.(viewGrep).filter.text },
		},
		{
			name: "팔레트",
			open: func(t *testing.T) tea.Model {
				m, _ := paletteMode(newTestEditor("abc\n", 80, 20).editor)

				return m
			},
			text: func(m tea.Model) string { return m.(viewPalette).input.text },
		},
		{
			name: "특수문자",
			open: func(t *testing.T) tea.Model {
				m, _ := symbolMode(newTestEditor("abc\n", 80, 20).editor)

				return m
			},
			text: func(m tea.Model) string { return m.(viewSymbol).input.text },
		},
		{
			name: "grep 바꿀 글",
			open: func(t *testing.T) tea.Model {
				return send(newGrepView(t, "x", grepHit{path: "a.go", text: "x"}), "r")
			},
			text: func(m tea.Model) string { return m.(viewGrep).answer.text },
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			m := send(test.open(t), "b", "c")
			require.Equal(t, "bc", test.text(m), "친 그대로 들어간다")

			m = send(m, "home", "a")
			assert.Equal(t, "abc", test.text(m), "`home` 뒤에 친 것이 맨 앞에 들어간다")

			m = send(m, "delete")
			assert.Equal(t, "ac", test.text(m), "`delete` 가 커서 뒤를 지운다")

			m = send(m, "end", "backspace")
			assert.Equal(t, "a", test.text(m), "`end` 뒤의 `backspace` 는 마지막 글자다")

			m = send(m, "home", "end", "home")
			assert.Equal(t, "a", test.text(m), "커서만 옮기는 키는 글을 바꾸지 않는다")
		})
	}
}

// 명령줄의 커서가 그리는 자리와 맞는다. 커서를 앞으로 옮기면 화면 커서도 따라온다.
func TestCommandCursorFollowsInput(t *testing.T) {
	m := send(newTestEditor("abc\n", 80, 10), ":", "a", "b", "c").(viewEditorCommand)

	end := m.View().Cursor.X

	moved := send(m, "left", "left").(viewEditorCommand)

	assert.Equal(t, end-2, moved.View().Cursor.X, "두 칸 왼쪽이다")
	assert.Equal(t, "abc", moved.input.text, "글은 그대로다")
}

// `tab` 은 커서 앞만 완성한다. 커서 뒤에 친 것은 완성할 조각이 아니다.
func TestCommandTabCompletesBeforeCursor(t *testing.T) {
	completeDir(t, "buffer.go", "docs/")

	// `e bu` 를 친 뒤 커서를 한 칸 왼쪽으로 옮긴다. 커서 앞은 `e b` 이므로 완성 대상은
	// `b` 이고, 커서 뒤의 `u` 는 완성에 섞이지 않는다.
	m := commandAfter(t, ":", "e", " ", "b", "u")
	m = send(m, "left", "tab").(viewEditorCommand)

	assert.Equal(t, "e buffer.gou", m.input.text, "커서 뒤가 살아남는다")
	assert.Equal(t, len("e buffer.go"), m.input.cursor, "커서는 채워진 끝이다")
}

// **특수문자 격자만 `←`·`→` 를 지킨다.** 격자에서 옆 칸은 세로 이동으로 대신할 수 없다 —
// 다른 아홉은 목록이 세로로 서 있어서 그 둘이 비어 있었다(ADR-0113 §2).
func TestSymbolKeepsArrowsForTheGrid(t *testing.T) {
	m, _ := symbolMode(newTestEditor("abc\n", 80, 20).editor)

	typed := send(m, "b", "c")
	require.Equal(t, "bc", typed.(viewSymbol).input.text)

	// `home` 은 입력줄 것이라 커서가 앞으로 간다. 거기서 친 것이 맨 앞에 들어간다.
	assert.Equal(t, "abc", send(typed, "home", "a").(viewSymbol).input.text)

	// `←` 는 격자 것이라 커서를 옮기지 않는다. 앞으로 갔으면 아래 넣기가 맨 앞에 들어갔을 것이다.
	assert.Equal(t, "bca", send(typed, "left", "a").(viewSymbol).input.text)
}

// 팔레트는 `←`·`→` 도 입력줄 것이다. 목록이 세로로 서 있어서 그 둘이 비어 있었다.
func TestPaletteTakesArrowsForTheInput(t *testing.T) {
	m, _ := paletteMode(newTestEditor("abc\n", 80, 20).editor)

	typed := send(m, "b", "c")

	assert.Equal(t, "bac", send(typed, "left", "a").(viewPalette).input.text)
}
