package textarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/scheme"
)

// markLine 은 그 mark 의 줄이다. 없으면 -1 이다.
func markLine(buf Viewport, name string) int {
	at, ok := buf.Mark(name)
	if !ok {
		return -1
	}

	return at.Line
}

func TestMarkNameIsLowerLetterOnly(t *testing.T) {
	assert.True(t, IsMarkName("a"))
	assert.True(t, IsMarkName("z"))
	assert.False(t, IsMarkName("A"))
	assert.False(t, IsMarkName("1"))
	assert.False(t, IsMarkName("ab"))
	assert.False(t, IsMarkName(""))
}

// 같은 이름으로 다시 찍으면 옮겨진다.
func TestMarkSetMovesExisting(t *testing.T) {
	buf := NewBuffer("a.txt", []byte("a\nb\nc\n"))

	buf.SetMark("a", scheme.Cursor{Line: 0, Col: 0})
	buf.SetMark("a", scheme.Cursor{Line: 2, Col: 0})

	assert.Equal(t, 2, markLine(buf, "a"))
	assert.Len(t, buf.Marks(), 1)
}

func TestMarksSortedByName(t *testing.T) {
	buf := NewBuffer("a.txt", []byte("a\nb\nc\n"))
	buf.SetMark("c", scheme.Cursor{Line: 0})
	buf.SetMark("a", scheme.Cursor{Line: 2})

	assert.Equal(t, []Mark{{Name: "a", At: scheme.Cursor{Line: 2}}, {Name: "c", At: scheme.Cursor{Line: 0}}}, buf.Marks())
}

// 위에서 줄이 늘고 줄면 mark 가 따라간다. 줄 안을 고치는 것은 줄을 옮기지 않는다.
func TestMarkFollowsLines(t *testing.T) {
	tests := []struct {
		name  string
		at    int
		count int
		with  []string
		want  int
	}{
		{name: "위에 줄이 늘었다", at: 0, count: 0, with: []string{"new"}, want: 3},
		{name: "위의 줄이 사라졌다", at: 0, count: 1, with: nil, want: 1},
		{name: "아래에서 일어난 일", at: 3, count: 1, with: nil, want: 2},
		{name: "그 줄을 고쳤다", at: 2, count: 1, with: []string{"C!"}, want: 2},
		{name: "그 줄 위에 줄을 냈다(O)", at: 2, count: 1, with: []string{"", "c"}, want: 3},
		{name: "그 줄 아래에 줄을 냈다(o)", at: 2, count: 1, with: []string{"c", ""}, want: 2},
		{name: "그 줄을 갈랐다", at: 2, count: 1, with: []string{"x", "y"}, want: 2},
		{name: "위 줄과 합쳤다(J)", at: 1, count: 2, with: []string{"b c"}, want: -1},
		{name: "그 줄로 아래 줄을 당겼다(J)", at: 2, count: 2, with: []string{"c d"}, want: 2},
		{name: "그 줄이 지워졌다", at: 2, count: 1, with: nil, want: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := NewBuffer("a.txt", []byte("a\nb\nc\nd\n"))
			buf.SetMark("a", scheme.Cursor{Line: 2})

			with := make([][]byte, 0, len(test.with))
			for _, line := range test.with {
				with = append(with, []byte(line))
			}
			buf.ReplaceLines(test.at, test.count, with)

			assert.Equal(t, test.want, markLine(buf, "a"))
		})
	}
}

// **창을 복사해 고쳐도 원본의 mark 는 그대로다.** motion 이 복사본 위에서 이동을 돌리는데
// map 이 같은 것을 가리키면 그것이 원본을 민다(mark.go).
func TestMarkCopyDoesNotShareEdits(t *testing.T) {
	buf := NewBuffer("a.txt", []byte("a\nb\nc\n"))
	buf.SetMark("a", scheme.Cursor{Line: 2})

	copied := buf
	copied.ReplaceLines(0, 1, nil)

	assert.Equal(t, 1, markLine(copied, "a"))
	assert.Equal(t, 2, markLine(buf, "a"))
}

// 지운 줄의 mark 는 `u` 로 돌아오고, 되살린 것을 `ctrl+r` 이 다시 지운다.
func TestMarkComesBackWithUndo(t *testing.T) {
	buf := NewBuffer("a.txt", []byte("a\nb\nc\n"))
	buf.SetMark("a", scheme.Cursor{Line: 1, Col: 0})
	buf.Cursor = scheme.Cursor{Line: 1}

	_, cut := buf.DeleteRange(scheme.MotionRange{Start: scheme.Cursor{Line: 1}, End: scheme.Cursor{Line: 1}, Linewise: true})
	require.True(t, cut)
	assert.Equal(t, -1, markLine(buf, "a"))

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, 1, markLine(buf, "a"))

	require.True(t, buf.ApplyRedo())
	assert.Equal(t, -1, markLine(buf, "a"))

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, 1, markLine(buf, "a"))
}

// 지운 뒤 같은 이름을 다른 자리에 찍었으면 `u` 는 그것을 덮지 않는다(ADR-0154).
func TestMarkUndoKeepsNewerMark(t *testing.T) {
	buf := NewBuffer("a.txt", []byte("a\nb\nc\n"))
	buf.SetMark("a", scheme.Cursor{Line: 1})
	buf.Cursor = scheme.Cursor{Line: 1}

	buf.DeleteRange(scheme.MotionRange{Start: scheme.Cursor{Line: 1}, End: scheme.Cursor{Line: 1}, Linewise: true})
	buf.SetMark("a", scheme.Cursor{Line: 0})

	require.True(t, buf.ApplyUndo())
	assert.Equal(t, 0, markLine(buf, "a"))
}

// 다시 읽으면 줄 번호를 이어받고, 짧아졌으면 끝 줄로 끌어온다.
func TestMarkSurvivesAdopt(t *testing.T) {
	buf := NewBuffer("a.txt", []byte("a\nb\nc\nd\n"))
	buf.SetMark("a", scheme.Cursor{Line: 1})
	buf.SetMark("b", scheme.Cursor{Line: 3})

	next := buf.Adopt(NewBuffer("a.txt", []byte("x\ny\n")))

	assert.Equal(t, 1, markLine(next, "a"))
	assert.Equal(t, 1, markLine(next, "b"))
}
