package core

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenerFor(t *testing.T) {
	assert.Equal(t, "open", openerFor("darwin"))
	assert.Equal(t, "xdg-open", openerFor("linux"))
	assert.Equal(t, "xdg-open", openerFor("freebsd"))
}

func TestOpenResult(t *testing.T) {
	assert.NoError(t, openResult(nil, "").err, "끝났으면 성공이다")
	assert.NoError(t, openResult(exec.ErrWaitDelay, "").err, "브라우저가 pipe 를 붙든 것은 성공이다")

	failed := openResult(errors.New("exit status 1"), "first\nNo application knows how to open URL\n")
	assert.EqualError(t, failed.err, "No application knows how to open URL", "마지막 줄이 이유다")

	silent := openResult(errors.New("exit status 1"), "")
	assert.EqualError(t, silent.err, "exit status 1", "출력이 없으면 exec 의 것이다")
}
