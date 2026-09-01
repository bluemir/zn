package core

import (
	"bytes"
	"crypto/sha256"
	"os"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/lsp"
	"github.com/bluemir/zn/internal/syntax"
)

// Buffer 는 파일 하나다. 이 파일은 type 과 태어나는 자리만 들고, 메서드 116 개는 갈래별로
// `buffer-*.go` 에 나뉘어 있다.
//
// **`buffer-*.go` 에는 Buffer 의 메서드만 둔다.** 여러 갈래가 나눠 쓰는 type·도우미와 다른
// receiver 의 메서드는 접두 없는 파일에 남는다 — `register` 는 register.go, `motionRange` 는
// range.go, `screenRow` 는 row.go 에 있다.
//
// **줄 하나를 재고 가르는 것도 접두 없는 파일이다.** 화면에서 재고 그릴 글자로 바꾸는 것은
// cluster.go, 무엇이 한 단어인지는 glyph-class.go 다 (ADR-0119).
//
// # Buffer 가 하는 일과 안 하는 일 (ADR-0100)
//
// **드는 것은 글과, 그 글을 보고 있는 상태다.** 줄과 커서만이 아니다 — 화면 스크롤 자리
// (`top`·`topRow`), 고른 범위, 되돌리기 이력, 디스크와 맞춰 본 것, 그리고 밖에서 채워 넣는
// 캐시 셋(언어 서버 진단·git 마커·문법 토큰) 이 같이 있다.
//
// **내놓는 것은 한 걸음짜리 조작이다.** 커서를 옮기고, 줄을 넣고 지우고, 파일에 쓴다.
// 그 한 걸음마다 지켜야 할 불변(`desiredCol`, 되돌리기 구간) 을 안에서 지킨다.
//
// **안 하는 것이 셋이다.**
//
//   - **키가 무엇을 뜻하는지 모른다.** `motion` 을 받지 않고 `motionRange` 만 받는다.
//     「`dw` 가 어디까지인가」를 정하는 것은 motion.go 이고, 여기는 그 범위를 받아 일한다
//   - **범위만으로 정해지는 커서 자리는 여기서 두지 않는다.** 복사는 `y` 의 커서 규칙을
//     모르고 register 만 만든다. 부르는 쪽이 「복사하고, 커서를 옮긴다」를 그 차례로 시켜서
//     `y` 와 `~` 의 미묘한 차이가 그 자리에 드러난다(action.go)
//
//     **편집이 만들어 낸 자리는 반대다.** 붙인 글이 끝난 자리, 지운 자리, 이은 자리, 지운
//     줄을 메운 줄 — 그것을 아는 것은 편집뿐이라 `pasteText`·`deleteText`·`joinLines`·
//     `deleteLines` 가 제 커서를 둔다. 밖으로 내면 그 자리를 돌려주는 값이 하나 늘 뿐이다
//   - **register 를 어디에 담을지 모른다.** `register` 라는 짐은 주고받지만 `"a`·숫자 링
//     같은 이름은 `e.registers` 가 다룬다
//
// **경계가 흐린 자리 둘은 알고 둔다.** 커서와 스크롤은 tab 마다 유지하려고 여기 든 것이라
// 화면 분할이 오면 밖으로 빼야 하고(아래 주석), 화면 폭(`width`) 을 메서드 예순 남짓이 받는다 —
// 화면 행 이동과 `desiredCol` 이 줄바꿈에 걸려 있어서다.

// viewPlace 는 「이 파일의 어디를 보고 있나」다. 커서와 화면 자리를 함께 담는다.
//
// **`Buffer` 가 이것을 embed 한다.** 그래서 이 넷은 지금 보고 있는 자리이면서, 담아 두었다가
// 되돌릴 수 있는 한 덩어리다. 담는 자리(검색 무르기·치환 그만두기) 가 넷을 따로 들던 것을
// 이 type 이 걷었고, `Buffer` 쪽도 같은 덩어리로 묶었다.
//
// **넷이 함께여야 한다.** 커서만 되돌리면 보이는 곳이 달라진 채로 남는다.
//
// 되돌리는 것은 `Buffer` 가 한다(buffer-screen.go 의 moveToPlace). 커서를 쓰는 일이라
// `desiredCol` 을 다시 맞추는 것까지 안에서 끝나야 한다(ADR-0100).
//
// **`desiredCol` 과 `selection` 은 여기 없다.** 앞엣것은 담는 것이 아니라 되돌린 뒤 다시
// 재는 파생값이고, 뒤엣것은 고른 범위라 「어디를 보고 있나」와 갈래가 다르다.
type viewPlace struct {
	// cursorLine 은 lines 의 index, cursorCol 은 그 줄 안의 byte offset 이다.
	cursorLine, cursorCol int

	// top, topRow 는 화면 최상단에 그릴 자리다. **커서에서 파생할 수 없다** — 커서를 두고
	// 화면만 움직이는 동작이 있고, 커서가 화면 안에 있는 동안은 화면이 움직이지 않아야 한다.
	// 줄 하나가 화면 행 여러 개가 될 수 있어서 줄 번호만으로는 부족하다.
	top    int // lines 의 index
	topRow int // 그 줄의 몇 번째 wrap 행부터 그리는지
}

// gitCache 는 HEAD 에 든 이 파일과 견줘 낸 것이다(git-lines.go, ADR-0094).
//
// 셋이 **같이 채워지고 같이 낡는다.** head 가 지금 HEAD 와 다르면 base 도 marks 도 낡은
// 것이라 셋을 함께 다시 짓는다 — `git commit`·`checkout` 으로 기준이 통째로 움직이는 자리다.
//
// 값 필드라 Reload 가 buffer 를 통째로 갈아끼울 때(`*buf = next`) 저절로 비워진다. 다음 git
// 갱신이 다시 채운다(진단·문법 캐시와 같은 자리다).
type gitCache struct {
	// base 는 HEAD 에 든 이 파일의 내용이다. 추적하지 않는 파일은 nil 이다.
	base [][]byte

	// head 는 base 를 읽어온 HEAD 해시다. 추적하지 않는 파일은 base 가 nil 인 채 이것만
	// 적힌다 — 「없다」와 「아직 안 읽었다」를 이것이 가른다.
	head string

	// marks 는 base 와 지금 내용을 견줘 낸 줄별 마커다. 줄번호 칸과 트리가 이것을 그린다.
	marks map[int]gitLineMark
}

// diskSeen 은 디스크와 마지막으로 맞춰 봤을 때 그 파일이 어떠했는지다.
//
// **지금 이 순간의 사실이 아니다.** 맞춰 보는 것은 포커스가 돌아올 때·셸에서 올라올 때뿐이라
// 여기 든 것은 그때 본 것이다. 저장은 이것을 믿지 않고 그 자리에서 다시 읽는다 (ADR-0015).
//
// 넷을 묶은 것은 **같이 갱신되고 같이 낡기** 때문이다. 하나만 새것이면 판정이 어긋난다.
type diskSeen struct {
	// size, mtime 은 그때 파일의 크기와 mtime 이다.
	//
	// 다음 검사에서 이 둘이 그대로면 내용을 읽지 않는다 — 읽고 해시를 내는 것이 검사 값의
	// 거의 전부여서, 유휴 상태의 값이 파일 크기와 무관해진다(ADR-0044).
	//
	// mtime 을 *판정* 으로 쓰지는 않는다. 내용이 같아도 mtime 이 바뀌는 일이 흔해서 그것으로
	// 판정하면 헛경고가 잦다(ADR-0015). 여기서는 「그대로면 안 읽는다」 는 한쪽으로만 쓴다 —
	// 틀리는 방향이 「괜히 한 번 더 읽는다」 라서 판정이 달라지지 않는다.
	//
	// mtime 이 zero 면 앞잡이가 없다는 뜻이고 그때는 읽어서 해시를 낸다.
	size  int64
	mtime time.Time

	// hash 는 마지막으로 읽거나 쓴 시점의 파일 내용 해시다. nil 이면 그때 파일이 없었다는 뜻이다.
	// 저장하기 직전에 파일을 다시 읽어 이것과 맞춰 보고, 다르면 쓰지 않는다 (ADR-0015).
	hash []byte

	// outside 는 그때 바깥이 어떻게 달라져 있었는지다. statusBar 의 `[!]` 가 이것이고,
	// 알림과 달리 다음 키에 사라지지 않는다 (ADR-0031).
	outside outsideChange
}

// lineEnding 은 파일의 줄끝 형식이다. 읽을 때 판정해서 저장할 때 그대로 되돌린다.
type lineEnding int

const (
	lineEndingLF lineEnding = iota
	lineEndingCRLF
)

func (e lineEnding) bytes() []byte {
	if e == lineEndingCRLF {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// name 은 사람에게 보이는 이름이다. `.editorconfig` 의 `end_of_line` 값과 같은 글자다 —
// 저장할 때 무엇으로 맞췄는지 알리는 자리가 쓴다(editorconfig.go, ADR-0052).
func (e lineEnding) name() string {
	if e == lineEndingCRLF {
		return "CRLF"
	}

	return "LF"
}

// Buffer 는 파일 하나에 대응 한다.
//
// data 는 파일을 통째로 읽은 것으로 읽은 뒤에는 바꾸지 않는다.
// lines 는 data 를 가리키는 subslice 라 줄 내용을 복사하지 않는다.
// 편집한 줄만 새로 할당한 []byte 로 갈아끼운다. (ADR-0001)
//
// lines 의 각 줄은 줄끝 문자를 담지 않는다. CRLF 파일이어도 \r 을 뺀 범위를 가리키므로
// 편집과 렌더는 줄끝을 신경 쓰지 않는다. 줄끝은 저장할 때 다시 끼워 넣는다.
type Buffer struct {
	path string

	// language 는 이 파일의 언어다. 이름에서 한 번 골라 들고 있는다 — 강조·들여쓰기·머리줄이
	// 이것의 세 칸에서 온다(syntax/detect.go).
	//
	// **경로와 나뉜 것이 요점이다.** path 는 「어디에 쓰는가」이고 이것은 「어떤 문법인가」라,
	// 한 필드가 둘을 겸하면 캐시가 무엇 때문에 무효가 되는지 갈리지 않는다 (ADR-0080).
	//
	// nil 은 모르는 언어다. 세 칸을 꺼내는 것이 nil 인 채로도 되므로 검사를 앞세우지 않는다.
	language *syntax.Language

	data  []byte
	lines [][]byte

	lineEnding      lineEnding
	finalLineEnding bool //파일 마지막 줄이 줄끝 문자로 끝났는지

	// 아래는 파일 내용이 아니라 이 파일을 어떻게 보고 있는지다.
	// tab 을 오갈 때 파일별로 유지되어야 하므로 Buffer 가 들고 있다.
	// 화면 분할을 도입하면 같은 파일에 커서가 둘이 되므로 그때는 밖으로 빼야 한다.
	//
	// **한 덩어리로 묶어 두었다.** 나갈 때 통째로 나가야 하는 것들이고, 담아 두었다가
	// 되돌리는 자리도 그 단위로 다룬다(place·moveToPlace).

	viewPlace

	// desiredCol 은 위아래로 움직일 때 지킬 열이다. **화면 행 안에서 센 칸이다**(ADR-0108).
	//
	// 그렇게 둔 것은 `↑`/`↓` 가 화면 행 단위라서다(ADR-0006, ADR-0076). 대가로 wrap 된 줄의
	// 둘째 행 이후에서 `j`/`k` 를 누르면 다음 줄의 첫 화면 행에 선다. vim 은 이 칸을 줄
	// 시작에서 세므로 그 자리에서 다르고, 그대로 두기로 정했다(placeCursorInLine).
	//
	// viewPlace 에 안 든 것은 **담는 값이 아니기 때문**이다. 되돌린 뒤 다시 잰다.
	desiredCol int

	// selection 은 visual mode 가 고른 범위의 반대쪽 끝이다. 이쪽 끝은 커서다(selection.go).
	selection selection

	// indent 는 이 파일이 한 단계에 쓰는 공백이다(indent.go). 게을러서 처음 쓸 때 정한다.
	indent indentUnit

	// tab 은 이 파일에서 tab 하나가 미는 화면 칸 수다(indent.go).
	//
	// **파일마다 다르다.** `.editorconfig` 는 경로별이라 `tab_width = 8` 인 하위 디렉터리와
	// 그렇지 않은 곳이 한 화면에 tab 으로 같이 열려 있을 수 있다 (ADR-0096).
	//
	// indent 와 달리 게으르지 않다. language 처럼 경로에서 한 번 골라 들고 있는다 — 까닭은
	// resolveTabWidth 에 있다. 읽는 자리는 tabWidth() 하나다.
	tab int

	// undo, redo 는 되돌리기 이력
	// editing 은 열린 구간이 있는지다. 이어지는 타이핑을 한 항목으로 모은다.
	undo    []edit
	redo    []edit
	editing bool

	dirty bool //마지막 저장 이후 변경사항의 여부.

	// readOnly 는 이 파일을 고칠 수 없다는 것이다. 열 때 권한을 보고 정하고 그 뒤로 바뀌지 않는다.
	//
	// 정의로 뛰어서 열리는 표준 라이브러리·의존 모듈의 파일이 이것이다 — module cache 는
	// `r--r--r--` 이다(ADR-0051). 고치는 동작이 첫 줄에서 이것을 본다(readonly.go).
	readOnly bool

	// disk 는 디스크와 마지막으로 맞춰 본 것이다. 아래 diskSeen 에 무엇이 왜 드는지 있다.
	disk diskSeen

	// diagnostics 는 gopls 가 이 파일에 대해 보낸 진단이다. 줄번호로 모아 둔다(diagnostics.go).
	//
	// 파일 내용에서 나온 것이라 커서·스크롤·문법 토큰과 같이 이 파일에 딸려 있다(위 주석).
	// 값 필드라 Reload 가 buffer 를 통째로 갈아끼울 때(`*buf = next`) 저절로 비워진다 —
	// 다시 읽은 내용의 진단은 서버가 새로 보내온다(ADR-0086).
	diagnostics map[int][]lsp.Diagnostic

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

	cursorLine, cursorCol int // 되돌린 뒤 커서를 놓을 자리
}

// newEmptyBuffer 는 파일 없이 시작하는 빈 Buffer 를 만든다.
func newEmptyBuffer(path string) Buffer {
	return Buffer{
		path:            path,
		language:        syntax.LanguageFor(path),
		tab:             resolveTabWidth(path),
		lines:           [][]byte{{}},
		finalLineEnding: true, // 새 파일은 줄끝으로 끝낸다
	}
}

// newBufferReadOnly 는 읽기 전용 표시를 붙인다. 파일을 여는 길이 여럿이라 표시를 붙이는
// 자리도 여럿이 되지 않게, 읽는 자리에서 한 번 본다.

// OpenBuffer 는 파일을 읽어서 Buffer 로 만든다.
// 파일이 없으면 빈 줄 하나짜리 새 Buffer 를 만든다.
func OpenBuffer(path string) (Buffer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newEmptyBuffer(path), nil
	}
	if err != nil {
		// **무엇을 하려 했는지만 표시하고 문구는 짓지 않는다.** 경로와 까닭은 os 가 준
		// `*os.PathError` 가 이미 들고 있어서, 글자로 부수면 그 구조를 버리는 것이 된다.
		// 한 줄로 만드는 자리는 보여 주는 곳 하나다(notice.go 의 noticeText, ADR-0053).
		return Buffer{}, errors.Mark(err, errOpenFile)
	}

	return newBuffer(path, data), nil
}

func newBuffer(path string, data []byte) Buffer {
	sum := sha256.Sum256(data)
	buf := Buffer{
		path:       path,
		language:   syntax.LanguageFor(path),
		tab:        resolveTabWidth(path),
		data:       data,
		lineEnding: detectLineEnding(data),
		disk:       diskSeen{hash: sum[:]},
		readOnly:   detectReadOnly(path),
	}

	buf.lines, buf.finalLineEnding = splitLines(data)

	return buf
}

// splitLines 는 파일 내용을 줄로 가른다. 줄끝은 줄에 남기지 않는다.
//
// HEAD 에 든 내용을 가르는 자리도 이것을 쓴다(git-lines.go). 규칙이 두 벌이면 마지막 줄
// 하나가 늘 다르게 갈려서, 고치지 않은 파일의 끝줄에 마커가 선다.
func splitLines(data []byte) ([][]byte, bool) {
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
