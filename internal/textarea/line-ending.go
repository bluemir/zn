package textarea

// lineEnding 은 파일의 줄끝 형식이다. 읽을 때 판정해서 저장할 때 그대로 되돌린다.
type lineEnding int

const (
	lineEndingLF lineEnding = iota
	lineEndingCRLF
)

func (e lineEnding) bytes() []byte {
	if e == lineEndingCRLF {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// name 은 사람에게 보이는 이름이다. `.editorconfig` 의 `end_of_line` 값과 같은 글자다 —
// 저장할 때 무엇으로 맞췄는지 알리는 자리가 쓴다(editorconfig.go, ADR-0052).
func (e lineEnding) name() string {
	if e == lineEndingCRLF {
		return "CRLF"
	}

	return "LF"
}
