package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/lsp"
)

// watchDeadline 은 감시 시험이 이벤트를 기다리는 상한이다.
//
// 넉넉하게 둔다. 재려는 것은 「오는가」이지 「얼마나 빨리 오는가」가 아니고, 붐비는 CI 에서
// 커널이 알려주는 데 걸리는 시간을 우리가 정하지 못한다.
const watchDeadline = 3 * time.Second

// newTestWatcher 는 그 디렉터리를 감시하는 감시기를 만든다.
func newTestWatcher(t *testing.T, root string) *watcher {
	t.Helper()

	w, err := newWatcher(t.Context(), root)
	require.NoError(t, err)

	t.Cleanup(w.close)

	added, err := w.addTree("")
	require.NoError(t, err)
	require.Positive(t, added, "뿌리조차 붙이지 못했다")

	return w
}

// waitWatched 는 그 경로가 잡힐 때까지 기다려 갈래를 준다.
//
// 모아 가며 기다린다. 종이 여러 번 울릴 수 있고 한 번의 take 가 찾는 것을 담고 있지 않을
// 수 있어서, 꺼낸 것을 쌓아 두고 다시 기다린다.
func waitWatched(t *testing.T, w *watcher, path string) lsp.FileChangeKind {
	t.Helper()

	seen := gatherWatched(t, w, path)

	kind, ok := seen[path]
	require.True(t, ok, "%s 를 잡지 못했다. 잡은 것: %v", path, seen)

	return kind
}

// gatherWatched 는 until 이 잡힐 때까지 모은 것을 준다. until 이 안 오면 시험을 세운다.
func gatherWatched(t *testing.T, w *watcher, until string) map[string]lsp.FileChangeKind {
	t.Helper()

	seen := map[string]lsp.FileChangeKind{}
	deadline := time.After(watchDeadline)

	for {
		select {
		case <-w.changed:
			changes, errs := w.take()
			require.Empty(t, errs)

			for _, change := range changes {
				seen[change.Path] = change.Kind
			}

			if _, ok := seen[until]; ok {
				return seen
			}
		case <-deadline:
			t.Fatalf("%s 를 기다리다 시간이 다 됐다. 잡은 것: %v", until, seen)
		}
	}
}

// 이벤트 갈래를 규격의 번호로 옮기는 규칙이다.
//
// 한 이벤트가 여럿을 겹쳐 들고 오므로(`Write|Chmod`) bit 로 견준다.
func TestWatchKind(t *testing.T) {
	kind, ok := watchKind(fsnotify.Create)
	assert.True(t, ok)
	assert.Equal(t, lsp.FileCreated, kind)

	kind, ok = watchKind(fsnotify.Write)
	assert.True(t, ok)
	assert.Equal(t, lsp.FileChanged, kind)

	kind, ok = watchKind(fsnotify.Remove)
	assert.True(t, ok)
	assert.Equal(t, lsp.FileDeleted, kind)

	// rename 은 「이 이름에서 사라졌다」다. 옮겨간 자리는 그쪽 디렉터리가 Create 로 낸다.
	kind, ok = watchKind(fsnotify.Rename)
	assert.True(t, ok)
	assert.Equal(t, lsp.FileDeleted, kind)

	// 지운 것이 겹쳐 오면 지운 것으로 읽는다.
	kind, ok = watchKind(fsnotify.Create | fsnotify.Remove)
	assert.True(t, ok)
	assert.Equal(t, lsp.FileDeleted, kind)

	// 겹쳐 온 Chmod 는 옆에 있는 갈래를 가리지 않는다.
	kind, ok = watchKind(fsnotify.Write | fsnotify.Chmod)
	assert.True(t, ok)
	assert.Equal(t, lsp.FileChanged, kind)
}

// Chmod 만 온 것은 버린다.
//
// macOS 의 Spotlight·백업·백신이 이것을 많이 낸다(fsnotify README). 받으면 아무도 손대지
// 않은 저장소에서 편집기가 5 초 tick 과 무관하게 계속 깨어난다.
func TestWatchKindDropsChmod(t *testing.T) {
	_, ok := watchKind(fsnotify.Chmod)

	assert.False(t, ok, "Chmod 만 온 것을 잡으면 유휴 상태가 조용하지 않다")
}

// 같은 파일에 이벤트가 여럿 오면 나중 것이 이긴다. 예외는 하나뿐이다.
func TestMergeWatchKind(t *testing.T) {
	// 만든 뒤 곧바로 쓰는 흔한 차례다. 「만들어졌다」를 잃지 않는다.
	assert.Equal(t, lsp.FileCreated, mergeWatchKind(lsp.FileCreated, lsp.FileChanged))

	// 만들었다 지운 것은 지운 것이다.
	assert.Equal(t, lsp.FileDeleted, mergeWatchKind(lsp.FileCreated, lsp.FileDeleted))

	// **원자적 저장의 모양이다.** rename 이 지운 것으로 오고 그 뒤에 만든 것이 온다.
	assert.Equal(t, lsp.FileCreated, mergeWatchKind(lsp.FileDeleted, lsp.FileCreated))

	assert.Equal(t, lsp.FileDeleted, mergeWatchKind(lsp.FileChanged, lsp.FileDeleted))
}

// 밖에서 쓴 것을 잡는다. 감시의 가장 기본이다.
func TestWatcherCatchesWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	w := newTestWatcher(t, root)

	require.NoError(t, os.WriteFile(path, []byte("package a // 남이 쓴 것\n"), 0644))

	assert.Equal(t, lsp.FileChanged, waitWatched(t, w, path))
}

// 밖에서 만든 파일을 잡는다. gopls 가 열지 않은 파일을 끝까지 모르던 자리다(ADR-0092 §0).
func TestWatcherCatchesCreate(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)

	path := filepath.Join(root, "새 파일.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	assert.Equal(t, lsp.FileCreated, waitWatched(t, w, path))
}

// 원자적 저장(임시 파일에 쓰고 rename 으로 덮기) 도 잡는다.
//
// **이것이 파일이 아니라 디렉터리를 감시하는 근거다.** 편집기와 포매터가 이렇게 쓰는데,
// 파일 하나를 감시하면 rename 으로 inode 가 바뀌는 순간 그 뒤로 아무 이벤트도 오지 않는다
// (fsnotify README 가 그렇게 적어 두었다).
func TestWatcherCatchesAtomicSave(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	w := newTestWatcher(t, root)

	temp := filepath.Join(root, ".a.go.tmp")
	require.NoError(t, os.WriteFile(temp, []byte("package a // 남이 쓴 것\n"), 0644))
	require.NoError(t, os.Rename(temp, path))

	// **갈래는 플랫폼마다 갈린다.** 재보니 macOS 에서는 이 파일이 `Remove` 로 오고
	// (감시하던 inode 가 정말 사라진다) Linux 에서는 `Create` 로 온다. 지운 것으로 오는
	// 쪽을 record 가 디스크를 보고 되돌린다 — 그러지 않으면 gopls 가 살아 있는 파일을
	// 지운 것으로 알고 자기 view 에서 뺀다.
	kind := waitWatched(t, w, path)
	assert.NotEqual(t, lsp.FileDeleted, kind, "덮어쓴 파일이 사라진 것으로 읽혔다")
}

// 지운 것을 잡는다.
func TestWatcherCatchesRemove(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	w := newTestWatcher(t, root)

	require.NoError(t, os.Remove(path))

	assert.Equal(t, lsp.FileDeleted, waitWatched(t, w, path))
}

// 새로 생긴 디렉터리도 감시에 올린다.
//
// fsnotify 에는 재귀 감시가 없어서 이것을 하지 않으면 방금 만든 폴더 안이 통째로 보이지
// 않는다. `mkdir pkg && vim pkg/a.go` 가 그 자리다.
func TestWatcherAddsNewDirectory(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)

	dir := filepath.Join(root, "pkg")
	require.NoError(t, os.Mkdir(dir, 0755))

	// 디렉터리가 붙기를 기다린다. 붙이는 것은 감시 goroutine 이 하므로 그것이 이 이벤트를
	// 다루고 난 뒤라야 안쪽 파일이 보인다.
	require.Equal(t, lsp.FileCreated, waitWatched(t, w, dir))

	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	assert.Equal(t, lsp.FileCreated, waitWatched(t, w, path))
}

// Chmod 는 잡지 않는다. 뒤따라 쓴 파일이 잡힐 때까지 기다려도 그 자리는 비어 있다.
//
// 「일어나지 않는 것」을 재는 것이라 뒤에 확실히 일어날 일을 하나 붙여서 시각을 정한다.
// 이벤트는 차례대로 오므로, 뒤엣것이 왔으면 앞엣것도 이미 지났다.
func TestWatcherDropsChmod(t *testing.T) {
	root := t.TempDir()
	quiet := filepath.Join(root, "quiet.go")
	require.NoError(t, os.WriteFile(quiet, []byte("package a\n"), 0644))

	w := newTestWatcher(t, root)

	require.NoError(t, os.Chmod(quiet, 0600))

	noisy := filepath.Join(root, "noisy.go")
	require.NoError(t, os.WriteFile(noisy, []byte("package a\n"), 0644))

	seen := gatherWatched(t, w, noisy)

	assert.NotContains(t, seen, quiet, "권한만 바뀐 파일이 잡혔다")
}

// 무시되는 디렉터리는 감시하지 않는다.
//
// **이것이 값의 전부다.** `.git` 하나만 해도 이 저장소에서 감시할 디렉터리가 39 개에서
// 313 개가 되고, kqueue(macOS) 는 디렉터리 안의 항목마다 fd 를 열어서 그 차이가 곧 fd 수다.
func TestWatcherSkipsIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("zn-hidden/\n"), 0644))

	for _, dir := range []string{".git", "zn-hidden", "zn-kept"} {
		require.NoError(t, os.Mkdir(filepath.Join(root, dir), 0755))
	}

	w := newTestWatcher(t, root)

	watched := map[string]bool{}
	for _, dir := range w.fs.WatchList() {
		watched[filepath.Base(dir)] = true
	}

	assert.True(t, watched["zn-kept"], "무시되지 않은 디렉터리를 감시하지 않는다")
	assert.False(t, watched[".git"], ".git 을 감시한다")
	assert.False(t, watched["zn-hidden"], "`.gitignore` 에 적힌 디렉터리를 감시한다")
}

// 무시되는 디렉터리가 새로 생겨도 붙지 않는다. `npm install` 이 그 자리다.
func TestWatcherSkipsNewIgnoredDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("zn-hidden/\n"), 0644))

	w := newTestWatcher(t, root)

	require.NoError(t, os.Mkdir(filepath.Join(root, "zn-hidden"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(root, "zn-kept"), 0755))

	// 뒤엣것이 붙을 때까지 기다린다. 그 시점이면 앞엣것도 이미 지났다.
	require.Equal(t, lsp.FileCreated, waitWatched(t, w, filepath.Join(root, "zn-kept")))

	watched := map[string]bool{}
	for _, dir := range w.fs.WatchList() {
		watched[filepath.Base(dir)] = true
	}

	assert.True(t, watched["zn-kept"])
	assert.False(t, watched["zn-hidden"], "무시되는 디렉터리가 새로 생기면 감시에 붙는다")
}

// 감시기를 닫으면 기다리던 Cmd 가 nil 을 준다.
//
// **여기서 nil 을 주지 않으면 두 가지 중 하나가 난다.** 닫힌 채널을 끝없이 읽어 msg 를
// 쏟거나, 다시 울리지 않는 종을 영영 기다리는 goroutine 이 남는다. 뒤쪽은 ADR-0092 §5 가
// 진단 쪽에서 감수한 자리이고 여기서는 처음부터 피한다.
func TestWaitWatchStopsWhenClosed(t *testing.T) {
	w, err := newWatcher(t.Context(), t.TempDir())
	require.NoError(t, err)

	w.close()

	done := make(chan tea.Msg, 1)
	go func() { done <- waitWatch(w)() }()

	select {
	case msg := <-done:
		assert.Nil(t, msg, "감시기가 닫혔는데 msg 를 냈다")
	case <-time.After(watchDeadline):
		t.Fatal("감시기를 닫았는데 기다리던 Cmd 가 돌아오지 않는다")
	}
}

// take 는 꺼내고 자리를 비운다. 같은 것을 두 번 보내면 gopls 가 같은 파일을 또 읽는다.
func TestWatcherTakeEmpties(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)

	path := filepath.Join(root, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	require.Equal(t, lsp.FileCreated, waitWatched(t, w, path))

	changes, errs := w.take()

	assert.Empty(t, changes, "꺼낸 뒤에도 남아 있다")
	assert.Empty(t, errs)
}

// withWatcher 는 그 편집기에 감시기를 달아 준다.
//
// 시험 편집기는 감시기 없이 뜬다(그것이 맞다 — 감시를 붙이는 것은 Init 이 하는 일이다).
// watchMsg 를 다루는 자리를 재려면 달려 있어야 한다.
func withWatcher(t *testing.T, m viewEditorNormal) viewEditorNormal {
	t.Helper()

	w, err := newWatcher(t.Context(), t.TempDir())
	require.NoError(t, err)

	t.Cleanup(w.close)

	m.watch = w

	return m
}

// mode 를 오가는 동안에도 감시 고리가 끊기지 않는다.
//
// **한 mode 라도 흘려보내면 그 mode 에 머무는 동안 감시가 조용히 멈춘다.** 고리를 잇는 것이
// msg 를 받는 자리이기 때문이다 — 받은 곳에서 waitWatch 를 다시 내지 않으면 다음 종을
// 기다리는 자가 없어진다. tick 과 달리 스스로 다시 오지 않아서 되살아나지도 않는다.
//
// mode 마다 편집기를 새로 만드는 것은 git·파일 tick 시험과 같은 까닭이다.
func TestEveryModeKeepsWatching(t *testing.T) {
	modes := []struct {
		name  string
		enter func(t *testing.T) tea.Model
		want  tea.Model
	}{
		{"normal", func(t *testing.T) tea.Model { return withWatcher(t, newTestEditor("abc\n", 80, 10)) }, viewEditorNormal{}},
		{"insert", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTestEditor("abc\n", 80, 10)), "i")
		}, viewEditorInsert{}},
		{"visual", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTestEditor("abc\n", 80, 10)), "v")
		}, viewEditorVisual{}},
		{"command", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTestEditor("abc\n", 80, 10)), ":")
		}, viewEditorCommand{}},
		{"search", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTestEditor("abc\n", 80, 10)), "/")
		}, viewEditorSearch{}},
		{"palette", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTestEditor("abc\n", 80, 10)), "ctrl+p")
		}, viewPalette{}},
		{"jobs", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTestEditor("abc\n", 80, 10)), ":", "j", "o", "b", "s", "enter")
		}, viewJobs{}},
		{"tree", func(t *testing.T) tea.Model {
			return send(withWatcher(t, newTreeEditor(t, 80, 10)), "ctrl+w", "ctrl+w")
		}, viewSidebar{}},
		{"confirm", func(t *testing.T) tea.Model {
			return send(send(withWatcher(t, newTestEditor("abc\n", 80, 10)), "x"), ":", "q", "enter")
		}, viewConfirmDiscard{}},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			model := mode.enter(t)
			require.IsType(t, mode.want, model, "이 mode 로 들어가지 못했다")

			_, cmd := model.Update(watchMsg{})

			assert.NotNil(t, cmd, "이 mode 에서 감시 고리가 끊긴다")
		})
	}
}

// 감시기가 없어도 watchMsg 를 받아 넘어간다.
//
// 감시를 붙이지 못한 손에서도 편집기는 그대로 돈다. 그 자리를 시험이 지킨다.
func TestWatchMsgWithoutWatcher(t *testing.T) {
	m := newTestEditor("abc\n", 80, 10)

	_, cmd := m.Update(watchMsg{})

	assert.Nil(t, cmd, "감시기가 없는데 기다리는 자를 냈다")
}

// 밖에서 바뀐 것이 열린 파일이면 cooldown 을 기다리지 않고 그 파일의 검사를 시작한다.
//
// **이것이 「앞잡이로만 쓴다」의 전부다.** 이벤트를 판정으로 쓰지 않고 검사를 앞당길 뿐이다
// (ADR-0015, ADR-0093).
func TestWatchStartsCheckForOpenFile(t *testing.T) {
	m, path := newWideFileEditor(t, "abc\n")
	m = withWatcher(t, m)

	require.NoError(t, os.WriteFile(path, []byte("남이 쓴 것\n"), 0644))

	// 감시기가 잡은 것처럼 담아 둔다. 커널이 알려주기를 기다리지 않는다 — 여기서 재는 것은
	// 잡은 뒤에 무엇을 하는가이고, 잡는 것은 위의 시험들이 잰다.
	m.watch.pending[path] = lsp.FileChanged

	next, cmd := m.Update(watchMsg{})
	require.NotNil(t, cmd)

	assert.True(t, next.(viewEditorNormal).jobRunning(fileJobName, []string{path}), "그 파일의 검사가 시작되지 않았다")

	// 몰기 전에 감시기를 닫는다. 고리를 잇는 Cmd 가 다음 종을 기다려 막혀 있고, 이 시험에서
	// 그 종을 울릴 자가 없다. 닫으면 그 Cmd 가 nil 을 내고 물러난다.
	m.watch.close()

	settled := settle(t, next, cmd)

	assert.Equal(t, "남이 쓴 것", string(bufferOf(t, settled).Line(0)))
}

// 열려 있지 않은 파일이 바뀐 것은 gopls 만 듣는다. 읽을 buffer 가 없다.
func TestWatchIgnoresUnopenedFileForBuffers(t *testing.T) {
	m := withWatcher(t, newTestEditor("abc\n", 80, 10))

	m.watch.pending["/전혀/열지/않은.go"] = lsp.FileChanged

	next, cmd := m.Update(watchMsg{})

	require.NotNil(t, cmd, "고리는 이어져야 한다")
	assert.Empty(t, next.(viewEditorNormal).jobs, "열지도 않은 파일의 검사를 시작했다")
}

// 지웠다고 온 것이 아직 있으면 덮어쓴 것으로 되돌린다.
//
// 이벤트 갈래를 그대로 믿지 않는 유일한 자리다. 원자적 저장이 macOS 에서 `Remove` 로 오는데,
// 그것을 그대로 gopls 에 넘기면 서버가 살아 있는 파일을 자기 view 에서 뺀다.
func TestWatcherKeepsReplacedFileAlive(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("package a\n"), 0644))

	w := newTestWatcher(t, root)

	// 감시기가 이벤트를 받는 자리를 곧바로 부른다. 커널이 어느 갈래로 알려주는지는
	// 플랫폼이 정하므로, 재려는 「지웠다고 왔을 때 어떻게 하는가」를 여기서 직접 준다.
	w.record(fsnotify.Event{Name: path, Op: fsnotify.Remove})

	changes, _ := w.take()
	require.Len(t, changes, 1)

	assert.Equal(t, lsp.FileChanged, changes[0].Kind, "아직 있는 파일이 사라진 것으로 남았다")
}

// 정말로 사라진 것은 사라진 것으로 남는다.
func TestWatcherKeepsRemovedFileDeleted(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root)

	w.record(fsnotify.Event{Name: filepath.Join(root, "없는.go"), Op: fsnotify.Remove})

	changes, _ := w.take()
	require.Len(t, changes, 1)

	assert.Equal(t, lsp.FileDeleted, changes[0].Kind)
}

// 붙이다 막히면 개수와 오류를 같이 준다.
//
// **조용히 실패하지 않는 자리다.** 이 오류가 작업의 err 로 올라가 알림이 된다 — vim + coc 가
// watchman 이 없을 때 아무 말 없이 감시를 잃는 것이 정확히 여기다(ADR-0092 §6, ADR-0093).
//
// 실제로 막히는 까닭은 상한(`EMFILE`·inotify 의 `ENOSPC`) 인데 그것을 시험에서 만들 수
// 없어서, 같은 길로 오는 「닫힌 감시기」로 잰다.
func TestWatcherAddTreeReportsFailure(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "zn-kept"), 0755))

	w, err := newWatcher(t.Context(), root)
	require.NoError(t, err)

	w.close()

	added, err := w.addTree("")

	assert.Zero(t, added, "붙지도 않았는데 붙었다고 센다")
	require.Error(t, err, "붙이지 못한 것을 삼켰다")
	assert.Contains(t, err.Error(), "감시하지 못했습니다")
}

// 감시를 붙이는 작업이 알아낸 것을 `:jobs` 에 남긴다.
func TestStartWatchTreeReportsCount(t *testing.T) {
	m := withWatcher(t, newTestEditor("abc\n", 80, 10))

	cmd := m.startWatchTree()
	require.NotNil(t, cmd)

	settled := settle(t, tea.Model(m), cmd)

	editor := settled.(viewEditorNormal).editor
	require.NotEmpty(t, editor.finished, "끝난 작업 목록에 남지 않았다")

	assert.Equal(t, watchJobName, editor.finished[0].name)
	assert.Contains(t, editor.finished[0].summary, "폴더")
}

// 밖에서 파일이 생기면 그 디렉터리가 저절로 다시 읽힌다. 키를 누르지 않는다(ADR-0134).
func TestWatchReloadsTreeOnCreate(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	require.NoError(t, os.WriteFile(filepath.Join(root, "새.go"), []byte("x\n"), 0644))

	settle(t, m, m.reloadWatchedDirs([]lsp.FileChange{
		{Path: filepath.Join(root, "새.go"), Kind: lsp.FileCreated},
	}))

	assert.Contains(t, names(m.sidebar.rows()), "1:새.go")
}

// 사라진 것도 저절로 없어진다.
func TestWatchReloadsTreeOnDelete(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	require.NoError(t, os.Remove(filepath.Join(root, "README.md")))

	settle(t, m, m.reloadWatchedDirs([]lsp.FileChange{
		{Path: filepath.Join(root, "README.md"), Kind: lsp.FileDeleted},
	}))

	assert.NotContains(t, names(m.sidebar.rows()), "1:README.md")
}

// **내용만 바뀐 것은 버린다.** 저장은 목록을 바꾸지 않으므로 다시 읽을 것이 없다.
// 타이핑하며 저장하는 동안 트리가 계속 디스크를 읽게 되는 자리다.
func TestWatchIgnoresWriteForTree(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	assert.Nil(t, m.reloadWatchedDirs([]lsp.FileChange{
		{Path: filepath.Join(root, "README.md"), Kind: lsp.FileChanged},
	}), "저장만으로는 다시 읽지 않는다")
}

// 트리에 없는 자리는 지나간다. `.git` 안과 아직 펼치지 않은 층 아래가 그렇다.
func TestWatchSkipsDirsOutsideTree(t *testing.T) {
	m := newTreeEditor(t, 80, 10)
	root := m.sidebar.root

	assert.Nil(t, m.reloadWatchedDirs([]lsp.FileChange{
		{Path: filepath.Join(root, ".git", "index"), Kind: lsp.FileCreated},
		{Path: filepath.Join(root, "docs", "새.md"), Kind: lsp.FileCreated},
	}), "`.git` 안도 접힌 docs 안도 다시 읽을 자리가 아니다")
}
