package core

import (
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// graphCellsPerLane 은 열 하나가 쓰는 칸 수다. 글자 하나와 그 오른쪽 사이 칸이다.
//
// 사이 칸이 있어야 옆으로 뻗는 선을 그릴 수 있다. git 의 `--graph` 도 같은 자를 쓴다.
const graphCellsPerLane = 2

// graphLanes 는 커밋 하나를 그리는 데 필요한 열 수다. 두 행이 같은 폭을 쓴다.
//
// **커밋마다 잰다.** 화면 전체를 한 번에 재면 저 아래 갈래 하나 때문에 위쪽 커밋이 통째로
// 오른쪽으로 밀리고, 훑어 내려갈 때마다 본문이 좌우로 흔들린다. git 의 `--graph` 도
// 줄마다 따로 잰다 — 갈래가 없는 구간은 두 칸이고 merge 자리에서만 네 칸이 된다.
func graphLanes(row graphRow) int {
	return max(len(row.before), len(row.after))
}

// renderGraphMark 는 커밋이 놓인 줄의 그래프 칸이다.
//
//	│ ● │      가운데 열의 커밋. 양옆 갈래는 지나간다
//	●─┘        오른쪽 갈래가 이 커밋으로 모인다
//
// 이 커밋을 기다리던 다른 열은 여기서 끝나므로 모서리(`┘`·`└`) 로 꺾어 점까지 이어 붙인다.
// **사이에 살아 있는 열이 있으면 그 자리는 세로선이 이긴다.** 가로선이 한 칸 끊기지만,
// 지나가는 갈래를 지우면 그 갈래가 사라진 것처럼 보인다.
func renderGraphMark(chars boxSet, row graphRow, lanes int) string {
	cells := graphCells(lanes)

	// 이 커밋으로 모여드는 열들이다. 자기 열은 뺀다.
	for i, hash := range row.before {
		switch {
		case i == row.lane:
			continue
		case hash == row.commit.hash:
			cells[i*graphCellsPerLane] = graphCorner(chars, i, row.lane)
			graphConnect(chars, cells, row.lane, i, row.before)
		case hash != plumbing.ZeroHash:
			cells[i*graphCellsPerLane] = chars.vertical
		}
	}

	cells[row.lane*graphCellsPerLane] = chars.dot

	return strings.Join(cells, "")
}

// renderGraphLink 는 커밋 아래 이어지는 줄의 그래프 칸이다. 두 줄짜리 항목의 둘째 줄이 이것을 쓴다.
//
//	│ │ │      셋이 그대로 내려간다
//	│┐         merge 라 열이 하나 열렸다
//
// 이 커밋이 연 열(merge 의 둘째 부모) 은 모서리(`┐`·`┌`) 로 꺾어 점의 열에서 뻗어 나온다.
func renderGraphLink(chars boxSet, row graphRow, lanes int) string {
	cells := graphCells(lanes)

	for i, hash := range row.after {
		if hash == plumbing.ZeroHash {
			continue
		}

		// 앞줄에 없던 열이면 이 커밋이 방금 연 것이다.
		if i < len(row.before) && row.before[i] != plumbing.ZeroHash {
			cells[i*graphCellsPerLane] = chars.vertical

			continue
		}

		cells[i*graphCellsPerLane] = graphElbow(chars, i, row.lane)
		graphConnect(chars, cells, row.lane, i, row.after)
	}

	return strings.Join(cells, "")
}

// graphCells 는 빈 칸으로 채운 한 줄이다.
func graphCells(lanes int) []string {
	cells := make([]string, max(lanes, 0)*graphCellsPerLane)
	for i := range cells {
		cells[i] = " "
	}

	return cells
}

// graphCorner 는 열 at 이 열 lane 으로 **모여드는** 자리의 모서리다.
// 위에서 내려와 옆으로 꺾으므로 아래쪽 모서리다.
func graphCorner(chars boxSet, at, lane int) string {
	if at > lane {
		return chars.bottomRight
	}

	return chars.bottomLeft
}

// graphElbow 는 열 lane 에서 열 at 으로 **뻗어 나가는** 자리의 모서리다.
// 옆에서 와서 아래로 꺾으므로 위쪽 모서리다.
func graphElbow(chars boxSet, at, lane int) string {
	if at > lane {
		return chars.topRight
	}

	return chars.topLeft
}

// graphConnect 는 두 열 사이를 가로선으로 잇는다.
//
// 사이에 살아 있는 열이 있으면 그 자리는 건드리지 않는다. 열 사이의 빈 칸만 채운다.
func graphConnect(chars boxSet, cells []string, from, to int, lanes []plumbing.Hash) {
	low, high := min(from, to), max(from, to)

	for i := low; i < high; i++ {
		// 열과 열 사이 칸이다. 여기는 늘 가로선이다.
		cells[i*graphCellsPerLane+1] = chars.horizontal

		// 지나가는 열 자리다. 비어 있을 때만 가로선이 통과한다.
		if next := i + 1; next < high {
			if next < len(lanes) && lanes[next] != plumbing.ZeroHash {
				cells[next*graphCellsPerLane] = chars.vertical

				continue
			}

			cells[next*graphCellsPerLane] = chars.horizontal
		}
	}
}
