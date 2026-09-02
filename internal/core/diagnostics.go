package core

import (
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
