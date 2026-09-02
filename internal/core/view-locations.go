package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/lsp"
)

// locationsMinTextHeight 는 판을 열고도 편집 영역에 남아야 할 행 수다.
// 다른 두 판과 같은 값이다 — 판 하나가 편집 화면을 다 먹지 않는다는 규칙은 같아야 한다.
const locationsMinTextHeight = symbolMinTextHeight

// locationsFrame 은 판이 목록 말고 쓰는 행 수다. 테두리 둘이다.
// register 판과 같다 — 치는 것이 없고 보여주는 것만 있다.
const locationsFrame = 2

// locationsMaxRows 는 목록에 보일 최대 행 수다.
//
// 담긴 것만큼 자라되 여기서 멈춘다. 사용처는 셋일 때도 서른일 때도 있는데(패키지 이름 위의
// `\gd` 는 그 패키지의 파일 수만큼 온다) 서른을 다 그리면 판이 화면을 거의 다 먹어서
// **「간 자리가 같이 보인다」는 판을 쓰는 까닭 자체가 사라진다**(ADR-0069).
const locationsMaxRows = 16

// viewLocations 는 「어디로 갈까」를 고르는 하단 drawer 다. 정의 후보가 여럿일 때(ADR-0051)
// 와 사용처가 여럿일 때(ADR-0068) 가 여기로 온다. 둘의 차이는 아래 줄에 적히는 말뿐이다.
//
// **화면을 통째로 쓰지 않고 편집 영역의 행을 가져간다**(ADR-0069). 기호 판·register 판과
// 같은 자리다(ADR-0056, ADR-0058). 이 목록을 보는 까닭이 「이 이름을 어디서 쓰나」를 훑는
// 것이라 **간 자리가 같이 보여야** 한다.
//
// **둘러보고 확정하는 판이다.** `j`/`k` 가 커서를 실제로 그 자리로 옮겨 보여주고, `enter`
// 가 판을 닫으며 확정한다. `q`·`esc` 는 취소라 판을 열기 전 자리로 되돌아가고 둘러보며
// 연 tab 도 닫는다. 되돌아간 자리 판과 같은 손이다(ADR-0071, ADR-0073).
//
// 처음에는 「`enter` 로 뛰어도 판을 열어 둔다」였다. 사용처 여러 군데를 차례로 훑는 것이
// 목적이니 열려 있어야 한다고 보았는데, **써 보니 목록에 `경로:줄` 뿐이라 그 줄이 무엇인지
// 보려면 어차피 가 봐야 했다.** 가 보는 것이 `j` 면 `enter` 는 고르는 일만 남는다(ADR-0073).
//
// 목록이 든 것은 mode 안에서만 산다. tab 을 오가며 남을 것이 아니라 이 화면이 사라지면
// 같이 사라지는 것이라 editor 가 아니라 여기 있다(ADR-0002).
func locationsMode(e *editor, title string, locations []lsp.Location) (tea.Model, tea.Cmd) {
	// 뛸 자리가 없으면 열지 않는다. 판은 편집 맥락에 딸린 자리라, 볼 파일이 없는 화면에
	// 띄우면 그 자리 설명이 무너진다 — 물어 놓고 답이 오기 전에 tab 을 닫은 경우다.
	// register 판과 같다(ADR-0064).
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	// 알림은 이 판이 대신한다. 「무엇을 고르는 중이고 몇 개인가」는 아래 줄에 적힌다.
	e.clearNotice()

	m := viewLocations{editor: e, title: title, locations: locations}

	// 둘러보기 전 화면을 적어 둔다. 취소하면 이것으로 되돌린다(preview.go).
	//
	// **여는 것만으로는 옮기지 않는다.** 미리보기는 `j`/`k` 를 쳐야 시작한다.
	m.preview = startPreview(e)

	e.drawerHeight = m.locationsDrawerHeight()
	e.scrollToCursor()

	return m, nil
}

type viewLocations struct {
	*editor

	title     string
	locations []lsp.Location

	selected int // locations 안의 자리
	top      int // 판의 첫 행

	// preview 는 둘러보기 전 화면이다. 취소하면 이것으로 되돌리고, 확정하면 여기 적힌
	// 자리 하나가 되돌아오기 이력에 담긴다(preview.go, ADR-0070, ADR-0073).
	preview previewSession

	// pending 은 방금 미리보기가 낸 Cmd 다. selectTo 가 포인터 수신자라 돌려줄 자리가 없다.
	pending tea.Cmd
}

// locationsRows 는 목록에 쓸 수 있는 행 수다. 담긴 것과 최대치와 남은 자리 중 가장 작은 것이다.
//
// 화면이 낮으면 들어가는 만큼만 그리고 `j`/`k` 로 훑는다. 거절하지 않는 것은 여기 오는 것이
// 사용자가 부른 화면이 아니라 **물어본 답**이라서다 — 답을 못 보이면 물어본 것이 사라진다.
func (m viewLocations) locationsRows() int {
	room := m.textAndDrawerHeight() - locationsFrame - locationsMinTextHeight
	want := min(max(len(m.locations), 1), locationsMaxRows)

	return min(want, max(room, 1))
}

// locationsDrawerHeight 는 판이 편집 영역에서 가져갈 행 수다.
func (m viewLocations) locationsDrawerHeight() int {
	return m.locationsRows() + locationsFrame
}

func (m viewLocations) Init() tea.Cmd { return nil }

func (m viewLocations) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		m.drawerHeight = m.locationsDrawerHeight()
		m.scrollToCursor()
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseWheelMsg:
		// 판이 화면 아래에 붙어 있어서 포인터가 어디 있든 목록을 오르내린다.
		// 다른 두 판과 같은 규칙이다.
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		cmd := m.pending
		m.pending = nil

		return m, cmd
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// model 이 오면 mode 가 바뀐 것이다. 정의나 사용처를 또 물은 답이 오면 이 판이
		// 새 판으로 갈리는 자리이기도 하다(job.go).
		//
		// **판 높이는 여기서 지우지 않는다.** 새 판이 자기 높이를 이미 잡았다(ADR-0069).
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
// 한글은 되돌린다. 입력줄이 없는 판이라 `ㅓ` 를 받아도 `j` 로 읽어야 한다(ADR-0008).
// 한 키가 여럿으로 풀릴 수 있고, 도중에 이 화면을 벗어나면 남은 것은 버린다.
func (m viewLocations) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	// **미리보기가 낸 Cmd 를 모아서 나간다.** 자모 하나가 키 여럿으로 풀리므로(`접` 은
	// `w`·`j`·`q` 다) 앞 키가 미리보기를 태우고 뒤 키가 이 화면을 벗어날 수 있다. 그때
	// 흘리면 안 되는 것이 그 안에 있다 — 파일을 연 뒤 언어 서버를 띄우는 Cmd 인데,
	// startServer 는 이미 그 서버의 `starting` 을 세워 두어서 Cmd 를 버리면 서버가 영영 뜨지
	// 않는다(ADR-0008, ADR-0051).
	var previews []tea.Cmd

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewLocations)
		if !ok {
			return model, tea.Batch(previews...)
		}

		next, cmd := here.run(expanded)
		if cmd != nil {
			return next, tea.Batch(append(previews, cmd)...)
		}

		// 미리보기 Cmd 는 여기서 걷는다. run 에서 돌려주면 이 고리가 그것을 「이 화면을
		// 벗어났다」로 읽어 남은 자모를 버린다.
		if after, ok := next.(viewLocations); ok && after.pending != nil {
			previews = append(previews, after.pending)
			after.pending = nil
			next = after
		}

		model = next
	}

	return model, tea.Batch(previews...)
}

// run 은 동작 하나다. 모르는 키면 아무 일도 하지 않는다.
func (m viewLocations) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return m.cancel()
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
		m.move(pageRows(pageFull, m.locationsDrawerHeight()))

		return m, nil
	case "pgup":
		m.move(-pageRows(pageFull, m.locationsDrawerHeight()))

		return m, nil
	case "g", "home":
		m.selectTo(0)

		return m, nil
	case "G", "end":
		m.selectTo(len(m.locations) - 1)

		return m, nil
	case "enter":
		return m.open()
	default:
		return m, nil
	}
}

// open 은 `enter` 다. 둘러보던 자리로 **확정하고 판을 닫는다**(ADR-0073).
//
// 커서를 다시 맞추지 않아도 된다 — jumpTo 안의 moveToLocation 이 이미 판을 뺀 높이로
// 스크롤한다(`textHeight`).
func (m viewLocations) open() (tea.Model, tea.Cmd) {
	if len(m.locations) == 0 {
		return m.cancel()
	}

	target := m.locations[m.selected]

	// **판을 열기 전 자리 하나만 이력에 담는다.** 판 안에서 몇 군데를 둘러봤든 한 동작으로
	// 본다 — 다섯 군데를 훑었다고 `ctrl+o` 를 다섯 번 쳐야 하면 물어보던 자리가 그만큼
	// 멀어진다. 커서는 이미 둘러보던 곳에 가 있으므로 **적어 둔 자리**를 담는다(ADR-0070).
	if m.preview.hasOrigin {
		m.recordJumpFrom(m.preview.origin)
	}

	// 아직 그 자리를 안 밟았을 수 있다 — 열자마자 `enter` 를 친 경우다.
	cmd := m.jumpTo(target)
	m.arrive()

	// 둘러보느라 연 tab 중 확정한 것만 남긴다.
	m.preview.keep(m.editor, target.Path())

	model, next := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(next, cmd, m.startGitRefresh())
}

// cancel 은 `q`·`esc` 다. 판을 열기 전 자리로 되돌리고 둘러보며 연 tab 을 전부 닫는다.
//
// **이력에도 담지 않는다** — 물어보고 아무 데도 가지 않았으므로 `ctrl+o` 가 데려다줄 자리가
// 생기면 안 된다(ADR-0070, ADR-0073).
func (m viewLocations) cancel() (tea.Model, tea.Cmd) {
	cmd := m.preview.restore(m.editor)

	model, next := normalMode(m.editor)

	return model, tea.Batch(next, cmd)
}

// move 는 고른 자리를 옮기고 **그 자리를 곧바로 보여준다**(ADR-0073).
func (m *viewLocations) move(delta int) {
	m.selectTo(m.selected + delta)
}

// selectTo 는 고른 자리를 그 index 로 옮기고 미리보기를 태운다. 양끝에서 멈춘다.
// `j`/`k`·`g`/`G`·휠이 모두 여기로 온다 — 「고른 자리가 바뀌면 보여준다」가 한 자리에 있다.
func (m *viewLocations) selectTo(index int) {
	if len(m.locations) == 0 {
		return
	}

	before := m.selected

	m.selected = min(max(index, 0), len(m.locations)-1)
	m.scrollTo()

	// 양끝에서 멈췄으면 다시 그리로 갈 것이 없다.
	if m.selected == before {
		return
	}

	m.pending = m.jumpTo(m.locations[m.selected])
}

// scrollTo 는 고른 자리가 보이도록 첫 행을 최소한으로 움직인다.
func (m *viewLocations) scrollTo() {
	rows, height := len(m.locations), m.locationsRows()
	if rows == 0 {
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
	// 커서는 편집 내용에 그대로 둔다. 뛴 자리가 보이는 편이 낫다 — register 판과 같다.
	// 고른 줄은 판 안에서 반전으로 드러난다(renderRow).
	view := m.editorView(tea.CursorBlock, "GOTO", m.noticeOr(m.hint()))

	// 편집 화면을 다 그린 뒤 그 아래 빈 자리에 판을 얹는다. textHeight 가 이미 그만큼
	// 줄어 있어서 덮는 것이 없다(view-symbol.go).
	left, top := m.sidebarLeft(), tablineHeight+m.textHeight()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderDrawer()).X(left).Y(top).Z(1),
	).Render()

	return view
}

// hint 는 statusBar 아래 줄에 적는 말이다.
//
// 「무엇을 고르는 중인가」가 판 안이 아니라 여기 있다. 판에 제목줄을 두면 목록에 쓸 행이
// 하나 줄고, 그 한 줄이 낮은 화면에서는 목록의 절반이다. 기호 판이 개수를 판 안에 둔 것과
// 갈리는데, 저쪽은 치면서 줄어드는 수라 눈이 입력줄에 붙어 있다(ADR-0056).
//
// **몇 번째인지도 몇 개인지도 적지 않는다.** 둘 다 아랫 테두리가 든다 — 숫자가 설명하는
// 대상 옆에 있는 것이 낫고, 여기는 그만큼 키 안내에 자리를 넘긴다(ADR-0079, ADR-0101).
func (m viewLocations) hint() string {
	return m.title + "  j/k 둘러보기  enter 확정  esc 취소"
}

// renderDrawer 는 판 전체를 화면 행 문자열로 만든다. 각 행이 정확히 textWidth() 칸이다.
func (m viewLocations) renderDrawer() string {
	return drawer{
		chars:  m.boxChars,
		width:  m.textWidth(),
		height: m.locationsRows(),
		top:    m.top,
		count:  len(m.locations),
		empty:  "갈 곳이 없습니다",
		row: func(at, inner int) string {
			return m.renderRow(m.locations[at], at == m.selected, inner)
		},
		at: m.selected + 1,
	}.render()
}

// locationsMarkWidth 는 고른 자리 표시가 쓰는 폭이다.
const locationsMarkWidth = 2

// renderRow 는 갈 곳 한 줄이다.
//
//	▸ internal/core/action.go:32
//	  ~/go/pkg/mod/charm.land/bubbletea/v2@v2.0.9/tea.go:603
//
// 고른 줄은 반전이다. 커서가 편집 내용에 남아 있어서(View) 이 줄이 스스로 어느 것이
// 골렸는지 말해야 한다 — 팔레트가 고른 줄을 칠하는 것과 같다(view-palette.go).
//
// 줄 내용은 아직 붙이지 않는다. 붙이려면 그 파일을 읽어야 하는데, 여기 오는 것들은 대개
// 열려 있지 않은 파일이라 「디스크에서 읽은 글」과 「tab 에서 고치던 글」이 갈린다.
// 그 셈은 여러 파일 검색이 이 판을 쓸 때 같이 정한다(docs/tasks.md).
func (m viewLocations) renderRow(target lsp.Location, selected bool, inner int) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}

	place := fmt.Sprintf("%s:%d", shortenPath(target.Path()), target.Range.Start.Line+1)

	// **왼쪽부터 접는다.** 오른쪽부터 자르면 파일 이름과 줄 번호가 먼저 사라지는데, 목록에서
	// 고르는 데 쓰는 것이 바로 그 둘이다 — 남는 것이 `/Users/bluemir/go/pkg/mod/...` 처럼
	// 어느 줄에나 같은 앞머리뿐이게 된다(render-status-bar.go 의 trimLeftToWidth).
	body := max(inner-locationsMarkWidth, 0)

	// 강조를 입힌 뒤에는 폭을 잴 수 없다(lines.WidthOf 가 escape 까지 센다). 칸을 먼저
	// 채우고 그다음에 반전을 입힌다 — register 판과 같은 순서다.
	text := marker + padTo(trimLeftToWidth(place, body), body)
	if selected {
		return reverse.Render(text)
	}

	return text
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
