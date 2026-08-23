package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/lsp"
)

// viewLocations 는 「어디로 갈까」를 고르는 목록이다. 지금 이것을 여는 것은 정의 후보가
// 여럿일 때뿐이다(ADR-0051).
//
// **팔레트가 아니라 화면을 통째로 쓰는 목록이다.** 팔레트는 「입력 맨 앞 글자가 갈래를
// 정한다」가 중심 규칙이라(view-palette.go 의 paletteKind) 프로그램이 띄우는 목록과 맞지
// 않는다. 그 글자를 지우면 파일 찾기로 돌아가는 것이 팔레트의 성질인데, 여기서 돌아갈 곳은
// 파일 찾기가 아니다. `:jobs` 와 같은 자리에 둔 이유가 이것이다(ADR-0027).
//
// 목록이 든 것은 mode 안에서만 산다. tab 을 오가며 남을 것이 아니라 이 화면이 사라지면
// 같이 사라지는 것이라 editor 가 아니라 여기 있다(ADR-0002).
func locationsMode(e *editor, title string, locations []lsp.Location) (tea.Model, tea.Cmd) {
	// 알림은 이 화면이 대신한다. 「후보 N 개」는 제목줄에 적힌다.
	e.clearNotice()

	return viewLocations{editor: e, title: title, locations: locations}, nil
}

type viewLocations struct {
	*editor

	title     string
	locations []lsp.Location

	selected int // locations 안의 자리
	top      int // 화면 첫 행
}

func (m viewLocations) Init() tea.Cmd { return nil }

func (m viewLocations) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg:
		// model 이 오면 mode 가 바뀐 것이다. 정의를 또 물은 답이 오면 이 목록이 새 목록으로
		// 갈리는 자리이기도 하다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}
		m.scrollTo()

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹고 그것으로 완성된 동작을 실행한다.
//
// **키 상태 기계를 두지 않았다.** 받는 키가 전부 한 개짜리(`j` `k` `enter` `q` `esc` `ctrl+c`)
// 라서다. 숫자 접두나 접두 키가 붙으면 `:jobs` 의 파서(jobs-key-parser.go) 와 같은 것이
// 여기에도 생긴다 — 그때까지는 표를 나눠 가질 것이 없다.
//
// 한글은 되돌린다. 입력줄이 없는 화면이라 `ㅓ` 를 받아도 `j` 로 읽어야 한다(ADR-0008).
// 한 키가 여럿으로 풀릴 수 있고, 도중에 이 화면을 벗어나면 남은 것은 버린다.
func (m viewLocations) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewLocations)
		if !ok {
			return model, nil
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, cmd
		}

		model = next
	}

	return model, nil
}

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
func (m viewLocations) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		// 고르지 않고 나간다. 커서는 물어본 자리에 그대로 있다.
		return normalMode(m.editor)
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "g":
		m.selected = 0
		m.scrollTo()

		return m, nil
	case "G":
		m.selected = len(m.locations) - 1
		m.scrollTo()

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 고른 자리로 간다. 정의가 하나였을 때와 같은 길이다(gopls.go 의 jumpTo).
func (m viewLocations) open() (tea.Model, tea.Cmd) {
	if len(m.locations) == 0 {
		return normalMode(m.editor)
	}

	cmd := m.jumpTo(m.locations[m.selected])

	model, next := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(next, cmd, m.startGitRefresh())
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다. 팔레트·트리·작업 목록과 같은 규칙이다.
func (m *viewLocations) move(delta int) {
	if len(m.locations) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(m.locations)-1)
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. `:jobs` 의 것과 같은 규칙이다.
func (m *viewLocations) scrollTo() {
	rows, height := len(m.locations), m.listHeight()
	if rows == 0 || height < 1 {
		m.selected, m.top = 0, 0

		return
	}

	m.selected = min(max(m.selected, 0), rows-1)
	m.top = min(max(m.top, 0), max(rows-height, 0))

	if m.selected < m.top {
		m.top = m.selected
	}
	if m.selected >= m.top+height {
		m.top = m.selected - height + 1
	}
}

func (m viewLocations) View() tea.View {
	height := m.listHeight()

	body := make([]string, 0, height)
	for i := m.top; i < len(m.locations) && len(body) < height; i++ {
		body = append(body, m.renderRow(m.locations[i], i == m.selected))
	}

	if len(m.locations) == 0 && height > 0 {
		body = append(body, " 갈 곳이 없습니다")
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(" j/k 이동  enter 열기  q 닫기"))
	screen = append(screen, m.renderBareStatusBar()...)

	view := newView(screen)

	if len(m.locations) > 0 {
		view.Cursor = tea.NewCursor(0, m.selected-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// renderTitle 은 맨 윗줄이다. 무엇을 고르는 중이고 몇 개인지를 적는다.
// `:jobs` 와 같이 반전이다 — 화면이 바뀐 것이 아니라 다른 화면으로 왔다는 문법이다.
func (m viewLocations) renderTitle() string {
	label := fmt.Sprintf("%s  %d 개", m.title, len(m.locations))

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// renderRow 는 갈 곳 한 줄이다.
//
//	▸ internal/core/action.go:32
//	  ~/go/pkg/mod/charm.land/bubbletea/v2@v2.0.9/tea.go:603
//
// 줄 내용은 아직 붙이지 않는다. 붙이려면 그 파일을 읽어야 하는데, 여기 오는 것들은 대개
// 열려 있지 않은 파일이라 「디스크에서 읽은 글」과 「tab 에서 고치던 글」이 갈린다.
// 그 셈은 여러 파일 검색이 이 화면을 쓸 때 같이 정한다(docs/tasks.md).
func (m viewLocations) renderRow(target lsp.Location, selected bool) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	// 화면 전체를 쓰므로 편집 영역이 아니라 화면 너비 기준이다. 표시와 앞 여백이 쓰는 만큼 뗀다.
	head := " " + marker
	place := fmt.Sprintf("%s:%d", shortenPath(target.Path()), target.Range.Start.Line+1)

	return head + trimPathLeft(place, m.width-screenWidthOf(head))
}

// trimPathLeft 는 너무 긴 자리를 **왼쪽부터** 줄인다.
//
// 오른쪽부터 자르면 파일 이름과 줄 번호가 먼저 사라진다 — 목록에서 고르는 데 쓰는 것이
// 바로 그 둘이라, 남는 것이 `/Users/bluemir/go/pkg/mod/charm.land/...` 처럼 어느 줄에나
// 같은 앞머리뿐이게 된다. 앞을 `…` 로 접으면 뒤가 살아남는다.
func trimPathLeft(place string, width int) string {
	if width < 1 {
		return ""
	}
	if screenWidthOf(place) <= width {
		return place
	}

	// `…` 한 칸을 남겨 두고, 들어갈 때까지 앞에서 한 글자씩 뗀다.
	kept := place
	for len(kept) > 0 && screenWidthOf(kept) > width-1 {
		size, _ := clusterAt([]byte(kept), 0, 0)
		kept = kept[size:]
	}

	return "…" + kept
}

// renderBareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다. `:jobs` 와 같은 이유다 —
// 이 화면은 트리를 덮으므로 statusBar 만 32 칸 밀리면 화면이 반쪽만 바뀐 것으로 보인다.
func (m viewLocations) renderBareStatusBar() []string {
	bare := *m.editor
	bare.sidebar = sidebar{}

	return bare.renderStatusBar("GOTO", bare.notice)
}

// shortenPath 는 화면에 적을 경로다. 이 목록과 저장 문구(view-editor-command.go) 가 나눠 쓴다.
//
// 오는 경로는 두 갈래다. 저장소 안의 파일과, 정의를 따라 나간 module cache 의 파일이다.
// 앞엣것은 지금 자리 기준 상대 경로가 짧고 눈에 익다. 뒤엣것은 홈 아래 깊은 자리라
// `~` 로 줄이는 것이 전부다 — 그래도 길지만 어느 판(`@v2.0.9`) 의 파일인지가 그 안에 있다.
//
// 줄이기만 하고 자르지는 않는다. 폭에 맞춰 접는 것은 그리는 쪽이 한다.
func shortenPath(path string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
			return "~" + string(filepath.Separator) + rest
		}
	}

	return path
}
