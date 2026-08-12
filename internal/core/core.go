package core

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

func Run(ctx context.Context, files []string) error {
	buffers := make([]Buffer, 0, len(files))
	for _, file := range files {
		buf, err := OpenBuffer(file)
		if err != nil {
			return err
		}
		buffers = append(buffers, buf)
	}
	if len(buffers) == 0 {
		buffers = append(buffers, newEmptyBuffer(""))
	}

	first, _ := normalMode(editor{buffers: buffers})

	if _, err := tea.NewProgram(
		first,
		tea.WithContext(ctx),
	).Run(); err != nil {
		return err
	}

	return nil
}
