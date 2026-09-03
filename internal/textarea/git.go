package textarea

// gitCache 는 HEAD 에 든 이 파일과 견줘 낸 것이다(git-lines.go, ADR-0094).
//
// 셋이 **같이 채워지고 같이 낡는다.** head 가 지금 HEAD 와 다르면 base 도 marks 도 낡은
// 것이라 셋을 함께 다시 짓는다 — `git commit`·`checkout` 으로 기준이 통째로 움직이는 자리다.
//
// 값 필드라 Reload 가 buffer 를 통째로 갈아끼울 때(`*buf = next`) 저절로 비워진다. 다음 git
// 갱신이 다시 채운다(진단·문법 캐시와 같은 자리다).
type gitCache struct {
	// base 는 HEAD 에 든 이 파일의 내용이다. 추적하지 않는 파일은 nil 이다.
	base [][]byte

	// head 는 base 를 읽어온 HEAD 해시다. 추적하지 않는 파일은 base 가 nil 인 채 이것만
	// 적힌다 — 「없다」와 「아직 안 읽었다」를 이것이 가른다.
	head string

	// marks 는 base 와 지금 내용을 견줘 낸 줄별 마커다. 줄번호 칸과 트리가 이것을 그린다.
	marks map[int]GitLineMark
}

// hasGitBase 는 견줄 HEAD 원본이 있는지다.
func (buf Buffer) HasGitBase() bool { return len(buf.git.base) > 0 }

// gitHead 는 이 파일이 견주고 있는 commit 이다. 없으면 빈 문자열이다.
func (buf Buffer) GitHead() string { return buf.git.head }

// gitMarkAt 은 그 줄이 HEAD 와 어떻게 다른지다. 그리는 자리가 행마다 묻는다.
func (buf Buffer) GitMarkAt(line int) GitLineMark { return buf.git.marks[line] }

// setGitBase 는 견줄 원본을 갈아끼우고 줄 마커를 다시 잰다.
func (buf *Buffer) SetGitBase(base [][]byte, head string) {
	buf.git.base, buf.git.head = base, head
	buf.RefreshGitLines()
}

// clearGitBase 는 들고 있던 것을 전부 내린다. 저장소가 아닌 자리로 옮겨 갔을 때다 —
// 표시가 남아 있으면 그것이 어느 저장소의 것인지 알 수 없다.
func (buf *Buffer) ClearGitBase() {
	buf.git.base, buf.git.head, buf.git.marks = nil, "", nil
}
