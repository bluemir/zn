package core

import (
	"github.com/bluemir/zn/internal/assets"
)

// `:+1:` 처럼 쳐서 특수문자를 넣는 자리다(ADR-0135).
//
// **후보를 세우는 것만 여기 있다.** 목록과 키와 그리는 것은 completion.go·render-completion.go
// 것을 그대로 쓴다. 창을 하나로 둔 까닭이 그것이다 — 뜨는 자리와 고르는 키가 서버 후보와
// 같아야 두 갈래를 따로 익히지 않는다.

// symbolAliasChar 는 `:` 뒤에 이어질 수 있는 글자다.
//
// github 의 이모지 이름이 쓰는 글자다(`+1`·`ok_hand`·`white_check_mark`). 여기 없는 글자를
// 만나면 그 자리에서 별칭이 끝난 것으로 본다.
func symbolAliasChar(b byte) bool {
	return b == '+' || b == '-' || b == '_' ||
		(b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// symbolAliasAt 는 커서 앞이 `:별칭` 꼴인지 보고 그 별칭과 `:` 의 자리를 준다.
//
// **`:` 뒤에 한 글자는 있어야 한다.** `:` 만 쳤을 때 목록이 뜨면 산문과 yaml 에서 걸리는
// 자리가 너무 넓다. 닫는 `:` 까지 친 것도 같은 별칭으로 본다 — `:+1:` 를 끝까지 치는 사람과
// 목록에서 고르는 사람이 같은 답에 닿아야 한다.
//
// 별칭에 없는 글자를 만나면 곧바로 그만둔다. 줄 전체를 훑지 않는다.
func symbolAliasAt(line []byte, col int) (alias string, start int, ok bool) {
	if col > len(line) {
		return "", 0, false
	}

	end := col

	// 닫는 `:` 는 별칭에 들지 않는다. 그 앞까지가 이름이다.
	if end > 0 && line[end-1] == ':' {
		end--
	}

	at := end
	for at > 0 && symbolAliasChar(line[at-1]) {
		at--
	}

	// `:` 로 열려 있고 그 뒤에 한 글자라도 있어야 한다.
	if at == 0 || line[at-1] != ':' || at == end {
		return "", 0, false
	}

	return string(line[at:end]), at - 1, true
}

// inSymbolAlias 는 커서가 `:별칭` 안에 있는지다.
//
// **걸리는 별칭이 있는지와 다른 질문이다.** 자리이기만 하면 서버에 물을 곳이 아니다 —
// `:zz` 처럼 아무것도 걸리지 않는 것도 서버가 답할 자리가 아니라서, 그때는 뜬 목록을 닫는
// 것이 맞다. 그 갈림을 부르는 쪽에서 세우려면 「걸렸나」와 「자리인가」가 한 답에 섞인다.
func (e editor) inSymbolAlias() bool {
	if !e.hasTab() {
		return false
	}

	buf := e.activeBuffer()
	_, _, ok := symbolAliasAt(buf.Line(buf.Cursor.Line), buf.Cursor.Col)

	return ok
}

// symbolCompletionLimit 은 목록에 실어 보내는 후보 수다.
//
// 창에 여덟 줄이 보이고(completionRows) 밀어서 더 볼 수 있다. 별칭이 손으로 적은 것뿐이라
// 이 수를 넘게 걸리는 패턴은 없지만, 표가 늘어도 창이 목록을 다 들고 있을 이유는 없다.
const symbolCompletionLimit = 24

// startSymbolCompletion 은 커서 앞의 `:별칭` 으로 목록을 세운다. 세울 것이 없으면 거짓이다.
//
// **서버에 묻지 않는다.** 답이 우리 표에 있어서 기다릴 것이 없고, 그래서 LSP 가 붙지 않는
// 파일(markdown·평문) 에서도 뜬다.
//
// 넣을 범위는 `:` 부터 커서까지다. 친 것을 우리가 세는 자리인데, 서버 후보와 달리 범위를
// 알려줄 사람이 없어서 여기서 담는다(completionItem 의 start·end).
func (e *editor) startSymbolCompletion() bool {
	if !e.hasTab() {
		return false
	}

	buf := e.activeBuffer()
	line := buf.Cursor.Line

	alias, start, ok := symbolAliasAt(buf.Line(line), buf.Cursor.Col)
	if !ok {
		return false
	}

	items := []completionItem{}
	for _, entry := range symbolsWithAlias() {
		for _, name := range entry.AliasList() {
			if !aliasMatches(name, alias) {
				continue
			}

			items = append(items, completionItem{
				label:  ":" + name + ":  " + entry.Char,
				detail: entry.Name,
				text:   entry.Char,
				start:  start,
				end:    buf.Cursor.Col,
			})

			break
		}

		if len(items) >= symbolCompletionLimit {
			break
		}
	}

	if len(items) == 0 {
		return false
	}

	e.completion = completion{items: items, line: line}

	return true
}

// symbolsWithAlias 는 별칭이 붙은 글자들이다.
//
// 큐레이션 표에서 곧바로 읽는다. editor 가 든 목록(e.symbols) 이 아니라 표를 보는 것은,
// 그쪽이 유니코드 훑기로 12000 개까지 늘고 훑어서 얻은 글자에는 별칭이 없어서다.
func symbolsWithAlias() []assets.Symbol {
	with := make([]assets.Symbol, 0, 64)
	for _, entry := range assets.CuratedSymbols {
		if entry.Aliases != "" {
			with = append(with, entry)
		}
	}

	return with
}

// aliasMatches 는 친 것이 그 별칭에 걸리는지다. 앞에서부터 맞아야 한다.
//
// 팔레트의 흩어진 맞추기(filterPalette) 를 쓰지 않는다. 저쪽은 목록을 눈으로 훑는 자리라
// 넓게 걸리는 것이 이득인데, 여기는 글자마다 목록이 뜨고 내려가는 자리라 `:oh` 가 다른
// 별칭까지 끌어오면 무엇이 뜬 것인지 종잡을 수 없다.
func aliasMatches(name, typed string) bool {
	if len(typed) > len(name) {
		return false
	}

	return name[:len(typed)] == typed
}
