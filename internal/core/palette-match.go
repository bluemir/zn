package core

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/bluemir/zn/internal/textarea"
)

// 점수 규칙이다. 한 곳에 모아둬야 순위가 이상할 때 여기만 보면 된다.
//
// fzf 의 값을 그대로 따르되 파일이름 시작에 큰 값을 준 것만 다르다. `edit` 을 쳤을 때
// `internal/core/edit.go` 가 `internal/core/view-editor-command.go` 를 이겨야 하는데,
// 경로가 깊은 저장소에서는 그 보너스가 없으면 목록이 쓸모없어진다.
const (
	scoreChar          = 16 // 글자 하나가 맞았다
	scoreConsecutive   = 8  // 바로 앞 글자도 맞았다
	scoreBoundary      = 8  // 앞이 `/ . - _ 공백` 이다
	scoreCamel         = 7  // 소문자 뒤의 대문자다
	scoreFileNameStart = 16 // 마지막 `/` 바로 뒤다
	scoreFirstChar     = 8  // 맨 앞 글자다
	scoreGapStart      = -3 // 건너뛰기가 시작됐다
	scoreGapExtend     = -1 // 건너뛰기가 이어진다
	scoreCaseExact     = 2  // 대소문자까지 같다
)

// fuzzyMatch 는 pattern 의 글자가 target 안에 순서대로 들어 있는지 본다.
//
// 붙어 있지 않아도 되지만 순서는 지켜야 한다 — `icedit` 이 `internal/core/edit.go` 를 찾는다.
// positions 는 맞은 글자가 시작하는 byte offset 이라 강조가 그대로 쓴다.
//
// 대소문자는 smartcase 다. 패턴이 전부 소문자면 무시하고, 대문자가 하나라도 있으면 가린다.
// 무시하는 중에도 대소문자까지 맞은 글자에 점수를 조금 더 줘서 `E` 가 `Editor` 를 위로 올린다.
func fuzzyMatch(pattern, target string) (score int, positions []int, ok bool) {
	if pattern == "" {
		return 0, nil, true
	}

	line, want := []byte(target), []byte(pattern)
	fold := !hasUpper(want)

	starts, wantStarts := clusterStarts(line), clusterStarts(want)

	matched, ok := matchForward(want, wantStarts, line, starts, 0, fold)
	if !ok {
		return 0, nil, false
	}

	// 앞에서 훑기만 하면 정렬이 흩어진다 — `edit` 이 `e...d...i...t` 로 잡힐 수 있다.
	// 마지막 자리에서 거꾸로 맞춰 시작을 최대한 뒤로 민 다음 거기서 다시 잡으면 가장 뭉친다.
	from := matchBackward(want, wantStarts, line, starts, matched[len(matched)-1], fold)
	if tight, ok := matchForward(want, wantStarts, line, starts, from, fold); ok {
		matched = tight
	}

	positions = make([]int, 0, len(matched))
	for _, index := range matched {
		positions = append(positions, starts[index])
	}

	return fuzzyScore(want, wantStarts, line, starts, matched, fold), positions, true
}

// clusterStarts 는 각 글자가 시작하는 byte offset 이다. 마지막에 len(line) 이 붙는다.
//
// 매칭을 byte 가 아니라 글자 단위로 하려면 경계를 먼저 알아야 한다. grapheme cluster 는
// 뒤에서 앞으로 읽을 수 없어서(buffer.go 의 prevGlyphStart 와 같은 이유) 한 번 모아둔다.
func clusterStarts(line []byte) []int {
	starts := make([]int, 0, len(line)+1)

	for offset := 0; offset < len(line); offset += textarea.GlyphSize(line, offset) {
		starts = append(starts, offset)
	}

	return append(starts, len(line))
}

// matchForward 는 from 번째 글자부터 앞으로 훑으며 패턴을 차례로 맞춘다.
// 맞은 글자의 index 를 준다. 끝까지 못 맞추면 false 다.
func matchForward(want []byte, wantStarts []int, line []byte, starts []int, from int, fold bool) ([]int, bool) {
	matched := make([]int, 0, len(wantStarts)-1)

	next := 0
	for i := from; i < len(starts)-1 && next < len(wantStarts)-1; i++ {
		if !equalCluster(want[wantStarts[next]:wantStarts[next+1]], line[starts[i]:starts[i+1]], fold) {
			continue
		}

		matched = append(matched, i)
		next++
	}

	if next < len(wantStarts)-1 {
		return nil, false
	}

	return matched, true
}

// matchBackward 는 last 번째 글자에서 거꾸로 패턴을 맞춰 시작할 수 있는 가장 늦은 자리를 찾는다.
func matchBackward(want []byte, wantStarts []int, line []byte, starts []int, last int, fold bool) int {
	next := len(wantStarts) - 2

	for i := last; i >= 0; i-- {
		if !equalCluster(want[wantStarts[next]:wantStarts[next+1]], line[starts[i]:starts[i+1]], fold) {
			continue
		}
		if next == 0 {
			return i
		}

		next--
	}

	return 0
}

// fuzzyScore 는 맞은 자리 하나하나에 점수를 매겨 더한다.
//
// 길이는 점수에 넣지 않는다. 넣으면 아래 보너스 서열이 길이에 눌린다. 짧은 것을 위로
// 올리는 것은 정렬의 동점 처리가 맡는다.
func fuzzyScore(want []byte, wantStarts []int, line []byte, starts []int, matched []int, fold bool) int {
	score := 0
	nameStart := nameStartOf(line, starts)

	for order, index := range matched {
		score += scoreChar + startBonus(line, starts, index, nameStart)

		if fold && bytes.Equal(want[wantStarts[order]:wantStarts[order+1]], line[starts[index]:starts[index+1]]) {
			score += scoreCaseExact
		}

		if order == 0 {
			continue
		}

		gap := index - matched[order-1] - 1
		if gap == 0 {
			score += scoreConsecutive

			continue
		}

		score += scoreGapStart + (gap-1)*scoreGapExtend
	}

	return score
}

// nameStartOf 는 파일 이름이 시작하는 글자의 index 다. 마지막 `/` 바로 뒤이고, `/` 가 없으면 0 이다.
func nameStartOf(line []byte, starts []int) int {
	nameStart := 0

	for i := 0; i < len(starts)-1; i++ {
		if line[starts[i]] == '/' {
			nameStart = i + 1
		}
	}

	return nameStart
}

// startBonus 는 그 자리가 무엇의 시작인지에 주는 점수다. 사람이 단어의 첫 글자를 치기 때문이다.
//
// 파일 이름의 시작은 **마지막** `/` 뒤여야 한다. `/` 뒤를 전부 그렇게 치면
// `internal/core.go` 와 `internal/core/buffer.go` 가 `core` 에 같은 점수를 받아서,
// 정작 이 보너스로 가리려던 것을 못 가린다.
func startBonus(line []byte, starts []int, index, nameStart int) int {
	if index == nameStart {
		return scoreFileNameStart
	}
	if index == 0 {
		return scoreFirstChar
	}

	prev, _ := utf8.DecodeRune(line[starts[index-1]:starts[index]])
	cur, _ := utf8.DecodeRune(line[starts[index]:starts[index+1]])

	switch {
	case prev == '/' || prev == '.' || prev == '-' || prev == '_' || prev == ' ':
		return scoreBoundary
	case unicode.IsLower(prev) && unicode.IsUpper(cur):
		return scoreCamel
	default:
		return 0
	}
}

// equalCluster 는 글자 하나가 같은지 본다. fold 면 대소문자를 가리지 않는다.
func equalCluster(want, got []byte, fold bool) bool {
	if fold {
		return bytes.EqualFold(want, got)
	}

	return bytes.Equal(want, got)
}

// hasUpper 는 대문자가 하나라도 있는지다. smartcase 가 이것으로 갈린다.
func hasUpper(pattern []byte) bool {
	for _, r := range string(pattern) {
		if unicode.IsUpper(r) {
			return true
		}
	}

	return false
}
