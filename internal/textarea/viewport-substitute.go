package textarea

// `:s` 가 파일을 건드리는 자리다. 무엇을 바꿀지는 substitution 이 정하고(substitute.go)
// 여기는 그것을 buffer 에 얹는다 (ADR-0084).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-substitute.go 에 있다 (ADR-0121).

// ReplaceRun 은 [at, at+len(next)) 를 next 로 갈아끼우고 **한 번의 되돌리기로 묶는다.**
//
// 손이 한 번 친 것은 한 번의 `u` 로 돌아가야 한다(ADR-0046). 줄 수가 그대로인 대량 변경이
// 이 문을 쓴다 — 치환·표 맞추기·줄 끝 공백 떼기가 그렇다.
//
// **앞의 타이핑 구간을 먼저 닫는다.** 섞이면 `u` 한 번에 남의 편집까지 딸려온다.
//
// 새 줄을 짓는 것은 부르는 쪽이다. 무엇으로 바꿀지가 언어나 명령의 일이라 창이 알 것이
// 아니다 (ADR-0126, ADR-0128).
func (viewport *Viewport) ReplaceRun(at int, next [][]byte) {
	viewport.EndEdit()
	viewport.BeginEdit(at, len(next))
	viewport.ReplaceLines(at, len(next), next)
	viewport.EndEdit()
}
