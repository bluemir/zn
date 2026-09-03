package core

import (
	"testing"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/textarea"
)

// selectionRange 는 커서가 선 글자까지 넣는다. 어느 쪽 끝에서 골랐든 범위는 같다.
func TestSelectionRangeIsInclusive(t *testing.T) {
	tests := []struct {
		name                  string
		anchorLine, anchorCol int
		cursorLine, cursorCol int
		linewise              bool
		want                  scheme.MotionRange
	}{
		{
			name: "한 글자", want: scheme.MotionRange{End: scheme.Cursor{Col: 1}},
		},
		{
			name: "앞으로 고르기", cursorCol: 3,
			want: scheme.MotionRange{End: scheme.Cursor{Col: 4}},
		},
		{
			name: "뒤로 고르기", anchorCol: 3,
			want: scheme.MotionRange{End: scheme.Cursor{Col: 4}},
		},
		{
			name: "줄을 넘어", cursorLine: 1, cursorCol: 1,
			want: scheme.MotionRange{End: scheme.Cursor{Line: 1, Col: 2}},
		},
		{
			// 한글 한 글자는 3 byte 다. cluster 통째로 든다.
			name: "한글", anchorLine: 2, cursorLine: 2,
			want: scheme.MotionRange{Start: scheme.Cursor{Line: 2}, End: scheme.Cursor{Line: 2, Col: 3}},
		},
		{
			name: "빈 줄에는 밀 글자가 없다", anchorLine: 3, cursorLine: 3,
			want: scheme.MotionRange{Start: scheme.Cursor{Line: 3}, End: scheme.Cursor{Line: 3}},
		},
		{
			// 줄 단위도 칸을 담는다. 고치는 자리는 줄 전체를 쓰지만, 복사가 커서를 처음
			// 짚은 자리로 되돌릴 때 그 칸이 필요하다(ADR-0100).
			name: "줄 단위도 칸을 담는다", anchorCol: 2, cursorLine: 1, cursorCol: 5, linewise: true,
			want: scheme.MotionRange{Start: scheme.Cursor{Col: 2}, End: scheme.Cursor{Line: 1, Col: 5}, Linewise: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := textarea.NewBuffer("test.txt", []byte("foo bar\nbaz qux\n한글\n\n"))
			buf.Selection = textarea.Selection{
				Active:   true,
				Linewise: test.linewise,
				From:     scheme.Cursor{Line: test.anchorLine, Col: test.anchorCol},
				To:       scheme.Cursor{Line: test.cursorLine, Col: test.cursorCol},
			}

			area, ok := buf.SelectionRange()
			require.True(t, ok)
			assert.Equal(t, test.want, area)
		})
	}
}

func TestSelectionRangeNeedsActiveSelection(t *testing.T) {
	buf := textarea.NewBuffer("test.txt", []byte("foo\n"))

	_, ok := buf.SelectionRange()
	assert.False(t, ok, "고른 것이 없으면 범위도 없다")
}

// selectionOn 은 범위를 줄마다의 byte 구간으로 자른다.
func TestSelectionOn(t *testing.T) {
	buf := textarea.NewBuffer("test.txt", []byte("foo bar\nbaz qux\nquux\n"))

	t.Run("한 줄 안", func(t *testing.T) {
		span, toEnd, ok := buf.SelectionOn(scheme.MotionRange{Start: scheme.Cursor{Col: 1}, End: scheme.Cursor{Col: 4}}, 0)

		require.True(t, ok)
		assert.Equal(t, []int{1, 4}, span)
		assert.False(t, toEnd, "줄 끝을 넘지 않았다")
	})

	t.Run("여러 줄", func(t *testing.T) {
		area := scheme.MotionRange{Start: scheme.Cursor{Col: 4}, End: scheme.Cursor{Line: 2, Col: 2}}

		span, toEnd, ok := buf.SelectionOn(area, 0)
		require.True(t, ok)
		assert.Equal(t, []int{4, 7}, span)
		assert.True(t, toEnd, "첫 줄은 개행까지다")

		span, toEnd, ok = buf.SelectionOn(area, 1)
		require.True(t, ok)
		assert.Equal(t, []int{0, 7}, span)
		assert.True(t, toEnd, "가운데 줄은 통째로다")

		span, toEnd, ok = buf.SelectionOn(area, 2)
		require.True(t, ok)
		assert.Equal(t, []int{0, 2}, span)
		assert.False(t, toEnd, "마지막 줄은 커서 자리까지다")
	})

	t.Run("줄 단위는 줄 전체", func(t *testing.T) {
		span, toEnd, ok := buf.SelectionOn(scheme.MotionRange{End: scheme.Cursor{Line: 1}, Linewise: true}, 1)

		require.True(t, ok)
		assert.Equal(t, []int{0, 7}, span)
		assert.True(t, toEnd)
	})

	t.Run("범위 밖의 줄", func(t *testing.T) {
		_, _, ok := buf.SelectionOn(scheme.MotionRange{End: scheme.Cursor{Line: 1}}, 2)

		assert.False(t, ok)
	})
}

// 겹친 자리는 검색이 이긴다. 선택 배경이 찾은 자리를 덮으면 `n` 이 데려다 놓은 곳이 안 보인다.
func TestRowSegmentsGiveSearchPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		highlight rowHighlight
		want      []rowSegment
	}{
		{
			name:      "선택만",
			highlight: rowHighlight{cursorCol: -1, selection: []int{2, 5}},
			want:      []rowSegment{{start: 2, end: 5, style: styleSelection}},
		},
		{
			name:      "검색만",
			highlight: rowHighlight{cursorCol: -1, matches: [][]int{{1, 3}}},
			want:      []rowSegment{{start: 1, end: 3, style: styleSearchMatch}},
		},
		{
			name: "선택 가운데의 매칭",
			highlight: rowHighlight{
				cursorCol: -1, selection: []int{0, 9}, matches: [][]int{{3, 5}},
			},
			want: []rowSegment{
				{start: 0, end: 3, style: styleSelection},
				{start: 3, end: 5, style: styleSearchMatch},
				{start: 5, end: 9, style: styleSelection},
			},
		},
		{
			name: "선택 끝에 걸친 매칭",
			highlight: rowHighlight{
				cursorCol: -1, selection: []int{0, 5}, matches: [][]int{{3, 8}},
			},
			want: []rowSegment{
				{start: 0, end: 3, style: styleSelection},
				{start: 3, end: 8, style: styleSearchMatch},
			},
		},
		{
			name: "선택 앞의 매칭",
			highlight: rowHighlight{
				cursorCol: -1, selection: []int{5, 8}, matches: [][]int{{0, 2}},
			},
			want: []rowSegment{
				{start: 0, end: 2, style: styleSearchMatch},
				{start: 5, end: 8, style: styleSelection},
			},
		},
		{
			name: "커서가 선 매칭만 색이 다르다",
			highlight: rowHighlight{
				cursorCol: 3, selection: []int{0, 9}, matches: [][]int{{3, 5}},
			},
			want: []rowSegment{
				{start: 0, end: 3, style: styleSelection},
				{start: 3, end: 5, style: styleSearchCurrent},
				{start: 5, end: 9, style: styleSelection},
			},
		},
		{
			name:      "행 밖은 잘린다",
			highlight: rowHighlight{cursorCol: -1, selection: []int{0, 20}, matches: [][]int{{15, 18}}},
			want:      []rowSegment{{start: 0, end: 10, style: styleSelection}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.highlight.topSegments(0, 10))
		})
	}
}
