package syntax

// shellExpansionEnd 는 `$` 로 시작하는 값 참조가 끝나는 자리다.
//
// shell 과, shell 을 품은 Makefile·Dockerfile 이 같이 쓴다. 셋 다 `$(...)`·`${...}`·`$VAR`
// 모양이 같다. 다만 각자만 쓰는 것(make 의 `$@`, docker 의 `${VAR:-기본값}`) 이 있어서
// 낱말 표는 언어별 파일에 남기고 이 모양 찾기만 여기 둔다.
//
// **이 파일은 언어가 아니다.** 여러 언어가 같이 쓰는 조각 찾기다. shell 언어 자체는 shell.go 다.
//
// `$$` 는 값이 아니라 `$` 한 글자를 내는 것이라 참조가 아니다.
func shellExpansionEnd(line []byte, at, limit int) (int, bool) {
	if at+1 >= limit {
		return 0, false
	}

	switch line[at+1] {
	case '$':
		return 0, false
	case '(':
		return shellClosingEnd(line, at+2, limit, ')')
	case '{':
		return shellClosingEnd(line, at+2, limit, '}')
	}

	// `$VAR` 처럼 괄호 없이 이어지는 이름이다. 한 글자짜리 특수 이름(`$@` `$<`) 도 여기 든다.
	end := at + 1
	for end < limit && shellNameByte(line[end]) {
		end++
	}
	if end > at+1 {
		return end, true
	}

	return at + 2, true
}

// shellClosingEnd 는 닫는 괄호까지다. 닫히지 않으면 참조로 보지 않는다.
func shellClosingEnd(line []byte, from, limit int, closing byte) (int, bool) {
	for at := from; at < limit; at++ {
		if line[at] == closing {
			return at + 1, true
		}
	}

	return 0, false
}

func shellNameByte(b byte) bool {
	switch {
	case b >= '0' && b <= '9', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b == '_':
		return true
	}

	return false
}

// quotedEnd 는 따옴표 안이 끝나는 자리다. 닫히지 않으면 문자열로 보지 않는다 —
// 줄을 넘는 문자열이 없는 언어들이 이것을 쓴다.
func quotedEnd(line []byte, at, limit int) (int, bool) {
	quote := line[at]

	for i := at + 1; i < limit; i++ {
		switch line[i] {
		case '\\':
			i++
		case quote:
			return i + 1, true
		}
	}

	return 0, false
}
