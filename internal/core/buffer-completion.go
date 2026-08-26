package core

import "github.com/bluemir/zn/internal/lsp"

// 자동완성이 고른 것을 buffer 에 넣는다.
//
// **Buffer 표면에서 `internal/lsp` 를 아는 유일한 자리**라 파일을 따로 두어 그 import 를 여기
// 가둔다. 밖으로 뺄지는 아직 정하지 않았다 (ADR-0066, docs/tasks.md).

// insertCompletion 은 후보 하나를 커서 자리에 넣는다.
//
// **서버가 준 범위(TextEdit) 를 그대로 쓴다.** 이미 친 접두를 그 범위가 덮고 있어서
// (`rand.IntN` 에서 `IntN` 넉 자였다) 우리가 접두를 셀 일이 없다. 범위가 없거나 여러 줄에
// 걸치면 커서 자리에 넣는다 — 여러 줄짜리는 snippet 쪽 이야기이고 우리는 그것을 켜지 않았다
// (lsp/client.go 의 initialize 가 능력을 비워 둔다).
//
// **되돌리기 구간을 닫지 않는다.** insert 에서 친 글자와 한 구간에 있어야 `u` 한 번으로
// 그 insert 가 통째로 돌아간다. vim 과 같다.
func (buf *Buffer) insertCompletion(item lsp.CompletionItem, width int) {
	line := buf.cursorLine
	start, end := buf.cursorCol, buf.cursorCol

	if edit := item.TextEdit; edit != nil &&
		edit.Range.Start.Line == line && edit.Range.End.Line == line {
		start = lsp.ByteColumn(buf.lines[line], edit.Range.Start.Character)
		end = lsp.ByteColumn(buf.lines[line], edit.Range.End.Character)
	}

	// 서버가 보던 판과 지금 판이 어긋났으면 범위가 줄 밖을 가리킬 수 있다.
	start = min(max(start, 0), len(buf.lines[line]))
	end = min(max(end, start), len(buf.lines[line]))

	text := []byte(item.Text())

	next := make([]byte, 0, len(buf.lines[line])-(end-start)+len(text))
	next = append(next, buf.lines[line][:start]...)
	next = append(next, text...)
	next = append(next, buf.lines[line][end:]...)

	buf.beginEdit(line, 1)
	buf.replaceLines(line, 1, [][]byte{next})

	buf.cursorCol = start + len(text)
	buf.updateDesiredCol(width)
}
