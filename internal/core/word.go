package core

import (
	"unicode"
	"unicode/utf8"
)

// 무엇이 한 단어인지를 정하는 표다. 글자를 세 부류로 가르고(`charClass`) 그 경계가 단어의
// 끝이 된다. 옮기기(buffer-word.go)·지우기·바꾸기·검색이 나눠 쓴다.

// charClass 는 단어를 끊는 글자 부류다. 부류가 바뀌는 자리가 곧 단어 경계다.
//
// vim 과 같이 네 부류로 나눈다. 한글·한자·가나는 단어 글자와 다른 부류라
// `한글abc` 에서 `한글` 과 `abc` 가 각각 한 단어다. 한글 문서에서 영문 단어만
// 건너뛸 수 있어야 하기 때문이다.
type charClass int

const (
	classBlank charClass = iota
	classWord            // 영문·숫자·`_` 와 그 밖의 문자
	classCJK             // 한글·한자·가나
	classPunct           // 나머지 문장부호
)

// wordKind 는 단어를 어떻게 끊는지다. vim 의 word 와 WORD 다.
type wordKind int

const (
	smallWord wordKind = iota // 부류가 바뀌면 끊는다. w b e
	bigWord                   // 공백으로만 끊는다. W B E
)

// clusterClass 는 offset 에서 시작하는 글자의 부류다.
//
// 부류는 첫 rune 으로 정한다. 결합 문자가 뒤에 붙어도 그 글자가 무엇인지는 첫 rune 이 정한다.
func clusterClass(line []byte, offset int, kind wordKind) charClass {
	r, _ := utf8.DecodeRune(line[offset:])

	if r == ' ' || r == '\t' {
		return classBlank
	}
	if kind == bigWord {
		// 큰 단어는 공백으로만 끊으므로 공백이 아닌 것은 전부 한 부류다.
		return classWord
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
