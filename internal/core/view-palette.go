package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bluemir/zn/internal/textarea"
)

// 박스 크기다. sidebar 의 수치와 같은 자리에 둔다(docs/spec.md).
const (
	paletteMaxWidth = 64 // 이보다 넓으면 경로가 아니라 여백을 보게 된다
	paletteMinWidth = 24 // 이보다 좁으면 경로가 무의미하게 잘린다
	paletteRows     = 10 // 목록에 보여줄 최대 행 수
	paletteTop      = 1  // tabline(0 행) 은 덮지 않는다. 어느 tab 을 보고 있었는지가 남는다
)

// paletteFrame 은 박스가 목록 말고 쓰는 행 수다. 테두리 둘, 입력줄, 가름줄이다.
const paletteFrame = 4

// viewPalette 는 `ctrl+p` 로 여는 command palette 다.
//
// 화면 위쪽 가운데에 박스를 띄우고 그 아래로 편집 화면이 그대로 비친다. 기본은 파일 찾기이고
// 맨 앞 글자가 갈래를 정한다 — `>` 는 명령 목록, `!` 는 셸, `/` 는 프로젝트 검색이다.
//
// 어디서 열렸는지 기억하지 않는다. sidebar 에서 열어도 끝나면 편집 영역으로 나온다 —
// 돌아갈 곳을 들고 다니는 것은 ADR-0005 가 `:` 에서 이미 거절한 비용이다.
func paletteMode(e *editor) (tea.Model, tea.Cmd) {
	if !e.paletteFits() {
		return normalModeMessage(e, "화면이 좁아 팔레트를 열 수 없습니다")
	}

	// 트리 뿌리와 같은 기준이다. 화면에 이미 보이고 있어서 무엇을 찾는지 설명할 필요가 없다.
	root, err := os.Getwd()
	if err != nil {
		return normalModeError(e, err)
	}

	// 목록은 백그라운드 작업이 채운다. 열 때마다 새로 읽는 것은 그대로이고(ADR-0011),
	// 다 읽을 때까지 기다리지 않을 뿐이다. 이미 돌고 있으면 그것에 붙는다.
	//
	// 앞서 모아둔 목록이 있으면 그것을 보면서 시작한다. 새 조각이 오면 통째로 갈린다 —
	// 빈 목록에서 시작하면 열 때마다 화면이 한 번 번쩍인다.
	cmd := e.startJob("파일 인덱싱", nil, func(ctx context.Context) <-chan jobProgress {
		return indexFiles(ctx, root)
	})

	m := viewPalette{editor: e}
	m.filter()

	return m, cmd
}

type viewPalette struct {
	*editor

	// input 은 친 그대로다. 맨 앞의 `>` 도 지우지 않고 들고 있다 — 그것이 곧 어느 표를 보는지다.
	input inputLine

	hits     []paletteHit
	selected int // hits 안의 자리
	top      int // 목록의 첫 행. sidebar 의 top 과 같은 뜻이다
}

func (m viewPalette) Init() tea.Cmd { return nil }

func (m viewPalette) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 좁아져서 못 그리게 됐다. 보이지 않는 mode 에 갇히면 키를 쳐도 아무 일이 안 난다.
		if !m.paletteFits() {
			return normalMode(m.editor)
		}
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return normalMode(m.editor)
		case "enter":
			return m.run()
		case "up":
			m.move(-1)

			return m, nil
		case "down":
			m.move(1)

			return m, nil
		case "pgdown":
			// 한 화면이다. 편집 영역·트리와 같은 자를 쓴다(page.go 의 pageRows).
			m.move(textarea.PageRows(textarea.PageFull, m.paletteListRows()))

			return m, nil
		case "pgup":
			m.move(-textarea.PageRows(textarea.PageFull, m.paletteListRows()))

			return m, nil
		case "left", "right", "home", "end":
			// **다섯 키가 목록이 아니라 입력줄 것이다**(ADR-0113 §2). 목록은 세로로 서
			// 있으므로 `↑`·`↓`·`pgup`·`pgdown`·휠로 다 갈 수 있고, 치고 있는 글 안을
			// 오갈 길은 이것뿐이다. `g`·`G` 는 여기 둘 수 없다 — 그 글자가 걸러낼 말의
			// 일부다.
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()
			m.filter()

			return m, nil
		case "ctrl+p":
			// 여는 키를 다시 눌러도 아무 일도 하지 않는다. 목록 이동은 위아래뿐이다.
			return m, nil
		case "backspace":
			// command·search mode 와 같이 다 지우면 나간다.
			if m.input.empty() {
				return normalMode(m.editor)
			}
			m.input.deleteBackward()
			m.filter()

			return m, nil
		default:
			// 한글은 여기서 글자다. 입력줄이 있는 mode 는 키를 되돌리지 않는다(ADR-0008).
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)
			m.filter()

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 팔레트는 화면 위에 얹힌 박스라 아래 편집 내용을 굴려도 볼 수 없다.
		// 그래서 다른 mode 와 달리 포인터가 어디 있든 목록을 오르내린다 — 화살표와 같다.
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 다른 mode 와 같이 공용 처리에 넘기고, 여기서만 목록을 다시 거른다.
		// 인덱싱이 도는 동안 목록이 길어지므로 새로 온 파일도 치고 있는 패턴에 걸려야 한다.
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}
		m.refilter()

		return m, cmd
	default:
		return m, nil
	}
}

// paletteKind 는 팔레트가 지금 무엇을 하는 중인지다. 입력 맨 앞 글자가 정한다.
//
// 별도 키도 상태도 아니다. 그 글자를 지우는 순간 파일 찾기로 저절로 돌아온다.
type paletteKind int

const (
	paletteKindFile            paletteKind = iota // 접두 없음. 파일 찾기다
	paletteKindCommand                            // `>`. 명령 목록이다
	paletteKindShell                              // `!`. 고를 목록이 없고 친 것을 셸에 넘긴다
	paletteKindShellBackground                    // `!&`. 셸에 넘기되 작업으로 돌린다(ADR-0138)
	paletteKindGrep                               // `/`. 고를 목록이 없고 친 것으로 프로젝트를 뒤진다
)

// hasList 는 고를 후보가 있는 갈래인지다.
//
// 셸과 검색은 친 것을 그대로 넘기므로 후보가 없다. **목록·개수·Enter 셋이 같은 답을 봐야
// 한다** — 하나만 갈라 두면 고를 것이 없는 자리에 「일치하는 것이 없습니다」가 뜬다.
func (kind paletteKind) hasList() bool {
	return kind == paletteKindFile || kind == paletteKindCommand
}

// kind 는 갈래와 접두를 뗀 나머지를 준다.
func (m viewPalette) kind() (paletteKind, string) {
	if rest, ok := strings.CutPrefix(m.input.text, ">"); ok {
		return paletteKindCommand, strings.TrimLeft(rest, " ")
	}
	// **`!&` 를 `!` 보다 먼저 본다.** `!` 가 `!&` 의 접두라 순서가 뒤집히면 영영 안 걸리고,
	// `&` 가 셸 줄의 첫 글자가 되어 조용히 문법 오류를 낸다(ADR-0138).
	if rest, ok := strings.CutPrefix(m.input.text, "!&"); ok {
		return paletteKindShellBackground, rest
	}
	if rest, ok := strings.CutPrefix(m.input.text, "!"); ok {
		// `>` 와 달리 앞 공백을 떼지 않는다. 셸이 읽을 글자를 여기서 고치지 않는다 —
		// `:!` 도 뗀 적이 없다(ADR-0045).
		return paletteKindShell, rest
	}
	if rest, ok := strings.CutPrefix(m.input.text, "/"); ok {
		// `!` 와 같이 앞 공백을 떼지 않는다. 찾을 것은 정규식이라 공백도 패턴의 글자다.
		return paletteKindGrep, rest
	}

	return paletteKindFile, m.input.text
}

// labels 는 지금 표에서 매칭 대상이 되는 글자들이다.
//
// 셸과 검색은 고를 것이 없어서 비어 있다. 그 덕에 filter 는 그 갈래들을 몰라도 되고 hits 가
// 저절로 빈다.
func (m viewPalette) labels() []string {
	kind, _ := m.kind()

	if !kind.hasList() {
		return nil
	}
	if kind == paletteKindFile {
		return m.files
	}

	commands := m.commands()

	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		labels = append(labels, command.label())
	}

	return labels
}

// commands 는 지금 성립하는 명령들이다.
//
// **목록도 고르는 것도 그리는 것도 이것을 지난다.** 셋이 같은 것을 보아야 hits 의 자리가
// 어긋나지 않는다 — 하나만 걸러 놓으면 고른 줄과 도는 명령이 갈린다.
//
// 성립하지 않는 것은 목록에서 뺀다(paletteCommand.when). 열린 파일이 없는 화면에서는
// 여기 든 열여섯 중 절반이 「열린 파일이 없습니다」로 끝나던 것들이다.
func (m viewPalette) commands() []paletteCommand {
	available := make([]paletteCommand, 0, len(paletteCommands))

	for _, command := range paletteCommands {
		if command.when != nil && !command.when(m.editor) {
			continue
		}

		available = append(available, command)
	}

	return available
}

// filter 는 입력으로 목록을 다시 거른다. 입력이 바뀌었으므로 고른 자리는 처음으로 돌아간다.
func (m *viewPalette) filter() {
	_, pattern := m.kind()

	m.hits = filterPalette(pattern, m.labels())
	m.selected, m.top = 0, 0
}

// refilter 는 후보가 늘었을 때 다시 거른다. filter 와 달리 고른 자리를 그대로 둔다.
//
// 인덱싱이 파일을 부을 때마다 맨 위로 튀면 목록을 훑을 수 없다. 새로 온 파일이 위로 끼어들면
// 커서가 가리키는 항목은 바뀔 수 있지만, 그것이 매번 처음으로 돌아가는 것보다 낫다.
func (m *viewPalette) refilter() {
	_, pattern := m.kind()

	m.hits = filterPalette(pattern, m.labels())
	m.scrollTo()
}

// move 는 고른 자리를 옮긴다. 양끝에서 멈춘다 — 둘러 가면 목록의 끝이 어디인지 알 수 없다.
func (m *viewPalette) move(delta int) {
	if len(m.hits) == 0 {
		return
	}

	m.selected = min(max(m.selected+delta, 0), len(m.hits)-1)
	m.scrollTo()
}

// scrollTo 는 고른 자리가 보이도록 top 을 최소한으로 움직인다. sidebar 의 것과 같은 규칙이다.
func (m *viewPalette) scrollTo() {
	rows := m.paletteListRows()

	m.selected = min(max(m.selected, 0), max(len(m.hits)-1, 0))
	m.top = min(max(m.top, 0), max(len(m.hits)-rows, 0))

	if m.selected < m.top {
		m.top = m.selected
	}
	if m.selected >= m.top+rows {
		m.top = m.selected - rows + 1
	}
}

// run 은 고른 것을 실행한다.
func (m viewPalette) run() (tea.Model, tea.Cmd) {
	kind, line := m.kind()

	// 셸과 검색은 고른 것이 아니라 친 것을 실행한다. 그래서 hits 를 보기 전에 갈린다.
	// 아무것도 치지 않았으면 가만히 있는다 — 고를 것이 없을 때와 같다.
	if !kind.hasList() && line == "" {
		return m, nil
	}
	if kind.hasList() && len(m.hits) == 0 {
		return m, nil
	}

	// **고른 범위를 놓기 전에 집는다.** whenSelection 이 그것을 보므로, 놓은 뒤에 다시 부르면
	// 목록에서 둘이 빠지고 hits 의 자리가 어긋난다 — 「대문자로 맞추기」를 골랐는데 엉뚱한
	// 명령이 도는 자리다(commands 주석, ADR-0111).
	commands := m.commands()

	// **여기서부터는 반드시 팔레트를 떠난다. 고른 범위를 사본으로 옮기고 놓는 자리다.**
	//
	// visual 에서 `ctrl+p` 로 열었으면 고른 것이 살아 있고 상자 뒤로 칠해져 있다(ADR-0037).
	// 무엇을 고르든 이 문을 지나므로 옮기는 자리도 놓는 자리도 여기 하나다 — `esc` 는
	// normalMode 가 놓는다.
	//
	// **놓기 전에 싣는다.** 범위를 보는 명령은 buffer 가 아니라 이 사본을 읽는다(ADR-0111).
	// 그래서 놓는 일이 지금까지와 똑같이 무조건이다.
	//
	// 놓지 않으면 셋이 어긋난다. 커서를 옮기며 둘러보는 판(되돌아간 자리) 에서 강조가 커서를
	// 따라 널뛰고, 특수문자 판은 고른 것이 칠해진 채 커서 뒤에 글자를 넣으며, 다른 파일을 열면
	// normalMode 가 **새 buffer 만** 지워서 원래 tab 에 유령 강조가 남는다.
	// **tab 이 바뀌기 전이라 지우는 buffer 가 언제나 옳다.**
	opts := []runOption{}
	if m.hasTab() {
		if area, ok := m.activeBuffer().SelectionRange(); ok {
			opts = append(opts, withRange(area))
		}

		m.activeBuffer().ClearSelection()
	}

	if kind == paletteKindShell {
		return runShell(m.editor, line)
	}
	if kind == paletteKindShellBackground {
		// `:!&` 와 오는 자리가 같다. `%` 를 펴는 것은 그쪽이 한다(ADR-0114, ADR-0138).
		return runShellBackgroundMode(m.editor, line)
	}
	if kind == paletteKindGrep {
		// `:grep <패턴>` 과 팔레트의 「프로젝트 검색」이 오는 자리다. 패턴이 정규식으로 말이
		// 되지 않으면 runGrep 이 알림만 적고 판을 열지 않는다(ADR-0078).
		return runGrep(m.editor, line)
	}

	index := m.hits[m.selected].index

	if kind == paletteKindCommand {
		// 고른 뒤에 성립하지 않게 되었다. 지금은 팔레트가 열린 동안 조건이 바뀌지 않지만,
		// 자리를 벗어난 채 집으면 그 자리에서 터진다.
		if index >= len(commands) {
			return normalMode(m.editor)
		}

		return commands[index].run(m.editor, opts...)
	}

	return m.openFile(m.files[index])
}

// openFile 은 고른 파일을 tab 으로 연다. sidebar 의 enter 와 같은 길이다.
func (m viewPalette) openFile(path string) (tea.Model, tea.Cmd) {
	// 목록을 읽은 뒤에 지워졌을 수 있다. Stat 은 symlink 를 따라가므로 가리키는 것이 무엇인지로 본다.
	// FIFO 나 소켓은 ReadFile 이 영영 돌아오지 않아서 편집기가 통째로 멈춘다.
	info, err := os.Stat(path)
	if err != nil {
		return normalModeError(m.editor, err)
	}
	if !info.Mode().IsRegular() {
		return normalModeMessage(m.editor, "일반 파일이 아닙니다: "+path)
	}

	reveal, err := m.openTab(path)
	if err != nil {
		return normalModeError(m.editor, err)
	}
	m.scrollToCursor()
	m.arrive()

	model, cmd := normalMode(m.editor)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	// reveal 은 트리가 아직 그 자리를 읽지 않았으면 읽는 작업을 시작한다(ADR-0032).
	return model, tea.Batch(cmd, m.startGitRefresh(), reveal)
}

// paletteWidth 는 박스 전체 너비다. 테두리를 포함한다.
// 좌우로 한 칸씩은 아래 화면이 보여야 떠 있는 것으로 읽힌다.
func (e editor) paletteWidth() int {
	return min(paletteMaxWidth, e.width-2)
}

// paletteListRows 는 목록에 쓸 수 있는 행 수다. statusBar 를 침범하지 않는 선까지다.
func (e editor) paletteListRows() int {
	return min(paletteRows, e.sidebarHeight()-paletteTop-paletteFrame)
}

// paletteLeft 는 박스가 시작하는 화면 칸이다.
//
// 편집 영역이 아니라 **화면** 가운데다. sidebar 를 여닫는다고 박스가 옮겨 다니면 안 된다.
func (e editor) paletteLeft() int {
	return (e.width - e.paletteWidth()) / 2
}

// paletteFits 는 박스를 그릴 수 있는지다. sidebarVisible 과 같은 종류의 방어다.
func (e editor) paletteFits() bool {
	return e.paletteWidth() >= paletteMinWidth && e.paletteListRows() >= 1
}

func (m viewPalette) View() tea.View {
	view := m.editorView(tea.CursorBar, "PALETTE", m.renderCounter())

	// 편집 화면을 다 그린 뒤 그 위에 박스를 얹는다. 셀 단위라 두 칸 글자와 색이 어긋나지 않는다.
	// 합성은 팔레트에서만 태운다 — 셀 버퍼를 지나면 줄 끝의 빈 칸이 잘려서 화면 문자열이 달라진다.
	left := m.paletteLeft()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.renderBox()).X(left).Y(paletteTop).Z(1),
	).Render()

	// 커서는 편집 내용이 아니라 박스 안 치는 자리에 있어야 한다.
	_, cursor := m.inputText()
	view.Cursor = tea.NewCursor(left+2+cursor, paletteTop+1)
	view.Cursor.Shape = tea.CursorBar

	return view
}

// renderCounter 는 statusBar 아래 줄이다. 몇 개 중 몇 개가 걸렸는지 보여준다.
//
// 셸과 검색은 셀 것이 없어서 비운다. `0/0` 은 아무것도 못 찾은 것처럼 보이는 거짓말이다.
func (m viewPalette) renderCounter() string {
	if kind, _ := m.kind(); !kind.hasList() {
		return ""
	}

	return fmt.Sprintf("%d/%d", len(m.hits), len(m.labels()))
}

// renderBox 는 박스 전체를 화면 행 문자열로 만든다. 각 행이 정확히 paletteWidth() 칸이다.
//
// 테두리는 lipgloss 의 Border 를 쓰지 않고 손으로 붙인다. Border 는 안쪽 내용의 폭을 스스로
// 재는데 강조 escape 가 이미 섞여 있어서 그 계산을 믿을 수 없다. sidebar 가 구분선을 손으로
// 붙이는 것과 같은 이유다.
func (m viewPalette) renderBox() string {
	width := m.paletteWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{
		chars.topLeft + line + chars.topRight,
		m.renderInputRow(inner),
		chars.leftTee + line + chars.rightTee,
	}
	rows = append(rows, m.renderListRows(inner)...)
	rows = append(rows, chars.bottomLeft+line+chars.bottomRight)

	return strings.Join(rows, "\n")
}

// inputText 는 입력줄에 그릴 글과 그 안에서 커서가 설 칸이다.
//
// **넘치면 커서가 보이도록 왼쪽부터 접는다.** 커서가 줄 안으로 들어오기 전에는 오른쪽부터
// 잘랐는데, 그때는 커서도 늘 오른쪽 끝이라 담을 것이 없었다. 이제 커서가 앞으로 갈 수
// 있으므로 접는 자리가 따라와야 한다 — grep 검색창이 같은 박스에서 같은 답을 쓴다
// (input-line.go, view-grep-input.go).
//
// **커서 자리를 재는 데도 쓰므로 한 자리에서 만든다** — 그리는 글과 재는 글이 갈리면 커서가
// 글자 뒤에 서지 않는다.
func (m viewPalette) inputText() (text string, cursorCol int) {
	// 커서 한 칸을 남긴다. 안 남기면 마지막 글자를 친 순간 커서가 테두리 위에 선다.
	room := m.paletteWidth() - 4 - 1

	return m.input.visible("", max(room, 1))
}

// renderInputRow 는 치고 있는 것을 보여주는 줄이다. 비어 있으면 무엇을 치면 되는지 흐리게 알려준다.
func (m viewPalette) renderInputRow(inner int) string {
	side := m.boxChars.vertical

	if m.input.empty() {
		return side + " " + styleDetail.Render(padTo(truncateToWidth("파일 찾기. > 명령, ! 셸, !& 백그라운드, / 검색", inner), inner)) + " " + side
	}

	text, _ := m.inputText()

	return side + " " + padTo(text, inner) + " " + side
}

// renderListRows 는 목록 행들이다. 걸린 것이 없으면 그 사실을 한 줄로 알린다.
func (m viewPalette) renderListRows(inner int) []string {
	side := m.boxChars.vertical

	kind, rest := m.kind()

	// 셸과 검색은 목록이 아니라 안내를 놓는다. 고를 것이 없는데 「일치하는 것이 없습니다」가
	// 뜨면 무엇을 잘못 쳤는지 찾게 된다.
	if !kind.hasList() {
		guide := ""

		switch kind {
		case paletteKindShell:
			guide = "Enter 로 셸에서 실행합니다"
			if rest == "" {
				guide = "! 뒤에 셸 명령을 칩니다. `&` 를 더 치면 백그라운드다"
			}
		case paletteKindShellBackground:
			guide = "Enter 로 백그라운드에서 실행합니다"
			if rest == "" {
				guide = "!& 뒤에 백그라운드로 돌릴 셸 명령을 칩니다"
			}
		case paletteKindGrep:
			guide = "Enter 로 프로젝트를 검색합니다"
			if rest == "" {
				guide = "/ 뒤에 찾을 것을 칩니다"
			}
		}

		return []string{side + " " + styleDetail.Render(padTo(truncateToWidth(guide, inner), inner)) + " " + side}
	}

	if len(m.hits) == 0 {
		return []string{side + " " + styleDetail.Render(padTo("일치하는 것이 없습니다", inner)) + " " + side}
	}

	rows := []string{}
	for i := m.top; i < len(m.hits) && len(rows) < m.paletteListRows(); i++ {
		hit := m.hits[i]

		row := paletteRow{positions: hit.positions, selected: i == m.selected}
		if kind == paletteKindCommand {
			command := m.commands()[hit.index]
			row.left, row.right = command.name, command.detail()
		} else {
			row.left = m.files[hit.index]
		}

		rows = append(rows, side+" "+row.render(inner)+" "+side)
	}

	return rows
}

// paletteRow 는 목록 한 줄이다. 왼쪽이 이름, 오른쪽이 흐린 설명이다.
//
// positions 는 왼쪽과 오른쪽을 이어 붙인 글자에서의 자리다(paletteCommand.label 참고).
// 화면에서는 둘 사이에 채움 칸이 들어가므로 그리기 전에 갈라야 한다.
type paletteRow struct {
	left, right string
	positions   []int
	selected    bool
}

// render 는 행 하나를 inner 칸으로 그린다.
//
// sidebar 의 cell 과 같은 순서다 — 먼저 자르고, 그 다음 색을 입히고, 남은 칸을 채운다.
// 색을 입힌 뒤에는 escape 가 섞여서 폭을 셀 수 없다.
func (r paletteRow) render(inner int) string {
	left := truncateToWidth(sanitizeName(r.left), inner)
	leftWidth := textarea.WidthOf(left)

	// 오른쪽은 붙일 칸이 있을 때만 넣는다. 이름이 먼저다 — statusBar 의 git 표시와 같은 규칙이다.
	right, rightWidth := "", 0
	if r.right != "" {
		right = sanitizeName(r.right)
		rightWidth = textarea.WidthOf(right)

		if leftWidth+2+rightWidth > inner {
			right, rightWidth = "", 0
		}
	}

	pad := strings.Repeat(" ", max(inner-leftWidth-rightWidth, 0))

	// 고른 행은 반전만 쓴다. 색을 섞으면 안쪽의 색 초기화가 반전까지 꺼버린다.
	if r.selected {
		return reverse.Render(left + pad + right)
	}

	leftMatch, rightMatch := r.splitPositions(len(left), len(right))

	return renderMatches(left, leftMatch, lipgloss.NewStyle(), styleMatch) +
		pad +
		renderMatches(right, rightMatch, styleDetail, styleMatch)
}

// splitPositions 는 이어 붙인 자리를 왼쪽 것과 오른쪽 것으로 가른다.
// 잘려나간 자리는 버린다 — 안 보이는 글자를 강조할 수는 없다.
func (r paletteRow) splitPositions(leftLen, rightLen int) (left, right []int) {
	// 이어 붙일 때 사이에 공백 하나가 들어갔다(paletteCommand.label).
	rightAt := len(r.left) + 1

	for _, at := range r.positions {
		switch {
		case at < leftLen:
			left = append(left, at)
		case at >= rightAt && at-rightAt < rightLen:
			right = append(right, at-rightAt)
		}
	}

	return left, right
}

// renderMatches 는 맞은 글자에만 다른 색을 입힌다.
// 붙어 있는 자리는 한 구간으로 묶는다 — 글자마다 escape 를 내면 행이 escape 로 뒤덮인다.
func renderMatches(text string, positions []int, base, match lipgloss.Style) string {
	if len(positions) == 0 {
		return base.Render(text)
	}

	line := []byte(text)
	out := strings.Builder{}
	offset := 0

	// put 은 빈 조각에 색을 입히지 않는다. 빈 문자열에도 escape 가 붙어서 행이 지저분해진다.
	put := func(part string, style lipgloss.Style) {
		if part != "" {
			out.WriteString(style.Render(part))
		}
	}

	for i := 0; i < len(positions); i++ {
		start := positions[i]
		end := start + textarea.GlyphSize(line, start)

		// 이어지는 자리는 한 구간으로 묶는다.
		for i+1 < len(positions) && positions[i+1] == end {
			i++
			end += textarea.GlyphSize(line, positions[i])
		}

		put(text[offset:start], base)
		put(text[start:end], match)
		offset = end
	}

	put(text[offset:], base)

	return out.String()
}

// padTo 는 화면 칸을 채운다.
func padTo(text string, width int) string {
	return text + strings.Repeat(" ", max(width-textarea.WidthOf(text), 0))
}
