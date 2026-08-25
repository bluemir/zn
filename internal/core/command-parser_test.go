package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// words 는 여느 토큰(이름·인자) 을 짧게 적는 손이다. 갈래를 보는 줄만 kind 를 적는다.
func words(texts ...string) []token {
	tokens := []token{}
	for _, text := range texts {
		tokens = append(tokens, token{text: text})
	}

	return tokens
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []token
	}{
		{name: "빈 입력", input: "", want: words()},
		{name: "공백만", input: "   ", want: words()},
		{name: "이름 하나", input: "w", want: words("w")},
		{name: "앞뒤 공백", input: "  w  ", want: words("w")},
		{name: "이름과 인자", input: "e main.go", want: words("e", "main.go")},
		{name: "공백 여러 개", input: "e    main.go", want: words("e", "main.go")},
		{name: "tab 으로도 끊는다", input: "e\tmain.go", want: words("e", "main.go")},
		{name: "인자 여럿", input: "e a.go b.go", want: words("e", "a.go", "b.go")},
		{name: "`!` 는 글자다", input: "q!", want: words("q!")},

		// 여기부터가 tokenizer 를 만든 이유다. 공백이 든 파일 이름.
		{name: "따옴표", input: `w "my notes.txt"`, want: words("w", "my notes.txt")},
		{name: "이스케이프", input: `w my\ notes.txt`, want: words("w", "my notes.txt")},
		{name: "따옴표 안의 `\\` 는 글자", input: `w "C:\tmp\a"`, want: words("w", `C:\tmp\a`)},
		{name: "따옴표를 넣으려면 밖에서 이스케이프", input: `w \"quoted\"`, want: words("w", `"quoted"`)},
		{name: "따옴표가 토큰 중간에서 열린다", input: `w a"b c"`, want: words("w", "ab c")},
		{name: "따옴표 뒤에 글자가 이어진다", input: `w "a b"c`, want: words("w", "a bc")},
		{name: "빈 따옴표", input: `w ""`, want: words("w")},
		{name: "따옴표 안의 공백만", input: `w " "`, want: words("w", " ")},
		{name: "한글 경로", input: `w "내 메모.txt"`, want: words("w", "내 메모.txt")},

		// 맨 앞의 `!` 는 이름 하나로 끊고 뒤를 통째로 넘긴다. 여느 자리의 `!` 는 그냥 글자다.
		{name: "맨 앞의 `!` 는 뒤를 통째로", input: "!ls -la", want: []token{{text: "ls -la", kind: tokenKindShell}}},
		{name: "`!` 만", input: "!", want: []token{{kind: tokenKindShell}}},
		{name: "`!` 앞의 공백은 아직 맨 앞", input: "  !ls -la", want: []token{{text: "ls -la", kind: tokenKindShell}}},
		{name: "셸에 넘길 것은 따옴표도 뜯지 않는다", input: `!echo "a b"`, want: []token{{text: `echo "a b"`, kind: tokenKindShell}}},
		{name: "셸에 넘길 것은 `\\` 도 글자다", input: `!echo a\ b`, want: []token{{text: `echo a\ b`, kind: tokenKindShell}}},
		{name: "맨 앞이 아닌 `!` 는 뜻이 없다", input: "w !foo", want: words("w", "!foo")},
		{name: "이스케이프한 `!` 는 맨 앞이 아니다", input: `\!ls`, want: words("!ls")},

		// 이름 앞의 줄 범위도 맨 앞에서만 뜻을 갖는다. 안쪽은 뜯지 않고 한 토큰이다.
		{name: "범위와 이름", input: "1,5d", want: []token{{text: "1,5", kind: tokenKindRange}, {text: "d"}}},
		{name: "숫자 하나도 범위다", input: "5d", want: []token{{text: "5", kind: tokenKindRange}, {text: "d"}}},
		{name: "범위만", input: "42", want: []token{{text: "42", kind: tokenKindRange}}},
		{name: "`%` 는 범위다", input: "%d", want: []token{{text: "%", kind: tokenKindRange}, {text: "d"}}},
		{name: "커서와 끝", input: ".,$d", want: []token{{text: ".,$", kind: tokenKindRange}, {text: "d"}}},
		{name: "자리 옮김", input: ".,+3d", want: []token{{text: ".,+3", kind: tokenKindRange}, {text: "d"}}},
		{name: "범위와 이름 사이의 공백", input: "1,5 d", want: []token{{text: "1,5", kind: tokenKindRange}, {text: "d"}}},
		{name: "범위 뒤에 인자도 온다", input: "1,5 y a", want: []token{{text: "1,5", kind: tokenKindRange}, {text: "y"}, {text: "a"}}},
		{name: "맨 앞이 아닌 숫자는 인자다", input: "e 1,5", want: words("e", "1,5")},
		{name: "범위 뒤의 `!` 는 이름 자리다", input: "1,5!sort", want: []token{{text: "1,5", kind: tokenKindRange}, {text: "sort", kind: tokenKindShell}}},
		{name: "숫자로 시작하는 파일 이름", input: "e 1.txt", want: words("e", "1.txt")},
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
	assert.Equal(t, words("w", "foo"), got, "마지막 인자가 사라지면 안 된다")
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

		// 범위는 이름 앞에 붙는다. 안쪽은 parseLineRange 가 뜯는다(command-range_test.go).
		{name: "범위와 이름", input: "1,5d", want: command{name: "d", lines: lineRange{from: addrLine(1), to: addrLine(5)}}},
		{name: "범위와 force", input: "1,5d!", want: command{name: "d", force: true, lines: lineRange{from: addrLine(1), to: addrLine(5)}}},
		{name: "범위와 인자", input: "1,5w foo", want: command{name: "w", args: []string{"foo"}, lines: lineRange{from: addrLine(1), to: addrLine(5)}}},
		{name: "범위만", input: "42", want: command{lines: lineRange{from: addrLine(42), to: addrLine(42)}}},
		{name: "범위와 셸", input: "1,5!sort", want: command{name: "!", args: []string{"sort"}, lines: lineRange{from: addrLine(1), to: addrLine(5)}}},
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

// 범위를 뜯다 막히면 그 오류가 명령줄까지 온다. 뒤에 남은 토큰이 그것을 덮지 않는다.
func TestParseCommandFailsOnBadRange(t *testing.T) {
	_, err := parseCommand("1,2,3d foo")

	require.Error(t, err)
	assert.Equal(t, "범위를 알 수 없습니다: 2,3", err.Error())
}

func TestParseCommandPassesTokenizeError(t *testing.T) {
	_, err := parseCommand(`w "unfinished`)

	require.Error(t, err)
	assert.Equal(t, "따옴표가 닫히지 않았습니다", err.Error())
}

// `:grep` 뒤는 뜯지 않고 통째로 한 토큰이다. 정규식이 오는 자리다(ADR-0077).
func TestTokenizeGrepTakesRawArgument(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []token
	}{
		{name: "빈 칸이 있어도 한 토큰", input: `grep func New`,
			want: []token{{text: "grep"}, {text: "func New", kind: tokenKindShell}}},
		{name: "`\\` 가 살아남는다", input: `grep func\s+New`,
			want: []token{{text: "grep"}, {text: `func\s+New`, kind: tokenKindShell}}},
		{name: "따옴표도 글자다", input: `grep "a b"`,
			want: []token{{text: "grep"}, {text: `"a b"`, kind: tokenKindShell}}},
		{name: "force 표시도 통째로 받는다", input: `grep! \d+`,
			want: []token{{text: "grep!"}, {text: `\d+`, kind: tokenKindShell}}},
		{name: "이름만", input: `grep`, want: words("grep")},
		{name: "이름 뒤 공백뿐", input: `grep `,
			want: []token{{text: "grep"}, {text: "", kind: tokenKindShell}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := tokenize(test.input)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

// 다른 이름은 그대로 뜯는다. 통째로 받는 것은 `grep` 하나뿐이다.
func TestTokenizeOtherNamesStillSplit(t *testing.T) {
	got, err := tokenize(`e a\b c`)

	require.NoError(t, err)
	assert.Equal(t, words("e", "ab", "c"), got, "`\\` 가 먹히고 공백으로 끊긴다")
}

// 뒤가 비어 있으면 인자가 아니다. `:grep ` 는 `:grep` 과 같아야 한다.
func TestParseGrepEmptyRawArgumentIsNotAnArgument(t *testing.T) {
	for _, input := range []string{"grep", "grep ", "grep   "} {
		cmd, err := parseCommand(input)

		require.NoError(t, err, input)
		assert.Equal(t, "grep", cmd.name, input)
		assert.Empty(t, cmd.args, input)
	}
}

// 패턴을 대면 인자 하나로 온다.
func TestParseGrepKeepsPatternWhole(t *testing.T) {
	cmd, err := parseCommand(`grep func\s+New(`)

	require.NoError(t, err)
	assert.Equal(t, "grep", cmd.name)
	assert.Equal(t, []string{`func\s+New(`}, cmd.args)
}
