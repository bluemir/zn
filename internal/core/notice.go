package core

import (
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/textarea"
)

// 알림을 세우고 기록하는 자리다(ADR-0053).
//
// **알림을 내는 길은 여기 셋뿐이다.** 예전에는 `e.message` 에 직접 대입하는 자리가 스물,
// `normalModeMessage` 를 지나는 자리가 서른넷으로 갈려 있어서 기록을 붙일 자리가 없었다.
// 지금은 그 쉰넷이 전부 이 파일을 지난다 — `normalModeMessage` 도 `notify` 를 부른다.

// notice 는 statusBar 아래 줄에 떴던 알림 하나다.
//
// at 은 기록할 때 굳는다. `:jobs` 가 프레임마다 `time.Now()` 를 떠서 경과를 재는 것과 다르다 —
// 저쪽은 「지금」이 필요하고 이쪽은 「그때」가 필요하다(view-jobs.go 의 formatElapsed).
type notice struct {
	at     time.Time
	text   string
	failed bool // `error` 값에서 온 것인가
}

// notify 는 알림을 세우고 기록한다.
//
// 빈 문구를 걸러내지 않는다. 부르는 쪽이 빈 것을 넘기지 않아야 한다 — 여기서 조용히
// 아무 일도 안 하면, 알림이 안 뜬 까닭을 찾는 사람이 이 함수를 의심하지 않는다.
func (e *editor) notify(text string) {
	e.record(notice{text: text})
}

// notifyError 는 오류를 알림으로 세운다.
//
// **갈래는 사람이 판단하지 않는다.** `error` 값에서 왔으면 오류이고 아니면 그 밖이다.
// 알림을 내는 자리마다 「이건 오류인가 거절인가」를 물으면 그 판정이 쉰넷으로 흩어지고,
// 새 알림이 늘 때마다 같은 물음이 되풀이된다. 갈래가 이미 type 으로 있으니 그것을 쓴다(ADR-0053).
func (e *editor) notifyError(err error) {
	e.record(notice{text: noticeText(err), failed: true})
}

// 무엇을 하려다 실패했는지를 나타내는 센티넬들이다. 실패를 내는 자리가 `errors.Mark` 로
// 붙이고, 아래 noticeText 가 골라서 앞말로 쓴다.
//
// **앞말을 문자열로 지어서 나르지 않는 이유가 이것이다.** 지어 나르면 error 가 들고 있던
// 구조(경로·까닭)를 부수어 글자로 만든 뒤 다시 error 에 담게 된다. 표시는 오려면 언젠가
// 글자가 되어야 하지만, 그 자리는 **보여 주는 곳 하나**여야 한다(ADR-0053).
//
// **글을 읽고 쓰다 나는 둘은 buffer.go 에 있다.** 내는 자리가 그쪽이라 거기 두고, 고르는 것은
// 여기 noticeText 가 한다.
var (
	errCreateFile   = errors.New("만들 수 없습니다")
	errRemoveFile   = errors.New("지울 수 없습니다")
	errRenameFile   = errors.New("이름을 바꿀 수 없습니다")
	errNoWorkingDir = errors.New("지금 디렉터리를 찾을 수 없습니다")
)

// noticeText 는 오류 하나를 사람이 읽을 한 줄로 바꾼다. **오류가 글자가 되는 자리는 여기뿐이다.**
//
// 세 조각을 `:` 로 잇는다 — 무엇을 하려 했는가, 어느 파일인가, 왜 안 됐는가.
//
//	열 수 없습니다: /etc: is a directory
//	쓸 수 없습니다: /etc/x: permission denied
//	permission denied
//
// 가운데 둘은 우리가 짓지 않는다. `os.ReadFile`·`os.WriteFile` 이 `*os.PathError` 로 경로와
// 까닭을 들고 오므로 그대로 꺼내 쓴다. **그 안의 `Op` 은 쓰지 않는다** — 그것은 syscall 이지
// 우리 뜻이 아니라서, 쓰기가 권한으로 막히면 `Op` 이 `open` 이다. 하려던 일은 센티넬이 안다.
//
// 앞말이 없으면 그 자리를 비운다. 거절이 아니라 정말 예상 밖의 실패는 까닭만으로 읽힌다.
func noticeText(err error) string {
	parts := make([]string, 0, 3)

	if what := failedAction(err); what != "" {
		parts = append(parts, what)
	}

	// 아는 모양이면 그 구조에서 꺼낸다. 모양이 늘면 여기 위에 target 하나와 아래 case 하나가 는다.
	//
	// **Go 의 type switch 로는 안 된다.** `errors.Cause` 는 끝까지 벗겨서 `*os.PathError` 를
	// 지나 그 안의 errno 를 주고, 벗기지 않은 것은 우리 센티넬 표시(errors.Mark) 에 싸여 있어서
	// 겉 type 이 `*os.PathError` 가 아니다. 겹을 지나 그 모양을 찾아 주는 것이 `errors.As` 다.
	var pathErr *os.PathError

	switch {
	case errors.As(err, &pathErr):
		parts = append(parts, pathErr.Path, pathErr.Err.Error())
	default:
		// 감싼 개발자용 맥락(`cannot check %s`) 은 벗긴다. 사람에게 보일 앞말은 센티넬이 준다.
		parts = append(parts, errors.Cause(err).Error())
	}

	return strings.Join(parts, ": ")
}

// failedAction 은 무엇을 하려다 실패했는지다. 표시가 없으면 빈 문자열이다.
//
// 센티넬이 자기 문구를 들고 있어서 고르기만 하면 된다. 새 갈래는 위에 변수 하나와
// 여기 case 하나가 는다.
func failedAction(err error) string {
	switch {
	case errors.Is(err, textarea.ErrOpenFile):
		return textarea.ErrOpenFile.Error()
	case errors.Is(err, textarea.ErrWriteFile):
		return textarea.ErrWriteFile.Error()
	case errors.Is(err, errCreateFile):
		return errCreateFile.Error()
	case errors.Is(err, errRemoveFile):
		return errRemoveFile.Error()
	case errors.Is(err, errRenameFile):
		return errRenameFile.Error()
	case errors.Is(err, errNoWorkingDir):
		return errNoWorkingDir.Error()
	}

	return ""
}

// notifyFailure 는 이미 지은 문구를 오류 갈래로 세운다.
//
// 오류 값을 손에 들고 있으면서 **그 앞에 오류 밖의 것을 붙여야 하는** 자리가 쓴다. 지금
// 둘이다 — `job.go` 의 `<작업 이름> 실패: …` 와 `shell.go` 의 `셸 명령이 실패했습니다: …`.
// 이 둘을 `notifyError` 로 태우면 `errors.Cause` 가 앞말을 도로 벗겨 내서 어느 작업이
// 실패했는지가 사라진다.
//
// 셋째가 생기면 그때 앞말을 받는 꼴로 바꿀지 다시 본다. 지금 그렇게 만들면 인자 하나가
// 두 자리를 위해 늘 붙어 다닌다.
func (e *editor) notifyFailure(text string) {
	e.record(notice{text: text, failed: true})
}

// clearNotice 는 아래 줄을 비운다. **기록은 건드리지 않는다.**
//
// 「알림이 사라졌다」는 알림이 아니다. 지우는 것까지 남기면 목록이 빈 줄로 반쯤 찬다.
func (e *editor) clearNotice() {
	e.notice = ""
}

// record 는 세우고 남기는 한 자리다. 시각도 여기서 굳는다.
//
// 상한을 두지 않는다. 알림 하나가 백 바이트 남짓이라 만 개라도 1 MB 이고, 알림은 사람
// 속도로만 는다 — 키를 누르거나 백그라운드 일이 끝나야 난다.
//
// 같은 문구를 접지도 않는다. 끝난 작업 목록이 이름당 하나만 남기는 것(job.go 의 finishJob,
// ADR-0030) 과 규칙이 정반대인데, 거기서는 5 초마다 도는 갱신이 목록을 뒤덮는 것이 문제였고
// 여기서는 **같은 알림이 되풀이된 것 자체가 정보**다(ADR-0053).
func (e *editor) record(entry notice) {
	entry.at = time.Now()

	e.notice = entry.text
	e.notices = append(e.notices, entry)

	// tip 을 한 칸 민다. 알림이 지나는 깔때기가 여기 하나라서 「알림이 났다」를 정확히 한 번
	// 볼 수 있는 자리도 여기뿐이다 — clearNotice 는 키마다 불려서 매 키에 문장이 갈린다.
	//
	// 아래 줄이 이 알림에 덮여 있는 동안 바뀌므로 눈앞에서 글자가 갈리지 않는다. 다음 키에
	// 알림이 걷히면 그때 새 문장이 드러난다(tip.go 의 renderWithTip, ADR-0061).
	e.nextTip()
}
