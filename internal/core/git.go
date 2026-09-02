package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// gitStatus 는 statusBar 오른쪽에 찍히는 저장소 상태다.
//
// 저장소가 아니거나 git 이 없으면 전부 빈 값이고 아무것도 그리지 않는다.
// sidebar 의 gitignore 표시와 같은 태도다 — 표시가 없을 뿐 틀리지는 않는다.
type gitStatus struct {
	branch string // detached HEAD 면 빈 문자열이다
	commit string // 짧은 해시. 이것이 비면 저장소가 아니거나 commit 이 하나도 없는 것이다
	dirty  bool

	// head 는 자르지 않은 HEAD 해시다. 화면에 찍지 않고 **HEAD 가 움직였는지**를 보는 데만
	// 쓴다 — 짧은 해시는 저장소 크기에 따라 길이가 달라져서(gitShortHashLen) 견주는 자를
	// 그것으로 삼을 수 없다(ADR-0092).
	head string
}

// gitMinShortHashLen 은 짧은 해시가 짧아지지 않는 자리 수다. git 의 기본값과 같다.
const gitMinShortHashLen = 7

// gitShortHashLen 은 팩에 든 객체가 count 개일 때 짧은 해시의 자리 수다.
//
// **git 의 셈법을 그대로 쓴다.** 객체 수의 최상위 비트 자리를 반으로 접고 하나 더한 것이고,
// 일곱 자리보다 짧아지지 않는다.
//
// git 2.52 로 재서 맞췄다. 팩 객체 16,383 개까지 일곱 자리이고 16,384(2^14) 개에서 여덟
// 자리로 넘어간다. 그 자리가 정확히 이 식의 경계다(ADR-0042).
//
// **겹치는지는 확인하지 않는다.** git 도 이 길이를 먼저 정하고 나서 겹치면 늘리는데, 그러려면
// 객체를 훑어야 한다. tick 마다 도는 자리라(readGitStatus) 낼 수 없는 값이다. 고치려던 것은
// 「큰 저장소에서 `git log` 와 달라 보인다」였고 그것은 이 식이 답한다.
func gitShortHashLen(count uint64) int {
	// 최상위 비트가 몇 번째 자리인가다. 0 개면 자리가 없어서 아래 최소값이 답한다.
	msb := bits.Len64(count)
	if msb > 0 {
		msb--
	}

	return max(msb/2+1, gitMinShortHashLen)
}

// gitPackedObjectCount 는 팩에 든 객체 수다. 세지 못하면 0 이고 그때는 일곱 자리가 된다.
//
// **느슨한 객체는 세지 않는다.** git 도 세지 않는다 — 재보니 느슨한 객체 17,002 개에 팩이
// 비어 있으면 일곱 자리다. 이 수를 git 이 `approximate_object_count` 라고 부르는 까닭이다.
func gitPackedObjectCount(root string) uint64 {
	dir, err := gitDir(root)
	if err != nil {
		return 0
	}

	names, err := filepath.Glob(filepath.Join(dir, "objects", "pack", "*.idx"))
	if err != nil {
		return 0
	}

	total := uint64(0)
	for _, name := range names {
		total += idxObjectCount(name)
	}

	return total
}

// gitDir 은 저장소의 `.git` 자리다.
//
// 대개는 뿌리 아래의 디렉터리인데, worktree 와 submodule 에서는 **파일**이고 그 안의
// `gitdir: <경로>` 한 줄이 진짜 자리를 가리킨다.
func gitDir(root string) (string, error) {
	path := filepath.Join(root, ".git")

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return path, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	pointed, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return "", errors.Errorf("gitdir 을 찾을 수 없습니다: %s", path)
	}

	pointed = strings.TrimSpace(pointed)
	if filepath.IsAbs(pointed) {
		return pointed, nil
	}

	// 적힌 경로는 이 파일이 놓인 자리 기준이다.
	return filepath.Join(root, pointed), nil
}

// idxObjectCount 는 팩 색인 한 장에 든 객체 수다.
//
// **부채꼴 표의 마지막 칸이 그 수다.** 표는 첫 byte 값마다 「그 값까지의 누적 개수」를 담으므로
// 255 번째 칸이 곧 전체다. git 의 `open_pack_index` 가 `num_objects` 를 얻는 자리와 같다.
//
// **색인을 통째로 읽지 않는 것이 요점이다.** go-git 의 idxfile 디코더는 이름·오프셋·CRC 를
// 전부 메모리에 올려서 객체 백만 개면 수십 MB 다. 여기서 읽는 것은 1,032 byte 다.
//
// v2 는 앞에 여덟 byte 의 머리(`\377tOc` 와 판 번호)가 붙고, v1 은 표부터 시작한다.
// 판을 가르는 것은 그 머리가 있는지뿐이다.
func idxObjectCount(path string) uint64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	head := make([]byte, 8)
	if _, err := io.ReadFull(file, head); err != nil {
		return 0
	}

	fanout := make([]byte, 1024)

	if bytes.Equal(head[:4], []byte{0xff, 't', 'O', 'c'}) {
		if _, err := io.ReadFull(file, fanout); err != nil {
			return 0
		}
	} else {
		// v1 은 머리가 없다. 방금 읽은 여덟 byte 가 이미 표의 앞머리다.
		copy(fanout, head)

		if _, err := io.ReadFull(file, fanout[8:]); err != nil {
			return 0
		}
	}

	return uint64(binary.BigEndian.Uint32(fanout[1020:]))
}

// readGitStatus 는 cwd 저장소의 상태를 읽는다.
//
// 활성 파일이 아니라 cwd 를 본다. sidebar 트리의 뿌리와 같은 기준이라 tab 을 오가도
// 표시가 흔들리지 않고, 지금 작업 중인 저장소 하나를 가리킨다.
//
// 프로세스를 띄우지 않고 라이브러리로 읽는다(ADR-0042). 그래도 Update 안에서 부르면 안 된다 —
// 큰 저장소에서 dirty 판정이 백 밀리초를 넘는다. 백그라운드 작업이 부르고 결과만 msg 로
// 돌아온다(ADR-0030). ctx 는 그 작업의 것이라 취소하면 훑는 것이 멎는다.
func readGitStatus(ctx context.Context, wanted map[string]string) gitSnapshot {
	repo, root, _, ok := openGitRepo(".")
	if !ok {
		return gitSnapshot{}
	}

	// 저장소이지만 commit 이 하나도 없으면 여기서 갈린다. 찍을 것이 없다.
	head, err := repo.Head()
	if err != nil {
		return gitSnapshot{}
	}

	// detached HEAD 면 이름이 `HEAD` 그대로다. branch 가 비는 것이 곧 detached 다.
	branch := ""
	if head.Name() != plumbing.HEAD {
		branch = head.Name().Short()
	}

	// 자리 수는 저장소 크기에서 온다. 해시보다 길게 나올 일은 없지만 자르는 자리라 막아 둔다.
	hash := head.Hash().String()

	snapshot := gitSnapshot{
		status: gitStatus{
			branch: branch,
			commit: hash[:min(gitShortHashLen(gitPackedObjectCount(root)), len(hash))],
			head:   hash,
		},
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return snapshot
	}

	snapshot.changes = gitFileChanges(ctx, repo, root, commit.TreeHash)
	snapshot.status.dirty = len(snapshot.changes) > 0
	snapshot.bases = gitReadBases(commit, root, wanted, hash)

	return snapshot
}

// gitSnapshot 은 갱신 작업 한 번이 읽어 오는 것 전부다.
//
// 셋이 한 번에 온다. 같은 저장소를 세 번 여는 대신 한 번 열어 다 읽고, 화면에 놓이는 순간도
// 하나여서 statusBar 의 `*` 와 트리 마커와 줄 마커가 서로 다른 순간의 사실을 말하지 않는다.
type gitSnapshot struct {
	status  gitStatus
	changes gitChanges

	// bases 는 열려 있는 파일들의 HEAD 원본이다. 절대 경로가 키다.
	// HEAD 에 없는 파일은 값이 nil 인 채 키만 든다 — 「없다」와 「아직 안 읽었다」가 갈린다.
	bases map[string][][]byte
}

// gitReadBases 는 열려 있는 파일들의 HEAD 원본을 읽는다.
//
// wanted 는 「경로 → 그 buffer 가 지금 들고 있는 원본의 HEAD 해시」다. 그 해시가 지금 HEAD 와
// 같으면 읽지 않는다 — 5 초마다 도는 자리라(gitCooldown) 매번 blob 을 풀면 그 값이 그대로 든다.
//
// 읽지 못한 파일은 키를 두지 않는다. 다음 갱신에서 다시 물어본다.
func gitReadBases(commit *object.Commit, root string, wanted map[string]string, head string) map[string][][]byte {
	bases := map[string][][]byte{}

	for path, at := range wanted {
		if at == head {
			continue
		}

		rel, ok := gitRelPath(root, path)
		if !ok {
			// 저장소 밖의 파일이다. 견줄 원본이 없다는 것도 답이라 키를 둔다.
			bases[path] = nil

			continue
		}

		file, err := commit.File(rel)
		if err != nil {
			// HEAD 에 없는 파일이다. 새로 만든 것이 대개 이것이다.
			bases[path] = nil

			continue
		}

		contents, err := file.Contents()
		if err != nil {
			continue
		}

		lines, _ := splitLines([]byte(contents))
		bases[path] = lines
	}

	return bases
}

// gitRelPath 는 열려 있는 파일의 경로를 저장소 뿌리 기준 이름으로 바꾼다. 뿌리 밖이면 거짓이다.
//
// **상대 경로로 열린 파일이 있다.** CLI 인자로 받은 것이 그대로 buffer 의 경로가 되어서
// (`zn internal/core/layout.go`) 절대 경로로 맞추는 일이 먼저다. symlink 도 푼다 —
// 뿌리는 go-git 이 풀어서 주므로(openGitRepo) 한쪽만 풀려 있으면 견줄 수 없다.
func gitRelPath(root, path string) (string, bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		resolved = absolute
	}

	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

// label 은 statusBar 에 찍히는 글자다. `master(a1b2c3d*)` 이고 `*` 가 dirty 다.
//
// detached HEAD 는 branch 자리가 비어서 해시만 찍는다. `(a1b2c3d)` 처럼 빈 괄호를 두면
// 이름을 못 읽은 것처럼 보이는데, 사실은 가리킬 branch 가 없는 상태다.
func (g gitStatus) label() string {
	if g.commit == "" {
		return ""
	}

	commit := g.commit
	if g.dirty {
		commit += "*"
	}

	if g.branch == "" {
		return commit
	}

	return g.branch + "(" + commit + ")"
}

// gitCooldown 은 갱신이 끝난 뒤 다음 갱신까지 쉬는 시간이다.
//
// 주기가 아니라 cooldown 이다 — 5 초를 재는 출발선이 「지난 tick」이 아니라 「지난 갱신이
// 끝난 시각」이다. 그래서 갱신이 오래 걸리는 저장소에서도 쉬는 시간이 반드시 5 초 들어간다.
// 고정 주기였을 때는 갱신이 5 초를 넘으면 끝나는 대로 다음 것이 시작해 사실상 쉼 없이
// 돌았다(ADR-0030 이 감수한 것). cooldown 은 그 상태를 만들지 않는다(ADR-0043).
//
// 5 초는 다른 터미널에서 `commit`·`checkout` 을 하고 편집기로 눈을 돌리는 사이에 값이 맞는
// 정도다.
const gitCooldown = 5 * time.Second

// gitJobName 은 갱신 작업의 이름이자 신원이다.
//
// 같은 이름은 한 번에 하나만 돈다(job.go). cooldown 이라 tick 이 갱신과 겹치지는 않지만,
// `:w` 로 저장하는 길이 갱신을 한 번 더 내므로 그때 겹칠 수 있다 — 그것이 쌓이지 않게 한다.
const gitJobName = "git 상태"

// gitTickMsg 는 다시 읽을 때가 되었다는 것이다. 갱신 자체는 이것을 받고 시작하는 작업이 한다.
type gitTickMsg time.Time

// scheduleGitTick 은 cooldown 을 재고 다음 갱신을 예약한다.
//
// 예약된 tick 은 늘 하나다. `:w` 나 파일 열기로 갱신이 한 번 더 도는 일이 있고, 그것이
// 끝날 때도 이 자리를 지나는데 — 그때마다 새로 걸면 고리가 갈라져서 저장할 때마다 갱신이
// 두 배로 는다. 이미 걸어둔 것이 있으면 그것을 그대로 쓴다(ADR-0043).
func (e *editor) scheduleGitTick() tea.Cmd {
	if e.gitTickScheduled {
		return nil
	}

	e.gitTickScheduled = true

	return tea.Tick(gitCooldown, func(t time.Time) tea.Msg {
		return gitTickMsg(t)
	})
}

// startGitRefresh 는 저장소 상태를 백그라운드에서 읽는 작업을 시작한다.
//
// 결과는 apply 가 editor 에 넣는다 — Update 안에서 불리므로 잠금이 필요 없다(job.go).
func (e *editor) startGitRefresh() tea.Cmd {
	// **앞의 HEAD 와 서버들을 여기서 뜬다.** 작업이 도는 동안 editor 를 읽을 수 없고, 뜨는
	// 자리는 Update 안이라 안전하다. 떠 있는 서버가 없으면 nil 이고 그때는 알릴 곳이 없다.
	previous := e.git.head
	clients := e.runningClients()

	// 열려 있는 파일마다 「지금 들고 있는 원본이 어느 HEAD 의 것인가」를 같이 뜬다.
	// 그것이 지금 HEAD 와 같으면 작업이 blob 을 다시 풀지 않는다(gitReadBases).
	wanted := make(map[string]string, len(e.buffers))
	for i := range e.buffers {
		if e.buffers[i].path != "" {
			wanted[e.buffers[i].path] = e.buffers[i].gitHead()
		}
	}

	return e.startJob(gitJobName, nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress, 1)

		go func() {
			defer close(ch)

			snapshot := readGitStatus(ctx, wanted)
			status := snapshot.status

			// **HEAD 가 움직였으면 그 사이에 달라진 파일을 언어 서버에 알린다.**
			// 알리지 않으면 `git checkout` 뒤에 「쓰는 곳이 있는데 없다」가 된다(ADR-0092).
			//
			// 처음 읽는 때(previous 가 빈 때) 는 알리지 않는다. 견줄 앞이 없고, 서버는 그때
			// 막 뜬 것이라 디스크를 이미 지금 모습으로 읽었다.
			serverWatchChanges(ctx, clients, previous, status.head)

			// 끊긴 작업이 읽어온 것은 "저장소가 아니다" 와 구별되지 않는다.
			// 그대로 넣으면 편집기를 끝내는 길에 표시가 사라진다.
			if err := ctx.Err(); err != nil {
				ch <- jobProgress{err: err}

				return
			}

			// 요약은 읽어온 표시 그대로다. `:jobs` 에서 언제 무엇으로 바뀌었는지가 보인다.
			summary := status.label()
			if summary == "" {
				summary = "저장소 아님"
			}

			ch <- jobProgress{
				done:    1,
				total:   1,
				summary: summary,
				apply:   func(e *editor) { e.applyGitSnapshot(snapshot) },
			}
		}()

		return ch
	})
}

// applyGitSnapshot 은 읽어 온 것을 화면 상태에 놓는다. Update 안에서 불리므로 잠금이 없다(job.go).
//
// 원본을 받은 buffer 는 그 자리에서 줄 마커를 다시 낸다. `git commit` 뒤에 마커가 사라지고
// `git checkout` 뒤에 기준이 바뀌는 것이 이 자리다 — 키를 치지 않아도 화면이 따라온다.
//
// 저장소가 아니면(head 가 빈 문자열) 들고 있던 것을 전부 내린다. 표시가 남아 있으면 그것이
// 어느 저장소의 것인지 알 수 없다.
func (e *editor) applyGitSnapshot(snapshot gitSnapshot) {
	e.git = snapshot.status
	e.gitChanges = snapshot.changes

	for i := range e.buffers {
		buf := &e.buffers[i]

		if snapshot.status.head == "" {
			buf.clearGitBase()

			continue
		}

		if base, ok := snapshot.bases[buf.path]; ok {
			buf.setGitBase(base, snapshot.status.head)

			continue
		}

		buf.refreshGitLines()
	}
}
