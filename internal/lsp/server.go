package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
)

// Server 는 언어 서버 하나를 어떻게 띄우는지다. 표의 한 줄이 서버 하나이므로 줄을 통째로
// 건네는 것이 서버를 건네는 것이다 — syntax 의 언어 표와 같은 손이다(syntax/detect.go 의
// languageRules, ADR-0080).
//
// **이 표는 syntax 의 언어 표와 따로 산다.** 저쪽은 lexer 를 고르는 표이고 이쪽은 「어느
// 언어 서버가 보는가」다. 합치면 서버 없는 언어 아홉 줄에 빈 칸이 생기고, 무엇보다 lexer 를
// 더할 때마다 서버가 조용히 붙거나 떨어진다(ADR-0107).
type Server struct {
	// Name 은 사람에게 보이는 이름이다. 알림 문구와 설치 작업 이름이 이것으로 적힌다.
	Name string

	// exts 는 이 서버가 볼 확장자다. 소문자로 적는다 — 보는 쪽에서 내려 견준다.
	exts []string

	// languageID 는 didOpen 에 적는 규격의 언어 이름이다. 확장자와 따로 두는 것은 규격이
	// 정한 이름이 우리 확장자와 갈리기 때문이다 — `.py` 하나에 `python` 이다.
	languageID string

	// args 는 stdio 로 붙는 실행 인자다. 서버마다 다른 글자를 쓴다.
	args []string

	// initOptions 는 악수에 싣는 서버 설정이다. 비어 있으면 빈 객체로 간다.
	//
	// **능력(capabilities) 이 아니다.** 그쪽은 비워 두어야 서버가 우리에게 되묻지 않는다
	// (initialize 의 주석, ADR-0051).
	initOptions map[string]any

	// find 는 실행 파일을 찾는다. root 는 편집기를 연 자리다 — python 은 저장소마다 자기
	// 환경을 두어서 그 안을 먼저 본다.
	find func(root string) (string, error)

	// install 은 없을 때 깔 명령이다. 첫 칸이 그것을 낼 도구라, 그 도구가 없으면 묻지
	// 않고 물러난다(core 의 installServer).
	install []string
}

// servers 는 붙이는 언어 서버 전부다. 새 서버는 여기 한 줄이 는다.
//
// gopls 의 `semanticTokens` 는 기본값이 꺼짐이라 켜 주어야 악수 응답에 이름표가 온다
// (ADR-0103). pyright 는 그 칸이 없다 — 재보니 semantic token 을 아예 알리지 않아서
// 켤 것이 없고, python 의 색은 우리 lexer 가 그대로 맡는다(ADR-0107).
var servers = []Server{
	{
		Name:        "gopls",
		exts:        []string{".go"},
		languageID:  "go",
		args:        []string{"-mode=stdio"},
		initOptions: map[string]any{"semanticTokens": true},
		find:        findGopls,
		install:     []string{"go", "install", "golang.org/x/tools/gopls@latest"},
	},
	{
		Name:       "pyright",
		exts:       []string{".py"},
		languageID: "python",
		args:       []string{"--stdio"},
		find:       findPyright,
		install:    []string{"uv", "tool", "install", "pyright[nodejs]"},
	},
}

// ServerFor 는 그 파일을 볼 서버다. 볼 서버가 없으면 nil 이다.
//
// **이름만 본다.** 파일이 없어도(새로 만드는 buffer) 같은 답이 나온다 — syntax 의
// LanguageFor 와 같은 태도다.
func ServerFor(path string) *Server {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return nil
	}

	for i := range servers {
		if slices.Contains(servers[i].exts, ext) {
			return &servers[i]
		}
	}

	return nil
}

// Servers 는 표의 모든 줄이다.
//
// 부르는 쪽이 서버를 되짚을 때 쓴다 — 끝난 설치 작업의 이름으로 「어느 서버였나」를 찾는
// 자리가 그것이다(core 의 serverForJobName). 표가 여전히 한 곳이라 되짚는 길이 늘어도
// 서버를 더할 자리는 하나다.
func Servers() []*Server {
	all := make([]*Server, 0, len(servers))
	for i := range servers {
		all = append(all, &servers[i])
	}

	return all
}

// serverNamed 는 이름으로 찾은 표의 한 줄이다. 없으면 nil 이다.
//
// 시험이 「그 서버를 실제로 띄워」 묻는 자리가 쓴다(gopls_test.go). 이름으로 찾는 것은
// 확장자로 찾으면 시험이 「`.go` 는 gopls 가 본다」까지 같이 시험하게 되어서다.
func serverNamed(name string) *Server {
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i]
		}
	}

	return nil
}

// Install 은 이 서버를 깔 명령이다. 첫 칸이 그것을 낼 도구다.
func (server *Server) Install() []string {
	return server.install
}

// InstallHint 는 설치 명령을 사람이 셸에 칠 수 있는 한 줄로 적은 것이다.
//
// 대괄호가 든 칸에 따옴표를 붙인다. `pyright[nodejs]` 를 그대로 적으면 zsh 가 그것을
// 파일 이름 짝맞추기로 읽어서, 화면에서 옮겨 친 사람이 `no matches found` 를 만난다.
// 우리가 돌릴 때는 셸을 거치지 않으므로 인자 그대로 간다(core 의 installServer).
func (server *Server) InstallHint() string {
	quoted := make([]string, 0, len(server.install))
	for _, arg := range server.install {
		if strings.ContainsAny(arg, "[]*?") {
			arg = "'" + arg + "'"
		}

		quoted = append(quoted, arg)
	}

	return strings.Join(quoted, " ")
}

// findGopls 는 gopls 실행 파일을 찾는다.
//
// **PATH 만 보지 않는다.** `go install` 은 `$GOBIN` 또는 `$GOPATH/bin` 에 넣고 그 자리가
// PATH 에 없는 기계가 흔하다 — 이 기능을 만든 기계가 그랬다(ADR-0051). 그래서 go 의 관례
// 자리까지 본다.
//
// root 를 보지 않는다. Go 는 도구를 저장소마다 따로 깔지 않는 언어라 볼 자리가 없다 —
// 표가 root 를 건네는 것은 pyright 때문이다.
func findGopls(root string) (string, error) {
	if path, err := exec.LookPath("gopls"); err == nil {
		return path, nil
	}

	for _, dir := range goBinDirs() {
		path := filepath.Join(dir, "gopls")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", errors.New("gopls 를 찾지 못했다. `go install golang.org/x/tools/gopls@latest` 로 넣는다")
}

// goBinDirs 는 `go install` 이 실행 파일을 넣는 자리들이다. 앞에 있는 것이 먼저다.
func goBinDirs() []string {
	if bin := os.Getenv("GOBIN"); bin != "" {
		return []string{bin}
	}

	dirs := []string{}
	for _, root := range filepath.SplitList(os.Getenv("GOPATH")) {
		if root != "" {
			dirs = append(dirs, filepath.Join(root, "bin"))
		}
	}

	// GOPATH 가 비어 있으면 go 의 기본값은 `~/go` 다.
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}

	return dirs
}

// pyrightBinary 는 언어 서버 실행 파일 이름이다. `pyright` 는 명령줄 검사기라 stdio 로
// 붙지 않는다.
const pyrightBinary = "pyright-langserver"

// findPyright 는 pyright 실행 파일을 찾는다.
//
// **저장소의 환경을 먼저 본다.** python 은 프로젝트마다 자기 환경을 두는 언어라 그 안에
// 깔린 것이 그 저장소의 뜻이다. goimports 가 그 저장소의 `tool` 선언을 PATH 보다 앞에
// 두는 것과 같은 자리다(ADR-0065).
//
// **PATH 만 보지 않는다.** `uv tool install` 은 `~/.local/bin` 에 넣고 그 자리가 PATH 에
// 없는 기계가 흔하다 — gopls 의 GOBIN 이 같은 일을 먼저 겪었다(findGopls, ADR-0051).
func findPyright(root string) (string, error) {
	for _, dir := range pythonBinDirs(root) {
		path := filepath.Join(dir, pyrightBinary)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	if path, err := exec.LookPath(pyrightBinary); err == nil {
		return path, nil
	}

	if dir := uvToolBin(); dir != "" {
		path := filepath.Join(dir, pyrightBinary)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", errors.New("pyright 를 찾지 못했다. `uv tool install 'pyright[nodejs]'` 로 넣는다")
}

// pythonBinDirs 는 그 저장소의 python 환경이 실행 파일을 두는 자리들이다.
//
// `VIRTUAL_ENV` 가 앞이다. 사람이 이미 그 환경에 들어와 편집기를 띄웠다는 뜻이라, 저장소
// 안의 다른 환경보다 그 뜻이 앞선다. 뒤의 둘은 이름을 짓는 관례다.
//
// **위로 올라가지 않는다.** 받는 root 가 편집기를 연 자리, 곧 이미 맨 위다. 저장 포매터
// 쪽은 그 **파일의 폴더**를 받아서 올라가며 찾는다(core/save-hook.go 의 같은 이름).
func pythonBinDirs(root string) []string {
	dirs := []string{}

	if env := os.Getenv("VIRTUAL_ENV"); env != "" {
		dirs = append(dirs, filepath.Join(env, "bin"))
	}

	if root != "" {
		dirs = append(dirs, filepath.Join(root, ".venv", "bin"), filepath.Join(root, "venv", "bin"))
	}

	return dirs
}

// uvToolBin 은 `uv tool install` 이 실행 파일을 넣는 자리다. 물어볼 수 없으면 빈 글이다.
//
// **자리를 우리가 풀지 않고 uv 에게 묻는다.** 그 규칙은 uv 의 것이라 옮겨 적으면 판이
// 바뀔 때 어긋난다 — goimports 를 찾을 때 `go env` 에게 묻는 것과 같은 태도다(ADR-0065).
//
// 물음을 모르는 판이면 uv 가 문서에 적어 둔 기본값을 본다. 우리가 깔아 주는 길이 있어서
// (core 의 installServer) 여기서 물러나면 「깔았는데 못 찾는다」가 된다.
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
