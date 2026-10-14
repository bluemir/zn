package core

import (
	tea "charm.land/bubbletea/v2"
)

// 파일 안에 찍어 둔 자리(mark) 다(ADR-0154).
//
// 찍은 자리는 buffer 가 들고 편집을 따라간다(textarea/mark.go). 여기 있는 것은 키가 그것을
// 부르는 자리뿐이다 — 찍기(`ma`), 그리로 가는 motion(`'a` `` `a ``, motion.go 의 motionMark),
// 명령줄 주소(`:'a`, command-range.go), 목록(`:marks`, view-marks.go).

// actionSetMark 는 `m` 과 이름 하나다. 커서 자리를 그 이름으로 찍는다.
//
// **알리지 않는다.** 찍은 것은 곧바로 쓸 일이 없어서 알림이 오히려 다음 알림을 가린다.
// vim 도 조용하다. 무엇이 어디 찍혔는지는 `:marks` 로 본다.
type actionSetMark struct {
	name string
}

func (c actionSetMark) run(e *editor) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()
	buf.SetMark(c.name, buf.Cursor)

	return nil, nil
}

// refuseMissingMark 는 찍지 않은 mark 로 가려 하면 알리고 true 다.
//
// motion 은 buffer 만 받아서(motion.go) 알릴 자리가 없다. 그래서 동작이 motion 을 부르기
// 전에 여기서 묻는다. 알리지 않으면 `'a` 가 그냥 안 먹은 키로 읽힌다 — `%` 가 짝을 못 찾았을
// 때 알리는 것과 같은 까닭이다(action.go 의 actionMove).
func (e *editor) refuseMissingMark(mo motion) bool {
	mark, ok := mo.(motionMark)
	if !ok {
		return false
	}

	if _, set := e.activeBuffer().Mark(mark.name); set {
		return false
	}

	e.notify("찍지 않은 mark 입니다: " + mark.name)

	return true
}
