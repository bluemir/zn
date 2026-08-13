package core

import (
	"bytes"
	"fmt"
	"io/fs"
	"os/exec"
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

// paletteFiles 는 고를 수 있는 파일 목록이다. root 기준 상대 경로다.
//
// 저장소 안이면 git 에게 통으로 묻는다 — `.gitignore` 가 공짜로 따라오고 프로세스가 하나다.
// 규칙을 직접 구현하지 않는 것은 sidebar 의 gitignore 표시와 같은 태도다(ADR-0005).
// 저장소가 아니거나 git 이 없으면 직접 훑고 `.git` 만 건너뛴다.
func paletteFiles(root string) []string {
	files, ok := gitFiles(root)
	if !ok {
		files = walkFiles(root)
	}

	// git 은 추적 중인 것과 아직 추가하지 않은 것을 따로 모아서 주므로 섞어 정렬한다.
	// 아무것도 치지 않았을 때 보이는 첫 화면이 이 순서다 — 예측할 수 있는 차례여야 한다.
	slices.Sort(files)

	return files
}

// gitFiles 는 git 이 아는 파일 목록이다. 추적 중인 것과 아직 추가하지 않은 것을 모두 주되
// 무시된 것은 빼준다. 저장소가 아니거나 git 이 없으면 false 다.
func gitFiles(root string) ([]string, bool) {
	// `-z` 로 받는다. 이름에 줄바꿈이 든 파일이 있으면 줄 단위로 끊을 수 없다.
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	files := []string{}
	for _, name := range bytes.Split(out, []byte{0}) {
		if len(name) == 0 {
			continue
		}

		files = append(files, string(name))
	}

	return files, true
}

// walkFiles 는 root 아래를 직접 훑는다. `.git` 은 어느 깊이에서든 건너뛴다.
//
// 읽지 못하는 디렉터리는 조용히 지나간다. 목록이 조금 모자랄 뿐이고, 팔레트를 못 여는 것보다 낫다.
func walkFiles(root string) []string {
	files := []string{}

	filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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

	// run 은 editor 를 값으로 받는다. normalMode·commandMode·quitAll 과 같은 서명이다.
	// 값이라서 안전한 이유도 같다 — buffer 는 공유되는 backing array 를 가리키고,
	// sidebar 처럼 값인 필드는 여기서 바꾼 복사본이 그대로 다음 화면에 넘어간다.
	run func(e editor) (tea.Model, tea.Cmd)
}

// paletteCommands 는 `>` 로 고를 수 있는 명령 전부다. 새 명령은 여기 한 줄이 는다.
var paletteCommands = []paletteCommand{
	{name: "줄 끝 공백 지우기", hint: "trim trailing space", run: runTrimTrailingSpace},
	{name: "파일 다시 읽기", hint: "reload file", run: runReloadFile},
	{name: "파일 트리 열기/닫기", hint: "toggle file tree", alias: ":tree", run: runToggleTree},
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

func runTrimTrailingSpace(e editor) (tea.Model, tea.Cmd) {
	buf := e.buffer()
	width := e.contentWidth()

	count := buf.trimTrailingSpace(width)
	if count == 0 {
		return normalModeMessage(e, "지울 줄 끝 공백이 없습니다")
	}

	buf.clampToNormal(width)
	buf.scrollTo(width, e.textHeight())

	return normalModeMessage(e, fmt.Sprintf("%d 줄의 끝 공백을 지웠습니다", count))
}

// runReloadFile 은 파일을 다시 읽는다. 밖에서 바뀐 내용을 편집기 안으로 가져오는 길이다.
//
// 저장하지 않은 변경이 있으면 그것이 사라지므로 한 번 더 묻는다. 팔레트 항목에는 `:w!` 의 `!`
// 처럼 강제를 붙일 자리가 없어서 확인창을 쓴다 (ADR-0016).
func runReloadFile(e editor) (tea.Model, tea.Cmd) {
	// 이름이 없으면 다시 읽을 곳도 없다. 물어보기 전에 여기서 끝낸다 —
	// Yes 를 눌러도 실패로 끝나는 확인창을 띄우지 않는다.
	if e.buffer().path == "" {
		return normalModeMessage(e, "파일 이름이 없습니다")
	}

	if !e.buffer().dirty {
		return reloadFile(e)
	}

	// 취소하면 팔레트가 아니라 normal 로 돌아간다. `:q` 의 확인창과 같다.
	back, _ := normalMode(e)

	return ConfirmDiscard(back, "다시 읽으시겠습니까?", func() (tea.Model, tea.Cmd) {
		return reloadFile(e)
	}), nil
}

// reloadFile 은 묻지 않고 다시 읽는다. 확인창의 Yes 와 dirty 가 아닐 때가 쓴다.
func reloadFile(e editor) (tea.Model, tea.Cmd) {
	buf := e.buffer()

	if err := buf.Reload(); err != nil {
		return normalModeMessage(e, errors.Cause(err).Error())
	}

	// 커서 칸은 유지하지만 그 자리가 새 내용에서는 줄 끝 다음일 수 있다.
	buf.clampToNormal(e.contentWidth())
	buf.scrollTo(e.contentWidth(), e.textHeight())

	// 밖에서 checkout 이나 commit 이 있었을 자리다. 파일을 열 때·저장할 때와 같이 여기서 맞춘다(ADR-0009).
	e.git = readGitStatus()

	return normalModeMessage(e, "다시 읽음: "+buf.path)
}

func runToggleTree(e editor) (tea.Model, tea.Cmd) {
	if err := e.toggleTree(); err != nil {
		return normalModeMessage(e, errors.Cause(err).Error())
	}

	return normalMode(e)
}
