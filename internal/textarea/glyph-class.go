package textarea

import (
	"unicode"
	"unicode/utf8"
)

// 무엇이 한 단어인지를 정하는 표다. 글자를 네 부류로 가르고(`charClass`) 그 경계가 단어의
// 끝이 된다. core 의 옮기기(buffer-word.go)·지우기·바꾸기·검색이 나눠 쓴다.

// charClass 는 단어를 끊는 글자 부류다. 부류가 바뀌는 자리가 곧 단어 경계다.
//
// vim 과 같이 네 부류로 나눈다. 한글·한자·가나는 단어 글자와 다른 부류라
// `한글abc` 에서 `한글` 과 `abc` 가 각각 한 단어다. 한글 문서에서 영문 단어만
// 건너뛸 수 있어야 하기 때문이다.
type charClass int

const (
	ClassBlank charClass = iota
	classWord            // 영문·숫자·`_` 와 그 밖의 문자
	classCJK             // 한글·한자·가나
	classPunct           // 나머지 문장부호
)

// glyphClass 는 offset 에서 시작하는 글자의 부류다.
//
// 부류는 첫 rune 으로 정한다. 결합 문자가 뒤에 붙어도 그 글자가 무엇인지는 첫 rune 이 정한다.
//
// **`w` 와 `W` 를 여기서 가르지 않는다.** 큰 단어(`W`) 는 공백이 아닌 셋을 한 부류로 접는데,
// 그것은 글자가 무엇인지가 아니라 그 키의 규칙이다. 접는 일은 core 가 한다
// (`Buffer.classAt`, ADR-0100 §3, ADR-0117).
func glyphClass(line []byte, offset int) charClass {
	r, _ := utf8.DecodeRune(line[offset:])

	if r == ' ' || r == '\t' {
		return ClassBlank
	}

	switch {
	case r == '_' || unicode.IsDigit(r):
		return classWord
	case unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana):
		return classCJK
	case unicode.IsLetter(r):
		return classWord
	default:
		return classPunct
	}
}
