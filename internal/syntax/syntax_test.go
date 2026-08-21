package syntax

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindNames 는 갈래의 시험용 이름이다.
//
// 자리를 숫자로 적으면 한글이 든 줄에서 눈으로 맞춰 볼 수 없다. 그래서 시험은
// `갈래:잘라낸 글자` 로 적는다.
//
// 갈래를 늘리면서 여기 빠뜨리면 lexed 가 그 갈래를 만나는 순간 떨어진다. 갈래 목록을
// 손으로 또 적어 두고 견주지는 않는다 — 그 목록도 같이 빠뜨릴 수 있어서 지키는 것이 없다.
var kindNames = map[Kind]string{
	KindComment:  "comment",
	KindString:   "string",
	KindNumber:   "number",
	KindKeyword:  "keyword",
	KindType:     "type",
	KindFunction: "func",
	KindConstant: "const",
	KindVariable: "var",
	KindHeading:  "heading",
	KindEmphasis: "em",
	KindStrong:   "strong",
	KindLink:     "link",
}

// lexed 는 문맥에 줄을 먹여 토큰을 `갈래:글자` 로 늘어놓는다.
func lexed(t *testing.T, state State, line string) ([]string, State) {
	t.Helper()

	tokens, next := state.Lex([]byte(line))
	require.NotNil(t, next, "돌려주는 문맥은 nil 이 아니다")

	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		name, ok := kindNames[token.Kind]
		require.True(t, ok, "kindNames 에 없는 갈래다: %d", token.Kind)
		out = append(out, name+":"+line[token.Start:token.End])
	}

	return out, next
}

// lexedAll 은 줄들을 차례로 먹인다. 여러 줄에 걸치는 것을 볼 때 쓴다.
func lexedAll(t *testing.T, state State, lines ...string) ([][]string, State) {
	t.Helper()

	out := make([][]string, 0, len(lines))
	for _, line := range lines {
		var got []string
		got, state = lexed(t, state, line)
		out = append(out, got)
	}

	return out, state
}

// languageSamples 는 언어별 표본이다.
//
// 갈래가 맞는지는 언어별 시험이 보고, 여기서는 **어느 언어든 지켜야 하는 것**만 본다.
// 표본에는 여러 줄에 걸치는 것과 한글, 빈 줄이 들어간다 — 그 셋이 자리를 어긋나게 하는
// 자리다. 언어를 더할 때 표본도 같이 는다.
var languageSamples = []struct {
	name  string
	state State
	lines []string
}{
	{
		name:  "go",
		state: goNormal{},
		lines: []string{
			"package core",
			"",
			"// colorWhitespace 는 공백 마커의 색이다(ADR-0005).",
			"func lexTail(state State, from int) ([]Token, State) {",
			"\tname := \"한글 이름\" // 한글 주석이다",
			"\ts := `여러 줄에",
			"걸친 문자열` + \"가족👨‍👩‍👧\"",
			"\t/* 블록 주석이",
			"\t   여기서 끝난다 */ n := 0x1f + 1e9",
			"\treturn nil, false",
			"}",
		},
	},
	{
		name:  "markdown",
		state: mdNormal{},
		lines: []string{
			"# 언어별 syntax highlighting",
			"",
			"한글 본문에 `replaceLines` 와 *강조* 가 있고 [문서](https://example.com) 도 있다.",
			"> 인용문이다",
			"```go",
			"func main() {}",
			"\ts := `한글 여러 줄에",
			"걸친 문자열`",
			"```",
			"- [ ] 할 일이다",
		},
	},
	{
		name:  "html",
		state: htmlNormal{},
		lines: []string{
			"<!DOCTYPE html>",
			"",
			"<!-- 여러 줄에",
			"     걸친 주석 -->",
			`<div class="한글" data-id=1>`,
			"  <p>한글 &amp; 글이다</p>",
			"  <div title=\"여러 줄에",
			"        걸친 값\">",
			"<script>",
			`  const s = "한글 </div>";`,
			"</script>",
			"<script>let y = 2;</script>",
			"<style>",
			"  a { color: #fff; }",
			"</style>",
		},
	},
	{
		name:  "js",
		state: jsNormal{},
		lines: []string{
			"// 한글 주석이다",
			"",
			"class Buffer extends Base {",
			"\tconstructor() { this.n = 0x1f; }",
			"}",
			"const q = `여러 줄에",
			"걸친 ${값} 문자열`;",
			"/* 여러 줄에",
			"   걸친 주석 */",
		},
	},
	{
		name:  "css",
		state: cssNormal{},
		lines: []string{
			"/* 한글 주석이다 */",
			"",
			".버튼, #main > a:hover {",
			"\tcolor: #fff;",
			"\t--my-color: rgba(0, 0, 0, 0.5);",
			"\tcontent: \"한글\" !important;",
			"\t/* 여러 줄에",
			"\t   걸친 주석 */",
			"}",
		},
	},
	{
		name:  "python",
		state: pyNormal{},
		lines: []string{
			"# 한글 주석이다",
			"",
			"class Buffer:",
			"    @property",
			"    def 이름(self) -> str:",
			`        """여러 줄에 걸친`,
			`        문서다"""`,
			`        return f"{self.n} 개"`,
			"        n = 0x1f + 1_000",
		},
	},
	{
		name:  "shell",
		state: shNormal{},
		lines: []string{
			"# 한글 주석이다",
			"",
			"for f in *.go; do",
			`\techo "한글 ${f}" $(basename $f)`,
			"done",
			`echo "여러 줄에`,
			`걸친 글"`,
		},
	},
	{
		name:  "makefile",
		state: makeNormal{},
		lines: []string{
			"# 한글 주석이다",
			"",
			"VERSION?=$(shell git describe --tags)",
			"build: dep ## 도움 문구다",
			"\t@go build -o $@ .",
			"build/zn: $(GO_SOURCES) \\",
			"\t\t-o $@ .",
		},
	},
	{
		name:  "dockerfile",
		state: dockerNormal{},
		lines: []string{
			"# 한글 주석이다",
			"",
			"FROM golang:1.26 AS build",
			"ENV PATH=${GOPATH}/bin",
			"RUN apt-get update \\",
			" && rm -rf /var/lib/apt/lists/*",
			`CMD ["go", "run", "."]`,
		},
	},
}

// 어느 언어든 토큰은 앞에서 뒤로 겹치지 않는다.
func TestTokenOrder(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				var tokens []Token
				tokens, state = state.Lex([]byte(line))

				last := 0
				for _, token := range tokens {
					assert.Less(t, token.Start, token.End, "빈 토큰이 있습니다: %q", line)
					assert.GreaterOrEqual(t, token.Start, last,
						"토큰이 겹치거나 뒤로 갑니다: %q", line)
					last = token.End
				}
			}
		})
	}
}

// 자리가 줄을 넘으면 그리는 쪽이 없는 byte 를 집어 죽는다(core/render-row.go 의 expandRow).
func TestTokenInLine(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				var tokens []Token
				tokens, state = state.Lex([]byte(line))

				for _, token := range tokens {
					assert.GreaterOrEqual(t, token.Start, 0, "자리가 줄 앞을 넘습니다: %q", line)
					assert.LessOrEqual(t, token.End, len(line),
						"자리가 줄 끝을 넘습니다: %q", line)
				}
			}
		})
	}
}

// 경계가 grapheme cluster 를 가르면 그 뒤 폭 계산이 어긋난다(core/buffer.go 의 clusterAt).
// 한글과 ZWJ 로 이어진 이모지가 그 자리다.
func TestTokenClusterBoundary(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				var tokens []Token
				tokens, state = state.Lex([]byte(line))

				assert.Subset(t, clusterBoundaries(line), tokenBoundaries(tokens),
					"경계가 글자 가운데를 가릅니다: %q", line)
			}
		})
	}
}

// clusterBoundaries 는 줄에서 글자가 시작하거나 끝나는 자리 전부다.
func clusterBoundaries(line string) []int {
	bounds := []int{0}
	for rest, at := []byte(line), 0; len(rest) > 0; {
		cluster, _ := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		if len(cluster) < 1 {
			break
		}

		at += len(cluster)
		rest = rest[len(cluster):]
		bounds = append(bounds, at)
	}

	return bounds
}

func tokenBoundaries(tokens []Token) []int {
	bounds := make([]int, 0, len(tokens)*2)
	for _, token := range tokens {
		bounds = append(bounds, token.Start, token.End)
	}

	return bounds
}

// 줄은 read-only data 의 subslice 다(ADR-0001). lexer 가 고치면 파일 내용이 바뀐다.
func TestLexDoesNotChangeLine(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, text := range sample.lines {
				line := []byte(text)
				before := bytes.Clone(line)

				_, state = state.Lex(line)

				assert.Equal(t, before, line, "lexer 가 줄을 고쳤습니다: %q", text)
			}
		})
	}
}

// 이 시험이 컴파일 오류를 대신한다. 견줄 수 없는 문맥은 core 가 `==` 하는 자리에서
// 편집기를 죽인다(syntax.go 의 State).
func TestStateComparable(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				_, state = state.Lex([]byte(line))

				require.NotNil(t, state)
				assert.True(t, reflect.TypeOf(state).Comparable(),
					"%T 를 `==` 로 견줄 수 없습니다. slice·map 을 든 문맥은 두면 안 됩니다", state)
			}
		})
	}
}

// **reflect 의 Comparable 로는 부족하다.**
//
// interface 칸을 든 struct 는 안에 무엇이 들었든 Comparable 이 true 다 — cssComment 의 back 이
// 이미 그렇다. 위 시험은 그런 자리를 그냥 지나간다. 그래서 실제로 `==` 를 해 본다. core 가
// 죽는 그 연산이다(core/syntax.go 의 after == before, ADR-0039).
func TestStateEqualityDoesNotPanic(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				first, _ := state.Lex([]byte(line))
				second, next := state.Lex([]byte(line))
				_, _ = first, second

				// slice 에 담아 견준다. 곧바로 견주면 컴파일러가 접어 버릴 수 있다.
				pair := []State{next, next}
				assert.NotPanics(t, func() { _ = pair[0] == pair[1] },
					"%T 를 `==` 로 견줄 수 없습니다: %q", next, line)

				state = next
			}
		})
	}
}

// uncomparableState 는 위 시험이 헛돌지 않는지 보는 데만 쓴다.
// 견줄 수 없는 칸(slice) 을 들었다.
type uncomparableState struct {
	seen []int
}

func (s uncomparableState) Lex([]byte) ([]Token, State) {
	return nil, s
}

// 문지기가 실제로 잡는지 본다. 이것이 없으면 지키는 것이 없는 시험을 들고 있게 된다.
//
// 여러 줄에 걸친 문맥이 안쪽 언어의 문맥을 품게 되면서(mdFence.inner, htmlRawText.inner)
// 이 구멍이 떠받치는 자리가 되었다.
func TestUncomparableStatePanics(t *testing.T) {
	pair := []State{uncomparableState{}, uncomparableState{}}

	assert.Panics(t, func() { _ = pair[0] == pair[1] },
		"견줄 수 없는 문맥인데 `==` 가 통과했습니다 — 문지기가 헛돌고 있습니다")
}

// 같은 문맥에 같은 줄을 두 번 먹이면 같은 결과여야 한다. 캐시가 그것을 믿는다.
func TestLexIsRepeatable(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				first, next := state.Lex([]byte(line))
				second, again := state.Lex([]byte(line))

				assert.Equal(t, first, second, "토큰이 다릅니다: %q", line)
				assert.Equal(t, next, again, "나가는 문맥이 다릅니다: %q", line)

				state = next
			}
		})
	}
}

// KindPlain 은 「강조 없음」이라 토큰이 되지 않는다. 비우는 것으로 나타낸다(syntax.go).
func TestKindPlainNotEmitted(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			state := sample.state
			for _, line := range sample.lines {
				var tokens []Token
				tokens, state = state.Lex([]byte(line))

				for _, token := range tokens {
					assert.NotEqual(t, KindPlain, token.Kind,
						"KindPlain 을 내보냈습니다: %q", line)
				}
			}
		})
	}
}

// 빈 줄에는 칠할 것이 없다. 어느 문맥에서든 그렇다.
func TestEmptyLineHasNoTokens(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			for _, state := range []State{sample.state, openedState(t, sample.state, sample.lines)} {
				tokens, _ := state.Lex(nil)

				assert.Empty(t, tokens, "%T 에서 빈 줄에 토큰이 붙었습니다", state)
			}
		})
	}
}

// 빈 줄은 시작 문맥을 바꾸지 않는다.
//
// 여러 줄에 걸친 문맥에는 이것을 걸지 않는다. `\` 로 이어진 줄은 빈 줄에서 **끝나는 것이
// 맞다** — make 도 docker 도 빈 줄에서 논리적 한 줄이 끝난다. 반대로 블록 주석이나 코드펜스는
// 빈 줄이 닫지 않는데, 그것은 언어별 시험이 본다.
func TestEmptyLineKeepsStartState(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			_, next := sample.state.Lex(nil)

			assert.Equal(t, sample.state, next, "빈 줄이 시작 문맥을 바꿨습니다")
		})
	}
}

// openedState 는 표본을 훑는 동안 나온 문맥 중 시작 문맥이 아닌 것 하나다.
// 여러 줄에 걸친 것 안의 문맥을 얻는 데 쓴다.
func openedState(t *testing.T, start State, lines []string) State {
	t.Helper()

	state := start
	for _, line := range lines {
		_, state = state.Lex([]byte(line))
		if state != start {
			return state
		}
	}

	t.Fatalf("표본에 여러 줄에 걸치는 것이 없습니다")

	return nil
}

// 표본이 자리를 어긋나게 하는 세 가지를 실제로 들고 있는지 본다. 표본이 싱거워지면
// 위의 성질 시험들이 아무것도 지키지 않게 된다.
func TestSamplesCoverHardCases(t *testing.T) {
	for _, sample := range languageSamples {
		t.Run(sample.name, func(t *testing.T) {
			joined := strings.Join(sample.lines, "\n")

			assert.Contains(t, joined, "한", "표본에 한글이 없습니다")
			assert.Contains(t, sample.lines, "", "표본에 빈 줄이 없습니다")
			assert.NotEqual(t, sample.state, openedState(t, sample.state, sample.lines),
				"표본이 여러 줄에 걸치는 문맥을 열지 않습니다")
		})
	}
}
