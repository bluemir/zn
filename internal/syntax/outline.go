package syntax

import "math"

// Outline 은 언어 하나의 뼈대 규칙이다. State·Indent 와 나란한 세 번째 언어별 값이다.
//
// **깊이로만 답한다.** 그 깊이를 화면 몇 줄로 보일지, 어디까지 붙일지는 core 가 정한다.
// 여기는 언어를 아는 자리다 — Indent 가 「한 단계가 tab 인지 space 넷인지」를 모르는 것과
// 같은 경계다(ADR-0047).
//
// **깊이의 뜻은 언어마다 달라도 된다.** 한 파일 안에서만 견주기 때문이다 — markdown 은
// `#` 개수이고 나머지는 들여쓴 칸 수다. 두 값이 한 화면에서 만나지 않는다.
type Outline interface {
	// Depth 는 그 줄이 놓인 깊이다. **0 이 가장 바깥이고 작을수록 바깥이다.**
	//
	// 머리줄이 아닌 줄도 답한다. 위로 훑는 쪽이 **지나는 줄마다** 이 값으로 한계를 낮추기
	// 때문이다 — 머리줄만 보고 낮추면 이미 닫힌 블록이 되살아난다(OutlineDeep 설명 참고).
	//
	// 깊이를 말할 수 없는 줄은 OutlineDeep 이다.
	Depth(line []byte, tokens []Token) int

	// Heads 는 그 줄이 아래 줄들을 거느리는 머리줄인지다.
	//
	// 머리줄일 때의 깊이는 따로 묻지 않는다 — Depth 가 그대로 그 값이다.
	Heads(line []byte, tokens []Token) bool
}

// OutlineDeep 은 깊이를 말할 수 없는 줄이다. **어떤 줄보다도 깊다.**
//
// 빈 줄과, 여러 줄에 걸친 문자열·주석 안의 줄이 이것이다. 그런 줄은 코드가 아니라 글이라
// 왼쪽 끝에서 시작해도 「가장 바깥」이라는 뜻이 아니다.
//
// **이것이 없으면 두 가지가 무너진다.** Go 의 raw string 안에 왼쪽 끝으로 붙여 쓴 SQL 한 줄이
// 깊이 0 으로 읽혀서 그 위의 함수를 통째로 가리고, 함수 안의 빈 줄 하나가 위로 훑기를
// 거기서 끝낸다.
const OutlineDeep = math.MaxInt

// outlineTabColumns 는 깊이를 잴 때 tab 하나를 몇 칸으로 보는지다.
//
// core 가 그리는 폭(tabWidth) 과 맞출 필요가 없다. 여기서 재는 것은 줄끼리 견주기 위한
// 것이고, 한 파일이 한 가지 글자로 들여쓰면 값이 무엇이든 순서가 같다. 섞여 있을 때만
// 갈리는데 그때는 어느 값도 옳지 않다.
const outlineTabColumns = 4

// indentDepth 는 들여쓰기를 깊이로 읽는다. markdown 을 뺀 여덟 언어가 나눠 쓴다.
//
// **byte 가 아니라 칸으로 센다.** byte 로 세면 tab 하나(1) 가 space 네 칸(4) 보다 얕아 보여서,
// 섞어 쓴 파일에서 안쪽 줄이 바깥 줄보다 얕다고 나온다.
func indentDepth(line []byte, tokens []Token) int {
	columns := 0
	for at := 0; at < len(line); at++ {
		switch line[at] {
		case ' ':
			columns++
		case '\t':
			columns += outlineTabColumns - columns%outlineTabColumns
		default:
			if insideText(tokens, at) {
				return OutlineDeep
			}

			return columns
		}
	}

	// 공백뿐인 줄이다.
	return OutlineDeep
}

// insideText 는 그 자리가 문자열이나 주석 안인지다.
//
// codeBytes 와 같은 사실을 보지만 줄을 복제하지 않는다 — 여기는 줄마다 도는 자리다.
func insideText(tokens []Token, at int) bool {
	for _, token := range tokens {
		if token.Start <= at && at < token.End {
			return token.Kind == KindString || token.Kind == KindComment
		}
	}

	return false
}

// blockOutline 은 들여쓰기가 곧 깊이인 언어의 뼈대다. 여섯이 나눠 쓴다 — go js css python shell html.
//
// **「다음 줄이 한 단계 들어간다」와 「이 줄이 아래를 거느린다」는 같은 사실이다.** 그 판정을
// 들여쓰기 규칙이 이미 아홉 언어에 해 두었으므로 다시 쓰지 않고 그것에 묻는다 — 문자열·주석
// 안의 괄호를 거르는 것까지 딸려 온다(ADR-0047 이 적어 둔 「같은 토큰을 쓴다」가 이 자리다).
// 표에서 indent 칸과 **같은 값**을 넣는 이유이기도 하다. 둘이 갈리면 한쪽만 고쳐진다.
//
// makefile 과 dockerfile 은 여기 들지 않는다. 둘은 Next 가 내는 답의 뜻이 달라서 자기 것을 쓴다.
type blockOutline struct {
	opens Indent
}

func (rule blockOutline) Depth(line []byte, tokens []Token) int {
	return indentDepth(line, tokens)
}

func (rule blockOutline) Heads(line []byte, tokens []Token) bool {
	level, _ := rule.opens.Next(line, tokens)

	return level > 0
}

// flatOutline 은 뼈대가 없는 언어다. 머리줄이 하나도 없어서 붙는 것도 없다.
//
// dockerfile 이 이것이다. 블록이 없고 `\` 로 줄을 잇는 것뿐이라 거느릴 것이 없다.
// blockOutline 을 주면 **틀린다** — dockerIndent.Next 가 이어짐이 시작하는 줄에 단계 1 을
// 주므로 이어지는 줄마다 머리줄이 되어 화면 위가 `RUN` 으로 뒤덮인다.
type flatOutline struct{}

func (flatOutline) Depth(line []byte, tokens []Token) int { return indentDepth(line, tokens) }

func (flatOutline) Heads([]byte, []Token) bool { return false }
