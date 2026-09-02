package core

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/bluemir/zn/internal/lsp"
)

// 언어 서버가 밀어주는 진단(오류·경고) 을 화면에 올리는 자리다(ADR-0086).
//
// 서버 쪽 이야기는 internal/lsp 가 안다. 여기 있는 것은 「언제 받아오고 어디에 그리는가」다.
//
// 그리는 자리는 둘이다. 줄번호 왼쪽 마커 칸이 「이 줄에 있다」를 말하고(layout.go 의
// renderGutter), statusBar 아래 줄이 커서가 선 줄의 「무엇인지」를 말한다.

// 마커 글자다. 둘 다 East Asian Width 가 Neutral 이라 어느 터미널에서나 한 칸이다 —
// 공백 마커가 Ambiguous 인 `·` 를 버린 것과 같은 기준이다(ADR-0020, ADR-0025).
const (
	markerDiagnosticError   = "✖" // U+2716
	markerDiagnosticWarning = "⚠" // U+26A0
)

// diagnosticsMsg 는 진단이 바뀌었다는 것이다. **어느 파일의 무엇이** 바뀌었는지는 싣지
// 않는다 — 받은 자리에서 열려 있는 파일을 훑어 다시 맞춘다(applyDiagnostics).
//
// 어느 서버가 울렸는지는 싣는다. 고리를 그 서버에 다시 걸어야 하고, 서버가 여럿이라
// 종만 보고는 누구의 것인지 알 수 없다(ADR-0107).
type diagnosticsMsg struct {
	server string
}

// waitDiagnostics 는 종이 울릴 때까지 기다려 msg 로 바꾸는 Cmd 다.
//
// 받을 때마다 다시 발행해야 다음 것이 온다. 작업 진행 조각을 받는 것과 같은 고리다
// (job.go 의 waitJob).
//
// **고리는 서버마다 하나다.** 서버가 자기 때에 보내는 것이라 종도 서버마다 따로 울린다.
func waitDiagnostics(name string, client *lsp.Client) tea.Cmd {
	changed := client.DiagnosticsChanged()

	return func() tea.Msg {
		<-changed

		return diagnosticsMsg{server: name}
	}
}

// applyDiagnostics 는 열려 있는 파일들의 진단을 서버에서 읽어 buffer 에 담는다.
//
// **여기서도 무엇이 바뀌었는지 적어 두지 않는다.** 열려 있는 목록을 훑어 지금 상태로
// 다시 맞추는 것이 서버와 맞추는 자리에서 이미 쓰는 손이다(language-server.go 의 syncServers).
//
// 열지 않은 파일의 진단은 버린다. gopls 는 같은 패키지의 열지 않은 파일 것도 보내는데
// (잰 값이다) 지금 그것을 그릴 자리가 없다 — 담아 두면 아무도 읽지 않는 낡은 진단이
// 조용히 쌓인다(ADR-0086).
//
// **파일마다 자기 서버에게 묻는다.** 서버가 여럿이라 한 서버에 다 물으면 남의 언어 파일이
// 진단 없는 것으로 읽힌다.
// diagnostic 은 창이 드는 진단 하나다.
//
// **`lsp.Diagnostic` 을 그대로 담지 않는다.** 창은 마커에 무엇이 서고 아래 줄에 무엇이
// 적히는지만 알면 되고, 그것을 언어 서버가 만들었다는 것은 알 필요가 없다. 옮기는 것은
// editor 가 한다(applyDiagnostics) — 서버와 말을 섞는 것이 그쪽의 일이다 (ADR-0125).
//
// 열은 담지 않는다. 서버가 주는 열은 UTF-16 이라 byte 로 바꿔야 쓸 수 있는데(lsp/position.go),
// 지금 쓰는 자리 둘이 모두 줄까지만 본다. 본문에 밑줄을 긋게 되면 그때 더한다.
type diagnostic struct {
	line     int
	severity diagnosticSeverity
	message  string
}

// diagnosticSeverity 는 진단이 얼마나 심한지다. **작을수록 심하다** — LSP 의 눈금을 그대로
// 쓴다(1 오류, 2 경고, 3 정보, 4 힌트). 줄마다 앞에 오는 것을 고르는 데 이 순서를 쓴다.
type diagnosticSeverity int

const (
	severityError diagnosticSeverity = iota + 1
	severityWarning
	severityInfo
	severityHint
)

// diagnosticsOf 는 서버가 준 것을 창이 드는 자료로 옮긴다.
//
// **여기가 lsp 와 말을 섞는 마지막 자리다.** 이 아래로는 창의 것만 간다.
func diagnosticsOf(list []lsp.Diagnostic) []diagnostic {
	if len(list) == 0 {
		return nil
	}

	out := make([]diagnostic, 0, len(list))
	for _, item := range list {
		out = append(out, diagnostic{
			line:     item.Range.Start.Line,
			severity: diagnosticSeverity(item.Severity),
			message:  item.Message,
		})
	}

	return out
}

func (e *editor) applyDiagnostics() {
	for i := range e.buffers {
		server, path, ok := serverPath(e.buffers[i].path)
		if !ok {
			continue
		}

		client := e.clientOf(server)
		if client == nil {
			continue
		}

		e.buffers[i].setDiagnostics(diagnosticsOf(client.Diagnostics(path)))
	}
}

// setDiagnostics 는 그 파일의 진단을 줄별로 모아 담는다.
//
// 줄로 모으는 것은 읽는 쪽이 줄로 묻기 때문이다 — 마커는 행마다 그 줄을 묻고, 아래 줄은
// 커서 줄을 묻는다. 목록을 그대로 들고 있으면 그리는 행마다 목록 전체를 훑어야 한다.
//
// 열은 담지 않는다. 서버가 주는 열은 UTF-16 이라 byte 로 바꿔야 쓸 수 있는데(lsp/position.go),
// 지금 쓰는 자리 둘이 모두 줄까지만 본다. 본문에 밑줄을 긋게 되면 그때 바꾼다.
func (b *Buffer) setDiagnostics(list []diagnostic) {
	if len(list) == 0 {
		b.diagnostics = nil

		return
	}

	byLine := make(map[int][]diagnostic, len(list))
	for _, item := range list {
		byLine[item.line] = append(byLine[item.line], item)
	}

	// **줄마다 심한 것이 앞에 온다.** 마커 칸은 하나이고 아래 줄에 적는 문구도 하나라, 둘이
	// 각자 고르면 마커는 오류인데 문구는 경고인 화면이 나온다. 앞을 심한 것으로 맞춰 두면
	// 읽는 쪽 둘이 같이 `list[0]` 만 본다.
	//
	// 숫자가 작은 것이 심하다(diagnosticSeverity). 같은 갈래끼리는 서버가 준 순서, 곧 파일에
	// 나오는 순서를 지킨다.
	for line := range byLine {
		slices.SortStableFunc(byLine[line], func(a, b diagnostic) int {
			return int(a.severity) - int(b.severity)
		})
	}

	b.diagnostics = byLine
}

// diagnosticAt 은 그 줄의 진단들이다. 없으면 nil 이다.
//
// **줄이 어긋나 있을 수 있다.** 서버에 보내는 것은 마지막 키에서 250ms 뒤라(language-server.go 의
// editIdleDelay) 줄을 넣거나 지운 직후에는 진단이 낡은 판 기준이다. 그것을 보정하지 않고
// 그대로 두는 것이 결정이다 — 다음 publish 가 갈아치운다(ADR-0086).
func (b *Buffer) diagnosticAt(line int) []diagnostic {
	return b.diagnostics[line]
}

// renderDiagnosticMarker 는 마커 칸 한 칸이다. 진단이 없으면 빈 칸이다.
//
// 목록의 첫 것을 본다. 심한 것이 앞에 오도록 담았다(setDiagnostics).
//
// 정보·힌트(severity 3·4) 는 그리지 않는다. 잰 값으로 gopls 는 보내지 않았고(ADR-0086),
// 온다면 그것은 「고쳐야 할 것」이 아니라 곁말이라 오류와 같은 칸에 설 것이 아니다.
// 칸은 그대로 비므로 그것이 와도 화면이 밀리지 않는다.
func renderDiagnosticMarker(list []diagnostic) string {
	if len(list) == 0 {
		return " "
	}

	switch list[0].severity {
	case severityError:
		return styleDiagnosticError.Render(markerDiagnosticError)
	case severityWarning:
		return styleDiagnosticWarning.Render(markerDiagnosticWarning)
	}

	return " "
}

// renderDiagnostic 은 커서가 선 줄의 진단 한 줄이다. 없으면 빈 문자열이다.
//
// **커서 자리 뒤에 붙는다**(render-status-bar.go 의 renderPosition). 진단은 커서에 딸린
// 정보라 커서 위치와 성격이 같고, 붙여 두면 tip 이 남은 칸을 재서 알아서 물러난다
// (tip.go 의 renderWithTip). 알림이 뜨는 순간에는 알림이 그 줄을 통째로 쓰므로 이것이
// 가려진다 — 알림은 방금 한 일의 결과이고 진단은 가만히 있어도 되는 사실이다(ADR-0086).
//
// 여럿이면 첫 것과 개수다. 개수가 「더 있다」를 알려주어, 하나를 고치고 마커가 남은 까닭을
// 따로 묻지 않게 된다.
func (e editor) renderDiagnostic() string {
	buf := e.buffers[e.active]

	list := buf.diagnosticAt(buf.cursor.Line)
	if len(list) == 0 {
		return ""
	}

	// 마커와 같은 글자를 앞에 둔다. 위(마커 칸) 와 아래(이 줄) 가 같은 것을 말하고 있다는 것이
	// 색뿐 아니라 글자로도 이어진다.
	text := renderDiagnosticMarker(list) + " " + list[0].message

	if len(list) > 1 {
		text += " (" + strconv.Itoa(len(list)) + ")"
	}

	return text
}
