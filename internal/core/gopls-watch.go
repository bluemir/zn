package core

import (
	"context"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	"github.com/bluemir/zn/internal/lsp"
)

// 밖에서 바뀐 파일을 gopls 에 알리는 자리다(ADR-0092).
//
// **gopls 는 스스로 디스크를 감시하지 않는다.** 클라이언트가 보내는
// `workspace/didChangeWatchedFiles` 에 기댄다. 알리지 않으면 처음 읽은 사본을 끝까지 든다 —
// 재보니 밖에서 만든 파일의 사용처가 몇 초를 기다려도 0 개였고, 알림 하나에 500ms 뒤 잡혔다.
//
// **여기서 잡는 것은 git 이 움직인 것뿐이다.** `git checkout`·`pull`·`rebase` 로 HEAD 가
// 옮겨가면 그 사이에 달라진 파일을 git 이 목록으로 준다. 다른 도구가 고친 것은 아직 못 잡는다 —
// 그것은 훑거나 감시해야 하고 값이 달라서 갈랐다(ADR-0092 §4).

// goplsWatchChanges 는 HEAD 가 from 에서 to 로 옮겨간 사이에 달라진 Go 파일을 알린다.
//
// 백그라운드 작업 안에서 부른다. tree 둘을 견주는 일이라 Update 에서 부를 값이 아니다(ADR-0030).
//
// 알리지 못하는 경우(저장소를 못 열거나 commit 이 사라진 경우) 는 조용히 지나간다. 이것은
// 있으면 정확해지는 알림이고, 없으면 예전처럼 낡은 채로 도는 것이라 알릴 실패가 아니다.
func goplsWatchChanges(ctx context.Context, client *lsp.Client, from, to string) {
	if client == nil || from == "" || to == "" || from == to {
		return
	}

	changes := gitTreeChanges(ctx, from, to)

	_ = client.FilesChanged(changes)
}

// gitTreeChanges 는 두 commit 사이에 달라진 Go 파일들이다.
//
// **Go 파일만 고른다.** gopls 가 볼 것만 보낸다 — 브랜치를 옮기면 수백 개가 달라질 수 있고,
// 그 전부를 보내면 서버가 읽을 이유 없는 파일을 읽는다.
//
// **열어 둔 파일도 걸러내지 않는다.** overlay 가 디스크를 이기므로(ADR-0092) 그 파일에 대한
// 알림은 서버가 알아서 무시한다. 거르는 쪽이 「무엇이 열려 있나」를 여기까지 들고 와야 해서
// 값이 더 크다.
func gitTreeChanges(ctx context.Context, from, to string) []lsp.FileChange {
	repo, root, _, ok := openGitRepo(".")
	if !ok {
		return nil
	}

	fromTree, ok := gitTreeOf(repo, from)
	if !ok {
		return nil
	}

	toTree, ok := gitTreeOf(repo, to)
	if !ok {
		return nil
	}

	diff, err := fromTree.DiffContext(ctx, toTree)
	if err != nil {
		return nil
	}

	changes := make([]lsp.FileChange, 0, len(diff))
	for _, change := range diff {
		kind, name, ok := gitChangeKind(change)
		if !ok || !isGoFile(name) {
			continue
		}

		changes = append(changes, lsp.FileChange{
			Path: filepath.Join(root, filepath.FromSlash(name)),
			Kind: kind,
		})
	}

	return changes
}

// gitTreeOf 는 그 해시가 가리키는 commit 의 tree 다.
//
// 없을 수 있다. `git gc` 뒤나 얕은 clone 에서 앞 commit 이 사라진 경우다.
func gitTreeOf(repo *git.Repository, hash string) (*object.Tree, bool) {
	commit, err := repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, false
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, false
	}

	return tree, true
}

// gitChangeKind 는 tree 차이 하나를 규격의 갈래와 경로로 바꾼다.
//
// 이름이 바뀐 것은 지운 것과 만든 것 둘로 온다. go-git 의 tree 차이가 rename 을 따로
// 세지 않기 때문인데, gopls 에게는 그 둘이 곧 답이라 맞춰 줄 것이 없다.
func gitChangeKind(change *object.Change) (lsp.FileChangeKind, string, bool) {
	action, err := change.Action()
	if err != nil {
		return 0, "", false
	}

	switch action {
	case merkletrie.Insert:
		return lsp.FileCreated, change.To.Name, true
	case merkletrie.Delete:
		return lsp.FileDeleted, change.From.Name, true
	case merkletrie.Modify:
		return lsp.FileChanged, change.To.Name, true
	}

	return 0, "", false
}
