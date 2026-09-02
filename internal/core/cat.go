package core

import (
	"bufio"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/bluemir/zn/internal/scheme"
)

// 화면이나 고른 범위를 cat 과 같은 평문으로 터미널에 내보내는 자리다.
//
// **하는 일은 복사·붙여넣기다.** 이 편집기는 mouse 를 켜 두어서 터미널 드래그 복사가 막혀
// 있고(ADR-0012), 긁어낸다 해도 화면에는 줄번호 칸·공백 마커(`»` `⋅`)·문법 색이 섞여 있어
// 쓸 수 있는 글이 아니다(ADR-0020). 여기서 내는 것은 화면이 아니라 **파일의 byte 그대로**다.
//
// 길은 `:!` 가 낸 것을 그대로 쓴다 — `tea.Exec` 가 터미널을 넘겨줄 때 bubbletea 가 대체
// 화면을 나가면서 **마우스 보고까지 끈다.** 그래서 평문이 주 화면에 찍히고, 그 동안 터미널의
// 드래그 복사가 살아난다. 지나간 것은 스크롤백에 남는다 — 대체 화면에는 없는 것이다
// (ADR-0045, ADR-0085).

// catRun 은 tea.Exec 가 터미널을 넘겨주고 돌리는 것이다.
//
// shellRun 과 나란한 모양이고 하는 일만 다르다 — 남의 프로세스를 돌리는 대신 우리가 든 글을
// 찍는다. 둘을 합치지 않은 것은 겹치는 것이 「Enter 를 기다린다」 두 줄뿐이기 때문이다.
type catRun struct {
	// header 는 무엇을 낸 것인지 적는 한 줄이다. 본문 앞에 선다.
	header string
	lines  [][]byte

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (c *catRun) SetStdin(r io.Reader)  { c.stdin = r }
func (c *catRun) SetStdout(w io.Writer) { c.stdout = w }
func (c *catRun) SetStderr(w io.Writer) { c.stderr = w }

// Run 은 글을 찍고 Enter 를 기다린다.
//
// 끝에서 멈춰 서는 것은 shellRun 과 같은 까닭이다 — 멈추지 않으면 돌아가는 순간 대체 화면이
// 출력을 덮는다(ADR-0045).
//
// **줄끝이 두 가지다.** 본문은 `\n` 이고 우리가 두르는 두 줄만 `\r\n` 이다. bubbletea 가
// Run 을 부르기 **전에** 터미널을 되돌려 놓아서(exec.go 의 releaseTerminal) 이 자리는 이미
// cooked 이고 `ONLCR` 이 `\n` 을 알아서 `\r\n` 으로 만든다 — `:!ls` 에서 `ls` 가 내는 맨
// `\n` 이 계단지지 않는 것이 그 증거다. 본문이 `\n` 이어야 「cat 과 같은 형태」다.
// 두르는 두 줄의 `\r` 은 shellRun 이 그렇듯 경계 순간의 보험이라 그대로 둔다.
//
// 한 줄씩 흘려보낸다. `:%cat` 은 파일 전체가 올 수 있어서 통째로 이어 붙이지 않는다.
// 묻기 **전에** 반드시 Flush 한다 — 안 그러면 묻는 줄이 안 보이는 채로 멈춘다.
func (c *catRun) Run() error {
	out := bufio.NewWriter(c.stdout)

	fmt.Fprintf(out, "\r\n%s\r\n", c.header)

	for _, line := range c.lines {
		out.Write(line)
		out.WriteByte('\n')
	}

	fmt.Fprint(out, "\r\n계속하려면 Enter 를 누르세요")

	if err := out.Flush(); err != nil {
		return err
	}

	_, _ = bufio.NewReader(c.stdin).ReadString('\n')

	return nil
}

// runCat 은 범위를 평문으로 내보낸다. `\c`·visual 의 `\c`·`:cat`·팔레트가 모두 여기로 온다.
//
// **글을 먼저 뜬다.** normalMode 가 visual 의 고른 범위를 놓기 때문에(ADR-0037) 그 뒤에
// 뜨면 이미 없다.
//
// 돌아온 뒤에 할 일이 없어서 tea.Exec 에 콜백을 주지 않는다. `:!` 는 종료 코드를 알리고
// 바깥 파일 검사를 다시 걸지만, 이쪽은 남의 프로세스도 아니고 파일을 건드리지도 않았다.
// 돌아갈 곳은 언제나 normal 이다 — 셸에 다녀오는 길과 같은 태도다(ADR-0011, ADR-0045).
func runCat(e *editor, area scheme.MotionRange) (tea.Model, tea.Cmd) {
	buf := *e.activeBuffer()

	run := tea.Exec(&catRun{header: catHeader(buf, area), lines: buf.catLines(area)}, nil)

	model, next := normalMode(e)

	return model, tea.Batch(next, run)
}

// catHeader 는 무엇을 낸 것인지 적는 한 줄이다.
//
// `:!` 가 친 명령을 먼저 찍는 것과 같은 자리다 — 주 화면에는 지난 출력이 그대로 남아 있어서,
// 머리말이 없으면 어느 것의 글인지 갈리지 않는다. 줄 번호는 사람이 세는 대로 1 부터다.
func catHeader(buf viewport, area scheme.MotionRange) string {
	path := "[No Name]"
	if buf.path != "" {
		// 트리나 팔레트로 연 파일은 절대 경로라 그대로 두면 한 줄을 다 먹는다.
		// 줄이는 법은 저장 문구·목록들과 같다(view-locations.go).
		path = shortenPath(buf.path)
	}

	return fmt.Sprintf(":cat %s %d-%d", path, area.Start.Line+1, area.End.Line+1)
}

// visibleRange 는 화면에 보이는 줄들이다. 범위를 대지 않은 `\c`·`:cat`·팔레트가 쓴다.
//
// **wrap 된 줄은 한 조각만 보여도 그 줄 전체가 든다.** 내는 것이 화면 행이 아니라 파일의
// 줄이라서다 — 접힌 자리에서 끊어 내면 붙여넣은 글에 없던 줄바꿈이 생긴다.
//
// 감싸는 머리줄(ADR-0049) 이 덮은 위 몇 행은 눈에 안 보이지만 그대로 든다. 내는 것은
// 「이어진 한 덩이」여야 하고, 머리줄이 든 줄은 화면 밖에 있어서 붙이면 사이가 끊긴다.
func (e *editor) visibleRange() scheme.MotionRange {
	buf := e.activeBuffer()

	rows := buf.visibleRows(e.textHeight())
	if len(rows) == 0 {
		// 그릴 행이 없을 만큼 좁은 화면이다. 커서 줄 하나를 낸다.
		return scheme.MotionRange{Start: scheme.Cursor{Line: buf.cursor.Line}, End: scheme.Cursor{Line: buf.cursor.Line}, Linewise: true}
	}

	first, last := rows[0].line, rows[len(rows)-1].line

	return scheme.MotionRange{Start: scheme.Cursor{Line: first}, End: scheme.Cursor{Line: last}, Linewise: true}
}
