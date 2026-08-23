package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 알림은 아래 줄에 서는 동시에 기록에 남는다. 둘이 갈리면 「지나간 알림 보기」가 성립하지 않는다.
func TestNotifyRecords(t *testing.T) {
	e := &editor{}

	e.notify("저장함: a.txt")

	assert.Equal(t, "저장함: a.txt", e.notice)

	require.Len(t, e.notices, 1)
	assert.Equal(t, "저장함: a.txt", e.notices[0].text)
	assert.False(t, e.notices[0].failed)
	assert.False(t, e.notices[0].at.IsZero(), "시각은 기록할 때 굳는다")
}

// 갈래는 사람이 판단하지 않고 `error` 값에서 왔는가로 갈린다(ADR-0053).
func TestNoticeKindComesFromErrorValue(t *testing.T) {
	e := &editor{}

	e.notify("읽기 전용 파일입니다")
	e.notifyError(errors.New("permission denied"))
	e.notifyFailure("git 상태 실패: exit status 128")

	require.Len(t, e.notices, 3)
	assert.False(t, e.notices[0].failed, "거절은 오류가 아니다")
	assert.True(t, e.notices[1].failed)
	assert.True(t, e.notices[2].failed, "앞말을 붙인 실패도 오류다")
}

// 감싼 개발자용 맥락은 벗긴다. 벗기는 자리가 하나여야 문구가 자리마다 갈리지 않는다.
func TestNoticeTextUnwrapsCause(t *testing.T) {
	e := &editor{}

	e.notifyError(errors.Wrap(errors.New("permission denied"), "cannot check /긴/경로/a.txt"))

	assert.Equal(t, "permission denied", e.notice)
}

// 사람에게 보일 앞말은 감싸지 않고 지은 것이라 벗겨지지 않는다(search.go 의 `잘못된 패턴`).
func TestNoticeTextKeepsComposedMessage(t *testing.T) {
	e := &editor{}

	e.notifyError(errors.Newf("잘못된 패턴: %v", errors.New("missing closing )")))

	assert.Equal(t, "잘못된 패턴: missing closing )", e.notice)
}

// **오류가 글자가 되는 자리는 noticeText 하나다.**
//
// 무엇을 하려 했는지는 센티넬이, 경로와 까닭은 `*os.PathError` 가 들고 온다. 어느 쪽도
// 문자열로 미리 지어 나르지 않는다(ADR-0053).
func TestNoticeTextJoinsSentinelPathAndCause(t *testing.T) {
	dir := t.TempDir()

	t.Run("열기 실패", func(t *testing.T) {
		_, err := OpenBuffer(dir) // 디렉터리는 읽을 수 없다

		require.Error(t, err)
		assert.Equal(t, "열 수 없습니다: "+dir+": is a directory", noticeText(err))
	})

	t.Run("쓰기 실패", func(t *testing.T) {
		locked := filepath.Join(dir, "locked.txt")
		require.NoError(t, os.WriteFile(locked, []byte("x"), 0444))

		buf, err := OpenBuffer(locked)
		require.NoError(t, err)

		_, err = buf.SaveForce(wide)

		require.Error(t, err)
		assert.Equal(t, "쓸 수 없습니다: "+locked+": permission denied", noticeText(err))
	})

	t.Run("표시가 없으면 까닭만", func(t *testing.T) {
		assert.Equal(t, "무언가 잘못됐다", noticeText(errors.New("무언가 잘못됐다")))
	})
}

// `Op` 은 보지 않는다. 쓰기가 권한으로 막히면 os 가 `Op` 을 `open` 으로 준다 —
// 그것으로 동사를 고르면 `:w` 실패가 「열 수 없습니다」가 된다.
func TestNoticeTextIgnoresSyscallOp(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.txt")
	require.NoError(t, os.WriteFile(locked, []byte("x"), 0444))

	raw := os.WriteFile(locked, []byte("y"), 0644)
	require.Error(t, raw)

	var pathErr *os.PathError
	require.ErrorAs(t, raw, &pathErr)
	require.Equal(t, "open", pathErr.Op, "os 는 쓰기 실패에도 open 이라 한다")

	assert.Equal(t, "쓸 수 없습니다: "+locked+": permission denied",
		noticeText(errors.Mark(raw, errWriteFile)))
}

// 센티넬을 붙여도 구조가 그대로 남는다. 붙이는 것이 감싸는 것과 다른 점이다.
func TestMarkKeepsStructure(t *testing.T) {
	_, raw := os.ReadFile(t.TempDir())
	marked := errors.Mark(raw, errOpenFile)

	assert.True(t, errors.Is(marked, errOpenFile))
	assert.False(t, errors.Is(marked, errWriteFile))

	var pathErr *os.PathError
	assert.True(t, errors.As(marked, &pathErr), "PathError 가 살아 있다")
}

// 지우는 것은 알림이 아니다. 아래 줄만 비고 기록은 그대로 남는다.
func TestClearNoticeKeepsRecord(t *testing.T) {
	e := &editor{}

	e.notify("12 글자 복사되었습니다")
	e.clearNotice()

	assert.Empty(t, e.notice)
	assert.Len(t, e.notices, 1, "지웠다고 기록에서 사라지지 않는다")
}

// 같은 문구가 되풀이되면 되풀이된 만큼 쌓인다.
//
// 끝난 작업 목록이 이름당 하나만 남기는 것(job.go 의 finishJob) 과 규칙이 정반대다 —
// 여기서는 같은 알림이 몇 번 났는지가 정보다(ADR-0053).
func TestNoticesDoNotFold(t *testing.T) {
	e := &editor{}

	for range 100 {
		e.notifyFailure("git 상태 실패: exit status 128")
	}

	assert.Len(t, e.notices, 100)
}

// 아래 줄에 뜨는 모든 길이 기록을 지나는지 본다. 깔때기를 지나지 않는 길이 생기면
// 그 알림은 조용히 기록에서 빠진다.
func TestEveryNoticePathRecords(t *testing.T) {
	t.Run("normalModeMessage", func(t *testing.T) {
		e := &editor{buffers: []Buffer{newEmptyBuffer("")}}

		normalModeMessage(e, "알 수 없는 명령: :xyz")

		assert.Equal(t, "알 수 없는 명령: :xyz", e.notice)
		assert.Len(t, e.notices, 1)
	})

	t.Run("normalModeError", func(t *testing.T) {
		e := &editor{buffers: []Buffer{newEmptyBuffer("")}}

		normalModeError(e, errors.New("no such file"))

		assert.Equal(t, "no such file", e.notice)
		require.Len(t, e.notices, 1)
		assert.True(t, e.notices[0].failed)
	})

	t.Run("refuseReadOnly", func(t *testing.T) {
		buf := newEmptyBuffer("a.txt")
		buf.readOnly = true

		e := &editor{buffers: []Buffer{buf}}

		require.True(t, e.refuseReadOnly())
		assert.Len(t, e.notices, 1)
	})
}

// 백그라운드 작업의 실패가 기록에 남는다. **이것이 메세지 센터의 첫 표적이다** —
// 사람이 키를 누르지 않았는데 도착해서 다음 키에 사라지던 알림이다(ADR-0053).
func TestBackgroundFailureIsRecorded(t *testing.T) {
	e := &editor{}
	e.putJob(job{name: "git 상태"})
	e.jobs[0].err = errors.New("exit status 128")

	e.finishJob("git 상태")

	require.Len(t, e.notices, 1)
	assert.True(t, e.notices[0].failed)
	assert.Contains(t, e.notices[0].text, "git 상태 실패")
	assert.Contains(t, e.notices[0].text, "exit status 128")
}

// 취소는 알리지 않으므로 기록에도 남지 않는다. 그만하라고 한 사람이 결과를 이미 안다.
func TestCancelledJobIsNotRecorded(t *testing.T) {
	e := &editor{}
	e.putJob(job{name: "파일 인덱싱"})
	e.cancelJob("파일 인덱싱")

	e.finishJob("파일 인덱싱")

	assert.Empty(t, e.notice)
	assert.Empty(t, e.notices)
}
