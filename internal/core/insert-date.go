package core

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// insertDateFormat 은 「오늘 날짜 넣기」가 넣는 서식이다. RFC3339 의 날짜 부분만이다.
//
// 시각까지 넣는 쪽은 `time.RFC3339` 를 그대로 쓴다. 두 서식이 앞머리를 나눠 갖는 것이라
// 날짜만 넣은 뒤 뒤에 시각을 이어 붙여도 같은 글이 된다(ADR-0116).
const insertDateFormat = "2006-01-02"

// runInsertDate 는 오늘 날짜를 커서 자리에 넣는다. 팔레트 `>` 목록의 「오늘 날짜 넣기」다.
func runInsertDate(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return paletteInsertText(e, time.Now().Format(insertDateFormat))
}

// runInsertDateTime 은 지금 날짜와 시각을 RFC3339 로 넣는다. 팔레트 `>` 목록의
// 「오늘 날짜와 시각 넣기」다.
func runInsertDateTime(e *editor, opts ...runOption) (tea.Model, tea.Cmd) {
	return paletteInsertText(e, time.Now().Format(time.RFC3339))
}

// paletteInsertText 는 위 둘의 몸통이다. 갈리는 것이 서식 하나라 두 벌로 두지 않았다 —
// 대소문자 맞추기 둘이 paletteChangeCase 하나를 몸통으로 두는 것과 같다.
//
// **커서 뒤에 넣는다.** normal 의 `a` 와 같은 자리이고 특수문자 drawer 도 같은 손이다
// (ADR-0056). 팔레트는 어디서 열렸는지 기억하지 않으므로(ADR-0011) 여기 오는 커서는 늘
// normal 자리다 — insert 에서 연 경우는 그쪽에서 맞춰 놓고 넘긴다(view-editor-insert.go).
//
// 넣고 나면 normal 로 돌아간다. drawer 를 열어 잇달아 넣는 특수문자와 달리 이것은 한 번에
// 끝나는 일이라 열어 둘 판이 없다.
func paletteInsertText(e *editor, text string) (tea.Model, tea.Cmd) {
	if e.refuseNoBuffer() {
		return normalMode(e)
	}

	// 읽기 전용 파일은 고치지 않는다(readonly.go). 알림은 그쪽이 적는다.
	if e.refuseReadOnly() {
		return normalMode(e)
	}

	buf := e.activeBuffer()
	width := e.contentWidth()

	buf.moveRight(1, width)
	buf.insert([]byte(text), width)

	// 넣은 것을 한 편집으로 닫는다. 한 번의 `u` 로 통째로 돌아온다.
	// 커서는 마지막 글자 위에 선다 — insert 의 `esc` 와 같다.
	buf.endEdit()
	buf.moveLeft(1, width)
	e.scrollToCursor()

	return normalMode(e)
}
