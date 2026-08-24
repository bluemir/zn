package core

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/lsp"
)

// 이름 바꾸기다(ADR-0067).
//
// **부르는 문은 셋이고 하는 일은 여기 하나다** — `\rn`(action.go), `:rename`(view-editor-command.go),
// 팔레트의 「이름 바꾸기」(palette.go). 셋 다 커서 옆에 뜨는 창으로 모이고
// (view-rename-input.go), 이름을 대고 부른 `:rename <새 이름>` 만 그 창을 건너뛴다.
// 어느 문으로 들어와도 묻고 고치는 것은 여기다.

// renameMsg 는 물어본 답이다. 이 시점에 파일은 아직 하나도 바뀌지 않았다.
type renameMsg struct {
	newName string
	files   []lsp.FileEdits
	err     error
}

// startRename 은 커서 자리의 이름을 바꾸면 어디가 달라지는지 묻는다.
//
// 답을 기다리지 않는다 — 잰 값으로 664ms 이고, 기다리면 그동안 편집기가 멈춘다.
// 정의로 가기와 같은 모양이다(gopls.go 의 startDefinition, ADR-0051).
func (e *editor) startRename(newName string) tea.Cmd {
	if e.refuseNoBuffer() {
		return nil
	}

	buf := e.activeBuffer()

	path, ok := goplsPath(buf.path)
	if !ok {
		e.notify("Go 파일에서만 이름을 바꿉니다")

		return nil
	}

	if buf.readOnly {
		e.notify("읽기 전용 파일입니다")

		return nil
	}

	// **서버가 보는 자리 밖이면 거절한다.** 뿌리가 편집기를 연 자리라(gopls.go) 그 밖의
	// 파일은 참조를 찾을 범위에 들지 않는다 — 그때 rename 은 그 파일 안에서만 일어나고,
	// 나머지 참조가 옛 이름으로 남아 **빌드가 조용히 깨진다.** 반쪽짜리보다 안 하는 것이 낫다.
	if !underRoot(e.goplsRoot, path) {
		e.notify("편집기를 연 자리 밖의 파일이라 참조를 다 찾을 수 없습니다: " + shortenPath(e.goplsRoot))

		return nil
	}

	if e.gopls == nil {
		if e.goplsFailed {
			e.notify("gopls 가 없어 이름을 바꿀 수 없습니다")

			return nil
		}

		e.notify("gopls 를 띄우는 중입니다. 잠시 뒤 다시 칩니다")

		return e.startGopls()
	}

	client := e.gopls
	lines := buf.lines
	position := lsp.Position{
		Line:      buf.cursorLine,
		Character: lsp.UTF16Column(buf.lines[buf.cursorLine], buf.cursorCol),
	}

	e.notify("이름을 바꾸는 중입니다")

	return func() tea.Msg {
		// 묻기 직전에 전문으로 맞춘다. 저장하지 않은 편집도 이 한 번으로 서버에 닿는다.
		if err := client.SyncFull(path, lines); err != nil {
			return renameMsg{newName: newName, err: err}
		}

		files, err := client.Rename(path, position, newName)

		return renameMsg{newName: newName, files: files, err: err}
	}
}

// underRoot 는 그 파일이 뿌리 아래에 있는지다. 뿌리를 모르면(서버가 아직 안 떴다) 참이다 —
// 그때는 서버를 띄우는 길로 가고, 뿌리는 그 길에서 정해진다.
func underRoot(root, path string) bool {
	if root == "" {
		return true
	}

	rel, err := filepath.Rel(root, path)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// finishRename 은 답을 받아 파일을 고치고 쓴다.
//
// **여기서 바로 저장한다.** 열려 있지 않은 파일은 buffer 로 들고 있을 자리가 없어서 쓰는 것
// 말고는 길이 없고, 그러면 열린 tab 만 저장하지 않은 채 남는 것이 오히려 갈린다 — 한 번의
// 이름 바꾸기가 저장소를 같은 상태로 만들어야 한다(ADR-0067).
func (e *editor) finishRename(msg renameMsg) {
	if msg.err != nil {
		e.notifyError(msg.err)

		return
	}

	if len(msg.files) == 0 {
		e.notify("바꿀 자리가 없습니다")

		return
	}

	spots, failed := 0, []string{}

	// 파일 순서를 고정한다. 서버가 map 으로 답하는 길이 있어서(lsp/rename.go) 그대로 두면
	// 알림에 적히는 순서가 부를 때마다 달라진다.
	files := slices.Clone(msg.files)
	slices.SortFunc(files, func(a, b lsp.FileEdits) int { return strings.Compare(a.Path, b.Path) })

	for _, file := range files {
		done, err := e.applyRenameTo(file)
		if err != nil {
			failed = append(failed, shortenPath(file.Path)+": "+err.Error())

			continue
		}

		spots += done
	}

	note := fmt.Sprintf("%s (으)로 바꿨습니다: %d 개 파일 %d 곳",
		msg.newName, len(files)-len(failed), spots)

	if len(failed) > 0 {
		// **첫 하나만 적는다.** 아래 줄은 한 줄이고, 나머지는 `:messages` 에 남는다(ADR-0053).
		e.notifyError(errors.Errorf("%s · 못 고친 파일 %d 개 — %s", note, len(failed), failed[0]))

		return
	}

	e.notify(note)
}

// applyRenameTo 는 파일 하나를 고치고 쓴다. 고친 자리 수를 준다.
//
// 열려 있는 tab 은 buffer 에서 고친다. 디스크에서 다시 읽어 고치면 저장하지 않은 편집이
// 사라지고, 화면에 보이는 글과 파일이 갈린다.
//
// 열려 있지 않은 파일은 그 자리에서 열어 고치고 쓴다. tab 으로 만들지 않는다 — 이름 하나를
// 바꾸는 데 tab 이 열 개씩 열리면 그 뒤로 편집기를 쓸 수가 없다.
func (e *editor) applyRenameTo(file lsp.FileEdits) (int, error) {
	if index, ok := e.tabOf(file.Path); ok {
		buf := &e.buffers[index]

		next, done := applyEdits(buf.lines, file.Edits)
		if done == 0 {
			return 0, nil
		}

		buf.replaceAll(next, e.contentWidth())

		// 포매터는 걸지 않는다(hook 이 nil 이다). 이름을 바꾸다가 남의 파일이 통째로
		// 다시 포맷되면 그 diff 가 무엇 때문인지 알 수 없다(ADR-0065).
		if _, err := buf.Save(e.contentWidth(), nil); err != nil {
			return 0, err
		}

		return done, nil
	}

	buf, err := OpenBuffer(file.Path)
	if err != nil {
		return 0, err
	}

	if buf.readOnly {
		return 0, errors.New("읽기 전용입니다")
	}

	next, done := applyEdits(buf.lines, file.Edits)
	if done == 0 {
		return 0, nil
	}

	buf.lines = next

	return done, buf.write()
}

// applyEdits 는 줄 묶음에 편집을 적용한 새 줄 묶음과 고친 자리 수다.
//
// **뒤에서 앞으로 적용한다.** 앞부터 고치면 같은 줄의 뒤쪽 편집이 가리키는 열이 밀린다.
//
// 한 줄 안의 편집만 받는다. 이름은 줄을 넘지 않으므로 여러 줄짜리 편집은 rename 의 답에
// 오지 않는다 — 와도 건너뛴다. 줄 밖을 가리키는 것도 같다(서버가 보던 판과 어긋난 경우다).
//
// 줄 슬라이스만 새로 만든다. 줄 자체는 제자리에서 바뀌지 않는다(ADR-0001).
func applyEdits(lines [][]byte, edits []lsp.TextEdit) ([][]byte, int) {
	sorted := slices.Clone(edits)
	slices.SortFunc(sorted, func(a, b lsp.TextEdit) int {
		if a.Range.Start.Line != b.Range.Start.Line {
			return b.Range.Start.Line - a.Range.Start.Line
		}

		return b.Range.Start.Character - a.Range.Start.Character
	})

	next := slices.Clone(lines)
	done := 0

	for _, edit := range sorted {
		line := edit.Range.Start.Line
		if line != edit.Range.End.Line || line < 0 || line >= len(next) {
			continue
		}

		start := lsp.ByteColumn(next[line], edit.Range.Start.Character)
		end := lsp.ByteColumn(next[line], edit.Range.End.Character)

		start = min(max(start, 0), len(next[line]))
		end = min(max(end, start), len(next[line]))

		replaced := make([]byte, 0, len(next[line])-(end-start)+len(edit.NewText))
		replaced = append(replaced, next[line][:start]...)
		replaced = append(replaced, edit.NewText...)
		replaced = append(replaced, next[line][end:]...)

		next[line] = replaced
		done++
	}

	return next, done
}
