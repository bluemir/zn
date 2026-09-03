package textarea

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
)

// 디스크에 쓰는 길이다. `:w`·`:w!`·`:w <파일>`·`:w! <파일>` 넷이 문이고, 그 아래에 쓰기 직전
// buffer 를 맞추는 둘(포매터·`.editorconfig`) 이 붙는다.
//
// 맞추는 것이 쓰기보다 먼저다 — buffer 를 고치고 그것을 써야 화면과 파일이 같아진다
// (ADR-0015, ADR-0024, ADR-0052).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-save.go 에 있다 (ADR-0121).

// Save 는 buffer 를 파일에 쓴다.
// 읽은 뒤에 파일이 밖에서 바뀌었으면 쓰지 않고 알린다 (ADR-0015).
//
// format 은 쓰기 직전에 통과시킬 포매터다. 없으면 nil 이다. 부르는 쪽이 찾아서
// 넘기는 것은 「무엇이 깔려 있는가」가 편집기가 도는 동안의 상태라서다 — buffer 는 그것을
// 들 자리가 아니다.
func (viewport *Viewport) Save(format *SaveFormat) (string, error) {
	// :tabnew 로 만든 buffer 는 이름이 없어서 쓸 곳이 없다. vim 의 E32 와 같다.
	// 이름을 주려면 `:w <파일>`, 즉 SaveTo 다 (ADR-0024).
	if viewport.Path == "" {
		return "", errors.New("파일 이름이 없습니다. `:w <파일>` 로 이름을 주십시오")
	}

	// 바깥 검사가 맞추기보다 먼저다. 막힐 저장이면 buffer 를 건드리지 않아야 한다 —
	// 「저장하지 못했는데 파일이 달라졌다」가 되면 무엇을 잃었는지 셀 수 없다.
	if err := viewport.checkNotChangedOutside(); err != nil {
		return "", err
	}

	return viewport.formatAndWrite(format)
}

// SaveForce 는 밖에서 바뀌었는지 보지 않고 덮어쓴다. `:w!` 다.
//
// 맞추는 것은 건너뛰지 않는다. `!` 는 「바깥 변경을 무릅쓰고 덮어쓴다」 하나만 뜻한다 —
// 한 키에 뜻을 둘 담으면 어느 쪽을 부른 것인지 갈리지 않는다(ADR-0015, ADR-0052).
func (viewport *Viewport) SaveForce(format *SaveFormat) (string, error) {
	if viewport.Path == "" {
		return "", errors.New("파일 이름이 없습니다. `:w <파일>` 로 이름을 주십시오")
	}

	return viewport.formatAndWrite(format)
}

// formatAndWrite 는 `.editorconfig` 가 적어 둔 모습으로 맞춘 뒤 쓴다.
// 맞춘 것을 한 줄로 준다 — 부르는 쪽이 저장 문구에 붙인다(editorconfig.go, ADR-0052).
//
// 맞추는 것이 쓰기보다 먼저다. buffer 를 고치고 그것을 쓰는 순서라야 화면과 파일이 같아진다.
// 나가는 바이트만 고치면 화면에는 지운 공백이 그대로 남고, 그 상태로 dirty 가 내려가서
// 다음 자동 다시읽기(ADR-0038) 에 조용히 사라진다.
func (viewport *Viewport) formatAndWrite(format *SaveFormat) (string, error) {
	// 포매터가 먼저고 `.editorconfig` 가 뒤다. 적어 둔 사람의 뜻이 마지막에 서야 한다 —
	// gofmt 계열은 줄끝을 LF 로, 마지막 줄바꿈을 있는 것으로 내는데, 그 파일에 `end_of_line`
	// 이나 `insert_final_newline` 이 적혀 있으면 그쪽이 이긴다(ADR-0052, ADR-0065).
	notes := []string{}
	if note := viewport.applySaveHook(format); note != "" {
		notes = append(notes, note)
	}
	if note := viewport.applyFileFormat(); note != "" {
		notes = append(notes, note)
	}

	if err := viewport.Write(); err != nil {
		return "", err
	}

	return strings.Join(notes, "  "), nil
}

// applySaveHook 은 저장하기 직전에 buffer 를 포매터가 낸 글로 갈아끼운다.
// 한 줄로 무엇을 했는지 준다 — 부르는 쪽이 저장 문구에 붙인다(ADR-0052).
//
// **실패하면 buffer 를 건드리지 않고 까닭만 준다. 저장은 그대로 간다.** 편집 중의 Go 는 거의
// 언제나 문법이 깨져 있어서, 여기서 저장을 막으면 고치다 만 파일을 둘 곳이 없어진다.
//
// 읽기 전용 파일은 손대지 않는다. 쓰기가 어차피 실패하는데 buffer 만 바뀌면 되돌릴 길도
// 없다 — applyFileFormat 과 같은 자리다(editorconfig.go).
func (viewport *Viewport) applySaveHook(format *SaveFormat) string {
	if format == nil || viewport.ReadOnly {
		return ""
	}

	// 마지막 줄바꿈을 붙여 넘긴다. buffer 는 그것을 내용이 아니라 사실로 들고 있어서
	// (finalLineEnding) 붙이지 않으면 포매터가 마지막 줄만 다르게 본다.
	in := append(bytes.Join(viewport.lines, []byte{'\n'}), '\n')

	out, err := format.Run(in)
	if err != nil {
		return format.Name + ": " + err.Error()
	}

	next := SplitFormatted(out)
	if equalLines(viewport.lines, next) {
		return ""
	}

	changed := CountChangedLines(viewport.lines, next)
	grew := len(next) - len(viewport.lines)

	viewport.ReplaceAll(next)

	note := fmt.Sprintf("%s: %d 줄 맞춤", format.Name, changed)
	switch {
	case grew > 0:
		note += fmt.Sprintf(", %d 줄 늘어남", grew)
	case grew < 0:
		note += fmt.Sprintf(", %d 줄 줄어듦", -grew)
	}

	return note
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
func (viewport *Viewport) applyFileFormat() string {
	// 읽기 전용 파일은 손대지 않는다. 쓰기가 어차피 실패하는데 buffer 만 다듬어지면,
	// 되돌릴 길도 없다 — `u` 도 읽기 전용이라 거절된다(readonly.go, ADR-0051).
	if viewport.ReadOnly {
		return ""
	}

	def := editorconfigFor(viewport.Path)
	if def == nil {
		return ""
	}

	done := []string{}

	// 적혀 있고 참일 때만 지운다. 거짓이면 「지우지 않는다」이고, 이미 없는 공백을 새로
	// 넣어 줄 일은 없다.
	if def.TrimTrailingWhitespace != nil && *def.TrimTrailingWhitespace {
		// 저장은 언제나 파일 전체다. 고른 범위를 보는 것은 팔레트 쪽이다(ADR-0111).
		if count := viewport.TrimTrailingSpace(0, len(viewport.lines)); count > 0 {
			done = append(done, fmt.Sprintf("줄끝 공백 %d 줄 지움", count))
		}
	}

	if def.InsertFinalNewline != nil && *def.InsertFinalNewline != viewport.finalLineEnding {
		viewport.finalLineEnding = *def.InsertFinalNewline
		viewport.Dirty = true

		if viewport.finalLineEnding {
			done = append(done, "마지막 줄바꿈 넣음")
		} else {
			done = append(done, "마지막 줄바꿈 뗌")
		}
	}

	if want, ok := lineEndingNamed(def.EndOfLine); ok && want != viewport.lineEnding {
		viewport.lineEnding = want
		viewport.Dirty = true

		done = append(done, "줄끝 "+want.name()+" 로 맞춤")
	}

	if len(done) == 0 {
		return ""
	}

	return ".editorconfig: " + strings.Join(done, " · ")
}
