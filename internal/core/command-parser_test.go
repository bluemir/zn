package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "빈 입력", input: "", want: []string{}},
		{name: "공백만", input: "   ", want: []string{}},
		{name: "이름 하나", input: "w", want: []string{"w"}},
		{name: "앞뒤 공백", input: "  w  ", want: []string{"w"}},
		{name: "이름과 인자", input: "e main.go", want: []string{"e", "main.go"}},
		{name: "공백 여러 개", input: "e    main.go", want: []string{"e", "main.go"}},
		{name: "tab 으로도 끊는다", input: "e\tmain.go", want: []string{"e", "main.go"}},
		{name: "인자 여럿", input: "e a.go b.go", want: []string{"e", "a.go", "b.go"}},
		{name: "`!` 는 글자다", input: "q!", want: []string{"q!"}},

		// 여기부터가 tokenizer 를 만든 이유다. 공백이 든 파일 이름.
		{name: "따옴표", input: `w "my notes.txt"`, want: []string{"w", "my notes.txt"}},
		{name: "이스케이프", input: `w my\ notes.txt`, want: []string{"w", "my notes.txt"}},
		{name: "따옴표 안의 `\\` 는 글자", input: `w "C:\tmp\a"`, want: []string{"w", `C:\tmp\a`}},
		{name: "따옴표를 넣으려면 밖에서 이스케이프", input: `w \"quoted\"`, want: []string{"w", `"quoted"`}},
		{name: "따옴표가 토큰 중간에서 열린다", input: `w a"b c"`, want: []string{"w", "ab c"}},
		{name: "따옴표 뒤에 글자가 이어진다", input: `w "a b"c`, want: []string{"w", "a bc"}},
		{name: "빈 따옴표", input: `w ""`, want: []string{"w"}},
		{name: "따옴표 안의 공백만", input: `w " "`, want: []string{"w", " "}},
		{name: "한글 경로", input: `w "내 메모.txt"`, want: []string{"w", "내 메모.txt"}},

		// 맨 앞의 `!` 는 이름 하나로 끊고 뒤를 통째로 넘긴다. 여느 자리의 `!` 는 그냥 글자다.
		{name: "맨 앞의 `!` 는 뒤를 통째로", input: "!ls -la", want: []string{"!", "ls -la"}},
		{name: "`!` 만", input: "!", want: []string{"!"}},
		{name: "`!` 앞의 공백은 아직 맨 앞", input: "  !ls -la", want: []string{"!", "ls -la"}},
		{name: "셸에 넘길 것은 따옴표도 뜯지 않는다", input: `!echo "a b"`, want: []string{"!", `echo "a b"`}},
		{name: "셸에 넘길 것은 `\\` 도 글자다", input: `!echo a\ b`, want: []string{"!", `echo a\ b`}},
		{name: "맨 앞이 아닌 `!` 는 뜻이 없다", input: "w !foo", want: []string{"w", "!foo"}},
		{name: "이스케이프한 `!` 는 맨 앞이 아니다", input: `\!ls`, want: []string{"!ls"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := tokenize(test.input)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

// 끝나면 안 되는 자리에서 입력이 끝나면 알려야 한다.
// 조용히 넘기면 `:w "my notes` 가 `my notes` 로 저장된다.
func TestTokenizeUnfinished(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "따옴표가 안 닫힘", input: `w "my notes`, want: "따옴표가 닫히지 않았습니다"},
		{name: "따옴표만 열림", input: `w "`, want: "따옴표가 닫히지 않았습니다"},
		{name: "`\\` 로 끝남", input: `w foo\`, want: "`\\` 뒤에 글자가 없습니다"},
		{name: "`\\` 만", input: `\`, want: "`\\` 뒤에 글자가 없습니다"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := tokenize(test.input)

			require.Error(t, err)
			assert.Equal(t, test.want, err.Error())
		})
	}
}

// 마지막 토큰은 입력이 끝날 때 흘려야 한다. 공백으로 끝나지 않으면 버퍼에 남는다.
func TestTokenizeFlushesLastToken(t *testing.T) {
	got, err := tokenize("w foo")

	require.NoError(t, err)
	assert.Equal(t, []string{"w", "foo"}, got, "마지막 인자가 사라지면 안 된다")
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  command
	}{
		{name: "빈 명령", input: "", want: command{}},
		{name: "공백만", input: "  ", want: command{}},
		{name: "이름만", input: "w", want: command{name: "w"}},
		{name: "앞뒤 공백", input: " wq ", want: command{name: "wq"}},
		{name: "`!` 는 이름에서 뗀다", input: "q!", want: command{name: "q", force: true}},
		{name: "`!` 는 이름이 길어도 뗀다", input: "qa!", want: command{name: "qa", force: true}},
		{name: "인자", input: "e main.go", want: command{name: "e", args: []string{"main.go"}}},
		{name: "`!` 는 이름이고 뒤는 인자 하나다", input: "!ls -la", want: command{name: "!", args: []string{"ls -la"}}},
		{name: "`!` 만 치면 넘길 것이 없다", input: "!", want: command{name: "!"}},
		{name: "`!!` 는 `!` 를 셸에 넘긴다", input: "!!", want: command{name: "!", args: []string{"!"}}},
		{name: "`!` 는 force 가 아니다", input: "  !ls", want: command{name: "!", args: []string{"ls"}}},
		{name: "`!` 와 인자", input: `e! "my notes.txt"`, want: command{name: "e", force: true, args: []string{"my notes.txt"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCommand(test.input)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseCommandPassesTokenizeError(t *testing.T) {
	_, err := parseCommand(`w "unfinished`)

	require.Error(t, err)
	assert.Equal(t, "따옴표가 닫히지 않았습니다", err.Error())
}
