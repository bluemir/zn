package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grepIn 은 그 디렉터리를 뿌리로 삼아 검색을 끝까지 돌린 결과다.
//
// 작업을 실제로 돌린다 — 조각을 다 받아서 마지막 apply 를 editor 에 넣는다. 실행기를
// 지나가지 않는 것은 여기서 보는 것이 「무엇을 찾았나」이고 실행기 쪽은 job_test.go 가 본다.
func grepIn(t *testing.T, root, input string, overlay map[string][]byte) grepResult {
	t.Helper()

	return grepOpenIn(t, root, input, nil, overlay)
}

// grepOpenIn 은 열린 파일 목록까지 넘겨서 돌린다. 훑는 목록에 없는 파일이 드는지를 보는 자리다.
func grepOpenIn(t *testing.T, root, input string, open []string, overlay map[string][]byte) grepResult {
	t.Helper()

	pattern, err := parseSearchPattern(input)
	require.NoError(t, err)

	e := &editor{grep: grepResult{input: input, pattern: pattern}}

	for progress := range grepFiles(context.Background(), root, input, pattern, open, overlay) {
		if progress.apply != nil {
			progress.apply(e)
		}
	}

	return e.grep
}

// writeTree 는 시험용 파일 몇 개를 만든다. git 저장소가 아니라 walkFiles 가 훑는다.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, data := range files {
		full := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(data), 0644))
	}

	return root
}

// 여러 파일에 걸쳐 찾고, 적중마다 경로·줄·칸·줄 내용을 든다.
func TestGrepFindsAcrossFiles(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.go":     "package a\nfunc New() {}\n",
		"sub/b.go": "package b\n\nfunc NewThing() {}\n",
		"c.txt":    "아무것도 없다\n",
	})

	got := grepIn(t, root, "func New", nil)

	require.Len(t, got.hits, 2)
	assert.Equal(t, 2, got.files)

	// 훑는 차례는 파일 이름 순이다(gitFiles·walkFiles 가 정렬해 준다).
	assert.Equal(t, "a.go", got.hits[0].path)
	assert.Equal(t, 1, got.hits[0].line, "줄은 0 부터다")
	assert.Equal(t, 0, got.hits[0].col)
	assert.Equal(t, "func New() {}", got.hits[0].text)

	assert.Equal(t, filepath.Join("sub", "b.go"), got.hits[1].path)
	assert.Equal(t, 2, got.hits[1].line)
}

// 줄마다 첫 매칭만 담는다. 같은 글이 목록에 두 줄 서면 훑는 데 방해가 된다.
func TestGrepKeepsOneHitPerLine(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "aa aa aa\n"})

	got := grepIn(t, root, "aa", nil)

	require.Len(t, got.hits, 1)
	assert.Equal(t, 0, got.hits[0].col, "첫 매칭 자리다")
}

// 이진 파일은 건너뛴다. 앞부분에 NUL 이 있으면 이진으로 본다.
func TestGrepSkipsBinaryFiles(t *testing.T) {
	root := writeTree(t, map[string]string{
		"data.bin": "hit\x00hit\n",
		"a.txt":    "hit\n",
	})

	got := grepIn(t, root, "hit", nil)

	require.Len(t, got.hits, 1)
	assert.Equal(t, "a.txt", got.hits[0].path)
}

// flag 는 `/` 검색과 같은 문법이다. `/i` 는 대소문자를 무시한다.
func TestGrepSharesSearchPatternSyntax(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "NewBuffer\n"})

	assert.Len(t, grepIn(t, root, "newbuffer/i", nil).hits, 1)
	assert.Empty(t, grepIn(t, root, "newbuffer", nil).hits)
}

// overlay 가 있으면 디스크가 아니라 그 글을 훑는다. 고치던 buffer 다.
func TestGrepReadsOverlayInsteadOfDisk(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "디스크의 글\n"})

	got := grepIn(t, root, "고치던", map[string][]byte{"a.go": []byte("고치던 글\n")})

	require.Len(t, got.hits, 1)
	assert.Equal(t, "고치던 글", got.hits[0].text)

	// 디스크에만 있는 글은 이제 걸리지 않는다 — 사람이 보고 있는 것이 buffer 의 글이다.
	assert.Empty(t, grepIn(t, root, "디스크", map[string][]byte{"a.go": []byte("고치던 글\n")}).hits)
}

// 상한에 닿으면 그만두고 닿았다는 것을 남긴다. 조용히 자르면 「이게 전부」로 읽힌다.
func TestGrepCapsHitsAndSaysSo(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": strings.Repeat("hit\n", grepMaxHits+50)})

	got := grepIn(t, root, "hit", nil)

	assert.Len(t, got.hits, grepMaxHits)
	assert.True(t, got.capped)
	assert.Contains(t, grepSummary(len(got.hits), got.files, got.capped, got.skipped), "상한")
}

// 찾은 것이 없으면 빈 결과가 한 번은 온다. 조각이 하나도 없으면 앞서 보던 목록이 남는다.
func TestGrepReportsEmptyResult(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": "아무것도\n"})

	got := grepIn(t, root, "없는것", nil)

	assert.Empty(t, got.hits)
	assert.Zero(t, got.files)
	assert.False(t, got.capped)
}

// 앞 검색의 늦은 조각은 새 결과를 덮지 않는다. 이름이 같아도 인자가 다르면 다른 작업이다.
func TestGrepApplyIgnoresStaleResult(t *testing.T) {
	e := &editor{grep: grepResult{input: "새 패턴"}}

	applyGrep("지난 패턴", []grepHit{{path: "a.go"}}, 1, false, 0)(e)

	assert.Empty(t, e.grep.hits, "패턴이 바뀌었으면 넣지 않는다")

	applyGrep("새 패턴", []grepHit{{path: "a.go"}}, 1, false, 0)(e)
	assert.Len(t, e.grep.hits, 1)
}

// `.gitignore` 가 가린 파일도 tab 으로 열어 두었으면 검색에 든다 (ADR-0077).
//
// 보고 있는 파일이 검색에서 빠지는 것은 규칙이 아니라 놀라움이다. 열지 않았으면 그대로 가린다.
func TestGrepIncludesOpenIgnoredFile(t *testing.T) {
	root := newGitFixture(t)
	write(t, root, "debug.log", "찾는 글\n")

	assert.Empty(t, grepIn(t, root, "찾는", nil).hits, "열지 않았으면 가린 그대로다")

	got := grepOpenIn(t, root, "찾는", []string{"debug.log"}, nil)

	require.Len(t, got.hits, 1)
	assert.Equal(t, "debug.log", got.hits[0].path)
	assert.Equal(t, "찾는 글", got.hits[0].text)
}

// 가린 파일을 고치던 중이면 디스크가 아니라 그 글을 훑는다. 두 손이 같이 걸리는 자리다.
func TestGrepIncludesOpenIgnoredFileWithOverlay(t *testing.T) {
	root := newGitFixture(t)
	write(t, root, "debug.log", "디스크의 글\n")

	got := grepOpenIn(t, root, "고치던", []string{"debug.log"},
		map[string][]byte{"debug.log": []byte("고치던 글\n")})

	require.Len(t, got.hits, 1)
	assert.Equal(t, "고치던 글", got.hits[0].text)
}

// 뿌리 밖에 열어 둔 파일도 검색에 든다. 목록에는 `../` 로 선다 (ADR-0077).
func TestGrepIncludesOpenFileOutsideRoot(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "package a\n"})

	outside := t.TempDir()
	full := filepath.Join(outside, "far.go")
	require.NoError(t, os.WriteFile(full, []byte("찾는 글\n"), 0644))

	rel, err := filepath.Rel(root, full)
	require.NoError(t, err)

	got := grepOpenIn(t, root, "찾는", []string{rel}, nil)

	require.Len(t, got.hits, 1)
	assert.Equal(t, rel, got.hits[0].path)
	assert.Equal(t, "찾는 글", got.hits[0].text)
}

// 이미 훑는 목록에 있는 파일은 두 번 넣지 않는다. 두 번 훑으면 적중도 파일 수도 겹친다.
func TestGrepDoesNotSearchOpenFileTwice(t *testing.T) {
	root := newGitFixture(t)
	write(t, root, "main.go", "package main\n\nfunc 찾는것() {}\n")

	got := grepOpenIn(t, root, "찾는것", []string{"main.go", "main.go"}, nil)

	assert.Len(t, got.hits, 1)
	assert.Equal(t, 1, got.files)
}

// ctx 를 끊으면 그만둔다. 아무도 받지 않는 채널에 goroutine 이 남지 않는다.
func TestGrepStopsWhenCancelled(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": "hit\n", "b.txt": "hit\n"})

	ctx, cancel := context.WithCancel(context.Background())
	pattern, err := parseSearchPattern("hit")
	require.NoError(t, err)

	ch := grepFiles(ctx, root, "hit", pattern, nil, nil)
	cancel()

	// 채널은 닫힌다. 끊긴 뒤에도 보내려 하면 여기서 막혀서 시험이 끝나지 않는다.
	for range ch { //nolint:revive // 다 흘려보내는 것이 이 시험이다
	}
}

// dirtyOverlay 는 고치던 buffer 만 담고, 경로를 뿌리 기준 상대로 맞춘다.
//
// **buf.path 가 상대여도 맞춰야 한다.** CLI 인자로 열면 상대 경로가 그대로 담기는데,
// 그때 Rel 이 실패하면 overlay 가 조용히 비어서 고치던 글이 검색에서 빠진다.
func TestDirtyOverlayNormalizesPaths(t *testing.T) {
	root := t.TempDir()

	clean := newBuffer(filepath.Join(root, "clean.go"), []byte("깨끗\n"))

	absolute := newBuffer(filepath.Join(root, "abs.go"), []byte("절대\n"))
	absolute.dirty = true

	e := editor{buffers: []viewport{clean, absolute}}
	open, overlay := e.dirtyOverlay(root)

	assert.Len(t, overlay, 1, "깨끗한 buffer 의 글은 담지 않는다 — 디스크와 같다")
	assert.Contains(t, overlay, "abs.go")

	assert.Equal(t, []string{"clean.go", "abs.go"}, open,
		"열린 파일 목록에는 깨끗한 것도 든다")

	// 상대 경로로 담긴 buffer 다. 그 자리로 들어가서 만든다.
	//
	// **뿌리를 `Getwd` 로 받는다.** macOS 의 `t.TempDir()` 은 `/var/...` 를 주는데 `Getwd` 는
	// symlink 를 푼 `/private/var/...` 를 준다. 앱에서는 뿌리도 `Abs` 도 같은 `Getwd` 를
	// 지나므로 어긋날 수 없고, 여기서만 원본 경로를 섞으면 시험이 혼자 틀린다.
	inside := t.TempDir()

	back, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(inside))
	t.Cleanup(func() { _ = os.Chdir(back) })

	cwd, err := os.Getwd()
	require.NoError(t, err)

	relative := newBuffer(filepath.Join("sub", "rel.go"), []byte("상대\n"))
	relative.dirty = true

	e = editor{buffers: []viewport{relative}}

	open, overlay = e.dirtyOverlay(cwd)
	assert.Contains(t, overlay, filepath.Join("sub", "rel.go"))
	assert.Equal(t, []string{filepath.Join("sub", "rel.go")}, open)
}

// 뿌리 밖의 파일도 담는다. 열어 둔 파일은 어디에 있든 검색에 든다.
//
// 자는 `../` 로 시작하는 뿌리 기준 상대 경로다. 읽는 자리도 쓰는 자리도
// `filepath.Join(root, path)` 를 지나므로 그대로 제 파일이 된다.
func TestDirtyOverlayKeepsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	full := filepath.Join(outside, "far.go")

	buf := newBuffer(full, []byte("멀다\n"))
	buf.dirty = true

	e := editor{buffers: []viewport{buf}}

	open, overlay := e.dirtyOverlay(root)

	rel, err := filepath.Rel(root, full)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rel, ".."), "뿌리 밖을 가리키는 자다")

	assert.Equal(t, []string{rel}, open)
	assert.Contains(t, overlay, rel)
	assert.Equal(t, full, filepath.Join(root, rel), "그 자가 제 파일로 풀린다")
}
