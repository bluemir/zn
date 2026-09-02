package core

import (
	"bytes"
	"crypto/sha256"
	"os"

	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/syntax"
)

// 디스크에 쓰는 길이다. `:w`·`:w!`·`:w <파일>`·`:w! <파일>` 넷이 문이고, 그 아래에 쓰기 직전
// buffer 를 맞추는 둘(포매터·`.editorconfig`) 이 붙는다.
//
// 맞추는 것이 쓰기보다 먼저다 — buffer 를 고치고 그것을 써야 화면과 파일이 같아진다
// (ADR-0015, ADR-0024, ADR-0052).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-save.go 다 (ADR-0121).

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

// splitFormatted 는 포매터가 낸 글을 buffer 의 줄로 나눈다.
//
// 마지막 줄바꿈은 떼고 나눈다. buffer 는 그것을 내용이 아니라 사실로 들고 있고, 그 사실은
// 여기서 바꾸지 않는다 — 마지막 줄바꿈을 넣고 빼는 것은 `.editorconfig` 가 정할 일이다
// (editorconfig.go 의 insert_final_newline, ADR-0052).
//
// 아무것도 안 나왔으면 빈 줄 하나다. buffer 에는 줄이 적어도 하나 있어야 한다.
func splitFormatted(out []byte) [][]byte {
	out = bytes.TrimSuffix(out, []byte{'\n'})
	if len(out) == 0 {
		return [][]byte{{}}
	}

	return bytes.Split(out, []byte{'\n'})
}

// equalLines 는 두 줄 묶음이 같은지다.
func equalLines(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}

	return true
}

// countChangedLines 는 자리로 맞대어 다른 줄이 몇인지다.
//
// 참 diff 가 아니다. 줄이 하나 늘면 그 아래가 전부 다른 줄로 세어진다 — 여기서 세는 것은
// 「이만큼 달라졌다」는 눈짐작이고, 정확한 자리는 화면이 이미 보여주고 있다.
func countChangedLines(old, next [][]byte) int {
	changed := 0
	for i := range max(len(old), len(next)) {
		switch {
		case i >= len(old) || i >= len(next):
			changed++
		case !bytes.Equal(old[i], next[i]):
			changed++
		}
	}

	return changed
}
