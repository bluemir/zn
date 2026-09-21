package core

// 트리에도 sticky 머리줄을 붙인다. 화면 위로 올라가 버린 상위 디렉터리를 맨 위 몇 행에
// 덮어써서 「지금 보는 자리가 어느 폴더 안인가」를 늘 보여준다(ADR-0144).
//
// 편집 영역의 머리줄(sticky.go, ADR-0049)과 같은 손이지만 깊이가 어림이 아니다. 저쪽은
// 들여쓰기로 블록을 짐작하는데 여기는 treeRow.depth 가 트리에서 그대로 나온다.

// stickyRows 는 화면 맨 위에 덮어쓸 상위 디렉터리들의 행 번호다. rows() 안의 자리를 가리킨다.
//
// **자리를 떼지 않고 덮어쓴다.** 머리줄이 하나 붙을 때마다 그 아래 트리 행 하나가 가려진다.
// 자리를 떼면 머리줄이 느는 동안 스크롤이 제자리걸음을 한다 — 한 칸 내려도 위에 조상이 한 줄
// 더 서서 화면이 한 글자도 안 바뀐다(ADR-0144).
//
// 세는 규칙은 한 줄이다. **화면 i 번째 행에 올 항목이 깊이 i 보다 깊으면 그 자리에 그 항목의
// 깊이 i 짜리 조상을 세운다.** 깊지 않으면 거기서 멈춘다 — 그 항목은 자기 깊이 자리에 제대로
// 서 있으므로 그 위에 덮을 것이 없다.
func (s sidebar) stickyRows(height int) []int {
	rows := s.rows()

	// 훑는 자리가 곧 덮이는 자리다. 트리 끝까지 굴린 자리에서 마지막 행까지 세면 머리줄이
	// 트리를 통째로 덮어 아무것도 안 남는다 — 한 행 앞에서 멈춘다.
	sticky := []int{}
	for i := 0; s.top+i < len(rows)-1; i++ {
		if rows[s.top+i].depth <= i {
			break
		}

		sticky = append(sticky, ancestorRow(rows, s.top+i, i))
	}

	// 깊게 파묻힌 자리에서 화면이 통째로 머리줄이 되지 않게 절반을 넘지 않는다. 넘치면
	// **바깥쪽부터 버린다** — 가장 안쪽이 방금 화면 위로 사라진 것이라 잃으면 가장 아깝다.
	//
	// 정책이 아니라 자물쇠다. 40 행 화면에서 20 겹 중첩이라야 닿는다(ADR-0049).
	if over := len(sticky) - height/2; over > 0 {
		sticky = sticky[over:]
	}

	return sticky
}

// ancestorRow 는 rows[at] 를 거느리는 깊이 depth 의 행 번호다.
//
// 트리 행은 깊이 우선으로 늘어서 있으므로 at 에서 위로 훑어 처음 만나는 깊이 depth 의 행이
// 곧 그 조상이다. 사이에 낀 행은 모두 그보다 깊어서 걸리지 않는다.
//
// 뿌리가 깊이 0 이고 깊이는 한 번에 한 단계씩만 는다. rows[at] 가 depth 보다 깊으면 찾는 행이
// 반드시 있으므로 마지막 return 에 닿지 않는다.
func ancestorRow(rows []treeRow, at, depth int) int {
	for i := at - 1; i >= 0; i-- {
		if rows[i].depth == depth {
			return i
		}
	}

	return 0
}
