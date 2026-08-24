package core

// 커서 옆에 뜨는 작은 창을 놓는 자리다. 자동완성 목록(ADR-0066) 과 이름 바꾸기 입력창
// (ADR-0067) 이 같이 쓴다.
//
// 그리는 것은 여기 없다. 창 안에 무엇을 적는지는 창마다 다르고, 같은 것은 「커서 옆 어디에
// 놓는가」 하나뿐이다.

// popupPos 는 창을 놓을 화면 좌표다.
//
// **커서 바로 아래가 제자리다.** 치고 있는 글자 옆에 붙어야 무엇에 대한 창인지가 보인다.
// 아래에 자리가 없으면 커서 위로 올린다 — 파일 끝을 고칠 때가 그 자리다. 위아래 둘 다
// 모자라면 아래로 두고 화면 밖으로 나가는 만큼만 끌어올린다.
//
// 오른쪽은 화면 안으로 밀어 넣는다. 창을 좁히지 않는다 — 같은 창이 커서 자리에 따라 다른
// 폭으로 뜨면 눈이 그때마다 다시 읽어야 한다.
func popupPos(cursorX, cursorY, boxWidth, rows, width, height int) (left, top int) {
	left = max(0, min(cursorX, width-boxWidth))
	top = cursorY + 1

	// statusBar 두 줄은 덮지 않는다. 저장 문구와 커서 자리가 창에 가리면 안 된다.
	limit := height - statusBarHeight

	if top+rows > limit {
		if above := cursorY - rows; above >= tablineHeight {
			return left, above
		}

		top = max(0, limit-rows)
	}

	return left, top
}
