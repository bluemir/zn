package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
	"github.com/fsnotify/fsnotify"

	"github.com/bluemir/zn/internal/lsp"
)

// 디스크를 감시하는 자리다(ADR-0093).
//
// **이것 없이는 다른 도구가 고친 파일을 끝까지 모른다.** 주기 검사는 활성 buffer 하나만 보고
// (ADR-0044) 언어 서버에 알리는 것은 HEAD 가 움직일 때뿐이라(ADR-0092) 다른 창의 편집기나 AI 가
// 고친 파일은 어느 쪽에도 걸리지 않는다.
//
// **감시는 앞잡이일 뿐이다.** 이벤트를 받으면 검사를 앞당길 뿐 판정은 그대로 sha256 이 한다
// (ADR-0015). 주기 검사도 그대로 남는다 — 감시가 놓치는 자리가 있다(네트워크 파일 시스템,
// 감시 개수 상한, 새 디렉터리에 곧바로 생기는 파일).

// watchJobName 은 감시를 붙이는 작업의 이름이자 신원이다.
const watchJobName = "파일 감시"

// watchQuiet 은 첫 종을 받고 답할 때까지 두는 짬이다.
//
// **이 한 줄이 debounce 다.** 포매터나 빌드가 한 번에 이벤트를 수십 개 내는데, 그때마다
// 서버에 알림을 보내고 검사를 시작하면 그 값이 그대로 든다. 짬을 두는 동안 온 것은
// pending 에 모여 한 번에 나온다. 예약 상태를 하나 더 두는 대신 Cmd goroutine 이 자는
// 것이라 편집기에 상태가 늘지 않는다.
//
// 250ms 는 lspTickCooldown 이 타이핑을 모으는 짬과 같다. gopls 는 알림을 받고 500ms 뒤에
// 반응하므로(ADR-0092 §0) 이 짬이 사람에게 보이지 않는다.
const watchQuiet = 250 * time.Millisecond

// watchMsg 는 밖에서 무언가 바뀌었다는 것이다.
//
// 무엇이 바뀌었는지는 싣지 않는다. 받은 자리에서 감시기가 모아 둔 것을 꺼낸다 —
// 진단이 오는 길과 같은 손이다(diagnostics.go).
type watchMsg struct{}

// watcher 는 뿌리 아래 디렉터리들을 감시한다. 편집기 수명 내내 하나다.
//
// fsnotify 에는 재귀 감시가 없어서 디렉터리마다 따로 등록해야 한다(README FAQ). 그리고
// **파일 하나를 감시하면 안 된다** — 편집기와 포매터는 임시 파일에 쓰고 rename 으로 덮는데,
// 그러면 감시하던 inode 가 사라져 그 뒤로 아무 이벤트도 오지 않는다. 그래서 디렉터리를 본다.
type watcher struct {
	fs   *fsnotify.Watcher
	root string
	ctx  context.Context

	// changed 는 종이다. cap 1 이고 넘치면 버린다 — 무엇이 바뀌었는지는 pending 이 들고
	// 있으므로 종은 「볼 것이 있다」만 말하면 된다(lsp 의 진단 종과 같다).
	changed chan struct{}

	// done 은 run 이 나가면서 닫는다. 기다리던 Cmd 가 그것을 보고 물러난다.
	//
	// **끝을 알리는 데 종을 닫지 않는 까닭이 있다.** 닫으면 종을 울리는 자리와 닫는 자리가
	// 겨루게 되고, 그 겨룸에서 지는 쪽은 닫힌 채널에 보내다가 죽는다. 지금은 울리는 자리가
	// run 하나뿐이라 실제로 겨루지 않지만, 그 안전이 「부르는 자리가 하나뿐이다」라는 논증에
	// 기대게 된다. 채널을 하나 더 두면 그 논증이 필요 없다.
	done chan struct{}

	// mu 는 감시 goroutine 과 Update 가 나눠 쓰는 두 자리를 지킨다. 그 둘 말고는
	// 이 struct 에 쓰는 자리가 없다.
	mu      sync.Mutex
	pending map[string]lsp.FileChangeKind
	errs    []error
}

// newWatcher 는 감시기를 만들고 이벤트를 읽는 goroutine 을 띄운다.
//
// 여기서는 아직 아무것도 감시하지 않는다. 붙이는 것은 값이 드는 일이라 작업으로 내린다
// (startWatchTree).
func newWatcher(ctx context.Context, root string) (*watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(err, "감시기를 만들지 못했습니다")
	}

	w := &watcher{
		fs:      fs,
		root:    root,
		ctx:     ctx,
		changed: make(chan struct{}, 1),
		done:    make(chan struct{}),
		pending: map[string]lsp.FileChangeKind{},
	}

	go w.run()

	return w, nil
}

// run 은 이벤트를 받아 모으고 종을 울린다. 감시기의 유일한 goroutine 이다.
//
// 두 채널 다 close 로 끝난다. 나가면서 done 을 닫으므로 기다리던 Cmd 가 nil 을 내고 고리가
// 끊긴다 — 죽은 상대의 종을 영영 기다리는 goroutine 을 남기지 않는다(ADR-0092 §5 가
// 감수한 자리를 여기서는 처음부터 피한다).
func (w *watcher) run() {
	defer close(w.done)

	for {
		select {
		case event, ok := <-w.fs.Events:
			if !ok {
				return
			}

			w.record(event)
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}

			w.mu.Lock()
			w.errs = append(w.errs, err)
			w.mu.Unlock()

			w.ring()
		}
	}
}

// record 는 이벤트 하나를 담고 종을 울린다.
func (w *watcher) record(event fsnotify.Event) {
	// 새 디렉터리가 생기면 그 아래도 감시에 올린다. 재귀 감시가 없어서 이것을 하지 않으면
	// 방금 만든 폴더 안이 통째로 보이지 않는다.
	//
	// **이 goroutine 이 직접 한다.** 갓 생긴 디렉터리는 거의 언제나 비어 있어서 값이 없다.
	// 붙이는 사이에 그 안에서 만들어진 파일은 놓칠 수 있고, 그 그물이 주기 검사다.
	if event.Has(fsnotify.Create) {
		if info, err := os.Lstat(event.Name); err == nil && info.IsDir() {
			if rel, err := filepath.Rel(w.root, event.Name); err == nil {
				_, _ = w.addTree(filepath.ToSlash(rel))
			}
		}
	}

	kind, ok := watchKind(event.Op)
	if !ok {
		return
	}

	// **지웠다고 온 것이 아직 있으면 덮어쓴 것이다.**
	//
	// 재서 알았다. 원자적 저장(임시 파일에 쓰고 rename 으로 덮기) 을 macOS 에서 하면 그
	// 파일이 `Remove` 로 온다 — 감시하던 inode 가 정말로 사라지기 때문이다. 이대로 넘기면
	// 언어 서버가 **살아 있는 파일을 지운 것으로 알고** 자기 view 에서 뺀다. 편집기와 포매터가
	// 이 방식으로 쓰므로 흔하게 밟는 자리다.
	//
	// 반대 방향(만들었다는데 없는 것) 은 보지 않는다. 그 자리를 보려면 이벤트마다 Lstat 을
	// 해야 하는데 여기는 쓰기마다 지나는 자리이고, 없는 파일을 알린 서버는 읽어 보고 없으면
	// 그대로 지운 것으로 끝낸다.
	if kind == lsp.FileDeleted && watchExists(event.Name) {
		kind = lsp.FileChanged
	}

	w.mu.Lock()
	w.pending[event.Name] = mergeWatchKind(w.pending[event.Name], kind)
	w.mu.Unlock()

	w.ring()
}

// watchExists 는 그 이름에 아직 무엇이 있는지다.
//
// Lstat 이라 심볼릭 링크는 링크 자체를 본다. 가리키는 곳이 없어져도 링크는 남아 있으므로
// 「이 이름이 사라졌나」를 묻는 이 자리에는 그쪽이 맞다.
func watchExists(path string) bool {
	_, err := os.Lstat(path)

	return err == nil
}

// ring 은 볼 것이 생겼다고 종을 울린다. 이미 울려 있으면 그냥 지나간다.
func (w *watcher) ring() {
	select {
	case w.changed <- struct{}{}:
	default:
	}
}

// take 는 모아 둔 것을 꺼내고 자리를 비운다. `Update` 안에서만 부른다.
func (w *watcher) take() ([]lsp.FileChange, []error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	changes := make([]lsp.FileChange, 0, len(w.pending))
	for path, kind := range w.pending {
		changes = append(changes, lsp.FileChange{Path: path, Kind: kind})
	}

	errs := w.errs

	clear(w.pending)
	w.errs = nil

	return changes, errs
}

// addTree 는 rel 아래의 디렉터리를 전부 감시에 올린다. rel 이 빈 문자열이면 뿌리 전체다.
//
// 붙인 개수와, 멈추게 만든 오류를 준다. 상한(`EMFILE`·inotify 의 `ENOSPC`) 에 걸리면 더
// 붙여도 같은 오류라 그 자리에서 멈춘다.
//
// 읽지 못하거나 사라진 디렉터리 하나는 건너뛰고 이어간다. 훑는 사이에 지워지는 일이 흔하고,
// 그 하나 때문에 나머지를 다 잃을 이유가 없다.
func (w *watcher) addTree(rel string) (int, error) {
	ignore := gitIgnoreAt(w.root, rel)
	if rel != "" && rel != "." && ignore.match(rel, true) {
		return 0, nil
	}

	added := 0

	var failure error

	add := func(dir string) bool {
		if err := w.fs.Add(filepath.Join(w.root, dir)); err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
				return w.ctx.Err() == nil
			}

			failure = errors.Wrapf(err, "%s 를 감시하지 못했습니다", dir)

			return false
		}

		added++

		return w.ctx.Err() == nil
	}

	if !add(rel) {
		return added, failure
	}

	walkGitDirTree(w.ctx, w.root, rel, ignore, add)

	return added, failure
}

// close 는 감시를 끝낸다. run 이 빠져나오며 done 을 닫는다.
func (w *watcher) close() {
	_ = w.fs.Close()
}

// watchKind 는 이벤트 하나를 규격의 갈래로 바꾼다. 볼 것이 없으면 false 다.
//
// **Chmod 는 버린다.** macOS 에서 Spotlight·백업·백신이 이것을 많이 내고(fsnotify README 가
// 그렇게 적어 두었다) 받으면 아무도 손대지 않은 저장소에서 편집기가 계속 깨어난다.
//
// 갈래를 bit 로 견주는 것은 한 이벤트가 여럿을 겹쳐 들고 오기 때문이다(`Write|Chmod`).
func watchKind(op fsnotify.Op) (lsp.FileChangeKind, bool) {
	switch {
	case op.Has(fsnotify.Remove), op.Has(fsnotify.Rename):
		// rename 은 「이 이름에서 사라졌다」다. 옮겨간 자리는 그쪽 디렉터리가 Create 로 낸다.
		return lsp.FileDeleted, true
	case op.Has(fsnotify.Create):
		return lsp.FileCreated, true
	case op.Has(fsnotify.Write):
		return lsp.FileChanged, true
	}

	return 0, false
}

// mergeWatchKind 는 같은 파일에 이벤트가 여럿 올 때 무엇으로 남길지 정한다.
//
// **나중에 온 것이 이긴다.** 만들었다 지운 것은 지운 것이고, 지웠다 다시 만든 것은 만든
// 것이다 — 뒤쪽이 곧 원자적 저장(rename) 의 모양이라 이 규칙이 그것을 바르게 읽는다.
//
// 예외가 하나다. 만든 뒤 곧바로 쓰는 흔한 차례에서 「만들어졌다」를 잃지 않는다.
func mergeWatchKind(was, now lsp.FileChangeKind) lsp.FileChangeKind {
	if was == lsp.FileCreated && now == lsp.FileChanged {
		return was
	}

	return now
}

// startWatch 는 뿌리 아래를 감시하기 시작한다. Init 이 한 번 지나는 자리다(startInitialJobs).
//
// **뿌리는 cwd 다.** 트리·팔레트 뿌리와 같고 서버에게 준 뿌리와도 같다. 저장소 뿌리를
// 쓰지 않는 것은 하위 디렉터리에서 zn 을 열었을 때 위쪽 전체를 감시하게 되기 때문이다.
// 그 대신 cwd 위의 변경은 감시가 못 잡는다(ADR-0093).
func (e *editor) startWatch() tea.Cmd {
	if e.watch != nil {
		return nil
	}

	root, err := os.Getwd()
	if err != nil {
		e.notifyError(errors.Wrap(err, "감시할 뿌리를 알 수 없습니다"))

		return nil
	}

	w, err := newWatcher(e.rootContext(), root)
	if err != nil {
		// **조용히 실패하지 않는다.** vim + coc 가 watchman 이 없을 때 아무 말 없이 감시를
		// 잃는 자리가 정확히 이것이다(ADR-0092 §6). 감시가 없어도 주기 검사는 그대로 도므로
		// 정확성은 남고 반응만 느려진다.
		e.notifyError(err)

		return nil
	}

	e.watch = w

	return tea.Batch(e.startWatchTree(), waitWatch(w))
}

// startWatchTree 는 감시할 디렉터리를 훑어 붙이는 작업을 시작한다.
//
// 작업으로 내리는 까닭은 값이 들기 때문이다. 큰 저장소에서 훑기와 등록이 오래 걸리고,
// 5 초마다 도는 것을 감추지 않는 것과 같은 태도로 `:jobs` 에 보인다(ADR-0030).
func (e *editor) startWatchTree() tea.Cmd {
	w := e.watch

	return e.startJob(watchJobName, nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress, 1)

		go func() {
			defer close(ch)

			added, err := w.addTree("")

			ch <- jobProgress{
				done:    added,
				total:   added,
				summary: fmt.Sprintf("폴더 %d 개", added),
				err:     err,
			}
		}()

		return ch
	})
}

// stopWatch 는 감시를 끝낸다. 편집기가 내려가는 자리에서 부른다(core.Run).
func (e *editor) stopWatch() {
	if e.watch == nil {
		return
	}

	e.watch.close()
	e.watch = nil
}

// waitWatch 는 종이 울릴 때까지 기다려 msg 로 바꾸는 Cmd 다.
//
// 받을 때마다 다시 발행해야 다음 것이 온다. 진단을 받는 것과 같은 고리다(waitDiagnostics).
//
// 감시기가 내려갔으면 nil 을 준다. 기다릴 상대가 없고, 여기서 nil 을 주지 않으면 다시
// 울리지 않는 종을 안고 goroutine 하나가 그대로 남는다.
func waitWatch(w *watcher) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-w.changed:
			time.Sleep(watchQuiet)

			return watchMsg{}
		case <-w.done:
			return nil
		}
	}
}

// applyWatch 는 감시기가 모아 둔 것을 꺼내 갈 곳으로 보낸다. `Update` 안에서 불린다(job.go).
//
// 갈 곳이 둘이고 쓰는 법이 다르다. 언어 서버는 **이벤트를 그대로** 받아 자기가 디스크를 다시
// 읽고(ADR-0092), buffer 는 **앞잡이로만** 받아 검사를 앞당길 뿐 판정은 해시가 한다(ADR-0015).
func (e *editor) applyWatch() tea.Cmd {
	if e.watch == nil {
		return nil
	}

	changes, errs := e.watch.take()

	for _, err := range errs {
		e.notifyError(errors.Wrap(err, "파일 감시"))
	}

	e.serverWatchFiles(changes)

	cmds := []tea.Cmd{waitWatch(e.watch)}

	for _, change := range changes {
		// 감시기는 절대 경로를 주는데 CLI 로 연 buffer 는 적힌 그대로(상대 경로) 를 든다.
		// tabOf 가 samePath 로 견주므로 그것으로 찾고, 검사에는 **buffer 가 든 경로**를
		// 넘긴다 — 결과를 돌려 넣는 자리가 그 경로로 buffer 를 되찾는다(applyOutsideResult).
		index, ok := e.tabOf(change.Path)
		if !ok {
			continue
		}

		cmds = append(cmds, e.startOutsideCheckFor(e.buffers[index].Path))
	}

	return tea.Batch(cmds...)
}

// serverWatchFiles 는 밖에서 바뀐 파일을 각자의 서버에 알린다.
//
// **서버가 볼 것만 보낸다.** 그 언어의 파일이 아니면 서버가 읽을 이유가 없고, 남의 언어
// 파일을 알리면 그 서버가 그것을 자기 언어로 읽으려 든다 — `go.mod`·`go.sum`·`go.work` 를
// 같이 보낼지는 아직 정하지 않았다(docs/tasks.md).
//
// **열어 둔 파일도 걸러내지 않는다.** overlay 가 디스크를 이기므로 그 파일에 대한 알림은
// 서버가 알아서 무시한다(ADR-0092 §3).
//
// 실패는 삼킨다. 있으면 정확해지는 알림이고, 없으면 예전처럼 낡은 채로 도는 것이라
// 알릴 실패가 아니다.
func (e *editor) serverWatchFiles(changes []lsp.FileChange) {
	byServer := map[string][]lsp.FileChange{}

	for _, change := range changes {
		server := lsp.ServerFor(change.Path)
		if server == nil {
			continue
		}

		byServer[server.Name] = append(byServer[server.Name], change)
	}

	for name, files := range byServer {
		client := e.clientNamed(name)
		if client == nil {
			continue
		}

		_ = client.FilesChanged(files)
	}
}
