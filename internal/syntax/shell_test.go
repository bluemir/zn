package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "값 참조",
			line: "make install    # $HOME/.local/bin 에 넣는다",
			want: []string{"comment:# $HOME/.local/bin 에 넣는다"},
		},
		{
			name: "괄호 없는 값 참조",
			line: "echo $HOME/bin",
			want: []string{"var:$HOME"},
		},
		{
			name: "중괄호와 명령 치환",
			line: "echo ${HOME} $(pwd)",
			want: []string{"var:${HOME}", "var:$(pwd)"},
		},
		{
			name: "예약어",
			line: "if [ -f x ]; then echo 한글; fi",
			want: []string{"keyword:if", "keyword:then", "keyword:fi"},
		},
		{
			name: "for 고리",
			line: "for f in *.go; do echo $f; done",
			want: []string{"keyword:for", "keyword:in", "keyword:do", "var:$f", "keyword:done"},
		},
		{
			name: "값 낱말은 명령 자리에서만이다",
			line: "if true; then echo x; fi",
			want: []string{"keyword:if", "const:true", "keyword:then", "keyword:fi"},
		},
		{
			name: "대입값의 `true` 는 builtin 이 아니라 글자다",
			line: "x=true",
			want: []string{},
		},
		{
			name: "낱말 가운데의 예약어는 예약어가 아니다",
			line: "echo --done notdone",
			want: []string{},
		},
		{
			name: "따옴표",
			line: `echo "한글 $HOME" '그대로'`,
			want: []string{`string:"한글 $HOME"`, "string:'그대로'"},
		},
		{
			name: "홑따옴표 안에서 `\\` 는 escape 가 아니다",
			line: `echo 'a\' b`,
			want: []string{`string:'a\'`},
		},
		{
			name: "쌍따옴표 안에서는 escape 다",
			line: `echo "a\"b"`,
			want: []string{`string:"a\"b"`},
		},
		{
			name: "줄 전체가 주석",
			line: "# 한글 주석이다",
			want: []string{"comment:# 한글 주석이다"},
		},
		{
			name: "`${#var}` 의 `#` 은 주석이 아니다",
			line: "echo ${#list}",
			want: []string{"var:${#list}"},
		},
		{
			name: "낱말 가운데의 `#` 도 주석이 아니다",
			line: "echo a#b",
			want: []string{},
		},
		{
			name: "`$$` 는 값 참조가 아니다",
			line: "echo $$",
			want: []string{},
		},
		{
			name: "특수 인자",
			line: `echo "$@" $1 $?`,
			want: []string{`string:"$@"`, "var:$1", "var:$?"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, shNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, shNormal{}, next, "한 줄에서 끝나면 문맥이 그대로다")
		})
	}
}

// shell 의 따옴표는 줄을 넘는다. js·python 의 한 겹 따옴표와 다른 자리다.
func TestShellQuoteAcrossLines(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "쌍따옴표가 세 줄에 걸친다",
			lines: []string{`echo "여러 줄에`, "가운데", `걸친 글" && ls`},
			want: [][]string{
				{`string:"여러 줄에`}, {"string:가운데"}, {`string:걸친 글"`},
			},
			end: shNormal{},
		},
		{
			name:  "홑따옴표도 그렇다",
			lines: []string{"echo '열었다", "닫는다'"},
			want:  [][]string{{"string:'열었다"}, {"string:닫는다'"}},
			end:   shNormal{},
		},
		{
			name:  "안쪽의 `#` 은 주석이 아니다",
			lines: []string{`echo "열었다`, "# 주석이 아니다", `"`},
			want:  [][]string{{`string:"열었다`}, {"string:# 주석이 아니다"}, {`string:"`}},
			end:   shNormal{},
		},
		{
			name:  "닫히지 않으면 계속 안쪽이다",
			lines: []string{`echo "열었다`, "그대로"},
			want:  [][]string{{`string:"열었다`}, {"string:그대로"}},
			end:   shQuoted{quote: '"'},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, shNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}
