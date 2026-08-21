package buildinfo

import (
	"fmt"
	"runtime"
)

// defaultAppName 은 ldflags 로 이름이 들어오지 않았을 때 쓸 이름이다.
// Makefile 이 module path 의 끝에서 뽑는 것과 같은 값이다.
const defaultAppName = "zn"

// Describe 는 이 binary 가 무엇으로 언제 지어진 것인지 한 줄로 적는다.
//
//	zn v0.3.1 (2026-08-21T10:22:03Z, go1.24.2)
//
// CLI 의 `--version` 과 편집기의 `:version` 이 같이 쓴다. 버그 신고에 그대로 붙일 수
// 있어야 해서 어느 쪽으로 물어도 같은 줄이 나온다.
//
// 이름·버전·빌드시각은 ldflags 로 넣으므로 `go run` 이나 맨 `go build` 로 띄우면 비어 있다.
// 빈 자리를 그대로 두면 ` (, go1.24.2)` 가 되어 값이 빠진 것인지 그런 값인지 알 수 없다.
// 그래서 없다는 것도 적는다.
func Describe() string {
	name := AppName
	if name == "" {
		name = defaultAppName
	}

	version := Version
	if version == "" {
		version = "개발 빌드"
	}

	buildTime := BuildTime
	if buildTime == "" {
		buildTime = "빌드시각 모름"
	}

	return fmt.Sprintf("%s %s (%s, %s)", name, version, buildTime, runtime.Version())
}
