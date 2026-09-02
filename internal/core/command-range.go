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

	// addressSelectStart, addressSelectEnd 는 visual 로 고른 범위의 두 끝(`'<` `'>`) 이다.
	//
	// visual 에서 `:` 로 들어오면 고른 것이 살아 있어서 셀 수 있다. normal 에서 치면 셀
	// 것이 없어서 거절한다 — mark 를 남기지 않았다(ADR-0089).
	addressSelectStart
	addressSelectEnd
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
	case strings.HasPrefix(rest, "'<"):
		addr.base, rest = addressSelectStart, rest[2:]
	case strings.HasPrefix(rest, "'>"):
		addr.base, rest = addressSelectEnd, rest[2:]
	case rest[0] == '\'':
		// `'` 로 시작했는데 `<` `>` 가 아니다. mark 를 넣지 않았으므로 여기서 끝이다
		// (ADR-0089). 아래로 흘리면 「범위를 알 수 없습니다」가 되는데, 무엇이 없는지를
		// 말해 주는 편이 낫다.
		return lineAddress{}, errors.Newf("mark 는 아직 쓸 수 없습니다: %s", text)
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
func (r lineRange) resolve(buf viewport) (from, to int, err error) {
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

// area 는 명령이 일할 자리다. 줄 범위를 `motionRange` 로 바꿔 준다.
//
// **`'<,'>` 를 그대로 친 것이면 고른 범위를 그대로 준다.** 글자로 골랐으면 글자 구간까지
// 담겨서, `:s` 와 `:d` 가 고른 밖을 건드리지 않는다(ADR-0089).
//
// 그 밖의 범위는 줄 단위다. 한쪽만 `'<` 인 것(`:'<,5d`)·자리를 옮긴 것(`:'<,'>+3`) 도 여기다 —
// 사람이 줄 번호를 섞어 넣은 것이라 글자 구간을 지킬 뜻이 없어졌다.
func (r lineRange) area(buf viewport) (motionRange, error) {
	from, to, err := r.resolve(buf)
	if err != nil {
		return motionRange{}, err
	}

	if r.isSelection() {
		if area, ok := buf.selectionRange(); ok {
			return area, nil
		}
	}

	// **손으로 친 범위에는 따라갈 이동이 없다.** 그래서 커서를 옮길지도 여기서 정하지 않는다 —
	// `:y` 가 `isSelection()` 을 보고 가른다. `:1,5y` 는 커서를 1 줄로 끌어가지 않고
	// `:'<,'>y` 만 visual 의 `y` 처럼 범위 시작으로 간다(view-editor-command.go, ADR-0100).
	return motionRange{start: cursor{line: from}, end: cursor{line: to},
		linewise: true}, nil
}

// isSelection 은 범위가 `'<,'>` 그 자체인지다. 자리 옮김이 붙으면 아니다.
func (r lineRange) isSelection() bool {
	return r.from == lineAddress{base: addressSelectStart} &&
		r.to == lineAddress{base: addressSelectEnd}
}

// resolve 는 주소를 buffer 의 줄 자리로 바꾼다.
//
// 없는 줄을 가리키면 오류다. 끝으로 잘라 주지 않는다 — `:1,500d` 를 조용히 파일 전체로 읽으면
// 손이 미끄러진 것과 시킨 것을 가를 수 없다. vim 도 여기서 거절한다.
func (a lineAddress) resolve(buf viewport) (int, error) {
	base := buf.cursor.line

	switch a.base {
	case addressNumber:
		// 사람이 치는 줄 번호는 1 부터고 buffer 는 0 부터다.
		base = a.line - 1
	case addressLast:
		base = len(buf.lines) - 1
	case addressSelectStart, addressSelectEnd:
		// 고른 범위는 Buffer 가 든다(ADR-0037). 그래서 서명이 그대로다.
		area, ok := buf.selectionRange()
		if !ok {
			return 0, errors.New("고른 범위가 없습니다")
		}

		base = area.start.line
		if a.base == addressSelectEnd {
			base = area.end.line
		}
	}

	line := base + a.offset
	if line < 0 || line >= len(buf.lines) {
		return 0, errors.Newf("그런 줄이 없습니다: %d", line+1)
	}

	return line, nil
}
