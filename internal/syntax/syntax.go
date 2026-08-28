// Package syntax 는 줄 하나를 훑어 「어느 자리가 어떤 갈래인가」를 돌려준다.
//
// **색을 모른다.** 갈래(Kind) 까지만 말하고, 갈래를 무엇으로 그릴지는 core 가 정한다
// (core/style.go 의 styleSyntax). 그래서 언어를 더해도 그리는 코드와 팔레트가 바뀌지 않고,
// 색을 고쳐도 lexer 가 바뀌지 않는다.
package syntax

// Kind 는 토큰의 갈래다. 색은 core 가 정한다 — syntax 는 색을 모른다.
//
// **닫힌 집합이 아니다.** 필요하면 늘린다. 늘리는 값이 싼 이유는 core 의 표가 갈래를 색이
// 아니라 style 로 옮기기 때문이다 — 새 갈래가 기존 색을 그대로 쓰거나 굵기·기울임만 얹을 수
// 있어서, 갈래 하나가 팔레트에 색 하나를 요구하지 않는다.
//
// **이름은 그것이 무엇인지로 짓는다.** 어떤 색이어야 하는지로 짓지 않는다 — 그것이 이 경계의
// 뜻이다. 그래서 markdown 제목은 KindKeyword 가 아니라 KindHeading 이다. 언어 이름을 붙이지도
// 않는다(KindMarkdownHeading 아님) — 같은 개념이 언어 수만큼 갈라지면 색표가 어차피 그것을
// 다시 합치고, 갈라 둔 보람이 거기서 사라진다.
//
// 늘리는 데 붙는 위험은 하나다 — 색표에 넣기를 잊으면 조용히 색이 없어진다. 표에 없는 갈래를
// 그리는 쪽이 그냥 흘려보내므로 컴파일도 걸리지 않는다. 그것만 테스트가 지킨다
// (core 의 TestEveryKindHasStyle).
type Kind int

const (
	// KindPlain 은 강조 없음이다. **어느 lexer 도 이것을 내보내지 않는다** — 강조하지 않는
	// 자리는 토큰을 내지 않고 비운다. 첫 자리에 두어 zero value 가 아무 색도 아니게 한다.
	KindPlain Kind = iota

	KindComment  // go js py css html make docker / markdown 인용문
	KindString   // go js py css html(속성 값) docker / markdown 코드 스팬·링크 주소
	KindNumber   // go js py css. css 의 `#fff` 도 여기다
	KindKeyword  // go js py / css 속성·at-rule, docker 명령, make 지시자, html tag 이름
	KindType     // go(미리 선언된 이름·새 type), py(class 이름), css 선택자
	KindFunction // go js py(def), make 대상 이름
	KindConstant // go(`nil` `true`), js, py(`None`), css 값 낱말, html entity
	KindVariable // make·docker 의 `$(...)`·`${...}`, css 의 `var(--x)`, yaml 의 anchor·alias

	// KindKey 는 「이름 = 값」 의 이름 자리다. json·yaml 의 키, html 속성 이름, css 사용자 속성
	// 선언이 이것이다.
	//
	// KindVariable 과 갈라 둔다. 저쪽은 **값을 가리키는** 자리(`$(VAR)`, `var(--x)`) 이고
	// 이쪽은 **값에 이름을 붙이는** 자리다. 설정 파일은 거의 전부가 키와 값이라, 둘을 한
	// 갈래로 두면 `--x: red` 의 선언과 `var(--x)` 의 참조가 같아 보인다.
	//
	// css 의 표준 속성(`color:`) 은 여기가 아니라 KindKeyword 다. 그것은 css 가 아는 낱말이라
	// 예약어에 가깝다 — 사람이 지은 이름만 이 갈래다.
	KindKey

	// 아래 둘은 지금 markdown 만 쓴다. 「셋 이상의 언어가 쓰는 갈래만 둔다」는 잣대는 색을
	// 아끼려던 것이었고, 이 둘은 새 색을 요구하지 않아서(굵기·기울임 + 기존 색) 들였다.
	KindHeading  // markdown 제목. 앞으로 html 의 `<h1>` 도 여기다
	KindEmphasis // markdown 의 `*foo*` `_foo_`

	// KindStrong 은 markdown 의 `**foo**` `__foo__` 다.
	//
	// KindEmphasis 와 갈라 둔다. 둘은 markdown 에서 뜻이 다르다 — 하나는 기울임이고 하나는
	// 굵기다. 한 갈래로 묶으면 `**굵게**` 가 기울어져 그려져서, 쓴 사람이 고른 표시와 화면이
	// 어긋난다.
	KindStrong

	// KindLink 는 가리키는 자리다. markdown 의 `[글](주소)` 의 주소와 `<주소>` 다.
	// 앞으로 html 의 `href`·`src` 값도 여기다.
	//
	// 문자열과 갈래를 나눈 이유는 링크가 **가리키는 것**이라서다. 주소를 문자열 색으로 두면
	// 문서에서 링크와 인용된 글자가 같아 보인다. `Web` 을 붙이지 않은 것은 `[글](docs/spec.md)`
	// 처럼 상대 경로도 같은 자리이기 때문이다.
	KindLink
)

// Token 은 줄 안의 byte 구간과 그 갈래다.
//
// Start 는 포함, End 는 제외다. Start < End 라 빈 토큰은 없다.
type Token struct {
	Start, End int
	Kind       Kind
}

// State 는 줄을 시작할 때의 문맥이다. 스스로 줄을 읽고 다음 문맥을 돌려준다.
//
// 글자를 먹어 토큰을 뱉는 tokenizerState(core/command-parser.go) 와 같은 모양인데, 먹는
// 단위가 글자가 아니라 줄이다. 줄이 단위인 이유는 Buffer 가 줄의 배열이고(ADR-0001) 그리는
// 것도 줄이라, 문맥이 줄 경계에만 있으면 고친 줄부터 아래로만 다시 훑으면 되기 때문이다.
//
// **구현은 반드시 `==` 로 견줄 수 있는 값이어야 한다.** core 가 나온 문맥을 캐시에 든 것과
// 견주어 다시 훑기를 멈춘다. slice·map 을 든 상태를 만들면 컴파일이 아니라 그 자리에서
// panic 이 난다. 그래서 상태는 값이고 receiver 도 값이다 — 포인터 상태는 주소로 견주어져
// 늘 다르고, 그러면 수렴이 조용히 깨져 파일 끝까지 다시 훑는다.
//
// **줄을 고쳐서는 안 된다.** 받는 것은 Buffer 가 든 read-only data 의 subslice 다(ADR-0001).
// 상태가 줄을 들고 있어서도 안 된다 — 위의 견줄 수 있음이 그것을 이미 막는다.
type State interface {
	// Lex 는 줄 하나의 토큰과 다음 줄의 문맥을 준다.
	//
	// 토큰의 자리는 그 줄 안의 byte offset 이고, 앞에서 뒤로 겹치지 않는다. 강조하지 않는
	// 자리는 토큰을 내지 않고 비운다 — 부르는 쪽이 빈 자리를 아무 색 없이 그린다. 갈래마다
	// 토큰을 다 채우는 것보다 이것이 낫다: 화면에 달라지는 것이 없는데 escape 만 늘어난다.
	//
	// 돌려주는 문맥은 nil 이 아니다. 여러 줄에 걸치는 것이 없는 줄이면 자기 자신이다.
	Lex(line []byte) ([]Token, State)

	// Indent 는 이 문맥에서 시작하는 줄이 따르는 들여쓰기 규칙이다.
	//
	// **파일 이름이 아니라 문맥이 고른다.** markdown 코드펜스 안과 html 의 `<script>`·`<style>`
	// 안은 바깥 언어가 아니라 안쪽 언어의 규칙을 받는다. 강조를 안쪽 언어에 넘기는 것과
	// 같은 자리다(ADR-0040).
	//
	// 대부분의 상태는 자기 언어의 규칙을 그대로 돌려준다. 문자열·주석 안에서도 그렇다 —
	// 그 안에서 Enter 를 쳐도 앞 줄의 들여쓰기를 잇는 것이 맞고, 그것이 이 규칙들의 답이다.
	Indent() Indent
}

// lexTail 은 줄의 뒤쪽을 다른 문맥에 맡기고, 나온 자리를 줄 기준으로 되돌린다.
//
// 여러 줄에 걸치던 것이 줄 가운데서 끝나는 자리마다 필요하다 — `*/` 뒤, 닫는 백틱 뒤,
// `"""` 뒤, `-->` 뒤가 그렇다. 맡기는 것은 잘라낸 뒤쪽이라 offset 이 0 부터 다시 세지므로
// 잘라낸 만큼 더한다.
//
// **되풀이가 거슬려서 모은 것이 아니라 규칙이 하나여서 모았다.** 「자리는 줄 기준」은 이
// 패키지의 계약이고, 그것을 손으로 되돌리는 자리가 언어마다 있다. 한 군데만 틀리면 엉뚱한
// byte 에 색이 입혀지고, End 가 줄을 넘으면 그리는 쪽이 없는 byte 를 집어 죽는다
// (core/render-row.go 의 expandRow).
func lexTail(state State, line []byte, from int) ([]Token, State) {
	tokens, next := state.Lex(line[from:])
	for i := range tokens {
		tokens[i].Start += from
		tokens[i].End += from
	}

	return tokens, next
}

// identEnd 는 그 자리에서 시작하는 이름이 끝나는 자리다. 이름이 아니면 at 을 그대로 준다.
//
// 글자·숫자·밑줄과 0x80 이상인 byte 를 이름으로 본다. 한글 이름을 쓸 수 있는 언어들이
// 있고(python 3, js) 0x80 이상은 이름 아닌 자리에 나올 일이 거의 없다.
//
// `-` 를 넣지 않는다. css 는 이름에 `-` 가 들어가서 자기 것을 따로 쓴다.
func identEnd(line []byte, at, limit int) int {
	for at < limit && isIdentByte(line[at]) {
		at++
	}

	return at
}

func isIdentByte(b byte) bool {
	switch {
	case b >= '0' && b <= '9', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b == '_':
		return true
	case b >= 0x80:
		return true
	}

	return false
}
