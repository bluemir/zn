package core

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	gitconfig "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// gitIgnore 는 어느 한 디렉터리에서 무엇이 무시되는지 아는 규칙 묶음이다.
//
// git 은 규칙을 디렉터리마다 쌓는다 — 뿌리의 `.gitignore` 가 아래로 내려가고, 그 아래
// 디렉터리가 자기 `.gitignore` 로 덧쓴다. 그래서 「어디에서 보는가」가 없으면 답이 정해지지
// 않는다. 이 struct 가 그 「어디」다.
//
// go-git 의 `gitignore.ReadPatterns` 를 쓰지 않는다. 그 함수는 재귀할 때 부모의 규칙을
// 물려주지 않아서, 뿌리에 적은 `node_modules` 로 `packages/*/node_modules` 를 가지치기하지
// 못하고 그 안을 `.gitignore` 를 찾아 끝까지 훑는다(ADR-0042). 답은 맞지만 값이 크다.
type gitIgnore struct {
	root string // 저장소 뿌리의 절대 경로

	// patterns 는 뿌리부터 지금 디렉터리까지 쌓인 규칙이다. 앞이 약하고 뒤가 세다 —
	// 뒤에 온 것이 이긴다(gitignore.Matcher 가 그렇게 읽는다).
	patterns []gitignore.Pattern

	matcher gitignore.Matcher
}

// newGitIgnore 는 저장소 뿌리에서 보는 규칙을 만든다.
//
// `git ls-files --exclude-standard` 가 세는 것과 같은 것들을 읽는다 — 시스템,
// 사용자 설정 둘(`core.excludesFile`), `.git/info/exclude`, 그리고 뿌리 `.gitignore` 다.
// 순서가 곧 세기라 이 순서를 지킨다.
func newGitIgnore(root string) gitIgnore {
	rootFS := osfs.New("/")

	patterns, _ := gitignore.LoadSystemPatterns(rootFS)

	// 사용자 설정은 둘이고 XDG 쪽이 먼저다(xdgGitConfigPath).
	patterns = append(patterns, readGitExcludesFile(xdgGitConfigPath())...)

	global, _ := gitignore.LoadGlobalPatterns(rootFS)
	patterns = append(patterns, global...)

	patterns = append(patterns, readGitIgnoreFile(filepath.Join(root, ".git", "info", "exclude"), nil)...)
	patterns = append(patterns, readGitIgnoreFile(filepath.Join(root, ".gitignore"), nil)...)

	return gitIgnore{
		root:     root,
		patterns: patterns,
		matcher:  gitignore.NewMatcher(patterns),
	}
}

// descend 는 한 층 내려간 자리에서 보는 규칙을 준다. rel 은 뿌리에서부터의 상대 경로다.
//
// 내려가며 부르는 것이라 값이 층마다 한 번씩만 든다. 훑는 코드가 이것을 들고 다니면
// 「뿌리부터 지금까지」를 다시 세지 않는다.
func (g gitIgnore) descend(rel string) gitIgnore {
	added := readGitIgnoreFile(filepath.Join(g.root, rel, ".gitignore"), strings.Split(rel, "/"))
	if len(added) == 0 {
		return g
	}

	// 물려받은 것을 건드리지 않고 새 slice 를 만든다. 형제 디렉터리가 서로의 규칙을
	// 물려받으면 안 된다 — append 가 뒷마당을 나눠 쓰면 그렇게 된다.
	patterns := make([]gitignore.Pattern, 0, len(g.patterns)+len(added))
	patterns = append(patterns, g.patterns...)
	patterns = append(patterns, added...)

	return gitIgnore{
		root:     g.root,
		patterns: patterns,
		matcher:  gitignore.NewMatcher(patterns),
	}
}

// match 는 뿌리에서부터의 상대 경로가 무시되는지 답한다. rel 은 `/` 로 나뉜 것이다.
func (g gitIgnore) match(rel string, isDir bool) bool {
	if rel == "" || rel == "." {
		return false
	}

	return g.matcher.Match(strings.Split(rel, "/"), isDir)
}

// readGitIgnoreFile 은 `.gitignore` 한 장을 읽는다. 없으면 빈 것이다 — 대부분의 디렉터리가 그렇다.
//
// domain 은 이 파일이 놓인 디렉터리다. `/` 로 시작하거나 가운데에 `/` 가 든 규칙이
// 그 디렉터리 안에서만 듣게 하는 것이 domain 이 하는 일이다.
func readGitIgnoreFile(path string, domain []string) []gitignore.Pattern {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	patterns := []gitignore.Pattern{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}

	return patterns
}

// xdgGitConfigPath 는 사용자 설정 파일 둘 중 XDG 쪽 경로다.
//
// git 은 사용자 설정을 둘 읽는다. `$XDG_CONFIG_HOME/git/config` 가 먼저이고 `~/.gitconfig`
// 가 나중이며, 나중에 온 것이 이긴다. go-git 의 `LoadGlobalPatterns` 는 `~/.gitconfig` 만
// 보므로(dir.go 의 `gitconfigFile`) 앞의 것을 우리가 채운다(ADR-0042).
//
// `XDG_CONFIG_HOME` 이 비어 있으면 `~/.config` 다. git 이 그렇게 정해 두었다.
//
// 홈을 못 찾으면 빈 문자열이다. 읽는 쪽이 없는 파일과 같이 다룬다.
func xdgGitConfigPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "git", "config")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".config", "git", "config")
}

// readGitExcludesFile 은 git 설정 파일 한 장에서 `core.excludesFile` 이 가리키는 규칙을 읽는다.
//
// 설정 파일이 없는 것, 그 칸이 비어 있는 것, 가리키는 파일이 없는 것이 모두 흔하다.
// 셋 다 규칙이 없는 것으로 치고 빈 것을 준다(readGitIgnoreFile 와 같은 손이다).
//
// domain 은 없다. 사용자 설정이 가리키는 파일은 저장소 밖에 있어서 어느 디렉터리에도
// 매이지 않는다.
func readGitExcludesFile(config string) []gitignore.Pattern {
	if config == "" {
		return nil
	}

	data, err := os.ReadFile(config)
	if err != nil {
		return nil
	}

	raw := gitconfig.New()
	if err := gitconfig.NewDecoder(bytes.NewReader(data)).Decode(raw); err != nil {
		return nil
	}

	// git 의 설정 이름은 대소문자를 가리지 않는다. go-git 의 `Options.Get` 이 그렇게 찾는다.
	path := raw.Section("core").Options.Get("excludesfile")
	if path == "" {
		return nil
	}

	// 설정 파일에 적힌 `~` 는 아무도 풀어 주지 않는다. 셸을 거치지 않은 글자다(ADR-0088).
	full, err := expandHome(path)
	if err != nil {
		return nil
	}

	return readGitIgnoreFile(full, nil)
}

// gitIgnoreAt 은 뿌리에서 startRel 까지 내려간 자리에서 보는 규칙을 준다.
//
// startRel 은 뿌리 기준 상대 경로(`/` 로 나뉜 것) 이고, 뿌리 자신은 빈 문자열이다.
// 층마다 그 층의 `.gitignore` 를 얹으므로 값은 깊이만큼만 든다.
func gitIgnoreAt(root, startRel string) gitIgnore {
	ignore := newGitIgnore(root)
	if startRel == "" {
		return ignore
	}

	parts := strings.Split(startRel, "/")
	for i := range parts {
		ignore = ignore.descend(strings.Join(parts[:i+1], "/"))
	}

	return ignore
}

// walkGitFiles 는 무시되지 않은 파일을 뿌리 기준 상대 경로(`/` 로 나뉜 것) 로 하나씩 준다.
//
// startRel 아래만 본다. 뿌리 전체를 볼 때는 빈 문자열이다 — 위쪽 층의 무시 규칙은 어느
// 경우에도 그대로 얹힌다. 팔레트가 cwd 아래만 모으면서도 뿌리의 `.gitignore` 를 따르는 것이
// 이 때문이다.
//
// visit 이 false 를 주면 그 자리에서 그만둔다. dirty 판정은 처음 하나를 찾으면 더 볼 것이
// 없으므로 이것으로 끊고, 팔레트 목록은 끝까지 받는다.
//
// 무시된 디렉터리는 들어가지 않는다. 이것이 이 함수의 값 전부다 — node_modules 가 있는
// 저장소에서 들어가느냐 마느냐가 13 만 개와 6 천 개를 가른다(ADR-0042).
//
// 읽지 못하는 디렉터리는 조용히 지나간다. 목록이 조금 모자랄 뿐이고, 아무것도 못 주는 것보다 낫다.
func walkGitFiles(ctx context.Context, root, startRel string, visit func(rel string) bool) {
	walkGitDir(ctx, root, startRel, gitIgnoreAt(root, startRel), visit)
}

// walkGitDir 은 한 디렉터리를 훑는다. false 는 「그만두라」는 뜻이라 위로 그대로 전해진다.
//
// filepath.WalkDir 을 쓰지 않는다. 무시 규칙은 층마다 쌓이므로 지금 어느 층에 있는지를
// 들고 다녀야 하는데, 평평한 callback 으로는 그 자리를 알 수 없다.
func walkGitDir(ctx context.Context, root, rel string, ignore gitIgnore, visit func(rel string) bool) bool {
	if ctx.Err() != nil {
		return false
	}

	entries, err := os.ReadDir(filepath.Join(root, rel))
	if err != nil {
		return true
	}

	for _, entry := range entries {
		// .git 은 어느 깊이에서든 건너뛴다. worktree 나 submodule 에서는 디렉터리가 아니라
		// 파일이므로 종류를 보지 않고 이름만 본다(sidebar 의 readDir 과 같은 태도다).
		if entry.Name() == ".git" {
			continue
		}

		childRel := entry.Name()
		if rel != "" {
			childRel = rel + "/" + entry.Name()
		}

		if entry.IsDir() {
			if ignore.match(childRel, true) {
				continue
			}

			// 내려가기 전에 그 디렉터리의 `.gitignore` 를 먼저 얹는다. 그러지 않으면
			// 그 안의 파일을 그 디렉터리 규칙 없이 판정한다.
			if !walkGitDir(ctx, root, childRel, ignore.descend(childRel), visit) {
				return false
			}

			continue
		}

		if ignore.match(childRel, false) {
			continue
		}

		if !visit(childRel) {
			return false
		}
	}

	return true
}
