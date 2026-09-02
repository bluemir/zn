package core

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

// 대소문자를 바꾸는 자리다. normal 의 `~` 와 visual 의 `~`·`u`·`U` 가 쓴다.
//
// 지우고 넣는 것이 아니라 그 자리 글자를 갈아끼운다 — `r` 과 같은 갈래다(buffer-replace.go).
// 다만 넣을 글자를 손이 대는 것이 아니라 **있던 글자에서 나온다**. 그래서 줄 수도 글자 수도
// 바뀌지 않는다.

// caseKind 는 어느 쪽으로 맞추는가다.
type caseKind int

const (
	caseToggle caseKind = iota // 대문자는 소문자로, 소문자는 대문자로
	caseUpper                  // 다 대문자로
	caseLower                  // 다 소문자로
)

// changeCaseChars 는 커서부터 count 글자를 바꾸고 커서를 그 다음 글자로 옮긴다. vim 의 `~` 다.
//
// **줄에 남은 글자가 count 보다 적으면 있는 만큼만 바꾼다.** 같은 자리의 `r` 은 반대로
// 아무것도 바꾸지 않는데(buffer-replace.go), 이쪽은 vim 이 그렇다(vim 9.1 로 쟀다).
// `r` 은 넣을 글자를 손이 대므로 「몇 개」가 친 것의 뜻에 들지만, 뒤집기는 있는 것을 뒤집는
// 것이라 줄 끝에서 멈춰도 친 것과 어긋나지 않는다.
//
// **줄을 넘지 않는다.** 이것도 vim 과 같다. 커서는 마지막으로 바꾼 글자 **다음** 이고,
// 줄 끝을 넘으면 normal 커서 자리로 당겨진다(clampToNormal).
func (buf *viewport) changeCaseChars(kind caseKind, count, width int) {
	line := buf.lines[buf.cursor.line]

	end := clusterEndClamped(line, buf.cursor.col, max(count, 1))
	changed := applyCase(line[buf.cursor.col:end], kind)

	if !bytes.Equal(changed, line[buf.cursor.col:end]) {
		next := concat(concat(line[:buf.cursor.col], changed), line[end:])

		// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. `r` 과 같은 자리다.
		buf.endEdit()
		buf.beginEdit(buf.cursor.line, 1)
		buf.replaceLines(buf.cursor.line, 1, [][]byte{next})
		buf.endEdit()
	}

	// 커서는 바뀐 것이 없어도 옮긴다. `~` 는 훑어 가는 키라 대소문자가 없는 글자
	// (한글·문장부호) 위에서도 오른쪽으로 간다. vim 과 같다.
	buf.cursor.col += len(changed)
	buf.updateDesiredCol(width)
	buf.clampToNormal(width)
}

// changeCaseRange 는 고른 범위의 대소문자를 바꾼다. visual 의 `~`·`u`·`U` 다.
//
// 줄 단위면 걸친 줄 전체이고 글자 단위면 고른 자리만이다. 커서는 범위의 시작으로 간다 —
// 복사(`y`) 와 같은 길이다(moveToRangeStart).
func (buf *viewport) changeCaseRange(area motionRange, kind caseKind, width int) {
	next := make([][]byte, 0, area.end.line-area.start.line+1)
	same := true

	for i := area.start.line; i <= area.end.line; i++ {
		line := buf.lines[i]

		start, end := 0, len(line)
		if !area.linewise {
			if i == area.start.line {
				start = area.start.col
			}
			if i == area.end.line {
				end = min(area.end.col, len(line))
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

	buf.endEdit()
	buf.beginEdit(area.start.line, len(next))

	// 줄 수가 그대로라 growEdit 은 부르지 않는다.
	buf.replaceLines(area.start.line, len(next), next)
	buf.endEdit()
}

// applyCase 는 글 한 덩이의 대소문자를 바꾼다.
//
// 대소문자가 없는 글자(한글·한자·문장부호) 는 그대로 지나간다 — unicode 가 그런 글자에는
// 같은 값을 돌려준다. 그래서 여기서 가려낼 것이 없다.
//
// **글자로 읽히지 않는 byte 는 건드리지 않고 옮긴다.** 편집기가 여는 파일이 언제나 올바른
// UTF-8 인 것은 아니고, 깨진 자리를 바꾸는 것은 이 키가 부탁받은 일이 아니다.
//
// 바뀐 글자의 byte 길이가 달라질 수 있어서(`ı` 는 2 byte, `I` 는 1 byte) 부르는 쪽은
// 원래 길이가 아니라 **돌아온 것의 길이**로 커서를 잡는다.
func applyCase(text []byte, kind caseKind) []byte {
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
func caseOf(char rune, kind caseKind) rune {
	switch kind {
	case caseUpper:
		return unicode.ToUpper(char)
	case caseLower:
		return unicode.ToLower(char)
	}

	// 뒤집기다. 대문자가 아닌 것을 다 대문자로 보내므로 대소문자가 없는 글자는 제자리다.
	if unicode.IsUpper(char) {
		return unicode.ToLower(char)
	}

	return unicode.ToUpper(char)
}

// clusterEndClamped 는 col 에서 글자 n 개 뒤의 offset 이다. 줄에 그만큼 없으면 줄 끝이다.
//
// 모자라면 false 를 주는 clusterEnd(buffer-replace.go) 와 갈리는 자리다. `r` 은 모자라면
// 아무것도 하지 않아야 하고 `~` 는 있는 만큼 해야 한다 — 어느 쪽인지는 부르는 키가 정한다.
func clusterEndClamped(line []byte, col, n int) int {
	for range n {
		if col >= len(line) {
			break
		}

		col += glyphSize(line, col)
	}

	return col
}
