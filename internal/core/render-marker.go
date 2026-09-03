package core

import (
	"github.com/bluemir/zn/internal/textarea"
)

// 마커 글자다. 둘 다 East Asian Width 가 Neutral 이라 어느 터미널에서나 한 칸이다 —
// 공백 마커가 Ambiguous 인 `·` 를 버린 것과 같은 기준이다(ADR-0020, ADR-0025).
const (
	markerDiagnosticError   = "✖" // U+2716
	markerDiagnosticWarning = "⚠" // U+26A0
)

// renderDiagnosticMarker 는 마커 칸 한 칸이다. 진단이 없으면 빈 칸이다.
//
// 목록의 첫 것을 본다. 심한 것이 앞에 오도록 담았다(setDiagnostics).
//
// 정보·힌트(severity 3·4) 는 그리지 않는다. 잰 값으로 gopls 는 보내지 않았고(ADR-0086),
// 온다면 그것은 「고쳐야 할 것」이 아니라 곁말이라 오류와 같은 칸에 설 것이 아니다.
// 칸은 그대로 비므로 그것이 와도 화면이 밀리지 않는다.
func renderDiagnosticMarker(list []textarea.Diagnostic) string {
	if len(list) == 0 {
		return " "
	}

	switch list[0].Severity {
	case textarea.SeverityError:
		return styleDiagnosticError.Render(markerDiagnosticError)
	case textarea.SeverityWarning:
		return styleDiagnosticWarning.Render(markerDiagnosticWarning)
	}

	return " "
}

// 마커 글자다. 셋 다 ASCII 라 어느 터미널에서나 한 칸이다 — 진단 마커가 East Asian Width 가
// Neutral 인 글자만 쓴 것과 같은 기준이다(ADR-0020, ADR-0086 §4).
const (
	markerGitAdded    = "+"
	markerGitModified = "~"
	markerGitRemoved  = "_"
)

// renderGitMarker 는 마커 칸 한 칸이다. 변경이 없으면 빈 칸이다.
func renderGitMarker(mark textarea.GitLineMark) string {
	switch mark {
	case textarea.GitLineAdded:
		return styleGitAdded.Render(markerGitAdded)
	case textarea.GitLineModified:
		return styleGitModified.Render(markerGitModified)
	case textarea.GitLineRemoved:
		return styleGitRemoved.Render(markerGitRemoved)
	}

	return " "
}
