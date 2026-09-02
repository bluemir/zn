package textarea

// indentDirection 은 범위를 미는 쪽이다. `searchDirection` 과 같은 손이다.
type IndentDirection int

const (
	IndentRight IndentDirection = iota // `>`
	IndentLeft                         // `<`
)

// concat 은 두 조각을 이은 새 것이다.
//
// `append(a, b...)` 를 그대로 쓸 수 없다. 여기서 이어 붙이는 앞쪽은 거의 언제나 Buffer 가 든
// read-only data 의 subslice 라, 남는 자리가 있으면 append 가 **다음 줄 위에 쓴다**(ADR-0001).
func concat(head, tail []byte) []byte {
	out := make([]byte, 0, len(head)+len(tail))
	out = append(out, head...)

	return append(out, tail...)
}
