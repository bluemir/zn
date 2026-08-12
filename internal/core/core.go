package core

import (
	"context"
	"os"

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

	e := editor{buffers: buffers}

	// filetree 는 기본으로 열어둔다. `:tree` 로 닫는다.
	//
	// cwd 를 못 읽으면 트리 없이 연다. 이때 편집기를 아예 못 열 이유는 없다 —
	// 지운 디렉터리에서 실행하면 나는 오류이고, 파일은 인자로 이미 받았다.
	// 화면이 좁으면 sidebarVisible 이 알아서 감추므로 여기서 크기는 보지 않는다.
	if root, err := os.Getwd(); err == nil {
		e.sidebar = openSidebar(root)
	}

	first, _ := normalMode(e)

	if _, err := tea.NewProgram(
		first,
		tea.WithContext(ctx),
	).Run(); err != nil {
		return err
	}

	return nil
}
