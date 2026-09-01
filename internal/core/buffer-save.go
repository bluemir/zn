package core

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/syntax"
)

// 디스크에 쓰는 길이다. `:w`·`:w!`·`:w <파일>`·`:w! <파일>` 넷이 문이고, 그 아래에 쓰기 직전
// buffer 를 맞추는 둘(포매터·`.editorconfig`) 이 붙는다.
//
// 맞추는 것이 쓰기보다 먼저다 — buffer 를 고치고 그것을 써야 화면과 파일이 같아진다
// (ADR-0015, ADR-0024, ADR-0052).

// Save 는 buffer 를 파일에 쓴다.
// 읽은 뒤에 파일이 밖에서 바뀌었으면 쓰지 않고 알린다 (ADR-0015).
//
// hook 은 쓰기 직전에 통과시킬 포매터다. 없으면 nil 이다(save-hook.go). 부르는 쪽이 찾아서
// 넘기는 것은 「무엇이 깔려 있는가」가 편집기가 도는 동안의 상태라서다 — buffer 는 그것을
// 들 자리가 아니다.
func (buf *viewport) Save(width int, hook *saveHook) (string, error) {
	// :tabnew 로 만든 buffer 는 이름이 없어서 쓸 곳이 없다. vim 의 E32 와 같다.
	// 이름을 주려면 `:w <파일>`, 즉 SaveTo 다 (ADR-0024).
	if buf.path == "" {
		return "", errors.New("파일 이름이 없습니다. `:w <파일>` 로 이름을 주십시오")
	}

	// 바깥 검사가 맞추기보다 먼저다. 막힐 저장이면 buffer 를 건드리지 않아야 한다 —
	// 「저장하지 못했는데 파일이 달라졌다」가 되면 무엇을 잃었는지 셀 수 없다.
	if err := buf.checkNotChangedOutside(); err != nil {
		return "", err
	}

	return buf.formatAndWrite(width, hook)
}

// SaveForce 는 밖에서 바뀌었는지 보지 않고 덮어쓴다. `:w!` 다.
//
// 맞추는 것은 건너뛰지 않는다. `!` 는 「바깥 변경을 무릅쓰고 덮어쓴다」 하나만 뜻한다 —
// 한 키에 뜻을 둘 담으면 어느 쪽을 부른 것인지 갈리지 않는다(ADR-0015, ADR-0052).
func (buf *viewport) SaveForce(width int, hook *saveHook) (string, error) {
	if buf.path == "" {
		return "", errors.New("파일 이름이 없습니다. `:w <파일>` 로 이름을 주십시오")
	}

	return buf.formatAndWrite(width, hook)
}

// formatAndWrite 는 `.editorconfig` 가 적어 둔 모습으로 맞춘 뒤 쓴다.
// 맞춘 것을 한 줄로 준다 — 부르는 쪽이 저장 문구에 붙인다(editorconfig.go, ADR-0052).
//
// 맞추는 것이 쓰기보다 먼저다. buffer 를 고치고 그것을 쓰는 순서라야 화면과 파일이 같아진다.
// 나가는 바이트만 고치면 화면에는 지운 공백이 그대로 남고, 그 상태로 dirty 가 내려가서
// 다음 자동 다시읽기(ADR-0038) 에 조용히 사라진다.
func (buf *viewport) formatAndWrite(width int, hook *saveHook) (string, error) {
	// 포매터가 먼저고 `.editorconfig` 가 뒤다. 적어 둔 사람의 뜻이 마지막에 서야 한다 —
	// gofmt 계열은 줄끝을 LF 로, 마지막 줄바꿈을 있는 것으로 내는데, 그 파일에 `end_of_line`
	// 이나 `insert_final_newline` 이 적혀 있으면 그쪽이 이긴다(ADR-0052, ADR-0065).
	notes := []string{}
	if note := buf.applySaveHook(hook, width); note != "" {
		notes = append(notes, note)
	}
	if note := buf.applyFileFormat(width); note != "" {
		notes = append(notes, note)
	}

	if err := buf.write(); err != nil {
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
func (buf *viewport) applySaveHook(hook *saveHook, width int) string {
	if hook == nil || buf.readOnly {
		return ""
	}

	// 마지막 줄바꿈을 붙여 넘긴다. buffer 는 그것을 내용이 아니라 사실로 들고 있어서
	// (finalLineEnding) 붙이지 않으면 포매터가 마지막 줄만 다르게 본다.
	in := append(bytes.Join(buf.lines, []byte{'\n'}), '\n')

	out, err := hook.run(in)
	if err != nil {
		return hook.name + ": " + err.Error()
	}

	next := splitFormatted(out)
	if equalLines(buf.lines, next) {
		return ""
	}

	changed := countChangedLines(buf.lines, next)
	grew := len(next) - len(buf.lines)

	buf.replaceAll(next, width)

	note := fmt.Sprintf("%s: %d 줄 맞춤", hook.name, changed)
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
func (buf *viewport) applyFileFormat(width int) string {
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
		// 저장은 언제나 파일 전체다. 고른 범위를 보는 것은 팔레트 쪽이다(ADR-0111).
		if count := buf.trimTrailingSpace(0, len(buf.lines), width); count > 0 {
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

// SaveTo 는 buffer 를 다른 파일에 쓴다. `:w <파일>` 이다.
// 그 자리에 이미 파일이 있으면 쓰지 않고 알린다 (ADR-0024).
func (buf *Buffer) SaveTo(path string) error {
	if err := checkNotExist(path); err != nil {
		return err
	}

	return buf.saveTo(path)
}

// SaveToForce 는 이미 있는 파일도 덮어쓴다. `:w! <파일>` 이다.
func (buf *Buffer) SaveToForce(path string) error {
	return buf.saveTo(path)
}

// saveTo 는 검사 없이 path 에 쓴다.
//
// 이름 있는 buffer 는 사본만 쓴다. 이름도 dirty 도 그대로 두어서 이어지는 `:w` 는 여전히
// 원래 파일에 쓴다. 이름 없는 buffer 만 이 저장으로 그 파일의 buffer 가 된다 — vim 과 같은
// 나눔이고, 이름을 갈아치우는 것은 `:saveas` 의 몫이다 (ADR-0024).
func (buf *Buffer) saveTo(path string) error {
	out := buf.contents()

	if err := os.WriteFile(path, out, 0644); err != nil {
		return errors.Mark(err, errWriteFile)
	}

	if buf.path == "" {
		// 이제 이 파일의 buffer 다. 방금 쓴 것이 저장 기준이 되어 이어지는 `:w` 가
		// 자기가 쓴 것을 남의 변경으로 보지 않는다 (ADR-0015).
		sum := sha256.Sum256(out)

		buf.path = path
		buf.disk.hash = sum[:]
		buf.dirty = false
		buf.disk.outside = outsideSame

		// **이름이 바뀌는 자리는 여기 하나뿐이다.** 그래서 언어와 tab 폭을 다시 고르는 것도
		// 여기 두 줄이고, 담아둔 문법 토큰은 남의 언어의 것이라 버린다. `:saveas` 처럼 이름
		// 있는 buffer 의 이름을 갈아치우는 길이 생기면 그 자리도 이것을 해야 한다
		// (ADR-0080, ADR-0096).
		//
		// tab 폭도 경로가 정한다 — `.editorconfig` 가 경로별이라 이름이 붙으면서 답이 바뀐다.
		// 한 단계(indent) 는 게을러서 다음에 물을 때 알아서 다시 정한다.
		buf.language = syntax.LanguageFor(path)
		buf.tab = resolveTabWidth(path)
		buf.syntax = syntaxCache{}
	}

	return nil
}

// checkNotExist 는 그 자리에 파일이 없는지 본다.
// 있으면 `:w!` 로 빠져나가는 길을 담은 error 를 준다. vim 의 E13 과 같다.
//
// 다른 파일에 쓰는 것은 ADR-0015 의 해시 비교로 막을 수 없다 — 읽은 적이 없는 파일이라
// 맞춰 볼 기준이 아예 없다. 그래서 내용이 아니라 있는지 없는지만 본다 (ADR-0024).
func checkNotExist(path string) error {
	_, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return errors.Wrapf(err, "cannot check %s", path)
	}

	return errors.Errorf("파일이 이미 있습니다: %s. 덮어쓰려면 `:w!` 입니다", path)
}

// contents 는 파일에 쓸 내용이다.
// 줄끝 형식과 파일 끝 줄끝 유무는 읽었을 때 그대로 되돌린다.
func (buf Buffer) contents() []byte {
	eol := buf.lineEnding.bytes()

	size := 0
	for _, line := range buf.lines {
		size += len(line) + len(eol)
	}

	out := make([]byte, 0, size)
	for i, line := range buf.lines {
		if i > 0 {
			out = append(out, eol...)
		}
		out = append(out, line...)
	}
	if buf.finalLineEnding {
		out = append(out, eol...)
	}

	return out
}

// write 는 검사 없이 보고 있는 파일에 쓴다.
func (buf *Buffer) write() error {
	out := buf.contents()

	// 이미 있는 파일은 원래 권한을 유지한다. 0644 는 새로 만들 때만 쓰인다.
	if err := os.WriteFile(buf.path, out, 0644); err != nil {
		return errors.Mark(err, errWriteFile)
	}

	// 방금 쓴 것이 새 기준이다. 이어서 저장할 때 자기가 쓴 것을 남의 변경으로 보지 않는다.
	sum := sha256.Sum256(out)
	buf.disk.hash = sum[:]
	buf.dirty = false

	// 밖에서 바뀐 것을 `:w!` 로 덮어썼으면 이제 어긋난 것이 없다.
	buf.disk.outside = outsideSame

	return nil
}
