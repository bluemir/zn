package core

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluemir/zn/internal/textarea"
	"github.com/cockroachdb/errors"
)

// 저장할 때 파일을 바깥 포매터에 통과시키는 자리다(ADR-0065).
//
// 하는 일은 셋이다 — 어떤 파일에 무엇을 돌릴지 고르기(formatters 표와 saveHookFor),
// 그것을 찾기(findGoimports·findRuff), buffer 를 흘려 넣고 받아 갈아끼우기(applySaveHook).
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

// formatter 는 저장할 때 통과시킬 포매터 하나다. 표의 한 줄이 포매터 하나다.
//
// **switch 가 아니라 표다.** ADR-0065 는 확장자 하나뿐일 때 switch 로 두고 「둘째 언어가
// 생기면 case 가 하나 는다」고 적었는데, 그 둘째가 오면서 **설치를 묻는 창이 이름과 명령을
// 값으로 읽어야** 하게 되었다. switch 로 두면 그 창이 볼 표를 따로 만들어야 하고, 그것이
// 바로 askGoimports 가 경계해 둔 「표가 두 자리에 있게 된다」다(ADR-0107).
type formatter struct {
	// name 은 알림과 설치 창에 적는 이름이다.
	name string

	// exts 는 이 포매터를 통과시킬 확장자다. 소문자로 적는다.
	exts []string

	// find 는 그 폴더에서 쓸 명령을 찾는다. 깔려 있지 않으면 nil 이다.
	find func(dir string) *saveHook

	// install 은 없을 때 깔 명령이다. 첫 칸이 그것을 낼 도구다.
	install []string
}

// formatters 는 저장할 때 통과시키는 포매터 전부다. 새 언어는 여기 한 줄이 는다.
//
// `ruff format -` 은 stdin 을 받고 black 호환 포맷을 낸다. import 는 정리하지 않는다 —
// goimports 가 하는 그 일은 `ruff check --select I --fix -` 라 명령이 둘이 되고, 저장 하나가
// 명령 둘을 지나는 것은 이 자리가 아직 하지 않는 일이다(docs/tasks.md).
var formatters = []formatter{
	{
		name:    "goimports",
		exts:    []string{".go"},
		find:    findGoimports,
		install: []string{"go", "install", "golang.org/x/tools/cmd/goimports@latest"},
	},
	{
		name:    "ruff",
		exts:    []string{".py"},
		find:    findRuff,
		install: []string{"uv", "tool", "install", "ruff"},
	},
}

// formatterFor 는 그 파일을 저장할 때 통과시킬 포매터다. 표에 없으면 nil 이다.
func formatterFor(path string) *formatter {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return nil
	}

	for i := range formatters {
		if slices.Contains(formatters[i].exts, ext) {
			return &formatters[i]
		}
	}

	return nil
}

// InstallHint 는 설치 명령을 사람이 셸에 칠 수 있는 한 줄이다.
func (f *formatter) InstallHint() string {
	return strings.Join(f.install, " ")
}

// saveHookFor 는 그 파일을 저장할 때 통과시킬 명령이다.
//
// 표에 없는 확장자는 (nil, nil) 이고, 표에 있는데 깔려 있지 않으면 errHookNotInstalled 다.
func (e *editor) saveHookFor(path string) (*saveHook, error) {
	spec := formatterFor(path)
	if spec == nil {
		return nil, nil
	}

	hook := e.lookupHook(spec, filepath.Dir(path))
	if hook == nil {
		return nil, errHookNotInstalled
	}

	return hook, nil
}

// lookupHook 은 그 폴더에서 쓸 명령이다. 찾지 못하면 nil 이다.
//
// **찾는 일은 폴더마다 한 번만 하고 담아 둔다.** `go tool` 은 저장마다 130ms 이고 PATH 의
// 바이너리는 그보다 짧다. 적어 두지 않은 저장소에 그 값을 물리지 않으려는 것이다.
//
// **키가 폴더만이 아니다.** 한 폴더에 `.go` 와 `.py` 가 같이 있는 저장소가 있어서, 폴더만
// 키로 두면 먼저 저장한 쪽의 답이 다른 쪽을 덮는다 — goimports 를 찾아 둔 자리에서 python
// 파일을 저장하면 python 이 goimports 를 통과하게 된다.
//
// 담아 둔 것은 편집기가 도는 동안 그대로다. 도중에 go.mod 에 tool 을 적었으면 다시 띄워야
// 보인다 — 언어 서버가 한 번 실패하면 다시 걸지 않는 것과 같은 태도다(language-server.go).
func (e *editor) lookupHook(spec *formatter, dir string) *saveHook {
	key := saveHookKey{tool: spec.name, dir: dir}

	if hook, ok := e.saveHooks[key]; ok {
		return hook
	}

	hook := spec.find(dir)

	if e.saveHooks == nil {
		e.saveHooks = map[saveHookKey]*saveHook{}
	}
	e.saveHooks[key] = hook

	return hook
}

// saveHookKey 는 찾아 둔 답의 자리다. 「어느 포매터를 어느 폴더에서」가 한 짝이다.
type saveHookKey struct {
	tool string
	dir  string
}

// findGoimports 는 그 자리에서 쓸 goimports 를 찾는다.
//
// **PATH 만 보지 않는다.** `go install` 은 `$GOBIN` 또는 `$GOPATH/bin` 에 넣고 그 자리가
// PATH 에 없는 기계가 흔하다 — 이 기능을 만든 기계가 그랬다. gopls 를 찾는 자리가 같은 일을
// 먼저 겪었다(lsp/server.go 의 findGopls, ADR-0051).
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

// findRuff 는 그 자리에서 쓸 ruff 를 찾는다.
//
// **저장소의 환경을 먼저 본다.** python 은 프로젝트마다 자기 환경을 두는 언어라 그 안에
// 깔린 것이 그 저장소의 뜻이다 — goimports 가 그 저장소의 `tool` 선언을 PATH 보다 앞에
// 두는 것과 같은 자리다. 언어 서버를 찾는 순서와도 같다(lsp/server.go 의 findPyright).
//
// **PATH 만 보지 않는다.** `uv tool install` 은 `~/.local/bin` 에 넣고 그 자리가 PATH 에
// 없는 기계가 흔하다 — goimports 가 GOBIN 에서 같은 일을 먼저 겪었다(ADR-0065).
//
// 인자는 `format -` 이다. `-` 가 stdin 을 받으라는 뜻이고, 설정은 명령을 돌리는 자리
// (hook.dir, 곧 그 파일의 폴더) 에서 위로 올라가며 찾는다 — 재보니 그 자리의
// `pyproject.toml` 의 `line-length`·`indent-width` 가 stdin 에도 그대로 들었다.
func findRuff(dir string) *saveHook {
	hook := func(path string) *saveHook {
		return &saveHook{name: "ruff", dir: dir, path: path, args: []string{"format", "-"}}
	}

	for _, bin := range pythonBinDirs(dir) {
		path := filepath.Join(bin, "ruff")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return hook(path)
		}
	}

	if path, err := exec.LookPath("ruff"); err == nil {
		return hook(path)
	}

	if bin := uvToolBin(); bin != "" {
		path := filepath.Join(bin, "ruff")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return hook(path)
		}
	}

	return nil
}

// pythonBinDirs 는 그 자리의 python 환경이 실행 파일을 두는 자리들이다.
//
// `VIRTUAL_ENV` 가 앞이다. 사람이 이미 그 환경에 들어와 편집기를 띄웠다는 뜻이라 그 뜻이
// 앞선다. 뒤의 둘은 이름을 짓는 관례다.
//
// **위로 올라가며 찾는다.** 저장소 뿌리에 `.venv` 를 두고 그 아래 폴더의 파일을 저장하는
// 것이 흔한 모습이라, 그 파일의 폴더만 보면 거의 언제나 못 찾는다.
func pythonBinDirs(dir string) []string {
	dirs := []string{}

	if env := os.Getenv("VIRTUAL_ENV"); env != "" {
		dirs = append(dirs, filepath.Join(env, "bin"))
	}

	for at := dir; ; {
		dirs = append(dirs, filepath.Join(at, ".venv", "bin"), filepath.Join(at, "venv", "bin"))

		parent := filepath.Dir(at)
		if parent == at {
			break
		}

		at = parent
	}

	return dirs
}

// uvToolBin 은 `uv tool install` 이 실행 파일을 넣는 자리다. 물어볼 수 없으면 빈 글이다.
//
// **자리를 우리가 풀지 않고 uv 에게 묻는다.** goimports 를 찾을 때 `go env` 에게 묻는 것과
// 같은 태도다. 물음을 모르는 판이면 uv 가 문서에 적어 둔 기본값을 본다 — 우리가 깔아 주는
// 길이 있어서 여기서 물러나면 「깔았는데 못 찾는다」가 된다.
func uvToolBin() string {
	out, err := exec.Command("uv", "tool", "dir", "--bin").Output()
	if err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return dir
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "bin")
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

// formatterJobName 은 그 포매터의 설치 작업 이름이다. `:jobs` 에 이 이름으로 뜬다.
func formatterJobName(spec *formatter) string {
	return spec.name + " 설치"
}

// askFormatter 는 그 포매터의 설치를 물어야 하는지다. 한 번 거절했거나 지금 깔고 있는
// 중이면 묻지 않는다.
//
// 「이 파일에 돌릴 것이 있는가」는 보지 않는다 — 그것은 errHookNotInstalled 가 이미 말해 주고,
// 여기서 다시 확장자를 보면 표가 두 자리에 있게 된다.
//
// **거절은 포매터마다 따로 적는다.** goimports 를 깔지 않기로 한 것이 ruff 까지 묻지 않게
// 하면, python 파일을 저장하는 사람이 자기가 거절한 적 없는 물음을 잃는다(ADR-0107).
func (e *editor) askFormatter(spec *formatter) bool {
	return !e.formattersDeclined[spec.name] && !e.jobRunning(formatterJobName(spec), nil)
}

// declineFormatter 는 그 포매터를 깔지 않기로 한 것을 적어 둔다.
func (e *editor) declineFormatter(spec *formatter) {
	if e.formattersDeclined == nil {
		e.formattersDeclined = map[string]bool{}
	}

	e.formattersDeclined[spec.name] = true
}

// installFormatter 는 그 포매터를 깔는 백그라운드 작업을 시작한다.
// installServer 와 같은 자리이고 깔 것만 다르다(language-server.go).
//
// 첫 칸의 도구가 없으면 깔지 않고 알린다. `uv` 가 없는 기계에서 ruff 를 깔 길이 없고,
// 그것은 우리가 대신 정해 줄 일이 아니다.
func (e *editor) installFormatter(spec *formatter) tea.Cmd {
	install := spec.install
	if len(install) == 0 {
		return nil
	}

	tool := install[0]
	if _, err := exec.LookPath(tool); err != nil {
		e.notifyError(errors.Errorf("%s 명령어를 찾을 수 없습니다", tool))

		return nil
	}

	return e.startJob(formatterJobName(spec), nil, func(ctx context.Context) <-chan jobProgress {
		ch := make(chan jobProgress)

		go func() {
			defer close(ch)

			cmd := exec.CommandContext(ctx, install[0], install[1:]...)
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
				summary: spec.name + " 설치 완료",
				apply: func(e *editor) {
					// 「없다」고 담아 둔 것을 버린다. 다음 저장이 다시 찾아서 방금 깐 것을 만난다.
					e.saveHooks = nil
					e.notify(spec.name + " 설치가 끝났습니다. 다음 저장부터 맞춥니다")
				},
			}
		}()

		return ch
	})
}

// saveFormat 은 이 포매터를 창이 쓸 수 있는 꼴로 싼다.
//
// 창은 「이름이 무엇이고 byte 를 넣으면 byte 가 나온다」 둘만 안다. 명령을 찾고 깔고 돌리는
// 것은 여기 일이라 그 앎이 그쪽으로 넘어가지 않는다(textarea.SaveFormat, ADR-0128).
func (hook *saveHook) saveFormat() *textarea.SaveFormat {
	if hook == nil {
		return nil
	}

	return &textarea.SaveFormat{Name: hook.name, Run: hook.run}
}
