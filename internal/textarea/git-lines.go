package textarea

import (
	"bytes"
	"slices"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// 줄번호 왼쪽 마커 칸에 「이 줄이 HEAD 와 다르다」를 그리는 자리다(ADR-0094).
//
// 진단이 서버가 밀어주는 상태였던 것과 달리(ADR-0086) 이것은 우리가 직접 잰다.
// HEAD 에 든 그 파일(gitBase) 과 지금 buffer 를 견주는 일이다.

// gitDiffTimeout 은 줄 diff 에 주는 시간 상한이다.
//
// 재 보니 5,000 줄 파일에서 흩어진 쉰 군데를 고친 것이 2.6ms, 한 군데는 4µs 다(앞뒤 공통 줄을
// 떼는 덕이다). 값이 터지는 것은 **파일이 통째로 달라진 때** 하나인데(50,000 줄에서 110ms)
// `git checkout` 직후나 포매터가 전부를 고친 뒤가 그렇다. 상한에 걸리면 go-diff 는 거친 답을
// 주고 멈춘다 — 마커가 조금 굵어질 뿐 틀리지 않는다.
const gitDiffTimeout = 100 * time.Millisecond

// gitLineMark 는 그 줄이 HEAD 와 어떻게 다른지다.
type GitLineMark byte

const (
	gitLineNone     GitLineMark = iota
	GitLineAdded                // HEAD 에 없던 줄이다
	GitLineModified             // 있던 줄인데 내용이 다르다
	GitLineRemoved              // 이 줄 **다음에** 있던 줄들이 없어졌다
)

// gitLineMarks 는 HEAD 의 내용과 지금 내용을 견줘 줄마다 마커를 낸다.
//
// **앞뒤 공통 줄을 먼저 뗀다.** 편집은 대개 한자리에서 일어나므로 이 한 번으로 diff 가 볼
// 것이 몇 줄로 줄어든다. 값이 파일 크기가 아니라 고친 크기에 비례하게 만드는 것이 이 줄이다.
//
// **지운 줄은 그 앞 줄에 표시한다.** 없어진 줄에는 그릴 칸이 없다. 파일 맨 앞이 지워졌으면
// 갈 곳이 없어서 첫 줄에 선다 — 그 자리에서는 마커가 「위쪽」을 가리킨다.
//
// **한 줄에 둘이 겹치면 고친 것이 이긴다.** 지운 표시는 옆줄에 얹히는 것이라, 그 줄 자신에
// 대한 사실을 밀어내서는 안 된다.
func gitLineMarks(base, now [][]byte) map[int]GitLineMark {
	if len(base) == 0 {
		return nil
	}

	head := 0
	for head < len(base) && head < len(now) && bytes.Equal(base[head], now[head]) {
		head++
	}

	tail := 0
	for tail < len(base)-head && tail < len(now)-head &&
		bytes.Equal(base[len(base)-1-tail], now[len(now)-1-tail]) {
		tail++
	}

	base = base[head : len(base)-tail]
	now = now[head : len(now)-tail]

	if len(base) == 0 && len(now) == 0 {
		return nil
	}

	marks := map[int]GitLineMark{}

	// 앞뒤를 뗀 나머지가 한쪽뿐이면 diff 를 돌릴 것도 없다. 통째로 넣었거나 통째로 지운 것이다.
	if len(base) == 0 {
		for i := range now {
			marks[head+i] = GitLineAdded
		}

		return marks
	}

	if len(now) == 0 {
		markGitRemoved(marks, head-1)

		return marks
	}

	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = gitDiffTimeout

	// 줄 하나를 글자 하나로 바꿔서 견준다. go-diff 는 글자 단위 diff 라 이 변환이 곧 줄 diff 다.
	left, right, _ := dmp.DiffLinesToRunes(joinLines(base), joinLines(now))

	line := head

	diffs := dmp.DiffMainRunes(left, right, false)
	for i := 0; i < len(diffs); i++ {
		count := len([]rune(diffs[i].Text))

		switch diffs[i].Type {
		case diffmatchpatch.DiffEqual:
			line += count
		case diffmatchpatch.DiffInsert:
			for range count {
				marks[line] = GitLineAdded
				line++
			}
		case diffmatchpatch.DiffDelete:
			// 지운 것 바로 뒤에 넣은 것이 붙어 있으면 그 겹치는 만큼이 「고친 줄」이다.
			// 사람이 한 일은 하나인데 diff 는 지우기와 넣기 둘로 적기 때문이다.
			inserted := 0
			if i+1 < len(diffs) && diffs[i+1].Type == diffmatchpatch.DiffInsert {
				inserted = len([]rune(diffs[i+1].Text))
				i++
			}

			for range min(count, inserted) {
				marks[line] = GitLineModified
				line++
			}

			// 넣은 것이 더 많으면 남는 줄은 새로 생긴 줄이다.
			for range inserted - min(count, inserted) {
				marks[line] = GitLineAdded
				line++
			}

			// 지운 것이 더 많으면 그만큼은 화면에 없다. 앞 줄에 표시를 남긴다.
			if count > inserted {
				markGitRemoved(marks, line-1)
			}
		}
	}

	return marks
}

// markGitRemoved 는 지워진 자리를 그 앞 줄에 남긴다. 앞 줄이 없으면 첫 줄이다.
func markGitRemoved(marks map[int]GitLineMark, line int) {
	line = max(line, 0)

	if marks[line] == gitLineNone {
		marks[line] = GitLineRemoved
	}
}

// joinLines 는 줄들을 다시 한 덩이로 잇는다. go-diff 가 문자열을 받는다.
func joinLines(lines [][]byte) string {
	return string(bytes.Join(lines, []byte{'\n'}))
}

// refreshGitLines 는 이 buffer 의 줄 마커를 다시 낸다.
//
// HEAD 원본이 없으면(저장소가 아니거나, 추적하지 않는 새 파일이거나, 아직 못 읽었으면)
// 마커도 없다. 새 파일의 모든 줄에 `+` 를 세우는 길도 있지만, 그것은 파일이 새것이라는
// 말을 줄 수만큼 되풀이하는 것이다 — 그 사실은 트리의 `?` 가 한 번 말한다(ADR-0094 §4).
func (buf *Buffer) RefreshGitLines() {
	buf.git.marks = gitLineMarks(buf.git.base, buf.lines)
}

// gitChangeLines 는 git 으로 바뀐 자리들의 **첫 줄**이다(ADR-0094).
//
// **잇달아 붙은 줄은 한 자리로 본다.** 열 줄을 고쳤으면 `~` 가 열 개 서는데, 뛰는 쪽에서는
// 그것이 열 곳이 아니라 한 곳이다. vim 이 `]c` 를 「변경의 시작으로」라고 적어 둔 것과 같다.
func (buf *Buffer) GitChangeLines() []int {
	if len(buf.git.marks) == 0 {
		return nil
	}

	lines := make([]int, 0, len(buf.git.marks))
	for line := range buf.git.marks {
		if buf.git.marks[line-1] == gitLineNone {
			lines = append(lines, line)
		}
	}

	slices.Sort(lines)

	return lines
}
