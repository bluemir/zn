package core

import (
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// lineRange 는 명령 이름 앞에 붙은 줄 범위다. `:1,5d` 의 `1,5` 다.
//
// zero 는 "치지 않았다" 는 뜻이다. 그래서 명령마다 범위 없이 칠 때의 기본값을 스스로 정할 수
// 있고, 범위를 받지 않는 명령은 붙은 것을 알아채고 거절할 수 있다.
type lineRange struct {
	from, to lineAddress
}

// lineAddress 는 범위의 한쪽 끝이다. 아직 줄 번호가 아니다 —
// `.` 와 `$` 가 몇 번째 줄인지는 buffer 를 봐야 알고, 그것은 실행할 때다(resolve).
type lineAddress struct {
	base addressBase
	line int // base 가 addressNumber 일 때의 줄 번호. 사람이 치는 대로 1 부터다
	// offset 은 `+3` `-2` 처럼 base 에서 옮긴 줄 수다.
	offset int
}

// addressBase 는 주소가 어디서 시작하는지다.
type addressBase int

const (
	// addressNone 은 base 를 치지 않은 것이다. `:+3d` 의 앞자리이고, 셀 때는 커서 줄이다.
	// zero 가 이것이어야 lineRange 의 zero 가 "범위를 치지 않았다" 로 읽힌다.
	addressNone   addressBase = iota
	addressCursor             // `.` 이거나 쉼표 한쪽이 빈 것(`:,5d`)
	addressNumber             // `42`
	addressLast               // `$`
)

// parseLineRange 는 범위 토큰 안쪽을 뜯는다. `1,5` `%` `.` `.,+3` `-2,.` `,5` 가 온다.
//
// 이 안쪽 문법을 읽는 자리는 여기 하나다. 글자 단위 기계가 붙여준 갈래는 "이것이 범위다"
// 까지이고, 주소 둘로 갈라 읽는 것은 범위 자신의 문법이다.
func parseLineRange(text string) (lineRange, error) {
	// `%` 는 파일 전체다. `1,$` 를 손으로 치는 것과 같고, 주소 자리에는 쓸 수 없다.
	if text == "%" {
		return lineRange{
			from: lineAddress{base: addressNumber, line: 1},
			to:   lineAddress{base: addressLast},
		}, nil
	}

	first, second, split := strings.Cut(text, ",")

	from, err := parseLineAddress(first)
	if err != nil {
		return lineRange{}, err
	}

	// 주소가 하나면 그 줄 하나다.
	if !split {
		return lineRange{from: from, to: from}, nil
	}

	to, err := parseLineAddress(second)
	if err != nil {
		return lineRange{}, err
	}

	return lineRange{from: from, to: to}, nil
}

// parseLineAddress 는 범위의 한쪽 끝을 뜯는다. `42` `.` `$` 에 `+3` `-2` 가 붙을 수 있다.
//
// 비어 있으면 커서 줄이다 — `:,5d` 의 앞자리와 `:1,d` 의 뒷자리다.
func parseLineAddress(text string) (lineAddress, error) {
	addr := lineAddress{}
	rest := text

	switch {
	case rest == "":
		addr.base = addressCursor
	case rest[0] == '.':
		addr.base, rest = addressCursor, rest[1:]
	case rest[0] == '$':
		addr.base, rest = addressLast, rest[1:]
	case rest[0] >= '0' && rest[0] <= '9':
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}

		line, err := strconv.Atoi(rest[:digits])
		if err != nil {
			return lineAddress{}, errors.Newf("줄 번호가 너무 큽니다: %s", rest[:digits])
		}

		addr.base, addr.line, rest = addressNumber, line, rest[digits:]
	}

	// 남은 것은 자리 옮김이다. base 를 치지 않았으면 커서 줄에서 옮기는 것이다(`:+3d`).
	if rest == "" {
		return addr, nil
	}

	sign := 1
	switch rest[0] {
	case '+':
	case '-':
		sign = -1
	default:
		return lineAddress{}, errors.Newf("범위를 알 수 없습니다: %s", text)
	}
	rest = rest[1:]

	// 숫자를 붙이지 않은 `+` 는 한 줄이다. vim 과 같다.
	step := 1
	if rest != "" {
		n, err := strconv.Atoi(rest)
		if err != nil {
			return lineAddress{}, errors.Newf("범위를 알 수 없습니다: %s", text)
		}
		step = n
	}

	addr.offset = sign * step

	return addr, nil
}

// resolve 는 범위를 buffer 의 줄 자리로 바꾼다. 0 부터 세고 양끝을 포함한다.
//
// 뒤집힌 범위(`:5,1d`) 는 되묻지 않고 바로잡는다. vim 은 확인창을 띄우지만 `:5,1d` 가 뜻할 수
// 있는 것은 1 부터 5 줄 하나뿐이고, 잘못 쳤으면 `u` 로 되돌린다.
func (r lineRange) resolve(buf Buffer) (from, to int, err error) {
	from, err = r.from.resolve(buf)
	if err != nil {
		return 0, 0, err
	}

	to, err = r.to.resolve(buf)
	if err != nil {
		return 0, 0, err
	}

	if from > to {
		from, to = to, from
	}

	return from, to, nil
}

// resolve 는 주소를 buffer 의 줄 자리로 바꾼다.
//
// 없는 줄을 가리키면 오류다. 끝으로 잘라 주지 않는다 — `:1,500d` 를 조용히 파일 전체로 읽으면
// 손이 미끄러진 것과 시킨 것을 가를 수 없다. vim 도 여기서 거절한다.
func (a lineAddress) resolve(buf Buffer) (int, error) {
	base := buf.cursorLine

	switch a.base {
	case addressNumber:
		// 사람이 치는 줄 번호는 1 부터고 buffer 는 0 부터다.
		base = a.line - 1
	case addressLast:
		base = len(buf.lines) - 1
	}

	line := base + a.offset
	if line < 0 || line >= len(buf.lines) {
		return 0, errors.Newf("그런 줄이 없습니다: %d", line+1)
	}

	return line, nil
}
