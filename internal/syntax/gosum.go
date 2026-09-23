package syntax

// gosumNormal 은 go.sum·go.work.sum 의 문맥이다.
//
// **문맥이 하나뿐이다.** 줄 하나가 통째로 한 항목이고 주석도 이어지는 줄도 없다. 도구가
// 쓰고 도구가 읽는 파일이라 사람이 손댈 것이 없어서 그렇다.
type gosumNormal struct{}

func (gosumNormal) Indent() Indent { return gosumIndent{} }

// Lex 는 `경로 버전 해시` 세 칸 중 앞의 둘에만 갈래를 준다.
//
// 해시는 비운다. 사람이 읽을 글자가 아닌데 줄에서 가장 긴 칸이라, 색을 주면 눈이 거기로
// 끌려가고 정작 찾는 경로와 버전이 묻힌다.
func (s gosumNormal) Lex(line []byte) ([]Token, State) {
	tokens := []Token{}

	pathStart, pathEnd := gosumField(line, 0)
	if pathEnd == pathStart {
		return tokens, s
	}

	tokens = append(tokens, Token{Start: pathStart, End: pathEnd, Kind: KindKey})

	versionStart, versionEnd := gosumField(line, pathEnd)
	if versionEnd > versionStart {
		tokens = append(tokens, Token{Start: versionStart, End: versionEnd, Kind: KindNumber})
	}

	return tokens, s
}

// gosumField 는 from 부터 다음 낱말이 놓인 자리다. 남은 낱말이 없으면 둘이 같다.
func gosumField(line []byte, from int) (int, int) {
	start := from
	for start < len(line) && line[start] == ' ' {
		start++
	}

	end := start
	for end < len(line) && line[end] != ' ' {
		end++
	}

	return start, end
}

// gosumIndent 는 go.sum 의 들여쓰기 규칙이다. 언제나 0 이다.
//
// 표가 규칙을 요구해서 억지로 채운 자리가 아니다. 「이 파일은 줄을 들여쓰지 않는다」가 답이고,
// 규칙을 비우면 앞 줄의 들여쓰기를 이어서 `go mod tidy` 가 도로 지울 빈 칸이 생긴다.
type gosumIndent struct{}

func (gosumIndent) Next([]byte, []Token) (int, []byte) { return 0, nil }

func (gosumIndent) Close([]byte) int { return 0 }

func (gosumIndent) Reindents() bool { return true }

func (gosumIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 tab 이다. go 도구가 쓰는 글자인데, 들여쓰는 줄이 없어서 쓰일 자리도 없다.
func (gosumIndent) Unit() []byte { return []byte("\t") }
