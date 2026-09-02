package core

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/sirupsen/logrus"
)

// 이 파일은 편집기로 들어오는 이벤트를 있는 그대로 기록한다. 화면에는 아무것도 안 그린다.
//
// **한글 입력기가 얽힌 버그를 잡으려고 둔다.** 터미널과 입력기가 무엇을 언제 보내는지는
// 편집기 밖의 일이라, 증상만 보고는 조합이 늦게 온 것인지·입력기가 지우려고 backspace 를
// 보낸 것인지·키도 없이 다시 그리다 조합이 지워진 것인지 가릴 수 없다. 재현 경로를 모르는
// 버그가 여럿 쌓여 있어서(docs/tasks.md) 한 번 재현할 때 그 순서가 통째로 남는 자리가 필요하다.
//
// `internal/core` 에서 로그를 내는 것은 이것이 처음이고, 편집기가 도는 동안 stderr 는 화면이
// 차지하고 있어서 함부로 낼 수 없다. 적을 곳이 없으면 버리는 것이 그 답이다(ADR-0050).

// traceEvent 는 bubbletea 가 msg 를 처리하기 전에 지나는 자리다(tea.WithFilter).
//
// **받은 것을 언제나 그대로 돌려준다.** nil 을 돌려주면 그 msg 가 사라진다 — 로그를 켜고
// 끄는 것이 편집기의 동작을 바꾸면 안 된다.
//
// Program 을 만드는 자리가 core.Run 하나뿐이라 여기 하나로 모든 mode 의 이벤트가 모인다.
// mode 마다 손대면 빠뜨린 mode 에서 고리가 끊긴다(ADR-0038 이 tick 에서 겪은 일이다).
func traceEvent(model tea.Model, msg tea.Msg) tea.Msg {
	// 꺼져 있으면 문자열을 짓지 않는다. 키마다 도는 자리다.
	if !logrus.IsLevelEnabled(logrus.DebugLevel) {
		return msg
	}

	if line := traceLine(model, msg); line != "" {
		logrus.Debug(line)
	}

	return msg
}

// traceLine 은 이벤트 하나를 적을 한 줄이다. 적지 않는 갈래면 빈 문자열이다.
//
// **모든 msg 를 적지는 않는다.** 다 적으면 정작 봐야 할 것이 잡음에 파묻힌다. 적는 것은
// 둘뿐이다 — 사람이 친 것과, **사람이 아무것도 안 쳤는데 화면을 다시 그리게 만드는 것**.
// 뒤엣것이 없으면 「손을 안 댔는데 글자가 사라졌다」를 로그에서 확인할 길이 없다.
func traceLine(model tea.Model, msg tea.Msg) string {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return fmt.Sprintf("key press  %s mode=%s%s",
			traceKey(tea.Key(msg)), modeName(model), traceBuffer(model))
	case tea.KeyReleaseMsg:
		return fmt.Sprintf("key release %s mode=%s", traceKey(tea.Key(msg)), modeName(model))

	// 입력기가 글자를 붙여넣기로 흘리는 터미널이 있다.
	case tea.PasteStartMsg:
		return "paste start"
	case tea.PasteMsg:
		return fmt.Sprintf("paste      %s", traceText(msg.Content))
	case tea.PasteEndMsg:
		return "paste end"

	// 터미널이 요청을 들어줬는지다. 이 msg 를 받는 자리가 여기 말고는 없어서, 지금 편집기는
	// BaseCode 를 받을 수 있는지조차 모른다(ADR-0014).
	case tea.KeyboardEnhancementsMsg:
		return fmt.Sprintf("keyboard enhancements %+v", msg)

	// 아래부터는 **키 없이** 화면을 다시 그리게 하는 것들이다.
	//
	// 조합 중인 글자는 편집기의 buffer 에 없고 터미널이 화면에만 그려 준다(ADR-0008).
	// 그 사이에 다시 그리면 그 자리가 덮인다. tick 이 5 초마다 둘씩 도므로(git.go, outside.go)
	// 손을 안 대도 일어난다 — 그것을 로그에서 보려고 적는다.
	case gitTickMsg:
		return "tick git" + traceBuffer(model)
	case fileTickMsg:
		return "tick file" + traceBuffer(model)

	// 감시기가 무언가를 잡았다. tick 과 달리 **밖에서 실제로 일이 일어났을 때만** 오므로,
	// 손을 대지 않았는데 이것이 쏟아지면 무엇이 그 저장소를 만지고 있다는 뜻이다(watch.go).
	case watchMsg:
		return "watch" + traceBuffer(model)
	case jobProgressMsg:
		return "job progress"
	case jobDoneMsg:
		return "job done"
	case tea.WindowSizeMsg:
		return fmt.Sprintf("resize     %dx%d", msg.Width, msg.Height)
	case tea.FocusMsg:
		return "focus"
	case tea.BlurMsg:
		return "blur"
	}

	return ""
}

// traceCursor 는 로그에서 커서 자리를 가리키는 표시다.
//
// 글자가 아닌 것을 골랐다. 파일에 들어 있을 만한 글자면 그것이 표시인지 내용인지 헷갈린다.
const traceCursor = "‸" // U+2038 CARET

// traceBuffer 는 커서가 선 자리와 그 줄이다. 줄 안의 커서 자리는 traceCursor 로 끼워 넣는다.
//
// **이 키가 처리되기 _전_ 상태다.** filter 는 bubbletea 가 msg 를 넘기기 전에 지나므로
// 여기서 보는 것은 아직 그 키가 닿지 않은 buffer 다. 그래서 어떤 키가 무엇을 바꿨는지는
// **다음 줄과 견주어** 읽는다.
//
// 그래서 tick 줄에도 붙인다. 마지막으로 누른 키의 결과를 보려고 키를 하나 더 누르게 하면,
// 그 키가 또 무언가를 바꿔서 정작 보려던 것이 묻힌다. tick 은 5 초마다 저절로 오므로
// **가만히 있어도** 결과가 남는다 (git.go, outside.go).
//
// 자리만으로는 모자라서 줄 내용을 통째로 남긴다. 「빈칸에서 지웠는데 옆 글자가 없어졌다」
// 같은 것은 무엇이 지워졌는지를 앞뒤로 견주어야만 가릴 수 있다.
func traceBuffer(model tea.Model) string {
	// hasTab 을 같이 묻는다. mode 를 가리지 않고 buffer 를 들여다보는 자리라, 빈 화면
	// model 도 editor 를 embed 해서 activeBuffer 를 만족해 버린다(ADR-0064).
	holder, ok := model.(interface {
		hasTab() bool
		activeBuffer() *viewport
	})
	if !ok || !holder.hasTab() {
		return ""
	}

	buf := holder.activeBuffer()
	if buf.Cursor.Line < 0 || buf.Cursor.Line >= len(buf.Lines) {
		return ""
	}

	line := buf.Lines[buf.Cursor.Line]
	at := min(max(buf.Cursor.Col, 0), len(line))

	return fmt.Sprintf(" cur=%d:%d screen=%d line=%q",
		buf.Cursor.Line+1, at, screenColAt(line, at, buf.TabWidth()),
		string(line[:at])+traceCursor+string(line[at:]))
}

// traceKey 는 키 하나가 들고 온 것 전부다.
//
// **Text 와 Code 를 따로 적는다.** 입력기가 낀 자리에서 둘이 갈린다. String() 은 Text 를
// 앞세우고 Keystroke() 는 BaseCode 를 쓰므로(ADR-0014) 그 둘이 갈리는 순간이 곧 입력기가
// 끼어든 순간이다.
//
// BaseCode 는 escape code 로 오는 키(modifier 붙은 것) 에만 붙는다. 평범한 글자 키는 그냥
// 글자로 와서 비어 있다 — `ctrl+ㅔ` 에는 `p` 가 붙고 `ㅁ` 에는 없다. **비어 있다는 것도
// 적어야 알 수 있다.**
func traceKey(key tea.Key) string {
	return fmt.Sprintf("text=%s code=%s shifted=%s base=%s mod=%s str=%q stroke=%q",
		traceText(key.Text),
		traceRune(key.Code),
		traceRune(key.ShiftedCode),
		traceRune(key.BaseCode),
		traceModifiers(key.Mod),
		tea.KeyPressMsg(key).String(),
		tea.KeyPressMsg(key).Keystroke(),
	)
}

// traceText 는 글자와 그 코드포인트다. 비면 `-` 다.
//
// 코드포인트를 같이 적는 이유는 눈으로 같은 글자가 다른 값일 수 있어서다 — 조합해 놓은
// `ㅁ`(U+3141, 호환 자모) 과 음절을 이루는 `ᄆ`(U+1106) 은 화면에서 가리기 어렵다.
func traceText(text string) string {
	if text == "" {
		return "-"
	}

	points := make([]string, 0, len(text))
	for _, r := range text {
		points = append(points, fmt.Sprintf("U+%04X", r))
	}

	return fmt.Sprintf("%q[%s]", text, strings.Join(points, " "))
}

// traceRune 은 글자 하나와 그 코드포인트다. 0 이면 `-` 다 — 그 칸이 안 채워졌다는 뜻이다.
func traceRune(r rune) string {
	switch {
	case r == 0:
		return "-"
	case unicode.IsPrint(r):
		return fmt.Sprintf("%q(U+%04X)", r, r)
	default:
		return fmt.Sprintf("U+%04X", r)
	}
}

// traceModifierNames 는 KeyMod 의 각 자리 이름이다.
//
// 손으로 든다. `KeyMod` 에는 String() 이 없고 Contains 뿐이다.
var traceModifierNames = []struct {
	mod  tea.KeyMod
	name string
}{
	{tea.ModShift, "shift"},
	{tea.ModAlt, "alt"},
	{tea.ModCtrl, "ctrl"},
	{tea.ModMeta, "meta"},
	{tea.ModHyper, "hyper"},
	{tea.ModSuper, "super"},
	{tea.ModCapsLock, "caps"},
	{tea.ModNumLock, "num"},
	{tea.ModScrollLock, "scroll"},
}

// traceModifiers 는 눌린 modifier 들의 이름이다. 없으면 `-` 다.
func traceModifiers(mod tea.KeyMod) string {
	if mod == 0 {
		return "-"
	}

	names := make([]string, 0, len(traceModifierNames))
	for _, m := range traceModifierNames {
		if mod.Contains(m.mod) {
			names = append(names, m.name)
		}
	}

	if len(names) < 1 {
		// 이름을 모르는 자리가 켜졌다. 값이라도 남긴다.
		return fmt.Sprintf("%#b", int(mod))
	}

	return strings.Join(names, "+")
}

// modeName 은 지금 mode 의 이름이다. statusBar 에 뜨는 것과 같은 말로 적는다.
//
// **type 으로 가른다.** model 은 값으로 오므로 포인터가 아니라 값으로 단정한다.
//
// 공통 메서드를 두지 않는다. ADR-0002 가 「밖에서 지금 어느 mode 냐를 묻기 어렵다」를 일부러
// 그렇게 두었고, 진단 하나 때문에 그 설계를 뒤집을 일이 아니다. 대신 빠뜨린 type 은 `%T` 로
// 그대로 드러나서 조용히 사라지지 않는다.
func modeName(model tea.Model) string {
	switch model.(type) {
	case viewEditorNormal:
		return "NORMAL"
	case viewEditorInsert:
		return "INSERT"
	case viewEditorVisual:
		return "VISUAL"
	case viewEditorCommand:
		return "COMMAND"
	case viewEditorSearch:
		return "SEARCH"
	case viewSidebar:
		return "TREE"
	case viewPalette:
		return "PALETTE"
	case viewJobs:
		return "JOBS"
	case viewLocations:
		return "GOTO"
	case viewMessages:
		return "MESSAGES"
	case viewConfirmDiscard, viewServerInstallConfirm, viewFormatterInstallConfirm:
		return "CONFIRM"
	}

	return fmt.Sprintf("%T", model)
}
