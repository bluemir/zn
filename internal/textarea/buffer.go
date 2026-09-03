package textarea

import (
	"bytes"
	"crypto/sha256"
	"os"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/scheme"
	"github.com/bluemir/zn/internal/syntax"
)

// Buffer 는 파일 하나의 **글**이다. 이 파일에는 type 과  생성자만 있고, 메서드 40 개는
// 갈래별로 `buffer-*.go` 에 나뉘어 있다.
//
// **커서와 화면은 여기서 다루지 않는다.** 「그 글의 어디를 보고 있나」는 viewport 가 들고, 커서를
// 만지는 메서드 88 개도 그쪽에 있다. 그 가름을 컴파일러가 지킨다 — 여기 커서 필드가 없으니
// 이 40 개는 커서를 만질 수 없다 (viewport.go, ADR-0121).
//
// **파일 접두가 곧 receiver 다.** `buffer-*.go` 에는 Buffer 의 메서드가, `viewport-*.go` 에는
// viewport 의 메서드가 있다. 같은 갈래가 양쪽에 있는 것(`buffer-delete.go` 와
// `viewport-delete.go`) 은 「글을 고치는 쪽」과 「커서를 옮기며 그것을 부르는 쪽」이다.
//
// 여러 갈래가 나눠 쓰는 type·도우미와 다른 receiver 의 메서드는 접두 없는 파일에 남는다 —
// `register` 는 register.go, `screenRow` 는 row.go 에 있다.
//
// **겹을 오가는 자료는 core 에도 없다.** `scheme.Cursor`·`scheme.Cell`·`scheme.MotionRange` 는
// `internal/scheme` 이다. 이 편집기 전체에서 통용되는 개념이라 어느 겹의 것도 아니다 (ADR-0122).
//
// **줄 하나를 재고 가르는 것도 접두 없는 파일이다.** 화면에서 재고 그릴 글자로 바꾸는 것은
// cluster.go, 무엇이 한 단어인지는 glyph-class.go 다 (ADR-0119).
//
// # Buffer 가 하는 일과 안 하는 일 (ADR-0100, ADR-0121)
//
// **드는 것은 글과 그 글에 딸린 것이다.** 줄, 되돌리기 이력, 디스크와 맞춰 본 것, 이 파일의
// tab 폭과 들여쓰기, 그리고 밖에서 채워 넣는 캐시 셋(언어 서버 진단·git 마커·문법 토큰) 이다.
//
// **내놓는 것은 글을 고치는 한 걸음이다.** 줄을 넣고 지우고 갈아끼우고, 파일에 쓴다. 그
// 한 걸음마다 지켜야 할 불변(되돌리기 구간, 문법 캐시의 어긋남) 을 안에서 지킨다.
//
// **안 하는 것이 셋이다.**
//
//   - **커서를 모른다.** 어디를 고칠지는 범위(`scheme.MotionRange`) 로 받는다. 그 범위를 만드는 것도
//     고치고 나서 커서를 어디 둘지도 viewport 가 한다
//   - **키가 무엇을 뜻하는지 모른다.** `motion` 을 받지 않는다. 「`dw` 가 어디까지인가」를
//     정하는 것은 motion.go 다
//   - **register 를 어디에 담을지 모른다.** `register` 라는 짐은 주고받지만 `"a`·숫자 링
//     같은 이름은 `e.registers` 가 다룬다
//
// **흐린 자리가 하나 남았다.** 화면 폭(`width`) 을 아직 메서드 예순 남짓이 받는다. 화면 행
// 이동과 `desiredX` 이 줄바꿈에 걸려 있어서인데, 그 둘이 viewport 로 갔으므로 이 인자도
// 그쪽으로 모을 수 있다 (docs/tasks.md).

// Buffer 는 파일 하나에 대응 한다.
//
// data 는 파일을 통째로 읽은 것으로 읽은 뒤에는 바꾸지 않는다.
// lines 는 data 를 가리키는 subslice 라 줄 내용을 복사하지 않는다.
// 편집한 줄만 새로 할당한 []byte 로 갈아끼운다. (ADR-0001)
//
// lines 의 각 줄은 줄끝 문자를 담지 않는다. CRLF 파일이어도 \r 을 뺀 범위를 가리키므로
// 편집과 렌더는 줄끝을 신경 쓰지 않는다. 줄끝은 저장할 때 다시 끼워 넣는다.
type Buffer struct {
	Path string

	// language 는 이 파일의 언어다. 이름에서 한 번 골라 들고 있는다(ADR-0080)
	// 강조·들여쓰기·머리줄의 동작 방식을 정한다.(syntax/detect.go). nil 은 모르는 언어다.
	Language *syntax.Language

	Dirty bool // 마지막 저장 이후 변경사항의 여부.

	// readOnly 는 이 파일을 고칠 수 없다는 것이다. 열 때 권한을 보고 정하고 그 뒤로 바뀌지 않는다.
	//
	// 정의로 뛰어서 열리는 표준 라이브러리·의존 모듈의 파일이 이것이다 — module cache 는
	// `r--r--r--` 이다(ADR-0051). 고치는 동작이 첫 줄에서 이것을 본다(readonly.go).
	ReadOnly bool

	data  []byte
	lines [][]byte

	lineEnding      lineEnding
	finalLineEnding bool //파일 마지막 줄이 줄끝 문자로 끝났는지

	// **커서와 화면 자리는 여기 없다.** 「이 파일의 어디를 보고 있나」는 viewport 가 든다
	// (viewport.go). 이 type 은 글과 그 글에 딸린 것만 든다.

	// indent 는 이 파일이 한 단계에 쓰는 공백이다(indent.go). 게을러서 처음 쓸 때 정한다.
	indent indentUnit

	// tab 은 이 파일에서 tab 하나가 미는 화면 칸 수다(indent.go).
	// .editorconfig 에 정의된 값을 존중 한다. 따라서 파일마다 다를수 있다.
	tab int

	// undo, redo 는 되돌리기 이력
	// editing 은 열린 구간이 있는지다. 이어지는 타이핑을 한 항목으로 모은다.
	undo    []edit
	redo    []edit
	editing bool

	// disk 는 디스크에서 파일을 읽어왔을때의 정보이다. 파일 변경 감지 등에 쓰인다.
	disk lastDiskState

	// diagnostics 는 gopls 가 이 파일에 대해 보낸 진단이다. 줄번호로 모아 둔다(diagnostics.go).
	//
	// 파일 내용에서 나온 것이라 커서·스크롤·문법 토큰과 같이 이 파일에 딸려 있다(위 주석).
	// 값 필드라 Reload 가 buffer 를 통째로 갈아끼울 때(`*buf = next`) 저절로 비워진다 —
	// 다시 읽은 내용의 진단은 서버가 새로 보내온다(ADR-0086).
	diagnostics map[int][]Diagnostic

	// git 은 HEAD 와 견줘 낸 것이다. 아래 gitCache 에 무엇이 왜 드는지 있다.
	git gitCache

	// syntax 는 문법 강조 토큰을 담아 둔 것이다. 파일 내용에서 나온 것이라 커서·스크롤과 같이
	// 이 파일에 딸려 있다(위 주석).
	//
	// 값 필드다. 그래서 newBuffer 로 새로 열 때와 Reload 가 buffer 를 통째로 갈아끼울 때
	// (`*buf = next`) 저절로 비워진다 — 비우는 코드를 따로 두지 않는다 (syntax.go).
	syntax syntaxCache
}

// edit 은 되돌릴 수 있는 변경 하나다. lines 의 [at, at+count) 를 before 로 바꾸면 되돌아간다.
//
// before 는 그 자리에 원래 있던 줄들을 참조로만 담는다. 갈아끼우기 방식이라 옛 줄이 그대로
// 살아있어서 내용 복사가 일어나지 않는다(ADR-0001).
//
// undo 할 때 지금 내용으로 역방향 edit 을 만들어 redo 에 넣는다. 그래서 한 type 으로 양방향을 다룬다.
type edit struct {
	at     int      // lines 의 시작 index
	before [][]byte // 그 자리에 원래 있던 줄들
	count  int      // 편집 후 그 자리를 차지하는 줄 수

	// cursor 는 되돌린 뒤 커서를 놓을 자리다.
	//
	// **위의 `at` 과 다른 물음의 답이다.** `at` 은 「무엇을 되돌리나」이고 이것은 「되돌린 뒤
	// 어디에 설까」다. 셋에서 갈린다.
	//
	//   - `at` 은 줄 index 뿐이라 **칸을 담지 못한다**
	//   - 범위를 받는 편집(`dd`·`:s`·`>`) 은 `at` 이 범위의 시작이라, visual 에서 아래에서
	//     위로 골랐으면 커서는 범위의 끝에 있다
	//   - 편집이 넓어지면 `at` 은 따라 밀리지만 이것은 **구간을 열 때 한 번만** 담긴다.
	//     줄 시작에서 Backspace 로 앞 줄과 합칠 때가 그 경로다(beginEdit)
	//
	// **그래서 되돌린 결과에서 유도할 수 없다.** 연속 타이핑을 한 항목으로 합치므로
	// (ADR-0001) 여기 드는 것은 「어디를 고쳤나」가 아니라 「고치기 시작할 때 어디 있었나」이고,
	// 그 시점의 커서는 되돌린 글에 남지 않는다. vim 도 같은 까닭으로 `uh_cursor` 를 따로 담는다.
	//
	// **담는 것은 `Buffer` 이고 뜻을 아는 것은 창이다.** 여기서는 int 둘일 뿐이고, 담고 읽는
	// 것은 viewport 의 beginEdit·revert 다 (ADR-0121).
	Cursor scheme.Cursor
}

// newEmptyBuffer 는 파일 없이 시작하는 빈 창을 만든다.
//
// **창(viewport) 까지 만들어 준다.** 지금은 tab 과 파일이 1:1 이라 「파일을 열면 그것을 볼
// 창이 하나 생긴다」가 사실이다. 화면 분할이 오면 그때 갈린다(viewport.go).
func NewEmptyBuffer(path string) Viewport {
	return Viewport{Buffer: Buffer{
		Path:            path,
		Language:        syntax.LanguageFor(path),
		tab:             resolveTabWidth(path),
		lines:           [][]byte{{}},
		finalLineEnding: true, // 새 파일은 줄끝으로 끝낸다
	}}
}

// newBufferReadOnly 는 읽기 전용 표시를 붙인다. 파일을 여는 길이 여럿이라 표시를 붙이는
// 자리도 여럿이 되지 않게, 읽는 자리에서 한 번 본다.

// OpenBuffer 는 파일을 읽어서 창으로 만든다(newEmptyBuffer 의 주석을 같이 본다).
// 파일이 없으면 빈 줄 하나짜리 새 창을 만든다.
func OpenBuffer(path string) (Viewport, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewEmptyBuffer(path), nil
	}
	if err != nil {
		// **무엇을 하려 했는지만 표시하고 문구는 짓지 않는다.** 경로와 까닭은 os 가 준
		// `*os.PathError` 가 이미 들고 있어서, 글자로 부수면 그 구조를 버리는 것이 된다.
		// 한 줄로 만드는 자리는 보여 주는 곳 하나다(notice.go 의 noticeText, ADR-0053).
		return Viewport{}, errors.Mark(err, ErrOpenFile)
	}

	return NewBuffer(path, data), nil
}

func NewBuffer(path string, data []byte) Viewport {
	sum := sha256.Sum256(data)
	buf := Buffer{
		Path:       path,
		Language:   syntax.LanguageFor(path),
		tab:        resolveTabWidth(path),
		data:       data,
		lineEnding: detectLineEnding(data),
		disk:       lastDiskState{Hash: sum[:]},
		ReadOnly:   DetectReadOnly(path),
	}

	buf.lines, buf.finalLineEnding = SplitLines(data)

	return Viewport{Buffer: buf}
}

// splitLines 는 파일 내용을 줄로 가른다. 줄끝은 줄에 남기지 않는다.
//
// HEAD 에 든 내용을 가르는 자리도 이것을 쓴다(git-lines.go). 규칙이 두 벌이면 마지막 줄
// 하나가 늘 다르게 갈려서, 고치지 않은 파일의 끝줄에 마커가 선다.
func SplitLines(data []byte) ([][]byte, bool) {
	// 마지막 줄끝은 빈 줄이 아니라 "줄끝으로 끝났다" 는 사실이므로 떼어내고 기록한다.
	rest := data
	finalLineEnding := false

	if len(rest) > 0 && rest[len(rest)-1] == '\n' {
		finalLineEnding = true
		rest = rest[:len(rest)-1]
		if len(rest) > 0 && rest[len(rest)-1] == '\r' {
			rest = rest[:len(rest)-1]
		}
	}

	lines := bytes.Split(rest, []byte{'\n'})
	for i, line := range lines {
		if len(line) > 0 && line[len(line)-1] == '\r' {
			lines[i] = line[:len(line)-1]
		}
	}

	return lines, finalLineEnding
}

// detectLineEnding 은 첫 줄의 줄끝으로 파일 전체의 형식을 판정한다.
// 줄마다 섞여 있는 파일은 저장할 때 이 형식으로 통일된다.
func detectLineEnding(data []byte) lineEnding {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		return lineEndingCRLF
	}
	return lineEndingLF
}

// detectReadOnly 는 그 파일을 고칠 수 없는지다. 파일이 없으면 거짓이다 — 새로 만드는 것이다.
//
// **권한 비트만 본다.** 소유자·그룹·ACL 을 따지지 않는다. 정확한 답은 실제로 열어 보는 것뿐인데,
// 그러면 파일을 열 때마다 쓰기로 한 번 더 여는 일이 붙는다. 여기서 놓치는 것(남의 파일이지만
// 비트는 열려 있는 경우) 은 저장할 때 오류로 잡히고, 그 자리에는 이미 문구가 있다.
func DetectReadOnly(path string) bool {
	if path == "" {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if info.IsDir() {
		return false
	}

	return info.Mode().Perm()&0200 == 0
}

// 글을 읽고 쓰다 나는 실패의 센티넬이다. 내는 자리가 여기라 여기 둔다 — 무엇을 하려다
// 실패했는지를 나타내는 나머지는 notice.go 에 있고, 고르는 것도 그쪽이 한다(ADR-0053).
var (
	ErrOpenFile  = errors.New("열 수 없습니다")
	ErrWriteFile = errors.New("쓸 수 없습니다")
)

// 아래 열은 **밖에서 안쪽 살림을 묻는 문**이다.
//
// `disk`·`git`·`syntax` 는 이 파일의 상태를 담아 둔 것이라 밖에서 속을 헤집을 것이 아니다.
// 그런데 바깥 검사·git 갱신·언어 서버가 그 값을 알아야 해서, 필드를 여는 대신 물음마다 문을
// 하나씩 낸다. 창을 새 패키지로 낼 때 열 면이 그만큼 좁아진다 (ADR-0127).

// Line 은 그 줄의 내용이다. 범위 밖이면 빈 줄이다.
//
// **돌려주는 것을 고치지 말 것.** 이 저장소는 줄의 byte 를 제자리에서 고치는 자리가 하나도
// 없다 — 편집은 언제나 새 []byte 를 지어 ReplaceLines 로 갈아끼운다. 제자리 수정은 되돌리기
// 기록을 덮어서 돌릴 수 없게 만든다(buffer-edit.go).
func (buf Buffer) Line(n int) []byte {
	if n < 0 || n >= len(buf.lines) {
		return nil
	}

	return buf.lines[n]
}

// LineCount 는 줄 수다. 빈 파일도 빈 줄 하나라 0 이 되지 않는다.
func (buf Buffer) LineCount() int {
	return len(buf.lines)
}

// AllLines 는 글 전체다. 파일을 통째로 훑는 쪽이 쓴다 — 언어 서버에 보내는 자리와
// 표 맞추기다.
//
// **겉 slice 는 사본이다.** 받은 쪽이 `lines[3] = ...` 로 창의 줄을 바꿔치기할 수 없다.
// 안쪽 []byte 는 같은 것을 가리키므로 Line 과 같은 규칙이 걸린다 — 고치지 말 것.
//
// 한 줄만 필요하면 Line 을 쓴다. 이것은 부를 때마다 겉 slice 를 새로 만든다.
func (buf Buffer) AllLines() [][]byte {
	out := make([][]byte, len(buf.lines))
	copy(out, buf.lines)

	return out
}

// diskSeenAt 은 마지막으로 읽거나 쓴 그 파일의 자국이다. 바깥 변경 검사가 기준으로 쓴다.
func (buf Buffer) DiskSeenAt() (hash []byte, size int64, mtime time.Time) {
	return buf.disk.Hash, buf.disk.Size, buf.disk.mtime
}

// markDiskStamp 는 크기와 시각만 새로 적는다. 해시는 그대로다 — 다음 검사가 읽지 않고
// 끝나게 해 주는 앞잡이라, 내용이 같다고 판정한 뒤에도 갱신한다(outside.go).
func (buf *Buffer) MarkDiskStamp(size int64, mtime time.Time) {
	buf.disk.Size, buf.disk.mtime = size, mtime
}

// outsideState 는 마지막 검사에서 바깥이 어떠했는지다. statusBar 의 `[!]` 가 이것을 본다.
func (buf Buffer) OutsideState() OutsideChange {
	return buf.disk.outside
}

// setOutsideState 는 그것을 적는다. 달라진 순간을 가리는 것은 부르는 쪽이 한다.
func (buf *Buffer) SetOutsideState(change OutsideChange) {
	buf.disk.outside = change
}

// syntaxRevision 은 내용이 갈린 횟수다. 언어 서버의 답이 지금 내용의 것인지 가르는 데 쓴다
// (semantic.go, ADR-0103).
func (buf Buffer) SyntaxRevision() int {
	return buf.syntax.revision
}
