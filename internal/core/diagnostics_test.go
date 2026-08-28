package core

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// newDiagnostic 은 서버 없이 진단 하나를 짓는다. 그리는 자리 둘이 줄까지만 보므로 열은 비운다.
func newDiagnostic(line int, severity lsp.Severity, message string) lsp.Diagnostic {
	return lsp.Diagnostic{
		Range:    lsp.Range{Start: lsp.Position{Line: line}},
		Severity: severity,
		Message:  message,
	}
}

// 마커 칸은 진단이 없어도 늘 한 칸이다. 첫 오류가 뜰 때 본문이 밀리면 안 된다.
func TestDiagnosticMarkerColumnAlwaysReserved(t *testing.T) {
	clean := newTestEditor("one\ntwo\n", 40, 2)
	dirty := newTestEditor("one\ntwo\n", 40, 2)
	dirty.buffers[0].setDiagnostics([]lsp.Diagnostic{newDiagnostic(0, lsp.SeverityError, "undefined: x")})

	assert.Equal(t, clean.gutterWidth(), dirty.gutterWidth(), "진단이 생겨도 칸 폭이 그대로다")
	assert.Equal(t, clean.contentWidth(), dirty.contentWidth(), "본문 너비도 그대로다")

	assert.Equal(t, []string{"    1  0 ", "    2  1 "}, gutterOf(t, clean), "진단이 없으면 마커 칸은 빈 칸")
	assert.Equal(t, []string{"✖   1  0 ", "    2  1 "}, gutterOf(t, dirty), "있으면 그 줄에만 마커")
}

// 오류와 경고는 글자가 다르다. 색만으로 가르지 않는다.
func TestDiagnosticMarkerBySeverity(t *testing.T) {
	m := newTestEditor("one\ntwo\nthree\nfour\n", 40, 4)
	m.buffers[0].setDiagnostics([]lsp.Diagnostic{
		newDiagnostic(0, lsp.SeverityError, "undefined: x"),
		newDiagnostic(1, lsp.SeverityWarning, "형식이 맞지 않는다"),
		newDiagnostic(2, lsp.SeverityHint, "곁말"),
	})

	assert.Equal(t, []string{
		"✖   1  0 ",
		"⚠   2  1 ",
		"    3  2 ",
		"    4  3 ",
	}, gutterOf(t, m), "정보·힌트는 그리지 않는다")
}

// 한 줄에 오류와 경고가 같이 있으면 마커는 오류다. 아래 줄의 문구도 같은 것을 가리킨다.
func TestDiagnosticWorstFirstOnOneLine(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 60, 2)
	m.buffers[0].setDiagnostics([]lsp.Diagnostic{
		newDiagnostic(0, lsp.SeverityWarning, "형식이 맞지 않는다"),
		newDiagnostic(0, lsp.SeverityError, "undefined: x"),
	})

	assert.Equal(t, "✖   1  0 ", gutterOf(t, m)[0])
	assert.Contains(t, ansi.Strip(m.renderDiagnostic()), "✖ undefined: x (2)", "심한 것이 문구가 되고 개수가 붙는다")
}

// 마커 색은 테마가 실패·주의라고 부르는 색이다(ANSI 1 과 11).
func TestDiagnosticMarkerColors(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 40, 2)
	m.buffers[0].setDiagnostics([]lsp.Diagnostic{
		newDiagnostic(0, lsp.SeverityError, "undefined: x"),
		newDiagnostic(1, lsp.SeverityWarning, "형식이 맞지 않는다"),
	})

	rows := contentRowsOf(t, m)

	assert.Contains(t, rows[0], "\x1b[31m✖", "오류는 ANSI 1")
	assert.Contains(t, rows[1], "\x1b[93m⚠", "경고는 ANSI 11. 오른쪽 절대 줄번호(ANSI 3) 와 갈린다")
}

// wrap 되어 이어지는 행에는 마커가 서지 않는다. 한 줄에 오류가 하나면 마커도 하나다.
func TestDiagnosticMarkerBlankOnWrappedRows(t *testing.T) {
	m := newTestEditor(strings.Repeat("a", 30)+"\nnext\n", 34, 3)
	require.Positive(t, m.gutterWidth())

	m.buffers[0].setDiagnostics([]lsp.Diagnostic{newDiagnostic(0, lsp.SeverityError, "undefined: x")})

	assert.Equal(t, []string{"✖   1  0 ", "         ", "    2  1 "}, gutterOf(t, m))
}

// 커서가 진단 있는 줄에 서면 아래 줄에 문구가 커서 자리 뒤로 붙는다.
func TestDiagnosticMessageFollowsCursor(t *testing.T) {
	normal := newTestEditor("one\ntwo\n", 60, 2)
	normal.buffers[0].setDiagnostics([]lsp.Diagnostic{newDiagnostic(1, lsp.SeverityError, "undefined: x")})

	var m tea.Model = normal

	// tip 이 오른쪽 끝에 붙어 있으므로(ADR-0061) 줄 전체를 견주지 않는다.
	assert.NotContains(t, barOf(t, m)[1], markerDiagnosticError, "커서가 다른 줄이면 문구가 없다")

	m = send(m, "j")

	assert.Contains(t, barOf(t, m)[1], "2:1  (2 줄)  ✖ undefined: x", "커서 자리 뒤에 붙는다")
}

// 알림이 뜨면 그 줄은 알림 것이다. 진단은 다음 키에 알림이 걷히면 다시 보인다.
func TestDiagnosticMessageYieldsToNotice(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 60, 2)
	m.buffers[0].setDiagnostics([]lsp.Diagnostic{newDiagnostic(0, lsp.SeverityError, "undefined: x")})
	m.notify("저장했습니다")

	assert.Equal(t, "저장했습니다", strings.TrimSpace(barOf(t, m)[1]))
}

// 좁은 화면에서는 줄번호와 함께 마커 칸도 사라진다. 본문이 편집 영역을 다 쓴다.
func TestDiagnosticMarkerHiddenOnNarrowScreen(t *testing.T) {
	m := newTestEditor("abc\n", 20, 3)
	m.buffers[0].setDiagnostics([]lsp.Diagnostic{newDiagnostic(0, lsp.SeverityError, "undefined: x")})

	assert.Zero(t, m.gutterWidth())
	assert.Equal(t, m.textWidth(), m.contentWidth())
	assert.NotContains(t, textOf(t, m), markerDiagnosticError, "마커를 그릴 칸이 없다")
}

// 빈 목록이 오면 마커가 사라진다. 고친 오류가 화면에 남아 있으면 안 된다.
func TestDiagnosticClearedByEmptyList(t *testing.T) {
	m := newTestEditor("one\ntwo\n", 40, 2)
	m.buffers[0].setDiagnostics([]lsp.Diagnostic{newDiagnostic(0, lsp.SeverityError, "undefined: x")})
	require.Equal(t, "✖   1  0 ", gutterOf(t, m)[0])

	m.buffers[0].setDiagnostics(nil)

	assert.Equal(t, []string{"    1  0 ", "    2  1 "}, gutterOf(t, m))
	assert.Empty(t, m.renderDiagnostic())
}
