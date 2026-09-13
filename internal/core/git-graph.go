package core

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// graphCommit 은 커밋 기록 목록의 한 항목이 드는 것이다.
//
// **전문은 담지 않는다.** 상세 화면이 그때 커밋 객체를 다시 읽는다(git-commit.go).
// 수만 개를 담는 자리라 본문까지 들면 그것이 메모리의 거의 전부가 된다.
//
// 보이는 값은 전부 **쓴 사람** 것이다. 날짜도 이름도 그렇다 — `git graph` 가 `%aD`·`%ar`·`%an`
// 을 쓰는 것과 같다. 차례를 정하는 것만 커밋한 때이고 그것은 여기 남지 않는다(graphWalk).
type graphCommit struct {
	hash    plumbing.Hash
	short   string // 짧은 해시. 자리 수는 저장소 크기에서 온다(gitShortHashLen)
	when    time.Time
	author  string
	subject string   // 메시지 첫 줄
	refs    []string // `HEAD → master`, `tag: v1.1.0`, `origin/master`
	parents []plumbing.Hash
}

// graphRow 는 커밋 하나가 그래프에서 차지하는 자리다.
//
// graph 는 이 커밋이 쓰는 그래프 행들이고 commitLine 이 그중 커밋이 선 행이다. 행 수가
// 커밋마다 다르다 — merge 가 연 열을 다시 왼쪽으로 당겨 붙이는 데 몇 행이 들기 때문이다
// (git-graph-draw.go, ADR-0141).
//
// **칸이 무엇인지만 담고 글자는 담지 않는다.** 어느 글자로 그릴지는 터미널에 따라 갈리는
// 판단이라 그리는 쪽이 고른다(ADR-0028).
type graphRow struct {
	commit graphCommit

	graph      [][]graphSymbol
	commitLine int

	// pad 는 행이 모자랄 때 덧붙일 행이다. 살아 있는 열을 그대로 내려 긋기만 한다.
	//
	// git 은 커밋 하나를 한 행으로 낼 수도 있는데 우리 형식은 두 줄이다. 그 자리를 빈 칸으로
	// 두면 지나가던 갈래의 선이 한 행 끊긴다.
	pad []graphSymbol
}

// graphWalk 는 훑는 도중의 자리다.
//
// **job 이 들고 갔다 돌려놓는다.** 시작하는 쪽이 editor 에서 떼어 goroutine 에 실어 보내고
// 끝나면 apply 가 되돌려 놓는다(view-graph.go). 그래서 이것을 두 곳에서 동시에 만지는 자리가
// 없고 잠금도 없다.
type graphWalk struct {
	repo *git.Repository

	// shortLen 은 짧은 해시의 자리 수다. 저장소를 열 때 한 번 정한다 — 훑는 내내 같은 값이라
	// 커밋마다 팩 색인을 다시 읽을 까닭이 없다(ADR-0042).
	shortLen int

	// refs 는 해시마다 붙는 이름들이다. `--decorate` 자리다.
	refs map[plumbing.Hash][]string

	// queue 는 아직 내보내지 않은 앞머리다. 커밋한 때 내림차순이고 같으면 해시 순이다.
	//
	// 담기는 것은 「지금 갈래들의 끝」이라 저장소가 아무리 커도 branch 수만큼이다.
	// heap 을 두지 않고 넣을 때 자리를 찾아 꽂는 것은 그래서다.
	queue []*object.Commit

	// seen 은 이미 큐에 넣었거나 내보낸 해시다. 갈래가 합쳐지는 자리에서 같은 커밋을
	// 두 번 내보내지 않게 한다.
	seen map[plumbing.Hash]bool

	// drawer 는 그래프 행을 내는 상태 기계다. git 의 `--graph` 를 옮긴 것이다.
	//
	// **훑기가 들고 다닌다.** 앞 커밋에서 이어지는 열 배치 위에서만 다음 행이 나온다
	// (git-graph-draw.go, ADR-0141).
	drawer *graphDrawer
}

// newGraphWalk 는 dir 이 든 저장소의 훑기를 세운다. 저장소가 아니면 오류다.
//
// **씨앗은 모든 ref 다**(`--all`). local branch·tag·remote branch 의 끝을 모두 넣는다.
// HEAD 에서 닿는 것만 담으면 방금 만든 branch 도 `git fetch` 로 받은 것도 보이지 않는데,
// 기록을 여는 까닭의 절반이 「저쪽은 어디까지 갔나」다.
func newGraphWalk(dir string) (*graphWalk, error) {
	repo, root, _, ok := openGitRepo(dir)
	if !ok {
		return nil, errors.New("git 저장소가 아닙니다")
	}

	walk := &graphWalk{
		repo:     repo,
		shortLen: gitShortHashLen(gitPackedObjectCount(root)),
		refs:     map[plumbing.Hash][]string{},
		seen:     map[plumbing.Hash]bool{},
		drawer:   newGraphDrawer(),
	}

	if err := walk.collectRefs(); err != nil {
		return nil, err
	}

	// 씨앗이 하나도 없으면 commit 이 아직 없는 저장소다. 갓 `git init` 한 자리가 그렇다.
	if len(walk.queue) == 0 {
		return nil, errors.New("커밋이 없습니다")
	}

	return walk, nil
}

// collectRefs 는 ref 를 모아 씨앗으로 넣고, 해시마다 붙일 이름을 적는다.
//
// 이름의 차례는 git 의 `--decorate` 와 같다. HEAD 가 먼저이고 그다음이 local branch, tag,
// remote branch 다. 한 커밋에 여럿이 붙는 일이 흔해서(방금 push 한 자리) 차례가 흔들리면
// 화면이 이유 없이 달라 보인다.
func (w *graphWalk) collectRefs() error {
	var heads, tags, remotes []plumbing.Reference

	iter, err := w.repo.References()
	if err != nil {
		return err
	}

	err = iter.ForEach(func(ref *plumbing.Reference) error {
		// 상징 참조는 가리키는 쪽이 따로 목록에 있다. 두 번 세지 않는다.
		if ref.Type() != plumbing.HashReference {
			return nil
		}

		switch {
		case ref.Name().IsBranch():
			heads = append(heads, *ref)
		case ref.Name().IsTag():
			tags = append(tags, *ref)
		case ref.Name().IsRemote():
			remotes = append(remotes, *ref)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// HEAD 를 먼저 적는다. branch 를 가리키고 있으면 그 branch 이름에 얹혀 `HEAD → master`
	// 한 덩이가 되고, 떨어져 있으면(detached) 해시에 `HEAD` 만 붙는다.
	head, pointed := w.headLabel()

	w.addRefs(heads, func(name string) string {
		if name == pointed {
			return "HEAD → " + name
		}

		return name
	})
	w.addRefs(tags, func(name string) string { return "tag: " + name })
	w.addRefs(remotes, func(name string) string { return name })

	if head != plumbing.ZeroHash {
		w.refs[head] = append([]string{"HEAD"}, w.refs[head]...)

		w.push(head)
	}

	return nil
}

// headLabel 은 HEAD 를 본다.
//
// branch 를 가리키면 그 짧은 이름을 주고 해시는 ZeroHash 다 — 이름은 branch 쪽에 얹힌다.
// 떨어져 있으면 그 해시를 주고 이름은 빈 문자열이다.
func (w *graphWalk) headLabel() (plumbing.Hash, string) {
	ref, err := w.repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return plumbing.ZeroHash, ""
	}

	if ref.Type() == plumbing.SymbolicReference {
		return plumbing.ZeroHash, ref.Target().Short()
	}

	return ref.Hash(), ""
}

// addRefs 는 ref 묶음 하나를 이름표로 적고 씨앗에 넣는다. label 이 이름을 꾸미는 법이다.
func (w *graphWalk) addRefs(refs []plumbing.Reference, label func(name string) string) {
	// 이름순으로 맞춘다. 저장소가 ref 를 주는 차례는 packed 인지 느슨한지에 따라 갈려서,
	// 그대로 두면 `git gc` 한 번에 화면의 이름 차례가 바뀐다.
	slices.SortFunc(refs, func(a, b plumbing.Reference) int {
		return strings.Compare(a.Name().Short(), b.Name().Short())
	})

	for _, ref := range refs {
		hash, ok := w.peel(ref.Hash())
		if !ok {
			continue
		}

		w.refs[hash] = append(w.refs[hash], label(ref.Name().Short()))
		w.push(hash)
	}
}

// peel 은 ref 가 가리키는 커밋을 찾는다. 주석 tag 는 벗겨서 그 안의 커밋을 준다.
//
// 커밋도 tag 도 아니면(tree 나 blob 에 붙인 tag) 거짓이다. 그런 것은 기록에 설 자리가 없다.
func (w *graphWalk) peel(hash plumbing.Hash) (plumbing.Hash, bool) {
	if _, err := w.repo.CommitObject(hash); err == nil {
		return hash, true
	}

	tag, err := w.repo.TagObject(hash)
	if err != nil {
		return plumbing.ZeroHash, false
	}

	commit, err := tag.Commit()
	if err != nil {
		return plumbing.ZeroHash, false
	}

	return commit.Hash, true
}

// push 는 커밋을 앞머리에 꽂고 그것이 있는 커밋인지를 준다.
//
// 이미 본 것이면 꽂지 않고 참이다 — 어딘가에 이미 서 있다는 뜻이라 부르는 쪽에는 있는 것이다.
// 읽히지 않으면 거짓이고, 얕은 복제(shallow) 로 잘린 자리가 그렇다.
//
// 자리는 커밋한 때 내림차순이고 같으면 해시 순이다. **git 이 세는 차례와 같다** — 쓴 때가
// 아니라 커밋한 때다. rebase 로 옮겨 붙인 커밋이 쓴 때로는 뒤죽박죽이어도 기록은 이어진다.
func (w *graphWalk) push(hash plumbing.Hash) bool {
	if w.seen[hash] {
		return true
	}

	commit, err := w.repo.CommitObject(hash)
	if err != nil {
		return false
	}

	w.seen[hash] = true

	at := sort.Search(len(w.queue), func(i int) bool {
		return graphAfter(commit, w.queue[i])
	})

	w.queue = slices.Insert(w.queue, at, commit)

	return true
}

// graphAfter 는 a 가 b 보다 앞에 서는지다. 나중에 커밋한 것이 위다.
//
// 같은 순간이면 해시 순으로 가른다. 한 스크립트가 커밋을 몰아서 만들면 초가 같은 것이
// 줄줄이 나오는데, 그때 차례가 실행마다 달라지면 시험이 흔들린다.
func graphAfter(a, b *object.Commit) bool {
	if !a.Committer.When.Equal(b.Committer.When) {
		return a.Committer.When.After(b.Committer.When)
	}

	return a.Hash.String() < b.Hash.String()
}

// next 는 다음 커밋 최대 n 개를 내보낸다. 더 남았는지도 같이 준다.
//
// **ctx 가 끊기면 거기까지만 준다.** 반쪽이라도 버리지 않는 것은 훑기가 이어지는 것이라서다 —
// 다음에 다시 부르면 끊긴 자리에서 이어진다.
func (w *graphWalk) next(ctx context.Context, n int) ([]graphRow, bool, error) {
	rows := make([]graphRow, 0, n)

	for len(rows) < n && len(w.queue) > 0 {
		if err := ctx.Err(); err != nil {
			return rows, true, err
		}

		commit := w.queue[0]
		w.queue = w.queue[1:]

		rows = append(rows, w.emit(commit))
	}

	return rows, len(w.queue) > 0, nil
}

// emit 은 커밋 하나를 내보내며 그래프 행을 받는다.
//
// 열을 옮기는 일은 전부 drawer 안이다. 여기 남은 것은 「어느 부모가 읽히는가」뿐이다.
func (w *graphWalk) emit(commit *object.Commit) graphRow {
	// 부모를 먼저 큐에 넣는다. 읽히지 않는 부모(얕은 복제) 는 레인에서도 뺀다 — 아무도
	// 서지 않을 열을 열어두면 그 선이 화면 끝까지 내려간다.
	parents := make([]plumbing.Hash, 0, len(commit.ParentHashes))
	for _, hash := range commit.ParentHashes {
		if w.push(hash) {
			parents = append(parents, hash)
		}
	}

	graph, at, pad := w.drawer.draw(commit.Hash, parents)

	return graphRow{
		commit:     w.describe(commit),
		graph:      graph,
		commitLine: at,
		pad:        pad,
	}
}

// describe 는 커밋 객체에서 목록에 쓸 것만 뽑는다.
func (w *graphWalk) describe(commit *object.Commit) graphCommit {
	hash := commit.Hash.String()

	return graphCommit{
		hash:    commit.Hash,
		short:   hash[:min(w.shortLen, len(hash))],
		when:    commit.Author.When,
		author:  commit.Author.Name,
		subject: commitSubject(commit.Message),
		refs:    w.refs[commit.Hash],
		parents: slices.Clone(commit.ParentHashes),
	}
}

// commitSubject 는 메시지의 첫 줄이다. 비어 있으면 빈 문자열이다.
func commitSubject(message string) string {
	line, _, _ := strings.Cut(message, "\n")

	return strings.TrimSpace(line)
}
