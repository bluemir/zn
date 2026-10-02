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
	commitgraph "github.com/go-git/go-git/v5/plumbing/format/commitgraph/v2"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
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

	// tips 는 아직 차례에 세우지 않은 씨앗이다. 첫 next 가 비운다(start).
	//
	// 저장소를 여는 자리에서는 해시만 모은다. 씨앗의 세대 번호를 아는 데 commit-graph 가
	// 없으면 기록 전체를 읽어야 해서, 그 일은 화면을 붙잡지 않는 job 안에서 한다.
	tips []plumbing.Hash

	// commitGraph 는 `.git/objects/info/commit-graph` 다. 없거나 읽히지 않으면 nil 이다.
	// 다 훑으면 닫는다(next).
	commitGraph commitgraph.Index

	// nodes 는 세대 번호와 부모를 셈한 커밋이다. 읽히지 않는 커밋(얕은 복제로 잘린 자리) 은
	// nil 로 적어서 다시 읽으러 가지 않는다.
	nodes map[plumbing.Hash]*graphGeneration

	// explore 는 자식 수를 세러 내려가는 앞머리다. 세대 번호 내림차순이다.
	//
	// **차례(queue) 보다 앞서 내려간다.** 커밋 하나를 내보내려면 그 부모의 자식이 모두
	// 나갔는지 알아야 하고, 자식은 늘 세대 번호가 부모보다 크다. 그래서 부모의 세대까지만
	// 내려가 세면 된다 — git 이 commit-graph 로 `--topo-order` 를 게으르게 내는 방법이다.
	explore []plumbing.Hash

	// explored 는 explore 에 한 번이라도 넣은 해시다. 같은 커밋의 자식을 두 번 세지 않는다.
	explored map[plumbing.Hash]bool

	// indegree 는 커밋마다 아직 나가지 않은 자식 수다. 0 이 되면 차례에 선다.
	indegree map[plumbing.Hash]int

	// queue 는 내보낼 차례를 기다리는 커밋이다. 자식이 모두 나간 부모와, 아직 가르지 않은
	// 씨앗이 든다(start). 커밋한 때 내림차순이고 같으면 먼저 들어온 것이 앞이다(addReady).
	//
	// **커밋한 때는 자식이 모두 나간 것들 사이의 차례일 뿐이다.** 시계는 틀린다. 자식이
	// 부모보다 이르게 커밋된 자리가 kubernetes 에 390 군데, linux 에 327 군데 있다.
	// 커밋한 때만으로 줄을 세우면 거기서 부모가 자식 위로 올라가 합쳐진 갈래가 따로 열린다.
	//
	// 담기는 것은 「지금 갈래들의 끝」이라 저장소가 아무리 커도 ref 수만큼이다.
	// heap 을 두지 않고 넣을 때 자리를 찾아 꽂는 것은 그래서다.
	queue []*object.Commit

	// queued 는 지금 queue 에 든 해시다.
	queued map[plumbing.Hash]bool

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
		repo:        repo,
		shortLen:    gitShortHashLen(gitPackedObjectCount(root)),
		refs:        map[plumbing.Hash][]string{},
		commitGraph: openCommitGraph(repo),
		nodes:       map[plumbing.Hash]*graphGeneration{},
		explored:    map[plumbing.Hash]bool{},
		indegree:    map[plumbing.Hash]int{},
		queued:      map[plumbing.Hash]bool{},
		drawer:      newGraphDrawer(),
	}

	if err := walk.collectRefs(); err != nil {
		return nil, err
	}

	// 씨앗이 하나도 없으면 commit 이 아직 없는 저장소다. 갓 `git init` 한 자리가 그렇다.
	if len(walk.tips) == 0 {
		return nil, errors.New("커밋이 없습니다")
	}

	return walk, nil
}

// openCommitGraph 는 저장소의 commit-graph 파일을 연다. 없으면 nil 이다.
//
// **없어도 차례는 같다.** 세대 번호를 직접 세는데 그러려면 기록 전체를 읽어야 해서 첫 화면이
// 늦을 뿐이다. kubernetes 에서 10 초, 파일이 있으면 1 초 안쪽이다(ADR-0150). 파일은 `git gc`
// 가 쓰고 갓 받은 clone 에는 없다.
func openCommitGraph(repo *git.Repository) commitgraph.Index {
	storage, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return nil
	}

	index, err := commitgraph.OpenChainOrFileIndex(storage.Filesystem())
	if err != nil {
		return nil
	}

	return index
}

// collectRefs 는 ref 를 모아 씨앗으로 넣고, 해시마다 붙일 이름을 적는다.
//
// 이름의 차례는 git 의 `--decorate` 와 같다. HEAD 가 먼저이고 그다음이 local branch, tag,
// remote branch 다. 한 커밋에 여럿이 붙는 일이 흔해서(방금 push 한 자리) 차례가 흔들리면
// 화면이 이유 없이 달라 보인다.
func (w *graphWalk) collectRefs() error {
	var heads, tags, remotes, symbolic []plumbing.Reference

	iter, err := w.repo.References()
	if err != nil {
		return err
	}

	err = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()
		if !name.IsBranch() && !name.IsTag() && !name.IsRemote() {
			return nil
		}

		// 상징 참조(`origin/HEAD`) 는 이름표를 달지 않는다. 가리키는 쪽이 따로 목록에 있다.
		// 씨앗의 차례에는 든다(addTips).
		if ref.Type() != plumbing.HashReference {
			resolved, err := w.repo.Reference(name, true)
			if err == nil {
				symbolic = append(symbolic, *plumbing.NewHashReference(name, resolved.Hash()))
			}

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

	w.addTips(slices.Concat(heads, tags, remotes, symbolic))

	if head != plumbing.ZeroHash {
		w.refs[head] = append([]string{"HEAD"}, w.refs[head]...)

		w.addTip(head)
	}

	return nil
}

// addTips 는 ref 를 씨앗으로 넣는다. **차례는 전체 이름순이다**(`refs/heads/` → `refs/remotes/`
// → `refs/tags/`). 이름표의 차례와 다르다.
//
// git 의 `--all` 이 ref 를 넣는 차례가 이것이고 HEAD 는 그 뒤다. 커밋한 때가 같은 씨앗은 이
// 차례로 나온다(addReady). 같은 커밋을 가리키는 ref 가 여럿이면 앞의 것 자리다.
func (w *graphWalk) addTips(refs []plumbing.Reference) {
	slices.SortFunc(refs, func(a, b plumbing.Reference) int {
		return strings.Compare(a.Name().String(), b.Name().String())
	})

	for _, ref := range refs {
		hash, ok := w.peel(ref.Hash())
		if !ok {
			continue
		}

		w.addTip(hash)
	}
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

// addRefs 는 ref 묶음 하나를 이름표로 적는다. label 이 이름을 꾸미는 법이다.
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

// addTip 은 ref 가 가리키는 커밋을 씨앗으로 적는다. 여러 ref 가 한 커밋을 가리키면 한 번만이다.
func (w *graphWalk) addTip(hash plumbing.Hash) {
	if slices.Contains(w.tips, hash) {
		return
	}

	w.tips = append(w.tips, hash)
}

// graphGeneration 은 차례를 정하는 데 드는 것만 담은 커밋이다.
//
// 세대 번호는 뿌리가 1 이고 커밋마다 부모 중 가장 큰 것에 1 을 더한 값이다. **자식은 늘
// 부모보다 크다.** 시계와 달리 틀릴 수 없는 값이라 이것으로 「아직 안 나간 자식이 있는가」를
// 가른다.
type graphGeneration struct {
	generation uint64
	parents    []plumbing.Hash
}

// lookup 은 커밋의 세대 번호와 부모를 준다. 읽히지 않는 커밋이면 nil 이다.
//
// commit-graph 에 있으면 거기서 바로 읽는다. 없으면(파일이 없거나 그 뒤에 생긴 커밋)
// 부모들의 세대를 먼저 세고 그 위에 1 을 얹는다. **파일이 없는 저장소에서 처음 부르면
// 기록 전체를 읽는다.** 그 값이 nodes 에 남아 다음부터는 바로 나온다.
//
// 재귀를 쓰지 않고 손수 쌓는다. linux 는 뿌리까지 백만 단이 넘는다.
//
// **ctx 가 끊기면 다 센 것만 남긴다.** nodes 에는 끝난 값만 들어가므로 다시 부르면 이어진다.
func (w *graphWalk) lookup(ctx context.Context, hash plumbing.Hash) (*graphGeneration, error) {
	if node, known := w.nodes[hash]; known {
		return node, nil
	}

	// waiting 은 부모의 세대를 기다리는 커밋의 부모 목록이다.
	waiting := map[plumbing.Hash][]plumbing.Hash{}
	stack := []plumbing.Hash{hash}

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		top := stack[len(stack)-1]

		if _, known := w.nodes[top]; known {
			stack = stack[:len(stack)-1]
			continue
		}

		parents, read := waiting[top]
		if !read {
			parents, generation, ok := w.read(top)

			switch {
			case !ok:
				w.nodes[top] = nil
				stack = stack[:len(stack)-1]
			case generation > 0:
				w.nodes[top] = &graphGeneration{generation: generation, parents: parents}
				stack = stack[:len(stack)-1]
			default:
				// 부모를 위에 쌓는다. 그것들이 다 셈해진 뒤에 이 자리로 돌아온다.
				waiting[top] = parents
				for _, parent := range parents {
					if _, known := w.nodes[parent]; !known {
						stack = append(stack, parent)
					}
				}
			}

			continue
		}

		generation := uint64(1)
		for _, parent := range parents {
			if node := w.nodes[parent]; node != nil {
				generation = max(generation, node.generation+1)
			}
		}

		w.nodes[top] = &graphGeneration{generation: generation, parents: parents}
		delete(waiting, top)
		stack = stack[:len(stack)-1]
	}

	return w.nodes[hash], nil
}

// read 는 커밋 하나의 부모와 commit-graph 가 적은 세대 번호를 준다. 읽히지 않으면 거짓이다.
//
// 세대 번호가 0 이면 모른다는 뜻이다. 파일에 없는 커밋이 그렇고, 옛 git 이 쓴 파일은 모든
// 커밋에 0 을 적었다.
func (w *graphWalk) read(hash plumbing.Hash) ([]plumbing.Hash, uint64, bool) {
	if w.commitGraph != nil {
		if at, err := w.commitGraph.GetIndexByHash(hash); err == nil {
			if data, err := w.commitGraph.GetCommitDataByIndex(at); err == nil {
				return data.ParentHashes, data.Generation, true
			}
		}
	}

	commit, err := w.repo.CommitObject(hash)
	if err != nil {
		return nil, 0, false
	}

	return commit.ParentHashes, 0, true
}

// start 는 씨앗을 차례에 세운다. 다른 씨앗의 조상인 것은 자식이 나갈 때까지 기다린다.
//
// **씨앗은 일단 차례에 넣고 꺼낼 때 가른다**(next). 넣는 자리에서 가르려면 가장 낮은 씨앗의
// 세대까지 내려가 세어야 하는데, 오래된 tag 하나가 그 자리를 기록 끝으로 끌어내린다. git 이
// 그렇게 해서 commit-graph 가 있어도 linux 에서 3 초가 든다.
func (w *graphWalk) start(ctx context.Context) error {
	for _, tip := range w.tips {
		node, err := w.lookup(ctx, tip)
		if err != nil {
			return err
		}

		// 씨앗은 peel 이 커밋인 것을 확인했다. 그래도 읽히지 않으면 얕은 복제의 끝이다.
		if node == nil {
			continue
		}

		w.addExplore(tip)
		w.addReady(tip)
	}

	w.tips = nil

	return nil
}

// addExplore 는 커밋을 explore 에 세대 번호 내림차순으로 꽂는다. 한 번 넣은 것은 다시 넣지 않는다.
//
// 부르는 쪽이 lookup 을 먼저 했다. nodes 에 값이 있다.
func (w *graphWalk) addExplore(hash plumbing.Hash) {
	if w.explored[hash] {
		return
	}

	w.explored[hash] = true

	generation := w.nodes[hash].generation
	at := sort.Search(len(w.explore), func(i int) bool {
		return w.nodes[w.explore[i]].generation < generation
	})

	w.explore = slices.Insert(w.explore, at, hash)
}

// exploreTo 는 세대 번호가 generation 보다 큰 커밋을 모두 내려가며 부모마다 자식 수를 센다.
//
// 그러고 나면 세대가 generation 인 커밋의 자식은 모두 세어진 것이다. 자식은 늘 세대가 더 크다.
//
// **끊겨도 이어진다.** 한 커밋을 다 센 뒤에야 explore 에서 뺀다.
func (w *graphWalk) exploreTo(ctx context.Context, generation uint64) error {
	for len(w.explore) > 0 {
		top := w.nodes[w.explore[0]]
		if top.generation <= generation {
			return nil
		}

		for _, parent := range top.parents {
			if _, err := w.lookup(ctx, parent); err != nil {
				return err
			}
		}

		w.explore = w.explore[1:]

		for _, parent := range top.parents {
			if w.nodes[parent] == nil {
				continue
			}

			w.indegree[parent]++
			w.addExplore(parent)
		}
	}

	return nil
}

// addReady 는 커밋을 차례에 꽂는다.
//
// 자식이 모두 나간 부모와 아직 가르지 않은 씨앗이 들어온다.
//
// 자리는 커밋한 때 내림차순이다. **git 의 `--date-order` 와 같다** — 쓴 때가 아니라 커밋한
// 때다. rebase 로 옮겨 붙인 커밋이 쓴 때로는 뒤죽박죽이어도 기록은 이어진다.
//
// **같은 순간이면 먼저 들어온 것이 앞이다.** git 의 차례(`prio_queue`) 가 그렇게 가른다.
// 한 스크립트가 커밋을 몰아서 만들거나 merge 를 같은 초에 여럿 받으면 초가 같은 것이 줄줄이
// 나온다. 그래서 들어오는 순간도 git 과 같아야 한다. 씨앗은 처음에 ref 이름순으로 들어오고
// (addTips), 부모는 마지막 자식이 나가는 순간 부모 차례대로 들어온다(emit).
//
// 이미 차례에 있으면 아직 가르지 않은 씨앗이다. 다른 씨앗의 조상이라 git 에서는 이제야 들어오는
// 것이므로, 빼서 지금 들어온 자리에 다시 꽂는다.
func (w *graphWalk) addReady(hash plumbing.Hash) {
	var commit *object.Commit

	if w.queued[hash] {
		at := slices.IndexFunc(w.queue, func(queued *object.Commit) bool {
			return queued.Hash == hash
		})

		commit = w.queue[at]
		w.queue = slices.Delete(w.queue, at, at+1)
	} else {
		read, err := w.repo.CommitObject(hash)
		if err != nil {
			return
		}

		commit = read
		w.queued[hash] = true
	}

	// 같은 순간인 것들의 뒤에 선다.
	at := sort.Search(len(w.queue), func(i int) bool {
		return commit.Committer.When.After(w.queue[i].Committer.When)
	})

	w.queue = slices.Insert(w.queue, at, commit)
}

// next 는 다음 커밋 최대 n 개를 내보낸다. 더 남았는지도 같이 준다.
//
// **ctx 가 끊기면 거기까지만 준다.** 반쪽이라도 버리지 않는 것은 훑기가 이어지는 것이라서다 —
// 다음에 다시 부르면 끊긴 자리에서 이어진다.
func (w *graphWalk) next(ctx context.Context, n int) ([]graphRow, bool, error) {
	rows := make([]graphRow, 0, n)

	if len(w.tips) > 0 {
		if err := w.start(ctx); err != nil {
			return rows, true, err
		}
	}

	for len(rows) < n && len(w.queue) > 0 {
		if err := ctx.Err(); err != nil {
			return rows, true, err
		}

		commit := w.queue[0]

		// 제 세대까지 내려가 세면 이 커밋의 자식이 모두 세어진다. 아직 안 나간 자식이 있으면
		// 차례에서 뺀다 — 다른 씨앗의 조상인 씨앗이 그렇고, 마지막 자식이 나갈 때 다시 들어온다.
		if err := w.exploreTo(ctx, w.nodes[commit.Hash].generation); err != nil {
			return rows, true, err
		}

		if w.indegree[commit.Hash] > 0 {
			w.queue = w.queue[1:]
			delete(w.queued, commit.Hash)

			continue
		}

		// 부모마다 그 세대까지 내려가 센다. 그래야 내보낸 뒤 자식 수가 0 이 된 부모가 정말로
		// 자식이 다 나간 것이다. 끊기면 이 커밋은 아직 차례에 남아 있다.
		for _, hash := range commit.ParentHashes {
			node, err := w.lookup(ctx, hash)
			if err != nil {
				return rows, true, err
			}

			if node == nil {
				continue
			}

			if err := w.exploreTo(ctx, node.generation); err != nil {
				return rows, true, err
			}
		}

		w.queue = w.queue[1:]
		delete(w.queued, commit.Hash)

		rows = append(rows, w.emit(commit))
	}

	more := len(w.queue) > 0

	// 다 훑었으면 commit-graph 파일을 닫는다. 화면을 다시 열면 훑기를 새로 세운다.
	if !more && w.commitGraph != nil {
		w.commitGraph.Close()
		w.commitGraph = nil
	}

	return rows, more, nil
}

// emit 은 커밋 하나를 내보내며 그래프 행을 받는다.
//
// 열을 옮기는 일은 전부 drawer 안이다. 여기 남은 것은 「어느 부모가 읽히는가」와 「어느
// 부모가 이제 나갈 수 있는가」다.
func (w *graphWalk) emit(commit *object.Commit) graphRow {
	// 읽히지 않는 부모(얕은 복제) 는 레인에서도 뺀다 — 아무도 서지 않을 열을 열어두면 그 선이
	// 화면 끝까지 내려간다.
	parents := make([]plumbing.Hash, 0, len(commit.ParentHashes))
	for _, hash := range commit.ParentHashes {
		if w.nodes[hash] == nil {
			continue
		}

		parents = append(parents, hash)

		w.indegree[hash]--
		if w.indegree[hash] == 0 {
			w.addReady(hash)
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
