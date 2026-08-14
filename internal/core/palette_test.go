package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPaletteFixture 는 파일 목록을 읽을 디렉터리를 만든다. git 저장소는 아니다.
func newPaletteFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{"internal", "build"} {
		require.NoError(t, os.Mkdir(filepath.Join(root, dir), 0755))
	}
	for _, file := range []string{"main.go", "internal/edit.go", "build/out"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte("x\n"), 0644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n"), 0644))

	return root
}

// indexedFiles 는 인덱싱 작업을 끝까지 돌려 모인 목록을 준다.
// 조각이 누적분 전체를 갈아끼우므로 마지막 것이 곧 결과다.
func indexedFiles(t *testing.T, root string) []string {
	t.Helper()

	e := editor{}
	for progress := range indexFiles(t.Context(), root) {
		if progress.apply != nil {
			progress.apply(&e)
		}
	}

	return e.files
}

// 저장소가 아니면 직접 훑는다. `.git` 안은 나오지 않는다.
func TestIndexFilesWalks(t *testing.T) {
	root := newPaletteFixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("x\n"), 0644))

	files := indexedFiles(t, root)

	assert.Equal(t, []string{".gitignore", "build/out", "internal/edit.go", "main.go"}, files)
}

// 저장소면 git 이 준 목록이라 gitignore 된 것이 빠진다.
func TestIndexFilesUsesGitignore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}

	root := newPaletteFixture(t)
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	require.NoError(t, cmd.Run())

	files := indexedFiles(t, root)

	assert.Contains(t, files, "internal/edit.go")
	assert.Contains(t, files, ".gitignore", "추가하지 않은 파일도 나온다")
	assert.NotContains(t, files, "build/out", "무시된 것은 빠진다")
	assert.NotContains(t, files, ".git/HEAD")
}

func TestFilterPaletteEmptyPatternKeepsOrder(t *testing.T) {
	hits := filterPalette("", []string{"b", "a", "c"})

	assert.Equal(t, []int{0, 1, 2}, hitIndexes(hits))
}

func TestFilterPaletteSortsByScore(t *testing.T) {
	hits := filterPalette("edit", []string{"view-editor-command.go", "edit.go"})

	require.NotEmpty(t, hits)
	assert.Equal(t, 1, hits[0].index)
}

// 같은 점수면 짧은 것이 위고, 그것도 같으면 원래 자리 순이다.
func TestFilterPaletteTieIsStable(t *testing.T) {
	labels := []string{"a/x.go", "b/x.go", "x.go"}

	first := hitIndexes(filterPalette("x", labels))
	second := hitIndexes(filterPalette("x", labels))

	assert.Equal(t, first, second)
	assert.Equal(t, 2, first[0], "짧은 것이 위")
	assert.Equal(t, []int{2, 0, 1}, first)
}

// 명령은 한글 이름을 몰라도 영문 설명과 `:` 명령으로 찾힌다.
func TestPaletteCommandMatchesEnglish(t *testing.T) {
	labels := make([]string, 0, len(paletteCommands))
	for _, command := range paletteCommands {
		labels = append(labels, command.label())
	}

	for _, testCase := range []struct {
		pattern string
		want    string
	}{
		{"trim", "줄 끝 공백 지우기"},
		{"tree", "파일 트리 열기/닫기"},
		{":tree", "파일 트리 열기/닫기"},
		{"공백", "줄 끝 공백 지우기"},
		{"noh", "검색 강조 끄기"},
	} {
		hits := filterPalette(testCase.pattern, labels)

		require.NotEmpty(t, hits, testCase.pattern)
		assert.Equal(t, testCase.want, paletteCommands[hits[0].index].name, testCase.pattern)
	}
}

func hitIndexes(hits []paletteHit) []int {
	out := make([]int, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.index)
	}

	return out
}
