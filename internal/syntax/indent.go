package syntax

import (
	"bytes"
	"strings"
)

// Indent 는 언어 하나의 들여쓰기 규칙이다. State 와 나란한 두 번째 언어별 값이다.
//
// **단계로만 답한다.** 한 단계가 tab 인지 space 몇 칸인지는 파일마다 다르고(.editorconfig,
// 그 파일이 이미 쓰고 있는 것) 그것을 아는 자리는 core 다. 여기는 언어를 아는 자리다.
//
// Unit 만 예외로 공백을 안다. 그것은 「이 파일이 무엇을 쓰는가」가 아니라 「이 언어가 관례로
// 무엇을 쓰는가」라서(gofmt 는 tab, PEP 8 은 space 4) 언어 지식이 맞다. 단서가 하나도 없는
// 빈 새 파일에서만 쓰인다.
type Indent interface {
	// Next 는 line 다음에 만드는 줄이 line 보다 몇 단계 더 들어가는지다.
	//
	// prefix 는 들여쓰기 뒤에 이어 붙일 글자다. markdown 의 목록 표시와 makefile 의 조리법
	// tab 뿐이고 나머지 언어는 nil 이다.
	Next(line []byte, tokens []Token) (level int, prefix []byte)

	// Close 는 줄 앞에 head 가 있을 때 그 줄이 몇 단계 나오는지다.
	//
	// head 는 들여쓰기 다음부터다. **끝이 줄의 끝이라고 보지 않는다** — 앞부분만 견주므로
	// `}` 와 `} else {` 가 같은 답이다. 부르는 자리가 둘이라 그렇다: 치는 도중에는 커서까지가
	// 들어오고(`=` 가 아니라 전기 동작), 이미 있는 줄을 다시 들여쓸 때는 줄 전체가 들어온다.
	//
	// **치는 도중에 줄을 옮길지는 부르는 쪽이 정한다.** 여기서는 「지금 이 줄은 나온 줄이다」만
	// 말한다. 글자 하나가 그 답을 0 에서 1 로 바꾼 순간에만 줄이 움직인다(core/indent.go) —
	// 그래서 `}` 뒤에 글자를 더 쳐도 줄이 또 당겨지지 않는다.
	//
	// 낱말로 닫는 언어(shell 의 `fi`) 는 낱말 뒤에 이름이 아닌 글자가 와야 끝난 것으로 본다.
	// `fi` 두 글자만 보고 당기면 `file=x` 를 칠 때 줄이 튄다. 줄을 떠나는 순간 `\n` 이
	// 붙어 오므로 「치다가 닫았다」와 「닫고 줄을 떠난다」가 규칙 하나로 걸린다.
	Close(head []byte) int

	// Reindents 는 이미 있는 줄들을 규칙만으로 다시 들여쓸 수 있는지다. `=` 가 묻는다.
	//
	// markdown 만 거짓이다 — 목록의 깊이와 네 칸 들여쓴 코드 블록은 글쓴이가 정한 것이라
	// 앞 줄에서 되짚을 수 없다. 되짚으려 들면 중첩 목록이 평평해진다.
	Reindents() bool

	// TabIndentsLine 은 이 줄에서 `tab` 키가 커서 자리가 아니라 **줄 전체**를 들여쓰는지다.
	//
	// markdown 의 목록 줄만 참이다. 목록 안에서 tab 은 「이 항목을 한 단계 깊게」라는 뜻이고,
	// 글 가운데 tab 글자는 markdown 에서 아무 뜻이 없다. 코드에서는 반대라 거짓이다 —
	// tab 은 커서 자리에 넣는 것이다.
	//
	// content 는 그 줄에서 **글이 시작하는 자리**다. 줄 전체가 움직이면 커서가 어디에 서야
	// 하는지는 규칙만 아는 것이라 여기서 같이 낸다. 표시(`- `·`1. `·checklist 의 `[ ] `) 가
	// 차지한 만큼을 지난 offset 이고, 거짓일 때는 0 이다 (ADR-0131).
	TabIndentsLine(line []byte) (content int, indents bool)

	// Unit 은 단서가 없을 때 이 언어가 관례로 쓰는 한 단계다.
	Unit() []byte
}

// codeBytes 는 문자열·주석 자리를 공백으로 지운 줄이다.
//
// 열한 규칙이 전부 이것을 먼저 부른다. 되풀이가 거슬려서가 아니라 「문자열·주석 안의 괄호는
// 세지 않는다」가 규칙 하나여서 모았다 — 한 군데만 틀리면 그 언어만 조용히 어긋난다.
//
// **새로 할당한다.** 받는 줄은 Buffer 가 든 read-only data 의 subslice 라 제자리에서 바꿔서는
// 안 된다(ADR-0001). tokens 가 비어 있으면(아직 훑지 않은 줄, 강조하지 않는 파일) 줄 그대로다.
func codeBytes(line []byte, tokens []Token) []byte {
	code := bytes.Clone(line)

	for _, token := range tokens {
		if token.Kind != KindString && token.Kind != KindComment {
			continue
		}

		for i := max(token.Start, 0); i < min(token.End, len(code)); i++ {
			code[i] = ' '
		}
	}

	return code
}

// bracketDepth 는 이 줄에서 연 괄호와 닫은 괄호의 차다.
// 양수면 열어 둔 채 줄이 끝났고, 음수면 이 줄에서 열지 않은 것을 닫았다.
func bracketDepth(code []byte, open, close string) int {
	depth := 0
	for _, b := range code {
		switch {
		case strings.IndexByte(open, b) >= 0:
			depth++
		case strings.IndexByte(close, b) >= 0:
			depth--
		}
	}

	return depth
}

// bracketOpens 는 이 줄에서 닫히지 않은 여는 괄호가 남았는지다.
//
// 몇 겹이 남았는지는 세지 않는다 — 한 줄에 두 겹을 열어도 한 단계다. gofmt 를 돌려 재 보니
// `foo(T{`·`[]T{{`·`if err := g(T{`·`"k": {{` 가 전부 한 단계이고 닫는 `}}`·`})` 도 한 단계다.
// 세면 그쪽과 어긋난다.
func bracketOpens(code []byte, open, close string) bool {
	return bracketDepth(code, open, close) > 0
}

// closesBracket 은 head 가 닫는 괄호로 시작하는지다.
//
// 뒤에 무엇이 오든 본다 — `}` 도 `} else {` 도 `};` 도 그 줄은 한 단계 나온 자리다.
func closesBracket(head []byte, close string) bool {
	return len(head) > 0 && strings.IndexByte(close, head[0]) >= 0
}

// startsWord 는 head 가 words 중 하나로 시작하고 그 낱말이 거기서 끝났는지다.
//
// 낱말이 끝났다는 것은 바로 뒤에 이름에 쓸 수 없는 글자가 온다는 뜻이다. `fi ` 와 `fi\n` 은
// 걸리고 `file` 은 걸리지 않는다. Enter 로 오는 `\n` 도 그중 하나라서, 낱말을 다 치고 줄을
// 떠나는 것과 낱말 뒤에 공백을 치는 것이 같이 걸린다.
//
// **낱말 하나만 있는 것으로는 모자란다.** `fi` 두 글자만으로 당기면 `file=x` 를 칠 때
// 세 번째 글자에서 이미 줄이 튄 뒤다.
func startsWord(head []byte, words ...string) bool {
	for _, w := range words {
		if len(head) > len(w) && !isIdentByte(head[len(w)]) &&
			bytes.Equal(head[:len(w)], []byte(w)) {
			return true
		}
	}

	return false
}

// firstWord 는 줄 앞 공백을 지난 첫 낱말이다. 낱말이 아니면 빈 것이다.
func firstWord(code []byte) []byte {
	at := 0
	for at < len(code) && (code[at] == ' ' || code[at] == '\t') {
		at++
	}

	return code[at:identEnd(code, at, len(code))]
}

// braceIndent 는 중괄호로 블록을 여닫는 언어의 규칙이다. golang·js·css 셋이 나눠 쓴다.
//
// 세 언어의 모양이 같아서 하나로 둔다. 값이 드는 것은 관례 단계뿐이라 표에서 언어마다 다른
// 값을 준다 — go 는 tab, js·css 는 space 두 칸이다.
//
// `case`/`default` 는 go 와 js 의 switch 모양이다. css 에는 그 낱말이 없어서 걸리지 않는다.
type braceIndent struct {
	unit string
}

const (
	braceOpen  = "{(["
	braceClose = "})]"
)

func (rule braceIndent) Next(line []byte, tokens []Token) (int, []byte) {
	code := codeBytes(line, tokens)

	if bracketOpens(code, braceOpen, braceClose) {
		return 1, nil
	}

	// `case 1:` 아래는 한 단계 들어간다. gofmt 는 `case` 를 `switch` 와 나란히 두고 그 몸통만
	// 들여쓴다. 여는 괄호가 없어서 위의 셈으로는 잡히지 않는다.
	trimmed := bytes.TrimRight(code, " \t")
	if bytes.HasSuffix(trimmed, []byte{':'}) && isSwitchLabel(firstWord(code)) {
		return 1, nil
	}

	// **이어지는 줄은 못 본다.** `if a &&` 나 `x := a +` 로 끝난 줄의 다음 줄은 gofmt 라면 한
	// 단계 들어가는데 여기서는 0 이다. 여는 괄호도 `case` 도 없어서다.
	//
	// 잡으려면 그 줄이 문법으로 끝났는지를 알아야 하고, 그것은 참 파서의 일이다. 편집 중의
	// 소스는 거의 언제나 문법이 깨져 있어서 파서를 두어도 답이 흔들린다. 끝의 낱말이나
	// 연산자를 보고 어림잡는 길도 있는데, 그러면 `a && b` 로 끝나는 멀쩡한 줄까지 걸린다.
	return 0, nil
}

func (rule braceIndent) Close(head []byte) int {
	if closesBracket(head, braceClose) || startsWord(head, "case", "default") {
		return 1
	}

	return 0
}

func (braceIndent) Reindents() bool { return true }

func (braceIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

func (rule braceIndent) Unit() []byte { return []byte(rule.unit) }

func isSwitchLabel(word []byte) bool {
	return bytes.Equal(word, []byte("case")) || bytes.Equal(word, []byte("default"))
}
