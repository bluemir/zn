package core

import "github.com/bluemir/zn/internal/scheme"

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
// # 무엇을 들고 무엇을 안 드나
//
// **드는 것**
//
//   - **글.** Buffer 를 embed 한다
//   - **그 글의 어디를 보고 있나.** 커서·`top`·`topRow`·`desiredX`·고른 범위
//   - **받은 크기.** editor 가 배정한 편집 영역의 폭과 높이다(viewSize)
//   - **본문 앞 칸의 폭.** gutterWidth·lineNumberDigits 다. 마커 두 칸과 번호 칸을 합친
//     것이고, 자릿수가 이 파일의 줄 수에서 나오므로 창이 잰다
//   - **본문 폭.** 받은 크기에서 그 칸을 뺀 나머지다(contentWidth). 줄을 어디서 접을지가
//     이 값으로 정해진다
//
// **안 드는 것**
//
//   - **크기를 정하는 일.** sidebar 를 열면 좁아지고 판이 열리면 낮아지는데, 그것은 화면
//     전체를 나누는 결정이라 창이 알 수 없다. editor 가 정해서 밀어 넣는다(layoutViews)
//   - **본문 앞 칸을 그리는 일.** 무엇을 그릴지는 색과 마커 내용이 필요해서 layout.go 의
//     renderGutter 가 한다. 창은 **몇 칸인지**만 안다
//   - **tabline·statusBar·sidebar·판.** 편집 영역 밖이라 editor 의 것이다
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

	// size 는 editor 가 이 창에 준 편집 영역이다. 그것이 바뀔 때 밀려 들어온다(layoutViews).
	size viewSize

	// viewPlace 는 커서와 화면 자리다. 담아 두었다가 되돌리는 단위이기도 하다(place·moveToPlace).
	viewPlace

	// desiredX 는 위아래로 움직일 때 지킬 열이다. **화면 행 안에서 센 칸이다**(ADR-0108).
	//
	// 그렇게 둔 것은 `↑`/`↓` 가 화면 행 단위라서다(ADR-0006, ADR-0076). 대가로 wrap 된 줄의
	// 둘째 행 이후에서 `j`/`k` 를 누르면 다음 줄의 첫 화면 행에 선다. vim 은 이 칸을 줄
	// 시작에서 세므로 그 자리에서 다르고, 그대로 두기로 정했다(placeCursorInLine).
	//
	// viewPlace 에 안 든 것은 **담는 값이 아니기 때문**이다. 되돌린 뒤 다시 잰다.
	desiredX int

	// selection 은 visual mode 가 고른 범위의 반대쪽 끝이다. 이쪽 끝은 커서다(selection.go).
	selection selection
}

// viewPlace 는 「이 파일의 어디를 보고 있나」다. 커서와 화면 자리를 함께 담는다.
//
// **viewport 가 이것을 embed 한다.** 그래서 이 넷은 지금 보고 있는 자리이면서, 담아 두었다가
// 되돌릴 수 있는 한 덩어리다. 담는 자리(검색 무르기·치환 그만두기) 가 넷을 따로 들던 것을
// 이 type 이 걷었고, 창 쪽도 같은 덩어리로 묶었다.
//
// **넷이 함께여야 한다.** 커서만 되돌리면 보이는 곳이 달라진 채로 남는다.
//
// 되돌리는 것은 창이 한다(아래 moveToPlace). 커서를 쓰는 일이라 `desiredX` 을 다시 맞추는
// 것까지 안에서 끝나야 한다(ADR-0100).
//
// **`desiredX` 과 `selection` 은 여기 없다.** 앞엣것은 담는 것이 아니라 되돌린 뒤 다시
// 재는 파생값이고, 뒤엣것은 고른 범위라 「어디를 보고 있나」와 갈래가 다르다.
type viewPlace struct {
	cursor scheme.Cursor
	top    viewTop
}

// viewTop 은 화면 맨 위에 그릴 자리다.
//
// **커서에서 파생할 수 없다.** 커서를 두고 화면만 움직이는 동작이 있고, 커서가 화면 안에
// 있는 동안은 화면이 움직이지 않아야 한다.
//
// **논리 줄과 그 안의 행으로 센다.** 「파일 앞에서부터 몇 번째 화면 행」으로 담으면 폭이 한
// 칸만 바뀌어도 통째로 틀리고, 다시 세려면 파일 앞부분을 처음부터 훑어야 한다. 논리 줄은
// 폭과 무관하므로 그것을 닻으로 삼고 그 안에서만 행을 센다 — 폭이 바뀌면 row 만 다시 재면
// 되고 line 은 그대로다 (ADR-0122).
type viewTop struct {
	line int // lines 의 index
	row  int // 그 줄의 몇 번째 wrap 행부터 그리는지
}

// viewSize 는 editor 가 창에 배정한 크기다. **본문 앞 칸을 아직 떼지 않은 편집 영역 전체**다.
//
// **`textAndDrawerHeight` 와 딱 `drawerHeight` 만큼 다르다.** 그쪽은 아래 판을 아직 안고
// 있고 이쪽은 뗀 나머지다. 판이 닫혀 있는 동안은 둘이 같아서 어긋남이 안 보인다.
//
// # 왜 필드인가
//
// 창은 이미 폭에서 나온 값을 들고 있다. `viewTop.row`(그 줄의 몇 번째 wrap 행) 도
// `desiredX`(화면 행 안에서 센 칸) 도 폭이 없으면 뜻이 없는 값인데, 정작 그 값들을 만든 폭만
// 밖에 있었다. 메서드 71 개가 그것을 인자로 이어 날랐고 **그중 52 개는 쓰지도 않고 아래로
// 넘기기만 했다** (ADR-0123).
//
// # 누가 넣나
//
// **editor 다.** sidebar 를 열면 좁아지고 판이 열리면 낮아지는데, 그것은 화면 전체를 나누는
// 결정이라 창이 알 수 없다. 바뀌는 자리에서 `layoutViews` 로 밀어 넣는다(layout.go).
//
// **줄 수는 여기 없다.** 자릿수가 늘면 본문 폭이 줄지만 그것은 창이 스스로 아는 것이라
// `contentWidth` 가 그때그때 잰다. 편집할 때마다 밀어 넣을 것이 없다.
type viewSize struct {
	width  int // 편집 영역 너비. 화면에서 sidebar 를 뗀 나머지다
	height int // 편집 내용을 그릴 높이. tabline·statusBar·판을 뗀 나머지다
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
// **`desiredX` 을 다시 맞춘다.** 커서를 옮기는 자리라 그 불변이 여기서 끝나야 한다 —
// 밖에서 필드를 직접 쓰면 그 겹이 이것을 같이 져야 하고, 잊으면 되돌린 뒤 `j` 가 엉뚱한
// 칸으로 간다(ADR-0100).
func (buf *viewport) moveToPlace(at viewPlace) {
	buf.viewPlace = at
	buf.updateDesiredCol()
}

// lineNumberDigits 는 절대·상대 번호가 각각 쓰는 자릿수다.
//
// 절대번호는 이 파일의 줄 수까지, 상대번호는 창 높이까지만 커진다. 상대번호는 화면 밖으로
// 나가면 볼 수 없으므로 줄 수와 무관하다.
//
// **줄 수를 아는 것이 창이라 여기서 센다.** 전에는 editor 가 `e.buffers[e.active].lines` 를
// 들여다봐 세고 그만큼 뗀 폭을 창에 돌려주었는데, 의존이 거꾸로 가는 자리였다. 화면 분할이
// 오면 창마다 파일이 달라 자릿수도 달라진다 (ADR-0121).
func (buf viewport) lineNumberDigits() (absolute, relative int) {
	return max(digits(len(buf.lines)), minAbsoluteDigits),
		max(digits(buf.size.height), minRelativeDigits)
}

// gutterWidth 는 이 창에서 본문 앞에 붙는 칸의 폭이다.
//
// 드는 것이 넷이다. **마커 두 칸**(진단·git, ADR-0086·ADR-0094), **절대번호**, **상대번호**,
// 그리고 그 둘 뒤의 빈 칸 하나씩이다. `    1  0 ` 처럼 보인다.
//
// **폭이 한 항이다.** 마커 칸과 번호 칸을 따로 재면 좁은 화면에서 한쪽만 사라질 수 있고,
// 폭을 보는 자리(contentWidth·contentLeft·sticky·시험의 gutterWidthOf) 가 둘을 각각
// 더해야 한다. 한 군데라도 어긋나면 화면 절반만 밀린 상태가 된다(ADR-0086).
func (buf viewport) gutterWidth() int {
	absolute, relative := buf.lineNumberDigits()

	// 칸을 떼고 나면 본문이 남지 않는 좁은 화면에서는 그리지 않는다. sidebar 와 같은 규칙이다.
	// 마커 칸도 여기서 같이 사라진다 — 번호가 없는데 마커만 남으면 그것이 어느 줄의 것인지
	// 셀 수 없다(ADR-0086).
	gutter := markerWidth + absolute + 1 + relative + 1
	if buf.size.width-gutter < minTextWidth {
		return 0
	}

	return gutter
}

// contentWidth 는 이 창에서 파일 내용을 그릴 너비다. 받은 크기에서 본문 앞 칸을 뗀 나머지다.
//
// **줄바꿈·스크롤·커서 계산이 모두 이 값을 쓴다.** 창이 하는 일 대부분이 「이 폭으로 접으면
// 어디에 서나」라서, 전에는 이것이 메서드 71 개에 인자로 실려 다녔다 (ADR-0123).
//
// **그때그때 잰다.** 자릿수가 이 파일의 줄 수에서 나오므로 편집할 때마다 달라질 수 있다.
// 담아 두면 999 줄에서 1000 줄로 넘어가는 순간부터 조용히 한 칸 틀린다.
func (buf viewport) contentWidth() int {
	return max(0, buf.size.width-buf.gutterWidth())
}
