package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 아래 기대값은 vim 9.1 에서 같은 키를 쳐서 확인한 것이다(ADR-0083).
func TestChangeCase(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		keys []string
		want []string
	}{
		{name: "~", data: "abc", keys: []string{"~"}, want: []string{"Abc"}},
		{name: "~ 대문자를 소문자로", data: "aBc", col: 1, keys: []string{"~"}, want: []string{"abc"}},
		{name: "3~ 는 세 글자", data: "abcdef", keys: []string{"3", "~"}, want: []string{"ABCdef"}},
		{name: "줄 끝 글자", data: "abc", col: 2, keys: []string{"~"}, want: []string{"abC"}},
		{name: "빈 줄에서는 아무 일도 없다", data: "\nabc", keys: []string{"~"}, want: []string{"", "abc"}},

		// `r` 과 갈리는 자리다. 모자라면 있는 만큼만 바꾼다.
		{name: "줄이 짧으면 있는 만큼", data: "ab", keys: []string{"9", "9", "~"}, want: []string{"AB"}},
		{name: "줄을 넘지 않는다", data: "ab\ncd", keys: []string{"9", "~"}, want: []string{"AB", "cd"}},

		// 대소문자가 없는 글자는 제자리다.
		{name: "한글은 그대로", data: "한글abc", keys: []string{"3", "~"}, want: []string{"한글Abc"}},
		{name: "문장부호는 그대로", data: "!@a", keys: []string{"3", "~"}, want: []string{"!@A"}},

		// visual 은 고른 범위다. 셋이 갈래만 다르다.
		{name: "v l ~", data: "abc", col: 1, keys: []string{"v", "l", "~"}, want: []string{"aBC"}},
		{name: "V j U", data: "abc\ndef", keys: []string{"V", "j", "U"}, want: []string{"ABC", "DEF"}},
		{name: "V u", data: "ABC", keys: []string{"V", "u"}, want: []string{"abc"}},
		{name: "l l v j l U", data: "abcdef\nghijkl",
			keys: []string{"l", "l", "v", "j", "l", "U"}, want: []string{"abCDEF", "GHIJkl"}},
		{name: "V 는 걸친 줄 전체다", data: "abc\ndef", col: 1,
			keys: []string{"V", "U"}, want: []string{"ABC", "def"}},
		{name: "v ~ 는 커서 한 글자", data: "abc", col: 1, keys: []string{"v", "~"}, want: []string{"aBc"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.want, linesOf(buf))
		})
	}
}

// 바꾼 뒤 커서 자리다. `~` 는 훑어 가는 키라 대소문자가 없는 글자 위에서도 오른쪽으로 간다.
func TestChangeCaseCursor(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		line     int
		col      int
		keys     []string
		wantLine int
		wantCol  int
	}{
		{name: "~ 는 다음 글자로", data: "abc", keys: []string{"~"}, wantCol: 1},
		{name: "3~ 는 세 글자 뒤로", data: "abcdef", keys: []string{"3", "~"}, wantCol: 3},
		{name: "줄 끝을 넘지 않는다", data: "abc", col: 2, keys: []string{"~"}, wantCol: 2},
		{name: "모자라도 줄 끝에 선다", data: "abcd", keys: []string{"9", "9", "~"}, wantCol: 3},
		{name: "한글 한 글자를 건넌다", data: "한글abc", keys: []string{"~"}, wantCol: 3},
		{name: "바뀐 것이 없어도 간다", data: "!@a", keys: []string{"~"}, wantCol: 1},
		{name: "빈 줄에서는 그대로", data: "\nabc", keys: []string{"~"}, wantCol: 0},

		// visual 은 범위의 시작으로 간다. 복사(`y`) 와 같은 길이다.
		{name: "l l v j l U 는 범위 시작으로", data: "abcdef\nghijkl",
			keys: []string{"l", "l", "v", "j", "l", "U"}, wantCol: 2},
		{name: "거꾸로 골라도 시작으로", data: "abcdef\nghijkl", line: 1,
			keys: []string{"l", "l", "v", "k", "U"}, wantCol: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := pressFrom(t, test.data, test.line, test.col, test.keys...)

			assert.Equal(t, test.wantLine, buf.cursor.Line, "줄")
			assert.Equal(t, test.wantCol, buf.cursor.Col, "칸")
		})
	}
}

// 되돌리기는 한 번에 돌아온다. 앞의 타이핑 구간에 섞이지 않는다.
func TestChangeCaseUndo(t *testing.T) {
	buf := pressFrom(t, "abc\ndef", 0, 0, "V", "j", "U", "u")
	assert.Equal(t, []string{"abc", "def"}, linesOf(buf), "visual 은 한 번에 돌아온다")

	buf = pressFrom(t, "abcdef", 0, 0, "i", "x", "esc", "3", "~", "u")
	assert.Equal(t, []string{"xabcdef"}, linesOf(buf), "친 글자는 남는다")
}

// 바뀐 것이 없으면 파일을 건드리지 않는다. 그냥 갈아끼우면 고친 표시가 서고 redo 가 날아간다.
func TestChangeCaseKeepsClean(t *testing.T) {
	buf := pressFrom(t, "한글", 0, 0, "V", "U")
	assert.False(t, buf.dirty, "대소문자가 없는 글자만 골랐다")

	buf = pressFrom(t, "한글", 0, 0, "~")
	assert.False(t, buf.dirty, "한 글자도 같다")
}
