package textarea

import (
	"github.com/bluemir/zn/internal/scheme"
)

// 단어 단위로 커서를 옮긴다. vim 의 `w`·`b`·`e` 다.
// 무엇이 한 단어인지는 glyph-class.go 가 정하고, 여기는 그 경계를 따라 걷는 일만 한다.
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-word.go 에 있다 (ADR-0121).

// moveWordForward 는 다음 단어의 첫 글자로 간다. vim 의 w/W 다.
func (viewport *Viewport) MoveWordForward(n int, kind WordKind) {
	for range n {
		viewport.WordForward(kind)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) WordForward(kind WordKind) {
	at := viewport.Cursor

	// 지금 글자와 같은 부류가 이어지는 동안 앞으로 간다.
	// 공백에서 시작했으면 건너뛸 단어가 없으므로 아래 공백 건너뛰기로 바로 간다.
	class := viewport.ClassAt(at, kind)
	for class != ClassBlank && viewport.ClassAt(at, kind) == class {
		next, ok := viewport.nextPos(at)
		if !ok {
			viewport.Cursor = at

			return
		}

		at = next
	}

	// 공백을 건너뛴다. 빈 줄은 그 자체로 단어라 거기서 멈춘다.
	for viewport.ClassAt(at, kind) == ClassBlank {
		if at.Col == 0 && len(viewport.lines[at.Line]) == 0 && at.Line != viewport.Cursor.Line {
			break
		}

		next, ok := viewport.nextPos(at)
		if !ok {
			break
		}

		at = next
	}

	viewport.Cursor = at
}

// moveWordBackward 는 단어의 첫 글자로 되돌아간다. vim 의 b/B 다.
func (viewport *Viewport) MoveWordBackward(n int, kind WordKind) {
	for range n {
		viewport.wordBackward(kind)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) wordBackward(kind WordKind) {
	at, ok := viewport.prevPos(viewport.Cursor)
	if !ok {
		viewport.Cursor.Col = 0

		return
	}

	// 공백을 거꾸로 건너뛴다. 빈 줄은 그 자체로 단어다.
	for viewport.ClassAt(at, kind) == ClassBlank && len(viewport.lines[at.Line]) > 0 {
		prev, ok := viewport.prevPos(at)
		if !ok {
			break
		}

		at = prev
	}
	if len(viewport.lines[at.Line]) == 0 {
		viewport.Cursor = scheme.Cursor{Line: at.Line}

		return
	}

	// 같은 부류가 시작하는 자리까지 거꾸로 간다.
	class := viewport.ClassAt(at, kind)
	for {
		prev, ok := viewport.prevPos(at)
		if !ok || prev.Line != at.Line || viewport.ClassAt(prev, kind) != class {
			break
		}

		at = prev
	}

	viewport.Cursor = at
}

// moveWordEnd 는 단어의 마지막 글자로 간다. vim 의 e/E 다.
//
// w/b 와 달리 빈 줄에서 멈추지 않는다. 빈 줄에는 끝낼 단어가 없기 때문이다. vim 과 같다.
func (viewport *Viewport) MoveWordEnd(n int, kind WordKind) {
	for range n {
		viewport.WordEnd(kind)
	}

	viewport.UpdateDesiredCol()
}

func (viewport *Viewport) WordEnd(kind WordKind) {
	at, ok := viewport.nextPos(viewport.Cursor)
	if !ok {
		return
	}

	// 공백을 건너뛴다. 끝낼 단어가 더 없으면 제자리에 둔다.
	for viewport.ClassAt(at, kind) == ClassBlank {
		next, ok := viewport.nextPos(at)
		if !ok {
			return
		}

		at = next
	}

	// 같은 부류가 끝나는 자리까지 간다.
	class := viewport.ClassAt(at, kind)
	for {
		next, ok := viewport.nextPos(at)
		if !ok || next.Line != at.Line || viewport.ClassAt(next, kind) != class {
			break
		}

		at = next
	}

	viewport.Cursor = at
}
