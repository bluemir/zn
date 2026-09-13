package core

import (
	"slices"

	"github.com/go-git/go-git/v5/plumbing"
)

// git 의 `--graph` 알고리즘을 옮긴 자리다(ADR-0141).
//
// **열이 움직인다는 것이 요점이다.** merge 는 새 열을 커밋 **바로 옆**에 끼우고 나머지를
// 오른쪽으로 민다. 그래서 여는 선이 언제나 한 칸이다. 그다음 몇 행에 걸쳐 열을 왼쪽으로
// 당겨 붙이는데, 그 당기는 과정이 `| |/`·`|/|` 같은 행으로 보인다. 대각선이 늘 바로 옆 열을
// 가리키는 것이 이 둘 덕이다.
//
// 열을 고정해 두고 그리면 열이 둘 이상 떨어진 자리에서 대각선이 엉뚱한 열을 가리킨다. 그
// 자리를 모서리로 메우면 git 과 그림이 갈린다. 둘 다 겪고 나서 알고리즘째 가져왔다.
//
// **글자는 여기서 정하지 않는다.** 칸마다 「무엇인지」만 내고 어느 글자로 그릴지는 그리는
// 쪽이 고른다. 터미널에 따라 갈리는 판단이라 훑기가 들 것이 아니다(ADR-0028, ADR-0115 §5).

// graphSymbol 은 그래프 칸 하나가 뜻하는 것이다.
type graphSymbol byte

const (
	graphBlank      graphSymbol = iota // 빈 칸
	graphNode                          // 커밋이 선 자리. `*`
	graphVertical                      // 이어져 내려가는 열. `|`
	graphSlashUp                       // 오른쪽 위에서 왼쪽 아래로. `/`
	graphSlashDown                     // 왼쪽 위에서 오른쪽 아래로. `\`
	graphUnderscore                    // 열을 건너 옆으로 가는 선. `_`
	graphDash                          // octopus 가 옆으로 펴는 선. `-`
	graphDot                           // octopus 의 끝. `.`
)

// graphState 는 지금 어느 갈래의 행을 낼 차례인지다. git 의 상태 이름을 그대로 쓴다.
type graphState byte

const (
	graphStatePadding    graphState = iota // 낼 것이 없다. 커밋 하나가 끝난 자리다
	graphStatePreCommit                    // octopus 가 설 자리를 넓히는 행
	graphStateCommit                       // 커밋이 선 행
	graphStatePostMerge                    // merge 가 연 열들이 뻗는 행
	graphStateCollapsing                   // 열을 왼쪽으로 당겨 붙이는 행
)

// graphDrawer 는 커밋을 하나씩 먹으며 그래프 행을 내는 상태 기계다.
//
// **훑기가 들고 다닌다.** 앞 커밋에서 이어지는 상태(열 배치·당기는 중인지) 위에서만 다음
// 행이 나온다. 화면이 나중에 다시 그릴 수 있는 종류의 값이 아니다.
type graphDrawer struct {
	// columns 는 이 커밋을 먹기 **전**의 열이고 newColumns 는 먹은 **뒤**의 열이다.
	// 열마다 그 열이 기다리는 커밋이 담긴다.
	columns    []plumbing.Hash
	newColumns []plumbing.Hash

	// mapping 은 화면 칸마다 그 칸이 어느 새 열로 가야 하는지다. 길이가 칸 수다.
	//
	// 커밋 행 직후에는 열이 제자리에 있지 않다(`mapping[i] != i/2`). 당기는 행이 한 번에
	// 한 칸씩 왼쪽으로 옮겨서 제자리에 맞춘다.
	mapping     []int
	oldMapping  []int
	mappingSize int

	commit      plumbing.Hash
	parents     []plumbing.Hash
	commitIndex int

	state           graphState
	prevState       graphState
	prevCommitIndex int

	// edgesAdded 는 이 커밋이 연 열 수다. 앞 커밋의 것과 함께 커밋 행의 글자를 정한다.
	edgesAdded     int
	prevEdgesAdded int

	// mergeLayout 은 merge 의 첫 부모가 커밋의 어느 쪽에 놓였는지다. 0 이 왼쪽, 1 이 제자리,
	// 2 가 오른쪽이고 `/`·`|`·`\` 로 그려진다.
	mergeLayout int

	expansionRow int
}

// newGraphDrawer 는 아무 커밋도 먹지 않은 상태다.
func newGraphDrawer() *graphDrawer {
	return &graphDrawer{state: graphStatePadding, mergeLayout: -1}
}

// draw 는 커밋 하나가 차지하는 그래프 행들을 낸다. 몇 번째가 커밋이 선 행인지와, 모자랄 때
// 덧붙일 행도 같이 준다.
//
// **git 이 내는 행을 그대로 낸다.** 커밋 하나가 한 행일 수도 있다. 화면이 두 줄 형식이라
// 모자란 자리를 채우는 것은 그리는 쪽의 일이고, 여기서 채워 넣으면 그 행이 git 과 갈린다.
// 대신 채울 행을 같이 줘서, 그리는 쪽이 열을 다시 세지 않아도 되게 한다.
func (d *graphDrawer) draw(commit plumbing.Hash, parents []plumbing.Hash) ([][]graphSymbol, int, []graphSymbol) {
	d.update(commit, parents)

	var (
		rows  [][]graphSymbol
		atRow = -1
		guard = 0
	)

	for d.state != graphStatePadding {
		if d.state == graphStateCommit {
			atRow = len(rows)
		}

		rows = append(rows, d.nextLine())

		// 상태 기계가 멎지 않는 자리를 만들지 않는다. 열 수보다 많은 행이 나올 수 없다.
		if guard++; guard > len(d.mapping)+len(d.columns)+8 {
			break
		}
	}

	return rows, max(atRow, 0), d.paddingLine()
}

// update 는 커밋 하나를 먹고 열을 다시 배치한다.
func (d *graphDrawer) update(commit plumbing.Hash, parents []plumbing.Hash) {
	d.commit, d.parents = commit, parents
	d.prevCommitIndex = d.commitIndex

	d.updateColumns()

	d.expansionRow = 0

	// 부모가 셋 이상이고 오른쪽에 열이 남아 있으면, 설 자리를 넓히는 행이 먼저 온다.
	if len(d.parents) >= 3 && d.commitIndex < len(d.columns)-1 {
		d.state = graphStatePreCommit
	} else {
		d.state = graphStateCommit
	}
}

// updateColumns 는 열 배치를 다시 짜고 mapping 을 채운다.
//
// **왼쪽부터 차례로 끼운다.** 그래서 열이 가야 할 자리는 늘 지금 자리이거나 그 왼쪽이다.
// 오른쪽으로 옮길 일이 없다는 것이 당기는 행을 단순하게 만든다. 갈래가 엇갈릴 때 한쪽만
// 움직이므로 읽기도 쉽다.
func (d *graphDrawer) updateColumns() {
	d.columns, d.newColumns = d.newColumns, d.columns[:0]

	maxNew := len(d.columns) + len(d.parents)

	d.mappingSize = 2 * maxNew
	d.mapping = graphResize(d.mapping, d.mappingSize)
	d.oldMapping = graphResize(d.oldMapping, d.mappingSize)

	for i := range d.mapping {
		d.mapping[i] = -1
	}

	d.prevEdgesAdded = d.edgesAdded
	d.edgesAdded = 0
	d.mergeLayout = -1

	seen, at := false, 0

	for i := 0; i <= len(d.columns); i++ {
		var column plumbing.Hash

		if i == len(d.columns) {
			// 이 커밋을 기다리는 열이 하나도 없었다. 자식을 아직 안 본 갈래의 끝이다.
			if seen {
				break
			}

			column = d.commit
		} else {
			column = d.columns[i]
		}

		if column != d.commit {
			d.insertColumn(column, &at, -1, -1)

			continue
		}

		before := at
		seen = true
		d.commitIndex = i

		for j, parent := range d.parents {
			d.insertColumn(parent, &at, i, j)
		}

		// 부모가 없어도 커밋은 두 칸을 쓴다.
		if at == before {
			at += 2
		}
	}

	for d.mappingSize > 1 && d.mapping[d.mappingSize-1] < 0 {
		d.mappingSize--
	}
}

// insertColumn 은 그 커밋을 기다리는 새 열을 찾고, 없으면 오른쪽 끝에 하나 연다.
//
// commitIndex 는 지금 먹는 커밋의 열이고, 그 커밋의 부모를 끼울 때만 0 이상이다.
// parentIndex 는 그 커밋의 몇 번째 부모인지다.
//
// merge 의 **첫 부모**가 어디 놓이는지가 그 아래 행의 글자를 정한다.
func (d *graphDrawer) insertColumn(hash plumbing.Hash, at *int, commitIndex, parentIndex int) {
	column := slices.Index(d.newColumns, hash)
	if column < 0 {
		column = len(d.newColumns)
		d.newColumns = append(d.newColumns, hash)

		// **첫 부모는 세지 않는다.** 그 열은 커밋이 물려받는 것이라 옆 열을 밀어내지 않는다.
		// 옆으로 밀리는 것은 둘째 부모부터 여는 열이고, 그 수가 커밋 행의 글자를 정한다.
		if parentIndex > 0 {
			d.edgesAdded++
		}
	}

	if commitIndex >= 0 && len(d.parents) > 1 && d.mergeLayout == -1 {
		switch distance := column - commitIndex; {
		case distance < 0:
			d.mergeLayout = 0
		case distance == 0:
			d.mergeLayout = 1
		default:
			d.mergeLayout = 2
		}
	}

	d.mapping[*at] = column
	*at += 2
}

// nextLine 은 지금 상태의 행 하나를 내고 다음 상태로 넘어간다.
func (d *graphDrawer) nextLine() []graphSymbol {
	state := d.state

	var line []graphSymbol

	switch state {
	case graphStatePreCommit:
		line = d.preCommitLine()
	case graphStateCommit:
		line = d.commitLine()
	case graphStatePostMerge:
		line = d.postMergeLine()
	case graphStateCollapsing:
		line = d.collapsingLine()
	default:
		line = d.paddingLine()
	}

	d.prevState = state

	return line
}

// paddingLine 은 열을 그대로 내려 긋기만 하는 행이다. 그리는 쪽이 모자란 자리를 채울 때 쓴다.
func (d *graphDrawer) paddingLine() []graphSymbol {
	line := graphLine(len(d.newColumns) * 2)
	for i := range d.newColumns {
		line[i*2] = graphVertical
	}

	return line
}

// preCommitLine 은 octopus 가 설 자리를 넓히는 행이다. 부모가 셋 이상일 때만 선다.
func (d *graphDrawer) preCommitLine() []graphSymbol {
	var line []graphSymbol

	seen := false
	for i, column := range d.columns {
		switch {
		case column == d.commit:
			seen = true
			line = append(line, graphVertical)

			for range d.expansionRow {
				line = append(line, graphBlank)
			}
		case seen && d.expansionRow == 0:
			// 앞 커밋이 merge 로 끝났으면 그 오른쪽 열들은 `\` 로 이어 그린다.
			if d.prevState == graphStatePostMerge && d.prevCommitIndex < i {
				line = append(line, graphSlashDown)
			} else {
				line = append(line, graphVertical)
			}
		case seen:
			line = append(line, graphSlashDown)
		default:
			line = append(line, graphVertical)
		}

		line = append(line, graphBlank)
	}

	d.expansionRow++
	if d.expansionRow >= (len(d.parents)-1)*2 {
		d.state = graphStateCommit
	}

	return line
}

// commitLine 은 커밋이 선 행이다.
//
// **칸을 이어 붙이며 그린다.** 절대 자리로 쓰면 octopus 가 옆으로 펴는 `-`·`.` 가 뒤따르는
// 열과 자리를 다툰다. git 도 이어 붙이는 모양이라 그 자리 셈이 아예 없다.
func (d *graphDrawer) commitLine() []graphSymbol {
	var line []graphSymbol

	seen := false
	for i := 0; i <= len(d.columns); i++ {
		var column plumbing.Hash

		if i == len(d.columns) {
			if seen {
				break
			}

			column = d.commit
		} else {
			column = d.columns[i]
		}

		switch {
		case column == d.commit:
			seen = true
			line = append(line, graphNode)

			if len(d.parents) > 2 {
				line = d.appendOctopus(line)
			}
		case seen && d.edgesAdded > 1:
			line = append(line, graphSlashDown)
		case seen && d.edgesAdded == 1:
			// 앞 커밋이 연 열이 이 자리까지 밀려 온 것이면 그 `\` 를 이어 그린다.
			if d.prevState == graphStatePostMerge && d.prevEdgesAdded > 0 && d.prevCommitIndex < i {
				line = append(line, graphSlashDown)
			} else {
				line = append(line, graphVertical)
			}
		default:
			line = append(line, graphVertical)
		}

		line = append(line, graphBlank)
	}

	switch {
	case len(d.parents) > 1:
		d.state = graphStatePostMerge
	case d.mappingDone():
		d.state = graphStatePadding
	default:
		d.state = graphStateCollapsing
	}

	return line
}

// appendOctopus 는 부모가 셋 이상인 커밋이 옆으로 펴는 `-`·`.` 다.
//
// 앞의 둘은 그냥 옆 열에 놓이므로 선을 긋지 않는다. 셋째부터가 이 선을 탄다.
func (d *graphDrawer) appendOctopus(line []graphSymbol) []graphSymbol {
	for range (len(d.parents)-2)*2 - 1 {
		line = append(line, graphDash)
	}

	return append(line, graphDot)
}

// postMergeLine 은 merge 가 연 열들이 뻗어 나가는 행이다.
//
// 커밋의 칸에 첫 부모의 글자가 서고, 나머지 부모마다 `\` 가 하나씩 오른쪽으로 이어진다.
// **커밋 오른쪽의 열들도 `\` 다** — merge 가 새 열을 끼우면서 그만큼 오른쪽으로 밀린다.
func (d *graphDrawer) postMergeLine() []graphSymbol {
	var line []graphSymbol

	seen := false
	for i := 0; i <= len(d.columns); i++ {
		var column plumbing.Hash

		if i == len(d.columns) {
			if seen {
				break
			}

			column = d.commit
		} else {
			column = d.columns[i]
		}

		if column != d.commit {
			if seen {
				line = append(line, graphSlashDown, graphBlank)
			} else {
				line = append(line, graphVertical, graphBlank)
			}

			continue
		}

		seen = true

		// 첫 부모 뒤에는 사이 칸을 두지 않는다. `|\` 처럼 둘이 붙어야 한 자리에서 갈라지는
		// 것으로 읽힌다.
		line = append(line, graphMergeChar(d.mergeLayout))
		for range len(d.parents) - 1 {
			line = append(line, graphSlashDown, graphBlank)
		}
	}

	if d.mappingDone() {
		d.state = graphStatePadding
	} else {
		d.state = graphStateCollapsing
	}

	return line
}

// graphMergeChar 는 merge 의 첫 부모가 놓인 쪽에 따른 글자다.
func graphMergeChar(layout int) graphSymbol {
	switch layout {
	case 0:
		return graphSlashUp
	case 1:
		return graphVertical
	default:
		return graphSlashDown
	}
}

// collapsingLine 은 열을 왼쪽으로 **한 칸** 당기는 행이다. 제자리에 다 앉을 때까지 되풀이한다.
//
// 옮길 자리가 비어 있으면 한 칸 왼쪽으로 가고, 이미 다른 선이 있으면 그 선을 건너뛴다.
// 건너뛰는 선은 한 행에 하나뿐이라, 여러 선이 한꺼번에 엇갈리지 않는다.
func (d *graphDrawer) collapsingLine() []graphSymbol {
	d.mapping, d.oldMapping = d.oldMapping, d.mapping

	for i := range d.mapping {
		d.mapping[i] = -1
	}

	horizontalEdge, horizontalTarget := -1, -1

	for i := 0; i < d.mappingSize; i++ {
		target := d.oldMapping[i]
		if target < 0 {
			continue
		}

		switch {
		case target*2 == i:
			// 이미 제자리다.
			d.mapping[i] = target
		case d.mapping[i-1] < 0:
			// 왼쪽이 비었다. 한 칸 간다.
			d.mapping[i-1] = target

			// 옆으로 길게 가는 선은 한 행에 하나만 고른다.
			if horizontalEdge == -1 {
				horizontalEdge, horizontalTarget = i, target

				for j := target*2 + 3; j < i-2; j += 2 {
					d.mapping[j] = target
				}
			}
		case d.mapping[i-1] == target:
			// 왼쪽에 같은 곳으로 가는 선이 이미 있다. 거기 합친다.
		default:
			// 왼쪽에 다른 선이 있다. 건너뛴다.
			d.mapping[i-2] = target

			if horizontalEdge == -1 {
				horizontalEdge = i
			}
		}
	}

	copy(d.oldMapping, d.mapping)

	if d.mapping[d.mappingSize-1] < 0 {
		d.mappingSize--
	}

	line := graphLine(d.mappingSize)
	usedHorizontal := false

	for i := 0; i < d.mappingSize; i++ {
		target := d.mapping[i]

		switch {
		case target < 0:
			line[i] = graphBlank
		case target*2 == i:
			line[i] = graphVertical
		case target == horizontalTarget && i != horizontalEdge-1:
			// 첫 조각만 남기고 나머지는 더 당기지 않는다.
			if i != target*2+3 {
				d.mapping[i] = -1
			}

			usedHorizontal = true
			line[i] = graphUnderscore
		default:
			if usedHorizontal && i < horizontalEdge {
				d.mapping[i] = -1
			}

			line[i] = graphSlashUp
		}
	}

	if d.mappingDone() {
		d.state = graphStatePadding
	}

	return line
}

// mappingDone 은 열이 모두 제자리에 앉았는지다. 앉았으면 더 당길 행이 없다.
func (d *graphDrawer) mappingDone() bool {
	for i := 0; i < d.mappingSize; i++ {
		if target := d.mapping[i]; target >= 0 && target != i/2 {
			return false
		}
	}

	return true
}

// graphLine 은 빈 칸으로 채운 행이다.
func graphLine(size int) []graphSymbol {
	return make([]graphSymbol, max(size, 0))
}

// graphResize 는 그 길이가 되도록 늘린다. 줄이지는 않는다.
func graphResize(values []int, size int) []int {
	for len(values) < size {
		values = append(values, -1)
	}

	return values[:max(size, len(values))]
}
