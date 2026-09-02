package core

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// renderStatusBar 는 화면 아래 두 줄을 그린다. 위 줄은 mode 와 파일과 git, 아래 줄은 부르는 쪽이 정한다.
//
// 위 줄만 반전이다. 아래 줄은 vim 처럼 명령줄이라 배경을 그대로 둔다.
// `:` 를 칠 때 배경이 뜨지 않고 명령 결과와 오류도 평범한 글자로 읽힌다.
//
// 줄은 sidebar 아래까지 화면 끝에서 끝까지 이어진다. 줄 자체는 tabline 과 달리 끊지 않는다 —
// 화면 맨 아래를 가로지르는 한 줄이라야 편집기 전체의 상태 표시로 읽힌다.
//
// 위 줄은 mode 가 sidebar 아래, 경로가 편집 영역 아래다. 둘 다 자기가 가리키는 것 바로 밑에
// 서게 된다. TREE 는 트리 아래에, 경로는 그 파일을 편집하는 자리 아래에 온다.
//
// sidebar 가 없으면 mode 를 놓을 왼쪽 칸 자체가 없으므로 경로 앞에 나란히 붙인다.
//
// 아래 줄은 mode 를 따라가지 않고 편집 영역에 맞춰 들여쓴다. 명령줄과 커서 위치는 편집 중인
// 파일에 딸린 것이라 위 줄의 경로와 세로로 맞아야 읽힌다.
func (e editor) renderStatusBar(mode, bottom string) []string {
	path := e.renderStatusPath()

	// 반전 안에 두어야 색이 왼쪽 끝까지 이어진다.
	// 자르는 것이 채우는 것보다 먼저다 — 두 칸짜리 글자가 경계에 걸치면 통째로 버려진다.
	left, text := "", mode+"  "+path
	if e.sidebarVisible() {
		label := truncateToWidth(mode, sidebarWidth)
		left = label + strings.Repeat(" ", max(0, sidebarWidth-widthOf(label)))
		text = path
	}

	width := e.textWidth()

	// Width 가 남은 칸을 공백으로 채워서 줄 끝까지 색이 간다.
	return []string{
		reverse.Width(e.width).Render(left + e.renderWithStatus(truncateToWidth(text, width))),
		strings.Repeat(" ", e.sidebarLeft()) + truncateToWidth(bottom, width),
	}
}

// renderStatusPath 는 statusBar 위 줄에서 mode 다음에 오는 자리다. 보고 있는 파일과
// 그것에 딸린 표시들이다.
//
// **tab 이 없으면 빈 문자열이다.** `[No Name]` 을 적지 않는다 — 그것은 이름 없는 tab 의
// 이름이라(tabline 의 tabName), 여기 적으면 「tab 이 없다」 와 「이름 없는 tab 이 하나
// 있다」 가 사람이 보는 유일한 자리에서 같은 글자가 된다(ADR-0064).
//
// git 은 여기 오지 않는다. 그것은 저장소 이야기라 보고 있는 파일과 무관하고, 그래서
// 빈 화면에서도 남는다(ADR-0009).
func (e editor) renderStatusPath() string {
	if !e.hasTab() {
		return ""
	}

	buf := e.buffers[e.active]

	path := buf.Path
	if path == "" {
		path = "[No Name]"
	}
	if buf.Dirty {
		path += " [+]"
	}

	// `[!]` 는 마지막으로 맞춰 봤을 때 바깥이 달라져 있었다는 것이다. `[+]` 가 내 손의 미저장
	// 변경이고 이것은 남의 변경이라, 둘이 같이 붙으면 양쪽에 잃을 것이 있다는 뜻이다(ADR-0031).
	if buf.OutsideState() != outsideSame {
		path += " [!]"
	}

	// 읽기 전용은 잃을 것이 있다는 표시가 아니라 애초에 고칠 수 없다는 것이다. 정의로 뛰어
	// 열린 표준 라이브러리·의존 모듈의 파일이 이것이다(ADR-0051).
	if buf.ReadOnly {
		path += " [읽기 전용]"
	}

	return path
}

// renderWithStatus 는 statusBar 위 줄 오른쪽 끝에 진행 표시와 저장소 상태를 붙인다.
//
// 붙일 칸이 없으면 그대로 둔다 — 지금 무슨 mode 인지와 어느 파일인지가 먼저다.
// 사이를 두 칸 이상 띄운다. 한 칸이면 파일 이름이 긴 tab 에서 경로에 붙은 글자처럼 읽힌다.
//
// 오른쪽 끝이 git 이고 그 왼쪽이 진행 표시다. 칸이 모자라면 진행 표시부터 줄인다 —
// 막대를 떼고, 그래도 모자라면 진행 표시를 통째로 뺀다. git 은 늘 같은 자리에 있어야 눈이 찾는다.
func (e editor) renderWithStatus(top string) string {
	git := e.git.label()

	// 진행 표시와 git 을 잇는다. 한쪽이 비면 나머지만 남는다.
	right := func(progress string) string {
		switch {
		case progress == "":
			return git
		case git == "":
			return progress
		default:
			return progress + "  " + git
		}
	}

	used := widthOf(top)

	for _, label := range []string{right(e.renderJobText(e.renderJobBar())), right(e.renderJobText("")), git} {
		if label == "" {
			continue
		}

		pad := e.textWidth() - used - widthOf(label)
		if pad < 2 {
			continue
		}

		return top + strings.Repeat(" ", pad) + label
	}

	return top
}

// noticeOr 는 statusBar 아래 줄에 무엇을 쓸지다. 알림이 있으면 그것이 먼저다.
//
// 명령줄·검색은 그 줄을 자기 입력에 쓰므로 이것을 부르지 않는다.
func (e editor) noticeOr(fallback string) string {
	if e.notice != "" {
		return e.notice
	}

	return fallback
}

// renderPosition 은 커서 위치와 전체 줄 수다. normal/insert 의 statusBar 아래 줄이다.
// 줄과 칸은 1 부터 세고, 칸은 byte offset 이 아니라 화면 칸이다.
//
// 커서가 선 줄에 진단이 있으면 뒤에 이어 붙는다. 진단은 커서에 딸린 것이라 커서 위치와
// 성격이 같고, 여기 붙여 두면 오른쪽 끝의 tip 이 남은 칸을 재서 알아서 물러난다
// (diagnostics.go 의 renderDiagnostic, tip.go 의 renderWithTip, ADR-0086).
func (e editor) renderPosition() string {
	buf := e.buffers[e.active]
	col := screenColAt(buf.Lines[buf.Cursor.Line], buf.Cursor.Col, buf.TabWidth())

	position := fmt.Sprintf("%d:%d  (%d 줄)", buf.Cursor.Line+1, col+1, len(buf.Lines))

	diagnostic := e.renderDiagnostic()
	if diagnostic == "" {
		return position
	}

	return position + "  " + diagnostic
}

// renderWithShowcmd 는 statusBar 아래 줄 오른쪽 끝에 치고 있는 키를 붙인다. vim 의 showcmd 와 같은 자리다.
//
// 숫자나 접두 키를 치는 동안 화면에 아무 표시가 없으면 편집기가 그 키를 먹었는지 알 수 없다.
// 붙일 칸이 없으면 아래 줄을 그대로 둔다 — 커서 위치나 명령 결과가 밀려나는 것이 더 나쁘다.
//
// **편집 화면은 이것을 직접 부르지 않는다.** 같은 칸을 tip 과 나눠 쓰므로 renderWithTip 을
// 지나서 온다(tip.go). 여기를 그대로 두는 것은 `:jobs` 같은 판이 tip 없이 이 칸을 쓰기
// 때문이다 — 판의 아래 줄은 그 판의 안내와 알림 자리다(ADR-0061).
func (e editor) renderWithShowcmd(bottom, showcmd string) string {
	if showcmd == "" {
		return bottom
	}

	pad := e.textWidth() - widthOf(bottom) - widthOf(showcmd)
	if pad < 1 {
		return bottom
	}

	return bottom + strings.Repeat(" ", pad) + showcmd
}

// trimLeftToWidth 는 너무 긴 글을 **왼쪽부터** 줄이고 접은 자리에 `…` 를 남긴다.
// truncateToWidth 와 짝이고, **뒤가 살아남아야 하는 자리**가 이것을 쓴다.
//
// 두 곳이 쓴다 — `GOTO` 목록의 경로(파일 이름과 줄 번호가 뒤에 있다) 와 이름 바꾸기 창
// (치고 있는 글자가 뒤에 있다) 이다. 무엇이 뒤에 오는지는 부르는 쪽이 알고, 여기는 자리만 잰다.
func trimLeftToWidth(text string, width int) string {
	if width < 1 {
		return ""
	}
	if widthOf(text) <= width {
		return text
	}

	// `…` 한 칸을 남겨 두고, 들어갈 때까지 앞에서 한 글자씩 뗀다.
	kept := text
	for len(kept) > 0 && widthOf(kept) > width-1 {
		size, _ := glyphAt([]byte(kept), 0, 0, defaultTabWidth)
		kept = kept[size:]
	}

	return "…" + kept
}

// truncateToWidth 는 화면 너비를 넘는 부분을 자른다.
// statusBar 가 넘치면 터미널이 줄바꿈해서 화면이 밀린다.
//
// **색을 입힌 자리를 지난다.** 아래 줄 오른쪽 끝의 tip 이 흐린 글씨라(tip.go), escape 를 폭으로
// 세면 줄이 그만큼 일찍 잘린다. 이 자리가 아래 줄에 색을 못 쓰게 하던 곳이다(ADR-0061).
func truncateToWidth(s string, width int) string {
	if width < 1 {
		return s
	}

	line := []byte(s)

	col, styled := 0, false
	for offset := 0; offset < len(line); {
		if size := escapeSizeAt(line, offset); size > 0 {
			offset, styled = offset+size, true
			continue
		}

		size, w := glyphAt(line, offset, col, defaultTabWidth)
		if col+w > width {
			// 색을 켠 채로 자르면 그 색이 줄 끝까지 번진다.
			if styled {
				return string(line[:offset]) + ansi.ResetStyle
			}
			return string(line[:offset])
		}

		col += w
		offset += size
	}

	return s
}

// bareStatusBar 는 트리가 없는 것으로 치고 그린 statusBar 다.
//
// statusBar 는 sidebar 가 열려 있으면 mode 를 그 아래 칸에 넣고 나머지를 32 칸 들여쓴다
// (ADR-0005). 화면을 통째로 쓰는 판은 트리를 덮으므로 그대로 두면 목록은 왼쪽 끝에서
// 시작하는데 statusBar 만 밀려서 화면이 반쪽만 바뀐 것처럼 보인다.
//
// `:jobs`·`:messages` 도 같은 것을 자기 안에 하나씩 들고 있다. 그 둘을 이리로 데려오는 것은
// 이 기능이 할 일이 아니라 두었다(docs/tasks.md).
func (e editor) bareStatusBar(mode, bottom string) []string {
	bare := e
	bare.sidebar = sidebar{}

	return bare.renderStatusBar(mode, bottom)
}
