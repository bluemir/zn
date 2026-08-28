package core

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// 어느 파일이 HEAD 와 다른지를 모으는 자리다(ADR-0094).
//
// statusBar 의 `*` 하나였던 판정(git-dirty.go) 이 여기서 파일별 표가 되었다. 트리가 그 표를
// 읽어 이름 옆에 마커를 그린다. 「하나라도 있는가」는 이제 표가 비었는지로 답한다.

// gitChange 는 그 파일이 HEAD 와 어떻게 다른지다.
type gitChange byte

const (
	gitChangeNone      gitChange = iota
	gitChangeModified            // HEAD 에 있고 내용이 다르다. 지운 것도 여기 든다
	gitChangeUntracked           // git 이 모르는 파일이다. 무시된 것은 여기 들지 않는다
)

// gitStageMerged 는 충돌이 없는 index 항목의 stage 다.
//
// **go-git 의 `index.Merged` 를 쓰지 않는다.** 그 상수는 1 인데 디코더는 파일에 적힌 값을
// 그대로 넣어서(`Stage(flags>>12) & 0x3`) 충돌이 없는 항목이 0 으로 온다. 그대로 견주면
// **모든 항목이 충돌로 읽힌다** — 재서 확인했다(docs/issues/0002).
const gitStageMerged = 0

// gitChanges 는 경로마다 무엇이 달라졌는지다. 비어 있으면 저장소가 깨끗한 것이다.
//
// **키가 절대 경로다.** 트리의 항목이 절대 경로를 들고 있어서(sidebar.go 의 treeNode) 그리는
// 자리가 이 표를 그대로 물을 수 있다. index 의 경로는 뿌리 기준 슬래시 표기라 여기서 한 번
// 맞춰 둔다 — 그리는 자리마다 다시 맞추면 그 셈이 여러 군데로 흩어진다(openGitRepo 와 같은 태도다).
//
// **디렉터리도 담는다.** 안에 달라진 것이 있으면 그 위 디렉터리마다 표시가 선다. 접혀 있는
// 디렉터리에서도 「이 안에 뭔가 있다」가 보여야 해서다. 갈래는 변경이 이긴다 — 안에 고친 파일과
// 새 파일이 섞여 있으면 commit 할 것이 있다는 쪽이 더 급한 소식이다.
type gitChanges map[string]gitChange

// at 은 그 경로의 갈래다. 표가 없으면(저장소가 아니거나 아직 안 읽었으면) 없음이다.
func (c gitChanges) at(path string) gitChange {
	return c[path]
}

// gitFileChanges 는 HEAD 와 다른 파일을 모두 모은다.
//
// `git status` 와 같은 셋을 본다(ADR-0009). index 와 HEAD 가 다른 것(`git add` 만 한 것),
// 작업 트리와 index 가 다른 것(고치고 저장한 것), 그리고 추적하지 않는 파일이다.
//
// **기준이 HEAD 다.** `git add` 를 해도 표시가 남는다. 「마지막 commit 이후 바뀐 것」이 사람이
// 묻는 것이고, stage 한 것을 표시에서 빼면 commit 을 빠뜨리는 쪽으로 틀린다(ADR-0094 §1).
//
// **끊기면 nil 이다.** 도중에 멈춘 표는 「깨끗하다」와 구별되지 않는다 — 부르는 쪽이 그것을
// 화면에 올리면 편집기를 끝내는 길에 표시가 사라진다(readGitStatus 가 같은 자리를 지킨다).
func gitFileChanges(ctx context.Context, repo *git.Repository, root string, headTree plumbing.Hash) gitChanges {
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil
	}

	changes := gitChanges{}

	gitCollectStaged(repo, idx, headTree, root, changes)
	gitCollectWorktree(ctx, repo, root, idx, changes)
	gitCollectUntracked(ctx, root, idx, changes)

	if ctx.Err() != nil {
		return nil
	}

	gitFillParents(root, changes)

	return changes
}

// gitCollectStaged 는 index 가 HEAD 와 다른 것을 담는다. `git add` 만 하고 commit 하지 않은 것이다.
//
// index 에는 「이 index 를 트리로 쓰면 무엇이 되는가」가 캐시로 딸려 있다(git 의 TREE 확장).
// 그것이 살아 있고 HEAD 트리와 같으면 **이 통과를 통째로 건너뛴다** — 대부분이 이 길이다.
// `git add` 는 그 캐시를 깨므로, 정작 볼 것이 있을 때만 트리를 훑는다.
func gitCollectStaged(repo *git.Repository, idx *index.Index, headTree plumbing.Hash, root string, changes gitChanges) {
	if cached, ok := gitCachedTree(idx); ok && cached == headTree {
		return
	}

	inIndex := make(map[string]*index.Entry, len(idx.Entries))
	for _, entry := range idx.Entries {
		// 병합 충돌은 한 경로가 여러 stage 로 들어와 있는 상태다. 그 자체가 변경이다.
		// `git add -N` 로 이름만 올린 것(IntentToAdd) 도 내용이 index 에 없어서 git 이 변경으로 센다.
		if entry.Stage != gitStageMerged || entry.IntentToAdd {
			changes[gitAbsPath(root, entry.Name)] = gitChangeModified
		}

		inIndex[entry.Name] = entry
	}

	tree, err := repo.TreeObject(headTree)
	if err != nil {
		return
	}

	walker := object.NewTreeWalker(tree, true, nil)
	defer walker.Close()

	inHead := make(map[string]bool, len(inIndex))

	for {
		name, entry, err := walker.Next()
		if err != nil {
			break
		}

		if entry.Mode == filemode.Dir {
			continue
		}

		inHead[name] = true

		// HEAD 에 있는데 index 에 없으면 `git rm` 한 것이다. 파일이 이미 없으므로 트리에는
		// 그 행이 없고, 표시는 그 위 디렉터리에만 선다(gitFillParents).
		indexed, ok := inIndex[name]
		if !ok || indexed.Hash != entry.Hash || indexed.Mode != entry.Mode {
			changes[gitAbsPath(root, name)] = gitChangeModified
		}
	}

	// HEAD 에 없고 index 에만 있으면 새로 add 한 것이다. 추적은 되고 있으므로 새 파일이
	// 아니라 변경이다 — `?` 는 git 이 아직 모르는 것에만 붙는다.
	for name := range inIndex {
		if !inHead[name] {
			changes[gitAbsPath(root, name)] = gitChangeModified
		}
	}
}

// gitCollectWorktree 는 작업 트리가 index 와 다른 것을 담는다. 고쳐서 저장했거나 지운 것이다.
func gitCollectWorktree(ctx context.Context, repo *git.Repository, root string, idx *index.Index, changes gitChanges) {
	fileMode := gitFileModeMatters(repo)

	for _, entry := range idx.Entries {
		if ctx.Err() != nil {
			return
		}

		// sparse checkout 으로 내려받지 않은 것이다. 없는 것이 정상이라 세지 않는다.
		if entry.SkipWorktree {
			continue
		}

		// submodule 안이 dirty 한지는 그 저장소를 열어야 안다. 지금은 보지 않는다(ADR-0042).
		if entry.Mode == filemode.Submodule {
			continue
		}

		path := gitAbsPath(root, entry.Name)

		info, err := os.Lstat(path)
		if err != nil {
			// 지운 것이다. 파일이 없으니 트리에는 행이 없고 위 디렉터리에만 표시가 선다.
			changes[path] = gitChangeModified

			continue
		}

		mode, err := filemode.NewFromOSFileMode(info.Mode())
		if err != nil {
			changes[path] = gitChangeModified

			continue
		}

		// 권한이나 종류가 바뀐 것은 내용이 그대로여도 변경이다. 내용만 견주면 `chmod +x` 를 놓친다.
		if mode != entry.Mode && (fileMode || !gitOnlyExecBitDiffers(mode, entry.Mode)) {
			changes[path] = gitChangeModified

			continue
		}

		if gitMetadataMatches(info, entry, idx.ModTime) {
			continue
		}

		// metadata 가 다르다는 것은 「바뀌었을 수도 있다」 까지다. mtime 만 바뀌고 내용은
		// 그대로일 수 있고, index 를 쓴 그 순간에 저장한 파일도 여기로 온다(racy git).
		// 내용을 재야 답이 갈린다 — git 이 하는 것과 같다.
		if gitBlobHash(path, info) != entry.Hash {
			changes[path] = gitChangeModified
		}
	}
}

// gitCollectUntracked 는 추적하지 않고 무시되지도 않는 파일을 담는다.
//
// **전부 훑는다.** 「하나라도 있는가」만 묻던 때는 처음 하나에서 끊었는데(ADR-0009), 트리가
// 파일마다 물으므로 이제 끊을 자리가 없다. 무시된 것을 걸러내는 일은 walkGitFiles 가 한다.
func gitCollectUntracked(ctx context.Context, root string, idx *index.Index, changes gitChanges) {
	tracked := make(map[string]bool, len(idx.Entries))
	for _, entry := range idx.Entries {
		tracked[entry.Name] = true
	}

	walkGitFiles(ctx, root, "", func(rel string) bool {
		if !tracked[rel] {
			changes[gitAbsPath(root, rel)] = gitChangeUntracked
		}

		return true
	})
}

// gitFillParents 는 달라진 것들의 위 디렉터리에도 표시를 올린다.
//
// 뿌리 자신에는 올리지 않는다. 트리의 뿌리 행은 늘 표시가 서 있게 되어 아무것도 가리키지 않는다.
//
// **여기서 한 번에 올린다.** 그리는 자리에서 「내 아래에 뭔가 있나」를 물으면 행마다 표를
// 훑어야 한다 — 그 값이 트리 높이에 비례해 든다.
func gitFillParents(root string, changes gitChanges) {
	// 훑으면서 넣으면 방금 넣은 디렉터리가 다시 훑기에 걸릴 수 있다. 먼저 떠 놓고 넣는다.
	files := make(map[string]gitChange, len(changes))
	maps.Copy(files, changes)

	for path, change := range files {
		for dir := filepath.Dir(path); strings.HasPrefix(dir, root) && dir != root; dir = filepath.Dir(dir) {
			// 변경이 이긴다. 새 파일만 있는 디렉터리에만 `?` 가 선다.
			if changes[dir] != gitChangeModified {
				changes[dir] = change
			}
		}
	}
}

// gitAbsPath 는 index 의 경로 표기(뿌리 기준 슬래시) 를 절대 경로로 바꾼다.
func gitAbsPath(root, name string) string {
	return filepath.Join(root, filepath.FromSlash(name))
}
