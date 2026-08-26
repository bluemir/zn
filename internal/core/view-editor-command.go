package core

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/buildinfo"
)

// viewEditorCommand 는 vim 의 command-line mode 다. normal 에서 `:` 로 들어간다.
//
// 치고 있는 명령 문자열은 이 mode 에만 있는 상태다.
// mode 를 model 로 나눈 덕에 다른 mode 가 이 필드를 이고 다니지 않는다(ADR-0002).
func commandMode(e *editor) (tea.Model, tea.Cmd) {
	return viewEditorCommand{editor: e}, nil
}

type viewEditorCommand struct {
	*editor

	input string // `:` 뒤에 친 것
}

func (m viewEditorCommand) Init() tea.Cmd { return nil }

func (m viewEditorCommand) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return normalMode(m.editor)
		case "enter":
			return m.run()
		case "backspace":
			// vim 처럼 `:` 까지 지우면 명령줄에서 나간다.
			if m.input == "" {
				return normalMode(m.editor)
			}
			m.input = m.input[:prevClusterStart([]byte(m.input), 0, len(m.input))]

			return m, nil
		default:
			if msg.Text == "" {
				return m, nil
			}
			m.input += msg.Text

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 명령을 치는 동안에도 화면은 둘러볼 수 있다.
		// 클릭은 받지 않는다 — 치던 명령이 클릭 한 번에 조용히 사라지면 안 된다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, lspTickMsg, goplsReadyMsg, definitionMsg, referencesMsg, renameMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go). 파일 검사 tick 은
		// 여기서 보지 않고 주기만 이어 간다 — 보는 것은 normal·트리다(ADR-0038).
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

// run 은 친 명령을 실행한다.
func (m viewEditorCommand) run() (tea.Model, tea.Cmd) {
	cmd, err := parseCommand(m.input)
	if err != nil {
		return normalModeError(m.editor, err)
	}

	// 인자를 받는 명령은 이들뿐이다. 나머지에 붙은 인자를 조용히 버리면
	// `:qa foo` 가 foo 에 무언가를 한 것처럼 보인다.
	//
	// `:!` 의 인자는 파일 이름이 아니라 뜯지 않은 셸 줄이고, tokenRest 가 그것을 한 토큰으로
	// 주므로 아래의 「하나만」 가드에는 걸릴 수 없다. `:grep` 과 `:s` 도 같다.
	//
	// `:rename` 의 인자만 파일 이름이 아니다 — 새 이름 하나다(ADR-0067).
	switch cmd.name {
	case "w", "e", "tabnew", "!", "rename", "grep", "s", "substitute":
	default:
		if len(cmd.args) > 0 {
			return normalModeMessage(m.editor, "알 수 없는 명령: "+m.input)
		}
	}
	// 파일 이름 하나만 받는다. 여럿을 tab 여러 개로 여는 것은 CLI 인자의 몫이다.
	//
	// `:grep`·`:s`·`:!` 의 인자는 파일 이름이 아니라 뜯지 않은 한 줄이라 tokenRest 가
	// 늘 한 토큰으로 주므로 여기 걸릴 수 없다(ADR-0045, ADR-0077, ADR-0084).
	if len(cmd.args) > 1 {
		return normalModeMessage(m.editor, "파일은 하나만 쓸 수 있습니다")
	}

	// 줄 범위를 받는 것은 이들뿐이다. 이름 없는 것(`:5`) 은 그 줄로 가는 것이다.
	// 나머지에 붙은 범위를 조용히 버리면 `:1,5w` 가 그 줄만 쓴 것처럼 보인다.
	switch cmd.name {
	case "d", "y", "s", "substitute", "":
	default:
		if cmd.lines != (lineRange{}) {
			return normalModeMessage(m.editor, "이 명령은 줄 범위를 받지 않습니다: "+m.input)
		}
	}

	// 볼 파일이 있어야 하는 명령들이다. 빈 화면에서는 알리고 물러난다 — 이름을 대고 친
	// 것이라 조용하면 편집기가 먹지 않는 것으로 읽힌다(refuseNoBuffer, ADR-0064).
	//
	// 인자 없는 `:e` 는 다시 읽는 것이라 여기 든다. 인자가 있으면 새로 여는 것이라 지나간다.
	switch cmd.name {
	case "d", "y", "s", "substitute", "w", "wq", "x":
		if m.refuseNoBuffer() {
			return normalMode(m.editor)
		}
	case "":
		if cmd.lines != (lineRange{}) && m.refuseNoBuffer() {
			return normalMode(m.editor)
		}
	case "e":
		if len(cmd.args) == 0 && m.refuseNoBuffer() {
			return normalMode(m.editor)
		}
	}

	switch cmd.name {
	case "":
		// 범위만 쳤으면 그 줄로 간다. 아무것도 안 쳤으면 그냥 나간다.
		if cmd.lines == (lineRange{}) {
			return normalMode(m.editor)
		}

		return m.goToLine(cmd)
	case "d":
		// 읽기 전용 파일은 고치지 않는다(readonly.go). `:y` 는 파일을 건드리지 않아 지나간다.
		if m.refuseReadOnly() {
			return normalMode(m.editor)
		}

		return m.deleteLines(cmd)
	case "y":
		return m.yankLines(cmd)
	case "s", "substitute":
		// `:y` 와 달리 파일을 고치므로 읽기 전용은 여기서 걸린다(readonly.go).
		if m.refuseReadOnly() {
			return normalMode(m.editor)
		}

		return m.substitute(cmd)
	case "w":
		return m.write(cmd)
	case "wq", "x":
		var (
			err  error
			note string
		)

		buf := m.activeBuffer()

		// **나가는 길에서는 설치를 묻지 않는다.** 깔려 있지 않으면(errHookNotInstalled) 맞추지
		// 않고 그냥 쓴다 — 여기서 창을 띄우면 tab 을 닫는 것과 겹친다. 다음 `:w` 가 묻는다
		// (ADR-0065).
		hook, _ := m.saveHookFor(buf.path)
		if cmd.force {
			note, err = buf.SaveForce(m.contentWidth(), hook)
		} else {
			note, err = buf.Save(m.contentWidth(), hook)
		}
		if err != nil {
			return normalModeError(m.editor, err)
		}

		// `.editorconfig` 를 따라 맞춘 것이 있으면 알린다(ADR-0052). 마지막 tab 이었어도
		// 보인다 — 닫은 자리에 빈 화면이 남고 그 아래 줄이 알림 자리다(ADR-0064).
		// note 는 맞출 것이 없었으면 빈 문자열이다. 빈 알림을 세우지 않는다 —
		// notify 가 빈 것을 조용히 걸러 주지 않는 것은 일부러다(notice.go).
		if note != "" {
			m.notify(note)
		}

		// 저장은 dirty 를 바꾸는 유일한 편집기 안의 동작이라 주기 갱신을 기다리지 않는다(ADR-0030).
		model, close := forceCloseTab(m.editor)

		return model, tea.Batch(close, m.startGitRefresh())
	case "e":
		return m.edit(cmd)
	case "tabnew":
		// 인자가 없으면 이름 없는 빈 tab 이다.
		if len(cmd.args) == 0 {
			m.newTab()

			return normalMode(m.editor)
		}

		// 이미 열려 있으면 새 tab 을 만들지 않고 그 tab 으로 옮겨간다(ADR-0021).
		reveal, err := m.openTab(cmd.args[0])
		if err != nil {
			return normalModeError(m.editor, err)
		}
		m.arrive()

		model, next := normalMode(m.editor)

		// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
		return model, tea.Batch(next, m.startGitRefresh(), reveal)
	case "!":
		// tokenRest 가 뒤를 한 토큰으로 준다. `:!` 만 쳤으면 args 가 비어 있다.
		if len(cmd.args) == 0 {
			return normalModeMessage(m.editor, "셸 명령이 없습니다")
		}

		return runShell(m.editor, cmd.args[0])
	case "grep":
		// 패턴을 대지 않았으면 박스에서 받는다. 팔레트로 들어올 때와 같은 자리다 —
		// 어느 길로 왔는지는 친 사람이 알고 무엇을 치는지는 같다(ADR-0077, ADR-0078 §7).
		if len(cmd.args) == 0 {
			return grepInputMode(m.editor)
		}

		return runGrep(m.editor, cmd.args[0])
	case "rename":
		// 이름을 대지 않았으면 커서 옆 창에서 받는다. `\rn` 과 같은 자리다(ADR-0067).
		if len(cmd.args) == 0 {
			return renameInputMode(m.editor)
		}

		model, next := normalMode(m.editor)

		return model, tea.Batch(next, m.startRename(cmd.args[0]))
	case "noh", "nohlsearch":
		// 강조만 끈다. 마지막 검색은 남아서 `n` 이 계속 먹는다. vim 과 같다.
		m.search.highlight = false

		return normalMode(m.editor)
	case "jobs":
		// `!` 는 이 명령에서 뜻이 없다. 목록을 열기만 한다.
		return jobsMode(m.editor)
	case "messages", "mes":
		// vim 이 `:mes` 를 줄임말로 받는다. 여기도 같게 둔다(ADR-0053).
		return messagesMode(m.editor)
	case "jumplogs":
		// 방문 기록이다. `:jumps`(되돌아간 자리) 와 다른 것이라 이름도 갈랐다(ADR-0074).
		return jumplogsMode(m.editor)
	case "jumps":
		// vim 과 같은 이름이다. `!` 는 이 명령에서 뜻이 없다(ADR-0070).
		return jumpsMode(m.editor)
	case "registers", "reg":
		// vim 이 `:reg` 를 줄임말로 받는다. `!` 는 이 명령에서 뜻이 없다(ADR-0058).
		return registersMode(m.editor)
	case "version":
		// `!` 는 이 명령에서 뜻이 없다. 찍기만 한다.
		// CLI 의 `--version` 과 같은 줄이다(buildinfo.Describe).
		return normalModeMessage(m.editor, buildinfo.Describe())
	case "tree":
		// `!` 는 이 명령에서 뜻이 없다. 그냥 여닫는다.
		load, err := m.toggleTree()
		if err != nil {
			return normalModeError(m.editor, err)
		}

		model, next := normalMode(m.editor)

		return model, tea.Batch(next, load)
	case "q":
		// 지금 보고 있는 tab 만 닫는다. 마지막 tab 을 닫으면 빈 화면이 남고, 그 화면에서
		// 다시 치면 종료다(ADR-0064).
		// `!` 는 묻지 않고 닫는다. 그냥 `:q` 는 저장하지 않은 변경이 있으면 확인창을 띄우고,
		// 취소하면 명령줄이 아니라 normal 로 돌아간다.
		if cmd.force {
			return forceCloseTab(m.editor)
		}
		back, _ := normalMode(m.editor)

		return closeTab(back, m.editor)
	case "qa":
		// 전체 종료다. `!` 는 묻지 않고, 그냥 `:qa` 는 어느 tab 이든 변경이 남아 있으면 묻는다.
		if cmd.force {
			return Exit()
		}
		back, _ := normalMode(m.editor)

		return quitAll(back, m.editor)
	default:
		return normalModeMessage(m.editor, "알 수 없는 명령: "+m.input)
	}
}

// deleteLines 는 `:[범위]d` 다. 범위를 치지 않았으면 커서 줄 하나다.
//
// 지운 줄 수를 알린다. 키로 치는 `dd` 는 글자가 사라지는 것이 화면에 보여서 알리지 않는데
// (ADR-0017), 손으로 친 범위는 화면 밖일 수 있어서 무엇이 사라졌는지 볼 길이 없다.
// vim 이 `5 fewer lines` 를 찍는 것과 같은 이유다.
func (m viewEditorCommand) deleteLines(cmd command) (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	from, to, err := cmd.lines.resolve(*buf)
	if err != nil {
		return normalModeError(m.editor, err)
	}

	removed := buf.deleteLines(from, to, m.contentWidth())
	m.registers.storeDelete(removed, "")
	m.scrollToCursor()

	return normalModeMessage(m.editor, fmt.Sprintf("%d 줄 지웠습니다", len(removed.lines)))
}

// yankLines 는 `:[범위]y` 다. 범위를 치지 않았으면 커서 줄 하나다.
//
// 알림 문구는 `y` 가 쓰는 것을 그대로 쓴다. 세는 법이 두 벌이 되면 `5y` 와 `:.,+4y` 가
// 같은 것을 하고 다르게 말한다.
func (m viewEditorCommand) yankLines(cmd command) (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	from, to, err := cmd.lines.resolve(*buf)
	if err != nil {
		return normalModeError(m.editor, err)
	}

	copied := buf.yankLines(from, to)
	m.registers.storeYank(copied, "")

	return normalModeMessage(m.editor, copied.copiedMessage())
}

// substitute 는 `:[범위]s/찾을 것/바꿀 글/flag` 다. 범위를 치지 않았으면 커서 줄 하나다.
//
// 커서는 마지막으로 바꾼 줄의 첫 비공백으로 간다. 줄로 뛰는 것은 `:5`·`gg`·`G` 와 같은
// 일이라 그쪽 규칙을 따른다(goToLine).
func (m viewEditorCommand) substitute(cmd command) (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	// 범위를 먼저 푼다. 없는 줄을 댔으면 패턴이 옳은지 보기 전에 걸려야 한다 — `:d` 와 같다.
	from, to, err := cmd.lines.resolve(*buf)
	if err != nil {
		return normalModeError(m.editor, err)
	}

	if len(cmd.args) == 0 {
		return normalModeMessage(m.editor, "바꿀 것을 대지 않았습니다")
	}

	sub, err := parseSubstitute(cmd.args[0])
	if err != nil {
		return normalModeError(m.editor, err)
	}

	// **찾은 것이 마지막 검색이 된다.** `n` 으로 남은 자리를 훑을 수 있고, 물어보며 바꾸는
	// 동안 화면에 칠해지는 것도 이것이다 — 그쪽에 그릴 것을 따로 만들지 않았다. vim 과 같다
	// (ADR-0010, ADR-0084).
	m.search = searchState{input: sub.input, pattern: sub.pattern, direction: searchForward, highlight: true}

	if sub.confirm {
		return substituteMode(m.editor, sub, from, to)
	}

	changes, lines, last := buf.substitute(sub, from, to)
	if changes == 0 {
		// 못 찾은 것은 실패가 아니라 결과다. 검색이 쓰는 문구를 그대로 쓴다.
		return normalModeMessage(m.editor, "찾을 수 없음: "+sub.input)
	}

	buf.cursorLine = last
	buf.moveLineFirstNonBlank(m.contentWidth())
	buf.clampToNormal(m.contentWidth())
	m.scrollToCursor()

	return normalModeMessage(m.editor, substituteMessage(changes, lines))
}

// goToLine 은 이름 없이 범위만 친 것이다. `:5` 로 그 줄로 간다.
//
// 두 자리를 쳤으면(`:1,5`) 뒤쪽으로 간다. vim 과 같다 — 범위의 끝이 곧 명령이 마지막으로
// 건드린 줄이고, 커서는 늘 그 자리에 남는다.
//
// 칸은 첫 비공백이다. 줄 번호로 뛰는 것은 `gg`·`G` 와 같은 일이라 그쪽 규칙을 따른다.
// 되돌아오기 이력에 담는 것도 같은 까닭으로 같다(ADR-0082).
func (m viewEditorCommand) goToLine(cmd command) (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	_, to, err := cmd.lines.resolve(*buf)
	if err != nil {
		return normalModeError(m.editor, err)
	}

	from, jumping := m.here()

	buf.cursorLine = to
	buf.moveLineFirstNonBlank(m.contentWidth())
	buf.clampToNormal(m.contentWidth())
	m.scrollToCursor()

	if jumping {
		m.recordJumpMove(from)
	}

	return normalMode(m.editor)
}

// write 는 `:w` 다.
//
//	:w             보고 있는 파일에 쓴다. 밖에서 바뀌었으면 알리고 만다 (ADR-0015)
//	:w!            밖에서 바뀐 파일도 덮어쓴다
//	:w <파일>       그 파일에 쓴다. 이미 있으면 알리고 만다 (ADR-0024)
//	:w! <파일>      이미 있는 파일도 덮어쓴다
//
// 인자가 붙으면 사본을 쓰는 것이라 보고 있는 파일도 tab 도 그대로다. 이름 없는 buffer 만
// 예외로 그 이름을 받는다 — `:tabnew` 로 만든 tab 을 저장하는 길이 이것뿐이다.
func (m viewEditorCommand) write(cmd command) (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	if len(cmd.args) == 0 {
		return m.save(cmd)
	}

	path := cmd.args[0]

	// `:w <보고 있는 파일>` 은 사본이 아니라 제자리 저장이다. 사본 쪽으로 보내면
	// 「이미 있습니다」로 막히고, `!` 를 붙여도 dirty 가 남는다.
	if buf.path != "" && samePath(buf.path, path) {
		return m.save(cmd)
	}

	// 이름 없는 buffer 는 이 저장으로 그 파일의 buffer 가 되므로 「같은 파일은 한 tab」에
	// 걸린다(ADR-0015, ADR-0021). 이름 있는 buffer 는 이름이 그대로라 걸리지 않는다.
	naming := buf.path == ""
	if naming {
		if _, ok := m.tabOf(path); ok {
			return normalModeMessage(m.editor, "그 파일은 이미 다른 tab 에 열려 있습니다")
		}
	}

	var err error
	if cmd.force {
		err = buf.SaveToForce(path)
	} else {
		err = buf.SaveTo(path)
	}
	if err != nil {
		return normalModeError(m.editor, err)
	}
	// 저장하면 저장소가 dirty 가 된다. 주기 갱신을 기다리지 않고 여기서 맞춘다(ADR-0009, ADR-0030).
	refresh := m.startGitRefresh()

	if !naming {
		// 보고 있는 파일이 아니라 다른 파일에 썼다. 문구를 나눠야 tabline 의 이름이
		// 그대로인 것이 실패로 읽히지 않는다.
		model, next := normalModeMessage(m.editor, "사본을 씀: "+path)

		return model, tea.Batch(next, refresh)
	}

	// 이름이 붙어서 이제 이 파일을 보고 있는 것이다. 트리도 그 자리를 가리켜야 한다(ADR-0019).
	reveal := m.revealInSidebar(path)

	model, next := normalModeMessage(m.editor, "저장함: "+path)

	return model, tea.Batch(next, refresh, reveal)
}

// save 는 보고 있는 파일에 쓴다. 인자 없는 `:w` 와 `:w <보고 있는 파일>` 이 쓴다.
func (m viewEditorCommand) save(cmd command) (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	// 쓰기 직전에 통과시킬 포매터다. 깔려 있지 않으면 hook 이 nil 이고 까닭이 따라온다 —
	// 그것을 가지고 물을지는 저장을 끝낸 뒤에 본다(save-hook.go, ADR-0065).
	hook, hookErr := m.saveHookFor(buf.path)

	// `!` 는 읽은 뒤 밖에서 바뀐 파일도 덮어쓴다는 뜻이다 (ADR-0015).
	var (
		err  error
		note string
	)
	if cmd.force {
		note, err = buf.SaveForce(m.contentWidth(), hook)
	} else {
		note, err = buf.Save(m.contentWidth(), hook)
	}
	if err != nil {
		return normalModeError(m.editor, err)
	}

	// 맞춘 것이 있으면 저장 문구 뒤에 붙인다. 손대지 않은 줄이 바뀌는 일이라 반드시 보여야
	// 한다(ADR-0052). 커서가 잘려나간 자리에 서 있었으면 trimTrailingSpace 가 이미 당겨 두었고,
	// 화면은 그 자리를 다시 잡아야 한다.
	m.scrollToCursor()

	// 저장은 dirty 를 바꾸므로 주기 갱신을 기다리지 않는다(ADR-0030).
	refresh := m.startGitRefresh()

	// 저장했다는 것이 앞이고 맞춘 것이 뒤다. 무엇을 했는지가 먼저 오고 곁들여 무엇이
	// 달라졌는지가 따라온다.
	//
	// **경로는 줄여 적는다.** 트리나 팔레트로 연 파일은 절대 경로라, 그대로 두면 좁은 화면에서
	// 경로가 줄을 다 먹고 뒤에 붙인 문구가 잘린다. 줄이는 법은 `GOTO` 목록과 같다(view-locations.go).
	message := "저장함: " + shortenPath(buf.path)
	if note != "" {
		message += "  " + note
	}

	model, next := normalModeMessage(m.editor, message)

	// 포매터를 찾지 못했으면 여기서 한 번 묻는다. **저장은 이미 끝났다** — 깔지 않기로 해도
	// 파일은 쓰인 상태다. 창은 방금 그린 화면 위에 얹히고 Yes·No 둘 다 그 화면으로 돌아온다
	// (ADR-0065).
	if errors.Is(hookErr, errHookNotInstalled) && m.askGoimports() {
		confirm, _ := goimportsInstallConfirmMode(model, m.editor)

		return confirm, tea.Batch(next, refresh)
	}

	return model, tea.Batch(next, refresh)
}

// edit 은 `:e` 다. 인자가 있으면 파일을 열고, 없으면 보고 있는 파일을 다시 읽는다(ADR-0021).
//
//	:e            다시 읽는다. 저장하지 않은 변경이 있으면 확인창을 띄운다
//	:e!           묻지 않고 다시 읽는다
//	:e <파일>      활성 tab 을 그 파일로 갈아끼운다. 잃을 것이 있으면 확인창을 띄운다
//	:e! <파일>     묻지 않고 갈아끼운다
func (m viewEditorCommand) edit(cmd command) (tea.Model, tea.Cmd) {
	if len(cmd.args) == 0 {
		// 팔레트의 「파일 다시 읽기」와 같은 길이다. `!` 가 확인창 자리를 대신한다(ADR-0016).
		if cmd.force {
			return reloadFile(m.editor)
		}

		return runReloadFile(m.editor)
	}

	path := cmd.args[0]

	// 지금 tab 의 편집이 사라지는 것은 갈아끼울 때뿐이다. 이미 다른 tab 에 열려 있으면
	// replaceTab 이 그리로 옮겨가기만 하므로 잃을 것이 없다.
	//
	// tab 이 아예 없으면 갈아끼우는 것이 아니라 새로 여는 것이라 여기서도 잃을 것이 없다
	// (replaceTab, ADR-0064).
	_, opened := m.tabOf(path)
	if m.hasTab() && m.activeBuffer().dirty && !cmd.force && !opened {
		// 취소하면 명령줄이 아니라 normal 로 돌아간다. `:q` 의 확인창과 같다.
		back, _ := normalMode(m.editor)

		return ConfirmDiscard(back, m.editor, "이 tab 에 다른 파일을 여시겠습니까?", func() (tea.Model, tea.Cmd) {
			return editFile(m.editor, path)
		}), nil
	}

	return editFile(m.editor, path)
}

// editFile 은 묻지 않고 연다. 확인창의 Yes 와 잃을 것이 없을 때가 쓴다.
// reloadFile 과 같은 짝이다.
func editFile(e *editor, path string) (tea.Model, tea.Cmd) {
	reveal, err := e.replaceTab(path)
	if err != nil {
		return normalModeError(e, err)
	}

	// 갈아끼운 buffer 는 맨 위에서 시작하지만, 옮겨간 tab 은 보던 자리를 그대로 이어받는다.
	// 어느 쪽이든 지금 폭에 맞춰 둔다 — sidebar 를 여닫은 뒤라면 폭이 달라져 있다.
	e.scrollToCursor()

	model, cmd := normalMode(e)

	// 파일을 여는 것은 바깥에서 `commit`·`checkout` 을 하고 돌아온 직후일 때가 많다(ADR-0030).
	return model, tea.Batch(cmd, e.startGitRefresh(), reveal)
}

func (m viewEditorCommand) View() tea.View {
	line := ":" + m.input
	view := m.editorView(tea.CursorBlock, "COMMAND", line)

	// 커서는 본문이 아니라 명령줄 끝에 있어야 한다.
	// 명령줄도 편집 영역 아래에 있으므로 sidebar 만큼 오른쪽으로 옮긴다.
	view.Cursor = tea.NewCursor(screenWidthOf(line)+m.sidebarLeft(), m.height-1)

	return view
}
