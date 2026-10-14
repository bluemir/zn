package textarea

import (
	"bytes"
	"maps"
	"slices"
	"strings"

	"github.com/bluemir/zn/internal/scheme"
)

// 파일 안에 찍어 둔 자리(mark) 다. `ma` 로 찍고 `'a`·`` `a `` 로 돌아온다(ADR-0154).
//
// **Buffer 가 든다.** 찍은 자리는 줄을 따라가야 하고, 줄을 갈아끼우는 자리가 replaceLines
// 하나라 거기서 같이 옮긴다. 문법 캐시가 같은 자리에 걸린 것과 같은 까닭이다(syntax.go).
// 그래서 수명도 buffer 와 같다 — tab 을 닫으면 사라진다.
//
// **바꿀 때마다 새 map 을 짓는다.** 창은 값으로 복사되어 돌아다니는데(motion 의 moveOn) map
// 은 복사해도 같은 것을 가리킨다. 복사본 위에서 줄을 고치면 원본의 mark 가 같이 밀린다.
// 많아야 스물여섯이라 새로 짓는 값이 없다.

// Mark 는 찍어 둔 자리 하나다. 목록에 보일 때 쓴다.
type Mark struct {
	Name string
	At   scheme.Cursor
}

// IsMarkName 은 찍을 수 있는 이름인지다. 소문자 하나다.
//
// 대문자(파일을 넘는 mark) 와 숫자·기호(vim 이 저절로 채우는 자리) 는 받지 않는다(ADR-0154).
func IsMarkName(name string) bool {
	return len(name) == 1 && name[0] >= 'a' && name[0] <= 'z'
}

// SetMark 는 그 이름으로 자리를 찍는다. 이미 있으면 옮긴다.
func (buf *Buffer) SetMark(name string, at scheme.Cursor) {
	next := buf.copyMarks()
	if next == nil {
		next = map[string]scheme.Cursor{}
	}
	next[name] = at
	buf.marks = next
}

// DeleteMark 는 그 이름의 mark 를 지운다. 없으면 아무 일도 없다.
func (buf *Buffer) DeleteMark(name string) {
	next := buf.copyMarks()
	delete(next, name)
	buf.marks = next
}

// Mark 는 그 이름으로 찍은 자리다. 찍지 않았으면 false 다.
//
// **칸은 찍은 그대로다.** 줄은 편집을 따라가지만 줄 안의 칸은 따라가지 않아서, 그 사이에
// 줄이 짧아졌으면 줄 끝 너머를 가리킬 수 있다. 옮기는 쪽이 줄 안으로 끌어온다(MoveTo).
func (buf Buffer) Mark(name string) (scheme.Cursor, bool) {
	at, ok := buf.marks[name]

	return at, ok
}

// Marks 는 찍어 둔 것 전부다. 이름 순이다.
func (buf Buffer) Marks() []Mark {
	list := make([]Mark, 0, len(buf.marks))
	for name, at := range buf.marks {
		list = append(list, Mark{Name: name, At: at})
	}

	slices.SortFunc(list, func(a, b Mark) int {
		return strings.Compare(a.Name, b.Name)
	})

	return list
}

// copyMarks 는 고칠 새 map 이다. 없으면 nil 이다.
func (buf Buffer) copyMarks() map[string]scheme.Cursor {
	return maps.Clone(buf.marks)
}

// shiftMarks 는 [at, at+len(old)) 가 with 로 갈린 뒤 mark 를 그 줄에 맞춰 옮긴다.
//
// **줄 수가 그대로면 아무것도 옮기지 않는다.** 타이핑 한 번이 거의 다 이 길이다.
//
// 줄 수가 바뀌었으면 어느 옛 줄이 어느 새 줄이 되었는지를 알아야 하는데 replaceLines 는 그것을
// 모른다. 그래서 **앞뒤로 같은 줄을 맞댄다.** git 마커가 HEAD 와 견주는 것과 같은 셈이다
// (git-lines.go).
//
//   - 앞에서 같은 줄은 그 자리에 남는다. `o` 가 아래에 줄을 낼 때다
//   - 뒤에서 같은 줄은 늘고 준 만큼 밀린다. `O` 가 위에 줄을 낼 때와 줄 첫 칸의 Enter 다
//   - 가운데는 자리 순으로 짝짓고, 새 줄이 모자라 짝이 없으면 **지운다**. `dd` 로 그 줄이
//     사라진 것이고 vim 도 그렇게 한다
//
// 지운 것은 되돌리기가 되살린다(viewport-edit.go 의 revert).
func (buf *Buffer) shiftMarks(at int, old, with [][]byte) {
	if len(buf.marks) == 0 || len(old) == len(with) {
		return
	}

	head := 0
	for head < len(old) && head < len(with) && bytes.Equal(old[head], with[head]) {
		head++
	}

	tail := 0
	for tail < len(old)-head && tail < len(with)-head &&
		bytes.Equal(old[len(old)-1-tail], with[len(with)-1-tail]) {
		tail++
	}

	delta := len(with) - len(old)

	next := make(map[string]scheme.Cursor, len(buf.marks))
	for name, place := range buf.marks {
		inside := place.Line - at

		switch {
		case inside < head:
			// 위쪽이거나 앞에서 같은 줄이다. 그대로다.
		case inside >= len(old)-tail:
			// 아래쪽이거나 뒤에서 같은 줄이다.
			place.Line += delta
		case inside >= len(with)-tail:
			// 짝이 될 새 줄이 없다. 그 줄이 사라졌다.
			continue
		}

		next[name] = place
	}

	buf.marks = next
}

// restoreMarks 는 편집 전에 담아 둔 mark 중 **지금 없는 것만** 되살린다. 되돌리기가 부른다.
//
// 있는 것은 건드리지 않는다. 지운 뒤 같은 이름을 다른 자리에 다시 찍었으면 그것이 더 새
// 뜻이다. vim 은 되돌릴 때 담아 둔 자리로 덮어쓰는데, 그러면 `u` 가 손으로 찍은 것을
// 무른다(ADR-0154).
func (buf *Buffer) restoreMarks(saved map[string]scheme.Cursor) {
	if len(saved) == 0 {
		return
	}

	next := buf.copyMarks()
	if next == nil {
		next = map[string]scheme.Cursor{}
	}

	for name, at := range saved {
		if _, ok := next[name]; ok {
			continue
		}

		next[name] = at
	}

	buf.marks = next
}

// clampMarks 는 mark 를 줄 수 안으로 끌어온다. 다시 읽은 내용에 옮겨 담을 때 쓴다(Adopt).
//
// 다시 읽기는 무엇이 어디로 갔는지 모르는 갈아끼우기라 줄 번호를 그대로 이어받는다. 커서가
// 그렇게 하는 것과 같다.
func clampMarks(marks map[string]scheme.Cursor, lineCount int) map[string]scheme.Cursor {
	if len(marks) == 0 {
		return nil
	}

	next := make(map[string]scheme.Cursor, len(marks))
	for name, at := range marks {
		at.Line = min(at.Line, lineCount-1)
		next[name] = at
	}

	return next
}
