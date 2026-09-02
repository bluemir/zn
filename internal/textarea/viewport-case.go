package textarea

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/bluemir/zn/internal/scheme"
)

// 대소문자를 바꾸는 자리다. normal 의 `~` 와 visual 의 `~`·`u`·`U` 가 쓴다.
//
// 지우고 넣는 것이 아니라 그 자리 글자를 갈아끼운다 — `r` 과 같은 갈래다(viewport-replace.go).
// 다만 넣을 글자를 손이 대는 것이 아니라 **있던 글자에서 나온다**. 그래서 줄 수는 바뀌지 않고
// 글자 수도 그대로다. byte 수는 달라질 수 있다 — `ı`(2) 가 `I`(1) 이 된다.
//
// **문이 하나다.** 범위를 받아 그 안을 바꾼다. 「어느 범위인가」는 키의 규칙이라 밖에 있다 —
// normal 의 `~` 는 motionRight 가 낸 범위를 넘기고(action.go) visual 은 고른 범위를 넘긴다.

// caseKind 는 어느 쪽으로 맞추는가다.
type CaseKind int

const (
	CaseToggle CaseKind = iota // 대문자는 소문자로, 소문자는 대문자로
	CaseUpper                  // 다 대문자로
	CaseLower                  // 다 소문자로
)

// ChangeCaseRange 는 그 범위의 대소문자를 바꾼다. `~`·`u`·`U` 가 다 여기로 온다.
//
// 줄 단위면 걸친 줄 전체이고 글자 단위면 고른 자리만이다.
//
// **커서를 옮기지 않는다.** 어디에 설지가 키마다 다르다 — visual 은 범위의 시작으로 가고
// (moveToRangeStart) normal 의 `~` 는 훑어 가듯 오른쪽으로 간다. 부르는 쪽이 정한다.
func (viewport *Viewport) ChangeCaseRange(area scheme.MotionRange, kind CaseKind) {
	next := make([][]byte, 0, area.End.Line-area.Start.Line+1)
	same := true

	for i := area.Start.Line; i <= area.End.Line; i++ {
		line := viewport.Lines[i]

		start, end := 0, len(line)
		if !area.Linewise {
			if i == area.Start.Line {
				start = area.Start.Col
			}
			if i == area.End.Line {
				end = min(area.End.Col, len(line))
			}
		}

		changed := applyCase(line[start:end], kind)
		if !bytes.Equal(changed, line[start:end]) {
			same = false
		}

		next = append(next, concat(concat(line[:start], changed), line[end:]))
	}

	// 바뀐 것이 없으면 손대지 않는다. 그냥 갈아끼우면 dirty 가 서고 redo 가 날아간다 —
	// replaceIndented 와 같은 자리다(buffer-indent.go).
	if same {
		return
	}

	viewport.EndEdit()
	viewport.BeginEdit(area.Start.Line, len(next))

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	viewport.ReplaceLines(area.Start.Line, len(next), next)
	viewport.EndEdit()
}

// applyCase 는 글 한 덩이의 대소문자를 바꾼다.
//
// 대소문자가 없는 글자(한글·한자·문장부호) 는 그대로 지나간다 — unicode 가 그런 글자에는
// 같은 값을 돌려준다. 그래서 여기서 가려낼 것이 없다.
//
// **글자로 읽히지 않는 byte 는 건드리지 않고 옮긴다.** 편집기가 여는 파일이 언제나 올바른
// UTF-8 인 것은 아니고, 깨진 자리를 바꾸는 것은 이 키가 부탁받은 일이 아니다.
//
// **rune 을 1:1 로 옮긴다.** 글자 수는 그대로이고 byte 수만 달라질 수 있다
// (`ı` 는 2 byte, `I` 는 1 byte). 그래서 바꾼 뒤에 커서를 잡는 쪽은 바꾸기 전 좌표를
// 쓰지 말고 새 줄에서 다시 세야 한다(action.go 의 actionChangeCase).
func applyCase(text []byte, kind CaseKind) []byte {
	next := make([]byte, 0, len(text))

	for i := 0; i < len(text); {
		char, size := utf8.DecodeRune(text[i:])
		if char == utf8.RuneError && size == 1 {
			next = append(next, text[i])
			i += size

			continue
		}

		next = utf8.AppendRune(next, caseOf(char, kind))
		i += size
	}

	return next
}

// caseOf 는 글자 하나를 어느 쪽으로 맞춘 것이다.
func caseOf(char rune, kind CaseKind) rune {
	switch kind {
	case CaseUpper:
		return unicode.ToUpper(char)
	case CaseLower:
		return unicode.ToLower(char)
	}

	// 뒤집기다. 대문자가 아닌 것을 다 대문자로 보내므로 대소문자가 없는 글자는 제자리다.
	if unicode.IsUpper(char) {
		return unicode.ToLower(char)
	}

	return unicode.ToUpper(char)
}
