package textarea

// wordKind 는 단어를 어떻게 끊는지다. vim 의 word 와 WORD 다.
//
// **키의 개념이라 glyph-class.go 가 아니라 여기 있다.** `glyphClass` 는 「이 글자가 무슨
// 부류인가」만 답하고, 「`W` 는 공백으로만 끊는다」는 이 갈래의 규칙은 `Buffer.classAt` 이
// 편다 (ADR-0100 §3).
//
// 정하는 것은 키를 읽는 자리(normal-key-parser.go) 이고, motion 이 그것을 담아 나른다.
type WordKind int

const (
	WordSmall WordKind = iota // 부류가 바뀌면 끊는다. w b e
	WordBig                   // 공백으로만 끊는다. W B E
)
