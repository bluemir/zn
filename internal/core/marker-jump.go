package core

// 줄 앞 마커 사이를 뛰는 자리다(ADR-0095).
//
// 마커가 둘이라 뛰는 키도 둘이다. `]c`·`[c` 가 git 으로 바뀐 자리(ADR-0094) 로 가고
// `]d`·`[d` 가 진단(ADR-0086) 으로 간다. 그리는 것은 각자 자기 파일에 있고, 여기 있는 것은
// 「어느 줄로 가는가」와 「가서 무엇을 하는가」다.

// markerDirection 은 어느 쪽으로 뛰는지다.
type markerDirection int

const (
	markerForward markerDirection = iota
	markerBackward
)

// nextMarkerLine 은 정렬된 줄 목록에서 from 의 다음(또는 앞) 줄이다.
//
// **감아 돈다.** 검색(`n`) 이 그렇고, 감았으면 그것을 알리는 것도 그쪽과 같다. vim 의 `]c` 는
// 끝에서 오류를 내는데, 그쪽은 diff mode 안이라 파일을 훑는 일이 아니고 우리 쪽은 「바뀐 것을
// 하나씩 짚어 나가는」 손이라 마지막 다음이 처음인 편이 맞다(ADR-0095).
//
// 목록이 비어 있으면 거짓이다. 하나뿐이고 그 줄에 서 있으면 제자리로 감아 돈다 — 아무 일도
// 안 하는 것보다 「이것 하나뿐이다」가 드러난다.
func nextMarkerLine(lines []int, from int, direction markerDirection) (int, bool, bool) {
	if len(lines) == 0 {
		return 0, false, false
	}

	if direction == markerBackward {
		for i := len(lines) - 1; i >= 0; i-- {
			if lines[i] < from {
				return lines[i], false, true
			}
		}

		return lines[len(lines)-1], true, true
	}

	for _, line := range lines {
		if line > from {
			return line, false, true
		}
	}

	return lines[0], true, true
}

// jumpToMarkerLines 는 그 줄들 사이를 count 번 뛴다.
//
// 검색이 지나는 자리를 그대로 지난다(view-editor-search.go 의 jumpToMatch). 뛰기 전 자리를
// 이력에 담고, 닿은 자리를 방문 기록에 남기고, 감았으면 아래 줄에 알린다(ADR-0070, ADR-0074).
//
// **하나도 없으면 커서를 두고 알리기만 한다.** 마커가 있는지 없는지는 화면 왼쪽 칸을 훑어야
// 아는 것이라, 아무 일도 안 나면 키가 안 먹은 것으로 읽힌다.
func (e *editor) jumpToMarkerLines(lines []int, direction markerDirection, count int, empty string) {
	buf := e.activeBuffer()

	line, wrapped := buf.Cursor.Line, false

	for range count {
		next, turned, ok := nextMarkerLine(lines, line, direction)
		if !ok {
			e.notify(empty)

			return
		}

		line, wrapped = next, wrapped || turned
	}

	// **닿을 자리를 확인한 뒤에 담는다.** 못 가면 커서가 그대로라 담을 것도 없다(jumpToMatch).
	e.recordJump()

	// 줄의 첫 글자로 간다. 마커가 가리키는 것이 줄이라 칸에는 뜻이 없고, `gg`·`G` 가 줄 앞으로
	// 가는 것과 같은 손이다.
	buf.MoveTo(line, 0)
	buf.ClampToNormal()
	e.scrollToCursor()

	e.arrive()

	if wrapped {
		e.notify(markerWrapMessage(direction))
	}
}

// markerWrapMessage 는 파일 끝을 지나 감쌌음을 알리는 말이다. 검색의 것과 같은 꼴이다.
func markerWrapMessage(direction markerDirection) string {
	if direction == markerBackward {
		return "위에서 끝으로 돌아옴"
	}

	return "아래에서 처음으로 돌아옴"
}
