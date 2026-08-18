package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"
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

// styleMatch 는 fuzzy 매칭으로 맞은 글자다.
//
// 검색 강조(styleSearchMatch) 를 쓰지 않는다 — 그것은 "파일 안에서 찾은 것" 이라는 뜻이
// 이미 붙었다. 반전도 못 쓴다. statusBar·tabline(ADR-0004) 과 이 목록의 고른 행이 쓰고 있다.
var (
	styleMatch  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	styleDetail = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// viewPalette 는 `ctrl+p` 로 여는 command palette 다.
//
// 화면 위쪽 가운데에 박스를 띄우고 그 아래로 편집 화면이 그대로 비친다. 기본은 파일 찾기이고
// 입력이 `>` 로 시작하면 명령 목록이 된다.
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
		return normalModeMessage(e, errors.Cause(err).Error())
	}

	// 목록은 백그라운드 작업이 채운다. 열 때마다 새로 읽는 것은 그대로이고(ADR-0011),
	// 다 읽을 때까지 기다리지 않을 뿐이다. 이미 돌고 있으면 그것에 붙는다.
	//
	// 앞서 모아둔 목록이 있으면 그것을 보면서 시작한다. 새 조각이 오면 통째로 갈린다 —
	// 빈 목록에서 시작하면 열 때마다 화면이 한 번 번쩍인다.
	cmd := e.startJob("파일 인덱싱", func(ctx context.Context) <-chan jobProgress {
		return indexFiles(ctx, root)
	})

	m := viewPalette{editor: e}
	m.filter()

	return m, cmd
}

type viewPalette struct {
	*editor

	// input 은 친 그대로다. 맨 앞의 `>` 도 지우지 않고 들고 있다 — 그것이 곧 어느 표를 보는지다.
	input string

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
		case "ctrl+p":
			// 여는 키를 다시 눌러도 아무 일도 하지 않는다. 이동은 화살표뿐이다.
			return m, nil
		case "backspace":
			// command·search mode 와 같이 다 지우면 나간다.
			if m.input == "" {
				return normalMode(m.editor)
			}
			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]
			m.filter()

			return m, nil
		default:
			// 한글은 여기서 글자다. 입력줄이 있는 mode 는 키를 되돌리지 않는다(ADR-0008).
			if msg.Text == "" {
				return m, nil
			}
			m.input += msg.Text
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
	case jobProgressMsg, jobDoneMsg, gitTickMsg:
		// 다른 mode 와 같이 공용 처리에 넘기고, 여기서만 목록을 다시 거른다.
		// 인덱싱이 도는 동안 목록이 길어지므로 새로 온 파일도 치고 있는 패턴에 걸려야 한다.
		cmd := m.handleJob(msg)
		m.refilter()

		return m, cmd
	default:
		return m, nil
	}
}

// commandInput 은 명령 목록을 고르는 중인지와 `>` 를 뗀 검색어를 준다.
//
// `>` 는 별도 키도 상태도 아니다. 지우는 순간 파일 목록으로 저절로 돌아온다.
func (m viewPalette) commandInput() (string, bool) {
	rest, ok := strings.CutPrefix(m.input, ">")
	if !ok {
		return m.input, false
	}

	return strings.TrimLeft(rest, " "), true
}

// labels 는 지금 표에서 매칭 대상이 되는 글자들이다.
func (m viewPalette) labels() []string {
	if _, ok := m.commandInput(); !ok {
		return m.files
	}

	labels := make([]string, 0, len(paletteCommands))
	for _, command := range paletteCommands {
		labels = append(labels, command.label())
	}

	return labels
}

// filter 는 입력으로 목록을 다시 거른다. 입력이 바뀌었으므로 고른 자리는 처음으로 돌아간다.
func (m *viewPalette) filter() {
	pattern, _ := m.commandInput()

	m.hits = filterPalette(pattern, m.labels())
	m.selected, m.top = 0, 0
}

// refilter 는 후보가 늘었을 때 다시 거른다. filter 와 달리 고른 자리를 그대로 둔다.
//
// 인덱싱이 파일을 부을 때마다 맨 위로 튀면 목록을 훑을 수 없다. 새로 온 파일이 위로 끼어들면
// 커서가 가리키는 항목은 바뀔 수 있지만, 그것이 매번 처음으로 돌아가는 것보다 낫다.
func (m *viewPalette) refilter() {
	pattern, _ := m.commandInput()

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
	if len(m.hits) == 0 {
		return m, nil
	}

	index := m.hits[m.selected].index

	if _, ok := m.commandInput(); ok {
		return paletteCommands[index].run(m.editor)
	}

	return m.openFile(m.files[index])
}

// openFile 은 고른 파일을 tab 으로 연다. sidebar 의 enter 와 같은 길이다.
func (m viewPalette) openFile(path string) (tea.Model, tea.Cmd) {
	// 목록을 읽은 뒤에 지워졌을 수 있다. Stat 은 symlink 를 따라가므로 가리키는 것이 무엇인지로 본다.
	// FIFO 나 소켓은 ReadFile 이 영영 돌아오지 않아서 편집기가 통째로 멈춘다.
	info, err := os.Stat(path)
	if err != nil {
		return normalModeMessage(m.editor, errors.Cause(err).Error())
	}
	if !info.Mode().IsRegular() {
		return normalModeMessage(m.editor, "일반 파일이 아닙니다: "+path)
	}

	reveal, err := m.openTab(path)
	if err != nil {
		return normalModeMessage(m.editor, errors.Cause(err).Error())
	}
	m.activeBuffer().scrollTo(m.contentWidth(), m.textHeight())

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
	view := m.render(tea.CursorBar, "PALETTE", m.counter())

	// 편집 화면을 다 그린 뒤 그 위에 박스를 얹는다. 셀 단위라 두 칸 글자와 색이 어긋나지 않는다.
	// 합성은 팔레트에서만 태운다 — 셀 버퍼를 지나면 줄 끝의 빈 칸이 잘려서 화면 문자열이 달라진다.
	left := m.paletteLeft()
	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(m.box()).X(left).Y(paletteTop).Z(1),
	).Render()

	// 커서는 편집 내용이 아니라 박스 안 입력줄에 있어야 한다.
	view.Cursor = tea.NewCursor(left+2+screenWidthOf(m.input), paletteTop+1)
	view.Cursor.Shape = tea.CursorBar

	return view
}

// counter 는 statusBar 아래 줄이다. 몇 개 중 몇 개가 걸렸는지 보여준다.
func (m viewPalette) counter() string {
	return fmt.Sprintf("%d/%d", len(m.hits), len(m.labels()))
}

// box 는 박스 전체를 화면 행 문자열로 만든다. 각 행이 정확히 paletteWidth() 칸이다.
//
// 테두리는 lipgloss 의 Border 를 쓰지 않고 손으로 붙인다. Border 는 안쪽 내용의 폭을 스스로
// 재는데 강조 escape 가 이미 섞여 있어서 그 계산을 믿을 수 없다. sidebar 가 구분선을 손으로
// 붙이는 것과 같은 이유다.
func (m viewPalette) box() string {
	width := m.paletteWidth()
	inner := width - 4 // 테두리 둘과 좌우 한 칸씩

	chars := m.boxChars
	line := strings.Repeat(chars.horizontal, width-2)

	rows := []string{
		chars.topLeft + line + chars.topRight,
		m.inputRow(inner),
		chars.leftTee + line + chars.rightTee,
	}
	rows = append(rows, m.listRows(inner)...)
	rows = append(rows, chars.bottomLeft+line+chars.bottomRight)

	return strings.Join(rows, "\n")
}

// inputRow 는 치고 있는 것을 보여주는 줄이다. 비어 있으면 무엇을 치면 되는지 흐리게 알려준다.
func (m viewPalette) inputRow(inner int) string {
	side := m.boxChars.vertical

	if m.input == "" {
		return side + " " + styleDetail.Render(padTo(truncateToWidth("파일 찾기. > 로 명령", inner), inner)) + " " + side
	}

	return side + " " + padTo(truncateToWidth(m.input, inner), inner) + " " + side
}

// listRows 는 목록 행들이다. 걸린 것이 없으면 그 사실을 한 줄로 알린다.
func (m viewPalette) listRows(inner int) []string {
	side := m.boxChars.vertical

	if len(m.hits) == 0 {
		return []string{side + " " + styleDetail.Render(padTo("일치하는 것이 없습니다", inner)) + " " + side}
	}

	_, isCommand := m.commandInput()

	rows := []string{}
	for i := m.top; i < len(m.hits) && len(rows) < m.paletteListRows(); i++ {
		hit := m.hits[i]

		row := paletteRow{positions: hit.positions, selected: i == m.selected}
		if isCommand {
			command := paletteCommands[hit.index]
			row.left, row.right = command.name, command.detail()
		} else {
			row.left = m.files[hit.index]
		}

		rows = append(rows, side+" "+row.cell(inner)+" "+side)
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

// cell 은 행 하나를 inner 칸으로 그린다.
//
// sidebar 의 cell 과 같은 순서다 — 먼저 자르고, 그 다음 색을 입히고, 남은 칸을 채운다.
// 색을 입힌 뒤에는 escape 가 섞여서 폭을 셀 수 없다.
func (r paletteRow) cell(inner int) string {
	left := truncateToWidth(sanitizeName(r.left), inner)
	leftWidth := screenWidthOf(left)

	// 오른쪽은 붙일 칸이 있을 때만 넣는다. 이름이 먼저다 — statusBar 의 git 표시와 같은 규칙이다.
	right, rightWidth := "", 0
	if r.right != "" {
		right = sanitizeName(r.right)
		rightWidth = screenWidthOf(right)

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

	return highlightMatches(left, leftMatch, lipgloss.NewStyle(), styleMatch) +
		pad +
		highlightMatches(right, rightMatch, styleDetail, styleMatch)
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

// highlightMatches 는 맞은 글자에만 다른 색을 입힌다.
// 붙어 있는 자리는 한 구간으로 묶는다 — 글자마다 escape 를 내면 행이 escape 로 뒤덮인다.
func highlightMatches(text string, positions []int, base, match lipgloss.Style) string {
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
		end := start + clusterSize(line, start)

		// 이어지는 자리는 한 구간으로 묶는다.
		for i+1 < len(positions) && positions[i+1] == end {
			i++
			end += clusterSize(line, positions[i])
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
	return text + strings.Repeat(" ", max(width-screenWidthOf(text), 0))
}
