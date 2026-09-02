package textarea

// SaveFormat 은 쓰기 직전에 글을 통과시킬 포매터다.
//
// **찾고 깔고 돌리는 것은 밖의 일이다.** 그 명령이 저장소의 `go tool` 인지 PATH 에 있는
// 것인지, 없으면 깔지를 정하는 것은 편집기의 결정이라(core/save-hook.go) 여기가 알 것이
// 아니다. 이쪽이 아는 것은 「이름이 무엇이고 byte 를 넣으면 byte 가 나온다」 둘뿐이다.
//
// 그래서 interface 를 두지 않았다. 부르는 쪽이 하나이고 그것이 자기 것을 이 꼴로 싸서
// 넘기면 된다 (ADR-0128).
type SaveFormat struct {
	// Name 은 알림에 적는 이름이다. 무엇이 내 줄을 고쳤는지 사람이 알아야 한다(ADR-0052).
	Name string

	// Run 은 글 전체를 통과시킨다. 실패하면 저장을 막지 않고 알림만 낸다(ADR-0065).
	Run func(in []byte) ([]byte, error)
}
