package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// 저장할 때 파일을 바깥 포매터에 통과시키는 자리다(ADR-0065).
//
// 하는 일은 셋이다 — 어떤 파일에 무엇을 돌릴지 고르기(saveHookFor), 그것을 찾기(goimports),
// buffer 를 흘려 넣고 받아 갈아끼우기(applySaveHook).
//
// **파일이 아니라 buffer 를 넘긴다.** 「맞추는 것이 쓰기보다 먼저다」가 그대로 서려면 아직
// 쓰지 않은 글을 넘겨야 한다 — 파일을 먼저 쓰고 포매터를 돌리면 화면과 파일이 갈리는 순간이
// 생기고, 저장이 막히는 길에서 파일만 바뀐다(ADR-0052).

// saveHookTimeout 은 포매터 하나를 기다리는 한계다.
//
// 저장이 이것을 기다리므로 화면이 그만큼 멈춘다. 잰 값은 `go tool goimports` 가 130ms,
// PATH 의 바이너리가 그보다 짧다 — 2 초는 그 열 배가 넘고, 여기까지 왔으면 그 판이 무언가
// 잘못된 것이라 기다리는 것보다 저장을 끝내는 것이 낫다.
const saveHookTimeout = 2 * time.Second

// saveHook 은 저장할 때 buffer 를 흘려 넣을 명령이다.
type saveHook struct {
	// name 은 알림에 적는 이름이다. 무엇이 내 줄을 고쳤는지 사람이 알아야 한다(ADR-0052).
	name string

	// dir 은 명령을 돌릴 자리, 곧 그 파일이 있는 폴더다.
	// `go tool` 이 어느 모듈의 tool 인지를 여기서 찾는다.
	dir string

	path string
	args []string
}

// errHookNotInstalled 는 표에 적힌 포매터가 이 판에 깔려 있지 않다는 것이다.
//
// **nil 하나로는 두 가지가 갈리지 않는다** — 「이 파일에는 돌릴 것이 없다」(`.txt`) 와
// 「돌릴 것이 있는데 깔려 있지 않다」(`.go` + goimports 없음) 다. 저장은 둘 다 그냥 지나가지만
// 설치를 묻는 것은 뒤쪽에서만 한다.
var errHookNotInstalled = errors.New("포매터가 설치되어 있지 않습니다")

// saveHookFor 는 그 파일을 저장할 때 통과시킬 명령이다.
//
// 표에 없는 확장자는 (nil, nil) 이고, 표에 있는데 깔려 있지 않으면 errHookNotInstalled 다.
//
// **표가 이 switch 다.** 확장자 하나에 map 리터럴을 두면 칸만 늘고 값은 한 줄에 있다 —
// isGoFile 이 syntax 의 언어 표를 빌려 오지 않은 것과 같은 자리다(gopls.go). 둘째 언어가
// 생기면 여기 case 가 하나 는다.
func (e *editor) saveHookFor(path string) (*saveHook, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		hook := e.goimports(filepath.Dir(path))
		if hook == nil {
			return nil, errHookNotInstalled
		}

		return hook, nil
	default:
		return nil, nil
	}
}

// goimports 는 그 폴더에서 쓸 goimports 다. 찾지 못하면 nil 이다.
//
// **찾는 순서는 「그 저장소가 tool 로 선언한 것 → PATH」다.** go.mod 에 `tool` 로 적어 둔
// 저장소는 판을 고정해 둔 것이라 그 뜻이 PATH 에 깔린 것보다 앞선다(go 1.24 의 tool 지시자).
//
// 대신 `go tool` 은 저장마다 130ms 이고 PATH 의 바이너리는 그보다 짧다. 적어 두지 않은
// 저장소에 그 값을 물리지 않으려고 **폴더마다 한 번만 묻고 담아 둔다.**
//
// 담아 둔 것은 편집기가 도는 동안 그대로다. 도중에 go.mod 에 tool 을 적었으면 다시 띄워야
// 보인다 — gopls 가 한 번 실패하면 다시 걸지 않는 것과 같은 태도다(gopls.go).
func (e *editor) goimports(dir string) *saveHook {
	if hook, ok := e.saveHooks[dir]; ok {
		return hook
	}

	hook := findGoimports(dir)

	if e.saveHooks == nil {
		e.saveHooks = map[string]*saveHook{}
	}
	e.saveHooks[dir] = hook

	return hook
}

// findGoimports 는 그 자리에서 쓸 goimports 를 찾는다.
//
// **PATH 만 보지 않는다.** `go install` 은 `$GOBIN` 또는 `$GOPATH/bin` 에 넣고 그 자리가
// PATH 에 없는 기계가 흔하다 — 이 기능을 만든 기계가 그랬다. gopls 를 찾는 자리가 같은 일을
// 먼저 겪었다(lsp/client.go 의 findGopls, ADR-0051).
func findGoimports(dir string) *saveHook {
	if goToolDeclares(dir, "goimports") {
		return &saveHook{name: "goimports", dir: dir, path: "go", args: []string{"tool", "goimports"}}
	}

	if path, err := exec.LookPath("goimports"); err == nil {
		return &saveHook{name: "goimports", dir: dir, path: path}
	}

	for _, bin := range goInstallDirs() {
		path := filepath.Join(bin, "goimports")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return &saveHook{name: "goimports", dir: dir, path: path}
		}
	}

	return nil
}

// goInstallDirs 는 `go install` 이 실행 파일을 넣는 자리다.
//
// **환경 변수를 우리가 풀지 않고 go 에게 묻는다.** `GOBIN` 이 비면 `GOPATH/bin` 이고 `GOPATH`
// 가 비면 `~/go/bin` 이라는 규칙은 go 의 것이라, 옮겨 적으면 판이 바뀔 때 어긋난다 —
// tool 을 `go tool` 에게 물어 찾는 것과 같은 태도다(ADR-0065).
//
// go 가 없으면 빈 목록이다. 그때는 애초에 `go install` 로 깔 수 있는 것이 없다.
func goInstallDirs() []string {
	out, err := exec.Command("go", "env", "GOBIN", "GOPATH").Output()
	if err != nil {
		return nil
	}

	// **줄을 먼저 가른다.** `GOBIN` 이 비어 있으면 첫 줄이 빈 줄로 오는데, 통째로 다듬고
	// 나누면 그 줄이 사라져서 GOPATH 를 GOBIN 으로 읽게 된다 — 이 기계가 그랬다.
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) < 2 {
		return nil
	}

	if bin := strings.TrimSpace(lines[0]); bin != "" {
		return []string{bin}
	}

	dirs := []string{}
	for _, root := range filepath.SplitList(strings.TrimSpace(lines[1])) {
		if root != "" {
			dirs = append(dirs, filepath.Join(root, "bin"))
		}
	}

	return dirs
}

// goToolDeclares 는 그 자리의 모듈이 그 tool 을 선언했는지다.
//
// `go tool` 이 쓸 수 있는 이름을 늘어놓는다. 모듈이 적어 둔 것은 `golang.org/x/tools/cmd/goimports`
// 처럼 경로째 나오고 `go tool goimports` 는 마지막 칸으로 찾으므로 여기서도 그렇게 본다.
//
// 물어보는 것으로 찾는 것은 go.mod 를 우리가 읽지 않으려는 것이다 — tool 지시자의 문법과
// 이름을 줄이는 규칙은 go 가 아는 것이고, 그것을 여기 옮겨 적으면 판이 바뀔 때 어긋난다.
func goToolDeclares(dir, name string) bool {
	cmd := exec.Command("go", "tool")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == name || strings.HasSuffix(line, "/"+name) {
			return true
		}
	}

	return false
}

// run 은 in 을 명령에 흘려 넣고 나온 글을 받는다.
func (hook saveHook) run(in []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), saveHookTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, hook.path, hook.args...)
	cmd.Dir = hook.dir
	cmd.Stdin = bytes.NewReader(in)

	var out, fail bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &fail

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.Errorf("%s 안에 끝나지 않았습니다", saveHookTimeout)
		}

		return nil, errors.New(firstLine(fail.String(), err))
	}

	return out.Bytes(), nil
}

// firstLine 은 명령이 낸 오류에서 알림 한 줄에 적을 것이다.
//
// 포매터가 stderr 에 내는 것은 `<standard input>:5:1: expected declaration` 처럼 첫 줄이
// 곧 까닭이다. 아래 줄은 같은 오류가 이어지는 자리라 한 줄만 있으면 어디를 볼지 알 수 있다.
// 아무 말도 없이 죽었으면(명령이 사라진 경우) exec 이 준 것을 그대로 쓴다.
func firstLine(text string, fallback error) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if line == "" {
		return fallback.Error()
	}

	return line
}

// applySaveHook 은 저장하기 직전에 buffer 를 포매터가 낸 글로 갈아끼운다.
// 한 줄로 무엇을 했는지 준다 — 부르는 쪽이 저장 문구에 붙인다(ADR-0052).
//
// **실패하면 buffer 를 건드리지 않고 까닭만 준다. 저장은 그대로 간다.** 편집 중의 Go 는 거의
// 언제나 문법이 깨져 있어서, 여기서 저장을 막으면 고치다 만 파일을 둘 곳이 없어진다.
//
// 읽기 전용 파일은 손대지 않는다. 쓰기가 어차피 실패하는데 buffer 만 바뀌면 되돌릴 길도
// 없다 — applyFileFormat 과 같은 자리다(editorconfig.go).
func (buf *Buffer) applySaveHook(hook *saveHook, width int) string {
	if hook == nil || buf.readOnly {
		return ""
	}

	// 마지막 줄바꿈을 붙여 넘긴다. buffer 는 그것을 내용이 아니라 사실로 들고 있어서
	// (finalLineEnding) 붙이지 않으면 포매터가 마지막 줄만 다르게 본다.
	in := append(bytes.Join(buf.lines, []byte{'\n'}), '\n')

	out, err := hook.run(in)
	if err != nil {
		return hook.name + ": " + err.Error()
	}

	next := splitFormatted(out)
	if equalLines(buf.lines, next) {
		return ""
	}

	changed := countChangedLines(buf.lines, next)
	grew := len(next) - len(buf.lines)

	buf.replaceAll(next, width)

	note := fmt.Sprintf("%s: %d 줄 맞춤", hook.name, changed)
	switch {
	case grew > 0:
		note += fmt.Sprintf(", %d 줄 늘어남", grew)
	case grew < 0:
		note += fmt.Sprintf(", %d 줄 줄어듦", -grew)
	}

	return note
}

// splitFormatted 는 포매터가 낸 글을 buffer 의 줄로 나눈다.
//
// 마지막 줄바꿈은 떼고 나눈다. buffer 는 그것을 내용이 아니라 사실로 들고 있고, 그 사실은
// 여기서 바꾸지 않는다 — 마지막 줄바꿈을 넣고 빼는 것은 `.editorconfig` 가 정할 일이다
// (editorconfig.go 의 insert_final_newline, ADR-0052).
//
// 아무것도 안 나왔으면 빈 줄 하나다. buffer 에는 줄이 적어도 하나 있어야 한다.
func splitFormatted(out []byte) [][]byte {
	out = bytes.TrimSuffix(out, []byte{'\n'})
	if len(out) == 0 {
		return [][]byte{{}}
	}

	return bytes.Split(out, []byte{'\n'})
}

// equalLines 는 두 줄 묶음이 같은지다.
func equalLines(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}

	return true
}

// countChangedLines 는 자리로 맞대어 다른 줄이 몇인지다.
//
// 참 diff 가 아니다. 줄이 하나 늘면 그 아래가 전부 다른 줄로 세어진다 — 여기서 세는 것은
// 「이만큼 달라졌다」는 눈짐작이고, 정확한 자리는 화면이 이미 보여주고 있다.
func countChangedLines(old, next [][]byte) int {
	changed := 0
	for i := range max(len(old), len(next)) {
		switch {
		case i >= len(old) || i >= len(next):
			changed++
		case !bytes.Equal(old[i], next[i]):
			changed++
		}
	}

	return changed
}

// goimportsJobName 은 설치 작업의 이름이다. `:jobs` 에 이 이름으로 뜬다.
const goimportsJobName = "goimports 설치"

// askGoimports 는 설치를 물어야 하는지다. 한 번 거절했거나 지금 깔고 있는 중이면 묻지 않는다.
//
// 「이 파일에 돌릴 것이 있는가」는 보지 않는다 — 그것은 errHookNotInstalled 가 이미 말해 주고,
// 여기서 다시 확장자를 보면 표가 두 자리에 있게 된다.
func (e *editor) askGoimports() bool {
	return !e.goimportsDeclined && !e.jobRunning(goimportsJobName, nil)
}

// installGoimports 는 go install 로 goimports 를 설치하는 백그라운드 작업을 시작한다.
// installGopls 와 같은 자리이고 깔 것만 다르다(gopls.go).
func (e *editor) installGoimports() tea.Cmd {
	if _, err := exec.LookPath("go"); err != nil {
		e.notifyError(errors.New("go 명령어를 찾을 수 없습니다"))

		return nil
	}

	return e.startJob(goimportsJobName, nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress)

		go func() {
			defer close(ch)

			cmd := exec.CommandContext(ctx, "go", "install", "golang.org/x/tools/cmd/goimports@latest")
			out, err := cmd.CombinedOutput()
			if err != nil {
				errMsg := strings.TrimSpace(string(out))
				if errMsg == "" {
					errMsg = err.Error()
				}

				ch <- jobProgress{err: errors.New(errMsg)}

				return
			}

			ch <- jobProgress{
				summary: "goimports 설치 완료",
				apply: func(e *editor) {
					// 「없다」고 담아 둔 것을 버린다. 다음 저장이 다시 찾아서 방금 깐 것을 만난다.
					e.saveHooks = nil
					e.notify("goimports 설치가 끝났습니다. 다음 저장부터 맞춥니다")
				},
			}
		}()

		return ch
	})
}
