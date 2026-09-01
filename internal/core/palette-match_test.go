package core

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bestOf 는 후보들을 걸러 점수 순으로 준다. 순위를 보는 테스트가 쓴다.
func bestOf(pattern string, targets ...string) []string {
	type hit struct {
		target string
		score  int
	}

	hits := []hit{}
	for _, target := range targets {
		score, _, ok := fuzzyMatch(pattern, target)
		if !ok {
			continue
		}
		hits = append(hits, hit{target: target, score: score})
	}

	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })

	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.target)
	}

	return out
}

func TestFuzzyMatchNeedsSubsequence(t *testing.T) {
	_, _, ok := fuzzyMatch("tide", "internal/core/edit.go")
	assert.False(t, ok, "순서가 다르면 못 찾는다")

	score, positions, ok := fuzzyMatch("icedit", "internal/core/edit.go")
	require.True(t, ok, "떨어져 있어도 순서만 맞으면 찾는다")
	assert.Greater(t, score, 0)
	assert.Len(t, positions, 6)
}

func TestFuzzyMatchEmptyPattern(t *testing.T) {
	score, positions, ok := fuzzyMatch("", "anything")

	assert.True(t, ok)
	assert.Zero(t, score)
	assert.Empty(t, positions)
}

// 붙어 있는 것이 흩어진 것을 이긴다.
func TestFuzzyMatchPrefersConsecutive(t *testing.T) {
	best := bestOf("edit", "internal/core/edit.go", "internal/core/view-editor-command.go")

	assert.Equal(t, "internal/core/edit.go", best[0])
}

// 파일 이름의 시작이 경로 중간을 이긴다. 이 기능의 핵심이다.
func TestFuzzyMatchPrefersFileNameStart(t *testing.T) {
	best := bestOf("core", "internal/core/buffer.go", "internal/core.go")

	assert.Equal(t, "internal/core.go", best[0])
}

// 단어 경계에서 시작하면 점수가 높다.
func TestFuzzyMatchPrefersBoundary(t *testing.T) {
	best := bestOf("vec", "internal/core/view-editor-command.go", "internal/core/service.go")

	assert.Equal(t, "internal/core/view-editor-command.go", best[0])
}

func TestFuzzyMatchSmartCase(t *testing.T) {
	_, _, ok := fuzzyMatch("editor", "Editor.go")
	assert.True(t, ok, "패턴이 전부 소문자면 대소문자를 가리지 않는다")

	_, _, ok = fuzzyMatch("Editor", "editor.go")
	assert.False(t, ok, "대문자를 치면 가린다")
}

// 대소문자를 안 가리는 중에도 정확히 맞은 쪽이 위로 온다.
func TestFuzzyMatchPrefersExactCase(t *testing.T) {
	best := bestOf("ed", "Editor.go", "editor.go")

	assert.Equal(t, "editor.go", best[0])
}

// 위치는 글자 경계여야 한다. byte 로 세면 한글이 반으로 잘려 강조가 깨진다.
func TestFuzzyMatchPositionsAreClusterStarts(t *testing.T) {
	target := "내부/한글 파일.go"

	_, positions, ok := fuzzyMatch("한글", target)
	require.True(t, ok)

	line := []byte(target)
	for _, offset := range positions {
		size := glyphSize(line, offset)
		assert.Contains(t, []string{"한", "글"}, string(line[offset:offset+size]))
	}
}

// 뒤에서 조이기가 앞에서 흩어놓은 정렬을 당겨온다.
// 앞에서 훑기만 하면 `a`(0) 와 `b`(4) 로 벌어진다.
func TestFuzzyMatchTightensStart(t *testing.T) {
	_, positions, ok := fuzzyMatch("ab", "a-xab")
	require.True(t, ok)

	assert.Equal(t, []int{3, 4}, positions)
}

// 이 저장소의 실제 경로로 사람이 기대하는 것이 1 등인지 본다.
func TestFuzzyMatchRanksRepositoryPaths(t *testing.T) {
	paths := []string{
		"internal/core/buffer.go",
		"internal/core/buffer-word.go",
		"internal/core/edit.go",
		"internal/core/edit_test.go",
		"internal/core/editor.go",
		"internal/core/view-editor-command.go",
		"internal/core/view-editor-normal.go",
		"internal/tui/components/input.go",
		"docs/adr/ADR-0005-sidebar-and-palette.md",
	}

	assert.Equal(t, "internal/core/edit.go", bestOf("edit", paths...)[0])
	assert.Equal(t, "internal/core/editor.go", bestOf("editor", paths...)[0])
	assert.Equal(t, "internal/core/buffer.go", bestOf("buffer", paths...)[0])
	assert.Equal(t, "internal/tui/components/input.go", bestOf("input", paths...)[0])
}
