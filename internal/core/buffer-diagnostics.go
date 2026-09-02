package core

import "slices"

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

// diagnosticLines 는 진단이 있는 줄들이다(ADR-0086).
//
// 이쪽은 묶지 않는다. 잇달아 선 오류는 저마다 다른 오류라, 한 줄씩 짚어 가며 고치는 것이
// 이 키를 쓰는 손이다.
func (buf *Buffer) diagnosticLines() []int {
	if len(buf.diagnostics) == 0 {
		return nil
	}

	lines := make([]int, 0, len(buf.diagnostics))
	for line := range buf.diagnostics {
		lines = append(lines, line)
	}

	slices.Sort(lines)

	return lines
}
