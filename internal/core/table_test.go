package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableBuffer 는 그 글을 담은 markdown buffer 다.
func tableBuffer(t *testing.T, name string, lines ...string) *viewport {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644))

	buf, err := OpenBuffer(path)
	require.NoError(t, err)

	return &buf
}

// bufferLines 는 지금 buffer 의 줄들이다.
func bufferLines(buf *viewport) []string {
	out := make([]string, 0, len(buf.lines))
	for _, line := range buf.lines {
		out = append(out, string(line))
	}

	return out
}

// 칸 폭을 맞추고, 한글은 화면에서 두 칸이라는 것까지 센다.
//
// **byte 나 글자 수로 세면 한글이 든 표가 어긋난다.** 그 자는 터미널에서 재서 맞춰 두었다
// (ADR-0020, ADR-0072).
func TestFormatTablesAlignsColumns(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 항목 | 값 |",
		"|---|---|",
		"| setext 제목 | 없다 |",
	)

	found, changed := buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, 1, found)
	assert.Equal(t, 1, changed)
	assert.Equal(t, []string{
		"| 항목        | 값   |",
		"|-------------|------|",
		"| setext 제목 | 없다 |",
	}, bufferLines(buf))
}

// 구분줄의 `:` 가 말하는 정렬을 지킨다. 적어 두지 않은 칸에 `:` 를 새로 달지 않는다.
func TestFormatTablesKeepsAlignment(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 왼쪽 | 가운데 | 오른쪽 | 없음 |",
		"|:--|:-:|--:|---|",
		"| a | b | c | d |",
	)

	buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, []string{
		"| 왼쪽 | 가운데 | 오른쪽 | 없음 |",
		"|:-----|:------:|-------:|------|",
		"| a    |   b    |      c | d    |",
	}, bufferLines(buf))
}

// 코드펜스 안의 `|` 줄은 표가 아니다. 남의 코드 예시를 고쳐 쓰지 않는다.
func TestFormatTablesSkipsFencedBlock(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"```",
		"| 이것은 | 코드펜스 안이다 |",
		"|---|---|",
		"```",
	)

	found, changed := buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, 0, found)
	assert.Equal(t, 0, changed)
	assert.Equal(t, []string{
		"```",
		"| 이것은 | 코드펜스 안이다 |",
		"|---|---|",
		"```",
	}, bufferLines(buf))
}

// 코드 스팬 안의 `|` 는 칸을 가르지 않는다. 가르면 그 자리에 없던 칸이 생겨 글이 깨진다.
func TestFormatTablesKeepsPipeInCodeSpan(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 명령 | 뜻 |",
		"|---|---|",
		"| `a | b` | 파이프 |",
	)

	buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, []string{
		"| 명령    | 뜻     |",
		"|---------|--------|",
		"| `a | b` | 파이프 |",
	}, bufferLines(buf))
}

// 칸 수가 모자란 행은 빈 칸으로 채운다. 줄 수는 그대로다.
func TestFormatTablesFillsMissingCells(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 하나 | 둘 | 셋 |",
		"|---|---|---|",
		"| a |",
	)

	buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, []string{
		"| 하나 | 둘 | 셋 |",
		"|------|----|----|",
		"| a    |    |    |",
	}, bufferLines(buf))
}

// 이미 맞춰져 있으면 고치지 않는다. 「표가 없다」와 「이미 맞다」는 다른 말이다.
func TestFormatTablesIsIdempotent(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 항목 | 값 |",
		"|---|---|",
		"| a | b |",
	)

	found, changed := buf.formatTables(0, len(buf.lines), 80)
	require.Equal(t, 1, found)
	require.Equal(t, 1, changed)

	before := bufferLines(buf)

	found, changed = buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, 1, found)
	assert.Equal(t, 0, changed, "두 번째는 바꿀 것이 없다")
	assert.Equal(t, before, bufferLines(buf))
}

// markdown 이 아닌 파일에는 표가 없다. 문맥이 그것을 가른다.
func TestFormatTablesOnlyMarkdown(t *testing.T) {
	buf := tableBuffer(t, "main.go",
		"package main",
		"",
		"// | 항목 | 값 |",
		"// |---|---|",
	)

	found, changed := buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, 0, found)
	assert.Equal(t, 0, changed)
}

// 들여쓴 표는 그 들여쓰기를 지킨다. 목록 안의 표가 그렇고 이 저장소의 목록은 tab 이다.
//
// **빈 칸만 떼면 tab 이 칸 글에 딸려 들어가** 첫 칸의 폭이 그만큼 넓어진다.
func TestFormatTablesKeepsIndent(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"- 목록 안의 표",
		"",
		"\t| 항목 | 값 |",
		"\t|---|---|",
		"\t| 표 | 쓴다 |",
	)

	found, changed := buf.formatTables(0, len(buf.lines), 80)

	assert.Equal(t, 1, found)
	assert.Equal(t, 1, changed)
	assert.Equal(t, []string{
		"- 목록 안의 표",
		"",
		"\t| 항목 | 값   |",
		"\t|------|------|",
		"\t| 표   | 쓴다 |",
	}, bufferLines(buf))
}

// 고른 범위에 걸친 표만 맞춘다. 범위 밖의 표는 그대로다 (ADR-0106).
func TestFormatTablesInRange(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 첫 표 | 값 |",
		"|---|---|",
		"| a | b |",
		"",
		"| 둘째 표 | 값 |",
		"|---|---|",
		"| c | d |",
	)

	found, changed := buf.formatTables(4, 7, 80)

	assert.Equal(t, 1, found)
	assert.Equal(t, 1, changed)
	assert.Equal(t, []string{
		"| 첫 표 | 값 |",
		"|---|---|",
		"| a | b |",
		"",
		"| 둘째 표 | 값 |",
		"|---------|----|",
		"| c       | d  |",
	}, bufferLines(buf))
}

// 범위가 표의 가운데를 잘라도 그 표는 통째로 맞춘다. 위아래가 다른 폭이면 표가 아니다.
func TestFormatTablesTakesWholeTable(t *testing.T) {
	buf := tableBuffer(t, "doc.md",
		"| 항목 | 값 |",
		"|---|---|",
		"| a | b |",
	)

	// 마지막 줄만 골랐다.
	buf.formatTables(2, 3, 80)

	assert.Equal(t, []string{
		"| 항목 | 값 |",
		"|------|----|",
		"| a    | b  |",
	}, bufferLines(buf))
}

// visual 의 `\mt` 다. 세 키짜리 leader 조합을 visual 도 같은 자로 본다 (ADR-0037, ADR-0106).
func TestFormatTablesKeyInVisual(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want action
	}{
		{name: "`\\mt`", keys: []string{"\\", "m", "t"}, want: actionVisualFormatTables{}},
		{
			name: "한글 자판의 원화 기호와 자모",
			keys: []string{"₩", "ㅡ", "ㅅ"},
			want: actionVisualFormatTables{},
		},

		// 고른 범위가 이미 정해져 있어 되풀이할 것이 없다. `\c` 와 같다.
		{name: "숫자는 버린다", keys: []string{"2", "\\", "m", "t"}, want: actionVisualFormatTables{}},

		{name: "짝 없는 조합은 아무 일도 없다", keys: []string{"\\", "m", "z"}, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var state visualState = visualStart{}

			var built action
			for _, key := range test.keys {
				var actions []action
				actions, state = state.press(key)

				built = nil
				if len(actions) > 0 {
					built = actions[len(actions)-1]
				}
			}

			assert.Equal(t, test.want, built)
			assert.IsType(t, visualStart{}, state, "조합이 그 자리에서 끝난다")
		})
	}
}

// `\m` 까지는 기다린다. 그 사이의 키를 삼키지 않는 것은 짝이 없을 때 드러난다.
func TestFormatTablesKeyWaitsInVisual(t *testing.T) {
	actions, state := visualStart{}.press("\\")
	require.Empty(t, actions)

	actions, state = state.press("m")

	assert.Empty(t, actions)
	assert.Equal(t, "\\m", state.showcmd())
}

// 표 여럿을 한 번에 맞추고, 되돌리기 한 번에 다 돌아온다.
func TestFormatTablesUndoesInOneStep(t *testing.T) {
	lines := []string{
		"| 첫 표 | 값 |",
		"|---|---|",
		"| a | b |",
		"",
		"사이의 문단",
		"",
		"| 둘째 표 | 값 |",
		"|---|---|",
		"| c | d |",
	}

	buf := tableBuffer(t, "doc.md", lines...)

	before := bufferLines(buf)

	found, changed := buf.formatTables(0, len(buf.lines), 80)
	require.Equal(t, 2, found)
	require.Equal(t, 2, changed)
	require.NotEqual(t, before, bufferLines(buf))

	assert.True(t, buf.applyUndo(80))
	assert.Equal(t, before, bufferLines(buf), "한 번에 다 돌아온다")
}
