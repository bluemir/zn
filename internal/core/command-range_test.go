package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 짧게 적는 손들이다. 표가 무엇을 말하는지가 struct 안쪽에 묻히지 않아야 한다.
func addrLine(line int) lineAddress   { return lineAddress{base: addressNumber, line: line} }
func addrDot(offset int) lineAddress  { return lineAddress{base: addressCursor, offset: offset} }
func addrLast(offset int) lineAddress { return lineAddress{base: addressLast, offset: offset} }
func addrRel(offset int) lineAddress  { return lineAddress{offset: offset} }

func TestParseLineRange(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  lineRange
	}{
		{name: "줄 하나", input: "5", want: lineRange{from: addrLine(5), to: addrLine(5)}},
		{name: "줄 둘", input: "1,5", want: lineRange{from: addrLine(1), to: addrLine(5)}},
		{name: "커서 줄", input: ".", want: lineRange{from: addrDot(0), to: addrDot(0)}},
		{name: "마지막 줄", input: "$", want: lineRange{from: addrLast(0), to: addrLast(0)}},
		{name: "커서부터 끝까지", input: ".,$", want: lineRange{from: addrDot(0), to: addrLast(0)}},
		{name: "`%` 는 1 부터 끝까지다", input: "%", want: lineRange{from: addrLine(1), to: addrLast(0)}},

		// 자리 옮김. base 를 치지 않으면 커서 줄에서 옮긴다.
		{name: "커서 아래로", input: ".,+3", want: lineRange{from: addrDot(0), to: addrRel(3)}},
		{name: "커서 위로", input: "-2,.", want: lineRange{from: addrRel(-2), to: addrDot(0)}},
		{name: "base 없이 옮김만", input: "+3", want: lineRange{from: addrRel(3), to: addrRel(3)}},
		{name: "숫자 없는 `+` 는 한 줄", input: "+", want: lineRange{from: addrRel(1), to: addrRel(1)}},
		{name: "숫자에도 붙는다", input: "10-2", want: lineRange{from: lineAddress{base: addressNumber, line: 10, offset: -2}, to: lineAddress{base: addressNumber, line: 10, offset: -2}}},
		{name: "끝에서 거슬러", input: "$-4,$", want: lineRange{from: addrLast(-4), to: addrLast(0)}},

		// 쉼표 한쪽이 비면 커서 줄이다.
		{name: "앞이 빔", input: ",5", want: lineRange{from: addrDot(0), to: addrLine(5)}},
		{name: "뒤가 빔", input: "1,", want: lineRange{from: addrLine(1), to: addrDot(0)}},
		{name: "쉼표만", input: ",", want: lineRange{from: addrDot(0), to: addrDot(0)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLineRange(test.input)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseLineRangeInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "쉼표 셋", input: "1,2,3", want: "범위를 알 수 없습니다: 2,3"},
		{name: "`%` 는 주소 자리에 못 온다", input: "%,5", want: "범위를 알 수 없습니다: %"},
		{name: "옮김 뒤에 또 옮김", input: "1+2+3", want: "범위를 알 수 없습니다: 1+2+3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseLineRange(test.input)

			require.Error(t, err)
			assert.Equal(t, test.want, err.Error())
		})
	}
}

// resolve 는 buffer 를 봐야 아는 것(`.` `$`) 을 줄 자리로 바꾼다. 0 부터 세고 양끝을 포함한다.
func TestLineRangeResolve(t *testing.T) {
	// 다섯 줄이고 커서는 셋째 줄(자리 2)이다.
	newBuffer := func() viewport {
		buf := newBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))
		buf.cursor.line = 2

		return buf
	}

	tests := []struct {
		name     string
		input    string
		from, to int
	}{
		{name: "줄 번호는 1 부터", input: "1,5", from: 0, to: 4},
		{name: "줄 하나", input: "3", from: 2, to: 2},
		{name: "`%` 는 전체", input: "%", from: 0, to: 4},
		{name: "커서 줄", input: ".", from: 2, to: 2},
		{name: "커서부터 끝까지", input: ".,$", from: 2, to: 4},
		{name: "커서 아래로", input: ".,+2", from: 2, to: 4},
		{name: "커서 위로", input: "-2,.", from: 0, to: 2},
		{name: "끝에서 거슬러", input: "$-1,$", from: 3, to: 4},
		{name: "뒤집힌 범위는 바로잡는다", input: "5,1", from: 0, to: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rng, err := parseLineRange(test.input)
			require.NoError(t, err)

			from, to, err := rng.resolve(newBuffer())

			require.NoError(t, err)
			assert.Equal(t, test.from, from, "from")
			assert.Equal(t, test.to, to, "to")
		})
	}
}

// 없는 줄은 끝으로 잘라 주지 않는다. 손이 미끄러진 것과 시킨 것을 가를 수 없기 때문이다.
func TestLineRangeResolveOutOfFile(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "파일보다 큰 줄", input: "9", want: "그런 줄이 없습니다: 9"},
		{name: "둘째 자리가 넘는다", input: "1,9", want: "그런 줄이 없습니다: 9"},
		{name: "0 번 줄", input: "0", want: "그런 줄이 없습니다: 0"},
		{name: "커서 위로 넘어간다", input: "-9,.", want: "그런 줄이 없습니다: -6"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte("a\nb\nc\nd\ne\n"))
			buf.cursor.line = 2

			rng, err := parseLineRange(test.input)
			require.NoError(t, err)

			_, _, err = rng.resolve(buf)

			require.Error(t, err)
			assert.Equal(t, test.want, err.Error())
		})
	}
}

// zero 가 "치지 않았다" 여야 명령이 기본값을 스스로 정할 수 있다.
func TestLineRangeZeroMeansNotGiven(t *testing.T) {
	cmd, err := parseCommand("d", testCurrentFile)
	require.NoError(t, err)
	assert.Equal(t, lineRange{}, cmd.lines, "범위를 치지 않았다")

	cmd, err = parseCommand(".d", testCurrentFile)
	require.NoError(t, err)
	assert.NotEqual(t, lineRange{}, cmd.lines, "`.` 는 친 것이다")

	cmd, err = parseCommand("+3d", testCurrentFile)
	require.NoError(t, err)
	assert.NotEqual(t, lineRange{}, cmd.lines, "base 없는 옮김도 친 것이다")
}

// 여기부터는 명령줄에서 끝까지 이어지는지 본다. 범위를 뜯는 것은 위에서 봤다.

func TestCommandDeleteRange(t *testing.T) {
	tests := []struct {
		name    string
		down    int // `:` 를 치기 전에 커서를 내려둔 줄 수. `.` 가 어디인지가 갈린다
		input   []string
		want    []string
		message string
	}{
		{name: "줄 둘", input: []string{"1", ",", "3", "d"}, want: []string{"d", "e"}, message: "3 줄 지웠습니다"},
		{name: "줄 하나", input: []string{"2", "d"}, want: []string{"a", "c", "d", "e"}, message: "1 줄 지웠습니다"},
		{name: "범위 없는 `:d` 는 커서 줄", down: 1, input: []string{"d"}, want: []string{"a", "c", "d", "e"}, message: "1 줄 지웠습니다"},
		{name: "커서 줄", down: 1, input: []string{".", "d"}, want: []string{"a", "c", "d", "e"}, message: "1 줄 지웠습니다"},
		{name: "커서부터 끝까지", down: 1, input: []string{".", ",", "$", "d"}, want: []string{"a"}, message: "4 줄 지웠습니다"},
		{name: "커서 아래로", down: 1, input: []string{".", ",", "+", "1", "d"}, want: []string{"a", "d", "e"}, message: "2 줄 지웠습니다"},
		{name: "커서 위로", down: 2, input: []string{"-", "1", ",", ".", "d"}, want: []string{"a", "d", "e"}, message: "2 줄 지웠습니다"},
		{name: "마지막 줄만", input: []string{"$", "d"}, want: []string{"a", "b", "c", "d"}, message: "1 줄 지웠습니다"},
		{name: "끝에서 거슬러", input: []string{"$", "-", "1", ",", "$", "d"}, want: []string{"a", "b", "c"}, message: "2 줄 지웠습니다"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var m tea.Model = newTestEditor("a\nb\nc\nd\ne\n", 40, 8)

			for range test.down {
				m = send(m, "j")
			}

			m = send(m, append(append([]string{":"}, test.input...), "enter")...)

			assert.Equal(t, test.want, linesOf(bufferOf(t, m)))
			assert.Contains(t, barOf(t, m)[1], test.message)
		})
	}
}

// `:%d` 는 파일을 통째로 지운다. 줄이 하나도 없는 buffer 는 있을 수 없어서 빈 줄이 남는다.
func TestCommandDeleteWholeFile(t *testing.T) {
	var m tea.Model = newTestEditor("a\nb\nc\n", 40, 8)

	m = send(m, ":", "%", "d", "enter")

	assert.Equal(t, []string{""}, linesOf(bufferOf(t, m)))
	assert.Contains(t, barOf(t, m)[1], "3 줄 지웠습니다")
}

// 지운 것은 register 에 남는다. 줄 단위라 `p` 가 줄로 붙인다(ADR-0017).
func TestCommandDeleteFillsRegister(t *testing.T) {
	var m tea.Model = newTestEditor("a\nb\nc\nd\n", 40, 8)

	m = send(m, ":", "1", ",", "2", "d", "enter")
	m = send(m, "p")

	assert.Equal(t, []string{"c", "a", "b", "d"}, linesOf(bufferOf(t, m)))
}

// 한 번의 `u` 로 범위 전체가 돌아와야 한다. 줄마다 되돌리기 구간이 생기면 안 된다.
func TestCommandDeleteRangeUndoesAtOnce(t *testing.T) {
	var m tea.Model = newTestEditor("a\nb\nc\nd\ne\n", 40, 8)

	m = send(m, ":", "2", ",", "4", "d", "enter")
	require.Equal(t, []string{"a", "e"}, linesOf(bufferOf(t, m)))

	m = send(m, "u")

	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, linesOf(bufferOf(t, m)))
}

func TestCommandYankRange(t *testing.T) {
	var m tea.Model = newTestEditor("a\nb\nc\nd\n", 40, 8)

	// 커서를 셋째 줄로 옮겨두고 앞쪽을 복사한다. 파일도 커서도 그대로여야 한다.
	m = send(m, "j", "j")
	m = send(m, ":", "1", ",", "2", "y", "enter")

	assert.Equal(t, []string{"a", "b", "c", "d"}, linesOf(bufferOf(t, m)), "파일은 그대로다")
	assert.Equal(t, 2, bufferOf(t, m).cursor.line, "커서도 그대로다")
	assert.Contains(t, barOf(t, m)[1], "2 줄 복사되었습니다")

	m = send(m, "p")

	assert.Equal(t, []string{"a", "b", "c", "a", "b", "d"}, linesOf(bufferOf(t, m)))
}

// 범위만 치면 그 줄로 간다. 두 자리면 뒤쪽이다.
func TestCommandGoToLine(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  int
	}{
		{name: "줄 번호", input: []string{"4"}, want: 3},
		{name: "두 자리는 뒤쪽", input: []string{"1", ",", "3"}, want: 2},
		{name: "마지막 줄", input: []string{"$"}, want: 4},
		{name: "커서에서 옮김", input: []string{"+", "2"}, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var m tea.Model = newTestEditor("a\nb\nc\nd\ne\n", 40, 8)

			m = send(m, append(append([]string{":"}, test.input...), "enter")...)

			assert.IsType(t, viewEditorNormal{}, m)
			assert.Equal(t, test.want, bufferOf(t, m).cursor.line)
		})
	}
}

// 칸은 첫 비공백이다. 줄 번호로 뛰는 것은 `gg`·`G` 와 같은 일이다.
func TestCommandGoToLineMovesToFirstNonBlank(t *testing.T) {
	var m tea.Model = newTestEditor("a\n    b\n", 40, 8)

	m = send(m, ":", "2", "enter")

	assert.Equal(t, 4, bufferOf(t, m).cursor.col)
}

func TestCommandRangeErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{name: "없는 줄", input: []string{"9", "d"}, want: "그런 줄이 없습니다: 9"},
		{name: "뜯을 수 없는 범위", input: []string{"1", ",", "2", ",", "3", "d"}, want: "범위를 알 수 없습니다: 2,3"},
		{name: "범위를 받지 않는 명령", input: []string{"1", ",", "5", "w"}, want: "이 명령은 줄 범위를 받지 않습니다"},
		{name: "범위 뒤의 셸도 받지 않는다", input: []string{"1", ",", "5", "!", "l", "s"}, want: "이 명령은 줄 범위를 받지 않습니다"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var m tea.Model = newTestEditor("a\nb\nc\n", 40, 8)

			m = send(m, append(append([]string{":"}, test.input...), "enter")...)

			assert.IsType(t, viewEditorNormal{}, m)
			assert.Contains(t, barOf(t, m)[1], test.want)
			assert.Equal(t, []string{"a", "b", "c"}, linesOf(bufferOf(t, m)), "실패했으면 파일을 건드리지 않는다")
		})
	}
}
