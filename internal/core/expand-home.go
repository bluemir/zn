package core

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// 경로 맨 앞의 `~` 를 홈 디렉터리로 푸는 자리다. 경로를 받는 명령 셋(`:w` `:e` `:tabnew`) 이
// 인자를 쓰기 전에 이것을 지난다(ADR-0088).
//
// 셸이 이미 풀어 주는 것은 CLI 인자뿐이다. 편집기 안에서 친 `:tabnew ~/.zshrc` 는 셸을
// 거치지 않으므로 아무도 풀어 주지 않고, 그대로 두면 `~` 라는 이름의 디렉터리를 찾다가
// 「파일이 없습니다」로 끝난다.
//
// 반대 방향은 이미 있었다 — shortenPath 가 홈 아래의 파일을 `~/...` 로 줄여 보여준다
// (view-locations.go). 보여주는 글자를 그대로 다시 칠 수 없는 것이 이 구멍이었다.

// expandHome 은 경로 맨 앞의 `~` 를 홈 디렉터리로 바꾼다. `~` 로 시작하지 않으면 그대로 준다.
//
//	~            내 홈
//	~/.zshrc     내 홈 아래
//	~bluemir     그 사용자의 홈
//	~bluemir/src 그 사용자의 홈 아래
//
// 맨 앞이 아닌 `~` 는 건드리지 않는다. `docs/~backup` 은 그런 이름의 파일이다.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	// `~` 다음부터 첫 `/` 까지가 사용자 이름이다. 이름이 비어 있으면 나를 뜻한다.
	//
	// 구분자는 `/` 만 본다. 여기 오는 것은 사람이 명령줄에 친 것이고, `~` 를 치는 사람은
	// `/` 를 친다.
	name, rest := path[1:], ""
	if slash := strings.IndexByte(name, '/'); slash >= 0 {
		name, rest = name[:slash], name[slash+1:]
	}

	home, err := homeDir(name)
	if err != nil {
		return "", err
	}

	// `~` 하나였으면 홈 그 자체다. Join 으로 빈 조각을 붙이면 뒤에 구분자가 남는다.
	if rest == "" {
		return home, nil
	}

	return filepath.Join(home, rest), nil
}

// homeDir 은 그 사용자의 홈을 준다. 이름이 비어 있으면 지금 사용자다.
func homeDir(name string) (string, error) {
	if name == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.Wrap(err, "홈 디렉터리를 찾을 수 없습니다")
		}

		return home, nil
	}

	// 없는 사용자면 오류로 낸다. `~` 를 홈으로 푸는 자리에서 `~nobody/x` 를 그런 이름의
	// 디렉터리로 되돌리면, 「파일이 없습니다」가 사용자를 못 찾은 것인지 파일이 없는 것인지
	// 가리지 못한다(ADR-0088).
	who, err := user.Lookup(name)
	if err != nil {
		return "", errors.Errorf("사용자를 찾을 수 없습니다: ~%s", name)
	}

	return who.HomeDir, nil
}
