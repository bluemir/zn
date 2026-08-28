package core

import (
	"path/filepath"
	"strings"

	"github.com/editorconfig/editorconfig-core-go/v2"
)

// `.editorconfig` 를 읽는 자리다. 읽는 키는 여섯이고 쓰이는 곳은 셋이다.
//
//	indent_style             ─┐
//	indent_size               ├─ autoindent 의 한 단계 (indent.go, ADR-0048)
//	tab_width                ─┤
//	                          └─ tab 하나를 그리는 화면 폭 (indent.go, ADR-0096)
//	trim_trailing_whitespace ─┐
//	insert_final_newline      ├─ 저장할 때의 모습 (여기, ADR-0052)
//	end_of_line              ─┘
//
// `tab_width` 가 둘에 걸쳐 있는 것이 요점이다. 한 단계를 재는 자리와 그리는 자리가 같은 값을
// 써야 눈에 보이는 것과 움직이는 것이 어긋나지 않는다(indent.go 의 blankColumns).
//
// 읽지 않는 키는 `charset` 과 `max_line_length` 다. 앞엣것은 읽기·쓰기 인코딩이라 변환 겹이
// 새로 생기고(zn 은 바이트를 UTF-8 로 간주한다) 뒤엣것은 저장 동작이 아니라 화면에 기준선을
// 긋는 새 기능이다. 둘 다 그 기능을 만들 때 같이 본다(docs/tasks.md).

// editorconfigFor 는 그 파일에 적용되는 설정이다. 없거나 읽지 못하면 nil 이다.
//
// 오류는 삼킨다. 파일을 못 읽거나 문법이 깨졌으면 「적힌 것이 없다」로 보는 것이 맞다 —
// 저장하는 자리에서 설정 파일 이야기를 꺼내면 정작 저장이 됐는지가 가려진다.
//
// 경로는 절대 경로여야 한다. 라이브러리가 위로 훑어 올라가며 `.editorconfig` 를 찾는데
// 상대 경로로는 어디서 시작할지 정하지 못한다.
func editorconfigFor(path string) *editorconfig.Definition {
	if path == "" {
		return nil
	}

	full, err := filepath.Abs(path)
	if err != nil {
		return nil
	}

	def, err := editorconfig.GetDefinitionForFilename(full)
	if err != nil {
		return nil
	}

	return def
}

// lineEndingNamed 는 `end_of_line` 에 적힌 이름이 가리키는 줄끝이다. 모르는 이름이면 false 다.
//
// **`cr` 은 받지 않는다.** zn 의 줄끝은 LF 와 CRLF 둘뿐이고, 여는 자리(detectLineEnding) 와
// 줄을 가르는 자리도 그 둘로 짜여 있다. 셋째를 넣으려면 그 길이 전부 늘어나는데, 그렇게 해서
// 얻는 것은 classic Mac 파일이다. 적혀 있어도 그 파일의 줄끝을 그대로 둔다 — 못 맞추는 것을
// 조용히 LF 로 바꿔 쓰는 것보다 낫다(docs/tasks.md).
func lineEndingNamed(name string) (lineEnding, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "lf":
		return lineEndingLF, true
	case "crlf":
		return lineEndingCRLF, true
	}

	return 0, false
}
