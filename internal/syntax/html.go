package syntax

import (
	"bytes"
	"slices"
	"strings"
)

// htmlNormal 은 tag 밖의 글이다.
type htmlNormal struct{}

// htmlTag 는 tag 안이다. `>` 가 다음 줄에 있는 tag 에서 쓰인다.
type htmlTag struct{}

// htmlComment 는 `<!--` 로 열린 주석 안이다.
type htmlComment struct{}

// htmlAttrValue 는 따옴표로 열린 속성 값 안이다. 여는 따옴표 글자를 든다.
type htmlAttrValue struct {
	quote byte
}

// htmlRawText 는 `<script>`·`<style>` 안이다. 닫는 tag 이름을 든다.
//
// 이름은 소문자로 담는다. `<SCRIPT>` 와 `<script>` 가 다른 값이면 문맥 수렴이 조용히 깨져서
// 파일 끝까지 다시 훑는다.
type htmlRawText struct {
	tag string

	// inner 는 안쪽 언어의 문맥이다. `<script>` 는 js, `<style>` 는 css 다.
	//
	// 이 칸이 interface 라 **안에 들어오는 것은 반드시 `==` 로 견줄 수 있어야 한다**
	// (ADR-0039, ADR-0040).
	inner State
}

func (htmlNormal) Indent() Indent    { return htmlIndent{} }
func (htmlTag) Indent() Indent       { return htmlIndent{} }
func (htmlComment) Indent() Indent   { return htmlIndent{} }
func (htmlAttrValue) Indent() Indent { return htmlIndent{} }

// Indent 는 `<script>`·`<style>` 안에서 안쪽 언어의 규칙이다. 강조를 넘기는 것과 같은 자리다.
//
// 안쪽 언어를 모르면(`<script type="text/template">`) html 규칙이다. 그 안은 우리가 무엇인지
// 모르는 글이고, html 규칙은 tag 를 보는 것이라 tag 가 없는 줄에 아무 일도 하지 않는다.
func (s htmlRawText) Indent() Indent {
	if s.inner == nil {
		return htmlIndent{}
	}

	return s.inner.Indent()
}

func (s htmlNormal) Lex(line []byte) ([]Token, State) {
	return htmlScan(line, 0, false)
}

func (s htmlTag) Lex(line []byte) ([]Token, State) {
	return htmlScan(line, 0, true)
}

func (s htmlComment) Lex(line []byte) ([]Token, State) {
	end := bytes.Index(line, []byte("-->"))
	if end < 0 {
		if len(line) < 1 {
			return nil, s
		}

		return []Token{{Start: 0, End: len(line), Kind: KindComment}}, s
	}

	tail, next := lexTail(htmlNormal{}, line, end+3)

	return append([]Token{{Start: 0, End: end + 3, Kind: KindComment}}, tail...), next
}

func (s htmlAttrValue) Lex(line []byte) ([]Token, State) {
	for at := 0; at < len(line); at++ {
		if line[at] == s.quote {
			tail, next := lexTail(htmlTag{}, line, at+1)

			return append([]Token{{Start: 0, End: at + 1, Kind: KindString}}, tail...), next
		}
	}

	if len(line) < 1 {
		return nil, s
	}

	return []Token{{Start: 0, End: len(line), Kind: KindString}}, s
}

// Lex 는 닫는 tag 를 찾을 때까지 안쪽을 그 언어로 훑는다.
func (s htmlRawText) Lex(line []byte) ([]Token, State) {
	at := htmlIndexFold(line, []byte("</"+s.tag))
	if at < 0 {
		if s.inner == nil {
			return nil, s
		}

		tokens, inner := s.inner.Lex(line)

		return tokens, htmlRawText{tag: s.tag, inner: inner}
	}

	// 닫는 tag **앞까지**가 안쪽이다. 앞쪽 조각이라 자리가 이미 줄 기준이어서 되돌릴 것이
	// 없고, End 가 at 을 넘지 않아 줄을 벗어날 수 없다.
	//
	// 안쪽 문맥은 버린다. 닫는 tag 는 안쪽에서 무엇이 열려 있든 이긴다 — 브라우저도 그렇다.
	var tokens []Token
	if s.inner != nil {
		tokens, _ = s.inner.Lex(line[:at])
	}

	tail, next := lexTail(htmlNormal{}, line, at)

	return append(tokens, tail...), next
}

// htmlScan 은 줄 하나를 훑는다. inTag 면 tag 안에서 시작한다.
//
// tag 안과 밖을 한 고리에서 다룬다. 나눠 두면 `<script>` 처럼 「tag 를 닫고 나서 그 줄의
// 나머지가 html 이 아닌」 자리를 표현할 수 없다 — 그 줄의 남은 부분을 누가 볼지가 tag 를
// 닫는 순간에 정해지기 때문이다.
func htmlScan(line []byte, from int, inTag bool) ([]Token, State) {
	tokens := []Token{}

	// raw 는 이 줄에서 연 `<script>`·`<style>` 의 이름이다. tag 를 닫는 순간 그 뒤가 안쪽이 된다.
	raw := ""

	for at := from; at < len(line); {
		if inTag {
			switch {
			case line[at] == '>':
				inTag = false
				at++

				if raw == "" {
					continue
				}

				// tag 를 닫는 순간 그 뒤가 안쪽이다. **그 안쪽이 이 줄에서 끝나는지를 여기서
				// 봐야 한다.** 보지 않고 돌아가면 같은 줄의 닫는 tag 를 놓쳐서, 그 뒤 파일
				// 나머지 전부가 안쪽으로 읽힌다.
				inner := htmlRawTextLanguages[raw]

				end := htmlIndexFold(line[at:], []byte("</"+raw))
				if end < 0 {
					// 줄 끝까지가 안쪽이다. 다음 줄도 안쪽에서 시작한다.
					if inner == nil {
						return tokens, htmlRawText{tag: raw}
					}

					body, next := lexTail(inner, line, at)

					return append(tokens, body...), htmlRawText{tag: raw, inner: next}
				}

				// 같은 줄에 닫는 tag 가 있다. **가운데만** 그 언어로 훑고, 닫는 tag 부터는
				// 다시 html 이라 고리를 이어서 돈다.
				//
				// 짧게 자른 줄을 lexTail 에 주면 정확히 line[at:closeAt] 을 훑고 at 만큼
				// 되돌린다 — 가운데 조각에도 같은 helper 가 그대로 쓰인다.
				closeAt := at + end
				if inner != nil {
					body, _ := lexTail(inner, line[:closeAt], at)
					tokens = append(tokens, body...)
				}

				raw = ""
				at = closeAt

				continue

			case line[at] == '"', line[at] == '\'':
				end, ok := quotedEnd(line, at, len(line))
				if !ok {
					tokens = append(tokens, Token{Start: at, End: len(line), Kind: KindString})

					return tokens, htmlAttrValue{quote: line[at]}
				}

				tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
				at = end

				continue

			case isIdentByte(line[at]):
				end := htmlNameEnd(line, at)

				// `=` 뒤의 낱말은 따옴표 없는 값이고, 그 밖은 속성 이름이다.
				// 속성 이름은 「이름 = 값」 의 이름이라 키다(KindKey 설명 참고).
				kind := KindKey
				if htmlAfterEquals(line, at) {
					kind = KindString
				}

				tokens = append(tokens, Token{Start: at, End: end, Kind: kind})
				at = end

				continue
			}

			at++

			continue
		}

		if line[at] == '&' {
			// `&amp;` 는 글자 하나를 가리키는 이름이다.
			if end, ok := htmlEntityEnd(line, at); ok {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindConstant})
				at = end

				continue
			}
		}

		if line[at] != '<' {
			at++

			continue
		}

		if bytes.HasPrefix(line[at:], []byte("<!--")) {
			if end := bytes.Index(line[at+4:], []byte("-->")); end >= 0 {
				to := at + 4 + end + 3
				tokens = append(tokens, Token{Start: at, End: to, Kind: KindComment})
				at = to

				continue
			}

			tokens = append(tokens, Token{Start: at, End: len(line), Kind: KindComment})

			return tokens, htmlComment{}
		}

		// `<!DOCTYPE html>` 은 tag 가 아니라 선언이다. 통째로 한 낱말로 본다.
		if at+1 < len(line) && line[at+1] == '!' {
			end := at + 2
			for end < len(line) && line[end] != '>' {
				end++
			}
			to := min(end+1, len(line))
			tokens = append(tokens, Token{Start: at, End: to, Kind: KindKeyword})
			at = to

			continue
		}

		name, end, closing, ok := htmlTagNameAt(line, at)
		if !ok {
			// `a < b` 의 `<` 다. tag 가 아니면 그냥 글이다.
			at++

			continue
		}

		tokens = append(tokens, Token{Start: at, End: end, Kind: KindKeyword})
		inTag = true
		at = end

		if !closing && htmlRawTextLanguages[name] != nil {
			raw = name
		}
	}

	if inTag {
		return tokens, htmlTag{}
	}

	return tokens, htmlNormal{}
}

// htmlIndexFold 는 대소문자를 가리지 않고 찾는다. `</SCRIPT>` 도 닫는다.
func htmlIndexFold(line, want []byte) int {
	return bytes.Index(bytes.ToLower(line), bytes.ToLower(want))
}

// htmlTagNameAt 은 `<` 자리에서 tag 이름을 읽는다. tag 가 아니면 ok 가 false 다.
func htmlTagNameAt(line []byte, at int) (name string, end int, closing, ok bool) {
	from := at + 1
	if from < len(line) && line[from] == '/' {
		from++
		closing = true
	}
	if from >= len(line) || !isIdentByte(line[from]) {
		return "", 0, false, false
	}

	end = htmlNameEnd(line, from)

	return strings.ToLower(string(line[from:end])), end, closing, true
}

// htmlNameEnd 는 이름이 끝나는 자리다.
// html 이름에는 `-`·`:`·`.` 이 들어간다(`data-id`, `xlink:href`).
func htmlNameEnd(line []byte, at int) int {
	for at < len(line) && (isIdentByte(line[at]) ||
		line[at] == '-' || line[at] == ':' || line[at] == '.') {
		at++
	}

	return at
}

// htmlAfterEquals 는 그 낱말 앞에 `=` 가 있는지다. 따옴표 없는 속성 값을 가리는 데 쓴다.
func htmlAfterEquals(line []byte, at int) bool {
	for i := at - 1; i >= 0; i-- {
		if line[i] == ' ' {
			continue
		}

		return line[i] == '='
	}

	return false
}

// htmlEntityEnd 는 `&이름;` 이 끝나는 자리다.
func htmlEntityEnd(line []byte, at int) (int, bool) {
	end := at + 1
	if end < len(line) && line[end] == '#' {
		end++
	}

	for end < len(line) && isIdentByte(line[end]) {
		end++
	}

	if end > at+1 && end < len(line) && line[end] == ';' {
		return end + 1, true
	}

	return 0, false
}

// htmlRawTextLanguages 는 안쪽이 html 이 아닌 tag 와 그 안쪽 언어다.
// 표에 없으면 raw text tag 가 아니다 — 없는 key 가 nil 이라 표 하나가 두 물음에 같이 답한다.
//
// languageByName 을 쓰지 않는다. 여기 이름은 사람이 고르는 언어 이름이 아니라 html 이 못 박아
// 둔 tag 이름이고, `<script>` 안이 js 라는 것은 문서 쓰는 사람이 고를 수 있는 것이 아니다.
//
// `type` 속성은 보지 않는다. `<script type="application/json">` 도 js 로 훑는데, 틀렸을 때
// 피해가 작다 — 그럴듯한 색이 나오고 자리는 가운데 조각 안이라 벗어나지 않는다(docs/tasks.md).
var htmlRawTextLanguages = map[string]State{"script": jsNormal{}, "style": cssNormal{}}

// htmlIndent 는 html 의 들여쓰기 규칙이다. 블록을 여는 것은 같은 줄에서 닫히지 않은 tag 다.
type htmlIndent struct{}

// htmlVoidTags 는 닫는 tag 가 없는 것들이다. 여는 것만으로 끝나므로 안쪽이 없다.
var htmlVoidTags = []string{
	"area", "base", "br", "col", "embed", "hr", "img", "input",
	"link", "meta", "param", "source", "track", "wbr",
}

func (htmlIndent) Next(line []byte, _ []Token) (int, []byte) {
	// tag 를 세는 것이라 토큰을 쓰지 않는다. 문자열은 속성 값 안이고 거기에는 tag 가 없다.
	depth := 0
	for at := 0; at < len(line); {
		open := bytes.IndexByte(line[at:], '<')
		if open < 0 {
			break
		}
		at += open

		end := bytes.IndexByte(line[at:], '>')
		if end < 0 {
			break
		}

		depth += htmlTagDepth(line[at : at+end+1])
		at += end + 1
	}

	if depth > 0 {
		return 1, nil
	}

	return 0, nil
}

func (htmlIndent) Close(head []byte) int {
	// `</div>` 의 `>` 를 치는 순간이다. 낱말이 아니라 그 자리에서 끝난 것을 안다.
	if bytes.HasPrefix(head, []byte("</")) && bytes.IndexByte(head, '>') >= 0 {
		return 1
	}

	return 0
}

func (htmlIndent) Reindents() bool { return true }

func (htmlIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 space 두 칸이다.
func (htmlIndent) Unit() []byte { return []byte("  ") }

// htmlTagDepth 는 tag 하나가 깊이를 얼마나 바꾸는지다. `<`…`>` 통째로 받는다.
func htmlTagDepth(tag []byte) int {
	if len(tag) < 3 {
		return 0
	}

	inner := tag[1 : len(tag)-1]

	// 주석·선언·자기를 닫는 tag 는 안쪽을 만들지 않는다.
	if inner[0] == '!' || inner[0] == '?' || bytes.HasSuffix(inner, []byte{'/'}) {
		return 0
	}

	if inner[0] == '/' {
		return -1
	}

	name := string(bytes.ToLower(inner[:identEnd(inner, 0, len(inner))]))
	if name == "" || slices.Contains(htmlVoidTags, name) {
		return 0
	}

	return 1
}
