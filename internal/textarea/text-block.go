package textarea

// textBlock 은 글에서 떼어 낸 한 덩이다. 지우기·복사가 내놓고 붙여넣기가 받는다.
//
// **줄 단위였는지를 같이 든다.** 붙여넣기가 줄로 넣을지 글자로 넣을지가 그것으로 갈리는데,
// 떼어 낸 뒤에는 줄들만 보아서는 알 수 없다 — 한 줄을 통째로 복사한 것과 그 줄의 글자를
// 처음부터 끝까지 고른 것이 같은 모양이다(ADR-0017).
//
// # register 와 무엇이 다른가
//
// `register` 는 이것을 embed 하고 **말**을 얹은 것이다 — 몇 자인지 세고 무엇을 알릴지
// 정한다(register.go). 그쪽은 「어느 칸에 담는가」와 함께 editor 의 것이고, 이쪽은 글에서
// 떼어 낸 자료라 글을 다루는 겹에 남는다.
//
// 나눈 까닭은 `deleteRange`·`yankRange`·`pasteAfter` 가 **자료만 쓰기 때문**이다. 붙여넣는
// 데 필요한 것은 자리와 글과 줄 단위인지이고, 그것이 어느 register 에서 왔는지는 부르는
// 쪽의 일이다.
type TextBlock struct {
	Lines    [][]byte // 줄 단위면 그 줄들, 글자 단위면 조각을 줄로 끊은 것
	Linewise bool
}

// filled 는 담긴 것이 있는지다.
func (block TextBlock) Filled() bool {
	return len(block.Lines) > 0
}
