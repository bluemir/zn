package core

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/lsp"
)

// 사용처로 가기다(ADR-0068).
//
// 정의로 가기(language-server.go) 와 짝이다 — 그쪽은 「이 이름은 어디서 왔나」이고 이쪽은 「이 이름을
// 누가 쓰나」다. 묻는 요청만 다르고, 답의 모양(Location 목록) 도 그것을 보이는 화면
// (view-locations.go) 도 같아서 붙는 데 든 것은 요청 하나와 이 파일뿐이다.
//
// 부르는 문은 둘이다 — `\gr`(action.go) 과 팔레트의 「사용처로 가기」(palette.go).
// `:` 명령은 없다. 받을 인자가 없어서 `:rename <새 이름>` 같은 벌이가 없다.

// referencesMsg 는 사용처를 물은 답이다.
type referencesMsg struct {
	locations []lsp.Location
	err       error
}

// gotoReferences 는 커서 자리의 사용처를 찾거나, 서버가 없으면 설치를 묻는다.
//
// 문 앞의 검사는 정의로 가기와 같은 것을 같은 순서로 본다(language-server.go 의 gotoDefinition).
// 하나로 묶지 않은 것은 알리는 문구가 「무엇을 찾는 중인가」로 갈리기 때문이다 — 묶으면
// 그 문구가 인자가 되고, 그 인자는 여기 있는 대여섯 줄보다 읽기 어렵다.
func gotoReferences(parent tea.Model, e *editor) (tea.Model, tea.Cmd) {
	// 볼 파일이 없으면 커서도 없다. `\gr` 과 팔레트가 함께 지나는 자리라 여기서 막는다
	// (ADR-0064).
	if e.refuseNoBuffer() {
		return nil, nil
	}

	server, _, ok := serverPath(e.activeBuffer().path)
	if !ok {
		e.notify("언어 서버가 붙는 파일에서만 사용처를 찾습니다")

		return nil, nil
	}

	// 죽은 서버는 여기서 자리를 비운다(ADR-0092).
	if e.clientOf(server) == nil {
		state := e.serverState(server)

		if e.jobRunning(serverJobName(server), nil) {
			e.notify(server.Name + " 를 설치하는 중입니다")

			return nil, nil
		}

		if state.starting {
			e.notify(server.Name + " 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

			return nil, nil
		}

		if state.failed {
			return serverInstallConfirmMode(parent, e, server)
		}

		e.notify(server.Name + " 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return nil, e.startServer(server)
	}

	return nil, e.startReferences()
}

// startReferences 는 커서 자리의 이름을 쓰는 자리들을 묻는다.
//
// 답을 기다리지 않는다. 서버가 저장소 전체를 훑는 일이라 rename(664ms) 쪽에 가깝다 —
// 잰 값으로 첫 요청이 935ms 이고 그 뒤는 2ms 다(ADR-0068).
func (e *editor) startReferences() tea.Cmd {
	buf := e.activeBuffer()

	server, path, ok := serverPath(buf.path)
	if !ok {
		e.notify("언어 서버가 붙는 파일에서만 사용처를 찾습니다")

		return nil
	}

	client := e.clientOf(server)
	if client == nil {
		if e.serverState(server).failed {
			e.notify(server.Name + " 가 없어 사용처를 찾을 수 없습니다")

			return nil
		}

		e.notify(server.Name + " 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return e.startServer(server)
	}

	lines := buf.lines
	position := lsp.Position{
		Line:      buf.cursor.Line,
		Character: lsp.UTF16Column(buf.lines[buf.cursor.Line], buf.cursor.Col),
	}

	e.notify("사용처를 찾는 중입니다")

	return func() tea.Msg {
		// 묻기 직전에 전문으로 맞춘다. 방금 친 이름도 이 한 번으로 서버에 닿는다(ADR-0051).
		if err := client.SyncFull(path, lines); err != nil {
			return referencesMsg{err: err}
		}

		locations, err := client.References(path, position)

		return referencesMsg{locations: locations, err: err}
	}
}

// finishReferences 는 답을 받아 목록을 열거나 그 자리로 간다.
//
// 하나면 곧바로 뛴다 — 정의로 가기와 같은 규칙이다(language-server.go 의 finishDefinition).
// 한 줄짜리 목록을 보여 주고 enter 를 또 받는 것은 손이 하나 더 드는 일이다.
//
// 선언 자리는 목록에 없다(lsp/references.go). 그래서 아무도 쓰지 않는 이름은 0 개로 온다 —
// 「사용처를 찾지 못했습니다」가 그것이고, 이 말이 맞으려면 선언이 빠져 있어야 한다.
func (e *editor) finishReferences(msg referencesMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		e.notifyError(msg.err)

		return nil, nil
	}

	switch len(msg.locations) {
	case 0:
		e.notify("사용처를 찾지 못했습니다")

		return nil, nil
	case 1:
		// 뛰기 전 자리를 이력에 담는다. 판이 열리는 쪽은 판이 담는다(view-locations.go, ADR-0070).
		e.recordJump()

		cmd := e.jumpTo(msg.locations[0])
		e.arrive()

		return nil, cmd
	}

	return locationsMode(e, "사용처", msg.locations)
}
