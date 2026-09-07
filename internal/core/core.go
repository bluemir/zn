package core

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/assets"
	"github.com/bluemir/zn/internal/terminal"

	"github.com/bluemir/zn/internal/textarea"
)

func Run(ctx context.Context, files []string) error {
	// 터미널이 East Asian Width 가 Ambiguous 인 글자를 몇 칸으로 그리는지 잰다. 박스 그리기
	// 문자가 그 갈래라 어느 쪽을 쓸지가 여기서 갈리고(ADR-0028), 폭 계산 전체가 그 답에
	// 맞춰 선다(ADR-0072).
	//
	// tea.NewProgram 보다 먼저다. bubbletea 가 stdin 을 읽기 시작하면 답을 그쪽이 가져간다.
	// 재는 일은 터미널을 직접 만지는 것이라 core 밖에 있다(internal/terminal).
	ambiguousWidth := terminal.ProbeAmbiguousWidth()

	// 두 칸으로 그리는 터미널이면 눈금을 켜고 처음부터 다시 시작한다 — **아래로 돌아오지
	// 않는다.** 그 눈금은 x/ansi 의 전역이고 그것을 읽는 init() 이 여기보다 먼저 지나가서,
	// 이미 뜬 프로세스에서는 켤 방법이 없다(ADR-0072).
	terminal.RestartForAmbiguousWidth(ambiguousWidth)

	buffers, err := openBuffers(files)
	if err != nil {
		return err
	}

	// 작업들이 갈라져 나올 뿌리를 한 겹 더 둔다.
	//
	// **`tea.Quit` 은 ctx 를 끊지 않는다.** 받은 것을 그대로 넘기면 `q` 로 끝냈을 때 도는
	// 작업의 ctx 가 살아 있고, 그러면 조각을 받는 쪽이 없어져 작업 goroutine 이 select 에
	// 갇힌다 — `defer` 도 `cmd.Wait` 도 돌지 않아 프로세스를 띄운 작업(`gopls 설치`) 이
	// 고아가 된다. 나가는 길에 이것을 끊어 그 자리를 막는다 (ADR-0027, ADR-0138).
	jobCtx, stopJobs := context.WithCancel(ctx)
	defer stopJobs()

	// git 표시는 여기서 읽지 않는다. 첫 화면이 뜬 뒤 갱신 작업이 채운다(ADR-0030).
	editor := &editor{ctx: jobCtx, buffers: buffers}

	// tab 이 없으면 활성 tab 도 없다. **-1 이라야** 첫 tab 이 0 번 자리에 생긴다 —
	// tab 을 여는 길이 활성 tab 바로 뒤에 끼우는 것이라(openTab, newTab) 0 으로 두면
	// 없는 tab 뒤를 가리킨다(ADR-0064).
	if !editor.hasTab() {
		editor.active = -1
	}

	switch {
	case ambiguousWidth == 1:
		editor.boxChars = boxUnicode
	default: // fallback option
		editor.boxChars = boxASCII
	}

	// tip 의 시작 자리를 흩는다. 늘 첫 문장부터면 편집기를 열 때마다 같은 것을 본다.
	//
	// 여는 순간 한 번뿐이다. 그 뒤로 자리를 옮기는 것은 알림이 날 때이고 그쪽도 무작위다
	// (tip.go 의 nextTip). 그리는 자리에서 굴리면 프레임마다 문장이 바뀌고, 검사는 editor 를
	// 직접 세워서 이 줄을 지나지 않으므로 언제나 첫 문장부터 본다(ADR-0061).
	editor.tipIndex = rand.IntN(len(assets.Tips))

	// filetree 는 기본으로 열어둔다. `:tree` 로 닫는다.
	//
	// cwd 를 못 읽으면 트리 없이 연다. 이때 편집기를 아예 못 열 이유는 없다 —
	// 지운 디렉터리에서 실행하면 나는 오류이고, 파일은 인자로 이미 받았다.
	// 화면이 좁으면 sidebarVisible 이 알아서 감추므로 여기서 크기는 보지 않는다.
	if root, err := os.Getwd(); err == nil {
		editor.sidebar = openSidebar(root)

		// CLI 인자로 연 파일 자리를 갈 곳으로 세워 둔다. 인자가 없으면 갈 자리가 없어서
		// 뿌리만 읽는다(ADR-0064).
		//
		// 읽기를 시작하지는 않는다. 여기는 Program 이 뜨기 전이라 Cmd 를 낼 자리가 없다 —
		// 첫 읽기는 첫 model 의 Init 이 startTree 로 시작하고, 트리는 그때부터
		// 이 자리를 향해 한 층씩 내려간다(ADR-0032).
		if editor.hasTab() {
			editor.sidebar.setRevealTarget(editor.activeBuffer().Path)
		}
	}

	// 언어 서버는 그 언어의 파일을 열 때 뜬다(ADR-0051, ADR-0107). 나가는 길에 내리는 자리는 여기 하나다 —
	// Program 이 돌아온 뒤가 편집기의 마지막이다.
	defer editor.shutdownServers()
	defer editor.stopWatch()

	// 도는 background 셸을 죽이고 나간다. defer 는 LIFO 라 이것이 가장 먼저 돈다.
	//
	// **위의 ctx 취소로는 모자라다.** 끊어도 신호를 보내는 것은 exec 의 감시 goroutine 이고,
	// 여기서 반환하면 프로세스가 끝나 그것이 돌지 않는다 — 신호가 나가기도 전이라 `make dev-run`
	// 이 편집기보다 오래 산다. 여기서 직접 그룹에 보내면 커널이 그 자리에서 배달한다
	// (shell-background.go, ADR-0138).
	defer editor.stopBackgroundShells()

	// tab 이 없으면 normalMode 가 빈 화면을 준다. 여기서 가르지 않는다 — 편집 화면으로
	// 가는 길이 다 그 함수를 지나므로 갈림길도 그 안에 있다(ADR-0064).
	first, _ := normalMode(editor)

	// 들어오는 이벤트를 로그로 남긴다. `-vv` 와 `--log-file` 이 둘 다 있어야 실제로 쓰인다 —
	// 그 전에는 첫 줄에서 곧바로 빠져나온다(trace.go, ADR-0050).
	//
	// 여기가 Program 을 만드는 유일한 자리라 이 한 줄로 모든 mode 의 키가 모인다.
	if _, err := tea.NewProgram(
		first,
		tea.WithContext(ctx),
		tea.WithFilter(traceEvent),
	).Run(); err != nil {
		return err
	}

	return nil
}

// openBuffers 는 CLI 인자로 받은 파일들을 tab 순서대로 연다.
//
// **인자가 없으면 tab 이 하나도 없다.** 예전에는 이름 없는 빈 buffer 하나를 끼워 넣었는데,
// 그러면 아무 파일도 열지 않았다는 것이 「이름 없는 파일을 편집하는 중」 으로 보인다.
// 그 자리에 오는 것이 빈 화면이다(ADR-0064).
//
// 같은 파일을 두 번 넘겨도 tab 은 하나다. 같은 파일에 Buffer 가 둘이면 한쪽에서 저장하는
// 순간 다른 쪽 편집이 사라진다 — openTab 이 이미 열린 tab 으로 옮겨 가는 것과 같은 이유다.
func openBuffers(files []string) ([]textarea.Viewport, error) {
	buffers := make([]textarea.Viewport, 0, len(files))
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

		buf, err := textarea.OpenBuffer(file)
		if err != nil {
			return nil, err
		}
		buffers = append(buffers, buf)
	}

	return buffers, nil
}
