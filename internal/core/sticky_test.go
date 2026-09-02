package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stickyLinesOf 는 그 자리를 화면 맨 위로 그릴 때 붙는 머리줄들이다.
func stickyLinesOf(t *testing.T, path, data string, line, height int) []int {
	t.Helper()

	buf := newBuffer(path, []byte(data))

	return buf.stickyAt(line, height)
}

// **강조하지 않는 파일에서는 아무 일도 일어나지 않는다.**
//
// 이 문 하나가 기존 시험 전부를 지킨다 — newTestEditor 가 `test.txt` 로 열어서 화면이
// 이 기능을 넣기 전과 한 글자도 다르지 않다.
func TestStickyIsInertWithoutLanguage(t *testing.T) {
	data := "func alpha() {\n\tif a {\n\t\tx()\n\t}\n\ty()\n}\n"

	assert.Nil(t, stickyLinesOf(t, "test.txt", data, 4, 10), "언어를 모르면 붙일 것이 없다")

	m := newTestEditor(strings.Repeat("본문\n", 40), 40, 5)
	m.activeBuffer().top.line = 20

	assert.Equal(t, strings.Repeat("본문\n", 5)[:len("본문\n")*5-1], textOf(t, m),
		"강조 없는 파일의 화면은 예전과 같다")
}

// **이미 닫힌 블록은 붙지 않는다.**
//
// 위로 훑을 때 머리줄만 보고 한계를 낮추면 이 자리가 틀린다 — `s := "text" +` 로 이어지는
// 줄에서 위를 보면 3 번 줄에서 이미 닫힌 `if a {` 가 붙는다. 지나는 줄마다 한계를 낮추면
// 닫는 줄(`\t}`) 이 그 블록과 같은 깊이라 여는 줄이 저절로 걸러진다(ADR-0049).
func TestStickyAtSkipsClosedSibling(t *testing.T) {
	data := "func alpha() {\n" + // 0
		"\tif a {\n" + // 1
		"\t\tx()\n" + // 2
		"\t}\n" + // 3
		"\ts := \"text\" +\n" + // 4
		"\t\t\"more\"\n" // 5

	assert.Equal(t, []int{0}, stickyLinesOf(t, "main.go", data, 5, 10),
		"닫힌 `if a {` 가 붙으면 안 된다")
}

// **raw string 안의 줄은 깊이를 말하지 않는다.**
//
// 왼쪽 끝에 붙여 쓴 SQL 한 줄을 깊이 0 으로 읽으면 그 위의 함수가 통째로 가려진다.
func TestStickyAtKeepsAncestorAcrossRawString(t *testing.T) {
	data := "func f() {\n" + // 0
		"\tconst q = `\n" + // 1
		"SELECT *\n" + // 2
		"`\n" + // 3
		"\ty()\n" // 4

	assert.Equal(t, []int{0}, stickyLinesOf(t, "main.go", data, 4, 10),
		"raw string 이 바깥 함수를 가리면 안 된다")
}

// 함수 안의 빈 줄이 위로 훑기를 끝내면 안 된다.
func TestStickyAtSkipsBlankLine(t *testing.T) {
	data := "func f() {\n\tif a {\n\t\tx()\n\n\t\ty()\n"

	assert.Equal(t, []int{0, 1}, stickyLinesOf(t, "main.go", data, 4, 10),
		"빈 줄이 감싸는 것을 지우면 안 된다")
}

// markdown 은 감싸는 제목이 전부 붙고, 옆 절(형제) 은 붙지 않는다.
func TestStickyAtMarkdownAncestors(t *testing.T) {
	data := "# A\n" + // 0
		"## B\n" + // 1
		"본문\n" + // 2
		"## C\n" + // 3
		"### D\n" + // 4
		"본문\n" // 5

	assert.Equal(t, []int{0, 3, 4}, stickyLinesOf(t, "doc.md", data, 5, 20),
		"지나온 `## B` 는 `## C` 에 밀린다")
}

// **코드펜스 안의 `#` 은 제목이 아니다.** 캐시를 지나서도 그렇다(ADR-0040).
func TestStickyAtMarkdownIgnoresFence(t *testing.T) {
	data := "# 진짜 제목\n" +
		"```py\n" +
		"# 파이썬 주석\n" +
		"x = 1\n" +
		"```\n"

	assert.Equal(t, []int{0}, stickyLinesOf(t, "doc.md", data, 3, 20),
		"펜스 안의 주석이 제목이 되면 안 된다")
}

// **stickyAt 이 스스로 토큰 캐시를 채운다.**
//
// scrollTo 는 Update 에서 돌고 토큰은 View 에서 채워진다. 채우지 않으면 두 쪽이 한 프레임
// 어긋나서 커서가 머리줄 아래에 그려진다.
func TestStickyAtFillsTokenCache(t *testing.T) {
	buf := newBuffer("doc.md", []byte("# A\n## B\n본문\n"))
	require.Equal(t, 0, buf.syntax.valid, "아직 한 번도 안 그렸다")

	assert.Equal(t, []int{0, 1}, buf.stickyAt(2, 20), "그리기 전에도 답한다")
	assert.Greater(t, buf.syntax.valid, 2, "훑은 만큼 캐시가 찼다")
}

// 맨 윗줄과 너무 낮은 화면에서는 붙지 않는다.
func TestStickyAtHasNothingToPin(t *testing.T) {
	data := "# A\n## B\n본문\n"

	assert.Nil(t, stickyLinesOf(t, "doc.md", data, 0, 20), "맨 윗줄 위에는 아무것도 없다")
	assert.Nil(t, stickyLinesOf(t, "doc.md", data, 2, 1), "머리줄에 내줄 자리가 없다")
}

// 자물쇠는 본문 절반이고, 넘치면 **바깥쪽부터** 버린다.
func TestStickyAtCapsAtHalfHeight(t *testing.T) {
	data := "# 1\n## 2\n### 3\n#### 4\n##### 5\n###### 6\n본문\n"

	require.Equal(t, []int{0, 1, 2, 3, 4, 5}, stickyLinesOf(t, "doc.md", data, 6, 20),
		"넉넉하면 여섯이 다 붙는다")
	assert.Equal(t, []int{2, 3, 4, 5}, stickyLinesOf(t, "doc.md", data, 6, 8),
		"넘치면 바깥쪽(`# 1`) 부터 버린다")
}

// 화면 위로 올라가 버린 제목이 맨 윗줄에 붙는다.
func TestStickyShowsAncestorWhenScrolledPast(t *testing.T) {
	data := "# 문서 제목\n## 두째 절\n" + strings.Repeat("본문\n", 40)

	m := newTestEditorFile("doc.md", data, 40, 5)
	m.activeBuffer().top.line = 20
	rows := contentRowsOf(t, m)

	assert.Contains(t, rows[0], "문서 제목", "맨 윗줄에 h1 이 붙는다")
	assert.Contains(t, rows[1], "두째 절", "그 아래에 h2 가 붙는다")
	assert.Len(t, rows, 5, "행 수는 그대로다")
}

// **커서가 머리줄에 덮이지 않는다.** scrollTo 가 그만큼 더 올려 준다.
//
// 커서를 화면 맨 윗줄까지 끌어올린 자리가 요점이다. 그냥 두면 커서가 머리줄 아래에 그려져서
// 누른 자리와 글자가 어긋난다.
func TestStickyNeverCoversCursor(t *testing.T) {
	data := "# A\n## B\n### C\n" + strings.Repeat("본문\n", 40)

	t.Run("커서를 화면 맨 위로 올린다", func(t *testing.T) {
		buf := newBuffer("doc.md", []byte(data))
		buf.top.line, buf.cursor.Line = 30, 30
		buf.scrollTo(10)

		sticky := buf.stickyAt(buf.top.line, 10)
		require.NotEmpty(t, sticky, "감싸는 제목이 있어야 시험이 뜻을 가진다")

		_, y, ok := buf.cursorScreenPos(10)
		require.True(t, ok, "커서가 화면 안이다")
		assert.GreaterOrEqual(t, y, len(sticky), "커서가 머리줄에 덮였다")
	})

	// 키로 커서를 위로 끌고 가도 마찬가지다.
	t.Run("G 뒤에 k 로 거슬러 올라간다", func(t *testing.T) {
		var m tea.Model = newTestEditorFile("doc.md", data, 40, 10)
		m, _ = m.(viewEditorNormal).press("G")
		for range 40 {
			m, _ = m.(viewEditorNormal).press("k")

			editor := m.(viewEditorNormal).editor
			buf := editor.activeBuffer()

			_, y, ok := buf.cursorScreenPos(editor.textHeight())
			require.True(t, ok, "커서가 화면 안이다")
			assert.GreaterOrEqual(t, y, len(buf.stickyAt(buf.top.line, editor.textHeight())),
				"커서가 머리줄에 덮였다")
		}
	})
}

// **scrollTo 가 진동하지 않는다.** 두 번 불러도 같은 자리다.
func TestStickyScrollToConverges(t *testing.T) {
	data := "func alpha() {\n\tif y {\n\t\tp()\n\t\tq()\n"

	buf := newBuffer("main.go", []byte(data))
	buf.cursor.Line = 3
	buf.scrollTo(4)

	top, topRow := buf.top.line, buf.top.row
	buf.scrollTo(4)

	assert.Equal(t, top, buf.top.line, "두 번째 부름이 화면을 또 옮기면 안 된다")
	assert.Equal(t, topRow, buf.top.row)
}

// 편집 영역보다 긴 머리줄도 **한 행**이다. 두 행이 되면 그 아래가 통째로 밀린다.
func TestStickyRowIsOneRow(t *testing.T) {
	data := "# " + strings.Repeat("아주 긴 제목 ", 20) + "\n" + strings.Repeat("본문\n", 40)

	m := newTestEditorFile("doc.md", data, 40, 5)
	m.activeBuffer().top.line = 20
	rows := contentRowsOf(t, m)

	assert.Len(t, rows, 5, "행 수는 그대로다")
	assert.LessOrEqual(t, widthOf(rows[0]), m.textWidth(), "편집 영역을 넘지 않는다")
}

// **줄번호 칸이 넘치지 않는다.**
//
// 상대번호 칸은 화면 높이까지만 잡는데 머리줄은 화면 밖에서 온다. `%*d` 의 폭은 최소라
// 넘치면 그대로 늘어나고, 한 칸이라도 넘치면 행이 편집 영역보다 넓어져서 터미널이 접는다 —
// 그 아래가 통째로 밀리고 sidebar 칸까지 어긋난다(ADR-0049).
//
// **폭이 꽉 찬 머리줄이라야 드러난다.** 짧은 제목은 한 칸 넘쳐도 편집 영역 안에 들어가고,
// 두 칸 글자로 채우면 접히는 자리가 한 칸 모자라서 넘침이 그 틈에 숨는다.
func TestStickyGutterDoesNotOverflow(t *testing.T) {
	data := "# " + strings.Repeat("a", 200) + "\n" + strings.Repeat("본문\n", 300)

	m := newTestEditorFile("doc.md", data, 40, 6)
	buf := m.activeBuffer()
	buf.top.line, buf.cursor.Line = 200, 203

	rows := contentRowsOf(t, m)
	require.Contains(t, rows[0], "aaa", "맨 윗줄이 머리줄이다")

	assert.LessOrEqual(t, widthOf(rows[0]), m.textWidth(),
		"머리줄이 편집 영역보다 넓다 — 상대번호 칸이 넘쳤다")
}
