package core

// 범위 하나를 담는 자리다.
//
// **만드는 곳이 넷이다** — motion.go 의 span, buffer-selection.go 의 visual,
// command-range.go 의 줄 범위, cat.go. 받는 곳은 `Buffer` 의 `*Range` 들이다.
// 그 사이를 오가는 자료라 한쪽 파일에 세 들어 살면 의존이 거꾸로 보인다 (ADR-0100).
//
// 앞서 `motion.go` 에 있었다. 한 패키지 안에서는 안 보였지만 `Buffer` 가 열세 곳에서 쓰고
// `motion` 도 `Buffer` 를 쓰므로, 겹을 패키지로 가르면 그 자리가 곧 순환이었다.

// MotionRange 는 operator 가 잡은 범위다. `d` 와 `y` 가 같이 쓴다.
//
// 글자 단위면 (startLine, startCol) 부터 (endLine, endCol) **앞까지** 이고,
// 줄 단위면 [startLine, endLine] 줄 전체다.
//
// **줄 단위여도 칸을 담는다.** 고치는 자리는 그 칸을 보지 않는다 — 줄 단위 연산은 전부
// `linewise` 로 갈라 줄 전체를 쓴다(`deleteLines`·`changeCaseRange`·`selectionOn`·`catLines`).
// 담는 까닭은 **복사가 커서를 범위의 시작으로 옮기기 때문**이다. `yk` 는 칸을 지키고 `ygg` 는
// 첫 비공백으로 가는데, 그 칸이 범위에 없으면 어디서도 만들어 낼 수 없다.
//
// 그래서 이 struct 는 좌표 다섯뿐이고 그 밖의 것을 싣지 않는다. 「범위를 잡은 손이 커서를 둔
// 자리」를 따로 들고 다니던 두 필드가 있었는데, 재 보니 **그것이 늘 범위의 시작이었다** —
// `span` 이 커서 자리와 이동이 닿은 자리로 범위를 짓기 때문이다(charSpan·lineSpan).
// 커서를 옮길지는 부르는 쪽이 정한다(ADR-0100 §5).
// **좌표를 cursor 로 든다.** 넷이 전부 쌍으로만 쓰여서(`textBetween(start, end)` 같은 자리)
// 따로 두면 부르는 쪽이 매번 짝을 맞춰야 하고, `col` 이 byte 인지 화면 칸인지도 이름만으로는
// 갈리지 않았다 (ADR-0122).
type MotionRange struct {
	start, end Cursor
	linewise   bool
}
