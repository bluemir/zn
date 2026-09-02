package core

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pickCommand 는 팔레트에 pattern 을 쳐서 맨 앞에 선 명령이 want 인지 보고 그것을 고른다.
// 이름을 확인하지 않고 enter 를 치면 둘 중 어느 것이 실행됐는지 모른 채 결과만 보게 된다.
func pickCommand(t *testing.T, m tea.Model, pattern, want string) tea.Model {
	t.Helper()

	m = typeInto(m, pattern)

	palette, ok := m.(viewPalette)
	require.True(t, ok)
	require.NotEmpty(t, palette.hits)
	require.Equal(t, want, palette.commands()[palette.hits[0].index].name)

	return send(m, "enter")
}

// 「오늘 날짜 넣기」는 커서 뒤에 오늘 날짜를 넣는다. normal 의 `a` 와 같은 자리다.
func TestPaletteInsertsDate(t *testing.T) {
	var m tea.Model = send(newTestEditor("ab\n", 80, 20), "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = pickCommand(t, m, ">insert date", "오늘 날짜 넣기")

	require.IsType(t, viewEditorNormal{}, m)
	line := linesOf(bufferOf(t, m))[0]

	// `a` 뒤에 들어가고 `b` 는 그 뒤에 남는다.
	require.True(t, strings.HasPrefix(line, "a"), line)
	require.True(t, strings.HasSuffix(line, "b"), line)

	got := strings.TrimSuffix(strings.TrimPrefix(line, "a"), "b")
	when, err := time.ParseInLocation(insertDateFormat, got, time.Local)
	require.NoError(t, err, got)
	assert.WithinDuration(t, time.Now(), when, 24*time.Hour)

	// 커서는 넣은 마지막 글자 위에 선다. insert 의 `esc` 와 같다.
	assert.Equal(t, len("a")+len(got)-1, bufferOf(t, m).cursor.Col)

	// 넣은 것은 한 번의 `u` 로 통째로 돌아온다.
	m = send(m, "u")
	assert.Equal(t, []string{"ab"}, linesOf(bufferOf(t, m)))
}

// 「오늘 날짜와 시각 넣기」는 RFC3339 로 넣는다. 날짜와 시각 사이가 `T` 다.
func TestPaletteInsertsDateTime(t *testing.T) {
	var m tea.Model = send(newTestEditor("\n", 80, 20), "ctrl+p")
	require.IsType(t, viewPalette{}, m)

	m = pickCommand(t, m, ">insert date and time", "오늘 날짜와 시각 넣기")

	require.IsType(t, viewEditorNormal{}, m)
	got := linesOf(bufferOf(t, m))[0]

	when, err := time.Parse(time.RFC3339, got)
	require.NoError(t, err, got)
	assert.WithinDuration(t, time.Now(), when, time.Minute)

	// 빈 줄에서는 커서 뒤가 곧 줄 맨 앞이다. 넣은 것만 있고 앞뒤에 붙은 것이 없다.
	assert.Equal(t, when.Format(time.RFC3339), got)
}

// 두 명령은 서식만 갈린다. 날짜 쪽이 시각 쪽의 앞머리와 같은 글이다.
func TestInsertDateFormatIsRFC3339Prefix(t *testing.T) {
	now := time.Now()

	assert.Equal(t, now.Format(insertDateFormat), strings.SplitN(now.Format(time.RFC3339), "T", 2)[0])
}
