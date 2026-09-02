package textarea

import (
	"os"

	"github.com/cockroachdb/errors"
)

// 디스크에서 읽어 오는 길이다. 밖에서 달라졌는지 맞춰 보고(checkOutside), 다시 읽고(Reload),
// 보던 자리를 새 내용에 옮겨 담는다(adopt) (ADR-0015, ADR-0038, ADR-0044).
//
// **여기 있는 것은 커서나 화면 자리를 만진다.** 글만 다루는 것은
// buffer-reload.go 에 있다 (ADR-0121).

// Reload 는 파일을 다시 읽어 내용을 갈아끼운다. 팔레트의 「파일 다시 읽기」가 쓴다 (ADR-0016).
//
// 다시 읽기를 "이 파일을 새로 연 것" 으로 본다. 저장하지 않은 변경은 사라지고 undo·redo 이력도
// 버린다. 잃을 것이 있을 때 묻는 것은 부르는 쪽이 한다 — 여기까지 왔으면 이미 정해진 것이다.
//
// 커서는 줄 번호와 화면 칸을 이어받는다. 밖에서 포매터를 돌린 뒤 같은 자리에서 이어 보게 된다.
// 파일이 짧아졌으면 범위 안으로 끌어온다.
func (viewport *Viewport) Reload() error {
	if viewport.Path == "" {
		return errors.New("파일 이름이 없습니다")
	}

	data, err := os.ReadFile(viewport.Path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// 없는 파일을 빈 buffer 로 갈아끼우지 않는다. 밖에서 지워졌다면 지금 손에 든 것이
		// 마지막 사본이라, 다시 읽기가 그것을 지우는 명령이 되어서는 안 된다.
		// OpenBuffer 가 없는 파일을 빈 buffer 로 여는 것과 여기서 갈리는 이유다.
		return errors.Errorf("파일이 없습니다: %s", viewport.Path)
	case err != nil:
		return errors.Mark(err, ErrOpenFile)
	}

	*viewport = viewport.Adopt(NewBuffer(viewport.Path, data))

	return nil
}

// adopt 은 새로 읽은 내용에 지금 보고 있던 자리를 옮겨 담는다.
//
// 읽기와 나누기에서 떼어 둔 것은 값이 갈리기 때문이다. 새 Buffer 를 만드는 것은 파일 크기만큼
// 드는 일이라 백그라운드에서 하고(ADR-0044), 자리를 옮겨 담는 것은 값이 없어서 `Update` 안에서
// 해도 된다 — 옮겨 담을 「지금 자리」는 그 순간에만 알 수 있는 것이라 미리 할 수도 없다.
func (viewport Viewport) Adopt(next Viewport) Viewport {
	// 커서 자리를 화면 칸으로 옮겨 둔다. byte offset 은 새 내용에서 다른 글자의 중간일 수 있다.
	// statusBar 가 보여주는 `줄:칸` 이 이 칸이라, 유지되는 것이 눈에 보이는 값과 같다.
	col := ScreenColAt(viewport.Lines[viewport.Cursor.Line], viewport.Cursor.Col, viewport.TabWidth())

	next.Cursor.Line = min(viewport.Cursor.Line, len(next.Lines)-1)
	next.Cursor.Col = OffsetAtScreenCol(next.Lines[next.Cursor.Line], col, viewport.TabWidth())
	next.desiredX = viewport.desiredX

	// 화면도 보고 있던 자리를 유지한다. topRow 는 폭에 따라 있을 수도 없을 수도 있는 행이라
	// 여기서 맞추지 못한다. 부르는 쪽의 scrollTo 가 clampTop 으로 맞춘다.
	next.Top.Line = min(viewport.Top.Line, len(next.Lines)-1)
	next.Top.Row = viewport.Top.Row

	// **editor 가 배정한 크기도 이어받는다.** 새 창은 newBuffer 가 만든 것이라 editor 를
	// 지나오지 않았고, 그대로 두면 폭이 0 이 되어 다시 읽은 순간 본문이 사라진다.
	// 다시 읽기는 editor 가 창을 새로 끼우는 길이 아니라서 layoutViews 가 닿지 않는다
	// (ADR-0123).
	next.Size = viewport.Size

	// 문법 캐시는 새 내용의 것으로 비워지지만 **갈린 횟수는 이어 간다.** 새 Buffer 라 0 에서
	// 다시 세는데, 그러면 다시 읽기 전에 물어 둔 언어 서버의 답이 새 내용에 맞아 보인다
	// (syntaxCache.revision, ADR-0103).
	next.syntax.revision = viewport.syntax.revision + 1

	return next
}
