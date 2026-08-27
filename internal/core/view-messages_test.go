package core

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// messagesFixture 는 알림 몇 개가 쌓인 목록 화면이다.
func messagesFixture(t *testing.T, texts ...string) viewMessages {
	t.Helper()

	e := &editor{
		buffers: []Buffer{newEmptyBuffer("a.txt")},
		width:   80,
		height:  20,
	}

	for _, text := range texts {
		e.notify(text)
	}

	model, _ := messagesMode(e)

	return model.(viewMessages)
}

// 열면 맨 아래(가장 최근) 가 고른 채로 시작한다. 찾는 것은 대개 방금 지나간 것이다.
func TestMessagesOpensAtBottom(t *testing.T) {
	m := messagesFixture(t, "하나", "둘", "셋")

	assert.Equal(t, 2, m.selected)
}

// 열면 아래 줄이 빈다. 그 알림은 이미 목록의 마지막 줄이라 두 번 보일 이유가 없다.
// 기록은 그대로다.
func TestMessagesClearsBottomLine(t *testing.T) {
	m := messagesFixture(t, "저장함: a.txt")

	assert.Empty(t, m.notice)
	assert.Len(t, m.notices, 1)
}

func TestMessagesMoves(t *testing.T) {
	m := messagesFixture(t, "하나", "둘", "셋")

	next, _ := m.press("k")
	m = next.(viewMessages)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("k")
	m = next.(viewMessages)
	assert.Equal(t, 0, m.selected)

	// 양끝에서 멈춘다. 둘러 가지 않는다.
	next, _ = m.press("k")
	m = next.(viewMessages)
	assert.Equal(t, 0, m.selected)

	next, _ = m.press("j")
	m = next.(viewMessages)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("G")
	m = next.(viewMessages)
	assert.Equal(t, 2, m.selected)

	next, _ = m.press("g")
	m = next.(viewMessages)
	assert.Equal(t, 0, m.selected)
}

// 한글 상태로 쳐도 움직인다. 입력줄이 없는 화면이라 파서가 되돌린다(ADR-0008).
func TestMessagesMovesInHangul(t *testing.T) {
	m := messagesFixture(t, "하나", "둘", "셋")

	next, _ := m.press("ㅏ") // k
	m = next.(viewMessages)
	assert.Equal(t, 1, m.selected)

	next, _ = m.press("ㅓ") // j
	m = next.(viewMessages)
	assert.Equal(t, 2, m.selected)
}

func TestMessagesLeaves(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		t.Run(key, func(t *testing.T) {
			m := messagesFixture(t, "하나")

			model, _ := m.press(key)

			assert.IsType(t, viewEditorNormal{}, model)
		})
	}
}

// 모르는 키는 아무 일도 하지 않는다. 지우거나 고치는 키는 없다.
func TestMessagesIgnoresUnknownKeys(t *testing.T) {
	m := messagesFixture(t, "하나", "둘")

	for _, key := range []string{"x", "d", "enter", "i"} {
		model, cmd := m.press(key)

		require.IsType(t, viewMessages{}, model)
		assert.Nil(t, cmd, key)
		assert.Len(t, model.(viewMessages).notices, 2, key)
	}
}

// **이 화면의 핵심 시험이다.** 위쪽을 보고 있는데 새 알림이 도착해도 고른 줄이 다른
// 알림으로 바뀌지 않는다. 뒤에만 붙기 때문이다(ADR-0053).
func TestMessagesKeepsSelectionWhenNoticeArrives(t *testing.T) {
	m := messagesFixture(t, "하나", "둘", "셋", "넷", "다섯")

	next, _ := m.press("g")
	m = next.(viewMessages)
	require.Equal(t, 0, m.selected)

	// 백그라운드 작업이 실패해 알림이 도착한다. 이 화면을 보고 있는 채로 온다.
	m.putJob(job{name: "git 상태"})
	m.jobs[0].err = errors.New("exit status 128")

	model, _ := m.Update(jobDoneMsg{name: "git 상태"})
	m = model.(viewMessages)

	assert.Len(t, m.notices, 6, "목록이 늘었다")
	assert.Equal(t, 0, m.selected, "고른 자리는 그대로다")
	assert.Equal(t, "하나", m.notices[m.selected].text, "가리키는 알림도 그대로다")
}

// **끝에서 둘째 줄을 고르고 있어도 끌려가지 않는다.**
//
// 「맨 아래였나」를 늘어난 개수로 되짚으면 그 짐작이 이 줄까지 삼킨다. 실제로 그렇게 짜서
// 목록이 눈앞에서 한 칸씩 내려가는 것을 봤다 — 알림이 늘지 않는 job msg 에도 불리기 때문이다.
func TestMessagesKeepsSelectionNextToBottom(t *testing.T) {
	m := messagesFixture(t, "하나", "둘")

	next, _ := m.press("k")
	m = next.(viewMessages)
	require.Equal(t, 0, m.selected, "끝에서 둘째 줄이자 첫 줄이다")

	m.putJob(job{name: "git 상태"})
	m.jobs[0].err = errors.New("exit status 128")

	model, _ := m.Update(jobDoneMsg{name: "git 상태"})
	m = model.(viewMessages)

	require.Len(t, m.notices, 3)
	assert.Equal(t, 0, m.selected, "고른 자리는 그대로다")
}

// 알림이 늘지 않는 msg 는 고른 자리를 건드리지 않는다. tick 은 5 초마다 온다 —
// 한 번에 한 칸씩 끌려가면 목록을 읽는 동안 커서가 저절로 아래로 흘러간다.
func TestMessagesIgnoresMsgsThatAddNothing(t *testing.T) {
	m := messagesFixture(t, "하나", "둘", "셋")

	next, _ := m.press("k")
	m = next.(viewMessages)
	require.Equal(t, 1, m.selected)

	for range 5 {
		model, _ := m.Update(gitTickMsg(time.Now()))
		m = model.(viewMessages)
	}

	require.Len(t, m.notices, 3, "알림이 늘지 않았다")
	assert.Equal(t, 1, m.selected, "커서가 흘러가지 않는다")
}

// 맨 아래를 보고 있었으면 새 알림을 따라 내려간다. 「지금 오는 것을 보는 중」이라는 뜻이다.
func TestMessagesFollowsWhenAtBottom(t *testing.T) {
	m := messagesFixture(t, "하나", "둘")
	require.Equal(t, 1, m.selected)

	m.putJob(job{name: "git 상태"})
	m.jobs[0].err = errors.New("exit status 128")

	model, _ := m.Update(jobDoneMsg{name: "git 상태"})
	m = model.(viewMessages)

	require.Len(t, m.notices, 3)
	assert.Equal(t, 2, m.selected, "새 줄을 따라간다")
}

// 목록이 화면보다 길면 고른 자리를 따라 스크롤한다.
func TestMessagesScrolls(t *testing.T) {
	texts := make([]string, 0, 40)
	for i := range 40 {
		texts = append(texts, strings.Repeat("가", 1)+string(rune('a'+i%26)))
	}

	m := messagesFixture(t, texts...)

	height := m.listHeight()
	require.Greater(t, height, 0)
	require.Less(t, height, len(texts), "화면보다 목록이 길어야 재는 뜻이 있다")

	// 맨 아래에서 열었으므로 마지막 화면이 보인다.
	assert.Equal(t, len(texts)-height, m.top)

	next, _ := m.press("g")
	m = next.(viewMessages)
	assert.Equal(t, 0, m.top)
}

// 시각과 갈래가 보인다. 갈래는 색만이 아니라 글자로도 나타난다 —
// 화면을 글자로 떠서 보는 길이 있어서다.
func TestMessagesShowsClockAndKind(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.txt")}, width: 80, height: 20}
	e.notify("저장함: a.txt")
	e.notifyFailure("git 상태 실패: exit status 128")

	model, _ := messagesMode(e)
	content := ansi.Strip(model.(viewMessages).View().Content)

	assert.Contains(t, content, "알림  2 개 · 실패 1")
	assert.Contains(t, content, "저장함: a.txt")
	assert.Contains(t, content, "! git 상태 실패: exit status 128")
	assert.NotContains(t, content, "! 저장함", "실패가 아닌 것에는 표시가 없다")
	assert.Contains(t, content, "MESSAGES")
	assert.Contains(t, content, "g/G 처음·끝")

	// 시각은 여덟 칸이다.
	assert.Regexp(t, `\d\d:\d\d:\d\d`, content)
}

func TestMessagesEmpty(t *testing.T) {
	m := messagesFixture(t)

	content := ansi.Strip(m.View().Content)

	assert.Contains(t, content, "알림  0 개")
	assert.NotContains(t, content, "· 실패", "실패가 없으면 적지 않는다")
	assert.Contains(t, content, "지나간 알림이 없습니다")
	assert.Nil(t, m.View().Cursor)
}

// `f` 는 실패만 남긴다. 제목줄의 개수는 담긴 것 전부다.
func TestMessagesFiltersFailures(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.txt")}, width: 80, height: 20}
	e.notify("저장함: a.txt")
	e.notifyFailure("git 상태 실패")
	e.notify("저장함: b.txt")

	model, _ := messagesMode(e)

	filtered, _ := model.(viewMessages).press("f")
	m := filtered.(viewMessages)

	require.Len(t, m.rows(), 1)
	assert.Equal(t, 0, m.selected, "거른 목록의 맨 아래로 간다")

	content := ansi.Strip(m.View().Content)
	assert.Contains(t, content, "git 상태 실패")
	assert.NotContains(t, content, "저장함", "실패가 아닌 것은 빠진다")
	assert.Contains(t, content, "알림  3 개 · 실패 1 · 실패만", "제목은 담긴 것 전부를 센다")
	assert.Contains(t, content, "f 전부", "안내는 누르면 무엇이 되는지를 적는다")

	// 다시 누르면 전부로 돌아가고 맨 아래에 선다.
	back, _ := m.press("f")
	m = back.(viewMessages)

	assert.Len(t, m.rows(), 3)
	assert.Equal(t, 2, m.selected)
	assert.Contains(t, ansi.Strip(m.View().Content), "f 실패만")
}

// 실패가 하나도 없는데 거르면 그 사실을 적는다. 「지나간 알림이 없습니다」로는
// 기록이 사라진 것처럼 읽힌다.
func TestMessagesFilterEmptyReason(t *testing.T) {
	m := messagesFixture(t, "하나", "둘")

	filtered, _ := m.press("f")

	content := ansi.Strip(filtered.(viewMessages).View().Content)

	assert.Contains(t, content, "실패한 알림이 없습니다")
	assert.Contains(t, content, "알림  2 개 · 실패만")
	assert.Nil(t, filtered.(viewMessages).View().Cursor, "가리킬 줄이 없다")
}

func TestMessagesCursorOnSelectedRow(t *testing.T) {
	m := messagesFixture(t, "하나", "둘", "셋")

	view := m.View()

	require.NotNil(t, view.Cursor)
	assert.Equal(t, tea.CursorBlock, view.Cursor.Shape)
	assert.Equal(t, m.selected-m.top+jobsTitleHeight, view.Cursor.Y)
}

// 트리를 열어 둔 채로 와도 statusBar 가 32 칸 들여쓰이지 않는다. 이 화면이 트리를 덮는다.
func TestMessagesStatusBarIsBare(t *testing.T) {
	e := &editor{
		buffers: []Buffer{newEmptyBuffer("a.txt")},
		sidebar: openSidebar("/"),
		width:   100,
		height:  20,
	}
	e.notify("하나")

	model, _ := messagesMode(e)
	rows := strings.Split(ansi.Strip(model.(viewMessages).View().Content), "\n")

	assert.True(t, strings.HasPrefix(rows[len(rows)-statusBarHeight], "MESSAGES"),
		"statusBar 가 왼쪽 끝에서 시작한다")
}

// 긴 알림은 오른쪽부터 접힌다. 앞머리가 무엇이 일어났는지다.
func TestTrimTextRight(t *testing.T) {
	text := "cannot write /Users/bluemir/src/bluemir/zn/internal/core/editor.go: permission denied"

	assert.Equal(t, text, trimTextRight(text, 200), "들어가면 그대로 둔다")

	short := trimTextRight(text, 20)
	assert.LessOrEqual(t, screenWidthOf(short), 20)
	assert.True(t, strings.HasPrefix(short, "cannot write"), "앞머리가 남아야 한다")
	assert.True(t, strings.HasSuffix(short, "…"))

	// 한글도 칸으로 센다. 두 칸짜리 글자가 경계에 걸려도 넘치지 않는다.
	korean := trimTextRight("가나다라마바사아자차카타파하", 11)
	assert.LessOrEqual(t, screenWidthOf(korean), 11)
	assert.True(t, strings.HasPrefix(korean, "가나다"))

	assert.Equal(t, "", trimTextRight(text, 0))
}

func TestFormatClock(t *testing.T) {
	at := time.Date(2026, 8, 23, 14, 3, 5, 0, time.Local)

	assert.Equal(t, "14:03:05", formatClock(at))
	assert.Len(t, formatClock(at), 8, "늘 여덟 칸이라 세로 줄이 맞는다")
}

// `:messages` 와 `:mes` 가 목록을 연다. vim 이 줄임말을 받는 것과 같다.
func TestMessagesCommand(t *testing.T) {
	for _, name := range []string{"messages", "mes"} {
		t.Run(name, func(t *testing.T) {
			e := &editor{buffers: []Buffer{newEmptyBuffer("a.txt")}, width: 80, height: 20}
			e.notify("하나")

			model := runCommand(viewEditorNormal{editor: e}, name)

			assert.IsType(t, viewMessages{}, model)
		})
	}
}

// 인자나 줄 범위를 받지 않는다. `:jobs` 와 같은 자리에 걸린다.
func TestMessagesCommandRefusesArgsAndRange(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.txt")}, width: 300, height: 20}

	model := runCommand(viewEditorNormal{editor: e}, "messages foo")
	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "알 수 없는 명령")

	model = runCommand(viewEditorNormal{editor: e}, "1,5messages")
	assert.IsType(t, viewEditorNormal{}, model)
	assert.Contains(t, e.notice, "줄 범위를 받지 않습니다")
}

// 팔레트에서도 연다. 「작업 목록」과 같은 자리다.
func TestMessagesInPalette(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.txt")}, width: 80, height: 20}
	e.notify("하나")

	model, _ := runMessages(e)

	assert.IsType(t, viewMessages{}, model)
}

// 이벤트 로그가 mode 이름을 안다. 모르면 `core.viewMessages` 로 찍힌다(ADR-0050).
func TestMessagesModeName(t *testing.T) {
	e := &editor{buffers: []Buffer{newEmptyBuffer("a.txt")}, width: 80, height: 20}

	messages, _ := messagesMode(e)
	assert.Equal(t, "MESSAGES", modeName(messages))

	locations, _ := locationsMode(e, "정의 후보", nil)
	assert.Equal(t, "GOTO", modeName(locations), "GOTO 도 등록되어 있어야 한다")
}
