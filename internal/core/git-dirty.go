package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// gitDirty 는 commit 하지 않은 것이 하나라도 있는지 답한다.
//
// `git status` 가 clean 이라고 부르는 것과 같은 기준이다(ADR-0009) — 셋을 다 본다.
// index 와 HEAD 가 다른 것(`git add` 만 한 것), 작업 트리와 index 가 다른 것(고치고 저장한 것),
// 그리고 추적하지 않는 파일이다. 무시된 파일은 세지 않는다.
//
// 싼 것부터 본다. 답이 참이면 그 자리에서 끝나므로, 고치는 중인 저장소에서는 대개 끝까지 가지
// 않는다. clean 을 확인하는 것만이 전부를 보는 일이다 — 그것이 이 판정의 바닥값이다.
func gitDirty(ctx context.Context, repo *git.Repository, root string, headTree plumbing.Hash) bool {
	idx, err := repo.Storer.Index()
	if err != nil {
		return false
	}

	if gitStagedChanged(repo, idx, headTree) {
		return true
	}

	if gitWorktreeChanged(ctx, repo, root, idx) {
		return true
	}

	return gitHasUntracked(ctx, root, idx)
}

// gitStagedChanged 는 index 가 HEAD 와 다른지 본다. `git add` 만 하고 commit 하지 않은 것이다.
//
// index 에는 「이 index 를 트리로 쓰면 무엇이 되는가」가 캐시로 딸려 있다(git 의 TREE 확장).
// 그것이 살아 있고 HEAD 트리와 같으면 더 볼 것이 없다 — 대부분이 이 길로 끝난다.
// `git add` 는 그 캐시를 깨므로, 정작 볼 것이 있을 때만 트리를 훑는다.
func gitStagedChanged(repo *git.Repository, idx *index.Index, headTree plumbing.Hash) bool {
	if cached, ok := gitCachedTree(idx); ok {
		return cached != headTree
	}

	inIndex := make(map[string]*index.Entry, len(idx.Entries))
	for _, entry := range idx.Entries {
		// 병합 충돌은 한 경로가 여러 stage 로 들어와 있는 상태다. 그 자체가 dirty 다.
		if entry.Stage != index.Merged {
			return true
		}

		// `git add -N` 로 이름만 올린 것이다. 내용이 index 에 없으므로 git 도 변경으로 센다.
		if entry.IntentToAdd {
			return true
		}

		inIndex[entry.Name] = entry
	}

	tree, err := repo.TreeObject(headTree)
	if err != nil {
		return false
	}

	walker := object.NewTreeWalker(tree, true, nil)
	defer walker.Close()

	seen := 0
	for {
		name, entry, err := walker.Next()
		if err != nil {
			break
		}

		if entry.Mode == filemode.Dir {
			continue
		}

		indexed, ok := inIndex[name]
		if !ok || indexed.Hash != entry.Hash || indexed.Mode != entry.Mode {
			return true
		}

		seen++
	}

	// HEAD 에 없고 index 에만 있는 것이 남아 있으면 새로 add 한 것이다.
	return seen != len(inIndex)
}

// gitCachedTree 는 index 가 들고 있는 「트리로 쓰면 이것이 된다」 는 해시다.
//
// 뿌리 항목은 경로가 빈 문자열이고, 개수가 음수면 깨진 것이라 믿을 수 없다.
func gitCachedTree(idx *index.Index) (plumbing.Hash, bool) {
	if idx.Cache == nil {
		return plumbing.ZeroHash, false
	}

	for _, entry := range idx.Cache.Entries {
		if entry.Path == "" && entry.Entries >= 0 {
			return entry.Hash, true
		}
	}

	return plumbing.ZeroHash, false
}

// gitWorktreeChanged 는 작업 트리가 index 와 다른지 본다. 고쳐서 저장했거나 지운 것이다.
func gitWorktreeChanged(ctx context.Context, repo *git.Repository, root string, idx *index.Index) bool {
	fileMode := gitFileModeMatters(repo)

	for _, entry := range idx.Entries {
		if ctx.Err() != nil {
			return false
		}

		// sparse checkout 으로 내려받지 않은 것이다. 없는 것이 정상이라 세지 않는다.
		if entry.SkipWorktree {
			continue
		}

		// submodule 안이 dirty 한지는 그 저장소를 열어야 안다. 지금은 보지 않는다(ADR-0042).
		if entry.Mode == filemode.Submodule {
			continue
		}

		path := filepath.Join(root, filepath.FromSlash(entry.Name))

		info, err := os.Lstat(path)
		if err != nil {
			return true
		}

		mode, err := filemode.NewFromOSFileMode(info.Mode())
		if err != nil {
			return true
		}

		// 권한이나 종류가 바뀐 것은 내용이 그대로여도 변경이다. 내용만 견주면 `chmod +x` 를
		// 놓친다.
		if mode != entry.Mode && (fileMode || !gitOnlyExecBitDiffers(mode, entry.Mode)) {
			return true
		}

		if gitMetadataMatches(info, entry, idx.ModTime) {
			continue
		}

		// metadata 가 다르다는 것은 「바뀌었을 수도 있다」 까지다. mtime 만 바뀌고 내용은
		// 그대로일 수 있고, index 를 쓴 그 순간에 저장한 파일도 여기로 온다(racy git).
		// 내용을 재야 답이 갈린다 — git 이 하는 것과 같다.
		if gitBlobHash(path, info) != entry.Hash {
			return true
		}
	}

	return false
}

// gitFileModeMatters 는 실행 권한 차이를 변경으로 볼지다.
//
// `core.fileMode` 가 정하고 기본값은 참이다. 권한을 지키지 못하는 파일 시스템에 올린
// 작업 트리에서는 이것을 꺼 두는데, 그때도 권한 차이를 변경으로 보면 저장소가 늘 dirty 로
// 보인다 — 표시가 틀린 채로 굳는 자리다(ADR-0009).
func gitFileModeMatters(repo *git.Repository) bool {
	cfg, err := repo.ConfigScoped(config.SystemScope)
	if err != nil {
		return true
	}

	return cfg.Raw.Section("core").Option("fileMode") != "false"
}

// gitOnlyExecBitDiffers 는 두 mode 가 실행 권한만 다른지다.
// symlink 가 되었거나 submodule 이 된 것은 `core.fileMode` 와 무관하게 변경이다.
func gitOnlyExecBitDiffers(worktree, indexed filemode.FileMode) bool {
	return (worktree == filemode.Regular && indexed == filemode.Executable) ||
		(worktree == filemode.Executable && indexed == filemode.Regular)
}

// gitMetadataMatches 는 내용을 읽지 않고 「바뀌지 않았다」 고 말할 수 있는지 본다.
//
// 참이면 확실히 그대로다. 거짓은 「모른다」 는 뜻이라 부르는 쪽이 내용을 재야 한다.
//
// mode 는 보지 않는다 — 부르는 쪽이 이미 봤고, 그쪽은 `core.fileMode` 까지 셈에 넣는다.
//
// racy git: index 를 쓴 시각과 같거나 그보다 새로운 파일은 metadata 를 믿을 수 없다.
// 초 단위로 같은 순간에 저장되면 크기도 mtime 도 index 와 같은 채로 내용이 다를 수 있다.
// https://git-scm.com/docs/racy-git
func gitMetadataMatches(info os.FileInfo, entry *index.Entry, indexModTime time.Time) bool {
	if uint32(info.Size()) != entry.Size {
		return false
	}

	if !info.ModTime().Equal(entry.ModifiedAt) {
		return false
	}

	if indexModTime.IsZero() || !info.ModTime().Before(indexModTime) {
		return false
	}

	return true
}

// gitBlobHash 는 파일 하나를 git 이 부르는 이름(blob 해시) 으로 바꾼다.
//
// 읽지 못하면 0 이다. 그러면 부르는 쪽에서 index 의 해시와 달라 「바뀌었다」 가 되는데,
// 읽을 수 없는 파일을 그대로 두는 것보다 알리는 편이 낫다.
func gitBlobHash(path string, info os.FileInfo) plumbing.Hash {
	// symlink 는 가리키는 경로 자체가 내용이다. 따라가면 대상 파일을 재게 된다.
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return plumbing.ZeroHash
		}

		hasher := plumbing.NewHasher(plumbing.BlobObject, int64(len(target)))
		hasher.Write([]byte(target))

		return hasher.Sum()
	}

	file, err := os.Open(path)
	if err != nil {
		return plumbing.ZeroHash
	}
	defer file.Close()

	hasher := plumbing.NewHasher(plumbing.BlobObject, info.Size())
	if _, err := io.Copy(hasher, file); err != nil {
		return plumbing.ZeroHash
	}

	return hasher.Sum()
}

// gitHasUntracked 는 추적하지 않고 무시되지도 않는 파일이 있는지 본다.
//
// 처음 하나를 찾으면 훑기를 끊는다. 몇 개인지는 알 필요가 없다.
func gitHasUntracked(ctx context.Context, root string, idx *index.Index) bool {
	tracked := make(map[string]bool, len(idx.Entries))
	for _, entry := range idx.Entries {
		tracked[entry.Name] = true
	}

	found := false
	walkGitFiles(ctx, root, "", func(rel string) bool {
		if tracked[rel] {
			return true
		}

		found = true

		return false
	})

	return found
}
