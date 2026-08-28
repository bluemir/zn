package core

import (
	"context"
	"io"
	"os"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// gitDirty 는 commit 하지 않은 것이 하나라도 있는지 답한다.
//
// `git status` 가 clean 이라고 부르는 것과 같은 기준이다(ADR-0009) — 셋을 다 본다.
// index 와 HEAD 가 다른 것(`git add` 만 한 것), 작업 트리와 index 가 다른 것(고치고 저장한 것),
// 그리고 추적하지 않는 파일이다. 무시된 파일은 세지 않는다.
//
// **표를 세어서 답한다.** 예전에는 싼 것부터 보다가 처음 하나에서 끊었는데, 트리에 파일별
// 표시가 서면서 어차피 전부를 훑게 되었다. 두 벌로 두면 `*` 와 트리 마커가 서로 다른 것을
// 말하는 날이 온다 — 판정은 하나여야 한다(ADR-0094 §2).
func gitDirty(ctx context.Context, repo *git.Repository, root string, headTree plumbing.Hash) bool {
	return len(gitFileChanges(ctx, repo, root, headTree)) > 0
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
