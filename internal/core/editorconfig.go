package core

import (
	"fmt"
	"path/filepath"
	"strings"

	editorconfig "github.com/editorconfig/editorconfig-core-go/v2"
)

// `.editorconfig` 를 읽는 자리다. 읽는 키는 여섯이고 쓰이는 곳은 둘이다.
//
//	indent_style             ─┐
//	indent_size               ├─ autoindent 의 한 단계 (indent.go, ADR-0048)
//	tab_width                ─┘
//	trim_trailing_whitespace ─┐
//	insert_final_newline      ├─ 저장할 때의 모습 (여기, ADR-0052)
//	end_of_line              ─┘
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

// applyFileFormat 은 저장하기 직전에 파일을 `.editorconfig` 가 적어 둔 모습으로 맞춘다.
// 맞춘 것을 한 줄로 준다. 맞출 것이 없었으면 빈 문자열이다.
//
// **지금까지의 규칙을 뒤집는 자리다.** zn 은 「열었을 때 모습을 지킨다」로 짜여 있었다 —
// 줄끝 종류를 재서 그대로 되돌리고 마지막 줄바꿈도 본 대로 되돌렸다. 여기서부터는 손대지
// 않은 줄도 저장할 때 바뀐다. 적어 둔 사람의 뜻이 파일에 이미 있는 모양보다 앞선다는
// 것이고, 그래서 무엇을 바꿨는지 반드시 알린다(ADR-0052).
//
// 파일을 읽거나 다시 읽는 길에는 걸리지 않는다. 여는 것은 있는 그대로 보여주는 일이고,
// 맞추는 것은 쓰는 일이다.
func (buf *Buffer) applyFileFormat(width int) string {
	// 읽기 전용 파일은 손대지 않는다. 쓰기가 어차피 실패하는데 buffer 만 다듬어지면,
	// 되돌릴 길도 없다 — `u` 도 읽기 전용이라 거절된다(readonly.go, ADR-0051).
	if buf.readOnly {
		return ""
	}

	def := editorconfigFor(buf.path)
	if def == nil {
		return ""
	}

	done := []string{}

	// 적혀 있고 참일 때만 지운다. 거짓이면 「지우지 않는다」이고, 이미 없는 공백을 새로
	// 넣어 줄 일은 없다.
	if def.TrimTrailingWhitespace != nil && *def.TrimTrailingWhitespace {
		if count := buf.trimTrailingSpace(width); count > 0 {
			done = append(done, fmt.Sprintf("줄끝 공백 %d 줄 지움", count))
		}
	}

	if def.InsertFinalNewline != nil && *def.InsertFinalNewline != buf.finalLineEnding {
		buf.finalLineEnding = *def.InsertFinalNewline
		buf.dirty = true

		if buf.finalLineEnding {
			done = append(done, "마지막 줄바꿈 넣음")
		} else {
			done = append(done, "마지막 줄바꿈 뗌")
		}
	}

	if want, ok := lineEndingNamed(def.EndOfLine); ok && want != buf.lineEnding {
		buf.lineEnding = want
		buf.dirty = true

		done = append(done, "줄끝 "+want.name()+" 로 맞춤")
	}

	if len(done) == 0 {
		return ""
	}

	return ".editorconfig: " + strings.Join(done, " · ")
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
