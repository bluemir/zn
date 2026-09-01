package core

// boxSet 은 테두리와 구분선에 쓰는 글자다. sidebar 구분선·tabline·팔레트 테두리가 같이 쓴다.
//
// 유니코드 박스 그리기 문자(U+2500 블록) 는 East Asian Width 가 전부 Ambiguous 라
// 터미널 설정에 따라 한 칸이기도 두 칸이기도 하다. 우리 폭 계산은 한 칸으로 보므로
// 두 칸으로 그리는 터미널에서는 sidebar 아래 모든 행이 밀린다(ADR-0028).
type boxSet struct {
	vertical    string
	horizontal  string
	topLeft     string
	topRight    string
	bottomLeft  string
	bottomRight string
	leftTee     string
	rightTee    string

	// dot 은 커밋 기록의 커밋 하나를 가리키는 점이다(render-graph.go).
	//
	// 박스 그리기 문자는 아니지만 같은 자리에 둔다. `●` 도 East Asian Width 가 Ambiguous 라
	// 두 칸으로 그리는 터미널에서는 그래프의 열이 통째로 밀리는데, 그 열 맞춤이 이 화면의
	// 전부다. 선과 점이 같은 판단을 타야 한다.
	dot string
}

// boxUnicode 는 폭이 1 칸으로 확인된 터미널에서 쓴다.
var boxUnicode = boxSet{
	vertical:    "│",
	horizontal:  "─",
	topLeft:     "┌",
	topRight:    "┐",
	bottomLeft:  "└",
	bottomRight: "┘",
	leftTee:     "├",
	rightTee:    "┤",
	dot:         "●",
}

// boxASCII 는 물러설 자리다. `| - +` 는 전부 East Asian Width 가 Narrow 라
// 어느 터미널에서도 한 칸이고, 폰트에 글리프가 없어 두부가 될 일도 없다.
//
// 모서리는 Neutral 인 대체 글자가 아예 없어서 넷 다 `+` 다.
var boxASCII = boxSet{
	vertical:    "|",
	horizontal:  "-",
	topLeft:     "+",
	topRight:    "+",
	bottomLeft:  "+",
	bottomRight: "+",
	leftTee:     "+",
	rightTee:    "+",
	dot:         "*",
}
