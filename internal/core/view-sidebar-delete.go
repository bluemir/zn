package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/textarea"
)

// removeTreeEntry 는 정말 지운다. 곧바로 지우는 파일과 확인을 거친 디렉터리가 같이 쓴다.
//
// 지울 수 있는 자리인지는 이미 걸러져 있다(viewSidebar.deleteFile). 여기서는 지우고,
// 그 자리를 다시 읽고, 무엇이 사라졌는지 남기는 것만 한다.
//
// 지운 뒤에도 트리에 머문다. 파일을 만들고 지우는 것은 몇 번을 이어서 하는 일이라, 결과마다
// 다른 화면으로 내보내면 그 되풀이가 끊긴다(ADR-0054).
func removeTreeEntry(e *editor, path string, isDir bool) (tea.Model, tea.Cmd) {
	var err error
	if isDir {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		return sidebarModeError(e, errors.Mark(err, errRemoveFile))
	}

	label := e.sidebar.relLabel(path)
	if isDir {
		label += "/"
	}

	// 지운 파일을 열어둔 tab 을 닫는다. **지운 뒤다** — 지우기가 실패하면 파일이 그대로
	// 있으므로 tab 도 그대로 두어야 한다(ADR-0130).
	//
	// 드러난 파일 자리로 트리를 데려가지 않는다. 고른 자리는 지운 자리이고 트리에 머무는
	// 것이 이 동작이라(ADR-0054), reveal 을 걸면 고른 자리가 남의 파일로 옮겨간다.
	closed := e.closeTabsUnder(path, isDir)

	// 지운 자리를 다시 읽는다. 고른 자리는 번호로 들고 있어서 한 행이 빠지면 그 아래 항목이
	// 그 자리에 올라온다 — 편집 영역에서 `dd` 뒤에 커서가 다음 줄에 서는 것과 같다.
	refresh := e.refreshDir(filepath.Dir(path))

	// 파일은 묻지 않고 지우므로 이 알림이 무엇이 사라졌는지 말하는 유일한 자리다(ADR-0057).
	// 닫은 tab 도 여기서 말한다. tabline 에서 사라진 것을 화면만 보고는 지우기가 한 일인지
	// 알 수 없다. 지나간 것은 `:messages` 에 남는다(ADR-0053, ADR-0130).
	message := "지웠습니다: " + label
	if closed > 0 {
		message += fmt.Sprintf(" (tab %d 개를 닫았습니다)", closed)
	}

	model, cmd := sidebarModeMessage(e, message)

	// 지운 것이 git 이 아는 파일이었으면 저장소 상태가 달라진다(ADR-0030).
	return model, tea.Batch(cmd, refresh, e.startGitRefresh())
}

// deleteConfirmMode 는 지우기 전에 한 번 더 묻는 modal 이다. 트리에서 `md` 로 들어온다.
//
// **묻는 자리가 이 창 하나다.** 깨끗한 파일은 여기 오지 않고 곧바로 지워지지만(ADR-0057),
// 물을 일이 있는 것은 셋 다 이 창으로 온다 — 디렉터리, 저장하지 않은 tab 이 딸린 파일,
// 그 둘이 겹친 것이다. 전에는 디렉터리만 statusBar 아래 줄에서 묻고 저장하지 않은 tab 은
// 종료 확인창이 물어서, 같은 `md` 가 무엇을 골랐는지에 따라 다른 자리에 나타났다(ADR-0130).
//
// statusBar 아래 줄에서 modal 로 올라온 까닭은 그 한 줄이 좁아서다. 편집 영역 폭뿐이라
// 80 칸 터미널에서 47 칸이고, 잃는 것이 둘(안의 것, 저장하지 않은 tab) 이면 이름이 잘린다.
//
// 트리의 고른 행이 가려지지는 않는다. 창은 화면 가운데에 뜨고 부모(트리) 의 화면을 그대로
// 깔고 그리므로 커서가 지울 행에 그대로 남는다 — 무엇을 지우는지 두 곳에서 맞춰 볼 수 있다.
//
// 노드 포인터가 아니라 경로를 든다. 묻는 사이에 디렉터리 읽기가 끝나면 그 포인터는 이미
// 어느 화면에도 없을 수 있다 — readDirJob 의 apply 가 경로로 다시 찾는 것과 같은 이유다.
func deleteConfirmMode(parent tea.Model, e *editor, path string, isDir bool, dirty int) (tea.Model, tea.Cmd) {
	return viewSidebarDelete{editor: e, parent: parent, path: path, isDir: isDir, dirty: dirty}, nil
}

type viewSidebarDelete struct {
	*editor

	parent tea.Model // 무르면 돌아갈 화면. 트리다
	path   string    // 지울 것의 절대 경로
	isDir  bool      // 안의 것까지 지우는지
	dirty  int       // 같이 닫히는, 저장하지 않은 tab 의 수
	cursor int       // 0 이 Yes, 1 이 No
}

func (m viewSidebarDelete) Init() tea.Cmd { return nil }

func (m viewSidebarDelete) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져 트리가 숨으면 무엇을 지우는지 보여줄 자리가 없어진다.
		// 묻던 것을 그만두고 편집 영역으로 내보낸다.
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}
		m.sidebar.scrollTo(m.sidebarHeight())

		return m, nil
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다(ADR-0008).
		// 종료 확인창과 같은 방식이다 — 음절 하나가 키 여럿으로 풀리므로 차례로 먹인다.
		keys := hangulKeys(msg.String())
		if keys == nil {
			return m.press(msg.String())
		}

		var model tea.Model = m
		for _, key := range keys {
			confirm, ok := model.(viewSidebarDelete)
			if !ok {
				// 앞의 키에서 창을 벗어났다. 남은 키는 버린다.
				return model, nil
			}

			next, cmd := confirm.press(key)
			if cmd != nil {
				return next, cmd
			}

			model = next
		}

		return model, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go).
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나를 먹는다.
//
// **`y` 는 고르기만 하고 `enter` 가 실행한다.** 종료 확인창과 같은 손이다. 전에는 `y` 한 키가
// 곧 실행이었는데(ADR-0057), 창이 화면 가운데에 뜨는 지금은 무엇을 묻는지 읽고 고르는 자리라
// 키 하나로 끝나지 않는 편이 맞다 — 잘못 누른 `y` 가 디렉터리를 지우지 않는다(ADR-0130).
//
// `ctrl+c` 는 여기서 취소다. 편집기를 끄는 것이 아니라 묻던 것을 무르는 것이다 —
// 이미 나가는 길에 뜨는 종료 확인창과 갈리는 자리다(ADR-0054).
func (m viewSidebarDelete) press(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "y":
		m.cursor = 0

		return m, nil
	case "right", "n":
		m.cursor = 1

		return m, nil
	case "esc", "ctrl+c":
		return sidebarModeMessage(m.editor, "지우지 않았습니다")
	case "enter":
		// 고른 자리는 left/right 로만 움직여서 0(Yes) 아니면 1(No) 이다.
		if m.cursor == 0 {
			return removeTreeEntry(m.editor, m.path, m.isDir)
		}

		return sidebarModeMessage(m.editor, "지우지 않았습니다")
	default:
		return m, nil
	}
}

// lines 는 창에 적는 줄들이다. 폭을 재는 자리와 그리는 자리가 같은 글을 보아야 한다.
// 빈 문자열은 빈 줄이다.
//
// **잃는 것이 이름보다 앞에 온다.** 이름만 보고 Yes 를 고르는 것을 막는 것이 이 창이 남은
// 까닭이다(ADR-0057). 디렉터리는 안의 것이, 열어둔 tab 이 있으면 그 편집이 잃는 것이다.
func (m viewSidebarDelete) lines() []string {
	label := m.sidebar.relLabel(m.path)
	if m.isDir {
		label += "/"
	}

	lines := []string{"지우면 되돌릴 수 없습니다.", ""}
	if m.isDir {
		lines = append(lines, "안의 것까지 모두 지웁니다.")
	}
	if m.dirty > 0 {
		lines = append(lines, fmt.Sprintf("저장하지 않은 tab %d 개가 같이 닫힙니다.", m.dirty))
	}

	return append(lines, "", label+" 를 지울까요?")
}

func (m viewSidebarDelete) boxWidth() int {
	maxContent := textarea.WidthOf(pickMark(true, "Yes") + "    " + pickMark(false, "No"))
	for _, line := range m.lines() {
		if w := textarea.WidthOf(line); w > maxContent {
			maxContent = w
		}
	}

	wanted := maxContent + 8
	if m.width <= 0 {
		return max(wanted, 40)
	}

	return min(max(wanted, 40), max(m.width-2, 10))
}

func (m viewSidebarDelete) renderBox(width int) string {
	inner := width - 4 // 좌우 테두리 2 + 좌우 1칸 공백
	chars := m.boxChars
	edge := strings.Repeat(chars.horizontal, width-2)

	// 빈 줄도 같은 손으로 그린다. padTo 가 안쪽을 공백으로 채운다.
	row := func(text string) string {
		return chars.vertical + " " + padTo(truncateToWidth(text, inner), inner) + " " + chars.vertical
	}

	rows := []string{chars.topLeft + edge + chars.topRight, row("")}
	for _, line := range m.lines() {
		rows = append(rows, row(line))
	}

	rows = append(rows,
		row(""),
		row(pickMark(m.cursor == 0, "Yes")+"    "+pickMark(m.cursor == 1, "No")),
		row(""),
		chars.bottomLeft+edge+chars.bottomRight,
	)

	return strings.Join(rows, "\n")
}

func (m viewSidebarDelete) View() tea.View {
	width := m.boxWidth()
	box := m.renderBox(width)
	boxRows := strings.Split(box, "\n")

	left := max((m.width-width)/2, 0)
	top := max((m.height-len(boxRows))/2, 0)

	// 부모의 view 를 그대로 쓰고 내용만 갈아끼운다. 종료 확인창과 같은 자리다(ADR-0110).
	//
	// 부모가 트리라서 커서가 지울 행에 그대로 남는다. 창이 이름을 말하고 커서가 그 자리를
	// 가리켜서 무엇을 지우는지 두 곳에서 맞춰 볼 수 있다(ADR-0054).
	view := m.parent.View()

	view.Content = lipgloss.NewCompositor(
		lipgloss.NewLayer(view.Content).Z(0),
		lipgloss.NewLayer(box).X(left).Y(top).Z(1),
	).Render()

	return view
}
