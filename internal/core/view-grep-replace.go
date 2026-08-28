package core

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// 검색 판이 치환을 확정하는 자리다(ADR-0097 §5, §6).
//
// **판을 새로 만들지 않았다.** 목록도 둘러보기도 거르기도 검색 판이 이미 들고 있고, 여기서
// 더할 것은 「이 목록을 바꿀까요」를 묻는 것뿐이다. 하단 drawer 다섯이 이미 글자까지 같은
// `renderDrawer` 를 복사해 가지고 있는데(docs/tasks.md) 여섯째를 보태지 않는다.
//
// `:s///c` 의 물음(view-editor-substitute.go) 과 키가 같다 — `y` `n` `a` `q`. 그쪽은 한
// buffer 안이라 커서가 「지금 물어보는 자리」를 드러내는데, 여기는 자리마다 파일이 달라서
// tab 을 옮겨 가며 물어야 하고 그 손을 판이 들고 있다(previewSession, ADR-0078).

// pressReplaceInput 은 `r` 뒤에 바꿀 글을 치는 동안의 키다.
//
// 거르기와 같은 손이다(pressFilter) — 판 안에서 글자를 그대로 받고 한글도 되돌리지 않는다.
// 바꿀 글은 통째로 글자라 `msg.Text` 를 그대로 쓴다(ADR-0008).
func (m viewGrep) pressReplaceInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return quitAll(m, m.editor)
	case "esc":
		m.asking, m.answer = false, ""

		return m, nil
	case "enter":
		return m.startReplaceFromGrep()
	case "backspace":
		// 다 지우면 치기에서 나간다. 명령줄·거르기와 같은 손이다.
		if m.answer == "" {
			m.asking = false

			return m, nil
		}
		m.answer = m.answer[:prevClusterStart([]byte(m.answer), 0, len(m.answer))]

		return m, nil
	default:
		if msg.Text == "" {
			return m, nil
		}
		m.answer += msg.Text

		return m, nil
	}
}

// startReplaceFromGrep 은 찾아 둔 목록을 치환 대기로 바꾼다. `:grep` 판에서 들어오는 문이다.
//
// **패턴은 이미 있다.** 찾은 것을 그대로 쓰므로 다시 칠 것이 없고, 받은 것은 바꿀 글뿐이다.
//
// `g` 를 켠 채로 간다. 이 판의 단위가 줄이라(ADR-0097 §2) 「이 줄을 바꾼다」가 그 줄의 찾은
// 자리를 다 바꾸는 것이어야 손과 맞는다. 첫 하나만 바꾸고 싶으면 `:replace` 로 flag 를 댄다.
func (m viewGrep) startReplaceFromGrep() (tea.Model, tea.Cmd) {
	replacement, err := parseReplacement(m.answer)
	if err != nil {
		m.asking, m.answer = false, ""

		return normalModeError(m.editor, err)
	}

	root, err := os.Getwd()
	if err != nil {
		m.asking, m.answer = false, ""

		return normalModeError(m.editor, err)
	}

	m.replace = replacePending{
		on:   true,
		root: root,
		sub: substitution{
			input:       m.grep.input,
			pattern:     m.grep.pattern,
			replacement: replacement,
			all:         true,
		},
	}

	m.asking, m.answer = false, ""

	return m, nil
}

// replaceKey 는 치환 대기 중에 먼저 먹는 키다. taken 이 거짓이면 판의 보통 키로 넘어간다.
//
// `q`·`esc` 는 여기서 먹지 않는다. 판의 `cancel` 이 치환 대기를 보고 갈라 주므로 취소하는
// 자리가 하나로 남는다.
func (m viewGrep) replaceKey(key string) (tea.Model, tea.Cmd, bool) {
	if len(m.rows()) == 0 {
		// 바꿀 것이 없다. `esc` 로 닫는 것만 남는다.
		return m, nil, false
	}

	// **`enter` 는 여기서 먹고 아무 일도 하지 않는다.** 판이 묻고 있는 자리이고 아래 줄이
	// 내놓는 답은 `a`·`y`·`q` 셋뿐이라, 안내에 없는 키가 판을 떠나게 두지 않는다. 뛰어가
	// 보고 싶으면 `q` 로 그만두고 `:grep` 으로 다시 연다.
	if key == "enter" {
		return m, nil, true
	}

	if !m.replace.stepping {
		switch key {
		case "a":
			return m.replaceAll()
		case "y":
			// 하나씩으로 들어간다. 지금 고른 자리부터 묻는다.
			m.replace.stepping = true

			return m.showReplaceSpot(), nil, true
		}

		return m, nil, false
	}

	switch key {
	case "y":
		m.replaceSpot()

		return m.stepReplace()
	case "n":
		return m.stepReplace()
	case "a":
		// 남은 것을 다 바꾸고 끝낸다. 도중에 생각이 바뀌었을 때 누르는 키다.
		return m.replaceRest()
	}

	return m, nil, false
}

// replaceAll 은 `a` 다. 목록에 있는 것을 한 번에 바꾼다.
//
// **거른 목록이 곧 바꿀 목록이다.** `/` 로 좁혀 두었으면 좁힌 것만 바뀐다 — 아래 줄이 세어
// 보여준 수가 그것이라, 보여준 것과 바꾸는 것이 갈리면 물어본 값이 없어진다.
func (m viewGrep) replaceAll() (tea.Model, tea.Cmd, bool) {
	paths, lines := replaceTargets(m.rows())

	m.replace.done.merge(m.editor.applyReplace(m.replace.root, paths, lines, m.replace.sub))

	model, cmd := m.finishReplace()

	return model, cmd, true
}

// replaceRest 는 하나씩 훑다가 친 `a` 다. 지금 자리부터 끝까지 바꾼다.
func (m viewGrep) replaceRest() (tea.Model, tea.Cmd, bool) {
	rest := m.rows()
	rest = rest[min(m.selected, len(rest)):]

	paths, lines := replaceTargets(rest)

	m.replace.done.merge(m.editor.applyReplace(m.replace.root, paths, lines, m.replace.sub))

	model, cmd := m.finishReplace()

	return model, cmd, true
}

// replaceSpot 은 `y` 다. 지금 고른 한 줄을 바꾸고 쓴다.
//
// **그 자리에서 쓴다.** 모아 두었다가 끝에 한 번에 쓰지 않는 것은, 도중에 `q` 로 나가도
// 답한 것이 남아야 하기 때문이다. `y` 한 번이 곧 한 번의 저장이다.
func (m viewGrep) replaceSpot() {
	rows := m.rows()
	if m.selected >= len(rows) {
		return
	}

	hit := rows[m.selected]

	changed, missed, err := m.editor.replaceFile(
		m.replace.fullPath(hit.path), []int{hit.line}, m.replace.sub)

	m.replace.done.missed += missed

	if err != nil {
		m.replace.done.failed = append(m.replace.done.failed, shortenPath(hit.path)+": "+err.Error())

		return
	}

	if changed > 0 {
		m.replace.done.mark(hit.path)
		m.replace.done.lines += changed
	}
}

// stepReplace 는 다음 자리로 넘어간다. 더 없으면 끝낸다.
func (m viewGrep) stepReplace() (tea.Model, tea.Cmd, bool) {
	if m.selected+1 >= len(m.rows()) {
		model, cmd := m.finishReplace()

		return model, cmd, true
	}

	m.move(1)

	return m, nil, true
}

// showReplaceSpot 은 하나씩으로 들어설 때 지금 자리를 보여준다.
//
// `move(0)` 이 아니라 `selectTo` 인 것은 아직 한 번도 안 밟았을 수 있어서다 — 판을 열자마자
// `y` 를 친 경우다. 그때 미리보기가 그 자리로 간다(preview.go).
func (m viewGrep) showReplaceSpot() tea.Model {
	m.selectTo(m.selected)

	return m
}

// finishReplace 는 치환을 끝내고 판을 닫는다.
//
// 판을 열기 전 자리로 되돌리고 둘러보며 연 tab 을 닫는 것은 보통 취소와 같다. **바꾼 것은
// 이미 디스크에 있다** — 되돌리는 것은 화면이지 파일이 아니다(ADR-0097 §4).
func (m viewGrep) finishReplace() (tea.Model, tea.Cmd) {
	sub, out := m.replace.sub, m.replace.done
	m.replace = replacePending{}

	restore := m.preview.restore(m.editor)

	if len(out.failed) > 0 {
		model, next := normalModeError(m.editor, replaceError(sub, out))

		return model, tea.Batch(next, restore)
	}

	if out.lines == 0 {
		model, next := normalModeMessage(m.editor, "바꾼 것이 없습니다")

		return model, tea.Batch(next, restore)
	}

	model, next := normalModeMessage(m.editor, replaceMessage(sub, out))

	return model, tea.Batch(next, restore)
}
