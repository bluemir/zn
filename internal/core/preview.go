package core

import (
	tea "charm.land/bubbletea/v2"
)

// 둘러보는 판이 「없던 일로 되돌리기」 위해 들어 두는 것이다(ADR-0071, ADR-0073).
//
// `GOTO` 판과 되돌아간 자리 판이 나눠 쓴다. **판을 나눈 것이 아니라 뒤처리를 나눈 것이다** —
// 그리는 것도, 무엇을 담았는지도, 확정할 때 무슨 일이 일어나는지도 둘이 다르다. 같은 것은
// 「둘러보기 전 화면이 어땠나」 하나뿐이고, 그것은 인자가 붙지 않는 순수한 상태다(CLAUDE.md).

// previewSession 은 둘러보기 시작할 때의 화면이다.
//
// 둘러보는 것이 커서를 실제로 옮기는 일이라(ADR-0071) 되돌릴 것이 둘 생긴다 — 커서가 있던
// 자리와, 그때 열려 있던 tab 이다. 닫혀 있는 파일을 보여주려면 tab 으로 여는 수밖에 없어서
// 목록을 훑는 동안 tab 이 늘어난다.
type previewSession struct {
	// origin 은 판을 열기 전 커서 자리다. 담지 못하는 자리(이름 없는 buffer) 면 hasOrigin 이 거짓이다.
	origin    jumpPlace
	hasOrigin bool

	// wasOpen 은 판을 열 때 이미 열려 있던 tab 의 경로다. 이 밖의 것이 둘러보다 열린 것이다.
	wasOpen map[string]bool
}

// startPreview 는 지금 화면을 적어 둔다. 판을 여는 자리에서 부른다.
func startPreview(e *editor) previewSession {
	session := previewSession{wasOpen: map[string]bool{}}

	session.origin, session.hasOrigin = e.here()

	for i := range e.buffers {
		session.wasOpen[e.buffers[i].path] = true
	}

	return session
}

// restore 는 취소다. 판을 열기 전 자리로 되돌리고 둘러보며 연 tab 을 전부 닫는다.
//
// **둘러본 것이 없던 일이 되어야 마음 놓고 훑을 수 있다.** 나갔을 때 마지막으로 본 자리에
// 남는다면 「보기만 하려다 옮겨져 버리는」 일이 생기고, 그러면 `j` 를 누르기 전에 망설이게
// 된다. 검색의 `esc` 가 커서·화면을 되돌리는 것과 같은 약속이다(ADR-0010, ADR-0071).
func (s previewSession) restore(e *editor) tea.Cmd {
	keep := ""

	var cmd tea.Cmd
	if s.hasOrigin {
		keep = s.origin.path
		cmd = e.goToPlace(s.origin)
	}

	s.closeOpened(e, keep)

	return cmd
}

// keep 은 확정이다. 둘러보며 연 tab 중 그 경로 하나만 남기고 닫는다.
func (s previewSession) keep(e *editor, path string) {
	s.closeOpened(e, path)
}

// closeOpened 는 둘러보다 연 tab 을 닫는다. keep 은 남길 경로다.
//
// **판을 열 때 이미 열려 있던 것은 건드리지 않는다.** 그것은 사용자가 연 tab 이고, 우리가
// 그 위를 지나갔다는 이유로 닫으면 안 된다. 미리보기가 편집을 하지 않으므로 우리가 닫는
// tab 에 저장하지 않은 변경이 있을 수 없다 — 있다면 그것은 `wasOpen` 에 들어 있다.
//
// 뒤에서 앞으로 닫는다. 앞부터 닫으면 뒤 index 가 밀린다.
func (s previewSession) closeOpened(e *editor, keep string) {
	for i := len(e.buffers) - 1; i >= 0; i-- {
		path := e.buffers[i].path
		if s.wasOpen[path] || path == keep {
			continue
		}

		e.closeTabAt(i)
	}
}
