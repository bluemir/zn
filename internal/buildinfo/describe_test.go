package buildinfo

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// set 은 ldflags 로 들어오는 전역을 시험 동안만 바꾼다.
// 전역이라 되돌려 놓지 않으면 다음 시험이 이 값을 본다.
func set(t *testing.T, name, version, buildTime string) {
	t.Helper()

	old := [3]string{AppName, Version, BuildTime}
	t.Cleanup(func() {
		AppName, Version, BuildTime = old[0], old[1], old[2]
	})

	AppName, Version, BuildTime = name, version, buildTime
}

func TestDescribe(t *testing.T) {
	set(t, "zn", "v0.3.1", "2026-08-21T10:22:03Z")

	assert.Equal(t, "zn v0.3.1 (2026-08-21T10:22:03Z, "+runtime.Version()+")", Describe())
}

// ldflags 없이 지으면 세 값이 비어 있다. 빈 자리를 그대로 두면 무엇이 빠졌는지 알 수 없다.
func TestDescribeWithoutBuildInfo(t *testing.T) {
	set(t, "", "", "")

	assert.Equal(t, "zn 개발 빌드 (빌드시각 모름, "+runtime.Version()+")", Describe())
}
