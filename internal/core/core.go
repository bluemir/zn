package core

import (
	"context"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

func Run(ctx context.Context, files []string) error {
	buffers, err := openBuffers(files)
	if err != nil {
		return err
	}

	// git 표시는 여기서 읽지 않는다. 첫 화면이 뜬 뒤 갱신 작업이 채운다(ADR-0030).
	e := &editor{ctx: ctx, buffers: buffers}

	// 박스 그리기 문자는 East Asian Width 가 Ambiguous 라 터미널마다 폭이 다르다.
	// 한 칸으로 확인된 터미널에서만 쓰고, 두 칸이거나 재지 못했으면 ASCII 로 내린다(ADR-0028).
	//
	// tea.NewProgram 보다 먼저다. bubbletea 가 stdin 을 읽기 시작하면 답을 그쪽이 가져간다.
	e.asciiBox = probeAmbiguousWidth() != 1

	// filetree 는 기본으로 열어둔다. `:tree` 로 닫는다.
	//
	// cwd 를 못 읽으면 트리 없이 연다. 이때 편집기를 아예 못 열 이유는 없다 —
	// 지운 디렉터리에서 실행하면 나는 오류이고, 파일은 인자로 이미 받았다.
	// 화면이 좁으면 sidebarVisible 이 알아서 감추므로 여기서 크기는 보지 않는다.
	if root, err := os.Getwd(); err == nil {
		e.sidebar = openSidebar(root)

		// CLI 인자로 연 파일 자리를 펼쳐 둔다.
		//
		// revealInSidebar 가 아니라 reveal 을 부른다. 아직 WindowSizeMsg 가 오지 않아
		// 화면 크기를 모르므로 여기서 스크롤할 수는 없다. 고른 자리는 sidebar 로
		// 포커스가 올 때 scrollTo 가 화면 안으로 데려온다.
		e.sidebar.reveal(e.buffer().path)
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

// openBuffers 는 CLI 인자로 받은 파일들을 tab 순서대로 연다.
// 인자가 없으면 이름 없는 빈 buffer 하나로 시작한다.
//
// 같은 파일을 두 번 넘겨도 tab 은 하나다. 같은 파일에 Buffer 가 둘이면 한쪽에서 저장하는
// 순간 다른 쪽 편집이 사라진다 — openTab 이 이미 열린 tab 으로 옮겨 가는 것과 같은 이유다.
func openBuffers(files []string) ([]Buffer, error) {
	buffers := make([]Buffer, 0, len(files))
	opened := map[string]bool{}

	for _, file := range files {
		// 표기가 달라도(`a.txt` 와 `./a.txt`) 같은 파일이면 한 번만 연다. tabOf 와 같은 기준이다.
		// 정규화하지 못하면 적힌 그대로를 기준으로 삼는다 — 글자가 같은 것까지는 걸러진다.
		key, err := filepath.Abs(file)
		if err != nil {
			key = file
		}
		if opened[key] {
			continue
		}
		opened[key] = true

		buf, err := OpenBuffer(file)
		if err != nil {
			return nil, err
		}
		buffers = append(buffers, buf)
	}

	if len(buffers) == 0 {
		buffers = append(buffers, newEmptyBuffer(""))
	}

	return buffers, nil
}
