package core

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// paletteHit 은 거른 결과 한 줄이다.
type paletteHit struct {
	index int // files 또는 paletteCommands 에서의 자리
	score int

	// positions 는 맞은 글자가 시작하는 byte offset 이다. 강조가 이것으로 자른다.
	positions []int
}

// filterPalette 는 후보를 걸러 점수 높은 순으로 준다.
//
// 빈 패턴은 매칭도 정렬도 하지 않는다. 열자마자 수만 개를 정렬해서 열 줄을 그릴 이유가 없다.
func filterPalette(pattern string, labels []string) []paletteHit {
	hits := make([]paletteHit, 0, len(labels))

	if pattern == "" {
		for i := range labels {
			hits = append(hits, paletteHit{index: i})
		}

		return hits
	}

	for i, label := range labels {
		score, positions, ok := fuzzyMatch(pattern, label)
		if !ok {
			continue
		}

		hits = append(hits, paletteHit{index: i, score: score, positions: positions})
	}

	// 동점이면 짧은 것이 위다. 길이는 점수에 넣지 않았다(palette-match.go).
	// 그래도 같으면 원래 자리 순이다 — SortFunc 는 안정 정렬이 아니라서, 이것이 없으면
	// 키를 칠 때마다 같은 점수끼리 자리를 바꾼다.
	slices.SortFunc(hits, func(a, b paletteHit) int {
		if a.score != b.score {
			return b.score - a.score
		}
		if len(labels[a.index]) != len(labels[b.index]) {
			return len(labels[a.index]) - len(labels[b.index])
		}

		return a.index - b.index
	})

	return hits
}

// indexChunk 는 한 조각에 실어 보내는 파일 수다.
// 조각마다 팔레트가 목록을 다시 거르므로 너무 잘게 나누면 거르는 일이 그만큼 늘어난다.
const indexChunk = 1024

// indexFiles 는 고를 수 있는 파일 목록을 백그라운드에서 모은다. root 기준 상대 경로다.
//
// 저장소 안이면 git 에게 통으로 묻는다 — `.gitignore` 가 공짜로 따라오고 프로세스가 하나다.
// 규칙을 직접 구현하지 않는 것은 sidebar 의 gitignore 표시와 같은 태도다(ADR-0005).
// 저장소가 아니거나 git 이 없으면 직접 훑고 `.git` 만 건너뛴다.
//
// 다 모으기 전에는 개수만 알린다. 어디까지 왔는지 모르는 채로 목록을 조금씩 보여주면 순서가
// 뒤에 뒤집히기 때문이다. 정렬을 끝낸 배열은 다시 고치지 않으므로 조각이 그 앞부분을 그대로
// 가리켜도 된다 — 보내는 쪽도 받는 쪽도 복사하지 않는다.
//
// ctx 가 끊기면 그만둔다. git 프로세스도 같이 죽고, 보내다 막히는 자리마다 빠져나온다 —
// 취소한 뒤 아무도 받지 않는 채널에 goroutine 이 남지 않는다(ADR-0027).
func indexFiles(ctx context.Context, root string) <-chan jobProgress {
	ch := make(chan jobProgress)

	// send 는 조각 하나를 보낸다. 취소됐으면 false 를 주고 부르는 쪽이 그만둔다.
	send := func(progress jobProgress) bool {
		select {
		case ch <- progress:
			return true
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(ch)

		files, ok := gitFiles(ctx, root)
		if !ok {
			files = walkFiles(ctx, root, func(done int) bool {
				return send(jobProgress{done: done})
			})
		}

		// 훑는 도중에 끊겼다. 모은 것은 반쪽이라 목록에 붓지 않고 그냥 물러난다.
		// 취소했다는 것은 실행기가 이미 적어 두었다(job.go).
		if ctx.Err() != nil {
			return
		}

		// git 은 추적 중인 것과 아직 추가하지 않은 것을 따로 모아서 주므로 섞어 정렬한다.
		// 아무것도 치지 않았을 때 보이는 첫 화면이 이 순서다 — 예측할 수 있는 차례여야 한다.
		slices.Sort(files)

		// 조각이 하나도 없으면 앞서 보던 목록이 그대로 남는다. 빈 것도 결과이므로 한 번은 보낸다.
		if len(files) == 0 {
			send(jobProgress{apply: func(e *editor) { e.files = nil }, summary: "0 개"})

			return
		}

		for sent := 0; sent < len(files); sent += indexChunk {
			done := min(sent+indexChunk, len(files))

			progress := jobProgress{
				done:  done,
				total: len(files),
				apply: func(e *editor) { e.files = files[:done] },
			}
			// 마지막 조각이 이 작업이 무엇을 했는지 남긴다.
			if done == len(files) {
				progress.summary = formatCount(len(files)) + " 개"
			}

			if !send(progress) {
				return
			}
		}
	}()

	return ch
}

// gitFiles 는 git 이 아는 파일 목록이다. 추적 중인 것과 아직 추가하지 않은 것을 모두 주되
// 무시된 것은 빼준다. 저장소가 아니면 false 다.
//
// `git ls-files` 를 부르지 않고 index 와 작업 트리를 직접 읽는다(ADR-0042). 추적 중인 것은
// index 가 그대로 들고 있어서 값이 거의 없고, 나머지를 찾는 훑기가 이 함수에서 가장 오래
// 걸리는 자리다 — 무시된 디렉터리는 들어가지 않는다.
//
// ctx 가 끊기면 반쪽짜리 목록을 주지 않고 false 를 준다.
func gitFiles(ctx context.Context, root string) ([]string, bool) {
	// prefix 는 저장소 뿌리에서 root 까지다. root 가 뿌리보다 아래면 그 아래만 주고 경로도
	// root 기준이다 — `git ls-files` 를 그 디렉터리에서 부르던 것과 같다.
	repo, repoRoot, prefix, ok := openGitRepo(root)
	if !ok {
		return nil, false
	}

	index, err := repo.Storer.Index()
	if err != nil {
		return nil, false
	}

	seen := map[string]bool{}
	files := []string{}

	// add 는 뿌리 기준 경로를 root 기준으로 바꿔 담는다. root 밖의 것은 버린다.
	add := func(rel string) {
		if prefix != "" {
			if !strings.HasPrefix(rel, prefix+"/") {
				return
			}

			rel = rel[len(prefix)+1:]
		}

		if seen[rel] {
			return
		}

		seen[rel] = true
		files = append(files, rel)
	}

	// 추적 중인 것이다(`--cached`). 무시되는 자리에 있어도 index 에 있으면 준다.
	for _, entry := range index.Entries {
		add(entry.Name)
	}

	// 아직 추가하지 않았고 무시되지도 않은 것이다(`--others --exclude-standard`).
	walkGitFiles(ctx, repoRoot, prefix, func(rel string) bool {
		add(rel)

		return true
	})

	if ctx.Err() != nil {
		return nil, false
	}

	return files, true
}

// walkFiles 는 root 아래를 직접 훑는다. `.git` 은 어느 깊이에서든 건너뛴다.
//
// 읽지 못하는 디렉터리는 조용히 지나간다. 목록이 조금 모자랄 뿐이고, 팔레트를 못 여는 것보다 낫다.
//
// report 는 지금까지 센 개수를 알린다. 훑는 것이 가장 오래 걸리는데 그동안 진행 표시가 0 에
// 멈춰 있으면 멎은 것으로 보인다. 조각마다 부르지 않고 indexChunk 마다 부른다.
// report 가 false 를 주면 취소된 것이라 훑기를 그만둔다.
func walkFiles(ctx context.Context, root string, report func(done int) bool) []string {
	files := []string{}

	filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return fs.SkipDir
			}

			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		files = append(files, filepath.ToSlash(rel))

		if len(files)%indexChunk == 0 && !report(len(files)) {
			return ctx.Err()
		}

		return nil
	})

	return files
}

// paletteCommand 는 `>` 목록의 한 줄이다.
//
// `:` 명령 표와 별개다. `:` 는 vim 의 ex command 라 짧은 것이 미덕이고, 이쪽은 목록에서
// 눈으로 읽는 것이라 이름이 설명적이어야 한다.
type paletteCommand struct {
	name string // 목록 왼쪽에 그대로 보인다

	// hint 는 영문 설명이고 alias 는 같은 일을 하는 command-line 명령이다.
	// 둘 다 행 오른쪽에 흐리게 보이면서 이름과 함께 매칭 대상이 된다 —
	// 한글 이름을 모르고도 `tree` 나 `trim` 으로 찾을 수 있어야 한다.
	hint  string
	alias string

	// run 은 editor 를 포인터로 받는다. normalMode·commandMode·quitAll 과 같은 서명이고,
	// 여기서 고친 것이 곧 다음 화면의 상태다(ADR-0026).
	run func(e *editor) (tea.Model, tea.Cmd)
}

// paletteCommands 는 `>` 로 고를 수 있는 명령 전부다. 새 명령은 여기 한 줄이 는다.
var paletteCommands = []paletteCommand{
	{name: "줄 끝 공백 지우기", hint: "trim trailing space", run: runTrimTrailingSpace},
	{name: "파일 다시 읽기", hint: "reload file", alias: ":e", run: runReloadFile},
	{name: "다른 tab 모두 닫기", hint: "close other tabs", run: runCloseOtherTabs},
	{name: "파일 트리 열기/닫기", hint: "toggle file tree", alias: ":tree", run: runToggleTree},
	{name: "검색 강조 끄기", hint: "disable search highlight", alias: ":noh", run: runDisableHighlight},
	{name: "작업 목록", hint: "jobs", alias: ":jobs", run: runJobs},
	{name: "정의로 가기", hint: "go to definition", run: runGotoDefinition},
}

// label 은 화면에 보이는 것 전부를 이어 붙인 것이다. 매칭이 이것을 본다.
// 보이는 순서와 같아야 맞은 자리를 그대로 강조에 쓸 수 있다.
func (c paletteCommand) label() string {
	return strings.TrimRight(c.name+" "+c.hint+" "+c.alias, " ")
}

// detail 은 행 오른쪽에 흐리게 붙는 것이다.
func (c paletteCommand) detail() string {
	return strings.TrimRight(c.hint+" "+c.alias, " ")
}

// runGotoDefinition 은 커서 자리의 정의로 간다. normal mode 의 `\gd` 와 같은 자리로 간다
// (ADR-0051).
//
// 팔레트를 닫고 normal 로 돌아가며 묻는 Cmd 를 같이 낸다. 알림은 startDefinition 이 적으므로
// 여기서 덧붙이지 않는다 — 「찾는 중」과 「Go 파일이 아니다」가 그쪽에서 갈린다.
func runGotoDefinition(e *editor) (tea.Model, tea.Cmd) {
	cmd := e.startDefinition()

	model, modeCmd := normalMode(e)

	return model, tea.Batch(cmd, modeCmd)
}

func runTrimTrailingSpace(e *editor) (tea.Model, tea.Cmd) {
	// 읽기 전용 파일은 고치지 않는다(readonly.go).
	if e.refuseReadOnly() {
		return normalMode(e)
	}

	buf := e.activeBuffer()
	width := e.contentWidth()

	count := buf.trimTrailingSpace(width)
	if count == 0 {
		return normalModeMessage(e, "지울 줄 끝 공백이 없습니다")
	}

	buf.clampToNormal(width)
	e.scrollToCursor()

	return normalModeMessage(e, fmt.Sprintf("%d 줄의 끝 공백을 지웠습니다", count))
}

// runReloadFile 은 파일을 다시 읽는다. 밖에서 바뀐 내용을 편집기 안으로 가져오는 길이다.
//
// 저장하지 않은 변경이 있으면 그것이 사라지므로 한 번 더 묻는다. 팔레트 항목에는 `:w!` 의 `!`
// 처럼 강제를 붙일 자리가 없어서 확인창을 쓴다 (ADR-0016).
func runReloadFile(e *editor) (tea.Model, tea.Cmd) {
	// 이름이 없으면 다시 읽을 곳도 없다. 물어보기 전에 여기서 끝낸다 —
	// Yes 를 눌러도 실패로 끝나는 확인창을 띄우지 않는다.
	if e.activeBuffer().path == "" {
		return normalModeMessage(e, "파일 이름이 없습니다")
	}

	if !e.activeBuffer().dirty {
		return reloadFile(e)
	}

	// 취소하면 팔레트가 아니라 normal 로 돌아간다. `:q` 의 확인창과 같다.
	back, _ := normalMode(e)

	return ConfirmDiscard(back, e, "다시 읽으시겠습니까?", func() (tea.Model, tea.Cmd) {
		return reloadFile(e)
	}), nil
}

// reloadFile 은 묻지 않고 다시 읽는다. 확인창의 Yes 와 dirty 가 아닐 때가 쓴다.
func reloadFile(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	if err := buf.Reload(); err != nil {
		return normalModeMessage(e, errors.Cause(err).Error())
	}

	// 커서 칸은 유지하지만 그 자리가 새 내용에서는 줄 끝 다음일 수 있다.
	buf.clampToNormal(e.contentWidth())
	e.scrollToCursor()

	return normalModeMessage(e, "다시 읽음: "+buf.path)
}

// runCloseOtherTabs 는 지금 보고 있는 tab 만 남기고 나머지를 닫는다.
//
// 다른 tab 에 저장하지 않은 변경이 있으면 한 번 더 묻는다. 그 tab 을 보고 있지 않으니 무엇을
// 잃는지 화면에 드러나지 않아서다. 팔레트 항목에는 `:q!` 의 `!` 처럼 강제를 붙일 자리가 없어서
// 확인창을 쓴다 — '파일 다시 읽기' 와 같다(ADR-0016).
func runCloseOtherTabs(e *editor) (tea.Model, tea.Cmd) {
	// tab 이 하나뿐이면 닫을 것이 없다. 확인창도 띄우지 않고 여기서 끝낸다.
	if len(e.buffers) < 2 {
		return normalModeMessage(e, "닫을 다른 tab 이 없습니다")
	}

	if !e.otherDirty() {
		return closeOtherTabs(e)
	}

	// 취소하면 팔레트가 아니라 normal 로 돌아간다. `:q` 의 확인창과 같다.
	back, _ := normalMode(e)

	return ConfirmDiscard(back, e, "다른 tab 을 모두 닫으시겠습니까?", func() (tea.Model, tea.Cmd) {
		return closeOtherTabs(e)
	}), nil
}

// closeOtherTabs 는 묻지 않고 닫는다. 확인창의 Yes 와 잃을 것이 없을 때가 쓴다.
func closeOtherTabs(e *editor) (tea.Model, tea.Cmd) {
	closed := e.closeOtherTabs()

	return normalModeMessage(e, fmt.Sprintf("%d 개의 tab 을 닫았습니다", closed))
}

// runDisableHighlight 는 강조만 끈다. 마지막 검색은 남아서 `n` 이 계속 먹는다. `:noh` 와 같다.
//
// 켜져 있지 않아도 아무 말 하지 않는다 — 끄라고 해서 껐고, 결과가 같다.
func runDisableHighlight(e *editor) (tea.Model, tea.Cmd) {
	e.search.highlight = false

	return normalMode(e)
}

func runToggleTree(e *editor) (tea.Model, tea.Cmd) {
	// 여는 쪽은 뿌리를 읽는 작업을 시작한다. 그 Cmd 를 흘리면 트리가 영영 `… 읽는 중` 이다.
	load, err := e.toggleTree()
	if err != nil {
		return normalModeMessage(e, errors.Cause(err).Error())
	}

	model, next := normalMode(e)

	return model, tea.Batch(next, load)
}
