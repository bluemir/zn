package core

// viewport 는 파일 하나를 보고 있는 창이다. 글과 「그 글의 어디를 보고 있나」를 함께 든다.
//
// **`Buffer` 를 embed 한다.** 그래서 `v.lines`·`v.insert`·`v.Save` 처럼 글 쪽 이름이 그대로
// 서고, 부르는 자리는 이 가름을 몰라도 된다.
//
// # 왜 갈랐나
//
// `Buffer` 메서드가 어느 필드를 만지는지 재보니 이렇게 갈렸다.
//
//	글만 쓰거나 읽는다              62 개   → Buffer 에 남는다
//	커서를 쓰면서 글을 읽는다        41 개   → 여기로 왔다
//	커서만 읽거나 쓴다              24 개   → 여기로 왔다
//	글을 쓰면서 커서도 옮긴다         0 개
//
// **마지막 줄이 요점이다.** 편집 메서드가 `lines` 를 직접 쓰지 않고 `replaceLines` 를 부르기
// 때문에, 「글을 고치는 일」과 「커서를 옮기는 일」이 이미 다른 겹에 있었다. 그래서 가르는 데
// 새로 지어낼 규칙이 없었다.
//
// 얽힌 자리는 하나였다. `beginEdit` 이 되돌린 뒤 커서를 놓을 자리를 undo 항목에 담는데,
// 그것만 커서를 인자로 받는다.
//
// # 지금은 tab 과 1:1 이다
//
// 화면 분할이 오면 같은 글을 보는 viewport 가 여럿이 된다. 그때 이 embed 를 포인터로 바꾸면
// `Buffer` 하나를 여럿이 가리킨다. ADR-0100 이 「화면 분할이 오면 밖으로 빼야 한다」고 적어
// 둔 자리가 여기다.
// 이 파일은 type 과 viewport 그 자체를 다루는 것만 든다. 커서를 옮기고 화면을 굴리는
// 메서드 88 개는 갈래별로 `viewport-*.go` 에 나뉘어 있다(buffer.go 의 머리글과 같은 손이다).
type viewport struct {
	Buffer

	// viewPlace 는 커서와 화면 자리다. 담아 두었다가 되돌리는 단위이기도 하다(place·moveToPlace).
	viewPlace

	// desiredCol 은 위아래로 움직일 때 지킬 열이다. **화면 행 안에서 센 칸이다**(ADR-0108).
	//
	// 그렇게 둔 것은 `↑`/`↓` 가 화면 행 단위라서다(ADR-0006, ADR-0076). 대가로 wrap 된 줄의
	// 둘째 행 이후에서 `j`/`k` 를 누르면 다음 줄의 첫 화면 행에 선다. vim 은 이 칸을 줄
	// 시작에서 세므로 그 자리에서 다르고, 그대로 두기로 정했다(placeCursorInLine).
	//
	// viewPlace 에 안 든 것은 **담는 값이 아니기 때문**이다. 되돌린 뒤 다시 잰다.
	desiredCol int

	// selection 은 visual mode 가 고른 범위의 반대쪽 끝이다. 이쪽 끝은 커서다(selection.go).
	selection selection
}

// place 는 지금 보고 있는 자리다. 무르는 자리가 이것을 담아 두었다가 moveToPlace 로 되돌린다.
//
// `Buffer` 가 viewPlace 를 embed 하므로 그 덩어리를 그대로 돌려주면 된다. 값 receiver 라
// 사본이 나가서 담아 둔 뒤에 커서가 움직여도 그것이 따라 바뀌지 않는다.
func (buf viewport) place() viewPlace {
	return buf.viewPlace
}

// moveToPlace 는 담아 둔 자리로 커서와 화면을 되돌린다.
//
// **`desiredCol` 을 다시 맞춘다.** 커서를 옮기는 자리라 그 불변이 여기서 끝나야 한다 —
// 밖에서 필드를 직접 쓰면 그 겹이 이것을 같이 져야 하고, 잊으면 되돌린 뒤 `j` 가 엉뚱한
// 칸으로 간다(ADR-0100).
func (buf *viewport) moveToPlace(at viewPlace, width int) {
	buf.viewPlace = at
	buf.updateDesiredCol(width)
}
