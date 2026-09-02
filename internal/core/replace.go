package core

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/cockroachdb/errors"
)

// 여러 파일 치환이다(ADR-0097).
//
// **빌려 쓰는 것이 셋이다.** 찾는 것은 `:grep` 이고(grep.go), 바꾸는 것은 `:s` 의
// `substitution` 이고(substitute.go), 쓰는 것은 rename 이 낸 손이다(rename.go).
// 이 파일이 새로 하는 일은 「찾은 목록을 파일별로 모아 그 줄만 다시 맞춰 바꾸고 쓰는 것」
// 하나다.
//
// 무엇을 물어보고 어떻게 훑는지는 판이 든다(view-grep.go).

// replacePending 은 찾아 두고 아직 확정하지 않은 치환이다. editor 가 든다.
//
// **`on` 이 판의 갈래를 가른다.** 같은 판(view-grep.go) 이 검색 결과이기도 하고 치환
// 대기이기도 해서, 아래 줄의 물음과 키가 이것으로 갈린다(ADR-0097 §1).
type replacePending struct {
	on  bool
	sub substitution

	// root 는 적중의 상대 경로가 딛는 자리다. 찾을 때 쓴 것을 그대로 들고 있는다 —
	// 판이 열려 있는 동안 다시 재면 그 사이 cwd 가 달라졌을 때 엉뚱한 파일을 쓴다.
	root string

	// stepping 은 하나씩 물어보는 중인지다. `c` flag 로 들어오거나 확인창에서 고른다.
	stepping bool

	// done 은 하나씩 훑으며 지금까지 바꾼 것이다. 끝낼 때 이것을 알린다.
	done replaceOutcome
}

// runReplace 는 `:replace/a/b/g` 다. **찾기만 하고 판을 연다.**
//
// 곧바로 바꾸지 않는 것이 이 기능의 요점이다. 한 명령이 찾자마자 써 버리면 무엇이 바뀔지
// 보기 전에 저장소가 바뀌는데, 되돌리기가 약한 자리라 그것을 감당할 수 없다. 판은 이미
// 둘러보고 확정하는 손을 들고 있어서 미리보기를 새로 만들 것이 없다(ADR-0078, ADR-0097 §1).
//
// 뜯는 것은 `:s` 와 같은 `parseSubstitute` 다. 구분자 규칙과 바꿀 글의 문법이 한 파일
// 치환과 갈릴 자리가 없다(ADR-0084).
func runReplace(e *editor, input string) (tea.Model, tea.Cmd) {
	sub, err := parseSubstitute(input)
	if err != nil {
		return normalModeError(e, err)
	}

	root, err := os.Getwd()
	if err != nil {
		return normalModeError(e, err)
	}

	// **찾은 것이 마지막 검색이 된다.** 판에서 뛴 뒤 `n` 이 남은 자리를 짚고, 하나씩 물어보는
	// 동안 화면에 칠해지는 것도 이것이다. `:s` 와 같은 자리다(ADR-0010, ADR-0084).
	e.search = searchState{input: sub.input, pattern: sub.pattern, direction: searchForward, highlight: true}

	start := e.startGrep(sub.input)
	if start == nil {
		// 패턴이 정규식으로 말이 되지 않았다. startGrep 이 알림을 적었다.
		return normalMode(e)
	}

	// startGrep 이 대기를 비우므로 그 **뒤에** 세운다.
	e.replace = replacePending{on: true, sub: sub, root: root, stepping: sub.confirm}

	model, next := grepMode(e)

	return model, tea.Batch(next, start)
}

// fullPath 는 적중의 상대 경로를 찾을 때 쓴 뿌리에 붙인 것이다.
func (p replacePending) fullPath(rel string) string {
	return filepath.Join(p.root, rel)
}

// replaceTargets 는 적중을 파일별 줄 번호로 모은 것이다.
//
// **적중이 정하는 것은 여기까지다.** 담아둔 `col`·`end` 는 쓰지 않는다 — 찾은 순간과 바꾸는
// 순간 사이에 파일이 움직이므로(fsnotify·편집) 그 byte 자리로 잘라 붙이면 글자 가운데를
// 자른다. 어느 줄인지만 받고 자리는 바꿀 때 다시 맞춘다(ADR-0097 §3).
//
// 경로 차례를 고정한다. map 을 그대로 돌면 알림에 적히는 차례가 부를 때마다 달라진다 —
// rename 이 서버의 map 답에 대고 한 것과 같은 자리다(ADR-0067).
func replaceTargets(hits []grepHit) ([]string, map[string][]int) {
	lines := map[string][]int{}
	for _, hit := range hits {
		lines[hit.path] = append(lines[hit.path], hit.line)
	}

	paths := make([]string, 0, len(lines))
	for path := range lines {
		paths = append(paths, path)
		slices.Sort(lines[path])
		lines[path] = slices.Compact(lines[path])
	}
	slices.Sort(paths)

	return paths, lines
}

// replaceOutcome 은 치환 한 번의 결과다. 알림이 이것을 읽는다.
type replaceOutcome struct {
	// touched 는 실제로 바뀐 파일이다. 수가 아니라 집합인 것은 하나씩 훑는 길 때문이다 —
	// 같은 파일의 다음 줄이 또 오므로, 셀 때마다 더하면 파일 하나를 여러 개로 센다.
	touched map[string]bool

	lines int // 바뀐 줄 수

	// missed 는 목록에 있었는데 지금은 패턴에 안 걸리는 줄 수다.
	//
	// 찾은 뒤에 그 줄이 바뀐 것이다. 조용히 지나가지 않고 세어서 알린다 — 「87 줄을
	// 바꾼다」고 묻고 85 줄만 바꿨으면 그 둘이 어디로 갔는지가 답해져야 한다.
	missed int

	// failed 는 쓰지 못한 파일이다. `경로: 까닭` 이다.
	failed []string
}

// files 는 실제로 바뀐 파일 수다.
func (out replaceOutcome) files() int {
	return len(out.touched)
}

// mark 는 그 파일이 바뀌었다고 적는다.
func (out *replaceOutcome) mark(path string) {
	if out.touched == nil {
		out.touched = map[string]bool{}
	}

	out.touched[path] = true
}

// merge 는 다른 결과를 이것에 더한다. 하나씩 훑다 `a` 로 끝낼 때 앞에서 바꾼 것이 알림에서
// 빠지지 않게 한다.
func (out *replaceOutcome) merge(add replaceOutcome) {
	for path := range add.touched {
		out.mark(path)
	}

	out.lines += add.lines
	out.missed += add.missed
	out.failed = append(out.failed, add.failed...)
}

// replaceLinesIn 은 줄 묶음에서 고른 줄만 바꾼 새 묶음과 바꾼 줄 수·놓친 줄 수를 준다.
//
// **줄 단위다.** `grepLines` 가 줄마다 첫 매칭만 담으므로(ADR-0077) 목록의 한 줄이 곧 여기
// 한 줄이다. 그 줄에 `applyRange` 를 통째로 걸어서, `g` 가 있으면 목록에 안 보이던 같은 줄의
// 둘째 매칭도 같이 바뀐다 — `:s///g` 가 한 파일에서 하는 일과 같아야 한다(ADR-0097 §2).
//
// 줄 슬라이스만 새로 만든다. 줄 자체는 제자리에서 바뀌지 않는다(ADR-0001).
func replaceLinesIn(lines [][]byte, at []int, sub substitution) (next [][]byte, changed, missed int) {
	next = slices.Clone(lines)

	for _, line := range at {
		if line < 0 || line >= len(next) {
			// 찾은 뒤에 파일이 짧아졌다. 놓친 것으로 센다.
			missed++

			continue
		}

		replaced, count := sub.applyRange(next[line], 0, len(next[line]))
		if count == 0 {
			missed++

			continue
		}

		next[line] = replaced
		changed++
	}

	return next, changed, missed
}

// applyReplace 는 고른 파일들을 바꾸고 쓴다.
//
// root 는 적중의 상대 경로가 딛는 자리다. `:grep` 이 훑을 때 쓴 것과 같아야 한다.
func (e *editor) applyReplace(root string, paths []string, lines map[string][]int, sub substitution) replaceOutcome {
	out := replaceOutcome{}

	for _, path := range paths {
		changed, missed, err := e.replaceFile(filepath.Join(root, path), lines[path], sub)
		out.missed += missed

		if err != nil {
			out.failed = append(out.failed, shortenPath(path)+": "+err.Error())

			continue
		}

		if changed > 0 {
			out.mark(path)
			out.lines += changed
		}
	}

	return out
}

// replaceFile 은 파일 하나의 고른 줄을 바꾸고 쓴다. 바꾼 줄 수와 놓친 줄 수를 준다.
//
// **열려 있는 tab 은 buffer 에서 고친다.** 디스크에서 다시 읽어 고치면 저장하지 않은 편집이
// 사라지고 화면에 보이는 글과 파일이 갈린다. buffer 를 거쳐 저장하므로 그 tab 에서는 `u` 가
// 산다 — 열려 있지 않은 파일은 git 만 받는다(ADR-0097 §4).
//
// 열려 있지 않은 파일은 그 자리에서 열어 고치고 쓴다. tab 으로 만들지 않는다. 치환 한 번에
// tab 이 스무 개씩 열리면 그 뒤로 편집기를 쓸 수 없다 — rename 이 같은 까닭으로 물린 길이다
// (ADR-0067).
//
// 포매터는 걸지 않는다(hook 이 nil 이다). 치환 하나에 남의 파일이 통째로 다시 포맷되면
// 그 diff 가 무엇 때문인지 알 수 없다(ADR-0065).
func (e *editor) replaceFile(full string, at []int, sub substitution) (changed, missed int, err error) {
	if index, ok := e.tabOf(full); ok {
		buf := &e.buffers[index]
		if buf.ReadOnly {
			return 0, 0, errors.New("읽기 전용입니다")
		}

		next, changed, missed := replaceLinesIn(buf.Lines, at, sub)
		if changed == 0 {
			return 0, missed, nil
		}

		buf.ReplaceAll(next)

		if _, err := buf.Save(nil); err != nil {
			return 0, missed, err
		}

		return changed, missed, nil
	}

	buf, err := OpenBuffer(full)
	if err != nil {
		return 0, 0, err
	}

	if buf.ReadOnly {
		return 0, 0, errors.New("읽기 전용입니다")
	}

	next, changed, missed := replaceLinesIn(buf.Lines, at, sub)
	if changed == 0 {
		return 0, missed, nil
	}

	buf.Lines = next

	return changed, missed, buf.Write()
}

// replaceMessage 는 무엇을 바꿨는지 한 줄로 적은 것이다.
//
// **이것이 되돌리기의 자리를 대신한다.** 열려 있지 않은 파일은 `u` 가 못 돌리므로 무엇이
// 바뀌었는지가 남아야 한다. 알림 센터에도 그대로 실린다(ADR-0053, ADR-0097 §4).
// 앞에 적는 것은 **패턴**이다. 바꿀 글은 `parseReplacement` 가 Go 문법으로 옮겨 둔 것이라
// (`\1` 이 `${1}` 이 된다) 그대로 보이면 친 것과 다른 글자가 된다.
func replaceMessage(sub substitution, out replaceOutcome) string {
	note := fmt.Sprintf("%s: 파일 %s 개 %s 줄을 바꿨습니다",
		sub.input, formatCount(out.files()), formatCount(out.lines))

	if out.missed > 0 {
		note += fmt.Sprintf(" · 그 사이 바뀐 줄 %s", formatCount(out.missed))
	}

	return note
}

// replaceError 는 쓰지 못한 파일이 있을 때의 알림이다.
//
// **첫 하나만 적는다.** 아래 줄은 한 줄이고 나머지는 `:messages` 에 남는다. rename 과 같은
// 손이다(ADR-0053, ADR-0067).
func replaceError(sub substitution, out replaceOutcome) error {
	return errors.Errorf("%s · 못 쓴 파일 %s: %s",
		replaceMessage(sub, out), formatCount(len(out.failed)), out.failed[0])
}

// replacePrompt 는 바꾸기 전에 아래 줄이 묻는 말이다.
//
// 세는 것이 파일과 **줄**이다. 적중 수가 아닌 것은 목록의 한 줄이 곧 바꿀 한 줄이기
// 때문이다(ADR-0097 §2).
func replacePrompt(paths []string, lines map[string][]int) string {
	count := 0
	for _, path := range paths {
		count += len(lines[path])
	}

	return fmt.Sprintf("파일 %s 개 %s 줄을 바꿉니다", formatCount(len(paths)), formatCount(count))
}
