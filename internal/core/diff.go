package core

import (
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/bluemir/zn/internal/syntax"
)

// 두 쪽을 견줘 조각(hunk) 목록을 내는 자리다(ADR-0140).
//
// **마커의 셈을 나눠 쓰지 않는다.** `gitLineMarks`(textarea/git-lines.go) 는 답이
// `map[int]GitLineMark` 라 지금 buffer 의 줄에 무엇이 서는지만 말하고 HEAD 쪽 줄은 버린다.
// 이 판은 양쪽 줄을 다 그려야 하므로 그 답으로는 아무것도 못 그린다. 겹치는 것은 「지운 것
// 바로 뒤에 넣은 것이 붙어 있으면 그 겹치는 만큼이 고친 자리」라는 규칙 하나인데, 그것을
// 공통 함수로 빼면 두 자리가 서로의 사정에 묶인다(ADR-0140 §9).

// diffContext 는 조각 둘레에 붙이는 같은 줄 수다. `git diff` 와 같이 셋이다.
const diffContext = 3

// diffTimeout 은 줄 diff 에 주는 시간 상한이다.
//
// 마커의 100ms(`gitDiffTimeout`) 보다 넉넉하다. 그쪽은 타이핑이 멎을 때마다 250ms 사이에
// 끼어드는 셈이고 이쪽은 판을 열 때 한 번 내는 셈이라, 같은 제약을 받을 자리가 아니다.
// 읽고 견주는 일이 통째로 Cmd 로 나가 있어서 이 동안 화면이 멈추지도 않는다(ADR-0140 §9).
const diffTimeout = 2 * time.Second

// diffSpanTimeout 은 줄 **안** 글자 diff 에 주는 상한이다. 줄마다 도는 셈이라 짧다.
const diffSpanTimeout = 20 * time.Millisecond

// diffSpanMaxBytes 는 줄 안 글자 diff 를 시도할 줄 길이 상한이다.
//
// 한 줄로 밀어 넣은 minified 파일이 이 자리를 터뜨린다. 넘으면 글자 구간 없이 줄만 칠한다 —
// 그런 줄은 어차피 화면 폭에서 잘려서 달라진 자리가 보이지도 않는다.
const diffSpanMaxBytes = 4096

// diffSide 는 견주는 한 쪽이다.
type diffSide struct {
	// label 은 제목줄에 적는 이름이다. `HEAD`·짧은 해시·경로다.
	label string

	// path 는 이 쪽의 파일 이름이다. 문법을 고르는 데 쓰고, 오른쪽 것은 `enter` 로 연다.
	path string

	lines [][]byte

	// tokens 는 줄마다의 문법 토큰이다. 모르는 언어면 nil 이다(lexDiffLines).
	tokens [][]syntax.Token
}

// tokensAt 은 그 줄의 문법 토큰이다. 없으면 nil 이라 색 없이 그려진다.
func (side diffSide) tokensAt(line int) []syntax.Token {
	if line < 0 || line >= len(side.tokens) {
		return nil
	}

	return side.tokens[line]
}

// textAt 은 그 줄의 글이다. 없는 줄이면 nil 이다.
func (side diffSide) textAt(line int) []byte {
	if line < 0 || line >= len(side.lines) {
		return nil
	}

	return side.lines[line]
}

// diffRowKind 는 한 자리가 어떻게 다른지다.
type diffRowKind byte

const (
	diffRowSame    diffRowKind = iota // 양쪽이 같다
	diffRowChanged                    // 양쪽에 다 있고 내용이 다르다
	diffRowAdded                      // 오른쪽에만 있다
	diffRowRemoved                    // 왼쪽에만 있다
)

// changed 는 이 자리가 바뀐 자리인지다. 조각을 묶는 자리와 `]c` 가 이것을 본다.
func (kind diffRowKind) changed() bool { return kind != diffRowSame }

// diffSpan 은 줄 안에서 달라진 byte 구간이다. Start 는 포함, End 는 제외다.
type diffSpan struct{ start, end int }

// diffCell 은 한 자리의 한쪽 줄이다. 그쪽에 줄이 없으면 line 이 -1 이다.
type diffCell struct {
	line int

	// spans 는 줄 안에서 달라진 구간이다. 고친 자리(diffRowChanged) 에만 있다.
	spans []diffSpan
}

// exists 는 이쪽에 줄이 있는지다. 없으면 그 자리가 filler 다.
func (cell diffCell) exists() bool { return cell.line >= 0 }

// diffRow 는 왼쪽과 오른쪽을 짝지은 한 자리다.
//
// **보기 둘이 이것을 다르게 편다.** unified 는 고친 자리를 두 행(`-` 와 `+`) 으로 내고
// side-by-side 는 한 행의 좌우로 낸다. 그래서 자료가 화면 행이 아니라 이 짝이다
// (ADR-0140 §2).
type diffRow struct {
	kind        diffRowKind
	left, right diffCell
}

// diffHunk 는 이어진 변경 한 덩이와 그 둘레의 같은 줄들이다.
type diffHunk struct {
	rows []diffRow

	// 머리줄(`@@ -47,2 +47,4 @@`) 에 적는 수다. 줄번호는 1 부터이고, 그쪽에 든 줄이
	// 하나도 없으면 start 가 0 이다. git 도 그렇게 적는다.
	leftStart, leftCount   int
	rightStart, rightCount int
}

// buildDiffHunks 는 두 쪽을 견줘 조각 목록을 낸다. 다른 것이 없으면 빈 목록이다.
func buildDiffHunks(left, right [][]byte) []diffHunk {
	return groupDiffHunks(buildDiffRows(left, right))
}

// buildDiffRows 는 두 쪽의 줄을 짝지어 늘어놓는다. 같은 줄까지 전부 든다.
//
// **줄 하나를 글자 하나로 바꿔서 견준다.** go-diff 는 글자 단위 diff 라 이 변환이 곧 줄
// diff 다. 마커 쪽과 같은 손이다(textarea/git-lines.go).
//
// 앞뒤 공통 줄을 떼지 않는다. 마커 쪽은 그것이 값을 정하는데, 거기서는 **답이 마커뿐**이라
// 떼어낸 줄을 다시 볼 일이 없다. 이 판은 떼어낸 줄도 문맥으로 그려야 해서 도로 이어 붙이는
// 일이 생기고, 그 값이 떼어서 아낀 값을 넘는다.
func buildDiffRows(left, right [][]byte) []diffRow {
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = diffTimeout

	a, b, _ := dmp.DiffLinesToRunes(joinDiffLines(left), joinDiffLines(right))
	diffs := dmp.DiffMainRunes(a, b, false)

	rows := make([]diffRow, 0, max(len(left), len(right)))
	at, to := 0, 0

	for i := 0; i < len(diffs); i++ {
		count := len([]rune(diffs[i].Text))

		switch diffs[i].Type {
		case diffmatchpatch.DiffEqual:
			for range count {
				rows = append(rows, diffRow{
					kind:  diffRowSame,
					left:  diffCell{line: at},
					right: diffCell{line: to},
				})
				at, to = at+1, to+1
			}
		case diffmatchpatch.DiffInsert:
			for range count {
				rows = append(rows, diffRow{
					kind:  diffRowAdded,
					left:  diffCell{line: -1},
					right: diffCell{line: to},
				})
				to++
			}
		case diffmatchpatch.DiffDelete:
			// 지운 것 바로 뒤에 넣은 것이 붙어 있으면 그 겹치는 만큼이 **고친 자리**다.
			// 사람이 한 일은 하나인데 diff 는 지우기와 넣기 둘로 적기 때문이다.
			inserted := 0
			if i+1 < len(diffs) && diffs[i+1].Type == diffmatchpatch.DiffInsert {
				inserted = len([]rune(diffs[i+1].Text))
				i++
			}

			paired := min(count, inserted)
			for range paired {
				from, into := diffSpansOf(lineAt(left, at), lineAt(right, to))

				rows = append(rows, diffRow{
					kind:  diffRowChanged,
					left:  diffCell{line: at, spans: from},
					right: diffCell{line: to, spans: into},
				})
				at, to = at+1, to+1
			}

			// 지운 것이 더 많으면 남는 줄은 오른쪽에 짝이 없다.
			for range count - paired {
				rows = append(rows, diffRow{
					kind:  diffRowRemoved,
					left:  diffCell{line: at},
					right: diffCell{line: -1},
				})
				at++
			}

			// 넣은 것이 더 많으면 남는 줄은 새로 생긴 줄이다.
			for range inserted - paired {
				rows = append(rows, diffRow{
					kind:  diffRowAdded,
					left:  diffCell{line: -1},
					right: diffCell{line: to},
				})
				to++
			}
		}
	}

	return rows
}

// groupDiffHunks 는 바뀐 자리 둘레를 문맥만큼 넓혀 조각으로 묶는다.
//
// **붙으면 이어 붙인다.** 두 변경 사이가 문맥 여섯 줄 안이면 조각 둘이 겹쳐서, 그대로 두면
// 같은 줄이 화면에 두 번 그려진다. git 도 겹치면 하나로 합친다.
func groupDiffHunks(rows []diffRow) []diffHunk {
	type span struct{ start, end int }

	spans := []span{}
	for at, row := range rows {
		if !row.kind.changed() {
			continue
		}

		start, end := max(at-diffContext, 0), min(at+diffContext+1, len(rows))

		if last := len(spans) - 1; last >= 0 && start <= spans[last].end {
			spans[last].end = end

			continue
		}

		spans = append(spans, span{start: start, end: end})
	}

	hunks := make([]diffHunk, 0, len(spans))
	for _, at := range spans {
		hunks = append(hunks, newDiffHunk(rows[at.start:at.end]))
	}

	return hunks
}

// newDiffHunk 는 자른 줄들로 조각 하나를 짓고 머리줄에 적을 수를 센다.
func newDiffHunk(rows []diffRow) diffHunk {
	hunk := diffHunk{rows: rows}

	for _, row := range rows {
		if row.left.exists() {
			if hunk.leftCount == 0 {
				hunk.leftStart = row.left.line + 1
			}

			hunk.leftCount++
		}

		if row.right.exists() {
			if hunk.rightCount == 0 {
				hunk.rightStart = row.right.line + 1
			}

			hunk.rightCount++
		}
	}

	return hunk
}

// diffSpansOf 는 고친 줄 둘에서 실제로 달라진 byte 구간을 낸다. 왼쪽 것과 오른쪽 것이다.
//
// **통째로 다른 줄에는 구간을 두지 않는다.** 같은 자리가 한 byte 도 없으면 줄 전체가 한
// 구간이 되는데, 그것은 줄 바탕이 이미 말하고 있는 사실을 한 번 더 칠하는 것이다.
//
// `DiffCleanupSemantic` 을 지난다. 날것의 글자 diff 는 우연히 같은 글자(`(`·공백) 마다
// 구간이 끊겨서, 한 낱말을 고친 자리가 점점이 칠해진다.
func diffSpansOf(left, right []byte) ([]diffSpan, []diffSpan) {
	if len(left) > diffSpanMaxBytes || len(right) > diffSpanMaxBytes {
		return nil, nil
	}

	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = diffSpanTimeout

	diffs := dmp.DiffCleanupSemantic(dmp.DiffMain(string(left), string(right), false))

	var from, into []diffSpan
	at, to, same := 0, 0, 0

	for _, d := range diffs {
		size := len(d.Text)

		switch d.Type {
		case diffmatchpatch.DiffEqual:
			at, to, same = at+size, to+size, same+size
		case diffmatchpatch.DiffDelete:
			from = append(from, diffSpan{start: at, end: at + size})
			at += size
		case diffmatchpatch.DiffInsert:
			into = append(into, diffSpan{start: to, end: to + size})
			to += size
		}
	}

	if same == 0 {
		return nil, nil
	}

	return from, into
}

// inDiffSpans 는 그 byte 자리가 달라진 구간 안인지다. 구간은 앞에서부터 겹치지 않게 늘어선다.
func inDiffSpans(spans []diffSpan, offset int) bool {
	for _, span := range spans {
		if offset < span.start {
			return false
		}

		if offset < span.end {
			return true
		}
	}

	return false
}

// diffSpanBounds 는 구간들의 경계 byte 자리를 차례대로 준다. 색을 나누는 자리다.
func diffSpanBounds(spans []diffSpan) []int {
	bounds := make([]int, 0, len(spans)*2)
	for _, span := range spans {
		bounds = append(bounds, span.start, span.end)
	}

	return bounds
}

// joinDiffLines 는 줄들을 다시 한 덩이로 잇는다. go-diff 가 문자열을 받는다.
func joinDiffLines(lines [][]byte) string {
	size := 0
	for _, line := range lines {
		size += len(line) + 1
	}

	out := make([]byte, 0, size)
	for i, line := range lines {
		if i > 0 {
			out = append(out, '\n')
		}

		out = append(out, line...)
	}

	return string(out)
}

// lineAt 은 그 줄이다. 범위 밖이면 빈 줄이다.
func lineAt(lines [][]byte, at int) []byte {
	if at < 0 || at >= len(lines) {
		return nil
	}

	return lines[at]
}

// lexDiffLines 는 파일 이름으로 언어를 골라 줄마다 문법 토큰을 낸다.
//
// **통째로 훑는다.** 그리는 것은 조각 안의 줄뿐인데, 줄 하나를 훑으려면 그 앞 줄을 끝낸
// 문맥이 필요해서 위에서부터 내려올 수밖에 없다(textarea/buffer-syntax.go). 판을 열 때 한 번
// 이고 그 일이 Cmd 안에 있다.
//
// 모르는 언어면 nil 이다. 그러면 판이 색 없이 그려진다.
func lexDiffLines(path string, lines [][]byte) [][]syntax.Token {
	lang := syntax.LanguageFor(path)
	if lang == nil {
		return nil
	}

	state := lang.State()
	if state == nil {
		return nil
	}

	tokens := make([][]syntax.Token, len(lines))
	for i := range lines {
		tokens[i], state = state.Lex(lines[i])
	}

	return tokens
}
