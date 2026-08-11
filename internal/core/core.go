package core

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

func Run(ctx context.Context, workingDirectory string, files []string) error {

	// TODO 파일을 읽어서 Buffer 로 변환 한다.

	if _, err := tea.NewProgram(
		&viewEditor{},
		tea.WithContext(ctx),
	).Run(); err != nil {
		return err
	}

	return errors.Errorf("Not Implemented")
}

// Buffer 는 파일 하나에 대응 한다.
type Buffer struct {
	path string

	data  []byte
	lines [][]rune
}
