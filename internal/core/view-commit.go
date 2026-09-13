package core

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bluemir/zn/internal/textarea"
)

// commitMsg 는 읽어 온 상세다. 어느 커밋의 것인지를 같이 들고 온다 —
// 기다리는 동안 목록으로 돌아갔다 다른 커밋을 열었으면 늦게 온 답은 버려야 한다.
type commitMsg struct {
	hash   plumbing.Hash
	detail commitDetail
	err    error
}

// commitMode 는 `enter` 로 여는 커밋 상세 화면이다. mode 는 `COMMIT` 다.
//
// **부모 화면을 그대로 들고 있다가 `q` 에 돌려준다.** 목록의 고른 자리와 스크롤이 저절로
// 남는다. 확인창이 부모를 드는 것과 같은 손이다(view-quit-confirm.go, ADR-0110).
//
// 읽는 것은 Cmd 로 나간다. tree 를 견주는 일이라 큰 커밋에서는 Update 안에서 할 값이 아니다.
func commitMode(parent tea.Model, e *editor, commit graphCommit) (tea.Model, tea.Cmd) {
	e.clearNotice()

	m := viewCommit{editor: e, parent: parent, commit: commit}

	return m, m.read()
}

// viewCommit 은 커밋 하나의 전문과 건드린 파일 목록이다.
type viewCommit struct {
	*editor

	// parent 는 이 화면을 연 목록이다. `q` 가 그대로 돌려준다.
	parent tea.Model

	// commit 은 목록이 이미 들고 있던 것이다. 짧은 해시와 ref 이름이 여기서 온다.
	commit graphCommit

	// detail 은 읽어 온 것이다. ready 가 서기 전에는 비어 있다.
	detail commitDetail
	ready  bool

	// selected 는 건드린 파일 목록에서 고른 자리다. 파일이 없으면 뜻이 없다.
	//
	// **목록에 커서가 생겼다.** 전에는 이 화면이 통째로 굴러가기만 했는데, 그러면 파일
	// 이름까지 보이고 그 파일이 어떻게 바뀌었는지는 볼 길이 없었다(ADR-0140 §3).
	selected int

	top int // 첫 행
}

// read 는 이 커밋의 상세를 읽어 오는 Cmd 다.
//
// **작업(job) 이 아니라 msg 를 돌려주는 Cmd 다.** 작업은 결과를 editor 에 놓는 틀인데
// (`apply func(*editor)`, job.go) 이것은 지금 보고 있는 화면 하나의 것이라 editor 에 남길
// 값이 없다. 정의를 묻고 답을 받는 자리와 같은 손이다(language-server.go, ADR-0051).
//
// 한 커밋을 읽는 일이라 오래 걸리지 않는다. 진행을 보일 것도 중간에 그만둘 것도 없다.
func (m viewCommit) read() tea.Cmd {
	ctx, hash := m.rootContext(), m.commit.hash

	return func() tea.Msg {
		detail, err := readCommitDetail(ctx, ".", hash)

		return commitMsg{hash: hash, detail: detail, err: err}
	}
}

func (m viewCommit) Init() tea.Cmd { return nil }

func (m viewCommit) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		m.scrollTo()

		return m, nil
	case commitMsg:
		// 기다리는 동안 다른 커밋을 열었으면 늦게 온 답이다. 버린다.
		if msg.hash != m.commit.hash {
			return m, nil
		}

		if msg.err != nil {
			m.notifyError(msg.err)

			return m.close()
		}

		m.detail, m.ready = msg.detail, true
		m.scrollTo()

		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg.String())
	case tea.MouseClickMsg:
		// 왼쪽만 본다. 이 화면은 tabline 을 덮고 있어서 오른쪽 버튼이 닫을 tab 이 없다.
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			m.click(mouse.Y)
		}

		return m, nil
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-wheelRows)
		case tea.MouseWheelDown:
			m.move(wheelRows)
		}

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다. 한글은 되돌린다. 목록과 같다(ADR-0008).
func (m viewCommit) press(key string) (tea.Model, tea.Cmd) {
	m.clearNotice()

	var model tea.Model = m
	for _, expanded := range expandHangul(key) {
		here, ok := model.(viewCommit)
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

// run 은 동작 하나다. 기록을 고치는 키는 없다. 읽고 고르는 화면이다.
//
// **건드린 파일이 있으면 `j`·`k` 가 그 목록을 고른다.** 없으면 전문을 굴리기만 한다.
// 커서를 세울 데가 없는 화면에서 고르는 손을 흉내 내면 키가 안 먹은 것으로 읽힌다.
func (m viewCommit) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "q", "esc":
		return m.close()
	case "j", "down":
		m.move(1)

		return m, nil
	case "k", "up":
		m.move(-1)

		return m, nil
	case "pgdown":
		m.move(textarea.PageRows(textarea.PageFull, m.listHeight()))

		return m, nil
	case "pgup":
		m.move(-textarea.PageRows(textarea.PageFull, m.listHeight()))

		return m, nil
	case "g", "home":
		m.moveTo(0)

		return m, nil
	case "G", "end":
		m.moveTo(len(m.detail.files))

		return m, nil
	case "enter":
		return m.openDiff()
	default:
		return m, nil
	}
}

// openDiff 는 `enter` 다. 고른 파일이 이 커밋에서 어떻게 바뀌었는지를 연다(ADR-0140 §3).
//
// **이 화면을 그대로 넘겨준다.** diff 판에서 `q` 를 치면 고른 자리와 스크롤이 남은 채로
// 돌아온다. 목록이 상세를 열 때와 같은 손이다(view-graph.go).
//
// 옮긴 파일은 왼쪽이 이전 이름이다. 그러지 않으면 옮긴 것이 통째로 지우고 새로 만든 것으로
// 보인다.
func (m viewCommit) openDiff() (tea.Model, tea.Cmd) {
	if !m.ready || len(m.detail.files) == 0 {
		return m, nil
	}

	_, root, _, ok := openGitRepo(".")
	if !ok {
		return m, nil
	}

	request := diffCommitRequest(m.detail, m.commit, m.detail.files[m.selected], root)

	return diffMode(m, m.editor, request)
}

// close 는 목록으로 돌아간다. 목록은 이 화면을 열 때 그대로 넘겨받은 것이다.
func (m viewCommit) close() (tea.Model, tea.Cmd) {
	return m.parent, nil
}

// click 은 누른 화면 행의 파일을 고른다. 파일 줄이 아니면 아무 일도 하지 않는다.
//
// **전문 쪽을 눌러도 고른 것이 움직이지 않는다.** 거기에는 고를 것이 없어서, 가장 가까운
// 파일로 끌어다 붙이면 누른 자리와 골라진 자리가 어긋난다.
//
// 열지는 않는다. 누르는 것은 고르는 일이고 여는 것은 `enter` 다(ADR-0012).
func (m *viewCommit) click(y int) {
	if len(m.detail.files) == 0 {
		return
	}

	row := y - jobsTitleHeight
	if row < 0 || row >= m.listHeight() {
		return
	}

	at := m.top + row - m.fileTop()
	if at < 0 || at >= len(m.detail.files) {
		return
	}

	m.selected = at
	m.scrollTo()
}

// move 는 delta 만큼 옮긴다. 고를 파일이 있으면 고른 자리를, 없으면 화면을 움직인다.
func (m *viewCommit) move(delta int) {
	if len(m.detail.files) == 0 {
		m.top += delta
		m.scrollTo()

		return
	}

	m.moveTo(m.selected + delta)
}

// moveTo 는 고른 파일을 그 자리로 옮긴다. 양끝에서 멈춘다.
func (m *viewCommit) moveTo(at int) {
	if len(m.detail.files) == 0 {
		m.top = 0
		m.scrollTo()

		return
	}

	m.selected = min(max(at, 0), len(m.detail.files)-1)
	m.scrollTo()
}

// fileTop 은 파일 목록의 첫 행이 화면 행 목록에서 몇 번째인지다.
//
// **목록이 늘 맨 아래다**(lines). 그래서 세는 것이 아니라 뒤에서 떼면 된다.
func (m viewCommit) fileTop() int {
	return len(m.lines()) - len(m.detail.files)
}

// scrollTo 는 첫 행을 담긴 것 안으로 맞춘다. 고른 파일이 있으면 그것이 보이게 한다.
//
// 마지막 행이 화면 맨 아래에 오는 자리가 끝이다. 편집 영역과 같은 한계다.
func (m *viewCommit) scrollTo() {
	rows, height := len(m.lines()), m.listHeight()

	m.top = max(0, min(m.top, max(rows-height, 0)))

	if len(m.detail.files) == 0 || height < 1 {
		return
	}

	m.selected = min(max(m.selected, 0), len(m.detail.files)-1)

	at := m.fileTop() + m.selected
	if at < m.top {
		m.top = at
	}
	if at >= m.top+height {
		m.top = at - height + 1
	}
}

func (m viewCommit) View() tea.View {
	lines := m.lines()
	height := m.listHeight()

	selected := m.fileTop() + m.selected

	body := make([]string, 0, height)
	for i := m.top; i < len(lines) && len(body) < height; i++ {
		// **고른 줄에는 색을 얹지 않는다.** 색을 켜고 끄는 escape 가 안에 있으면 그 자리에서
		// 반전이 끊겨 줄이 얼룩덜룩해진다. 고른 줄이 무엇인지는 반전 하나로 이미 다 말한다
		// (view-graph.go 의 renderCommit 과 같은 자리다).
		if len(m.detail.files) > 0 && i == selected {
			row := m.renderFile(m.selected, true)
			body = append(body, reverse.Render(padTo(truncateToWidth(row, m.width), m.width)))

			continue
		}

		body = append(body, truncateToWidth(lines[i], m.width))
	}

	for len(body) < height {
		body = append(body, "")
	}

	screen := append([]string{m.renderTitle()}, body...)
	screen = append(screen, styleDetail.Render(truncateToWidth(" "+m.hint(), m.width)))
	screen = append(screen, m.bareStatusBar("COMMIT", m.notice)...)

	view := newView(screen, m.renderWindowTitle())

	// 커서는 고른 파일의 왼쪽 끝이다. 건드린 파일이 없으면 고를 것이 없어서 두지 않는다 —
	// 첫 행 왼쪽 끝에 두면 그 줄이 골라진 것처럼 보인다.
	if len(m.detail.files) > 0 && selected >= m.top && selected < m.top+height {
		view.Cursor = tea.NewCursor(0, selected-m.top+jobsTitleHeight)
		view.Cursor.Shape = tea.CursorBlock
	} else {
		view.Cursor = nil
	}

	return view
}

// hint 는 아래 줄의 키 안내다. 고를 파일이 없으면 고르는 손을 적지 않는다.
func (m viewCommit) hint() string {
	if len(m.detail.files) == 0 {
		return "j/k 스크롤  q 목록으로"
	}

	return "j/k 파일  enter diff  q 목록으로"
}

// renderTitle 은 맨 윗줄이다. 짧은 해시와 제목이다.
func (m viewCommit) renderTitle() string {
	label := "커밋  " + m.commit.short + "  " + m.commit.subject

	return reverse.Width(m.width).Render(truncateToWidth(label, m.width))
}

// lines 는 화면에 그릴 행 전부다. `git show --name-status --format=fuller` 와 같은 차례다.
//
//	commit ca5e330a1b2c3d4e5f60718293a4b5c6d7e8f901 (HEAD → master)
//	Merge: 80830bf d0a8bd7
//	Author: BlueMir <hyeokjun.ahn@navercorp.com>
//	Date:   Tue, 1 Sep 2026 11:26:05 +0900
//
//	    feature: 명령줄의 `%` 를 지금 보고 있는 파일로 편다
//
//	M  internal/core/shell.go
//
// `Merge:` 는 부모가 둘 이상일 때만 선다. `Commit:`·`CommitDate:` 는 커밋한 사람이 쓴
// 사람과 다를 때만 선다 — rebase 나 남의 패치를 받은 자리다. 늘 적으면 같은 값이 두 번
// 적히는 줄이 커밋마다 둘씩 는다.
func (m viewCommit) lines() []string {
	if !m.ready {
		return []string{" 읽는 중입니다"}
	}

	lines := []string{" commit " + styleCommitHash.Render(m.commit.hash.String()) +
		styleCommitRefs.Render(renderRefs(m.commit.refs))}

	if len(m.detail.parents) > 1 {
		lines = append(lines, " Merge: "+styleCommitHash.Render(strings.Join(m.detail.parents, " ")))
	}

	lines = append(lines,
		" Author: "+signatureLine(m.detail.author),
		" Date:   "+styleCommitDate.Render(m.detail.author.When.Format(graphDateFormat)),
	)

	if m.detail.committer.Name != m.detail.author.Name ||
		m.detail.committer.Email != m.detail.author.Email ||
		!m.detail.committer.When.Equal(m.detail.author.When) {
		lines = append(lines,
			" Commit:     "+signatureLine(m.detail.committer),
			" CommitDate: "+styleCommitDate.Render(m.detail.committer.When.Format(graphDateFormat)),
		)
	}

	lines = append(lines, "")
	for _, line := range strings.Split(m.detail.message, "\n") {
		lines = append(lines, "     "+line)
	}

	if len(m.detail.files) == 0 {
		return lines
	}

	lines = append(lines, m.renderStat())
	for i := range m.detail.files {
		lines = append(lines, m.renderFile(i, false))
	}

	return lines
}

// renderStat 은 파일 목록 **위**에 서는 합계 한 줄이다.
//
// git 은 목록 아래에 적는데 우리는 위다. 목록이 길면 아래는 화면 밖이라, 먼저 보이는 자리에
// 있어야 「이 커밋이 얼마나 큰가」를 묻는 눈에 닿는다(ADR-0142).
func (m viewCommit) renderStat() string {
	added, removed := 0, 0
	for _, file := range m.detail.files {
		added, removed = added+file.added, removed+file.removed
	}

	return " 파일 " + formatCount(len(m.detail.files)) + " 개  " +
		styleGitAdded.Render("+"+formatCount(added)) + "  " +
		styleGitRemoved.Render("-"+formatCount(removed))
}

// renderFile 은 목록 한 줄이다. 이름이 왼쪽이고 줄 수가 오른쪽 끝이다.
//
// **이름 앞이 아니라 오른쪽 끝이다.** 앞에 두면 자릿수가 파일마다 달라 이름이 들쭉날쭉해진다.
// 오른쪽에 세우면 수끼리 한 줄로 서서 세로로 훑을 수 있다. 트리가 `M`·`?` 를 오른쪽 끝에
// 세운 것과 같은 손이다(ADR-0094 §6, ADR-0142).
//
// plain 은 고른 줄이다. 반전 위에 색을 얹으면 그 자리에서 반전이 끊겨 줄이 얼룩덜룩해진다.
func (m viewCommit) renderFile(at int, plain bool) string {
	file := m.detail.files[at]
	added, removed := m.countWidths()

	left := " " + file.label()
	counts := file.counts(added, removed)

	// 이름이 길면 수를 밀어내지 않고 이름을 자른다. 수는 늘 오른쪽 끝에 선다.
	room := max(m.width-textarea.WidthOf(counts)-1, 0)
	left = padTo(truncateToWidth(left, room), room)

	if plain {
		return left + " " + counts
	}

	return styleCommitFile(file.action).Render(left) + " " + m.paintCounts(file, counts)
}

// paintCounts 는 넣은 수를 초록, 들어낸 수를 빨강으로 칠한다. git 마커와 같은 색이다.
//
// 이진 파일은 수가 아니라 말이라 흐린 회색이다.
func (m viewCommit) paintCounts(file commitFile, counts string) string {
	if file.binary {
		return styleDetail.Render(counts)
	}

	at := strings.LastIndex(counts, " ")

	return styleGitAdded.Render(counts[:at]) + styleGitRemoved.Render(counts[at:])
}

// countWidths 는 넣은 수와 들어낸 수가 잡을 폭이다. 목록에서 가장 긴 것에 맞춘다.
//
// 부호와 자리 구분 쉼표까지 든 글자 수다. 이진 파일은 수가 아니라 말이라 세지 않는다.
func (m viewCommit) countWidths() (int, int) {
	added, removed := 0, 0

	for _, file := range m.detail.files {
		if file.binary {
			continue
		}

		added = max(added, len(file.addedText()))
		removed = max(removed, len(file.removedText()))
	}

	return added, removed
}

// styleCommitFile 은 건드린 파일 한 줄의 색이다. git 마커와 같은 색을 쓴다(ADR-0094).
//
// 넣은 것이 초록, 고친 것이 파랑, 지운 것이 빨강이다. 왼쪽 칸의 `+`·`~`·`_` 와 같은 색이라
// 「같은 것을 가리킨다」가 색으로 이어진다. 옮긴 것은 그 셋 어디도 아니라 흐린 회색이다.
func styleCommitFile(action string) lipgloss.Style {
	switch action {
	case "A":
		return styleGitAdded
	case "M":
		return styleGitModified
	case "D":
		return styleGitRemoved
	default:
		return styleDetail
	}
}

// signatureLine 은 `이름 <메일>` 이다. git 과 같은 모양이다.
func signatureLine(who object.Signature) string {
	return who.Name + " <" + who.Email + ">"
}
