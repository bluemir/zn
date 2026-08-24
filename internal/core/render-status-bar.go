package core

import (
	"fmt"
	"strings"
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
	buf := e.buffers[e.active]

	path := buf.path
	if path == "" {
		path = "[No Name]"
	}
	if buf.dirty {
		path += " [+]"
	}

	// `[!]` 는 마지막으로 맞춰 봤을 때 바깥이 달라져 있었다는 것이다. `[+]` 가 내 손의 미저장
	// 변경이고 이것은 남의 변경이라, 둘이 같이 붙으면 양쪽에 잃을 것이 있다는 뜻이다(ADR-0031).
	if buf.outside != outsideSame {
		path += " [!]"
	}

	// 읽기 전용은 잃을 것이 있다는 표시가 아니라 애초에 고칠 수 없다는 것이다. 정의로 뛰어
	// 열린 표준 라이브러리·의존 모듈의 파일이 이것이다(ADR-0051).
	if buf.readOnly {
		path += " [읽기 전용]"
	}

	// 반전 안에 두어야 색이 왼쪽 끝까지 이어진다.
	// 자르는 것이 채우는 것보다 먼저다 — 두 칸짜리 글자가 경계에 걸치면 통째로 버려진다.
	left, text := "", mode+"  "+path
	if e.sidebarVisible() {
		label := truncateToWidth(mode, sidebarWidth)
		left = label + strings.Repeat(" ", max(0, sidebarWidth-screenWidthOf(label)))
		text = path
	}

	width := e.textWidth()

	// Width 가 남은 칸을 공백으로 채워서 줄 끝까지 색이 간다.
	return []string{
		reverse.Width(e.width).Render(left + e.renderWithStatus(truncateToWidth(text, width))),
		strings.Repeat(" ", e.sidebarLeft()) + truncateToWidth(bottom, width),
	}
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

	used := screenWidthOf(top)

	for _, label := range []string{right(e.renderJobText(e.renderJobBar())), right(e.renderJobText("")), git} {
		if label == "" {
			continue
		}

		pad := e.textWidth() - used - screenWidthOf(label)
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
func (e editor) renderPosition() string {
	buf := e.buffers[e.active]
	col := screenColAt(buf.lines[buf.cursorLine], buf.cursorCol)

	return fmt.Sprintf("%d:%d  (%d 줄)", buf.cursorLine+1, col+1, len(buf.lines))
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

	pad := e.textWidth() - screenWidthOf(bottom) - screenWidthOf(showcmd)
	if pad < 1 {
		return bottom
	}

	return bottom + strings.Repeat(" ", pad) + showcmd
}

// truncateToWidth 는 화면 너비를 넘는 부분을 자른다.
// statusBar 가 넘치면 터미널이 줄바꿈해서 화면이 밀린다.
func truncateToWidth(s string, width int) string {
	if width < 1 {
		return s
	}

	line := []byte(s)
	return string(line[:offsetAtScreenCol(line, width)])
}
