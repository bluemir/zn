package core

import (
	tea "charm.land/bubbletea/v2"
	"github.com/bluemir/zn/internal/scheme"
)

// viewEditorSubstitute 는 `:s///c` 가 한 자리씩 물어보는 자리다.
//
// **화면은 편집 화면 그대로다.** 커서가 지금 물어보는 자리에 서고 아래 줄이 묻는다.
// 매칭이 칠해지는 것은 `:s` 가 패턴을 마지막 검색으로 굳혀 둔 덕이라 여기서 할 일이 없고,
// 지금 물어보는 한 자리만 색이 다른 것도 그렇다 — 「커서가 선 매칭」이 곧 이 자리다(ADR-0010).
type viewEditorSubstitute struct {
	*editor

	sub substitution

	// area 는 훑을 자리다. 바꾸어도 줄 수가 늘지 않아서 시작할 때 정한 끝이 끝까지 맞다.
	//
	// 줄 범위가 아니라 scheme.MotionRange 인 것은 글자로 고른 범위를 그대로 받기 때문이다.
	// 줄마다 볼 구간은 selectionOn 이 잘라 준다 — 일괄 치환과 같은 자리다(ADR-0089).
	area scheme.MotionRange

	// 지금 보고 있는 줄과 그 줄에서 바꿀 자리들이다.
	//
	// **origin 은 그 줄에 들어설 때의 글이다.** 잡은 것(`\1`) 을 꺼내려면 자리를 찾은
	// 시점의 글이어야 하는데, 앞의 것을 바꾸고 나면 buffer 의 그 줄은 이미 달라져 있다.
	// 뒤로 밀린 byte 수는 delta 가 센다.
	line    int
	origin  []byte
	matches [][]int
	at      int
	delta   int

	changes, lines, last int  // 바꾼 자리 수·줄 수와 마지막으로 바꾼 줄
	editing              bool // 되돌리기 구간을 열었는가

	// 시작한 자리다. 하나도 안 바꾸고 나가면 커서와 화면이 여기로 돌아온다.
	// 화면 자리까지 드는 까닭은 검색과 같다 — 커서만 되돌리면 보이는 곳이 달라진 채로
	// 남는다(viewport.go 의 viewPlace).
	back viewPlace
}

// substituteMode 는 물어보기를 시작한다.
//
// 범위 안에 하나도 없으면 mode 를 열지 않는다 — 물어볼 것이 없는 자리에 사람을 세우고
// `q` 를 치게 할 이유가 없다.
func substituteMode(e *editor, sub substitution, area scheme.MotionRange) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()

	m := viewEditorSubstitute{
		editor: e,
		sub:    sub,
		area:   area,

		// seek 이 첫 줄부터 들어서게 한 줄 앞에서 시작한다.
		line: area.Start.Line - 1,
		last: -1,

		back: buf.Place(),
	}

	if !m.seek() {
		return normalModeMessage(e, "찾을 수 없음: "+sub.input)
	}
	m.show()

	return m, nil
}

func (m viewEditorSubstitute) Init() tea.Cmd { return nil }

func (m viewEditorSubstitute) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		return m, nil
	case tea.KeyPressMsg:
		// 한글 입력 상태에서 온 키는 두벌식 자리의 영문 키로 바꾼다. 키가 곧 답인 자리라
		// 확인창들과 같은 손이다 — 음절 하나가 키 여럿으로 풀리므로 차례로 먹인다(ADR-0008).
		var model tea.Model = m
		for _, key := range expandHangul(msg.String()) {
			asked, ok := model.(viewEditorSubstitute)
			if !ok {
				// 앞의 조각에서 물어보기가 끝났다. 남은 조각은 버린다.
				return model, nil
			}

			next, cmd := asked.press(key)
			if cmd != nil {
				return next, cmd
			}

			model = next
		}

		return model, nil
	case tea.MouseWheelMsg:
		// 명령줄과 같이 둘러보기만 받는다. 클릭은 받지 않는다 — 물어보는 자리가 클릭
		// 한 번에 딴 데로 가면 무엇에 답한 것인지 알 수 없다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 진행도와 주기 tick 은 mode 와 무관하다. 다른 mode 와 같은 공용 처리다(job.go).
		//
		// **다만 mode 는 바꾸지 않는다.** 여기는 되돌리기 구간을 연 채 물어보는 자리라,
		// 언어 서버의 답(`\gd` 의 고르는 화면) 이 사람을 끌어내면 그 구간이 열린 채 남아서
		// 다음 편집이 이 치환에 딸려 들어간다. 답은 목록에 남으니 잃는 것도 없다.
		_, cmd := m.handleJob(msg)

		return m, cmd
	default:
		return m, nil
	}
}

// press 는 키 하나에 답한다. 넷 밖의 키는 흘린다 — 물음이 그대로 남아 다시 칠 수 있다.
func (m viewEditorSubstitute) press(key string) (tea.Model, tea.Cmd) {
	if m.stale() {
		return normalModeMessage(m.editor, "파일이 밖에서 바뀌어 치환을 멈췄습니다")
	}

	switch key {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "y":
		m.replaceHere()

		return m.step()
	case "n":
		return m.step()
	case "a":
		// 남은 것을 다 바꾸고 끝낸다. 도중에 생각이 바뀌었을 때 누르는 키다.
		m.rest()

		return m.finish()
	case "q", "esc":
		return m.finish()
	default:
		return m, nil
	}
}

// stale 은 물어보는 사이에 buffer 의 글이 통째로 갈아끼워졌는지다.
//
// **하나도 안 바꾼 동안은 그 길이 열려 있다.** 바깥 파일 검사는 mode 를 보지 않고 `dirty` 하나로
// 판정해서, 아직 `y` 를 한 번도 안 쳤으면 밖에서 바뀐 파일을 그 자리에서 읽어 들인다
// (ADR-0044). 그러면 시작할 때 정한 끝 줄도, 그 줄에서 찾아 둔 자리도 남의 글을 가리킨다.
//
// 하나라도 바꿨으면 dirty 가 서 있어서 그 길이 막혀 있다 — 그래서 여기서 멈출 때는 닫을
// 되돌리기 구간도 없다.
func (m viewEditorSubstitute) stale() bool {
	buf := m.activeBuffer()

	if m.area.End.Line >= len(buf.Lines) {
		return true
	}

	// 물어보는 자리는 늘 있다. seek 이 못 찾으면 그 자리에서 판이 끝난다.
	return m.matches[m.at][1]+m.delta > len(buf.Lines[m.line])
}

// seek 은 지금 자리부터 다음 물어볼 매칭을 찾는다. 범위 끝까지 없으면 false 다.
func (m *viewEditorSubstitute) seek() bool {
	buf := m.activeBuffer()

	for m.at >= len(m.matches) {
		m.line++
		if m.line > m.area.End.Line {
			return false
		}

		// 고른 밖은 물어보지도 않는다. 일괄 치환이 거르는 것과 같은 자리다.
		span, _, ok := buf.SelectionOn(m.area, m.line)
		if !ok {
			m.matches, m.at, m.delta = nil, 0, 0

			continue
		}

		m.origin = buf.Lines[m.line]
		m.matches = m.sub.matchesIn(m.origin, span[0], span[1])
		m.at, m.delta = 0, 0
	}

	return true
}

// step 은 다음 자리로 넘어간다. 더 없으면 끝낸다.
func (m *viewEditorSubstitute) step() (tea.Model, tea.Cmd) {
	m.at++

	if !m.seek() {
		return m.finish()
	}
	m.show()

	return *m, nil
}

// show 는 지금 물어보는 자리로 커서와 화면을 옮긴다.
func (m *viewEditorSubstitute) show() {
	buf := m.activeBuffer()

	buf.MoveTo(scheme.Cursor{Line: m.line, Col: m.matches[m.at][0] + m.delta})
	buf.ClampToNormal()
	m.scrollToCursor()
}

// replaceHere 는 지금 물어보는 자리를 바꾼다.
func (m *viewEditorSubstitute) replaceHere() {
	buf := m.activeBuffer()
	match := m.matches[m.at]
	with := m.sub.expand(m.origin, match)

	// 처음 바꿀 때 구간을 연다. 하나도 안 바꾸고 나가면 열리지 않아서 파일이 그대로다.
	// 범위 전체를 한 번에 담으므로 뒤의 줄들도 이 한 번의 `u` 로 돌아간다.
	if !m.editing {
		buf.EndEdit()
		buf.BeginEdit(m.area.Start.Line, m.area.End.Line-m.area.Start.Line+1)
		m.editing = true
	}

	buf.SpliceLine(m.line, match[0]+m.delta, match[1]+m.delta, with)
	m.delta += len(with) - (match[1] - match[0])

	m.changes++
	if m.last != m.line {
		m.lines++
		m.last = m.line
	}
}

// rest 는 남은 것을 다 바꾼다. `a` 다.
func (m *viewEditorSubstitute) rest() {
	for {
		m.replaceHere()
		m.at++

		if !m.seek() {
			return
		}
	}
}

// finish 는 물어보기를 끝내고 normal 로 나온다.
//
// 커서는 마지막으로 바꾼 줄이다. 하나도 안 바꿨으면 시작한 자리로 돌아간다 — 훑고 다니느라
// 커서가 딴 데 가 있는데 아무 일도 없었으면 그 자리에 남을 까닭이 없다.
func (m *viewEditorSubstitute) finish() (tea.Model, tea.Cmd) {
	buf := m.activeBuffer()

	if m.editing {
		buf.EndEdit()
	}

	if m.changes == 0 {
		buf.MoveToPlace(m.back)

		return normalModeMessage(m.editor, "바꾼 것이 없습니다")
	}

	buf.MoveToLine(m.last)
	buf.ClampToNormal()
	m.scrollToCursor()

	return normalModeMessage(m.editor, substituteMessage(m.changes, m.lines))
}

// 아래 줄은 물음이다. 키와 그 뜻이 같이 서는 것은 목록 화면들과 같은 손이다.
func (m viewEditorSubstitute) View() tea.View {
	return m.editorView(tea.CursorBlock, "SUBSTITUTE", "바꿀까요?  y 바꾸기  n 넘기기  a 남은 것 전부  q 그만")
}
