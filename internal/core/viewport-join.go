package core

// 줄을 이어 붙이는 자리다. normal 의 `J` 가 쓴다(ADR-0087).
//
// 지우기와 반대 방향의 일이지만 지우기 경로를 쓰지 않는다. `J` 는 줄바꿈만 걷어내는 것이
// 아니라 **이은 자리에 공백 규칙이 있다** — 다음 줄의 들여쓰기를 걷고 한 칸을 넣는데, 넣지
// 않는 경우가 넷이다(joinSeparator). 지우기로 흉내 내려면 그 규칙이 지우는 범위 계산 안으로
// 들어가야 한다.
//
// register 에 담지 않는다. 담기는 것은 지운 것이고 여기서는 줄바꿈과 들여쓰기만 사라지는데,
// 그것을 `p` 로 되붙일 자리가 없다. vim 도 담지 않는다(ADR-0058).

// joinLines 는 커서 줄부터 count 줄을 한 줄로 잇는다. vim 의 `J` 다.
//
// **count 는 「이을 줄 수」다.** `3J` 는 세 줄이 한 줄이 되고, 세는 자리가 없는 `J` 와 `1J`
// 와 `2J` 는 모두 두 줄이다(vim 9.1 로 쟀다). 남은 줄이 count 보다 적으면 있는 만큼 잇는다.
//
// 커서는 **마지막으로 이은 자리**에 선다. `3J` 는 셋째 줄이 붙은 자리이고, 그것이 다음에
// 칠 것(`x` 로 공백 지우기, `i` 로 손보기) 이 놓인 자리다. 이것도 vim 과 같다.
func (buf *viewport) joinLines(count int) {
	// 남은 줄로 이을 수 있는 만큼으로 줄인다. 마지막 줄에서는 이을 것이 없어 아무 일도 없다.
	lines := min(max(count, 2), len(buf.lines)-buf.cursor.Line)
	if lines < 2 {
		return
	}

	at := buf.cursor.Line

	joined := buf.lines[at]
	cursor := 0

	for _, next := range buf.lines[at+1 : at+lines] {
		// 다음 줄의 들여쓰기를 걷는다. 공백뿐인 줄은 줄 전체가 들여쓰기라 걷고 나면 빈 줄이
		// 되고, 그러면 이은 자리에 공백도 들어가지 않는다(joinSeparator).
		rest := next[len(leadingBlank(next)):]

		// 이은 자리는 붙기 **전** 의 길이다. 그 자리에 공백이 들어가면 그 공백을 짚고,
		// 안 들어가면 다음 줄의 첫 글자를 짚는다. 둘 다 「이어진 자리」다.
		cursor = len(joined)
		joined = concat(concat(joined, joinSeparator(joined, rest)), rest)
	}

	// 앞의 타이핑 구간에 섞이면 `u` 한 번에 남의 편집까지 딸려온다. `~`·`r` 과 같은 자리다.
	//
	// 이은 것 전부가 한 구간이다. `3J` 를 `u` 한 번으로 되돌린다 — 친 것이 하나였으므로
	// 무르는 것도 하나다.
	buf.endEdit()
	buf.beginEdit(at, lines)
	buf.replaceLines(at, lines, [][]byte{joined})

	// 줄 수가 줄었다고 알린다. 구간이 든 count 는 「편집 **후** 그 자리를 차지하는 줄 수」라
	// 이것을 빼먹으면 `u` 가 없는 줄을 되돌리려 한다(buffer-edit.go 의 revert).
	buf.growEdit(1 - lines)

	buf.endEdit()

	buf.cursor.Col = cursor
	buf.updateDesiredCol()

	// 이은 자리가 줄 끝을 넘을 수 있다. 다음 줄이 비어 있거나 공백뿐이면 붙는 것이 없어서
	// 이은 자리가 곧 줄 끝이다(joinSeparator).
	buf.clampToNormal()
}

// joinSeparator 는 이은 자리에 넣을 것이다. 넣지 않으면 빈 조각이다.
//
// **공백 하나를 넣는 것이 기본이고 넣지 않는 경우가 넷이다.** 앞이 비었을 때, 뒤가 비었을 때,
// 앞이 이미 공백으로 끝났을 때, 뒤가 `)` 로 시작할 때다. 앞 셋은 「없는 곳에 공백을 만들지
// 않는다」 는 한 뜻이고, 마지막 하나는 함수 호출과 괄호를 이어 붙이는 자리라 vim 이 예외로
// 둔 것이다 — `foo(` 와 `)` 를 이으면 `foo()` 여야 한다.
//
// **문장부호(`.` `?` `!`) 뒤에도 하나다.** vim 9.1 은 기본값이 `joinspaces` 켜짐이라 거기서
// 둘을 넣는데(잰 값이다) 그것은 영문 타이프라이터 관습이고, 한국어 본문과 코드에서는 이은
// 자리에만 공백이 둘인 줄이 남는다. neovim 의 기본값과 같은 쪽을 골랐다(ADR-0087).
func joinSeparator(head, tail []byte) []byte {
	if len(head) == 0 || len(tail) == 0 {
		return nil
	}

	if last := head[len(head)-1]; last == ' ' || last == '\t' {
		return nil
	}

	if tail[0] == ')' {
		return nil
	}

	return []byte(" ")
}
