package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaceEditor 는 치환만 시키는 최소 editor 다. 판도 tab 도 없다 — 여기서 보는 것은
// 「무엇을 고쳐 어디에 쓰는가」뿐이고, 물어보는 손은 판이 든다(view-grep.go).
func replaceEditor() *editor {
	return &editor{boxChars: boxUnicode, width: wide, height: 40}
}

// mustSubstitute 는 시험에서 `:s` 문법 한 줄을 뜯는다.
func mustSubstitute(t *testing.T, input string) substitution {
	t.Helper()

	sub, err := parseSubstitute(input)
	require.NoError(t, err)

	return sub
}

func TestReplaceTargetsGroupsByFile(t *testing.T) {
	// 차례가 뒤섞여 오고 같은 줄이 두 번 온다.
	paths, lines := replaceTargets([]grepHit{
		{path: "b.go", line: 4},
		{path: "a.go", line: 9},
		{path: "a.go", line: 2},
		{path: "a.go", line: 2},
	})

	assert.Equal(t, []string{"a.go", "b.go"}, paths, "경로 차례를 고정한다")
	assert.Equal(t, map[string][]int{"a.go": {2, 9}, "b.go": {4}}, lines, "줄도 차례대로이고 겹치지 않는다")
}

func TestReplaceLinesInTouchesOnlyChosenLines(t *testing.T) {
	lines := [][]byte{
		[]byte("foo one"),
		[]byte("foo two"),
		[]byte("foo three"),
	}

	next, changed, missed := replaceLinesIn(lines, []int{0, 2}, mustSubstitute(t, "/foo/bar/"))

	assert.Equal(t, 2, changed)
	assert.Equal(t, 0, missed)
	assert.Equal(t, "bar one", string(next[0]))
	assert.Equal(t, "foo two", string(next[1]), "고르지 않은 줄은 그대로다")
	assert.Equal(t, "bar three", string(next[2]))

	// 들어온 줄 묶음은 제자리에서 바뀌지 않는다(ADR-0001).
	assert.Equal(t, "foo one", string(lines[0]))
}

// `g` 가 있으면 목록에 안 보이던 같은 줄의 둘째 매칭도 같이 바뀐다(ADR-0097 §2).
func TestReplaceLinesInAppliesAllOnTheLine(t *testing.T) {
	lines := [][]byte{[]byte("foo foo foo")}

	one, _, _ := replaceLinesIn(lines, []int{0}, mustSubstitute(t, "/foo/bar/"))
	assert.Equal(t, "bar foo foo", string(one[0]), "`g` 가 없으면 그 줄의 첫 하나다")

	all, _, _ := replaceLinesIn(lines, []int{0}, mustSubstitute(t, "/foo/bar/g"))
	assert.Equal(t, "bar bar bar", string(all[0]))
}

// 찾은 뒤에 그 줄이 바뀌었으면 건드리지 않고 센다(ADR-0097 §3).
func TestReplaceLinesInCountsMissed(t *testing.T) {
	lines := [][]byte{
		[]byte("foo one"),
		[]byte("그 사이 바뀐 줄"),
	}

	next, changed, missed := replaceLinesIn(lines, []int{0, 1, 99}, mustSubstitute(t, "/foo/bar/"))

	assert.Equal(t, 1, changed)
	assert.Equal(t, 2, missed, "안 걸리는 줄과 파일 밖의 줄 둘")
	assert.Equal(t, "그 사이 바뀐 줄", string(next[1]))
}

// 열려 있지 않은 파일은 그 자리에서 열어 고치고 쓴다. tab 은 늘지 않는다.
func TestReplaceFileWritesUnopenedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("foo one\nfoo two\n"), 0o644))

	e := replaceEditor()
	before := len(e.buffers)

	changed, missed, err := e.replaceFile(path, []int{1}, mustSubstitute(t, "/foo/bar/"))
	require.NoError(t, err)

	assert.Equal(t, 1, changed)
	assert.Equal(t, 0, missed)
	assert.Equal(t, before, len(e.buffers), "tab 을 늘리지 않는다")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "foo one\nbar two\n", string(data))
}

// 열려 있는 tab 은 buffer 를 고쳐 저장한다. 그래서 그 tab 에서는 `u` 가 산다(ADR-0097 §4).
func TestReplaceFileGoesThroughTheOpenBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("foo one\nfoo two\n"), 0o644))

	e := replaceEditor()
	buf, err := OpenBuffer(path)
	require.NoError(t, err)
	e.buffers = []viewport{buf}
	e.active = 0

	changed, _, err := e.replaceFile(path, []int{0}, mustSubstitute(t, "/foo/bar/"))
	require.NoError(t, err)
	require.Equal(t, 1, changed)

	assert.Equal(t, "bar one", string(e.buffers[0].Lines[0]), "화면에 보이는 글이 같이 바뀐다")
	assert.False(t, e.buffers[0].Dirty, "저장까지 했다")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "bar one\nfoo two\n", string(data))

	// 되돌리기가 그 tab 에서는 산다.
	e.buffers[0].ApplyUndo()
	assert.Equal(t, "foo one", string(e.buffers[0].Lines[0]))
}

func TestApplyReplaceOverFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("foo\nkeep\nfoo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("keep\nfoo\n"), 0o644))

	paths, lines := replaceTargets([]grepHit{
		{path: "a.go", line: 0},
		{path: "a.go", line: 2},
		{path: "b.go", line: 1},
	})

	out := replaceEditor().applyReplace(dir, paths, lines, mustSubstitute(t, "/foo/bar/"))

	assert.Equal(t, 2, out.files())
	assert.Equal(t, 3, out.lines)
	assert.Equal(t, 0, out.missed)
	assert.Empty(t, out.failed)

	a, err := os.ReadFile(filepath.Join(dir, "a.go"))
	require.NoError(t, err)
	assert.Equal(t, "bar\nkeep\nbar\n", string(a))

	b, err := os.ReadFile(filepath.Join(dir, "b.go"))
	require.NoError(t, err)
	assert.Equal(t, "keep\nbar\n", string(b))
}

// 읽기 전용 파일은 쓰지 않고 까닭을 남긴다. 나머지 파일은 그대로 간다.
func TestApplyReplaceReportsUnwritableFile(t *testing.T) {
	dir := t.TempDir()
	readonly := filepath.Join(dir, "a.go")
	require.NoError(t, os.WriteFile(readonly, []byte("foo\n"), 0o444))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("foo\n"), 0o644))

	paths, lines := replaceTargets([]grepHit{
		{path: "a.go", line: 0},
		{path: "b.go", line: 0},
	})

	out := replaceEditor().applyReplace(dir, paths, lines, mustSubstitute(t, "/foo/bar/"))

	assert.Equal(t, 1, out.files(), "쓸 수 있던 것만 센다")
	require.Len(t, out.failed, 1)
	assert.Contains(t, out.failed[0], "a.go")

	kept, err := os.ReadFile(readonly)
	require.NoError(t, err)
	assert.Equal(t, "foo\n", string(kept), "읽기 전용 파일은 그대로다")
}

func TestReplacePromptCountsFilesAndLines(t *testing.T) {
	paths, lines := replaceTargets([]grepHit{
		{path: "a.go", line: 0},
		{path: "a.go", line: 3},
		{path: "b.go", line: 1},
	})

	assert.Equal(t, "파일 2 개 3 줄을 바꿉니다", replacePrompt(paths, lines))
}

func TestReplaceMessageMentionsMissed(t *testing.T) {
	sub := mustSubstitute(t, "/foo/bar/")

	// 앞에 적히는 것은 패턴이다. 바꿀 글은 Go 문법으로 옮겨져 있어 그대로 보이면 안 된다.
	assert.Equal(t, "foo: 파일 2 개 3 줄을 바꿨습니다",
		replaceMessage(sub, replaceOutcome{touched: map[string]bool{"a.go": true, "b.go": true}, lines: 3}))

	assert.Contains(t, replaceMessage(sub, replaceOutcome{touched: map[string]bool{"a.go": true, "b.go": true}, lines: 3, missed: 2}),
		"그 사이 바뀐 줄 2")
}

// newReplaceView 는 치환 대기 상태의 판과 그 파일들이다. 검색 판과 같은 것이고
// `replace` 만 서 있다(ADR-0097 §1).
//
// newGrepView 를 쓰지 않고 같은 것을 다시 짓는 것은 **경로가 필요해서**다. 그쪽은 파일을
// 안에서 만들고 알려주지 않는데, 여기서 볼 것이 「그 파일이 실제로 바뀌었나」다.
//
// 적중의 경로가 절대 경로라 뿌리는 비워 둔다 — `filepath.Join("", abs)` 가 그 경로 그대로다.
func newReplaceView(t *testing.T, flags string, lines ...int) (viewGrep, []string) {
	t.Helper()

	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	sub := mustSubstitute(t, "/func/쓴다/"+flags)

	e.grep = grepResult{input: sub.input, pattern: sub.pattern, hits: grepHitsIn(paths[0], lines...), files: 1}
	e.replace = replacePending{on: true, sub: sub, stepping: sub.confirm}

	model, _ := grepMode(e)

	here, ok := model.(viewGrep)
	require.True(t, ok, "판이 열려야 한다")

	return here, paths
}

// fileLinesOf 는 그 파일을 줄로 읽은 것이다. 무엇이 바뀌었는지를 이것으로 견준다.
func fileLinesOf(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// pressGrep 은 키를 먹이고 판이 아직 열려 있는지 같이 준다.
func pressGrep(t *testing.T, m viewGrep, key string) (tea.Model, bool) {
	t.Helper()

	next, _ := m.press(key)
	_, still := next.(viewGrep)

	return next, still
}

func TestReplaceDrawerAsksBeforeChanging(t *testing.T) {
	m, _ := newReplaceView(t, "g", 2, 4)

	assert.Contains(t, m.hint(), "파일 1 개 2 줄을 바꿉니다")
	assert.Contains(t, m.hint(), "a 전부")
	assert.Contains(t, m.hint(), "y 하나씩")
	assert.Contains(t, m.label(), "치환", "검색 판과 앞머리가 갈린다")
}

// `q` 로 바로 나가면 파일을 건드리지 않는다. 묻기만 하고 아무 일도 하지 않은 것이다.
func TestReplaceDrawerCancelChangesNothing(t *testing.T) {
	m, paths := newReplaceView(t, "g", 2)
	before := fileLinesOf(t, paths[0])

	next, still := pressGrep(t, m, "q")
	assert.False(t, still, "판이 닫힌다")
	assert.Equal(t, before, fileLinesOf(t, paths[0]))
	assert.False(t, next.(viewEditorNormal).replace.on, "대기를 비운다")
}

// `a` 는 목록에 있는 것을 한 번에 바꾸고 판을 닫는다.
func TestReplaceDrawerAppliesAll(t *testing.T) {
	m, paths := newReplaceView(t, "g", 2, 4)

	_, still := pressGrep(t, m, "a")
	require.False(t, still, "판이 닫힌다")

	after := fileLinesOf(t, paths[0])
	assert.Equal(t, "쓴다 a() {}", after[2])
	assert.Equal(t, "쓴다 b() {}", after[4])
	assert.Equal(t, "func c() {}", after[6], "목록에 없던 줄은 그대로다")
}

// `y` 는 하나씩으로 들어간다. 그 한 걸음으로는 아직 아무것도 바뀌지 않는다.
func TestReplaceDrawerStepsIntoAsking(t *testing.T) {
	m, paths := newReplaceView(t, "g", 2)
	before := fileLinesOf(t, paths[0])

	next, still := pressGrep(t, m, "y")
	require.True(t, still, "아직 판이다")

	asking := next.(viewGrep)
	assert.True(t, asking.replace.stepping)
	assert.Contains(t, asking.hint(), "바꿀까요?")
	assert.Equal(t, before, fileLinesOf(t, paths[0]), "들어서기만 해서는 안 바뀐다")
}

// 하나씩에서 `y` 와 `n` 이 고른 줄만 바꾼다.
func TestReplaceDrawerStepAcceptsAndSkips(t *testing.T) {
	m, paths := newReplaceView(t, "gc", 2, 4)
	require.True(t, m.replace.stepping, "`c` 는 곧바로 하나씩이다")

	// 첫 줄은 바꾸고 둘째 줄은 넘긴다.
	next, still := pressGrep(t, m, "y")
	require.True(t, still)

	done, still := pressGrep(t, next.(viewGrep), "n")
	require.False(t, still, "마지막에서 넘기면 끝난다")

	after := fileLinesOf(t, paths[0])
	assert.Equal(t, "쓴다 a() {}", after[2], "`y` 한 줄")
	assert.Equal(t, "func b() {}", after[4], "`n` 한 줄은 그대로다")
	assert.False(t, done.(viewEditorNormal).replace.on)
}

// 하나씩 훑다 친 `a` 가 앞에서 `y` 로 바꾼 것까지 세어 알린다.
func TestReplaceDrawerRestCountsWhatCameBefore(t *testing.T) {
	m, paths := newReplaceView(t, "gc", 2, 4, 6)

	next, _ := pressGrep(t, m, "y")
	done, still := pressGrep(t, next.(viewGrep), "a")
	require.False(t, still)

	after := fileLinesOf(t, paths[0])
	assert.Equal(t, []string{"쓴다 a() {}", "쓴다 b() {}", "쓴다 c() {}"},
		[]string{after[2], after[4], after[6]}, "셋 다 바뀐다")

	assert.Contains(t, done.(viewEditorNormal).notice, "1 개 3 줄")
}

// `:grep` 판에서 `r` 로 들어오는 둘째 문이다. 패턴은 이미 있고 받는 것은 바꿀 글뿐이다
// (ADR-0097 §1).
func TestGrepDrawerTurnsIntoReplace(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	e.grep = grepResult{
		input:   "func",
		pattern: regexp.MustCompile("func"),
		hits:    grepHitsIn(paths[0], 2, 4),
		files:   1,
	}

	model, _ := grepMode(e)
	m := model.(viewGrep)

	assert.Contains(t, m.hint(), "r 바꾸기", "검색 판이 그 문을 적는다")

	next, _ := m.press("r")
	asking := next.(viewGrep)
	require.True(t, asking.asking)
	assert.Contains(t, asking.hint(), "무엇으로")

	// 글자를 받고 확정한다.
	typed, _ := asking.Update(tea.KeyPressMsg{Code: '쓴', Text: "쓴다"})
	ready, _ := typed.(viewGrep).Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	pending := ready.(viewGrep)
	require.True(t, pending.replace.on, "치환 대기가 된다")
	assert.False(t, pending.asking)
	assert.True(t, pending.replace.sub.all, "판의 단위가 줄이라 `g` 로 간다")
	assert.Contains(t, pending.hint(), "파일 1 개 2 줄을 바꿉니다")

	// 뿌리는 cwd 다. 시험의 파일은 절대 경로라 비워서 견준다.
	pending.replace.root = ""

	done, _ := pending.press("a")
	require.IsType(t, viewEditorNormal{}, done)

	after := fileLinesOf(t, paths[0])
	assert.Equal(t, "쓴다 a() {}", after[2])
	assert.Equal(t, "쓴다 b() {}", after[4])
}

// 치환 대기에서 `enter` 는 아무 일도 하지 않는다. 아래 줄이 내놓는 답은 셋뿐이다.
func TestReplaceDrawerIgnoresEnter(t *testing.T) {
	m, paths := newReplaceView(t, "g", 2)
	before := fileLinesOf(t, paths[0])

	next, still := pressGrep(t, m, "enter")
	require.True(t, still, "판을 떠나지 않는다")
	assert.True(t, next.(viewGrep).replace.on, "대기가 그대로 선다")
	assert.Equal(t, before, fileLinesOf(t, paths[0]))
}

// 그냥 `:grep` 은 늘 검색 판이다. 앞선 치환 대기가 남아 낡은 바꿀 글로 열리면 안 된다.
func TestGrepClearsStalePendingReplace(t *testing.T) {
	e, paths := jumpEditor(t)
	e.active = 0
	require.NoError(t, gotoFile(e, paths[0], 0))

	// 앞의 `:replace` 가 남긴 것처럼 꾸민다.
	e.replace = replacePending{on: true, sub: mustSubstitute(t, "/func/zeta/g")}

	require.NotNil(t, e.startGrep("func"), "검색이 시작된다")
	assert.False(t, e.replace.on, "검색은 대기를 비운다")
}
