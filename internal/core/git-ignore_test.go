package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 사용자 설정은 둘이고 XDG 쪽을 go-git 이 보지 않는다. 그 자리를 우리가 채운다(ADR-0042).
func TestXDGGitConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	assert.Equal(t, "/tmp/xdg/git/config", xdgGitConfigPath())

	// 비어 있으면 `~/.config` 다. git 이 그렇게 정해 두었다.
	t.Setenv("XDG_CONFIG_HOME", "")

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "git", "config"), xdgGitConfigPath())
}

// 설정이 가리키는 규칙 파일을 읽어 온다.
func TestReadGitExcludesFile(t *testing.T) {
	dir := t.TempDir()

	ignore := filepath.Join(dir, "mine.ignore")
	require.NoError(t, os.WriteFile(ignore, []byte("# 주석\n\n*.log\nbuild/\n"), 0644))

	config := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(config, []byte("[core]\n\texcludesFile = "+ignore+"\n"), 0644))

	patterns := readGitExcludesFile(config)
	require.Len(t, patterns, 2, "주석과 빈 줄은 규칙이 아니다")

	// 규칙이 실제로 듣는지 본다. 개수만 세는 것으로는 domain 을 잘못 준 것을 못 잡는다.
	rules := gitIgnore{root: dir, patterns: patterns, matcher: gitignore.NewMatcher(patterns)}

	assert.True(t, rules.match("a.log", false))
	assert.True(t, rules.match("deep/b.log", false), "저장소 밖의 규칙이라 어느 층에서나 듣는다")
	assert.False(t, rules.match("a.txt", false))
}

// 없을 수 있는 것이 셋이다. 셋 다 규칙이 없는 것으로 친다.
func TestReadGitExcludesFileMissing(t *testing.T) {
	dir := t.TempDir()

	assert.Nil(t, readGitExcludesFile(""), "경로를 못 찾은 것")
	assert.Nil(t, readGitExcludesFile(filepath.Join(dir, "없는-설정")), "설정 파일이 없는 것")

	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(empty, []byte("[user]\n\tname = 나\n"), 0644))
	assert.Nil(t, readGitExcludesFile(empty), "그 칸이 없는 것")

	dangling := filepath.Join(dir, "dangling")
	require.NoError(t, os.WriteFile(dangling, []byte("[core]\n\texcludesFile = "+dir+"/없는-규칙\n"), 0644))
	assert.Nil(t, readGitExcludesFile(dangling), "가리키는 파일이 없는 것")
}

// 설정에 적힌 `~` 를 푼다. 셸을 거치지 않은 글자라 아무도 풀어 주지 않는다(ADR-0088).
func TestReadGitExcludesFileExpandsHome(t *testing.T) {
	dir := t.TempDir()

	// 홈을 옮겨 두고 그 아래에 규칙을 쓴다. 사용자의 실제 홈은 건드리지 않는다.
	t.Setenv("HOME", dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "mine.ignore"), []byte("*.tmp\n"), 0644))

	config := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(config, []byte("[core]\n\texcludesFile = ~/mine.ignore\n"), 0644))

	patterns := readGitExcludesFile(config)
	require.Len(t, patterns, 1, "`~` 가 풀리지 않으면 그런 이름의 디렉터리를 찾다가 빈 것이 온다")

	rules := gitIgnore{root: dir, patterns: patterns, matcher: gitignore.NewMatcher(patterns)}
	assert.True(t, rules.match("a.tmp", false))
}

// walkGitDirs 는 뿌리를 빈 문자열로 먼저 주고, 무시되는 디렉터리에는 들어가지 않는다.
//
// 감시를 붙일 목록이 이것이다(watch.go). 층층이 쌓이는 규칙을 따르므로 하위 `.gitignore`
// 도 듣는다.
func TestWalkGitDirs(t *testing.T) {
	root := t.TempDir()

	for _, dir := range []string{
		".git", ".git/objects",
		"zn-kept", "zn-kept/deep",
		"zn-hidden", "zn-hidden/deep",
		"zn-outer", "zn-outer/zn-inner",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("zn-hidden/\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "zn-outer", ".gitignore"), []byte("zn-inner/\n"), 0644))

	seen := []string{}
	walkGitDirs(t.Context(), root, func(rel string) bool {
		seen = append(seen, rel)

		return true
	})

	assert.Equal(t, "", seen[0], "뿌리가 빈 문자열로 먼저 와야 감시가 뿌리에도 붙는다")

	assert.Contains(t, seen, "zn-kept")
	assert.Contains(t, seen, "zn-kept/deep")
	assert.Contains(t, seen, "zn-outer")

	assert.NotContains(t, seen, ".git", ".git 은 어느 깊이에서든 건너뛴다")
	assert.NotContains(t, seen, ".git/objects", "무시된 디렉터리 안으로 들어갔다")
	assert.NotContains(t, seen, "zn-hidden")
	assert.NotContains(t, seen, "zn-hidden/deep")

	// 하위 `.gitignore` 도 듣는다. 내려가기 전에 그 층의 규칙을 얹기 때문이다.
	assert.NotContains(t, seen, "zn-outer/zn-inner")
}

// visit 이 그만두라고 하면 그 자리에서 멈춘다.
func TestWalkGitDirsStops(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "zn-kept/deep"), 0755))

	seen := 0
	walkGitDirs(t.Context(), root, func(string) bool {
		seen++

		return false
	})

	assert.Equal(t, 1, seen, "그만두라고 했는데 계속 걸었다")
}
