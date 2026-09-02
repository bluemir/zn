package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// markerFixture 는 열 줄짜리 파일에 git 마커와 진단을 심어 둔 화면이다.
func markerFixture(t *testing.T) tea.Model {
	t.Helper()

	var m tea.Model = newTestEditorFile("a.go", "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n", 60, 12)

	buf := m.(viewEditorNormal).activeBuffer()

	// 1~3 줄이 한 덩이, 7 줄이 또 한 덩이다. 뛰는 자리는 둘이어야 한다.
	buf.git.marks = map[int]gitLineMark{
		1: gitLineModified,
		2: gitLineModified,
		3: gitLineAdded,
		7: gitLineRemoved,
	}

	buf.setDiagnostics([]diagnostic{
		{line: 2, severity: severityError, message: "x"},
		{line: 3, severity: severityWarning, message: "y"},
		{line: 8, severity: severityError, message: "z"},
	})

	return m
}

// 잇달아 붙은 줄은 한 자리다. 열 줄을 고쳤어도 뛰는 자리는 하나다.
func TestGitChangeLinesGroupsRuns(t *testing.T) {
	buf := markerFixture(t).(viewEditorNormal).activeBuffer()

	assert.Equal(t, []int{1, 7}, buf.gitChangeLines())
}

// 진단은 묶지 않는다. 잇달아 서 있어도 저마다 다른 오류다.
func TestDiagnosticLinesAreNotGrouped(t *testing.T) {
	buf := markerFixture(t).(viewEditorNormal).activeBuffer()

	assert.Equal(t, []int{2, 3, 8}, buf.diagnosticLines())
}

// `]c` 는 다음 자리의 첫 줄로 가고 `[c` 는 앞 자리로 간다.
func TestJumpBetweenChanges(t *testing.T) {
	m := send(markerFixture(t), "]", "c")
	assert.Equal(t, 1, m.(viewEditorNormal).activeBuffer().cursor.Line, "첫 덩이의 첫 줄")

	m = send(m, "]", "c")
	assert.Equal(t, 7, m.(viewEditorNormal).activeBuffer().cursor.Line, "덩이 안을 짚지 않고 다음 덩이로")

	m = send(m, "[", "c")
	assert.Equal(t, 1, m.(viewEditorNormal).activeBuffer().cursor.Line)
}

// 숫자 접두를 받는다. `2]c` 는 둘을 건너뛴다.
func TestJumpBetweenChangesTakesCount(t *testing.T) {
	m := send(markerFixture(t), "2", "]", "c")

	assert.Equal(t, 7, m.(viewEditorNormal).activeBuffer().cursor.Line)
}

// 끝에서 감아 돈다. 검색과 같고, 감았으면 아래 줄에 알린다.
func TestJumpBetweenChangesWraps(t *testing.T) {
	m := send(markerFixture(t), "]", "c", "]", "c", "]", "c")

	next := m.(viewEditorNormal)

	assert.Equal(t, 1, next.activeBuffer().cursor.Line, "마지막 다음은 처음이다")
	assert.Equal(t, "아래에서 처음으로 돌아옴", next.notice)
}

// `]d` 는 진단을 한 줄씩 짚는다.
func TestJumpBetweenDiagnostics(t *testing.T) {
	m := send(markerFixture(t), "]", "d")
	assert.Equal(t, 2, m.(viewEditorNormal).activeBuffer().cursor.Line)

	m = send(m, "]", "d")
	assert.Equal(t, 3, m.(viewEditorNormal).activeBuffer().cursor.Line, "잇달아 선 것도 따로 짚는다")

	m = send(m, "[", "d")
	assert.Equal(t, 2, m.(viewEditorNormal).activeBuffer().cursor.Line)
}

// 뛰기 전 자리는 이력에 담긴다. `ctrl+o` 로 돌아온다(ADR-0070).
func TestJumpBetweenChangesRecordsJump(t *testing.T) {
	m := send(markerFixture(t), "5", "G")
	require.Equal(t, 4, m.(viewEditorNormal).activeBuffer().cursor.Line)

	m = send(m, "]", "c")
	require.Equal(t, 7, m.(viewEditorNormal).activeBuffer().cursor.Line)

	m = send(m, "ctrl+o")
	assert.Equal(t, 4, m.(viewEditorNormal).activeBuffer().cursor.Line)
}

// 하나도 없으면 커서를 두고 알리기만 한다. 아무 일도 안 나면 키가 안 먹은 것으로 읽힌다.
func TestJumpWithoutMarkersNotifies(t *testing.T) {
	var m tea.Model = newTestEditorFile("a.go", "one\ntwo\n", 60, 6)

	m = send(m, "j", "]", "c")
	next := m.(viewEditorNormal)

	assert.Equal(t, 1, next.activeBuffer().cursor.Line, "커서는 그대로다")
	assert.Equal(t, "바뀐 자리가 없습니다", next.notice)

	m = send(m, "]", "d")
	assert.Equal(t, "진단이 없습니다", m.(viewEditorNormal).notice)
}

// operator 뒤에는 오지 않는다. `d]c` 는 아무것도 아니다.
func TestBracketKeysAreNotMotions(t *testing.T) {
	m := send(markerFixture(t), "d", "]", "c")

	next := m.(viewEditorNormal)

	assert.Len(t, next.activeBuffer().lines, 10, "줄이 지워지지 않았다")
	assert.Zero(t, next.activeBuffer().cursor.Line, "커서도 그대로다")
}

// 접두 키를 먹는 동안 showcmd 에 그 키가 보인다.
func TestBracketShowsInShowcmd(t *testing.T) {
	m := send(markerFixture(t), "3", "]")

	assert.Equal(t, "3]", m.(viewEditorNormal).keyState().showcmd())
}
